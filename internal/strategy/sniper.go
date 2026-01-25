package strategy

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/clob"
	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/gamma"
	"github.com/dantezy/polymarket-sniper/internal/telegram"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
)

const (
	scanInterval    = 30 * time.Second
	checkInterval   = 100 * time.Millisecond
	cleanupInterval = 1 * time.Minute

	// Winner detection thresholds
	minWinnerConfidence = 0.65  // Minimum bid price to consider a clear winner
	maxUncertaintyGap   = 0.10  // If YES and NO bids are within this range, too risky
	maxSpreadPercent    = 0.05  // Maximum spread as percentage of price (5%)
	defaultMinLiquidity = 5.0   // Default minimum size in USD at best ask
	momentumThreshold   = 0.15  // Price jump threshold for momentum signal

	// Risk management
	defaultMaxLossPerTrade = 5.0   // Maximum loss per trade in USD
	defaultDailyLossLimit  = 50.0  // Maximum daily loss in USD
)

// SkipReason documents why a trade was skipped.
type SkipReason string

const (
	SkipReasonNoWinner       SkipReason = "no_clear_winner"
	SkipReasonTooUncertain   SkipReason = "prices_too_close"
	SkipReasonSpreadTooWide  SkipReason = "spread_too_wide"
	SkipReasonNoLiquidity    SkipReason = "insufficient_liquidity"
	SkipReasonPriceTooHigh   SkipReason = "price_above_threshold"
	SkipReasonMaxLossExceeds SkipReason = "max_loss_exceeded"
	SkipReasonDailyLimit     SkipReason = "daily_loss_limit"
)

// PriceSnapshot holds price data at a point in time for momentum tracking.
type PriceSnapshot struct {
	YesBid    float64
	YesAsk    float64
	NoBid     float64
	NoAsk     float64
	Timestamp time.Time
}

// TrackedMarket holds state for a market being monitored for sniping.
type TrackedMarket struct {
	Market     gamma.Market
	YesTokenID string
	NoTokenID  string
	EndTime    time.Time
	// CLOB order book prices (for execution)
	BestYesBid float64
	BestYesAsk float64
	BestNoBid  float64
	BestNoAsk  float64
	YesSize    float64 // Available size at best ask
	NoSize     float64 // Available size at best ask
	// Gamma indicative prices (for winner analysis)
	GammaYesPrice float64
	GammaNoPrice  float64
	sniped        bool

	// Price history for momentum detection (last 10 snapshots)
	priceHistory []PriceSnapshot
	mu           sync.RWMutex
}

// UpdateYesPrice updates the YES token prices thread-safely.
func (tm *TrackedMarket) UpdateYesPrice(bid, ask, size float64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.BestYesBid = bid
	tm.BestYesAsk = ask
	tm.YesSize = size
	tm.recordSnapshot()
}

// UpdateNoPrice updates the NO token prices thread-safely.
func (tm *TrackedMarket) UpdateNoPrice(bid, ask, size float64) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.BestNoBid = bid
	tm.BestNoAsk = ask
	tm.NoSize = size
	tm.recordSnapshot()
}

// recordSnapshot stores current prices for momentum tracking.
// Must be called with lock held.
func (tm *TrackedMarket) recordSnapshot() {
	snapshot := PriceSnapshot{
		YesBid:    tm.BestYesBid,
		YesAsk:    tm.BestYesAsk,
		NoBid:     tm.BestNoBid,
		NoAsk:     tm.BestNoAsk,
		Timestamp: time.Now(),
	}
	tm.priceHistory = append(tm.priceHistory, snapshot)

	// Keep only last 10 snapshots
	if len(tm.priceHistory) > 10 {
		tm.priceHistory = tm.priceHistory[1:]
	}
}

// GetPrices returns current prices thread-safely.
func (tm *TrackedMarket) GetPrices() (yesBid, yesAsk, noBid, noAsk float64) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.BestYesBid, tm.BestYesAsk, tm.BestNoBid, tm.BestNoAsk
}

