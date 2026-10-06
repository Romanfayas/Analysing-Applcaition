package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// Secrets are NEVER stored in source code.
type Config struct {
	// Application
	Env       string
	Port      string
	SecretKey string

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// JWT
	JWTSecret      string
	JWTExpiryHours int

	// Quant Engine
	QuantEngineURL string

	// Trading
	PaperTradingEnabled  bool
	LiveOrderExecution   bool
	BrokerTradingEnabled bool

	// Market Data Providers
	YahooFinanceEnabled bool
	AngelOneAPIKey      string
	AngelOneClientID    string

	// LLM
	LLMProvider  string
	GeminiAPIKey string
	OllamaURL    string

	// Telegram
	TelegramBotToken string
	TelegramChatID   string

	// Email
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	SMTPFrom     string

	// Logging
	LogLevel  string
	LogFormat string

	// Rate Limiting
	RateLimitRPS   int
	RateLimitBurst int
}

// Load reads configuration from environment variables.
// In development, it also loads from .env file if present.
func Load() (*Config, error) {
	// Load .env file if it exists (non-fatal if missing)
	_ = godotenv.Load()

	_ = getEnvBool("PAPER_TRADING_ENABLED", true)
	liveOrder := getEnvBool("LIVE_ORDER_EXECUTION", false)
	brokerTrading := getEnvBool("BROKER_TRADING_ENABLED", false)

	if liveOrder || brokerTrading {
		// Strict invariant validation as required by Phase 7 specs
		panic("SECURITY EXCEPTION: LIVE_ORDER_EXECUTION or BROKER_TRADING_ENABLED cannot be true. Real money trading is strictly disabled.")
	}

	cfg := &Config{
		Env:                 getEnv("APP_ENV", "development"),
		Port:                getEnv("APP_PORT", "8080"),
		SecretKey:           getEnv("APP_SECRET_KEY", ""),
		DatabaseURL:         getEnv("DATABASE_URL", ""),
		RedisURL:            getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:           getEnv("JWT_SECRET", ""),
		JWTExpiryHours:      getEnvInt("JWT_EXPIRY_HOURS", 24),
		QuantEngineURL:      getEnv("QUANT_ENGINE_URL", "http://localhost:8000"),
		PaperTradingEnabled: getEnvBool("PAPER_TRADING_ENABLED", true),
		LiveOrderExecution:  liveOrder,
		BrokerTradingEnabled: brokerTrading,
		YahooFinanceEnabled: getEnvBool("YAHOO_FINANCE_ENABLED", true),
		AngelOneAPIKey:      getEnv("ANGEL_ONE_API_KEY", ""),
		AngelOneClientID:    getEnv("ANGEL_ONE_CLIENT_ID", ""),
		LLMProvider:         getEnv("LLM_PROVIDER", "gemini"),
		GeminiAPIKey:        getEnv("GEMINI_API_KEY", ""),
		OllamaURL:           getEnv("OLLAMA_URL", ""),
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:      getEnv("TELEGRAM_CHAT_ID", ""),
		SMTPHost:            getEnv("SMTP_HOST", ""),
		SMTPPort:            getEnvInt("SMTP_PORT", 587),
		SMTPUser:            getEnv("SMTP_USER", ""),
		SMTPPassword:        getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:            getEnv("SMTP_FROM", ""),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		LogFormat:           getEnv("LOG_FORMAT", "json"),
		RateLimitRPS:        getEnvInt("RATE_LIMIT_RPS", 10),
		RateLimitBurst:      getEnvInt("RATE_LIMIT_BURST", 20),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWTSecret == "" && c.Env == "production" {
		return fmt.Errorf("JWT_SECRET is required in production")
	}
	if c.JWTSecret == "" {
		c.JWTSecret = "dev-jwt-secret-do-not-use-in-production"
	}
	return nil
}

// JWTExpiry returns the JWT expiry duration.
func (c *Config) JWTExpiry() time.Duration {
	return time.Duration(c.JWTExpiryHours) * time.Hour
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
