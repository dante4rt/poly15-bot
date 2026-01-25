package strategy

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/clob"
	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/gamma"
	"github.com/dantezy/polymarket-sniper/internal/sports"
	"github.com/dantezy/polymarket-sniper/internal/telegram"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
)

const (
	sportsCheckInterval = 10 * time.Second  // Check game status every 10s
	sportsScanInterval  = 5 * time.Minute   // Scan for new markets every 5m
	minWinProbability   = 0.95              // Minimum 95% win probability to trade
	gameDecidedLeadNFL  = 21                // 3 TD lead = game decided
)

// TrackedSportsMarket holds state for a sports market being monitored.
type TrackedSportsMarket struct {
	Market     gamma.Market
	YesTokenID string
	NoTokenID  string
	EndTime    time.Time

	// Matched ESPN game
	Game       *sports.Game
	TeamName   string  // Team this market is betting on (e.g., "Rams")

	// Prices from Gamma
	YesPrice   float64
	NoPrice    float64

	// Trade state
	Sniped     bool
	mu         sync.RWMutex
}

// SportsSniper implements the sniping strategy for sports markets.
type SportsSniper struct {
	config   *config.Config
	gamma    *gamma.Client
	espn     *sports.ESPNClient
	clob     *clob.Client
	builder  *clob.OrderBuilder
	telegram *telegram.Bot

	activeMarkets map[string]*TrackedSportsMarket
	mu            sync.RWMutex
}

// NewSportsSniper creates a new SportsSniper instance.
func NewSportsSniper(cfg *config.Config, w *wallet.Wallet, tg *telegram.Bot) (*SportsSniper, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if w == nil {
		return nil, fmt.Errorf("wallet is required")
	}

	return &SportsSniper{
		config:        cfg,
		gamma:         gamma.NewClient(),
		espn:          sports.NewESPNClient(),
		clob:          clob.NewClient(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, w.AddressHex()),
		builder:       clob.NewOrderBuilder(w, cfg.CLOBApiKey),
		telegram:      tg,
		activeMarkets: make(map[string]*TrackedSportsMarket),
	}, nil
}

// Run starts the sports sniper and blocks until context is cancelled.
func (s *SportsSniper) Run(ctx context.Context) error {
	log.Printf("[sports] starting in %s mode", s.modeString())
	log.Printf("[sports] config: max_position=$%.2f, min_win_prob=%.0f%%",
		s.config.MaxPositionSize, minWinProbability*100)

	// Initial scan for markets
	if err := s.ScanForMarkets(); err != nil {
		log.Printf("[sports] initial scan error: %v", err)
	}

	scanTicker := time.NewTicker(sportsScanInterval)
	checkTicker := time.NewTicker(sportsCheckInterval)

	defer scanTicker.Stop()
	defer checkTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[sports] shutting down")
			return ctx.Err()

		case <-scanTicker.C:
			if err := s.ScanForMarkets(); err != nil {
				log.Printf("[sports] scan error: %v", err)
			}

		case <-checkTicker.C:
			if err := s.CheckAndSnipe(); err != nil {
				log.Printf("[sports] check error: %v", err)
			}
		}
	}
}

// ScanForMarkets discovers NFL playoff markets and matches them to ESPN games.
func (s *SportsSniper) ScanForMarkets() error {
	// Get NFL playoff markets from Polymarket
	markets, err := s.gamma.GetNFLPlayoffMarkets()
	if err != nil {
		return fmt.Errorf("failed to fetch sports markets: %w", err)
	}

	// Get live NFL games from ESPN
	games, err := s.espn.GetNFLGames()
	if err != nil {
		log.Printf("[sports] warning: failed to fetch ESPN games: %v", err)
		games = []sports.Game{}
	}

	log.Printf("[sports] found %d playoff markets, %d live games", len(markets), len(games))

	for _, market := range markets {
		s.mu.RLock()
		_, exists := s.activeMarkets[market.Slug]
		s.mu.RUnlock()

		if exists {
			continue
		}

		tracked, err := s.trackMarket(market, games)
		if err != nil {
			log.Printf("[sports] failed to track market %s: %v", market.Slug, err)
			continue
		}

		s.mu.Lock()
		s.activeMarkets[market.Slug] = tracked
		s.mu.Unlock()

		gameInfo := "no matched game"
		if tracked.Game != nil {
			gameInfo = fmt.Sprintf("matched: %s", tracked.Game.ShortName)
		}

		log.Printf("[sports] tracking: %s (%s)", market.Question, gameInfo)
	}

	return nil
}