// GetSizes returns available liquidity at best ask for each side.
func (tm *TrackedMarket) GetSizes() (yesSize, noSize float64) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.YesSize, tm.NoSize
}

// GetMomentum returns price change for YES side over recent history.
// Positive = YES price increasing, Negative = YES price decreasing.
func (tm *TrackedMarket) GetMomentum() float64 {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if len(tm.priceHistory) < 2 {
		return 0
	}

	oldest := tm.priceHistory[0]
	newest := tm.priceHistory[len(tm.priceHistory)-1]

	return newest.YesBid - oldest.YesBid
}

// MarkSniped marks the market as already sniped to prevent duplicate trades.
func (tm *TrackedMarket) MarkSniped() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.sniped = true
}

// IsSniped returns whether the market has already been sniped.
func (tm *TrackedMarket) IsSniped() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.sniped
}

// TradeAnalysis contains the analysis results for a potential trade.
type TradeAnalysis struct {
	ShouldTrade     bool
	Side            string  // "YES" or "NO"
	TokenID         string
	EntryPrice      float64
	Confidence      float64 // 0-1 confidence score
	ExpectedProfit  float64
	MaxLoss         float64
	AvailableSize   float64
	Spread          float64
	SpreadPercent   float64
	Momentum        float64
	SkipReason      SkipReason
	SkipDescription string
}

// DailyStats tracks daily trading statistics for risk management.
type DailyStats struct {
	Date       time.Time
	TotalLoss  float64
	TotalGain  float64
	TradeCount int
	mu         sync.RWMutex
}

// AddLoss records a potential loss (position cost).
func (ds *DailyStats) AddLoss(amount float64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.TotalLoss += amount
	ds.TradeCount++
}

// GetTotalLoss returns current daily loss.
func (ds *DailyStats) GetTotalLoss() float64 {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.TotalLoss
}

// Reset resets daily stats (call at midnight).
func (ds *DailyStats) Reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.Date = time.Now().Truncate(24 * time.Hour)
	ds.TotalLoss = 0
	ds.TotalGain = 0
	ds.TradeCount = 0
}

// Sniper implements the sniping strategy for 15-minute up/down markets.
type Sniper struct {
	config   *config.Config
	gamma    *gamma.Client
	clob     *clob.Client
	ws       *clob.WSClient
	builder  *clob.OrderBuilder
	telegram *telegram.Bot

	activeMarkets map[string]*TrackedMarket
	dailyStats    *DailyStats
	mu            sync.RWMutex

	// Configurable risk parameters
	maxLossPerTrade float64
	dailyLossLimit  float64
	minLiquidity    float64
}

// NewSniper creates a new Sniper instance.
func NewSniper(cfg *config.Config, w *wallet.Wallet, tg *telegram.Bot) (*Sniper, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet is required")
	}

	gammaClient := gamma.NewClient()
	clobClient := clob.NewClient(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase)
	wsClient := clob.NewWSClient()
	builder := clob.NewOrderBuilder(w)

	minLiq := cfg.MinLiquidity
	if minLiq <= 0 {
		minLiq = defaultMinLiquidity
	}

	sniper := &Sniper{
		config:          cfg,
		gamma:           gammaClient,
		clob:            clobClient,
		ws:              wsClient,
		builder:         builder,
		telegram:        tg,
		activeMarkets:   make(map[string]*TrackedMarket),
		dailyStats:      &DailyStats{Date: time.Now().Truncate(24 * time.Hour)},
		maxLossPerTrade: defaultMaxLossPerTrade,
		dailyLossLimit:  defaultDailyLossLimit,
		minLiquidity:    minLiq,
	}

	// Register global WebSocket handler for price updates
	wsClient.OnUpdate(sniper.handleMarketUpdate)

	return sniper, nil
}

// SetRiskLimits configures risk management parameters.
func (s *Sniper) SetRiskLimits(maxLossPerTrade, dailyLossLimit float64) {
	s.maxLossPerTrade = maxLossPerTrade
	s.dailyLossLimit = dailyLossLimit
}

