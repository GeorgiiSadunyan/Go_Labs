package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// === Структуры для DeepSeek API ===

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var SafeDirs = []string{
	"/home/georgiy/Videos",
	"/home/georgiy/Downloads",
	"/home/georgiy/Desktop",
}

var AppPaths = map[string]string{
	"browser": "firefox", // или "firefox", "chromium", "brave"
	"player":  "vlc",           // или "mpv", "mpc-hc"
}

type LaunchCommand struct {
	Type   *string `json:"type"`   // "file", "site", или null
	Target *string `json:"target"` // путь к файлу или URL
	App    *string `json:"app"`    // "vlc", "chrome", и т.д.
}

type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type Interpreter struct {
	variables       map[string]float64
	stringVariables map[string]string // ← новое поле
	history         []string
}

func NewInterpreter(vars map[string]float64, strVars map[string]string, history []string) *Interpreter {
	if vars == nil {
		vars = make(map[string]float64)
	}
	if strVars == nil {
		strVars = make(map[string]string)
	}
	if history == nil {
		history = []string{}
	}
	return &Interpreter{
		variables:       vars,
		stringVariables: strVars,
		history:         history,
	}
}

func (i *Interpreter) Execute(command string) (interface{}, error) {
	if strings.TrimSpace(command) == "" {
		return 0.0, errors.New("пустая команда")
	}

	// Проверка на команду curl
	if strings.HasPrefix(command, "curl ") {
		return i.executeCurl(command)
	}

	// Проверка на команду history
	if command == "history" {
		return nil, errors.New("history") // специальный случай
	}

	// Проверка на присваивание с curl
	if strings.Contains(command, "curl ") {
		parts := strings.SplitN(command, "=", 2)
		if len(parts) == 2 {
			varName := strings.TrimSpace(parts[0])
			curlPart := strings.TrimSpace(parts[1])
			if strings.HasPrefix(curlPart, "curl ") {
				result, err := i.executeCurl(curlPart)
				if err != nil {
					return nil, err
				}
				i.stringVariables[varName] = result
				i.history = append(i.history, command)
				if len(i.history) > 10 {
					i.history = i.history[1:]
				}
				return result, nil
			}
		}
	}

	// Попытка разбора выражения
	parser := NewParser(command)
	node, err := parser.ParseExpression()
	if err != nil {
		// Если ошибка — значит, это не выражение
		// Отправляем в DeepSeek
		result, err := i.classifyAndExecute(command)
		if err != nil {
			return nil, err
		}

		// Добавляем в историю
		i.history = append(i.history, command)
		if len(i.history) > 10 {
			i.history = i.history[1:]
		}

		return result, nil
	}

	// Обработка присваивания
	if assignment, ok := node.(*AssignmentNode); ok {
		result, err := assignment.Value(i.variables, i.stringVariables)
		if err != nil {
			return 0.0, err
		}

		i.history = append(i.history, command)
		if len(i.history) > 10 {
			i.history = i.history[1:]
		}

		return result, nil
	}

	// Обычное выражение
	result, err := node.Value(i.variables, i.stringVariables)
	if err != nil {
		return 0.0, err
	}

	// Добавляем в историю
	i.history = append(i.history, command)
	if len(i.history) > 10 {
		i.history = i.history[1:]
	}

	return result, nil
}

// Новый метод для выполнения curl
func (i *Interpreter) executeCurl(command string) (string, error) {
	// Простой парсер: curl <url>
	parts := strings.SplitN(strings.TrimSpace(command), " ", 2)
	if len(parts) < 2 {
		return "", errors.New("использование: curl <url>")
	}
	url := strings.TrimSpace(parts[1])

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (i *Interpreter) GetVariables() map[string]float64 {
	result := make(map[string]float64, len(i.variables))
	for k, v := range i.variables {
		result[k] = v
	}
	return result
}

func (i *Interpreter) GetStringVariables() map[string]string {
	result := make(map[string]string, len(i.stringVariables))
	for k, v := range i.stringVariables {
		result[k] = v
	}
	return result
}

func (i *Interpreter) GetHistory() []string {
	// Возвращаем копию
	result := make([]string, len(i.history))
	copy(result, i.history)
	return result
}

func (i *Interpreter) sendToDeepSeek(userInput string) (string, error) {
	user := "41-2"
	password := "U0dMUjFs"

	// Basic Auth
	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	// Подготовка сообщений
	messages := []Message{
		{
			Role:    "system",
			Content: "Ты полезный ассистент. Отвечай на вопросы пользователя кратко и по существу.",
		},
		{
			Role:    "user",
			Content: userInput,
		},
	}

	reqBody := ChatCompletionRequest{
		Model:       "deepseek-chat",
		Messages:    messages,
		Temperature: 0.7,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://deproxy.kchugalinskiy.ru/deeproxy/api/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+encodedAuth)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ошибка от API: %d, тело: %s", resp.StatusCode, string(body))
	}

	var apiResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", err
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("API вернул пустой ответ")
	}

	return apiResp.Choices[0].Message.Content, nil
}


