package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"order-service/pkg/config"
)

// NewPostgresDB only opens and checks a connection. It never installs extensions,
// creates schema, or runs the legacy copied migrations.
func NewPostgresDB(ctx context.Context, cfg *config.Config) (*sqlx.DB, error) {
	u := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(cfg.Postgres.Host, cfg.Postgres.Port),
		User: url.UserPassword(cfg.Postgres.User, cfg.Postgres.Password), Path: "/" + cfg.Postgres.DBName}
	q := url.Values{}
	q.Set("sslmode", cfg.Postgres.SSLMode)
	q.Set("connect_timeout", "5")
	q.Set("default_transaction_read_only", "on")
	q.Set("statement_timeout", "2000")
	u.RawQuery = q.Encode()
	db, err := sqlx.Open("postgres", u.String())
	if err != nil {
		return nil, fmt.Errorf("open Postgres connection failed")
	}
	db.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.Postgres.ConnMaxLifetimeMin) * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("Postgres startup check failed")
	}
	return db, nil
}