// handleMarketUpdate processes incoming WebSocket price updates.
func (s *Sniper) handleMarketUpdate(update clob.MarketUpdate) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, tracked := range s.activeMarkets {
		if tracked.YesTokenID == update.TokenID {
			tracked.UpdateYesPrice(update.BestBid, update.BestAsk, update.AskSize)
		} else if tracked.NoTokenID == update.TokenID {
			tracked.UpdateNoPrice(update.BestBid, update.BestAsk, update.AskSize)
		}
	}
}

// Run starts the sniper and blocks until the context is cancelled.
func (s *Sniper) Run(ctx context.Context) error {
	log.Printf("[sniper] starting in %s mode", s.modeString())
	log.Printf("[sniper] config: snipe_price=%.4f, trigger_seconds=%d, max_position=$%.2f",
		s.config.SnipePrice, s.config.TriggerSeconds, s.config.MaxPositionSize)
	log.Printf("[sniper] risk: max_loss_per_trade=$%.2f, daily_limit=$%.2f",
		s.maxLossPerTrade, s.dailyLossLimit)

	// Connect to WebSocket for real-time price updates
	if err := s.ws.Connect(); err != nil {
		log.Printf("[sniper] warning: failed to connect WebSocket: %v (will use polling)", err)
	} else {
		// Start WebSocket event loop in background
		go func() {
			if err := s.ws.Run(ctx); err != nil {
				log.Printf("[sniper] WebSocket run error: %v", err)
			}
		}()
	}

	// Initial market scan
	if err := s.ScanForMarkets(); err != nil {
		log.Printf("[sniper] initial scan error: %v", err)
	}

	scanTicker := time.NewTicker(scanInterval)
	checkTicker := time.NewTicker(checkInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	statusTicker := time.NewTicker(60 * time.Second) // Log status every minute

	defer scanTicker.Stop()
	defer checkTicker.Stop()
	defer cleanupTicker.Stop()
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[sniper] shutting down")
			if err := s.ws.Close(); err != nil {
				log.Printf("[sniper] ws close error: %v", err)
			}
			return ctx.Err()

		case <-scanTicker.C:
			if err := s.ScanForMarkets(); err != nil {
				log.Printf("[sniper] scan error: %v", err)
			}

		case <-checkTicker.C:
			s.resetDailyStatsIfNeeded()
			if err := s.CheckAndSnipe(); err != nil {
				log.Printf("[sniper] check error: %v", err)
			}

		case <-cleanupTicker.C:
			s.cleanupExpiredMarkets()

		case <-statusTicker.C:
			s.logStatus()
		}
	}
}

// resetDailyStatsIfNeeded resets daily stats at midnight.
func (s *Sniper) resetDailyStatsIfNeeded() {
	today := time.Now().Truncate(24 * time.Hour)
	if s.dailyStats.Date.Before(today) {
		log.Printf("[sniper] resetting daily stats (previous: loss=$%.2f, trades=%d)",
			s.dailyStats.GetTotalLoss(), s.dailyStats.TradeCount)
		s.dailyStats.Reset()
	}
}

// ScanForMarkets discovers new 15-minute markets to track.
func (s *Sniper) ScanForMarkets() error {
	markets, err := s.gamma.GetActiveUpDownMarkets()
	if err != nil {
		return fmt.Errorf("failed to fetch markets: %w", err)
	}

	log.Printf("[sniper] found %d active up/down markets", len(markets))

	for _, market := range markets {
		s.mu.RLock()
		_, exists := s.activeMarkets[market.ConditionID]
		s.mu.RUnlock()

		if exists {
			continue
		}

		tracked, err := s.trackMarket(market)
		if err != nil {
			log.Printf("[sniper] failed to track market %s: %v", market.ConditionID, err)
			continue
		}

		s.mu.Lock()
		s.activeMarkets[market.ConditionID] = tracked
		s.mu.Unlock()

		log.Printf("[sniper] tracking market: %s (ends: %s)", market.Question, tracked.EndTime.Format(time.RFC3339))

		if s.telegram != nil {
			if err := s.telegram.NotifyMarketFound(market.Question, tracked.EndTime); err != nil {
				log.Printf("[sniper] telegram notify error: %v", err)
			}
		}
	}

	return nil
}