type ClassificationResult struct {
	Action *string `json:"action"` // например "сделай краткую сводку"
	URL    *string `json:"url"`    // например "http://example.com"
}

func (i *Interpreter) classifyAndExecute(userInput string) (string, error) {
	// 1. Отправляем пользовательский ввод в DeepSeek для классификации
	classifyPrompt := fmt.Sprintf(`Распознай команду пользователя. Если пользователь просит открыть файл (например, видео) или сайт, извлеки тип (file / site), путь/URL и цель (браузер, проигрыватель и т.п.). Ответь в формате JSON: {"type": "file"/"site", "target": "имя_файла_или_URL", "app": "vlc"/"chrome"/null}. Если команда не подходит — верни {"type": null, "target": null, "app": null}. Команда: %s`, userInput)

	// Подготовка тела запроса вручную, чтобы добавить response_format
	rawReq := map[string]interface{}{
		"model": "deepseek-chat",
		"messages": []Message{
			{
				Role:    "system",
				Content: "Ты классификатор команд. Всегда отвечай в формате JSON.",
			},
			{
				Role:    "user",
				Content: classifyPrompt,
			},
		},
		"temperature": 0.1,
		"response_format": map[string]string{"type": "json_object"}, // ✅ Правильное поле
	}

	jsonData, err := json.Marshal(rawReq)
	if err != nil {
		return "", err
	}

	// === Код аутентификации и отправки запроса (копия из sendToDeepSeek) ===
	user := "41-2" // или другой
	password := "U0dMUjFs" // или другой

	auth := user + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://deproxy.kchugalinskiy.ru/deeproxy/api/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+encodedAuth)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ошибка от API: %d, тело: %s", resp.StatusCode, string(body))
	}

	var apiResp ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", err
	}

	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("API вернул пустой ответ")
	}

	classificationJSON := apiResp.Choices[0].Message.Content

	// === Новая структура для результата классификации ===
	var result LaunchCommand // используем структуру, объявленную ранее
	if err := json.Unmarshal([]byte(classificationJSON), &result); err != nil {
		// Если JSON не удалось распарсить — это не команда, а обычный вопрос
		// Отправляем как обычный вопрос
		return i.sendToDeepSeek(userInput)
	}

	// 2. Проверяем, была ли распознана команда
	if result.Type != nil && result.Target != nil && result.App != nil {
		cmdType := *result.Type
		target := *result.Target
		app := *result.App

		switch cmdType {
		case "file":
			// Найти файл в безопасных директориях
			filePath, err := i.findFileInSafeDirs(target)
			if err != nil {
				return "", err
			}
			// Запустить приложение с файлом
			err = i.launchApp(app, filePath)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("Файл %s успешно открыт в %s", filePath, app), nil

		case "site":
			// Проверим, что target — URL
			if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
				target = "https://" + target
			}

			// Проверим, нужно ли сначала получить содержимое сайта (для сводки и т.п.)
			// Для простоты: если app == "chrome" или "firefox" — просто открываем сайт
			// Если app == "curl" или что-то другое — можно добавить обработку
			if app == "chrome" || app == "firefox" {
				err := i.launchApp(app, target)
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("Сайт %s успешно открыт", target), nil
			} else if app == "curl" {
				// Обработка curl + сводка (старая логика)
				content, err := i.executeCurl("curl " + target)
				if err != nil {
					return "", err
				}
				summaryPrompt := fmt.Sprintf(`На основе следующего содержимого сайта: \n\n%s\n\nДай краткую сводку.`, content)

				// === Отправляем содержимое в DeepSeek для генерации сводки ===
				summaryReq := map[string]interface{}{
					"model": "deepseek-chat",
					"messages": []Message{
						{
							Role:    "system",
							Content: "Ты помощник по анализу содержимого веб-сайтов.",
						},
						{
							Role:    "user",
							Content: summaryPrompt,
						},
					},
					"temperature": 0.7,
				}

				jsonData, err = json.Marshal(summaryReq)
				if err != nil {
					return "", err
				}

				req, err = http.NewRequest("POST", "http://deproxy.kchugalinskiy.ru/deeproxy/api/completions", bytes.NewBuffer(jsonData))
				if err != nil {
					return "", err
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Basic "+encodedAuth)

				resp, err = client.Do(req)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					return "", fmt.Errorf("ошибка от API: %d, тело: %s", resp.StatusCode, string(body))
				}

				var summaryResp ChatCompletionResponse
				if err := json.NewDecoder(resp.Body).Decode(&summaryResp); err != nil {
					return "", err
				}

				if len(summaryResp.Choices) == 0 {
					return "", fmt.Errorf("API вернул пустой ответ")
				}

				return summaryResp.Choices[0].Message.Content, nil
			} else {
				// Неизвестное приложение для сайта
				return fmt.Sprintf("Неизвестное приложение для сайта: %s", app), nil
			}

		default:
			// Неизвестный тип — обычный вопрос
			return i.sendToDeepSeek(userInput)
		}
	} else {
		// Не распознано как команда — обычный вопрос
		return i.sendToDeepSeek(userInput)
	}
}


