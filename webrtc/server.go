package webrtc

import (
	_ "embed" // Для встраивания файлов
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"calculator/core" // Импортируем core, чтобы использовать Interpreter

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

// Встраиваем index.html прямо в бинарник
//
//go:embed index.html
var indexHTML []byte

// --- ГЛОБАЛЬНЫЕ ПЕРЕМЕННЫЕ ---
var (
	once        sync.Once
	serverPort  = ":8080"
	jwtKey      = []byte("my_secret_key_for_lab7")
	interpreter *core.Interpreter // Ссылка на наш движок калькулятора
)

// Функция для привязки интерпретатора к серверу
func SetInterpreter(i *core.Interpreter) {
	interpreter = i
}

// --- СТРУКТУРЫ ДАННЫХ ---
type LoginRequest struct {
	Username string `json:"username"`
}
type LoginResponse struct {
	Token string `json:"token"`
}
type CreateSessionRequest struct {
	TargetUsername string `json:"targetUsername"`
	Type           string `json:"type"`
}
type SessionResponse struct {
	SessionID string `json:"sessionId"`
	Caller    string `json:"caller"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}
type AcceptSessionRequest struct {
	SessionID string `json:"sessionId"`
}
type WSSignal struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}
type WSMessage struct {
	Signal    *WSSignal `json:"signal,omitempty"`
	SessionID string    `json:"sessionId,omitempty"`
	Status    string    `json:"status,omitempty"`
	Target    string    `json:"target,omitempty"`
	Type      string    `json:"type,omitempty"`
}

// --- СОСТОЯНИЕ (State) ---
type Session struct {
	ID        string
	Caller    string
	Target    string
	Status    string
	CreatedAt time.Time
}
type ServerState struct {
	Clients     map[string]*websocket.Conn
	Sessions    map[string]*Session
	UserSession map[string]string
	mu          sync.RWMutex
}

var state = ServerState{
	Clients:     make(map[string]*websocket.Conn),
	Sessions:    make(map[string]*Session),
	UserSession: make(map[string]string),
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// --- JWT УТИЛИТЫ ---
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func generateToken(username string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

func validateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}
	return claims, nil
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}
		claims, err := validateToken(parts[1])
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}
		r.Header.Set("X-Username", claims.Username)
		next(w, r)
	}
}

// --- СТРУКТУРЫ ДЛЯ КАЛЬКУЛЯТОРА ---
type CalcRequest struct {
	Command string `json:"command"`
}
type CalcResponse struct {
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// --- НОВЫЕ ХЕНДЛЕРЫ ---

// POST /api/calc
func handleCalc(w http.ResponseWriter, r *http.Request) {
	var req CalcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad JSON", http.StatusBadRequest)
		return
	}

	if interpreter == nil {
		http.Error(w, "Interpreter not initialized", http.StatusInternalServerError)
		return
	}

	// Выполняем команду через ваш core/interpreter.go
	res, err := interpreter.Execute(req.Command)

	resp := CalcResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Result = fmt.Sprintf("%v", res)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GET /api/history
func handleHistory(w http.ResponseWriter, r *http.Request) {
	if interpreter == nil {
		http.Error(w, "Interpreter not initialized", http.StatusInternalServerError)
		return
	}

	history := interpreter.GetHistory()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// --- HTTP HANDLERS ---
func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	token, err := generateToken(req.Username)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(LoginResponse{Token: token})
}

func handleSessionDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handleCreateSession(w, r)
	case http.MethodGet:
		handleGetSession(w, r)
	case http.MethodDelete:
		handleDeleteSession(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleCreateSession(w http.ResponseWriter, r *http.Request) {
	caller := r.Header.Get("X-Username")
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	state.mu.Lock()
	sessionID := fmt.Sprintf("sess_%d", time.Now().UnixNano())
	newSession := &Session{
		ID:        sessionID,
		Caller:    caller,
		Target:    req.TargetUsername,
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	state.Sessions[sessionID] = newSession
	state.UserSession[caller] = sessionID
	state.UserSession[req.TargetUsername] = sessionID
	state.mu.Unlock()

	notifySessionUpdate(newSession)
	json.NewEncoder(w).Encode(SessionResponse{
		SessionID: newSession.ID,
		Caller:    newSession.Caller,
		Target:    newSession.Target,
		Status:    newSession.Status,
		CreatedAt: newSession.CreatedAt.Format(time.RFC3339),
	})
}

func handleGetSession(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	state.mu.RLock()
	sid, ok := state.UserSession[username]
	state.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}
	state.mu.RLock()
	session := state.Sessions[sid]
	state.mu.RUnlock()

	json.NewEncoder(w).Encode(SessionResponse{
		SessionID: session.ID,
		Caller:    session.Caller,
		Target:    session.Target,
		Status:    session.Status,
		CreatedAt: session.CreatedAt.Format(time.RFC3339),
	})
}

func handleAcceptSession(w http.ResponseWriter, r *http.Request) {
	var req AcceptSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	state.mu.Lock()
	session, ok := state.Sessions[req.SessionID]
	if ok {
		session.Status = "active"
		notifySessionUpdate(session)
	}
	state.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"sessionId": req.SessionID, "status": "active"})
}

func handleDeclineSession(w http.ResponseWriter, r *http.Request) {
	var req AcceptSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	state.mu.Lock()
	session, ok := state.Sessions[req.SessionID]
	if ok {
		session.Status = "declined"
		notifySessionUpdate(session)
		delete(state.UserSession, session.Caller)
		delete(state.UserSession, session.Target)
		delete(state.Sessions, req.SessionID)
	}
	state.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]string{"sessionId": req.SessionID, "status": "declined"})
}

func handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	username := r.Header.Get("X-Username")
	state.mu.Lock()
	sid, ok := state.UserSession[username]
	if ok {
		session := state.Sessions[sid]
		session.Status = "cancelled"
		notifySessionUpdate(session)
		delete(state.UserSession, session.Caller)
		delete(state.UserSession, session.Target)
		delete(state.Sessions, sid)
		json.NewEncoder(w).Encode(map[string]string{"sessionId": sid, "status": "cancelled"})
	} else {
		http.Error(w, "No active session", http.StatusNotFound)
	}
	state.mu.Unlock()
}

// --- WEBSOCKET ---
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	claims, err := validateToken(tokenStr)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	username := claims.Username

	state.mu.Lock()
	state.Clients[username] = conn
	state.mu.Unlock()

	defer func() {
		state.mu.Lock()
		delete(state.Clients, username)
		state.mu.Unlock()
		conn.Close()
	}()

	for {
		var msg WSMessage
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}
		if msg.Signal != nil {
			state.mu.RLock()
			sid, ok := state.UserSession[username]
			if !ok {
				state.mu.RUnlock()
				continue
			}
			session := state.Sessions[sid]
			destUser := session.Caller
			if session.Caller == username {
				destUser = session.Target
			}
			if destConn, ok := state.Clients[destUser]; ok {
				destConn.WriteJSON(WSMessage{Signal: msg.Signal})
			}
			state.mu.RUnlock()
		}
	}
}

func notifySessionUpdate(s *Session) {
	msg := WSMessage{
		Type:      "session_updated",
		SessionID: s.ID,
		Status:    s.Status,
		Target:    s.Target,
	}
	sendTo := func(user string) {
		if conn, ok := state.Clients[user]; ok {
			conn.WriteJSON(msg)
		}
	}
	sendTo(s.Caller)
	sendTo(s.Target)
}

func Start() {
	// Внимание: теперь Start блокирует выполнение (убрали go func),
	// так как в Docker это основной процесс
	log.Println("Запуск веб-сервера на :8080...")
	setupRoutes()
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}

func setupRoutes() {
	cors := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			if r.Method == "OPTIONS" {
				return
			}
			h(w, r)
		}
	}

	http.HandleFunc("/api/auth/login", cors(handleLogin))
	http.HandleFunc("/api/session", cors(authMiddleware(handleSessionDispatch)))
	http.HandleFunc("/api/session/accept", cors(authMiddleware(handleAcceptSession)))
	http.HandleFunc("/api/session/decline", cors(authMiddleware(handleDeclineSession)))
	http.HandleFunc("/ws", handleWebSocket)

	// НОВЫЕ РОУТЫ КАЛЬКУЛЯТОРА
	http.HandleFunc("/api/calc", cors(handleCalc))
	http.HandleFunc("/api/history", cors(handleHistory))

	// Отдаем встроенный HTML файл
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexHTML)
	})
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
