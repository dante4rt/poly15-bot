package strategy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/clob"
	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/gamma"
	"github.com/dantezy/polymarket-sniper/internal/telegram"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/dantezy/polymarket-sniper/internal/weather"
	"github.com/ethereum/go-ethereum/common"
)

const (
	weatherScanInterval   = 1 * time.Hour    // Scan for new markets every hour
	weatherCheckInterval  = 30 * time.Second // Check positions every 30 seconds
	weatherStatusInterval = 5 * time.Minute  // Log status every 5 minutes
	weatherMaxOrderAge    = 12 * time.Hour   // Cancel orders older than this
)

// WeatherOpportunity represents a trading opportunity in a weather market.
type WeatherOpportunity struct {
	WeatherMarket  *gamma.WeatherMarket
	Forecast       *weather.Forecast
	OurProbYes     float64 // Our calculated probability for YES
	MarketPriceYes float64 // Market's implied probability (YES price)
	Edge           float64 // OurProb - MarketPrice
	ExpectedValue  float64 // EV of the trade
	Side           string  // "yes" or "no"
	TokenID        string
	BidPrice       float64 // Our limit order price
	Confidence     float64 // How confident we are (0-1)
	Score          float64 // Overall opportunity score
}

// WeatherPosition tracks an active weather trade.
type WeatherPosition struct {
	OrderID        string
	TokenID        string
	MarketSlug     string
	MarketQuestion string
	Side           string // "yes" or "no"
	BidPrice       float64
	Shares         float64
	PlacedAt       time.Time
	Edge           float64
	Status         string // "open", "filled", "cancelled"
}

// WeatherPositionTracker manages open weather positions.
type WeatherPositionTracker struct {
	positions map[string]*WeatherPosition
	mu        sync.RWMutex
}

// NewWeatherPositionTracker creates a new position tracker.
func NewWeatherPositionTracker() *WeatherPositionTracker {
	return &WeatherPositionTracker{
		positions: make(map[string]*WeatherPosition),
	}
}

func (pt *WeatherPositionTracker) Add(pos *WeatherPosition) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.positions[pos.OrderID] = pos
}

func (pt *WeatherPositionTracker) Remove(orderID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.positions, orderID)
}

func (pt *WeatherPositionTracker) GetAll() []*WeatherPosition {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	result := make([]*WeatherPosition, 0, len(pt.positions))
	for _, pos := range pt.positions {
		result = append(result, pos)
	}
	return result
}

func (pt *WeatherPositionTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.positions)
}

func (pt *WeatherPositionTracker) TotalExposure() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	total := 0.0
	for _, pos := range pt.positions {
		total += pos.Shares * pos.BidPrice
	}
	return total
}

func (pt *WeatherPositionTracker) HasMarket(slug string) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	for _, pos := range pt.positions {
		if pos.MarketSlug == slug {
			return true
		}
	}
	return false
}

// WeatherSniper implements a weather market trading strategy.
type WeatherSniper struct {
	config   *config.Config
	gamma    *gamma.Client
	clob     *clob.Client
	builder  *clob.OrderBuilder
	weather  *weather.Client
	telegram *telegram.Bot
	tracker  *WeatherPositionTracker
	edgeCalc *weather.EdgeCalculator

	// Bankroll tracking
	bankroll     float64
	dailyLoss    float64
	lastResetDay int

	// Stats
	totalTrades   int
	totalFilled   int
	totalCanceled int
	totalProfit   float64
}

