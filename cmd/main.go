package main

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"

	"calculator/core"
	"calculator/storage"
	"calculator/ui"
	"calculator/webrtc" 
)

// Хелпер для открытия браузера
func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Ошибка открытия браузера: %v\n", err)
	}
}

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

	if len(history) > 0 {
		console.PrintHistory(history)
	}

	fmt.Println("Доступные команды: webrtc, позвони <имя>, calc <выражение>, history, exit")

	for {
		cmd, err := console.ReadCommand()
		if err != nil { break }
		if cmd == "exit" { break }

		// --- НОВАЯ ЛОГИКА ---
		
		// 1. Команда запуска сервера
		if cmd == "webrtc" {
			ip := webrtc.Start() // Запускаем сервер в фоне, получаем IP
			
			fmt.Println("---------------------------------------------------------")
			fmt.Println("✅ WebRTC Сервер активен (в фоновом режиме)")
			fmt.Printf("🏠 Ссылка для ВАС:   http://localhost:8080\n")
			fmt.Printf("🔗 Ссылка для ДРУГА: http://%s:8080\n", ip)
			fmt.Println("Теперь вы можете использовать команду: позвони <имя>")
			fmt.Println("---------------------------------------------------------")
			continue
		}

		// 2. Команда звонка
		if strings.HasPrefix(cmd, "позвони ") {
			targetName := strings.TrimSpace(strings.TrimPrefix(cmd, "позвони "))
			if targetName == "" {
				fmt.Println("Ошибка: укажите имя. Пример: позвони Ivan")
				continue
			}

			// Формируем умную ссылку:
			// login=George -> сайт сам войдет под именем George
			// target=... -> сайт сам впишет имя друга
			url := fmt.Sprintf("http://localhost:8080?login=George&target=%s", targetName)
			
			fmt.Printf("📞 Звоним пользователю %s...\n", targetName)
			openBrowser(url)
			continue
		}
		// --------------------

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

		switch v := result.(type) {
		case string:
			console.PrintStringResult(v)
		case float64:
			console.PrintResult(v)
		}

		if err := store.Save(interpreter.GetVariables(), interpreter.GetStringVariables(), interpreter.GetHistory()); err != nil {
			console.PrintError(err)
		}
	}
}