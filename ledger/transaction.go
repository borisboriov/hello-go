package main

import "errors"

// Transaction представляет транзакцию. Amount хранит сумму в копейках.
type Transaction struct {
	ID          int
	Amount      int
	Category    string
	Description string
	Date        string
}

var transactions = make([]Transaction, 0)

// AddTransaction проверяет сумму и назначает ID перед сохранением.
func AddTransaction(tx Transaction) error {
	if tx.Amount == 0 {
		return errors.New("transaction amount must not be zero")
	}
	tx.ID = len(transactions) + 1
	transactions = append(transactions, tx)
	return nil
}

// ListTransactions возвращает копию хранилища.
func ListTransactions() []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	return result
}