// NewWeatherSniper creates a new weather sniper strategy instance.
func NewWeatherSniper(cfg *config.Config, w *wallet.Wallet, tg *telegram.Bot) (*WeatherSniper, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet is required")
	}

	// Create Gamma client with optional proxy
	var gammaClient *gamma.Client
	if cfg.ProxyURL != "" {
		gammaClient = gamma.NewClientWithProxy(cfg.ProxyURL)
	} else {
		gammaClient = gamma.NewClient()
	}

	// Create CLOB client
	var clobClient *clob.Client
	walletAddr := w.AddressHex()
	if len(cfg.ProxyURLs) > 1 {
		log.Printf("[weather] using %d proxies with rotation", len(cfg.ProxyURLs))
		var err error
		clobClient, err = clob.NewClientWithProxyRotation(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr, cfg.ProxyURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to create CLOB client: %w", err)
		}
	} else if cfg.ProxyURL != "" {
		log.Printf("[weather] using proxy")
		var err error
		clobClient, err = clob.NewClientWithProxy(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr, cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create CLOB client: %w", err)
		}
	} else {
		clobClient = clob.NewClient(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr)
	}

	// Create order builder
	var builder *clob.OrderBuilder
	if cfg.UseProxyWallet() {
		builder = clob.NewOrderBuilderWithProxy(w, cfg.CLOBApiKey, common.HexToAddress(cfg.ProxyWalletAddress), cfg.SignatureType)
	} else {
		builder = clob.NewOrderBuilder(w, cfg.CLOBApiKey)
	}

	return &WeatherSniper{
		config:       cfg,
		gamma:        gammaClient,
		clob:         clobClient,
		builder:      builder,
		weather:      weather.NewClient(),
		telegram:     tg,
		tracker:      NewWeatherPositionTracker(),
		edgeCalc:     weather.NewEdgeCalculator(),
		bankroll:     cfg.WeatherBankroll,
		lastResetDay: time.Now().YearDay(),
	}, nil
}

// Run starts the weather sniper and blocks until context is cancelled.
func (ws *WeatherSniper) Run(ctx context.Context) error {
	log.Printf("[weather] starting in %s mode", ws.modeString())
	log.Printf("[weather] config: min_edge=%.0f%%, min_confidence=%.0f%%",
		ws.config.WeatherMinEdge*100, ws.config.WeatherMinConfidence*100)
	log.Printf("[weather] config: max_position=$%.2f, daily_loss_limit=$%.2f",
		ws.config.WeatherMaxPosition, ws.config.WeatherDailyLossLimit)
	log.Printf("[weather] config: min_volume=$%.0f, max_spread=%.0f%%",
		ws.config.WeatherMinVolume, ws.config.WeatherMaxSpread*100)
	log.Printf("[weather] bankroll: $%.2f", ws.bankroll)

	// Initial scan
	if err := ws.ScanAndTrade(); err != nil {
		log.Printf("[weather] initial scan error: %v", err)
	}

	scanTicker := time.NewTicker(weatherScanInterval)
	checkTicker := time.NewTicker(weatherCheckInterval)
	statusTicker := time.NewTicker(weatherStatusInterval)

	defer scanTicker.Stop()
	defer checkTicker.Stop()
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[weather] shutting down")
			return ctx.Err()

		case <-scanTicker.C:
			if err := ws.ScanAndTrade(); err != nil {
				log.Printf("[weather] scan error: %v", err)
			}

		case <-checkTicker.C:
			if err := ws.CheckPositions(); err != nil {
				log.Printf("[weather] check error: %v", err)
			}

		case <-statusTicker.C:
			ws.logStatus()
		}
	}
}

// ScanAndTrade scans for weather market opportunities and places trades.
func (ws *WeatherSniper) ScanAndTrade() error {
	log.Printf("[weather] scanning for weather market opportunities...")

	// Reset daily loss if new day
	today := time.Now().YearDay()
	if today != ws.lastResetDay {
		ws.dailyLoss = 0
		ws.lastResetDay = today
		log.Printf("[weather] daily loss reset for new day")
	}

	// Check daily loss limit
	if ws.dailyLoss >= ws.config.WeatherDailyLossLimit {
		log.Printf("[weather] daily loss limit reached ($%.2f), skipping scan", ws.dailyLoss)
		return nil
	}

	opportunities, err := ws.FindOpportunities()
	if err != nil {
		return fmt.Errorf("failed to find opportunities: %w", err)
	}

	if len(opportunities) == 0 {
		log.Printf("[weather] no opportunities found matching criteria")
		return nil
	}

	log.Printf("[weather] found %d opportunities", len(opportunities))

	// Sort by score (best first)
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].Score > opportunities[j].Score
	})

	// Place trades on top opportunities
	tradesPlaced := 0
	for _, opp := range opportunities {
		// Check position limits
		if ws.tracker.Count() >= ws.config.WeatherMaxTrades {
			log.Printf("[weather] max trades reached (%d)", ws.config.WeatherMaxTrades)
			break
		}

		// Check exposure limit
		if ws.tracker.TotalExposure() >= ws.config.WeatherMaxExposure {
			log.Printf("[weather] max exposure reached ($%.2f)", ws.config.WeatherMaxExposure)
			break
		}

		// Skip if we already have position in this market
		if ws.tracker.HasMarket(opp.WeatherMarket.Market.Slug) {
			continue
		}

		// Place the trade
		if err := ws.PlaceTrade(opp); err != nil {
			log.Printf("[weather] failed to place trade: %v", err)
			continue
		}

		tradesPlaced++
		if tradesPlaced >= 3 { // Max 3 new trades per scan
			break
		}
	}

	log.Printf("[weather] placed %d new trades", tradesPlaced)
	return nil
}

