package main

// Validatable задаёт контракт проверки данных перед сохранением.
type Validatable interface {
	Validate() error
}

// CheckValid проверяет любой тип, реализующий Validatable.
func CheckValid(v Validatable) error {
	return v.Validate()
}
