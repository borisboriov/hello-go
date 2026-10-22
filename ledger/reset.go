package ledger

// Reset очищает хранилище для изоляции тестов. Вызывается только тогда,
// когда другие операции с хранилищем не выполняются.
func Reset() {
	transactions = make([]Transaction, 0)
	budgets = make(map[string]Budget)
}
