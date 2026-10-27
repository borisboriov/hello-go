package migrations

import "embed"

// Files позволяет тестам применять ту же схему, что и Goose CLI.
//
//go:embed *.sql
var Files embed.FS