// trackMarket creates a TrackedMarket and subscribes to price updates.
func (s *Sniper) trackMarket(market gamma.Market) (*TrackedMarket, error) {
	endTime, err := market.EndTime()
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	// Skip markets that have already ended
	if time.Until(endTime) <= 0 {
		return nil, fmt.Errorf("market already ended at %s", endTime.Format(time.RFC3339))
	}

	yesToken := market.GetYesToken()
	noToken := market.GetNoToken()

	if yesToken == nil || noToken == nil {
		return nil, fmt.Errorf("market missing YES or NO token")
	}

	// Store Gamma's indicative prices (used for winner determination)
	gammaPrices := market.ParseOutcomePrices()
	gammaYes, gammaNo := 0.0, 0.0
	if len(gammaPrices) >= 2 {
		gammaYes = gammaPrices[0]
		gammaNo = gammaPrices[1]
	}

	tracked := &TrackedMarket{
		Market:        market,
		YesTokenID:    yesToken.TokenID,
		NoTokenID:     noToken.TokenID,
		EndTime:       endTime,
		BestYesBid:    yesToken.Price,
		BestYesAsk:    yesToken.Price,
		BestNoBid:     noToken.Price,
		BestNoAsk:     noToken.Price,
		GammaYesPrice: gammaYes,
		GammaNoPrice:  gammaNo,
		priceHistory:  make([]PriceSnapshot, 0, 10),
	}

	// Subscribe to WebSocket price updates for both tokens
	s.subscribeToToken(tracked, yesToken.TokenID, true)
	s.subscribeToToken(tracked, noToken.TokenID, false)

	// Also fetch initial order book prices
	s.updateOrderBookPrices(tracked)

	return tracked, nil
}

// subscribeToToken subscribes to WebSocket updates for a token.
func (s *Sniper) subscribeToToken(tracked *TrackedMarket, tokenID string, isYes bool) {
	if err := s.ws.Subscribe(tokenID); err != nil {
		log.Printf("[sniper] failed to subscribe to token %s: %v", tokenID, err)
	}
}

// refreshGammaPrices fetches latest prices from Gamma API.
func (s *Sniper) refreshGammaPrices(tracked *TrackedMarket) {
	market, err := s.gamma.GetMarketBySlug(tracked.Market.Slug)
	if err != nil {
		return
	}

	prices := market.ParseOutcomePrices()
	if len(prices) >= 2 {
		tracked.mu.Lock()
		tracked.GammaYesPrice = prices[0]
		tracked.GammaNoPrice = prices[1]
		tracked.mu.Unlock()
	}
}

// updateOrderBookPrices fetches current order book prices from REST API.
func (s *Sniper) updateOrderBookPrices(tracked *TrackedMarket) {
	// Fetch YES token order book
	if yesBook, err := s.clob.GetOrderBook(tracked.YesTokenID); err == nil {
		bid, ask, size := extractBestPricesWithSize(yesBook)
		tracked.UpdateYesPrice(bid, ask, size)
	}

	// Fetch NO token order book
	if noBook, err := s.clob.GetOrderBook(tracked.NoTokenID); err == nil {
		bid, ask, size := extractBestPricesWithSize(noBook)
		tracked.UpdateNoPrice(bid, ask, size)
	}
}

// extractBestPricesWithSize gets the best bid, ask, and ask size from an order book.
func extractBestPricesWithSize(book *clob.OrderBook) (bid, ask, askSize float64) {
	if len(book.Bids) > 0 {
		if price, err := strconv.ParseFloat(book.Bids[0].Price, 64); err == nil {
			bid = price
		}
	}
	if len(book.Asks) > 0 {
		if price, err := strconv.ParseFloat(book.Asks[0].Price, 64); err == nil {
			ask = price
		}
		if size, err := strconv.ParseFloat(book.Asks[0].Size, 64); err == nil {
			askSize = size
		}
	}
	return bid, ask, askSize
}

