package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// DB wraps the SQL database connection with convenience methods.
type DB struct {
	*sql.DB
}

// NewDB creates a new database connection.
func NewDB(connStr string) (*DB, error) {
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("[DB] Connected successfully")
	return &DB{db}, nil
}

// BuildConnString builds a PostgreSQL connection string from config.
func BuildConnString(host string, port int, user, password, dbname, sslmode string) string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)
}

// RunMigrations executes the init.sql migration file.
func (db *DB) RunMigrations(sqlContent string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, sqlContent)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("[DB] Migrations applied successfully")
	return nil
}

// HealthCheck verifies the database is reachable.
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.PingContext(ctx)
}
