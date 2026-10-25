// Package migrations holds the database schema as goose SQL migrations.
// It is separate from package store so the server, which never migrates,
// does not include goose.
package migrations

import (
	"context"
	"database/sql"
	"embed"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"
	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Up brings the database schema up to date, returning the migrations applied.
func Up(ctx context.Context, url string) ([]string, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	p, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	var applied []string
	for _, r := range results {
		applied = append(applied, r.Source.Path)
	}
	return applied, err
}
