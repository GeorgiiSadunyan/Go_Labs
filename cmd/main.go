package main

import (
	"log"

	"calculator/core"
	"calculator/storage"
	"calculator/ui"
)

func main() {
	store := storage.NewFileStorage("calculator_state.json")
	numVars, strVars, history, err := store.Load()
	if err != nil {
		log.Printf("Не удалось загрузить состояние: %v", err)
		numVars = make(map[string]float64)
		strVars = make(map[string]string)
		history = []string{}
	}

	interpreter := core.NewInterpreter(numVars, strVars, history)
	console := ui.NewConsoleUI()

	// Выводим историю при запуске
	if len(history) > 0 {
		console.PrintHistory(history)
	}

	for {
		cmd, err := console.ReadCommand()
		if err != nil {
			break
		}

		if cmd == "exit" {
			break
		}

		if cmd == "history" {
			console.PrintHistory(interpreter.GetHistory())
			continue
		}

		result, err := interpreter.Execute(cmd)
		if err != nil {
			if err.Error() == "history" {
				console.PrintHistory(interpreter.GetHistory())
				continue
			}
			console.PrintError(err)
			continue
		}

		// Вывод результата
		switch v := result.(type) {
		case string:
			console.PrintStringResult(v)
		case float64:
			console.PrintResult(v)
		}

		// Сохраняем состояние
		if err := store.Save(interpreter.GetVariables(), interpreter.GetStringVariables(), interpreter.GetHistory()); err != nil {
			console.PrintError(err)
		}
	}
}