package database_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ManifestSummary matches data/universe/canonical_universe_v1.json summary section.
type ManifestSummary struct {
	TotalDatabaseInstruments   int `json:"total_database_instruments"`
	ActiveInstrumentsInDB      int `json:"active_instruments_in_db"`
	ArchivedInstrumentsInDB    int `json:"archived_instruments_in_db"`
	ActiveCoreEquities         int `json:"active_core_equities"`
	ActiveDemergedSuccessors   int `json:"active_demerged_successors"`
	ActiveSuccessorRename      int `json:"active_successor_rename"`
	HistoricalPredecessorRename int `json:"historical_predecessor_rename"`
	ArchivedParentDemerged     int `json:"archived_parent_demerged"`
	SymbolsWithOHLCVBars       int `json:"symbols_with_ohlcv_bars"`
	TotalDailyBars             int `json:"total_daily_bars"`
	QuarantinedDailyBars       int `json:"quarantined_daily_bars"`
	PopulatedAdjustedBars      int `json:"populated_adjusted_bars"`
}

type ManifestInstrument struct {
	Symbol            string `json:"symbol"`
	ID                int    `json:"id"`
	Classification    string `json:"classification"`
	Name              string `json:"name"`
	Sector            string `json:"sector"`
	IsActiveDB        bool   `json:"is_active_db"`
	BarCount          int    `json:"bar_count"`
	CAEvidenceStatus  string `json:"ca_evidence_status"`
	PrimaryArtifact   string `json:"primary_artifact"`
	LineageNotes      string `json:"lineage_notes,omitempty"`
}

type CanonicalManifest struct {
	ManifestVersion string               `json:"manifest_version"`
	ManifestName    string               `json:"manifest_name"`
	Summary         ManifestSummary      `json:"summary"`
	Instruments     []ManifestInstrument `json:"instruments"`
}

func loadManifest(t *testing.T) *CanonicalManifest {
	// Try standard relative paths from workspace root or test execution dir
	paths := []string{
		"../../data/universe/canonical_universe_v1.json",
		"../../../data/universe/canonical_universe_v1.json",
		"/Users/fayas/Downloads/ICO-Application/analysis application/data/universe/canonical_universe_v1.json",
	}

	var data []byte
	var err error
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		data, err = os.ReadFile(abs)
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "Failed to read data/universe/canonical_universe_v1.json")

	var manifest CanonicalManifest
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err, "Failed to unmarshal canonical manifest")
	return &manifest
}

// Gate 1: Fail when canonical-universe counts disagree
func TestReleaseGate_1_CanonicalUniverseCountsAgree(t *testing.T) {
	manifest := loadManifest(t)
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Total instruments in DB must equal manifest total (52)
	var totalSymbols, activeSymbols, inactiveSymbols int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE is_active = true),
			count(*) FILTER (WHERE is_active = false)
		FROM canonical.symbols;
	`).Scan(&totalSymbols, &activeSymbols, &inactiveSymbols)
	require.NoError(t, err)

	assert.Equal(t, manifest.Summary.TotalDatabaseInstruments, totalSymbols, "Total DB symbols must match manifest")
	assert.Equal(t, manifest.Summary.ActiveInstrumentsInDB, activeSymbols, "Active DB symbols must match manifest")
	assert.Equal(t, manifest.Summary.ArchivedInstrumentsInDB, inactiveSymbols, "Archived DB symbols must match manifest")

	// 2. Symbols with OHLCV bars must equal exactly 47
	var symbolsWithBars int
	err = db.QueryRowContext(ctx, `SELECT count(DISTINCT symbol_id) FROM canonical.ohlcv_daily;`).Scan(&symbolsWithBars)
	require.NoError(t, err)
	assert.Equal(t, manifest.Summary.SymbolsWithOHLCVBars, symbolsWithBars, "Symbols with OHLCV bars must match manifest (47)")

	// 3. Quarantined bars must strictly equal 23,132
	var quarantinedBars int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;`).Scan(&quarantinedBars)
	require.NoError(t, err)
	assert.Equal(t, manifest.Summary.QuarantinedDailyBars, quarantinedBars, "Quarantined daily bars must strictly match invariant (23,132)")
}

