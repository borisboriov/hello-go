package main

import (
	"fmt"
	"log"
)

func main() {
	fmt.Println("Ledger service started")
	examples := []Transaction{
		{Amount: 12550, Category: "Продукты", Description: "Покупка продуктов", Date: "2026-10-05"},
		{Amount: 6500, Category: "Транспорт", Description: "Поездка на метро", Date: "2026-10-05"},
		{Amount: 35000, Category: "Книги", Description: "Учебник Go", Date: "2026-10-05"},
	}
	for _, tx := range examples {
		if err := AddTransaction(tx); err != nil {
			log.Fatal(err)
		}
	}
	for _, tx := range ListTransactions() {
		fmt.Printf("ID: %d | Сумма: %d коп. | Категория: %s | Описание: %s | Дата: %s\n", tx.ID, tx.Amount, tx.Category, tx.Description, tx.Date)
	}
}
