package postgres

import (
	"fmt"
	"time"

	"order-service/pkg/config"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func NewPostgresDB(cfg *config.Config) (*sqlx.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s", cfg.Postgres.Host, cfg.Postgres.Port, cfg.Postgres.User, cfg.Postgres.Password, cfg.Postgres.DBName, cfg.Postgres.SSLMode)
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMin) * time.Minute)
	_, _ = db.Exec(`CREATE EXTENSION IF NOT EXISTS "pgcrypto"`)
	_, _ = db.Exec(`CREATE EXTENSION IF NOT EXISTS "postgis"`)
	_, _ = db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	return db, nil
}
