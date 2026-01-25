package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// Wallet
	PrivateKey     string
	PolygonChainID int
	PolygonRPCURL  string

	// CLOB API credentials
	CLOBApiKey     string
	CLOBSecret     string
	CLOBPassphrase string

	// Telegram notifications (optional)
	TelegramBotToken string
	TelegramChatID   string

	// Trading parameters
	DryRun          bool
	MaxPositionSize float64
	SnipePrice      float64
	TriggerSeconds  int
	MinLiquidity    float64

	// Strategy parameters
	MinConfidence  float64 // Minimum winner confidence (e.g., 0.50 = 50%)
	MaxUncertainty float64 // Max gap between sides to consider uncertain (e.g., 0.10 = 10%)
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// .env file is optional if env vars are set directly
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env file: %w", err)
		}
	}

	cfg := &Config{
		PolygonChainID:  getEnvInt("POLYGON_CHAIN_ID", 137),
		PolygonRPCURL:   getEnvString("POLYGON_RPC_URL", "https://polygon-rpc.com"),
		DryRun:          getEnvBool("DRY_RUN", true),
		MaxPositionSize: getEnvFloat("MAX_POSITION_SIZE", 10),
		SnipePrice:      getEnvFloat("SNIPE_PRICE", 0.99),
		TriggerSeconds:  getEnvInt("TRIGGER_SECONDS", 1),
		MinLiquidity:    getEnvFloat("MIN_LIQUIDITY", 5),
		MinConfidence:   getEnvFloat("MIN_CONFIDENCE", 0.50),
		MaxUncertainty:  getEnvFloat("MAX_UNCERTAINTY", 0.10),
	}

	var missingFields []string

	cfg.PrivateKey = os.Getenv("PRIVATE_KEY")
	if cfg.PrivateKey == "" {
		missingFields = append(missingFields, "PRIVATE_KEY")
	}

	cfg.CLOBApiKey = os.Getenv("CLOB_API_KEY")
	if cfg.CLOBApiKey == "" {
		missingFields = append(missingFields, "CLOB_API_KEY")
	}

	cfg.CLOBSecret = os.Getenv("CLOB_SECRET")
	if cfg.CLOBSecret == "" {
		missingFields = append(missingFields, "CLOB_SECRET")
	}

	cfg.CLOBPassphrase = os.Getenv("CLOB_PASSPHRASE")
	if cfg.CLOBPassphrase == "" {
		missingFields = append(missingFields, "CLOB_PASSPHRASE")
	}

	if len(missingFields) > 0 {
		return nil, fmt.Errorf("missing required config: %v", missingFields)
	}

	// Optional telegram config
	cfg.TelegramBotToken = os.Getenv("TELEGRAM_BOT_TOKEN")
	cfg.TelegramChatID = os.Getenv("TELEGRAM_CHAT_ID")

	return cfg, nil
}

// LoadMinimal loads only basic config without requiring API credentials.
// Useful for commands that only need to query public APIs (e.g., scanner).
func LoadMinimal() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		// .env file is optional
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env file: %w", err)
		}
	}

	return &Config{
		PolygonChainID:  getEnvInt("POLYGON_CHAIN_ID", 137),
		PolygonRPCURL:   getEnvString("POLYGON_RPC_URL", "https://polygon-rpc.com"),
		DryRun:          getEnvBool("DRY_RUN", true),
		MaxPositionSize: getEnvFloat("MAX_POSITION_SIZE", 10),
		SnipePrice:      getEnvFloat("SNIPE_PRICE", 0.99),
		TriggerSeconds:  getEnvInt("TRIGGER_SECONDS", 1),
		MinLiquidity:    getEnvFloat("MIN_LIQUIDITY", 5),
		MinConfidence:   getEnvFloat("MIN_CONFIDENCE", 0.50),
		MaxUncertainty:  getEnvFloat("MAX_UNCERTAINTY", 0.10),
		PrivateKey:      os.Getenv("PRIVATE_KEY"),
	}, nil
}

// LoadWithPrivateKey loads config requiring only the private key.
// Useful for commands that need wallet access but not CLOB API credentials.
func LoadWithPrivateKey() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env file: %w", err)
		}
	}

	cfg := &Config{
		PolygonChainID:  getEnvInt("POLYGON_CHAIN_ID", 137),
		PolygonRPCURL:   getEnvString("POLYGON_RPC_URL", "https://polygon-rpc.com"),
		DryRun:          getEnvBool("DRY_RUN", true),
		MaxPositionSize: getEnvFloat("MAX_POSITION_SIZE", 10),
		SnipePrice:      getEnvFloat("SNIPE_PRICE", 0.99),
		TriggerSeconds:  getEnvInt("TRIGGER_SECONDS", 1),
		MinLiquidity:    getEnvFloat("MIN_LIQUIDITY", 5),
		MinConfidence:   getEnvFloat("MIN_CONFIDENCE", 0.50),
		MaxUncertainty:  getEnvFloat("MAX_UNCERTAINTY", 0.10),
	}

	cfg.PrivateKey = os.Getenv("PRIVATE_KEY")
	if cfg.PrivateKey == "" {
		return nil, errors.New("missing required config: PRIVATE_KEY")
	}

	return cfg, nil
}

// HasTelegram returns true if Telegram notifications are configured
func (c *Config) HasTelegram() bool {
	return c.TelegramBotToken != "" && c.TelegramChatID != ""
}

// Validate performs runtime validation of config values
func (c *Config) Validate() error {
	if c.SnipePrice < 0 || c.SnipePrice > 1 {
		return errors.New("SNIPE_PRICE must be between 0 and 1")
	}
	if c.MaxPositionSize <= 0 {
		return errors.New("MAX_POSITION_SIZE must be greater than 0")
	}
	if c.TriggerSeconds < 0 {
		return errors.New("TRIGGER_SECONDS must be non-negative")
	}
	return nil
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvFloat(key string, defaultVal float64) float64 {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvString(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}