// trackMarket creates a TrackedSportsMarket and tries to match it to an ESPN game.
func (s *SportsSniper) trackMarket(market gamma.Market, games []sports.Game) (*TrackedSportsMarket, error) {
	endTime, err := market.EndTime()
	if err != nil {
		return nil, fmt.Errorf("failed to parse end time: %w", err)
	}

	yesToken := market.GetYesToken()
	noToken := market.GetNoToken()

	if yesToken == nil || noToken == nil {
		return nil, fmt.Errorf("market missing YES or NO token")
	}

	// Parse outcome prices
	prices := market.ParseOutcomePrices()
	yesPrice, noPrice := 0.5, 0.5
	if len(prices) >= 2 {
		yesPrice = prices[0]
		noPrice = prices[1]
	}

	tracked := &TrackedSportsMarket{
		Market:     market,
		YesTokenID: yesToken.TokenID,
		NoTokenID:  noToken.TokenID,
		EndTime:    endTime,
		YesPrice:   yesPrice,
		NoPrice:    noPrice,
	}

	// Try to extract team name and match to game
	teamName := extractTeamName(market.Question)
	tracked.TeamName = teamName

	// Find matching game
	for i := range games {
		if gameMatchesTeam(&games[i], teamName) {
			tracked.Game = &games[i]
			break
		}
	}

	return tracked, nil
}

// extractTeamName extracts the team name from a market question.
// e.g., "Will the Rams win the NFC Championship?" -> "Rams"
func extractTeamName(question string) string {
	question = strings.ToLower(question)

	teams := map[string]string{
		"patriots": "Patriots",
		"broncos":  "Broncos",
		"rams":     "Rams",
		"seahawks": "Seahawks",
		"chiefs":   "Chiefs",
		"bills":    "Bills",
		"eagles":   "Eagles",
		"49ers":    "49ers",
		"lions":    "Lions",
		"cowboys":  "Cowboys",
		"packers":  "Packers",
		"vikings":  "Vikings",
		"ravens":   "Ravens",
		"texans":   "Texans",
		"commanders": "Commanders",
		"buccaneers": "Buccaneers",
	}

	for key, name := range teams {
		if strings.Contains(question, key) {
			return name
		}
	}

	return ""
}

// gameMatchesTeam checks if a game involves the given team.
func gameMatchesTeam(game *sports.Game, teamName string) bool {
	if teamName == "" {
		return false
	}

	teamLower := strings.ToLower(teamName)
	homeLower := strings.ToLower(game.HomeTeam.Name)
	awayLower := strings.ToLower(game.AwayTeam.Name)

	return strings.Contains(homeLower, teamLower) || strings.Contains(awayLower, teamLower)
}