// CheckAndSnipe evaluates all tracked markets and executes snipes when conditions are met.
func (s *Sniper) CheckAndSnipe() error {
	now := time.Now()

	s.mu.RLock()
	markets := make([]*TrackedMarket, 0, len(s.activeMarkets))
	for _, m := range s.activeMarkets {
		markets = append(markets, m)
	}
	s.mu.RUnlock()

	for _, tracked := range markets {
		if tracked.IsSniped() {
			continue
		}

		timeRemaining := tracked.EndTime.Sub(now)

		// Poll prices via REST (since WebSocket may not be connected)
		// Only poll when getting close to snipe window (within 30s)
		if timeRemaining <= 30*time.Second && timeRemaining > 0 {
			s.updateOrderBookPrices(tracked)
			s.refreshGammaPrices(tracked)
		}

		// Skip if not within snipe window yet
		if timeRemaining > time.Duration(s.config.TriggerSeconds)*time.Second {
			continue
		}

		// Skip if market has already ended
		if timeRemaining < 0 {
			continue
		}

		// Analyze and execute snipe
		analysis := s.analyzeMarket(tracked)
		s.logAnalysis(tracked, analysis, timeRemaining)

		if !analysis.ShouldTrade {
			tracked.MarkSniped() // Don't retry
			continue
		}

		if err := s.executeSnipe(tracked, analysis, timeRemaining); err != nil {
			log.Printf("[sniper] snipe error for %s: %v", tracked.Market.Question, err)
		}
	}

	return nil
}

