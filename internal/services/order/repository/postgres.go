package repository

import "github.com/jmoiron/sqlx"

type PostgresRepository struct{ db *sqlx.DB }

var _ Repository = (*PostgresRepository)(nil)

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository { return &PostgresRepository{db: db} }
func (r *PostgresRepository) InsertOrder() error            { return nil }
func (r *PostgresRepository) GetOrder() error               { return nil }
func (r *PostgresRepository) UpdateStatus() error           { return nil }
func (r *PostgresRepository) ListUserOrders() error         { return nil }
