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
)

// === Структуры для DeepSeek API ===

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
		result, err := i.sendToDeepSeek(command)
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