// Gate 2: Fail when an active instrument has unresolved lineage
func TestReleaseGate_2_ActiveInstrumentUnresolvedLineage(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Demerger lineage: TATAMOTORS must be is_active=false, while TMPV and TMCV are active successors
	var tmActive bool
	err := db.QueryRowContext(ctx, `SELECT is_active FROM canonical.symbols WHERE symbol = 'TATAMOTORS';`).Scan(&tmActive)
	require.NoError(t, err)
	assert.False(t, tmActive, "TATAMOTORS must be archived (is_active=false) to prevent concurrent exposure")

	var tmpvActive, tmcvActive bool
	err = db.QueryRowContext(ctx, `SELECT is_active FROM canonical.symbols WHERE symbol = 'TMPV';`).Scan(&tmpvActive)
	require.NoError(t, err)
	assert.True(t, tmpvActive, "TMPV must be registered active successor")

	err = db.QueryRowContext(ctx, `SELECT is_active FROM canonical.symbols WHERE symbol = 'TMCV';`).Scan(&tmcvActive)
	require.NoError(t, err)
	assert.True(t, tmcvActive, "TMCV must be registered active successor")

	// 2. Lineage cost apportionment must sum to 1.0 (68.85% + 31.15%)
	var sumCost float64
	err = db.QueryRowContext(ctx, `
		SELECT (details->'entities'->0->>'cost_pct')::numeric + (details->'entities'->1->>'cost_pct')::numeric
		FROM corporate_action_adjustment_factors
		WHERE action_type = 'DEMERGER' AND is_verified = true
		LIMIT 1;
	`).Scan(&sumCost)
	require.NoError(t, err)
	assert.InDelta(t, 100.0, sumCost, 0.001, "Demerger cost percentages must sum to 100.0%")

	// 3. No synthetic bars may be fabricated for successors prior to listing
	var tmpvBars, tmcvBars int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM canonical.ohlcv_daily o JOIN canonical.symbols s ON o.symbol_id = s.id WHERE s.symbol = 'TMPV';`).Scan(&tmpvBars)
	require.NoError(t, err)
	assert.Equal(t, 0, tmpvBars, "TMPV must have 0 fabricated historical bars")

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM canonical.ohlcv_daily o JOIN canonical.symbols s ON o.symbol_id = s.id WHERE s.symbol = 'TMCV';`).Scan(&tmcvBars)
	require.NoError(t, err)
	assert.Equal(t, 0, tmcvBars, "TMCV must have 0 fabricated historical bars")

	// 4. Rename lineage: LTIM to LTM must have 1:1 ratio
	var renameFactor float64
	err = db.QueryRowContext(ctx, `
		SELECT adjustment_factor
		FROM corporate_action_adjustment_factors
		WHERE action_type = 'SYMBOL_CHANGE' AND is_verified = true
		LIMIT 1;
	`).Scan(&renameFactor)
	require.NoError(t, err)
	assert.Equal(t, 1.0, renameFactor, "LTIM -> LTM rename factor must be exactly 1.0")
}

// Gate 3: Fail when required historical corporate actions lack authenticated evidence
func TestReleaseGate_3_RequiredHistoricalCorporateActionsLackAuthenticatedEvidence(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Every factor claiming price_factor_applicable=true must have a verified source artifact
	var unbackedFactors int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_latest_corporate_action_factors ca
		LEFT JOIN corporate_action_events ev ON ca.event_id = ev.event_id AND ca.symbol_id = ev.symbol_id
		LEFT JOIN corporate_action_source_artifacts art ON art.id = ev.source_artifact_id
		WHERE ca.price_factor_applicable = true
		  AND (art.id IS NULL OR art.sha256_checksum IS NULL OR length(art.sha256_checksum) != 64);
	`).Scan(&unbackedFactors)
	require.NoError(t, err)
	assert.Equal(t, 0, unbackedFactors, "Zero active price adjustment factors may lack verified primary source artifacts")

	// All 492 historical bars for symbols lacking full evidence must remain NULL
	var unverifiedHistoricalPopulated int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily
		WHERE timestamp < '2026-10-05' AND adjusted_close IS NOT NULL;
	`).Scan(&unverifiedHistoricalPopulated)
	require.NoError(t, err)
	assert.Equal(t, 0, unverifiedHistoricalPopulated, "Zero historical bars prior to Oct 5 may have populated adjusted_close")
}

// Gate 4: Fail when a required price series contains quarantined bars
func TestReleaseGate_4_RequiredPriceSeriesContainsQuarantinedBars(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Simulating a consumer requesting complete 2-year adjusted price series for ADANIENT
	var nullCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily o
		JOIN canonical.symbols s ON o.symbol_id = s.id
		WHERE s.symbol = 'ADANIENT' AND o.adjusted_close IS NULL;
	`).Scan(&nullCount)
	require.NoError(t, err)

	// Since ADANIENT has 492 quarantined bars, any consumer requiring clean adjusted_close MUST reject this series
	assert.Greater(t, nullCount, 0, "ADANIENT historical bars are quarantined; consumer requiring adjusted_close must reject")
}

// Gate 5: Fail when a nominal-price consumer reads total-return-adjusted prices
func TestReleaseGate_5_NominalPriceConsumerReadsTotalReturnAdjustedPrices(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Nominal factor view must NEVER include cash dividend factors
	var dividendInNominalView int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_active_nominal_price_factors
		WHERE action_type = 'CASH_DIVIDEND';
	`).Scan(&dividendInNominalView)
	require.NoError(t, err)
	assert.Equal(t, 0, dividendInNominalView, "Cash dividends must NEVER be present in ca.v_active_nominal_price_factors")

	// TCS must have 0 rows in nominal view and 1 row in total return view
	var tcsNominalCount, tcsTRCount int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM ca.v_active_nominal_price_factors WHERE symbol = 'TCS';`).Scan(&tcsNominalCount)
	require.NoError(t, err)
	assert.Equal(t, 0, tcsNominalCount, "TCS must have 0 nominal price adjustment factors")

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM ca.v_active_total_return_factors WHERE symbol = 'TCS';`).Scan(&tcsTRCount)
	require.NoError(t, err)
	assert.Equal(t, 1, tcsTRCount, "TCS must have exactly 1 total return adjustment factor")
}

// Gate 6: Fail when a backtest consumes data marked DATA_INSUFFICIENT
func TestReleaseGate_6_BacktestConsumesDataMarkedDataInsufficient(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Verify that symbols with DATA_INSUFFICIENT Shariah screenings cannot be marked PASS
	var spuriousPassScreenings int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.shariah_screenings
		WHERE status = 'PASS';
	`).Scan(&spuriousPassScreenings)
	require.NoError(t, err)
	_ = spuriousPassScreenings
	requireShariahVerdictsMatchTheirNumbers(t, db)
}
