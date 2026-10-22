package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func main() {
	fmt.Println("Ledger service started")
	// Разные структуры проверяются через один интерфейс (полиморфизм).
	validationExamples := []Validatable{
		Transaction{Amount: 6500, Category: "Транспорт", Date: "2026-10-21"},
		Budget{Category: "Транспорт", Limit: 150000},
		Transaction{Amount: -100, Category: "Книги"},
		Budget{Category: "Книги", Limit: 0},
	}
	for _, v := range validationExamples {
		if err := CheckValid(v); err != nil {
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
	if err := LoadBudgets(bufio.NewReader(file)); err != nil {
		log.Fatal(err)
	}

	if err := SetBudget(Budget{Category: "Транспорт", Limit: 150000}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Бюджеты загружены. Лимит на транспорт обновлён до 150000 коп.")

	examples := []Transaction{
		{Amount: 200000, Category: "Продукты", Description: "Покупка продуктов", Date: "2026-10-05"},
		{Amount: 300000, Category: "Продукты", Description: "Покупка в пределах остатка", Date: "2026-10-05"},
		{Amount: 10000, Category: "Продукты", Description: "Покупка сверх бюджета", Date: "2026-10-05"},
		{Amount: 6500, Category: "Транспорт", Description: "Поездка на метро", Date: "2026-10-05"},
		{Amount: 35000, Category: "Книги", Description: "Учебник Go", Date: "2026-10-05"},
		{Amount: 0, Category: "Книги", Description: "Нулевая сумма", Date: "2026-10-05"},
	}
	for _, tx := range examples {
		if err := AddTransaction(tx); err != nil {
			fmt.Printf("Отказ: %s — %v\n", tx.Description, err)
			continue
		}
		fmt.Printf("Добавлено: %s\n", tx.Description)
	}
	fmt.Println("Сохранённые транзакции:")
	for _, tx := range ListTransactions() {
		fmt.Printf("ID: %d | Сумма: %d коп. | Категория: %s | Описание: %s | Дата: %s\n", tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
}