// CheckAndSnipe evaluates all tracked markets and executes snipes when conditions are met.
func (s *SportsSniper) CheckAndSnipe() error {
	// Refresh ESPN game data
	games, err := s.espn.GetNFLGames()
	if err != nil {
		log.Printf("[sports] warning: failed to refresh games: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for slug, tracked := range s.activeMarkets {
		if tracked.Sniped {
			continue
		}

		// Update game data
		if tracked.Game != nil && len(games) > 0 {
			for i := range games {
				if gameMatchesTeam(&games[i], tracked.TeamName) {
					tracked.Game = &games[i]
					break
				}
			}
		}

		// Refresh market prices
		s.refreshMarketPrices(tracked)

		// Check for snipe opportunity
		analysis := s.analyzeMarket(tracked)

		if analysis.ShouldTrade {
			if err := s.executeSnipe(tracked, analysis); err != nil {
				log.Printf("[sports] snipe error for %s: %v", slug, err)
			}
		}
	}

	return nil
}

// SportsTradeAnalysis contains analysis results for a sports market.
type SportsTradeAnalysis struct {
	ShouldTrade    bool
	Side           string  // "YES" or "NO"
	TokenID        string
	EntryPrice     float64
	WinProbability float64
	ExpectedProfit float64
	Reason         string
}

// analyzeMarket analyzes a sports market for snipe opportunity.
func (s *SportsSniper) analyzeMarket(tracked *TrackedSportsMarket) SportsTradeAnalysis {
	analysis := SportsTradeAnalysis{}

	// If no matched game, we can't analyze
	if tracked.Game == nil {
		analysis.Reason = "no matched ESPN game"
		return analysis
	}

	game := tracked.Game

	// Log game status
	log.Printf("[sports] %s: %s %d - %s %d (Q%d %s) status=%s",
		tracked.TeamName,
		game.HomeTeam.Abbreviation, game.HomeTeam.Score,
		game.AwayTeam.Abbreviation, game.AwayTeam.Score,
		game.Quarter, game.TimeRemaining,
		game.Status)

	// Check if game is final
	if game.Status == sports.StatusFinal {
		winner := game.Winner()
		if winner == nil {
			analysis.Reason = "game ended in tie (no winner)"
			return analysis
		}

		// Does our team win?
		ourTeamWins := strings.Contains(strings.ToLower(winner.Name), strings.ToLower(tracked.TeamName))

		if ourTeamWins {
			analysis.Side = "YES"
			analysis.TokenID = tracked.YesTokenID
			analysis.EntryPrice = tracked.YesPrice
			analysis.WinProbability = 1.0
		} else {
			analysis.Side = "NO"
			analysis.TokenID = tracked.NoTokenID
			analysis.EntryPrice = tracked.NoPrice
			analysis.WinProbability = 1.0
		}

		// Only trade if price is favorable (not already at 0.99)
		if analysis.EntryPrice >= 0.99 {
			analysis.Reason = fmt.Sprintf("game final but price too high (%.2f)", analysis.EntryPrice)
			return analysis
		}

		analysis.ShouldTrade = true
		analysis.ExpectedProfit = (1.0 - analysis.EntryPrice) * s.config.MaxPositionSize
		analysis.Reason = "game final"
		return analysis
	}

	// Check if game is "decided" (big lead late)
	if game.Status == sports.StatusInProgress {
		winProb := game.WinProbability()
		leader := game.Leader()

		if leader == nil {
			analysis.Reason = "game tied"
			return analysis
		}

		if winProb < minWinProbability {
			analysis.Reason = fmt.Sprintf("win probability %.0f%% < %.0f%% threshold",
				winProb*100, minWinProbability*100)
			return analysis
		}

		// High probability - determine if our team is leading
		ourTeamLeading := strings.Contains(strings.ToLower(leader.Name), strings.ToLower(tracked.TeamName))

		if ourTeamLeading {
			analysis.Side = "YES"
			analysis.TokenID = tracked.YesTokenID
			analysis.EntryPrice = tracked.YesPrice
		} else {
			analysis.Side = "NO"
			analysis.TokenID = tracked.NoTokenID
			analysis.EntryPrice = tracked.NoPrice
		}

		analysis.WinProbability = winProb

		// Only trade if price is favorable
		if analysis.EntryPrice >= 0.95 {
			analysis.Reason = fmt.Sprintf("price too high (%.2f) for %.0f%% probability",
				analysis.EntryPrice, winProb*100)
			return analysis
		}

		analysis.ShouldTrade = true
		analysis.ExpectedProfit = (1.0 - analysis.EntryPrice) * s.config.MaxPositionSize * winProb
		analysis.Reason = fmt.Sprintf("high win probability (%.0f%%)", winProb*100)
		return analysis
	}

	analysis.Reason = fmt.Sprintf("game not started or in progress (status=%s)", game.Status)
	return analysis
}

// refreshMarketPrices updates market prices from Gamma API.
func (s *SportsSniper) refreshMarketPrices(tracked *TrackedSportsMarket) {
	market, err := s.gamma.GetMarketBySlug(tracked.Market.Slug)
	if err != nil {
		return
	}

	prices := market.ParseOutcomePrices()
	if len(prices) >= 2 {
		tracked.mu.Lock()
		tracked.YesPrice = prices[0]
		tracked.NoPrice = prices[1]
		tracked.mu.Unlock()
	}
}

// executeSnipe executes the trade.
func (s *SportsSniper) executeSnipe(tracked *TrackedSportsMarket, analysis SportsTradeAnalysis) error {
	log.Printf("[sports] SIGNAL %s", tracked.Market.Question)
	log.Printf("[sports]   side:%s entry:%.4f win_prob:%.0f%% expected_profit:$%.2f",
		analysis.Side, analysis.EntryPrice, analysis.WinProbability*100, analysis.ExpectedProfit)
	log.Printf("[sports]   reason: %s", analysis.Reason)

	if s.config.DryRun {
		log.Printf("[sports] DRY_RUN: WOULD BUY %s at %.4f", analysis.Side, analysis.EntryPrice)

		if s.telegram != nil {
			msg := fmt.Sprintf("SPORTS DRY RUN - Would buy %s at %.4f\n"+
				"Market: %s\n"+
				"Win Probability: %.0f%%\n"+
				"Expected Profit: $%.2f\n"+
				"Reason: %s",
				analysis.Side, analysis.EntryPrice, tracked.Market.Question,
				analysis.WinProbability*100, analysis.ExpectedProfit, analysis.Reason)
			if err := s.telegram.SendMessage(msg); err != nil {
				log.Printf("[sports] telegram error: %v", err)
			}
		}

		tracked.Sniped = true
		return nil
	}

	// Get actual ask price from CLOB
	book, err := s.clob.GetOrderBook(analysis.TokenID)
	if err != nil {
		return fmt.Errorf("failed to get order book: %w", err)
	}

	var actualAsk float64
	if len(book.Asks) > 0 {
		if price, err := strconv.ParseFloat(book.Asks[0].Price, 64); err == nil {
			actualAsk = price
		}
	}

	if actualAsk <= 0 || actualAsk >= 0.99 {
		return fmt.Errorf("no liquidity (ask=%.4f)", actualAsk)
	}

	// Build and submit order
	size := s.config.MaxPositionSize
	orderReq, err := s.builder.BuildFOKBuyOrder(analysis.TokenID, actualAsk, size)
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

	log.Printf("[sports] ORDER FILLED: %s at %.4f (order ID: %s)",
		analysis.Side, actualAsk, resp.OrderID)

	if s.telegram != nil {
		if err := s.telegram.NotifyOrderExecuted(analysis.Side, actualAsk, size, analysis.ExpectedProfit); err != nil {
			log.Printf("[sports] telegram error: %v", err)
		}
	}

	tracked.Sniped = true
	return nil
}

func (s *SportsSniper) modeString() string {
	if s.config.DryRun {
		return "DRY_RUN"
	}
	return "LIVE"
}

// GetActiveMarkets returns currently tracked markets.
func (s *SportsSniper) GetActiveMarkets() []TrackedSportsMarket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TrackedSportsMarket, 0, len(s.activeMarkets))
	for _, m := range s.activeMarkets {
		result = append(result, *m)
	}
	return result
}
