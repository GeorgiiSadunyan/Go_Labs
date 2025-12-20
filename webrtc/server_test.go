package webrtc

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Тест авторизации (Login)
func TestHandleLogin(t *testing.T) {
	// Подготовка данных
	reqBody := []byte(`{"username": "testuser"}`)
	req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(reqBody))
	rr := httptest.NewRecorder()

	// Вызов функции (хендлера) напрямую
	handler := http.HandlerFunc(handleLogin)
	handler.ServeHTTP(rr, req)

	// Проверка статуса (должен быть 200 OK)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидался статус код 200, получен %v", status)
	}

	// Проверка, что вернулся токен
	var resp LoginResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("Не удалось распарсить ответ: %v", err)
	}
	if resp.Token == "" {
		t.Error("Токен не должен быть пустым")
	}
}

// Тест создания сессии звонка
func TestCreateSession(t *testing.T) {
	// 1. Сначала нужно получить токен (чтобы пройти авторизацию)
	token, _ := generateToken("callerUser")

	// 2. Готовим запрос на создание звонка
	reqBody := []byte(`{"targetUsername": "friendUser", "type": "video"}`)
	req, _ := http.NewRequest("POST", "/api/session", bytes.NewBuffer(reqBody))
	
	// Добавляем заголовок авторизации
	req.Header.Set("Authorization", "Bearer " + token)
	
	rr := httptest.NewRecorder()

	// Оборачиваем хендлер в middleware авторизации
	handler := authMiddleware(handleCreateSession)
	handler.ServeHTTP(rr, req)

	// Проверяем результат
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Ожидался статус код 200, получен %v. Тело: %s", status, rr.Body.String())
	}

	var resp SessionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("Ошибка парсинга JSON: %v", err)
	}

	if resp.Caller != "callerUser" {
		t.Errorf("Неверный caller: %s", resp.Caller)
	}
	if resp.Status != "pending" {
		t.Errorf("Ожидался статус pending, получен %s", resp.Status)
	}
}