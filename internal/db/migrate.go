package db

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies pending migrations. It is a separate command, never run
// at web start-up: a start-up migration once ran against a read-only
// production connection in the sister project, and a schema change should
// be a deliberate deploy step.
func Migrate(ctx context.Context, databaseURL string) error {
	conn, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer conn.Close()
	dir, err := fs.Sub(migrations, "migrations") // goose expects the files at the root
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, dir)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
