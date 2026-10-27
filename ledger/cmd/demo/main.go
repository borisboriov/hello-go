package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/borisboriov/hello-go/ledger"
)

func main() {
	if err := ledger.Open(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer ledger.Close()
	fmt.Println("Ledger service started")
	// Разные структуры проверяются через один интерфейс (полиморфизм).
	validationExamples := []ledger.Validatable{
		ledger.Transaction{Amount: 6500, Category: "Транспорт", Date: "2026-10-21"},
		ledger.Budget{Category: "Транспорт", Limit: 150000},
		ledger.Transaction{Amount: -100, Category: "Книги"},
		ledger.Budget{Category: "Книги", Limit: 0},
	}
	for _, v := range validationExamples {
		if err := ledger.CheckValid(v); err != nil {
			fmt.Printf("Проверка %T: %v\n", v, err)
			continue
		}
		fmt.Printf("Проверка %T: данные корректны\n", v)
	}

	file, err := os.Open("budgets.json")
	if err != nil {
		log.Fatalf("Не удалось открыть budgets.json: %v", err)
	}
	defer file.Close()
	if err := ledger.LoadBudgets(bufio.NewReader(file)); err != nil {
		log.Fatal(err)
	}

	if err := ledger.SetBudget(ledger.Budget{Category: "Транспорт", Limit: 150000}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Бюджеты загружены. Лимит на транспорт обновлён до 150000 коп.")

	examples := []ledger.Transaction{
		{Amount: 200000, Category: "Продукты", Description: "Покупка продуктов", Date: "2026-10-05"},
		{Amount: 300000, Category: "Продукты", Description: "Покупка в пределах остатка", Date: "2026-10-05"},
		{Amount: 10000, Category: "Продукты", Description: "Покупка сверх бюджета", Date: "2026-10-05"},
		{Amount: 6500, Category: "Транспорт", Description: "Поездка на метро", Date: "2026-10-05"},
		{Amount: 35000, Category: "Книги", Description: "Учебник Go", Date: "2026-10-05"},
		{Amount: 0, Category: "Книги", Description: "Нулевая сумма", Date: "2026-10-05"},
	}
	for _, tx := range examples {
		if err := ledger.AddTransaction(tx); err != nil {
			fmt.Printf("Отказ: %s — %v\n", tx.Description, err)
			continue
		}
		fmt.Printf("Добавлено: %s\n", tx.Description)
	}
	fmt.Println("Сохранённые транзакции:")
	transactions, err := ledger.ListTransactions()
	if err != nil {
		log.Fatal(err)
	}
	for _, tx := range transactions {
		fmt.Printf("ID: %d | Сумма: %d коп. | Категория: %s | Описание: %s | Дата: %s\n", tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
}
