// Halal Equity & IPO Research Platform — API Server
//
// This is the main entry point for the Go REST API server.
// It initializes configuration, database connections, middleware,
// and routes, then starts serving HTTP requests.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/auth"
	"github.com/halal-equity/backend/internal/config"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/handlers"
	"github.com/halal-equity/backend/internal/middleware"
	"github.com/halal-equity/backend/internal/services/quant"
)

func main() {
	// Load configuration from environment
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Setup structured logging
	setupLogger(cfg)

	log.Info().
		Str("env", cfg.Env).
		Str("port", cfg.Port).
		Msg("starting halal equity api server")

	// Initialize JWT manager
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTExpiry())

	// Initialize Database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Initialize Quant Client
	quantClient := quant.NewClient(cfg.QuantEngineURL)

	// Setup router
	r := setupRouter(cfg, jwtManager, db, quantClient)

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Info().Msg("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server shutdown error")
		}
	}()

	// Start server
	log.Info().Str("addr", srv.Addr).Msg("server listening")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("server failed")
	}

	log.Info().Msg("server stopped")
}

func setupRouter(cfg *config.Config, jwtManager *auth.JWTManager, db *database.DB, quantClient *quant.Client) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(middleware.RequestLogger(log.Logger))
	r.Use(chiMiddleware.Recoverer)
	r.Use(middleware.RateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst))

	// CORS
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "http://localhost:3001"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check (public)
	r.Handle("/health", &handlers.HealthHandler{})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes
		r.Mount("/auth", handlers.NewAuthHandler().Routes())
		r.Get("/market-data/health", handlers.NewStocksHandler(db, quantClient).GetMarketHealth)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(jwtManager))

			r.Mount("/stocks", handlers.NewStocksHandler(db, quantClient).Routes())
			r.Mount("/ipos", handlers.NewIPOHandler(db, quantClient).Routes())
			r.Mount("/signals", handlers.NewSignalsHandler(db, quantClient).Routes())
			r.Mount("/shariah", handlers.NewShariahHandler(db, quantClient).Routes())
			r.Mount("/portfolio", handlers.NewPortfolioHandler(db, quantClient).Routes())
			r.Mount("/backtesting", handlers.NewBacktestingHandler(db, quantClient).Routes())
			r.Mount("/paper-trading", handlers.NewPaperTradingHandler(db, quantClient).Routes())
			r.Mount("/risk", handlers.NewRiskHandler(db, quantClient).Routes())
			r.Mount("/alerts", handlers.NewAlertsHandler(db, quantClient).Routes())
			r.Mount("/data-quality", handlers.NewDataQualityHandler(db, quantClient).Routes())
			r.Mount("/settings", handlers.NewSettingsHandler(db, quantClient).Routes())
		})
	})

	return r
}

func setupLogger(cfg *config.Config) {
	// Parse log level
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Use console writer in development
	if cfg.IsDevelopment() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	}

	// Add caller info
	log.Logger = log.Logger.With().Caller().Logger()
}
