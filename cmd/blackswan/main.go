package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/strategy"
	"github.com/dantezy/polymarket-sniper/internal/telegram"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
)

const banner = `
 ____  _        _    ____ _  __  ______        ___    _   _
| __ )| |      / \  / ___| |/ / / ___\ \      / / \  | \ | |
|  _ \| |     / _ \| |   | ' /  \___ \\ \ /\ / / _ \ |  \| |
| |_) | |___ / ___ \ |___| . \   ___) |\ V  V / ___ \| |\  |
|____/|_____/_/   \_\____|_|\_\ |____/  \_/\_/_/   \_\_| \_|

Black Swan Hunter v0.1.0
Power Law Distribution Strategy for Polymarket
`

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[blackswan] ")

	fmt.Print(banner)
	fmt.Println(strings.Repeat("-", 60))

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Log configuration
	mode := "LIVE"
	if cfg.DryRun {
		mode = "DRY RUN"
	}
	log.Printf("mode:             %s", mode)
	log.Printf("chain ID:         %d", cfg.PolygonChainID)
	log.Printf("bankroll:         $%.2f", cfg.MaxPositionSize)
	log.Printf("price range:      %.2f¢ - %.1f¢", cfg.BlackSwanMinPrice*100, cfg.BlackSwanMaxPrice*100)
	log.Printf("bet size:         %.1f%% of bankroll", cfg.BlackSwanBetPercent*100)
	log.Printf("max positions:    %d", cfg.BlackSwanMaxPositions)
	log.Printf("max exposure:     $%.2f", cfg.BlackSwanMaxExposure)
	log.Printf("bid discount:     %.0f%% below market", cfg.BlackSwanBidDiscount*100)

	// Initialize wallet
	log.Println("initializing wallet...")
	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("failed to initialize wallet: %v", err)
	}
	log.Printf("wallet address: %s", w.AddressHex())

	// Initialize telegram (optional)
	var tg *telegram.Bot
	if cfg.HasTelegram() {
		log.Println("initializing telegram bot...")
		tg, err = telegram.NewBot(cfg.TelegramBotToken, cfg.TelegramChatID)
		if err != nil {
			log.Printf("telegram init failed (continuing without): %v", err)
			tg = nil
		} else {
			log.Println("telegram: enabled")
		}
	} else {
		log.Println("telegram: disabled (no credentials)")
	}

	// Initialize Black Swan hunter
	log.Println("initializing black swan hunter...")
	hunter, err := strategy.NewBlackSwanHunter(cfg, w, tg)
	if err != nil {
		log.Fatalf("failed to initialize hunter: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("received signal: %v, initiating shutdown...", sig)
		cancel()
	}()

	fmt.Println(strings.Repeat("-", 60))
	log.Println("starting black swan hunter...")
	log.Println("strategy: buy low-probability events with limit orders")
	log.Println("looking for: overconfident markets resolving soon (fast capital turnover)")
	fmt.Println(strings.Repeat("-", 60))

	// Send startup notification
	if tg != nil {
		tg.SendMessage(fmt.Sprintf("Bot Started [%s]\n\n"+
			"Bankroll: $%.2f\n"+
			"Target: %.1f¢ - %.0f¢\n"+
			"Max Bets: %d",
			mode, cfg.MaxPositionSize,
			cfg.BlackSwanMinPrice*100, cfg.BlackSwanMaxPrice*100,
			cfg.BlackSwanMaxPositions))
	}

	// Run the hunter
	if err := hunter.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("hunter error: %v", err)
	}

	log.Println("shutdown complete")
}