func (i *Interpreter) findFileInSafeDirs(filename string) (string, error) {
	// Проверим, не является ли filename абсолютным путём (небезопасно)
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("абсолютные пути запрещены")
	}

	// Нормализуем имя файла, чтобы избежать обхода директорий
	filename = filepath.Clean(filename)
	if strings.HasPrefix(filename, "..") {
		return "", fmt.Errorf("путь выходит за пределы безопасных директорий")
	}

	for _, dir := range SafeDirs {
		fullPath := filepath.Join(dir, filename)
		_, err := os.Stat(fullPath)
		if err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("файл не найден в безопасных директориях: %s", filename)
}


func (i *Interpreter) launchApp(appName, arg string) error {
	var cmd *exec.Cmd

	switch appName {
	case "chrome":
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "chrome", arg)
		case "darwin":
			cmd = exec.Command("open", "-a", "Google Chrome", arg)
		case "linux":
			cmd = exec.Command("firefox", arg)
		default:
			return fmt.Errorf("браузер chrome не поддерживается на этой ОС")
		}
	case "firefox":
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", "firefox", arg)
		case "darwin":
			cmd = exec.Command("open", "-a", "Firefox", arg)
		case "linux":
			cmd = exec.Command("firefox", arg)
		default:
			return fmt.Errorf("браузер firefox не поддерживается на этой ОС")
		}
	case "vlc":
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("vlc", arg)
		case "darwin":
			cmd = exec.Command("/Applications/VLC.app/Contents/MacOS/VLC", arg)
		case "linux":
			cmd = exec.Command("vlc", arg)
		default:
			return fmt.Errorf("проигрыватель vlc не поддерживается на этой ОС")
		}
	default:
		return fmt.Errorf("приложение %s не поддерживается", appName)
	}

	return cmd.Start()
}


func (i *Interpreter) executeAction(action, target string) (string, error) {
	action = strings.ToLower(action)
	target = strings.TrimSpace(target)

	if strings.Contains(action, "видео") || strings.Contains(action, "video") {
		// Ищем файл
		filePath, err := i.findFileInSafeDirs(target)
		if err != nil {
			return "", err
		}

		// Запускаем плеер
		playerPath := AppPaths["player"]
		cmd := exec.Command(playerPath, filePath)
		err = cmd.Start() // Start — не ждёт завершения
		if err != nil {
			return "", fmt.Errorf("не удалось запустить плеер: %v", err)
		}

		return fmt.Sprintf("Видео '%s' запущено в %s", filePath, playerPath), nil
	}

	if strings.Contains(action, "браузер") || strings.Contains(action, "browser") || strings.Contains(action, "сайт") || strings.Contains(action, "site") {
		// target — URL
		browserPath := AppPaths["browser"]
		cmd := exec.Command(browserPath, target)
		err := cmd.Start()
		if err != nil {
			return "", fmt.Errorf("не удалось открыть браузер: %v", err)
		}

		return fmt.Sprintf("Сайт '%s' открыт в %s", target, browserPath), nil
	}

	return "", fmt.Errorf("неизвестное действие: %s", action)
}