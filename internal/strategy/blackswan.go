package strategy

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/clob"
	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/gamma"
	"github.com/dantezy/polymarket-sniper/internal/telegram"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
)

const (
	blackSwanScanInterval   = 5 * time.Minute  // Scan for new markets every 5 minutes
	blackSwanCheckInterval  = 30 * time.Second // Check positions every 30 seconds
	blackSwanStatusInterval = 2 * time.Minute  // Log status every 2 minutes
	orderRefreshInterval    = 1 * time.Hour    // Refresh stale orders
	maxOrderAge             = 24 * time.Hour   // Cancel orders older than this
)

// BlackSwanCandidate represents a market that meets Black Swan criteria.
type BlackSwanCandidate struct {
	Market        gamma.Market
	TokenID       string
	Outcome       string // "Yes" or "No"
	CurrentPrice  float64
	BidPrice      float64 // Our limit order price (below market)
	Score         float64 // Higher = better opportunity
	Volume        float64
	EndTime       time.Time
	OverConfident bool // True if one side > 90%
}

// OpenPosition tracks an active limit order.
type OpenPosition struct {
	OrderID      string
	TokenID      string
	MarketSlug   string
	MarketTitle  string
	Outcome      string
	BidPrice     float64
	Size         float64
	PlacedAt     time.Time
	CurrentPrice float64
	Status       string // "open", "filled", "cancelled"
}

// PositionTracker manages open limit orders.
type PositionTracker struct {
	positions map[string]*OpenPosition // orderID -> position
	mu        sync.RWMutex
}

// NewPositionTracker creates a new position tracker.
func NewPositionTracker() *PositionTracker {
	return &PositionTracker{
		positions: make(map[string]*OpenPosition),
	}
}

// Add adds a new position.
func (pt *PositionTracker) Add(pos *OpenPosition) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.positions[pos.OrderID] = pos
}

// Remove removes a position by order ID.
func (pt *PositionTracker) Remove(orderID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.positions, orderID)
}

// Get returns a position by order ID.
func (pt *PositionTracker) Get(orderID string) *OpenPosition {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.positions[orderID]
}

// GetAll returns all positions.
func (pt *PositionTracker) GetAll() []*OpenPosition {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	result := make([]*OpenPosition, 0, len(pt.positions))
	for _, pos := range pt.positions {
		result = append(result, pos)
	}
	return result
}

// Count returns the number of open positions.
func (pt *PositionTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.positions)
}

// TotalExposure returns the total USD at risk.
func (pt *PositionTracker) TotalExposure() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	total := 0.0
	for _, pos := range pt.positions {
		total += pos.Size * pos.BidPrice
	}
	return total
}

// HasMarket checks if we already have a position in a market.
func (pt *PositionTracker) HasMarket(marketSlug string) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	for _, pos := range pt.positions {
		if pos.MarketSlug == marketSlug {
			return true
		}
	}
	return false
}

// BlackSwanHunter implements the power-law distribution betting strategy.
type BlackSwanHunter struct {
	config   *config.Config
	gamma    *gamma.Client
	clob     *clob.Client
	builder  *clob.OrderBuilder
	telegram *telegram.Bot
	tracker  *PositionTracker

	// Bankroll tracking
	bankroll float64
	mu       sync.RWMutex

	// Stats
	totalBets     int
	totalFilled   int
	totalCanceled int
}

