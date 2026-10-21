package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/providers/corporateactions/nse"
	"github.com/halal-equity/backend/internal/workers/corporateactions"
)

func main() {
	symbolFlag := flag.String("symbol", "", "Specific symbol to harvest (e.g. TCS)")
	allFlag := flag.Bool("all", false, "Harvest all 47 active symbols")
	dryRunFlag := flag.Bool("dry-run", false, "Run without persisting to database")
	delayMsFlag := flag.Int("delay", 600, "Delay in milliseconds between requests")
	reportFlag := flag.Bool("report", false, "Print universe coverage matrix after run")
	sourceDirFlag := flag.String("source-dir", "", "Controlled statutory artifact directory (e.g. data/corporate_actions/artifacts)")
	fileFlag := flag.String("file", "", "Specific controlled artifact file to ingest")
	flag.Parse()

	if *symbolFlag == "" && !*allFlag && !*reportFlag && *sourceDirFlag == "" && *fileFlag == "" {
		fmt.Println("Usage: ca_harvester [-symbol <SYM>] [-all] [-source-dir <dir>] [-file <path>] [-dry-run] [-delay <ms>] [-report]")
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := getEnv("DB_HOST", "localhost")
		port := getEnv("DB_PORT", "5432")
		user := getEnv("DB_USER", "halal_user")
		password := getEnv("DB_PASSWORD", "devpassword")
		dbname := getEnv("DB_NAME", "halal_equity")
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, password, host, port, dbname)
	}

	db, err := database.NewDB(dsn)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	if *reportFlag && *symbolFlag == "" && !*allFlag && *sourceDirFlag == "" && *fileFlag == "" {
		printCoverageReport(ctx, db)
		return
	}

	client := nse.NewClient(time.Duration(*delayMsFlag) * time.Millisecond)
	adapter := nse.NewAdapter(client)
	harvester := corporateactions.NewHarvester(db, adapter)

	var symbols []string
	if *symbolFlag != "" {
		for _, s := range strings.Split(*symbolFlag, ",") {
			trimmed := strings.TrimSpace(s)
			if trimmed != "" {
				symbols = append(symbols, trimmed)
			}
		}
	}

	opts := corporateactions.HarvesterOptions{
		Symbols:   symbols,
		DryRun:    *dryRunFlag,
		Throttle:  time.Duration(*delayMsFlag) * time.Millisecond,
		SourceDir: *sourceDirFlag,
		FilePath:  *fileFlag,
	}

	summary, err := harvester.Run(ctx, opts)
	if err != nil {
		log.Printf("Harvesting ended with error: %v", err)
	}

	fmt.Println("\n======================================================================")
	fmt.Println("HARVESTING EXECUTION SUMMARY")
	fmt.Println("======================================================================")
	fmt.Printf("Symbols Targeted:    %d\n", summary.SymbolsTargeted)
	fmt.Printf("Symbols Processed:   %d\n", summary.SymbolsProcessed)
	fmt.Printf("Artifacts Retrieved: %d\n", summary.ArtifactsRetrieved)
	fmt.Printf("Events Parsed:       %d\n", summary.EventsParsed)
	fmt.Printf("Events Verified:     %d\n", summary.EventsVerified)
	fmt.Printf("Events Pending:      %d\n", summary.EventsPending)
	fmt.Printf("Rate Limit Hits:     %d\n", summary.RateLimitHits)
	fmt.Printf("Errors Count:        %d\n", summary.ErrorsCount)
	fmt.Printf("Duration:            %.2f seconds\n", summary.DurationSeconds)
	fmt.Println("======================================================================")

	if *reportFlag {
		printCoverageReport(ctx, db)
	}
}

func printCoverageReport(ctx context.Context, db *database.DB) {
	coverage, err := db.GetUniverseHarvestingCoverage(ctx)
	if err != nil {
		log.Fatalf("Failed to query universe coverage: %v", err)
	}

	fmt.Println("\n==========================================================================================================================================================================")
	fmt.Println("CORPORATE ACTION PRIMARY SOURCE COVERAGE MATRIX (CANONICAL UNIVERSE: 52 INSTRUMENTS)")
	fmt.Println("  - Tier-1 Core Equities: 47 Active Symbols with Daily OHLCV Data")
	fmt.Println("  - Lineage Rename Instruments: 2 Symbols (LTIM active successor, LTM active predecessor entry)")
	fmt.Println("  - Demerger Lineage Instruments: 3 Symbols (TATAMOTORS inactive parent, TMPV & TMCV active successors)")
	fmt.Println("==========================================================================================================================================================================")
	fmt.Printf("%-12s | %-16s | %-6s | %-10s | %-8s | %-6s | %-6s | %-6s | %-6s | %-12s | %-12s | %-26s | %-26s\n",
		"SYMBOL", "CATEGORY", "ACTIVE", "ARTIFACTS", "EVENTS", "DIV", "SPLIT", "BONUS", "RIGHTS", "EARLIEST", "LATEST", "COVERAGE STATUS", "DATA READINESS")
	fmt.Println("--------------------------------------------------------------------------------------------------------------------------------------------------------------------------")

	totalArts := 0
	totalEvs := 0
	totalDivs := 0
	totalSplits := 0
	totalBonuses := 0
	totalRights := 0

	coreCount := 0
	lineageCount := 0

	for _, c := range coverage {
		earliestStr := "-"
		latestStr := "-"
		if c.EarliestEventDate != nil {
			earliestStr = c.EarliestEventDate.Format("2006-01-02")
		}
		if c.LatestEventDate != nil {
			latestStr = c.LatestEventDate.Format("2006-01-02")
		}

		activeStr := "YES"
		if !c.IsActive {
			activeStr = "NO"
		}

		if c.InstrumentCategory == "CORE_EQUITY" {
			coreCount++
		} else {
			lineageCount++
		}

		fmt.Printf("%-12s | %-16s | %-6s | %-10d | %-8d | %-6d | %-6d | %-6d | %-6d | %-12s | %-12s | %-26s | %-26s\n",
			c.Symbol, c.InstrumentCategory, activeStr, c.ArtifactsCount, c.EventsCount, c.DividendsCount, c.SplitsCount, c.BonusesCount, c.RightsCount,
			earliestStr, latestStr, c.CoverageStatus, c.DataReadinessState)

		totalArts += c.ArtifactsCount
		totalEvs += c.EventsCount
		totalDivs += c.DividendsCount
		totalSplits += c.SplitsCount
		totalBonuses += c.BonusesCount
		totalRights += c.RightsCount
	}
	fmt.Println("--------------------------------------------------------------------------------------------------------------------------------------------------------------------------")
	fmt.Printf("%-12s | Core: %-2d, Lin: %-2d | %-6s | %-10d | %-8d | %-6d | %-6d | %-6d | %-6d | %-12s | %-12s | %-26s | %-26s\n",
		"TOTAL (52)", coreCount, lineageCount, "51/1", totalArts, totalEvs, totalDivs, totalSplits, totalBonuses, totalRights, "-", "-", "-", "-")
	fmt.Println("==========================================================================================================================================================================")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
