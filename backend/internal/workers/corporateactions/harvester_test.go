package corporateactions_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/workers/corporateactions"
)

func getTestDB(t *testing.T) *database.DB {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "192.168.18.43" // Default to Docker host LAN IP to avoid port 5432 native postgres conflict
		}
		connStr = "postgres://halal_user:devpassword@" + host + ":5432/halal_equity?sslmode=disable"
	}

	sqlDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Skipping integration test; postgres database not accessible: %v", err)
	}

	db, err := database.NewDB(connStr)
	require.NoError(t, err)
	return db
}

func TestHarvester_IngestArtifactFile_Idempotent(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	harvester := corporateactions.NewHarvester(db, nil)

	// Create temporary fixture artifact
	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "NSE_CA_TESTHARV_PRIMARY.json")
	content := `[
		{
			"symbol": "TCS",
			"comp": "Tata Consultancy Services Limited",
			"series": "EQ",
			"faceVal": "1",
			"isin": "INE467B01029",
			"subject": "Interim Dividend - Rs 12 Per Share",
			"exDate": "14-Oct-2026",
			"recDate": "14-Oct-2026"
		}
	]`
	err := os.WriteFile(fixturePath, []byte(content), 0644)
	require.NoError(t, err)

	// Run 1: Ingest
	res1, err1 := harvester.IngestArtifactFile(ctx, fixturePath, false)
	require.NoError(t, err1)
	require.NotNil(t, res1)
	assert.Equal(t, "TCS", res1.Symbol)
	assert.Len(t, res1.Events, 1)

	// Run 2: Re-ingest exact same file (idempotency test)
	res2, err2 := harvester.IngestArtifactFile(ctx, fixturePath, false)
	require.NoError(t, err2)
	require.NotNil(t, res2)
	assert.Equal(t, res1.Artifact.SHA256Checksum, res2.Artifact.SHA256Checksum)
	assert.Equal(t, len(res1.Events), len(res2.Events))

	// Verify events in database
	events, err := db.GetCorporateActionEvents(ctx, "TCS")
	require.NoError(t, err)
	assert.NotEmpty(t, events)
}

func TestHarvester_DryRun(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	harvester := corporateactions.NewHarvester(db, nil)

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "NSE_CA_DRYRUN_PRIMARY.json")
	content := `[
		{
			"symbol": "INFY",
			"comp": "Infosys Limited",
			"series": "EQ",
			"faceVal": "5",
			"isin": "INE009A01021",
			"subject": "Dividend - Rs 25 Per Share",
			"exDate": "10-Jun-2026",
			"recDate": "10-Jun-2026"
		}
	]`
	err := os.WriteFile(fixturePath, []byte(content), 0644)
	require.NoError(t, err)

	res, err := harvester.IngestArtifactFile(ctx, fixturePath, true)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "INFY", res.Symbol)
	assert.Len(t, res.Events, 1)
}

func TestHarvester_NonExistentFile(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	ctx := context.Background()
	harvester := corporateactions.NewHarvester(db, nil)

	_, err := harvester.IngestArtifactFile(ctx, "/path/does/not/exist.json", false)
	assert.Error(t, err)
}