// NewBlackSwanHunter creates a new Black Swan strategy instance.
func NewBlackSwanHunter(cfg *config.Config, w *wallet.Wallet, tg *telegram.Bot) (*BlackSwanHunter, error) {
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

	// Create CLOB client with optional proxy rotation
	var clobClient *clob.Client
	walletAddr := w.AddressHex()
	if len(cfg.ProxyURLs) > 1 {
		// Multiple proxies - use rotation
		log.Printf("[blackswan] using %d proxies with rotation", len(cfg.ProxyURLs))
		var err error
		clobClient, err = clob.NewClientWithProxyRotation(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr, cfg.ProxyURLs)
		if err != nil {
			return nil, fmt.Errorf("failed to create CLOB client with proxy rotation: %w", err)
		}
	} else if cfg.ProxyURL != "" {
		// Single proxy
		log.Printf("[blackswan] using proxy: %s", maskProxy(cfg.ProxyURL))
		var err error
		clobClient, err = clob.NewClientWithProxy(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr, cfg.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create CLOB client with proxy: %w", err)
		}
	} else {
		clobClient = clob.NewClient(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr)
	}

	// Create order builder - use proxy wallet if configured
	var builder *clob.OrderBuilder
	if cfg.UseProxyWallet() {
		builder = clob.NewOrderBuilderWithProxy(w, cfg.CLOBApiKey, common.HexToAddress(cfg.ProxyWalletAddress))
	} else {
		builder = clob.NewOrderBuilder(w, cfg.CLOBApiKey)
	}

	return &BlackSwanHunter{
		config:   cfg,
		gamma:    gammaClient,
		clob:     clobClient,
		builder:  builder,
		telegram: tg,
		tracker:  NewPositionTracker(),
		bankroll: cfg.MaxPositionSize, // Use max position as bankroll
	}, nil
}

// Run starts the Black Swan hunter and blocks until context is cancelled.
func (h *BlackSwanHunter) Run(ctx context.Context) error {
	log.Printf("[blackswan] starting in %s mode", h.modeString())
	log.Printf("[blackswan] config: max_price=%.4f (%.1f¢), min_price=%.4f (%.2f¢)",
		h.config.BlackSwanMaxPrice, h.config.BlackSwanMaxPrice*100,
		h.config.BlackSwanMinPrice, h.config.BlackSwanMinPrice*100)
	log.Printf("[blackswan] config: bet_percent=%.1f%%, max_positions=%d, max_exposure=$%.2f",
		h.config.BlackSwanBetPercent*100, h.config.BlackSwanMaxPositions, h.config.BlackSwanMaxExposure)
	log.Printf("[blackswan] config: bid_discount=%.0f%%, volume_range=$%.0f-$%.0f",
		h.config.BlackSwanBidDiscount*100, h.config.BlackSwanMinVolume, h.config.BlackSwanMaxVolume)
	log.Printf("[blackswan] bankroll: $%.2f", h.bankroll)

	// Initial scan
	if err := h.ScanAndBet(); err != nil {
		log.Printf("[blackswan] initial scan error: %v", err)
	}

	scanTicker := time.NewTicker(blackSwanScanInterval)
	checkTicker := time.NewTicker(blackSwanCheckInterval)
	statusTicker := time.NewTicker(blackSwanStatusInterval)

	defer scanTicker.Stop()
	defer checkTicker.Stop()
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[blackswan] shutting down")
			return ctx.Err()

		case <-scanTicker.C:
			if err := h.ScanAndBet(); err != nil {
				log.Printf("[blackswan] scan error: %v", err)
			}

		case <-checkTicker.C:
			if err := h.CheckPositions(); err != nil {
				log.Printf("[blackswan] check error: %v", err)
			}

		case <-statusTicker.C:
			h.logStatus()
		}
	}
}

// ScanAndBet scans for Black Swan opportunities and places bets.
func (h *BlackSwanHunter) ScanAndBet() error {
	log.Printf("[blackswan] scanning for black swan opportunities...")

	candidates, err := h.FindCandidates()
	if err != nil {
		return fmt.Errorf("failed to find candidates: %w", err)
	}

	if len(candidates) == 0 {
		log.Printf("[blackswan] no candidates found matching criteria")
		return nil
	}

	log.Printf("[blackswan] found %d candidates", len(candidates))

	// Sort by score (best first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Place bets on top candidates
	betsPlaced := 0
	for _, candidate := range candidates {
		// Check position limits
		if h.tracker.Count() >= h.config.BlackSwanMaxPositions {
			log.Printf("[blackswan] max positions reached (%d)", h.config.BlackSwanMaxPositions)
			break
		}

		// Check exposure limit
		if h.tracker.TotalExposure() >= h.config.BlackSwanMaxExposure {
			log.Printf("[blackswan] max exposure reached ($%.2f)", h.config.BlackSwanMaxExposure)
			break
		}

		// Skip if we already have position in this market
		if h.tracker.HasMarket(candidate.Market.Slug) {
			continue
		}

		// Place the bet
		if err := h.PlaceBet(candidate); err != nil {
			log.Printf("[blackswan] failed to place bet on %s: %v", candidate.Market.Question, err)
			continue
		}

		betsPlaced++
		if betsPlaced >= 5 { // Max 5 new bets per scan
			break
		}
	}

	log.Printf("[blackswan] placed %d new bets", betsPlaced)
	return nil
}

