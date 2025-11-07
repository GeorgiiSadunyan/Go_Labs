package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type ConsoleUI struct {
	scanner *bufio.Scanner
}

func NewConsoleUI() *ConsoleUI {
	return &ConsoleUI{
		scanner: bufio.NewScanner(os.Stdin),
	}
}

func (c *ConsoleUI) ReadCommand() (string, error) {
	fmt.Print("> ")
	if !c.scanner.Scan() {
		return "", c.scanner.Err()
	}
	return strings.TrimSpace(c.scanner.Text()), nil
}

func (c *ConsoleUI) PrintResult(result float64) {
	fmt.Println(result)
}

func (c *ConsoleUI) PrintError(err error) {
	fmt.Printf("Ошибка: %v\n", err)
}

func (c *ConsoleUI) PrintHistory(history []string) {
	if len(history) == 0 {
		fmt.Println("История пуста.")
		return
	}
	fmt.Println("Последние команды:")
	for i, cmd := range history {
		fmt.Printf("%d. %s\n", i+1, cmd)
	}
}

func (c *ConsoleUI) PrintStringResult(result string) {
	fmt.Println(result)
}