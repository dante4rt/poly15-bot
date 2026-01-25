package pricefeed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	binanceAPIURL = "https://api.binance.com/api/v3/ticker/price"
	cacheDuration = 500 * time.Millisecond // Cache prices for 500ms
)

// BinanceClient fetches real-time prices from Binance.
type BinanceClient struct {
	httpClient *http.Client
	cache      map[string]cachedPrice
	mu         sync.RWMutex
}

type cachedPrice struct {
	price     float64
	timestamp time.Time
}

// NewBinanceClient creates a new Binance price feed client.
func NewBinanceClient() *BinanceClient {
	return &BinanceClient{
		httpClient: &http.Client{Timeout: 2 * time.Second},
		cache:      make(map[string]cachedPrice),
	}
}

// GetPrice fetches the current price for a symbol (e.g., "BTCUSDT").
func (c *BinanceClient) GetPrice(symbol string) (float64, error) {
	symbol = strings.ToUpper(symbol)

	// Check cache first
	c.mu.RLock()
	cached, exists := c.cache[symbol]
	c.mu.RUnlock()

	if exists && time.Since(cached.timestamp) < cacheDuration {
		return cached.price, nil
	}

	// Fetch from Binance
	url := fmt.Sprintf("%s?symbol=%s", binanceAPIURL, symbol)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return 0, fmt.Errorf("binance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("binance returned status %d", resp.StatusCode)
	}

	var result struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse price: %w", err)
	}

	// Update cache
	c.mu.Lock()
	c.cache[symbol] = cachedPrice{price: price, timestamp: time.Now()}
	c.mu.Unlock()

	return price, nil
}

// GetBTCPrice returns the current BTC/USDT price.
func (c *BinanceClient) GetBTCPrice() (float64, error) {
	return c.GetPrice("BTCUSDT")
}

// GetETHPrice returns the current ETH/USDT price.
func (c *BinanceClient) GetETHPrice() (float64, error) {
	return c.GetPrice("ETHUSDT")
}

// GetSOLPrice returns the current SOL/USDT price.
func (c *BinanceClient) GetSOLPrice() (float64, error) {
	return c.GetPrice("SOLUSDT")
}

// GetXRPPrice returns the current XRP/USDT price.
func (c *BinanceClient) GetXRPPrice() (float64, error) {
	return c.GetPrice("XRPUSDT")
}

// PriceSnapshot holds price at a point in time.
type PriceSnapshot struct {
	Price     float64
	Timestamp time.Time
}

// PriceTracker tracks price changes over a window.
type PriceTracker struct {
	client    *BinanceClient
	symbol    string
	startTime time.Time
	startPrice float64
	mu        sync.RWMutex
}

// NewPriceTracker creates a tracker for a specific symbol.
func NewPriceTracker(client *BinanceClient, symbol string) *PriceTracker {
	return &PriceTracker{
		client: client,
		symbol: symbol,
	}
}

// StartTracking begins tracking from the current price.
func (t *PriceTracker) StartTracking() error {
	price, err := t.client.GetPrice(t.symbol)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.startTime = time.Now()
	t.startPrice = price
	t.mu.Unlock()

	return nil
}

// GetDirection returns "UP" if current price > start price, "DOWN" otherwise.
// Also returns the price change percentage.
func (t *PriceTracker) GetDirection() (direction string, changePercent float64, err error) {
	t.mu.RLock()
	startPrice := t.startPrice
	t.mu.RUnlock()

	if startPrice == 0 {
		return "", 0, fmt.Errorf("tracking not started")
	}

	currentPrice, err := t.client.GetPrice(t.symbol)
	if err != nil {
		return "", 0, err
	}

	changePercent = (currentPrice - startPrice) / startPrice * 100

	if currentPrice >= startPrice {
		return "UP", changePercent, nil
	}
	return "DOWN", changePercent, nil
}

// SymbolFromMarketQuestion extracts the Binance symbol from a market question.
// e.g., "Bitcoin Up or Down" -> "BTCUSDT"
func SymbolFromMarketQuestion(question string) string {
	question = strings.ToLower(question)

	switch {
	case strings.Contains(question, "bitcoin") || strings.Contains(question, "btc"):
		return "BTCUSDT"
	case strings.Contains(question, "ethereum") || strings.Contains(question, "eth"):
		return "ETHUSDT"
	case strings.Contains(question, "solana") || strings.Contains(question, "sol"):
		return "SOLUSDT"
	case strings.Contains(question, "xrp") || strings.Contains(question, "ripple"):
		return "XRPUSDT"
	default:
		return ""
	}
}
