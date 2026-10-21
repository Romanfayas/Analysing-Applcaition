package nse_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/providers/corporateactions/nse"
)

func TestBuildArtifact_SHA256(t *testing.T) {
	rawPayload := []byte(`[{"symbol":"TCS","subject":"Interim Dividend - Rs 12 Per Share","exDate":"14-Oct-2026"}]`)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	art := nse.BuildArtifact("TCS", "https://example.com/api", rawPayload, now)
	require.NotNil(t, art)

	// Verify SHA-256 calculation independently
	hasher := sha256.New()
	hasher.Write(rawPayload)
	expectedHex := hex.EncodeToString(hasher.Sum(nil))

	assert.Equal(t, expectedHex, art.SHA256Checksum)
	assert.Equal(t, "NSE_INDIA", art.IssuingAuthority)
	assert.Equal(t, "EXCHANGE_API_JSON", art.DocumentType)
	assert.Equal(t, "RAW_RETRIEVED", art.ValidationStatus)
	assert.Equal(t, "v1.8.11", art.ParserVersion)
	assert.Contains(t, art.ArtifactID, "NSE_CA_TCS_")
}

func TestDecodeRawJSON(t *testing.T) {
	rawPayload := []byte(`[
		{"symbol":"INFY","comp":"Infosys Limited","subject":"Dividend - Rs 25 Per Share","exDate":"10-Jun-2026"},
		{"symbol":"INFY","comp":"Infosys Limited","subject":"Bonus 1:1","exDate":"15-Jul-2025"}
	]`)

	items, err := nse.DecodeRawJSON(rawPayload)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "INFY", items[0].Symbol)
	assert.Equal(t, "Dividend - Rs 25 Per Share", items[0].Subject)
	assert.Equal(t, "Bonus 1:1", items[1].Subject)

	// Malformed JSON
	_, err2 := nse.DecodeRawJSON([]byte(`not json`))
	assert.Error(t, err2)
}