// FindOpportunities searches for weather markets with edge.
func (ws *WeatherSniper) FindOpportunities() ([]*WeatherOpportunity, error) {
	// Fetch weather markets from Gamma
	markets, err := ws.gamma.GetWeatherMarkets()
	if err != nil {
		return nil, fmt.Errorf("failed to get weather markets: %w", err)
	}

	log.Printf("[weather] found %d weather markets", len(markets))

	var opportunities []*WeatherOpportunity

	for _, market := range markets {
		// Parse as weather market
		wm := gamma.ParseWeatherMarket(market)
		if wm == nil {
			continue
		}

		// Skip unknown market types
		if wm.MarketType == gamma.WeatherTypeUnknown {
			continue
		}

		// Check liquidity
		if !wm.HasGoodLiquidity(ws.config.WeatherMinVolume) {
			continue
		}

		// Check spread
		spread := absFloat(wm.YesPrice - (1 - wm.NoPrice))
		if spread > ws.config.WeatherMaxSpread {
			continue
		}

		// Get forecast for the location
		location := weather.FindLocationByName(wm.Location)
		if location == nil {
			log.Printf("[weather] unknown location: %s", wm.Location)
			continue
		}

		// Skip Tier D cities - unpredictable, avoid losses
		if location.Tier == weather.TierD {
			log.Printf("[weather] skipping Tier D location: %s (unpredictable)", wm.Location)
			continue
		}

		// Fetch forecast
		daysAhead := int(wm.DaysUntilResolution())
		if daysAhead < 0 {
			continue
		}
		if daysAhead > 7 {
			daysAhead = 7 // Open-Meteo limit
		}

		forecast, err := ws.weather.GetForecast(location, wm.ResolutionDate)
		if err != nil {
			log.Printf("[weather] failed to get forecast for %s: %v", wm.Location, err)
			continue
		}

		// Calculate probability based on market type
		opp := ws.evaluateOpportunity(wm, forecast, daysAhead)
		if opp != nil {
			opportunities = append(opportunities, opp)
		}
	}

	return opportunities, nil
}