// FindCandidates searches for markets matching Black Swan criteria.
func (h *BlackSwanHunter) FindCandidates() ([]BlackSwanCandidate, error) {
	// Fetch a large number of active markets
	params := gamma.SearchParams{
		Active: true,
		Closed: false,
		Limit:  500,
	}

	markets, err := h.gamma.SearchMarketsWithParams(params)
	if err != nil {
		return nil, fmt.Errorf("failed to search markets: %w", err)
	}

	var candidates []BlackSwanCandidate

	for _, market := range markets {
		// Skip 15-min markets (use sniper for those)
		if market.Is15MinMarket() {
			continue
		}

		// Get tokens and prices
		yesToken := market.GetYesToken()
		noToken := market.GetNoToken()
		if yesToken == nil || noToken == nil {
			continue
		}

		// Check YES side for black swan opportunity
		if h.isBlackSwanCandidate(yesToken.Price, noToken.Price) {
			candidate := h.buildCandidate(market, yesToken, noToken)
			if candidate != nil {
				candidates = append(candidates, *candidate)
			}
		}

		// Check NO side for black swan opportunity
		if h.isBlackSwanCandidate(noToken.Price, yesToken.Price) {
			candidate := h.buildCandidateNo(market, noToken, yesToken)
			if candidate != nil {
				candidates = append(candidates, *candidate)
			}
		}
	}

	return candidates, nil
}

// isBlackSwanCandidate checks if a price qualifies as a black swan opportunity.
func (h *BlackSwanHunter) isBlackSwanCandidate(price, oppositePrice float64) bool {
	// Price must be in target range (0.1¢ - 5¢)
	if price < h.config.BlackSwanMinPrice || price > h.config.BlackSwanMaxPrice {
		return false
	}

	// Opposite side should be overconfident (>90%)
	if oppositePrice < 0.90 {
		return false
	}

	return true
}

// buildCandidate creates a BlackSwanCandidate for the YES side.
func (h *BlackSwanHunter) buildCandidate(market gamma.Market, yesToken, noToken *gamma.Token) *BlackSwanCandidate {
	endTime, _ := market.EndTime()
	if endTime.IsZero() || endTime.Before(time.Now().Add(24*time.Hour)) {
		// Skip markets ending too soon (less than 24h)
		return nil
	}

	// Calculate bid price (discount from current price)
	bidPrice := yesToken.Price * (1 - h.config.BlackSwanBidDiscount)
	if bidPrice < h.config.BlackSwanMinPrice {
		bidPrice = h.config.BlackSwanMinPrice
	}

	// Score the opportunity (lower price + higher opposite confidence = better)
	score := (1 - yesToken.Price) * noToken.Price * 100

	return &BlackSwanCandidate{
		Market:        market,
		TokenID:       yesToken.TokenID,
		Outcome:       "Yes",
		CurrentPrice:  yesToken.Price,
		BidPrice:      bidPrice,
		Score:         score,
		EndTime:       endTime,
		OverConfident: noToken.Price >= 0.90,
	}
}

// buildCandidateNo creates a BlackSwanCandidate for the NO side.
func (h *BlackSwanHunter) buildCandidateNo(market gamma.Market, noToken, yesToken *gamma.Token) *BlackSwanCandidate {
	endTime, _ := market.EndTime()
	if endTime.IsZero() || endTime.Before(time.Now().Add(24*time.Hour)) {
		return nil
	}

	bidPrice := noToken.Price * (1 - h.config.BlackSwanBidDiscount)
	if bidPrice < h.config.BlackSwanMinPrice {
		bidPrice = h.config.BlackSwanMinPrice
	}

	score := (1 - noToken.Price) * yesToken.Price * 100

	return &BlackSwanCandidate{
		Market:        market,
		TokenID:       noToken.TokenID,
		Outcome:       "No",
		CurrentPrice:  noToken.Price,
		BidPrice:      bidPrice,
		Score:         score,
		EndTime:       endTime,
		OverConfident: yesToken.Price >= 0.90,
	}
}

