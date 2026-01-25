package gamma

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL             = "https://gamma-api.polymarket.com"
	clobURL             = "https://clob.polymarket.com"
	defaultTimeout      = 30 * time.Second
	defaultLimit        = 100
	upDownWindowMinutes = 20
)

// Client handles communication with the Gamma API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Gamma API client with default settings.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
	}
}

// NewClientWithTimeout creates a new Gamma API client with custom timeout.
func NewClientWithTimeout(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: baseURL,
	}
}

// SearchMarkets queries the Gamma API for markets matching the given query.
func (c *Client) SearchMarkets(query string) ([]Market, error) {
	params := url.Values{}
	params.Set("_q", query)
	params.Set("active", "true")
	params.Set("closed", "false")
	params.Set("_limit", strconv.Itoa(defaultLimit))

	endpoint := fmt.Sprintf("%s/markets?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var markets []Market
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return markets, nil
}

// GetActiveUpDownMarkets retrieves active 15-minute BTC/ETH up-or-down markets
// expiring within the next 20 minutes.
func (c *Client) GetActiveUpDownMarkets() ([]Market, error) {
	// 15M markets use slug pattern: {asset}-updown-15m-{startTimestamp}
	// The slug contains the START time, endDate = start + 15 minutes
	assets := []string{"btc", "eth", "sol", "xrp"}
	marketMap := make(map[string]Market)
	now := time.Now()

	// Calculate window timestamps
	nowUnix := now.Unix()
	windowSize := int64(15 * 60)
	// Get CURRENT window start (floor to 15-min boundary)
	currentWindowStart := (nowUnix / windowSize) * windowSize

	for _, asset := range assets {
		// Check current window and next 2 windows
		// Current window is the one that's about to end!
		for i := int64(0); i < 3; i++ {
			targetStartTime := currentWindowStart + (i * windowSize)
			slug := fmt.Sprintf("%s-updown-15m-%d", asset, targetStartTime)

			market, err := c.GetMarketBySlug(slug)
			if err == nil && market != nil && market.Active && !market.Closed {
				// Verify the market hasn't ended
				endTime, _ := market.EndTime()
				if endTime.After(now) {
					// Use slug as key since ConditionID may be empty
					marketMap[market.Slug] = *market
				}
			}
		}
	}

	// Fallback: also try text search
	queries := []string{"updown-15m", "up or down"}
	for _, query := range queries {
		markets, err := c.SearchMarkets(query)
		if err != nil {
			continue
		}
		for _, market := range markets {
			if c.isValidUpDownMarket(market) {
				// Double-check end time
				endTime, _ := market.EndTime()
				if endTime.After(now) {
					// Use slug as key since ConditionID may be empty
					marketMap[market.Slug] = market
				}
			}
		}
	}

	result := make([]Market, 0, len(marketMap))
	for _, market := range marketMap {
		result = append(result, market)
	}

	return result, nil
}

// GetMarketBySlug fetches a market by its slug.
func (c *Client) GetMarketBySlug(slug string) (*Market, error) {
	params := url.Values{}
	params.Set("slug", slug)

	endpoint := fmt.Sprintf("%s/markets?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch market: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var markets []Market
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(markets) == 0 {
		return nil, fmt.Errorf("market not found: %s", slug)
	}

	return &markets[0], nil
}

// isValidUpDownMarket checks if a market meets the criteria for 15-min trading.
func (c *Client) isValidUpDownMarket(market Market) bool {
	if !market.Active || market.Closed {
		return false
	}

	question := strings.ToLower(market.Question)
	hasAsset := strings.Contains(question, "bitcoin") ||
		strings.Contains(question, "ethereum") ||
		strings.Contains(question, "btc") ||
		strings.Contains(question, "eth")

	hasMarketType := strings.Contains(question, "up or down") ||
		strings.Contains(question, "15-min") ||
		strings.Contains(question, "15 min")

	if !hasAsset || !hasMarketType {
		return false
	}

	return market.IsExpiringSoon(upDownWindowMinutes * time.Minute)
}

// SearchMarketsWithParams queries the Gamma API with custom parameters.
func (c *Client) SearchMarketsWithParams(params SearchParams) ([]Market, error) {
	queryParams := url.Values{}

	if params.Query != "" {
		queryParams.Set("_q", params.Query)
	}
	queryParams.Set("active", strconv.FormatBool(params.Active))
	queryParams.Set("closed", strconv.FormatBool(params.Closed))

	limit := params.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	queryParams.Set("_limit", strconv.Itoa(limit))

	if params.Offset > 0 {
		queryParams.Set("_offset", strconv.Itoa(params.Offset))
	}

	endpoint := fmt.Sprintf("%s/markets?%s", c.baseURL, queryParams.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var markets []Market
	if err := json.NewDecoder(resp.Body).Decode(&markets); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return markets, nil
}

// GetSportsMarkets retrieves active sports betting markets (NFL, NBA, etc.).
func (c *Client) GetSportsMarkets() ([]Market, error) {
	marketMap := make(map[string]Market)
	now := time.Now()

	// Search patterns for sports markets
	queries := []string{
		"Super Bowl",
		"NFC Championship",
		"AFC Championship",
		"NBA Championship",
		"NFL",
		"win the",
	}

	for _, query := range queries {
		markets, err := c.SearchMarkets(query)
		if err != nil {
			continue
		}
		for _, market := range markets {
			if c.isValidSportsMarket(market) {
				// Check end time is in the future
				endTime, _ := market.EndTime()
				if endTime.After(now) {
					marketMap[market.Slug] = market
				}
			}
		}
	}

	result := make([]Market, 0, len(marketMap))
	for _, market := range marketMap {
		result = append(result, market)
	}

	return result, nil
}

// isValidSportsMarket checks if a market is a valid sports betting market.
func (c *Client) isValidSportsMarket(market Market) bool {
	if !market.Active || market.Closed {
		return false
	}

	question := strings.ToLower(market.Question)

	// Must be a "will X win" type question
	if !strings.Contains(question, "win") {
		return false
	}

	// Must be sports-related
	sportsKeywords := []string{
		"super bowl",
		"nfc championship",
		"afc championship",
		"nba championship",
		"nfl",
		"nba",
		"patriots",
		"broncos",
		"rams",
		"seahawks",
		"chiefs",
		"bills",
		"eagles",
		"49ers",
		"lions",
		"cowboys",
		"packers",
		"vikings",
	}

	for _, keyword := range sportsKeywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}

	return false
}

// GetNFLPlayoffMarkets retrieves NFL playoff markets (Conference Championships, Super Bowl).
func (c *Client) GetNFLPlayoffMarkets() ([]Market, error) {
	markets, err := c.GetSportsMarkets()
	if err != nil {
		return nil, err
	}

	playoffMarkets := make([]Market, 0)
	for _, market := range markets {
		question := strings.ToLower(market.Question)
		if strings.Contains(question, "super bowl") ||
			strings.Contains(question, "nfc championship") ||
			strings.Contains(question, "afc championship") {
			playoffMarkets = append(playoffMarkets, market)
		}
	}

	return playoffMarkets, nil
}

// GetMarketByConditionID fetches a specific market by its condition ID.
func (c *Client) GetMarketByConditionID(conditionID string) (*Market, error) {
	endpoint := fmt.Sprintf("%s/markets/%s", c.baseURL, url.PathEscape(conditionID))

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch market: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("market not found: %s", conditionID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var market Market
	if err := json.NewDecoder(resp.Body).Decode(&market); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &market, nil
}