// evaluateOpportunity calculates edge for a weather market opportunity.
func (ws *WeatherSniper) evaluateOpportunity(wm *gamma.WeatherMarket, forecast *weather.Forecast, daysAhead int) *WeatherOpportunity {
	var ourProbYes float64
	var confidence float64

	switch wm.MarketType {
	case gamma.WeatherTypeTempAbove:
		// "Will temperature be above X?"
		thresholdC := wm.GetThresholdCelsius()
		dist := weather.NewHighTempDistribution(forecast, daysAhead)
		ourProbYes = dist.ProbAbove(thresholdC)
		confidence = ws.calculateConfidence(dist, thresholdC, daysAhead)

	case gamma.WeatherTypeTempBelow:
		// "Will temperature be below X?"
		thresholdC := wm.GetThresholdCelsius()
		dist := weather.NewLowTempDistribution(forecast, daysAhead)
		ourProbYes = dist.ProbBelow(thresholdC)
		confidence = ws.calculateConfidence(dist, thresholdC, daysAhead)

	case gamma.WeatherTypeSnow:
		// "Will it snow?"
		ourProbYes = weather.SnowProbability(forecast)
		confidence = 0.6 // Snow predictions are less reliable

	case gamma.WeatherTypeRain:
		// "Will it rain?"
		ourProbYes = weather.RainProbability(forecast)
		confidence = 0.7 // Rain predictions are moderately reliable

	default:
		return nil
	}

	// Skip if confidence too low
	if confidence < ws.config.WeatherMinConfidence {
		return nil
	}

	// Calculate edge for YES side
	edgeYes := ourProbYes - wm.YesPrice
	evYes := edgeYes

	// Calculate edge for NO side
	ourProbNo := 1 - ourProbYes
	edgeNo := ourProbNo - wm.NoPrice
	evNo := edgeNo

	// Determine which side to bet on
	var side string
	var edge, ev float64
	var tokenID string
	var bidPrice float64

	if edgeYes >= edgeNo && edgeYes >= ws.config.WeatherMinEdge {
		side = "yes"
		edge = edgeYes
		ev = evYes
		tokenID = wm.YesTokenID
		bidPrice = wm.YesPrice * (1 - ws.config.WeatherBidDiscount) // Bid below market
	} else if edgeNo > edgeYes && edgeNo >= ws.config.WeatherMinEdge {
		side = "no"
		edge = edgeNo
		ev = evNo
		tokenID = wm.NoTokenID
		bidPrice = wm.NoPrice * (1 - ws.config.WeatherBidDiscount)
	} else {
		// No significant edge
		return nil
	}

	// Score the opportunity
	// Higher edge + higher confidence + sooner resolution + better location tier = better
	timeBonus := 1.0
	if daysAhead <= 1 {
		timeBonus = 2.0 // Tomorrow - high bonus
	} else if daysAhead <= 3 {
		timeBonus = 1.5
	}

	volumeBonus := 1.0
	vol := wm.Market.GetVolume24hr()
	if vol > 1000 {
		volumeBonus = 1.0 + (vol / 10000)
		if volumeBonus > 2.0 {
			volumeBonus = 2.0
		}
	}

	// Get location tier bonus - prioritize predictable cities
	location := weather.FindLocationByName(wm.Location)
	tierBonus := 0.5 // Default for unknown locations
	tierStr := "?"
	if location != nil {
		tierBonus = location.Tier.TierMultiplier()
		tierStr = string(location.Tier)
	}

	score := edge * confidence * 100 * timeBonus * volumeBonus * tierBonus

	log.Printf("[weather] opportunity: %s - %s side, edge=%.1f%%, confidence=%.0f%%, tier=%s, score=%.1f",
		wm.Market.Question[:minInt(50, len(wm.Market.Question))], side, edge*100, confidence*100, tierStr, score)

	return &WeatherOpportunity{
		WeatherMarket:  wm,
		Forecast:       forecast,
		OurProbYes:     ourProbYes,
		MarketPriceYes: wm.YesPrice,
		Edge:           edge,
		ExpectedValue:  ev,
		Side:           side,
		TokenID:        tokenID,
		BidPrice:       bidPrice,
		Confidence:     confidence,
		Score:          score,
	}
}

// calculateConfidence estimates our confidence in the probability calculation.
func (ws *WeatherSniper) calculateConfidence(dist *weather.TempDistribution, threshold float64, daysAhead int) float64 {
	// Base confidence decreases with forecast horizon
	baseConfidence := 0.9
	switch {
	case daysAhead <= 0:
		baseConfidence = 0.95 // Same day
	case daysAhead == 1:
		baseConfidence = 0.85 // Tomorrow
	case daysAhead <= 3:
		baseConfidence = 0.75 // 2-3 days
	default:
		baseConfidence = 0.60 // 4+ days
	}

	// Confidence is higher when threshold is far from the mean
	// (more certain outcomes)
	zScore := absFloat(threshold-dist.Mean) / dist.StdDev
	certaintyBonus := 0.0
	if zScore > 2 {
		certaintyBonus = 0.1 // Very confident
	} else if zScore > 1 {
		certaintyBonus = 0.05
	}

	confidence := baseConfidence + certaintyBonus
	if confidence > 0.95 {
		confidence = 0.95
	}

	return confidence
}

