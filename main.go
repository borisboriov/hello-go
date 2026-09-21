package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Println("Пользователь:", os.Getenv("USER"))

	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Аргументы CLI: не переданы")
	} else {
		fmt.Println("Аргументы CLI:")
		for i, arg := range args {
			fmt.Printf("  %d: %s\n", i+1, arg)
		}
	}

	fmt.Println("Версия Go:", runtime.Version())
}
