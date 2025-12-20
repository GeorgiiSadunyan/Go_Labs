package main

import (
	"log"

	"calculator/core"
	"calculator/storage"
	"calculator/webrtc"
)

func main() {
	// 1. Загружаем данные
	store := storage.NewFileStorage("calculator_state.json")
	numVars, strVars, history, err := store.Load()
	if err != nil {
		log.Printf("Новая история создана: %v", err)
		numVars = make(map[string]float64)
		strVars = make(map[string]string)
		history = []string{}
	}

	// 2. Инициализируем ядро
	interpreter := core.NewInterpreter(numVars, strVars, history)

	// 3. Передаем интерпретатор в веб-сервер
	webrtc.SetInterpreter(interpreter)

	// 4. Запускаем сервер (теперь это блокирующая операция)
	// В Docker контейнере программа будет висеть здесь и слушать порт
	webrtc.Start()
}