// analyzeMarket performs comprehensive analysis to determine if we should trade.
func (s *Sniper) analyzeMarket(tracked *TrackedMarket) TradeAnalysis {
	yesBid, yesAsk, noBid, noAsk := tracked.GetPrices()
	yesSize, noSize := tracked.GetSizes()
	momentum := tracked.GetMomentum()

	// Get Gamma's indicative prices (more reliable for winner determination)
	tracked.mu.RLock()
	gammaYes := tracked.GammaYesPrice
	gammaNo := tracked.GammaNoPrice
	tracked.mu.RUnlock()

	analysis := TradeAnalysis{
		Momentum: momentum,
	}

	// Check for daily loss limit
	currentDailyLoss := s.dailyStats.GetTotalLoss()
	if currentDailyLoss >= s.dailyLossLimit {
		analysis.SkipReason = SkipReasonDailyLimit
		analysis.SkipDescription = fmt.Sprintf("daily loss $%.2f >= limit $%.2f", currentDailyLoss, s.dailyLossLimit)
		return analysis
	}

	// Determine winner using Gamma prices (more accurate than CLOB order book)
	// Gamma prices reflect actual market consensus; CLOB may have wide spreads
	yesWins := gammaYes > gammaNo
	if gammaYes == 0 && gammaNo == 0 {
		// Fallback to CLOB bid prices if Gamma prices unavailable
		yesWins = yesBid > noBid
	}

	// Signal 2: Strong momentum (price jumped significantly)
	strongYesMomentum := momentum >= momentumThreshold
	strongNoMomentum := momentum <= -momentumThreshold

	// Calculate confidence based on Gamma price of predicted winner
	var winnerGammaPrice, loserGammaPrice float64
	var winnerAsk, winnerSize float64

	// Prioritize momentum signal if strong, otherwise use Gamma price
	if strongYesMomentum || (yesWins && !strongNoMomentum) {
		analysis.Side = "UP"
		analysis.TokenID = tracked.YesTokenID
		winnerGammaPrice = gammaYes
		loserGammaPrice = gammaNo
		winnerAsk = yesAsk
		winnerSize = yesSize
	} else {
		analysis.Side = "DOWN"
		analysis.TokenID = tracked.NoTokenID
		winnerGammaPrice = gammaNo
		loserGammaPrice = gammaYes
		winnerAsk = noAsk
		winnerSize = noSize
	}

	// Check 1: Clear winner (Gamma price above threshold)
	if winnerGammaPrice < minWinnerConfidence {
		analysis.SkipReason = SkipReasonNoWinner
		analysis.SkipDescription = fmt.Sprintf("%s gamma_price %.4f < threshold %.4f", analysis.Side, winnerGammaPrice, minWinnerConfidence)
		return analysis
	}

	// Check 2: Not too uncertain (sides not too close)
	priceGap := winnerGammaPrice - loserGammaPrice
	if priceGap < maxUncertaintyGap {
		analysis.SkipReason = SkipReasonTooUncertain
		analysis.SkipDescription = fmt.Sprintf("price gap %.4f < threshold %.4f (UP:%.4f DOWN:%.4f)",
			priceGap, maxUncertaintyGap, gammaYes, gammaNo)
		return analysis
	}

	// Note: CLOB spreads are typically wide (0.01/0.99), we skip spread check
	// and focus on Gamma price confidence instead
	analysis.Spread = winnerAsk - 0.01 // Approximate spread from CLOB
	analysis.SpreadPercent = 0         // Not meaningful for these markets

	// Check 4: Sufficient liquidity at ask
	analysis.AvailableSize = winnerSize
	if winnerSize < s.minLiquidity {
		analysis.SkipReason = SkipReasonNoLiquidity
		analysis.SkipDescription = fmt.Sprintf("size $%.2f < min $%.2f", winnerSize, s.minLiquidity)
		return analysis
	}

	// Determine entry price: use actual ask, not config snipe price
	// The ask IS the price we'll pay (for market buy via FOK)
	analysis.EntryPrice = winnerAsk

	// Check 5: Entry price is acceptable (must be below our max)
	if winnerAsk <= 0 || winnerAsk > s.config.SnipePrice {
		analysis.SkipReason = SkipReasonPriceTooHigh
		analysis.SkipDescription = fmt.Sprintf("ask %.4f > max %.4f", winnerAsk, s.config.SnipePrice)
		return analysis
	}

	// Calculate position size based on confidence and limits
	// Higher confidence = larger position (within limits)
	analysis.Confidence = calculateConfidence(winnerGammaPrice, priceGap, 0, momentum, analysis.Side == "UP")

	// Calculate max loss for this trade (cost of position if it loses)
	positionSize := s.calculatePositionSize(analysis.Confidence, winnerSize)
	analysis.MaxLoss = positionSize * analysis.EntryPrice

	// Check 6: Max loss per trade
	if analysis.MaxLoss > s.maxLossPerTrade {
		// Reduce position to fit max loss
		positionSize = s.maxLossPerTrade / analysis.EntryPrice
		analysis.MaxLoss = positionSize * analysis.EntryPrice
	}

	// Check 7: Would this trade exceed daily limit?
	if currentDailyLoss+analysis.MaxLoss > s.dailyLossLimit {
		remainingBudget := s.dailyLossLimit - currentDailyLoss
		positionSize = remainingBudget / analysis.EntryPrice
		analysis.MaxLoss = positionSize * analysis.EntryPrice

		if positionSize < 1.0 { // Minimum meaningful position
			analysis.SkipReason = SkipReasonDailyLimit
			analysis.SkipDescription = fmt.Sprintf("remaining budget $%.2f too small", remainingBudget)
			return analysis
		}
	}

	// Expected profit if we win: ($1.00 - entry) * shares
	sharesCount := positionSize / analysis.EntryPrice
	analysis.ExpectedProfit = (1.0 - analysis.EntryPrice) * sharesCount

	analysis.ShouldTrade = true
	return analysis
}

// calculateConfidence computes a 0-1 confidence score based on multiple factors.
func calculateConfidence(winnerBid, bidGap, spreadPercent, momentum float64, isYes bool) float64 {
	// Base confidence from bid price (0.65 bid = 0.65 confidence)
	confidence := winnerBid

	// Bonus for large bid gap (clear separation)
	if bidGap > 0.30 {
		confidence += 0.05
	} else if bidGap > 0.20 {
		confidence += 0.03
	}

	// Penalty for wide spread (less confident market)
	if spreadPercent > 0.03 {
		confidence -= 0.05
	}

	// Bonus for momentum in our direction
	if isYes && momentum > momentumThreshold {
		confidence += 0.05
	} else if !isYes && momentum < -momentumThreshold {
		confidence += 0.05
	}

	// Clamp to 0-1 range
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0 {
		confidence = 0
	}

	return confidence
}

