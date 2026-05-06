package postgres

import (
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	mp "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
)

func RunMigrations(db *sqlx.DB, migrationsPath string) error {
	raw := db.DB
	var err error
	driver, err := mp.WithInstance(raw, &mp.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithDatabaseInstance(fmt.Sprintf("file://%s", migrationsPath), "postgres", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
