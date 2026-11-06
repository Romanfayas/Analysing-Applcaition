package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrUserExists is returned when registering an email that is already taken.
var ErrUserExists = errors.New("user already exists")

// ErrUserNotFound is returned when no active user matches the lookup.
var ErrUserNotFound = errors.New("user not found")

// UserRecord is a platform user account.
type UserRecord struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// NormalizeEmail canonicalizes an email address for storage and lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// CreateUser stores a new account. The password must already be hashed.
func (db *DB) CreateUser(ctx context.Context, email, name, passwordHash string) (*UserRecord, error) {
	var u UserRecord
	err := db.QueryRowContext(ctx, `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		ON CONFLICT (email) DO NOTHING
		RETURNING id, email, name, password_hash, is_active, created_at
	`, NormalizeEmail(email), strings.TrimSpace(name), passwordHash).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserExists
	}
	if err != nil {
		return nil, fmt.Errorf("create user failed: %w", err)
	}
	return &u, nil
}

// GetUserByEmail returns an account by email (active or not; callers must check IsActive).
func (db *DB) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	var u UserRecord
	err := db.QueryRowContext(ctx, `
		SELECT id, email, name, password_hash, is_active, created_at
		FROM users WHERE email = $1
	`, NormalizeEmail(email)).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user failed: %w", err)
	}
	return &u, nil
}

// GetUserByID returns an account by ID.
func (db *DB) GetUserByID(ctx context.Context, id uuid.UUID) (*UserRecord, error) {
	var u UserRecord
	err := db.QueryRowContext(ctx, `
		SELECT id, email, name, password_hash, is_active, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user failed: %w", err)
	}
	return &u, nil
}