// PlaceBet places a limit order for a Black Swan candidate.
func (h *BlackSwanHunter) PlaceBet(candidate BlackSwanCandidate) error {
	// Calculate bet amount in USD (% of bankroll)
	betAmountUSD := h.bankroll * h.config.BlackSwanBetPercent

	// Check if this would exceed max exposure
	currentExposure := h.tracker.TotalExposure()
	if currentExposure+betAmountUSD > h.config.BlackSwanMaxExposure {
		betAmountUSD = h.config.BlackSwanMaxExposure - currentExposure
		if betAmountUSD < 0.01 {
			return fmt.Errorf("insufficient remaining exposure")
		}
	}

	// Convert USD amount to number of shares
	// shares = USD / price (e.g., $0.75 / $0.01 = 75 shares)
	shares := betAmountUSD / candidate.BidPrice

	// Polymarket minimum order size is 5 shares
	const minShares = 5.0
	if shares < minShares {
		shares = minShares
		betAmountUSD = shares * candidate.BidPrice
	}

	log.Printf("[blackswan] placing bet: %s %s at %.4f (%.2f¢) shares=%.1f cost=$%.2f",
		candidate.Market.Question, candidate.Outcome,
		candidate.BidPrice, candidate.BidPrice*100, shares, betAmountUSD)

	if h.config.DryRun {
		log.Printf("[blackswan] DRY_RUN: would place GTC limit order")

		// Track as if placed (Size = shares for exposure tracking)
		position := &OpenPosition{
			OrderID:      fmt.Sprintf("dry-%d", time.Now().UnixNano()),
			TokenID:      candidate.TokenID,
			MarketSlug:   candidate.Market.Slug,
			MarketTitle:  candidate.Market.Question,
			Outcome:      candidate.Outcome,
			BidPrice:     candidate.BidPrice,
			Size:         shares,
			PlacedAt:     time.Now(),
			CurrentPrice: candidate.CurrentPrice,
			Status:       "open",
		}
		h.tracker.Add(position)
		h.totalBets++

		if h.telegram != nil {
			msg := fmt.Sprintf("[DRY RUN] Black Swan Bet\n"+
				"Market: %s\n"+
				"Side: %s\n"+
				"Bid: %.4f (%.2f¢)\n"+
				"Shares: %.1f ($%.2f)\n"+
				"Potential: %.0fx return",
				candidate.Market.Question, candidate.Outcome,
				candidate.BidPrice, candidate.BidPrice*100,
				shares, betAmountUSD, 1.0/candidate.BidPrice)
			h.telegram.SendMessage(msg)
		}

		return nil
	}

	// Build GTC limit order (size = number of shares)
	order, err := h.builder.BuildGTCBuyOrder(candidate.TokenID, candidate.BidPrice, shares)
	if err != nil {
		return fmt.Errorf("failed to build order: %w", err)
	}

	// Submit order
	resp, err := h.clob.CreateOrder(order)
	if err != nil {
		return fmt.Errorf("failed to submit order: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("order rejected: %s", resp.Error)
	}

	// Track the position (Size = shares)
	position := &OpenPosition{
		OrderID:      resp.OrderID,
		TokenID:      candidate.TokenID,
		MarketSlug:   candidate.Market.Slug,
		MarketTitle:  candidate.Market.Question,
		Outcome:      candidate.Outcome,
		BidPrice:     candidate.BidPrice,
		Size:         shares,
		PlacedAt:     time.Now(),
		CurrentPrice: candidate.CurrentPrice,
		Status:       "open",
	}
	h.tracker.Add(position)
	h.totalBets++

	log.Printf("[blackswan] ORDER PLACED: %s (order ID: %s)", candidate.Market.Question, resp.OrderID)

	if h.telegram != nil {
		msg := fmt.Sprintf("Black Swan Bet Placed\n"+
			"Market: %s\n"+
			"Side: %s\n"+
			"Bid: %.4f (%.2f¢)\n"+
			"Shares: %.1f ($%.2f)\n"+
			"Order: %s",
			candidate.Market.Question, candidate.Outcome,
			candidate.BidPrice, candidate.BidPrice*100,
			shares, betAmountUSD, resp.OrderID)
		h.telegram.SendMessage(msg)
	}

	return nil
}

// CheckPositions checks the status of open positions and handles fills/cancellations.
func (h *BlackSwanHunter) CheckPositions() error {
	if h.config.DryRun {
		// In dry run, just log positions
		return nil
	}

	// Get open orders from CLOB
	openOrders, err := h.clob.GetOpenOrders()
	if err != nil {
		return fmt.Errorf("failed to get open orders: %w", err)
	}

	// Build map of open order IDs
	openOrderMap := make(map[string]bool)
	for _, order := range openOrders {
		// Order struct has fields including Maker, TokenID, etc
		// Using Salt converted to string as identifier
		openOrderMap[strconv.FormatInt(order.Salt, 10)] = true
	}

	// Check our tracked positions
	for _, pos := range h.tracker.GetAll() {
		// Check if order is still open
		if !openOrderMap[pos.OrderID] {
			// Order was filled or cancelled
			log.Printf("[blackswan] order %s no longer open (was: %s)", pos.OrderID, pos.MarketTitle)
			h.tracker.Remove(pos.OrderID)
			h.totalFilled++ // Assume filled for now
			continue
		}

		// Check if order is too old
		if time.Since(pos.PlacedAt) > maxOrderAge {
			log.Printf("[blackswan] canceling stale order %s (age: %v)", pos.OrderID, time.Since(pos.PlacedAt))
			if err := h.clob.CancelOrder(pos.OrderID); err != nil {
				log.Printf("[blackswan] failed to cancel order %s: %v", pos.OrderID, err)
			} else {
				h.tracker.Remove(pos.OrderID)
				h.totalCanceled++
			}
		}
	}

	return nil
}

// logStatus logs the current status of the hunter.
func (h *BlackSwanHunter) logStatus() {
	positions := h.tracker.GetAll()
	exposure := h.tracker.TotalExposure()

	log.Printf("[blackswan] STATUS: positions=%d, exposure=$%.2f, bets=%d, filled=%d, canceled=%d",
		len(positions), exposure, h.totalBets, h.totalFilled, h.totalCanceled)

	if len(positions) > 0 {
		log.Printf("[blackswan] open positions:")
		for _, pos := range positions {
			age := time.Since(pos.PlacedAt).Truncate(time.Minute)
			log.Printf("[blackswan]   - %s %s @ %.4f (%.2f¢) $%.2f [%v old]",
				pos.MarketTitle, pos.Outcome, pos.BidPrice, pos.BidPrice*100, pos.Size, age)
		}
	}
}

// modeString returns "LIVE" or "DRY_RUN" based on config.
func (h *BlackSwanHunter) modeString() string {
	if h.config.DryRun {
		return "DRY_RUN"
	}
	return "LIVE"
}

// GetStats returns current hunter statistics.
func (h *BlackSwanHunter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"mode":           h.modeString(),
		"positions":      h.tracker.Count(),
		"exposure":       h.tracker.TotalExposure(),
		"total_bets":     h.totalBets,
		"total_filled":   h.totalFilled,
		"total_canceled": h.totalCanceled,
		"bankroll":       h.bankroll,
	}
}

// maskProxy masks the password in a proxy URL for logging.
// Input: "user:pass@host:port" -> Output: "user:***@host:port"
func maskProxy(proxyURL string) string {
	// Find @ separator
	atIdx := -1
	for i, c := range proxyURL {
		if c == '@' {
			atIdx = i
			break
		}
	}
	if atIdx == -1 {
		return proxyURL // No auth
	}

	// Find : in auth part
	colonIdx := -1
	for i, c := range proxyURL[:atIdx] {
		if c == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return proxyURL // No password
	}

	return proxyURL[:colonIdx+1] + "***" + proxyURL[atIdx:]
}