// calculatePositionSize determines how much to trade based on confidence.
func (s *Sniper) calculatePositionSize(confidence float64, availableSize float64) float64 {
	// Scale position by confidence
	// 0.65 confidence = 50% of max, 0.90 confidence = 100% of max
	confidenceScale := (confidence - 0.50) / 0.50 // 0.5->0, 1.0->1
	if confidenceScale < 0.5 {
		confidenceScale = 0.5
	}
	if confidenceScale > 1.0 {
		confidenceScale = 1.0
	}

	targetSize := s.config.MaxPositionSize * confidenceScale

	// Don't exceed available liquidity
	if targetSize > availableSize*0.8 { // Take max 80% of book
		targetSize = availableSize * 0.8
	}

	return targetSize
}

// logAnalysis logs the trade analysis with all relevant details.
func (s *Sniper) logAnalysis(tracked *TrackedMarket, analysis TradeAnalysis, timeRemaining time.Duration) {
	yesBid, yesAsk, noBid, noAsk := tracked.GetPrices()

	if !analysis.ShouldTrade {
		log.Printf("[sniper] SKIP %s: %s - %s",
			tracked.Market.Question, analysis.SkipReason, analysis.SkipDescription)
		log.Printf("[sniper]   prices: YES(bid:%.4f/ask:%.4f) NO(bid:%.4f/ask:%.4f) momentum:%.4f",
			yesBid, yesAsk, noBid, noAsk, analysis.Momentum)
		return
	}

	log.Printf("[sniper] SIGNAL %s", tracked.Market.Question)
	log.Printf("[sniper]   side:%s entry:%.4f confidence:%.2f%% spread:%.2f%%",
		analysis.Side, analysis.EntryPrice, analysis.Confidence*100, analysis.SpreadPercent*100)
	log.Printf("[sniper]   expected_profit:$%.2f max_loss:$%.2f liquidity:$%.2f",
		analysis.ExpectedProfit, analysis.MaxLoss, analysis.AvailableSize)
	log.Printf("[sniper]   prices: YES(bid:%.4f/ask:%.4f) NO(bid:%.4f/ask:%.4f)",
		yesBid, yesAsk, noBid, noAsk)
	log.Printf("[sniper]   momentum:%.4f time_remaining:%v", analysis.Momentum, timeRemaining)
}

// executeSnipe executes the trade based on analysis.
func (s *Sniper) executeSnipe(tracked *TrackedMarket, analysis TradeAnalysis, timeRemaining time.Duration) error {
	// Record potential loss for daily tracking
	s.dailyStats.AddLoss(analysis.MaxLoss)

	if s.config.DryRun {
		log.Printf("[sniper] DRY_RUN: WOULD BUY %s at %.4f (confidence: %.2f%%)",
			analysis.Side, analysis.EntryPrice, analysis.Confidence*100)

		if s.telegram != nil {
			msg := fmt.Sprintf("DRY RUN - Would buy %s at %.4f\n"+
				"Market: %s\n"+
				"Confidence: %.1f%%\n"+
				"Expected Profit: $%.2f\n"+
				"Max Loss: $%.2f",
				analysis.Side, analysis.EntryPrice, tracked.Market.Question,
				analysis.Confidence*100, analysis.ExpectedProfit, analysis.MaxLoss)
			if err := s.telegram.SendMessage(msg); err != nil {
				log.Printf("[sniper] telegram error: %v", err)
			}
		}

		tracked.MarkSniped()
		return nil
	}

	// Calculate actual size in dollars
	size := analysis.MaxLoss / analysis.EntryPrice * analysis.EntryPrice // This equals MaxLoss

	// Build and submit FOK order at actual ask price
	orderReq, err := s.builder.BuildFOKBuyOrder(analysis.TokenID, analysis.EntryPrice, size)
	if err != nil {
		return fmt.Errorf("failed to build order: %w", err)
	}

	resp, err := s.clob.CreateOrder(orderReq)
	if err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("order rejected: %s", resp.Error)
	}

	log.Printf("[sniper] ORDER FILLED: %s at %.4f (order ID: %s)", analysis.Side, analysis.EntryPrice, resp.OrderID)
	log.Printf("[sniper]   actual_cost:$%.2f expected_profit:$%.2f", analysis.MaxLoss, analysis.ExpectedProfit)

	if s.telegram != nil {
		if err := s.telegram.NotifyOrderExecuted(analysis.Side, analysis.EntryPrice, size, analysis.ExpectedProfit); err != nil {
			log.Printf("[sniper] telegram error: %v", err)
		}
	}

	tracked.MarkSniped()
	return nil
}

