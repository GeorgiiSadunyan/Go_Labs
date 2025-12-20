package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 1. ТЕСТ КАЛЬКУЛЯТОРА
func TestCalculator(t *testing.T) {
	interpreter := NewInterpreter(nil, nil, nil)

	tests := []struct {
		input    string
		expected float64
	}{
		{"2 + 2", 4},
		{"10 * 5", 50},
		{"100 / 2", 50},
		{"(2 + 2) * 2", 8},
	}

	for _, tt := range tests {
		res, err := interpreter.Execute(tt.input)
		if err != nil {
			t.Errorf("Ошибка при вычислении %s: %v", tt.input, err)
		}
		
		// Приведение типов, так как Execute возвращает interface{}
		val, ok := res.(float64)
		if !ok {
			t.Errorf("Ожидалось число, получено %T", res)
		}

		if val != tt.expected {
			t.Errorf("Для %s ожидалось %f, получено %f", tt.input, tt.expected, val)
		}
	}
}

// 2. ТЕСТ CURL (С фейковым сервером)
func TestCurl(t *testing.T) {
	// Создаем тестовый сервер, который отвечает "Hello from Test"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "Hello from Test")
	}))
	defer ts.Close()

	interpreter := NewInterpreter(nil, nil, nil)
	
	// Выполняем команду curl на наш локальный тестовый адрес
	cmd := "curl " + ts.URL
	res, err := interpreter.Execute(cmd)
	if err != nil {
		t.Fatalf("Ошибка curl: %v", err)
	}

	strRes, ok := res.(string)
	if !ok {
		t.Fatalf("Ожидалась строка, получено %T", res)
	}

	if strRes != "Hello from Test" {
		t.Errorf("Ожидалось 'Hello from Test', получено '%s'", strRes)
	}
}

// 3. ТЕСТ БЕЗОПАСНОСТИ ЗАПУСКА ФАЙЛОВ (Вместо реального запуска)
func TestSafeDirCheck(t *testing.T) {
	// Этот тест проверяет логику findFileInSafeDirs, которая используется перед открытием файлов
	interpreter := NewInterpreter(nil, nil, nil)

	// Попытка открыть файл по абсолютному пути (должна быть ошибка безопасности)
	// В Windows пути с диском (C:\) считаются абсолютными, в Linux - с /
	badPath := "/etc/passwd" 
	if strings.Contains(badPath, ":") { // Простая проверка для Windows теста
		badPath = "C:\\Windows\\System32\\calc.exe"
	}

	_, err := interpreter.findFileInSafeDirs(badPath)
	if err == nil {
		t.Error("Ожидалась ошибка при попытке открыть файл по абсолютному пути, но её нет")
	}

	// Попытка выхода из директории (..)
	_, err = interpreter.findFileInSafeDirs("../secret.txt")
	if err == nil {
		t.Error("Ожидалась ошибка при использовании '..', но её нет")
	}
}

// 4. ТЕСТ DEEPSEEK (Проверка на пустой ввод)
// Реальный запрос делать в тестах не стоит, проверяем валидацию
func TestDeepSeekValidation(t *testing.T) {
	interpreter := NewInterpreter(nil, nil, nil)
	
	// Пустая команда должна вернуть ошибку сразу, не отправляя запрос
	_, err := interpreter.Execute("")
	if err == nil {
		t.Error("Ожидалась ошибка на пустую команду")
	}
}