// PlaceTrade places a limit order for a weather opportunity.
func (ws *WeatherSniper) PlaceTrade(opp *WeatherOpportunity) error {
	// Calculate bet amount (% of bankroll, capped by max position)
	betAmount := ws.bankroll * ws.config.WeatherBetPercent
	if betAmount > ws.config.WeatherMaxPosition {
		betAmount = ws.config.WeatherMaxPosition
	}

	// Check exposure limits
	currentExposure := ws.tracker.TotalExposure()
	if currentExposure+betAmount > ws.config.WeatherMaxExposure {
		betAmount = ws.config.WeatherMaxExposure - currentExposure
		if betAmount < 0.01 {
			return fmt.Errorf("insufficient exposure remaining")
		}
	}

	// Calculate shares
	shares := betAmount / opp.BidPrice
	const minShares = 5.0
	if shares < minShares {
		shares = minShares
		betAmount = shares * opp.BidPrice
	}

	log.Printf("[weather] placing %s trade: %s @ $%.4f, shares=%.1f, cost=$%.2f, edge=%.1f%%",
		opp.Side, opp.WeatherMarket.Market.Question[:minInt(40, len(opp.WeatherMarket.Market.Question))],
		opp.BidPrice, shares, betAmount, opp.Edge*100)

	if ws.config.DryRun {
		log.Printf("[weather] DRY_RUN: would place GTC limit order")

		position := &WeatherPosition{
			OrderID:        fmt.Sprintf("dry-%d", time.Now().UnixNano()),
			TokenID:        opp.TokenID,
			MarketSlug:     opp.WeatherMarket.Market.Slug,
			MarketQuestion: opp.WeatherMarket.Market.Question,
			Side:           opp.Side,
			BidPrice:       opp.BidPrice,
			Shares:         shares,
			PlacedAt:       time.Now(),
			Edge:           opp.Edge,
			Status:         "open",
		}
		ws.tracker.Add(position)
		ws.totalTrades++

		if ws.telegram != nil {
			msg := fmt.Sprintf("[DRY RUN] Weather Trade\n\n"+
				"%s\n\n"+
				"Side: %s @ $%.4f\n"+
				"Size: %.0f shares ($%.2f)\n"+
				"Edge: %.1f%%\n"+
				"Forecast: High %.0f°F / Low %.0f°F",
				opp.WeatherMarket.Market.Question,
				opp.Side, opp.BidPrice,
				shares, betAmount,
				opp.Edge*100,
				opp.Forecast.TempHighF(), opp.Forecast.TempLowF())
			ws.telegram.SendMessage(msg)
		}

		return nil
	}

	// Check neg risk
	negRisk, err := ws.clob.GetNegRisk(opp.TokenID)
	if err != nil {
		log.Printf("[weather] warning: failed to check neg_risk: %v (assuming standard)", err)
		negRisk = false
	}

	// Build GTC limit order
	order, err := ws.builder.BuildGTCBuyOrder(opp.TokenID, opp.BidPrice, shares, negRisk)
	if err != nil {
		return fmt.Errorf("failed to build order: %w", err)
	}

	// Submit order
	resp, err := ws.clob.CreateOrder(order)
	if err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("order rejected: %s", resp.Error)
	}

	// Track the position
	position := &WeatherPosition{
		OrderID:        resp.OrderID,
		TokenID:        opp.TokenID,
		MarketSlug:     opp.WeatherMarket.Market.Slug,
		MarketQuestion: opp.WeatherMarket.Market.Question,
		Side:           opp.Side,
		BidPrice:       opp.BidPrice,
		Shares:         shares,
		PlacedAt:       time.Now(),
		Edge:           opp.Edge,
		Status:         "open",
	}
	ws.tracker.Add(position)
	ws.totalTrades++

	log.Printf("[weather] ORDER PLACED: %s (order ID: %s)", opp.WeatherMarket.Market.Question[:minInt(40, len(opp.WeatherMarket.Market.Question))], resp.OrderID)

	if ws.telegram != nil {
		msg := fmt.Sprintf("Weather Trade Placed\n\n"+
			"%s\n\n"+
			"Side: %s @ $%.4f\n"+
			"Size: %.0f shares ($%.2f)\n"+
			"Edge: %.1f%%\n"+
			"Forecast: High %.0f°F / Low %.0f°F",
			opp.WeatherMarket.Market.Question,
			opp.Side, opp.BidPrice,
			shares, betAmount,
			opp.Edge*100,
			opp.Forecast.TempHighF(), opp.Forecast.TempLowF())
		ws.telegram.SendMessage(msg)
	}

	return nil
}