// cleanupExpiredMarkets removes markets that have ended from tracking.
func (s *Sniper) cleanupExpiredMarkets() {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for conditionID, tracked := range s.activeMarkets {
		// Remove markets that ended more than 1 minute ago
		if now.Sub(tracked.EndTime) > 1*time.Minute {
			// Unsubscribe from WebSocket
			if err := s.ws.Unsubscribe(tracked.YesTokenID); err != nil {
				log.Printf("[sniper] unsubscribe error: %v", err)
			}
			if err := s.ws.Unsubscribe(tracked.NoTokenID); err != nil {
				log.Printf("[sniper] unsubscribe error: %v", err)
			}

			delete(s.activeMarkets, conditionID)
			log.Printf("[sniper] cleaned up expired market: %s", tracked.Market.Question)
		}
	}
}

// modeString returns "LIVE" or "DRY_RUN" based on config.
func (s *Sniper) modeString() string {
	if s.config.DryRun {
		return "DRY_RUN"
	}
	return "LIVE"
}

// GetActiveMarkets returns a snapshot of currently tracked markets.
func (s *Sniper) GetActiveMarkets() []TrackedMarket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TrackedMarket, 0, len(s.activeMarkets))
	for _, m := range s.activeMarkets {
		result = append(result, *m)
	}
	return result
}

// Stats holds current statistics about the sniper.
type Stats struct {
	ActiveMarkets   int
	Mode            string
	SnipePrice      float64
	TriggerSecs     int
	DailyLoss       float64
	DailyTradeCount int
}

// GetStats returns current sniper statistics.
func (s *Sniper) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Stats{
		ActiveMarkets:   len(s.activeMarkets),
		Mode:            s.modeString(),
		SnipePrice:      s.config.SnipePrice,
		TriggerSecs:     s.config.TriggerSeconds,
		DailyLoss:       s.dailyStats.GetTotalLoss(),
		DailyTradeCount: s.dailyStats.TradeCount,
	}
}

// logStatus logs the current status of tracked markets.
func (s *Sniper) logStatus() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.activeMarkets) == 0 {
		log.Printf("[status] no markets tracked, waiting for new markets...")
		return
	}

	now := time.Now()
	for _, tracked := range s.activeMarkets {
		timeRemaining := tracked.EndTime.Sub(now)

		tracked.mu.RLock()
		gammaYes := tracked.GammaYesPrice
		gammaNo := tracked.GammaNoPrice
		tracked.mu.RUnlock()

		if timeRemaining > 0 {
			// Determine likely winner
			winner := "UP"
			prob := gammaYes
			if gammaNo > gammaYes {
				winner = "DOWN"
				prob = gammaNo
			}
			log.Printf("[status] %s - ends in %v", tracked.Market.Question, timeRemaining.Truncate(time.Second))
			log.Printf("[status]   gamma: UP=%.1f%% DOWN=%.1f%% => likely %s", gammaYes*100, gammaNo*100, winner)
			log.Printf("[status]   confidence: %.1f%% (need >%.0f%% to trade)", prob*100, minWinnerConfidence*100)
		} else {
			log.Printf("[status] %s - ENDED (cleanup pending)", tracked.Market.Question)
		}
	}
}
