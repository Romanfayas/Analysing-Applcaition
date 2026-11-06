package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

const (
	// AppDatabaseName is the production/application database name.
	AppDatabaseName = "halal_equity"
	// TestDatabaseName is the isolated test database name.
	TestDatabaseName = "halal_equity_test"
)

// GetIsolatedTestDB returns a connection strictly pointing to the isolated test database.
func GetIsolatedTestDB(t *testing.T) *DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "192.168.18.171"
	}
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://halal_user:devpassword@" + host + ":5432/" + TestDatabaseName + "?sslmode=disable"
	}

	sqlDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Skipping test: isolated test database %s not accessible: %v", TestDatabaseName, err)
	}

	AssertNotApplicationDB(sqlDB, t)

	sqlDB.SetMaxOpenConns(5)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return &DB{DB: sqlDB}
}

// CheckDatabaseName evaluates any raw database name string against the protected application database.
func CheckDatabaseName(dbName string) error {
	cleaned := strings.ToLower(strings.TrimSpace(dbName))
	if cleaned == strings.ToLower(AppDatabaseName) {
		return fmt.Errorf("HARD SECURITY GUARD: detected application database target '%s'", dbName)
	}
	return nil
}

// CheckDatabaseGuard inspects the current database of sqlDB and returns an error if it points to AppDatabaseName.
func CheckDatabaseGuard(sqlDB *sql.DB) error {
	var currentDB string
	err := sqlDB.QueryRow("SELECT current_database();").Scan(&currentDB)
	if err != nil {
		return err
	}

	return CheckDatabaseName(currentDB)
}

// AssertNotApplicationDB is a hard security guard that fails closed if any fixture-writing test
// targets the production/application database 'halal_equity'.
func AssertNotApplicationDB(sqlDB *sql.DB, t *testing.T) {
	if err := CheckDatabaseGuard(sqlDB); err != nil {
		panic(fmt.Sprintf("%v! Integration tests must run against isolated database '%s'.", err, TestDatabaseName))
	}
}

// WithIsolatedTx executes a test callback inside a transaction on db that is guaranteed to rollback,
// preventing residual records across tests.
func WithIsolatedTx(t *testing.T, db *DB, fn func(tx *sql.Tx)) {
	ctx := context.Background()
	tx, err := db.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() {
		_ = tx.Rollback()
	}()

	fn(tx)
}