// CheckPositions checks the status of open positions.
func (ws *WeatherSniper) CheckPositions() error {
	if ws.config.DryRun {
		return nil
	}

	openOrders, err := ws.clob.GetOpenOrders()
	if err != nil {
		return fmt.Errorf("failed to get open orders: %w", err)
	}

	openOrderMap := make(map[string]bool)
	for _, order := range openOrders {
		orderID := order.GetID()
		if orderID != "" {
			openOrderMap[orderID] = true
		}
	}

	for _, pos := range ws.tracker.GetAll() {
		if !openOrderMap[pos.OrderID] {
			// Order was filled or cancelled
			log.Printf("[weather] order %s no longer open (was: %s %s)",
				pos.OrderID, pos.MarketQuestion[:minInt(30, len(pos.MarketQuestion))], pos.Side)

			if ws.telegram != nil {
				potentialPayout := pos.Shares
				msg := fmt.Sprintf("Weather Order Filled!\n\n"+
					"%s\n\n"+
					"You own: %.0f %s shares\n"+
					"Cost: $%.2f\n"+
					"Payout if wins: $%.2f",
					pos.MarketQuestion,
					pos.Shares, pos.Side,
					pos.Shares*pos.BidPrice,
					potentialPayout)
				ws.telegram.SendMessage(msg)
			}

			ws.tracker.Remove(pos.OrderID)
			ws.totalFilled++
			continue
		}

		// Check if order is too old
		if time.Since(pos.PlacedAt) > weatherMaxOrderAge {
			log.Printf("[weather] canceling stale order %s (age: %v)", pos.OrderID, time.Since(pos.PlacedAt))
			if err := ws.clob.CancelOrder(pos.OrderID); err != nil {
				log.Printf("[weather] failed to cancel order %s: %v", pos.OrderID, err)
			} else {
				ws.tracker.Remove(pos.OrderID)
				ws.totalCanceled++
			}
		}
	}

	return nil
}

// logStatus logs current status.
func (ws *WeatherSniper) logStatus() {
	positions := ws.tracker.GetAll()
	exposure := ws.tracker.TotalExposure()

	log.Printf("[weather] STATUS: positions=%d, exposure=$%.2f, trades=%d, filled=%d, canceled=%d, daily_loss=$%.2f",
		len(positions), exposure, ws.totalTrades, ws.totalFilled, ws.totalCanceled, ws.dailyLoss)

	if len(positions) > 0 {
		log.Printf("[weather] open positions:")
		for _, pos := range positions {
			age := time.Since(pos.PlacedAt).Truncate(time.Minute)
			log.Printf("[weather]   - %s %s @ $%.4f, edge=%.1f%% [%v old]",
				pos.MarketQuestion[:minInt(35, len(pos.MarketQuestion))], pos.Side, pos.BidPrice, pos.Edge*100, age)
		}
	}
}

func (ws *WeatherSniper) modeString() string {
	if ws.config.DryRun {
		return "DRY_RUN"
	}
	return "LIVE"
}

// GetStats returns current strategy statistics.
func (ws *WeatherSniper) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"mode":           ws.modeString(),
		"positions":      ws.tracker.Count(),
		"exposure":       ws.tracker.TotalExposure(),
		"total_trades":   ws.totalTrades,
		"total_filled":   ws.totalFilled,
		"total_canceled": ws.totalCanceled,
		"daily_loss":     ws.dailyLoss,
		"bankroll":       ws.bankroll,
	}
}

// Helper functions
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
