package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// Wallet
	PrivateKey         string
	ProxyWalletAddress string // Polymarket proxy wallet (Gnosis Safe), empty = EOA mode
	SignatureType      int    // 0=EOA, 1=POLY_PROXY (email/Google), 2=GNOSIS_SAFE (browser wallet)
	PolygonChainID     int
	PolygonRPCURL      string

	// CLOB API credentials
	CLOBApiKey     string
	CLOBSecret     string
	CLOBPassphrase string

	// Proxy (optional) - supports multiple proxies comma-separated
	ProxyURL  string   // Single proxy (legacy): user:pass@host:port
	ProxyURLs []string // Multiple proxies for rotation

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

	// Black Swan strategy parameters ($15 bankroll optimized)
	BlackSwanMaxPrice     float64 // Max price to consider (default: 0.10 = 10¢)
	BlackSwanMinPrice     float64 // Min price to avoid dust (default: 0.005 = 0.5¢)
	BlackSwanBetPercent   float64 // Bankroll percentage per bet (default: 0.05 = 5%)
	BlackSwanMaxPositions int     // Maximum concurrent open positions (default: 10)
	BlackSwanMaxExposure  float64 // Maximum total exposure in USD (default: 10)
	BlackSwanBidDiscount  float64 // How far below market to bid (default: 0.25 = 25%)
	BlackSwanMinVolume    float64 // Minimum market volume to consider (default: 100)
	BlackSwanMaxVolume    float64 // Maximum market volume (avoid liquid markets) (default: 10000)
	BlackSwanMaxDays      int     // Maximum days until resolution (default: 30) - prefer fast-resolving markets

	// Weather sniper strategy parameters (dynamic sizing)
	WeatherBalance        float64 // Your actual USDC balance (set this! 0 = try API)
	WeatherBankroll       float64 // Fallback if balance not set and API fails
	WeatherMinEdge        float64 // Minimum edge to trade (default: 0.10 = 10%)
	WeatherMinConfidence  float64 // Minimum confidence in forecast (default: 0.70 = 70%)
	WeatherMaxPosition    float64 // Maximum position size per trade (default: 5.00)
	WeatherBetPercent     float64 // Balance percentage per bet (default: 0.20 = 20%)
	WeatherDailyLossLimit float64 // Daily loss limit (default: 10.00)
	WeatherMaxTrades      int     // Maximum concurrent trades (default: 10)
	WeatherMaxExposure    float64 // Maximum total exposure (default: 50.00)
	WeatherMinVolume      float64 // Minimum market volume (default: 500)
	WeatherMaxSpread      float64 // Maximum bid-ask spread (default: 0.05 = 5%)
	WeatherBidDiscount    float64 // How far below market to bid (default: 0.12 = 12%)
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
		MaxPositionSize: getEnvFloat("MAX_POSITION_SIZE", 15),
		SnipePrice:      getEnvFloat("SNIPE_PRICE", 0.99),
		TriggerSeconds:  getEnvInt("TRIGGER_SECONDS", 1),
		MinLiquidity:    getEnvFloat("MIN_LIQUIDITY", 5),
		MinConfidence:   getEnvFloat("MIN_CONFIDENCE", 0.50),
		MaxUncertainty:  getEnvFloat("MAX_UNCERTAINTY", 0.10),

		// Black Swan defaults ($15 bankroll optimized)
		BlackSwanMaxPrice:     getEnvFloat("BLACKSWAN_MAX_PRICE", 0.10),
		BlackSwanMinPrice:     getEnvFloat("BLACKSWAN_MIN_PRICE", 0.001), // 0.1¢ minimum
		BlackSwanBetPercent:   getEnvFloat("BLACKSWAN_BET_PERCENT", 0.05),
		BlackSwanMaxPositions: getEnvInt("BLACKSWAN_MAX_POSITIONS", 10),
		BlackSwanMaxExposure:  getEnvFloat("BLACKSWAN_MAX_EXPOSURE", 10),
		BlackSwanBidDiscount:  getEnvFloat("BLACKSWAN_BID_DISCOUNT", 0.25),
		BlackSwanMinVolume:    getEnvFloat("BLACKSWAN_MIN_VOLUME", 100),
		BlackSwanMaxVolume:    getEnvFloat("BLACKSWAN_MAX_VOLUME", 10000),
		BlackSwanMaxDays:      getEnvInt("BLACKSWAN_MAX_DAYS", 30), // Prefer markets resolving within 30 days

		// Weather sniper defaults (dynamic sizing - uses actual balance)
		// Note: Polymarket requires minimum 5 shares per order
		// Set WEATHER_BALANCE to your actual USDC balance for accurate sizing
		WeatherBalance:        getEnvFloat("WEATHER_BALANCE", 0),             // Your balance (0 = try API)
		WeatherBankroll:       getEnvFloat("WEATHER_BANKROLL", 15),           // Fallback if balance not set
		WeatherMinEdge:        getEnvFloat("WEATHER_MIN_EDGE", 0.10),         // 10% minimum edge
		WeatherMinConfidence:  getEnvFloat("WEATHER_MIN_CONFIDENCE", 0.70),   // 70% confidence
		WeatherMaxPosition:    getEnvFloat("WEATHER_MAX_POSITION", 5.00),     // $5 max per trade (5 shares at $1)
		WeatherBetPercent:     getEnvFloat("WEATHER_BET_PERCENT", 0.20),      // 20% of balance per bet
		WeatherDailyLossLimit: getEnvFloat("WEATHER_DAILY_LOSS_LIMIT", 10.0), // Higher for larger bankrolls
		WeatherMaxTrades:      getEnvInt("WEATHER_MAX_TRADES", 10),
		WeatherMaxExposure:    getEnvFloat("WEATHER_MAX_EXPOSURE", 50.00),    // Higher for larger bankrolls
		WeatherMinVolume:      getEnvFloat("WEATHER_MIN_VOLUME", 500),
		WeatherMaxSpread:      getEnvFloat("WEATHER_MAX_SPREAD", 0.05), // 5% max spread
		WeatherBidDiscount:    getEnvFloat("WEATHER_BID_DISCOUNT", 0.12),
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

	// Optional proxy config - supports comma-separated list
	proxyEnv := os.Getenv("PROXY_URL")
	if proxyEnv != "" {
		proxies := strings.Split(proxyEnv, ",")
		for _, p := range proxies {
			p = strings.TrimSpace(p)
			if p != "" {
				cfg.ProxyURLs = append(cfg.ProxyURLs, p)
			}
		}
		if len(cfg.ProxyURLs) > 0 {
			cfg.ProxyURL = cfg.ProxyURLs[0] // First proxy as default
		}
	}

	// Optional proxy wallet (Gnosis Safe)
	cfg.ProxyWalletAddress = os.Getenv("PROXY_WALLET_ADDRESS")

	// Signature type: 0=EOA, 1=POLY_PROXY (email/Google login), 2=GNOSIS_SAFE (browser wallet)
	// Default to 2 (GNOSIS_SAFE) if proxy wallet is set, as most users connect via browser wallet
	if sigTypeStr := os.Getenv("SIGNATURE_TYPE"); sigTypeStr != "" {
		sigType, err := strconv.Atoi(sigTypeStr)
		if err != nil || sigType < 0 || sigType > 2 {
			return nil, fmt.Errorf("invalid SIGNATURE_TYPE: must be 0, 1, or 2")
		}
		cfg.SignatureType = sigType
	} else if cfg.ProxyWalletAddress != "" {
		cfg.SignatureType = 2 // Default to GNOSIS_SAFE for browser wallet connections
	}

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

// UseProxyWallet returns true if trading via Polymarket proxy wallet
func (c *Config) UseProxyWallet() bool {
	return c.ProxyWalletAddress != ""
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
