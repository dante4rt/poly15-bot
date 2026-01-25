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
 ____  ____   ___  ____ _____ ____    ____  _   _ ___ ____  _____ ____
/ ___||  _ \ / _ \|  _ \_   _/ ___|  / ___|| \ | |_ _|  _ \| ____|  _ \
\___ \| |_) | | | | |_) || | \___ \  \___ \|  \| || || |_) |  _| | |_) |
 ___) |  __/| |_| |  _ < | |  ___) |  ___) | |\  || ||  __/| |___|  _ <
|____/|_|    \___/|_| \_\|_| |____/  |____/|_| \_|___|_|   |_____|_| \_\

Sports Sniper Bot v0.1.0
NFL/NBA sniping for Polymarket
`

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[sports] ")

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
	log.Printf("max position:     $%.2f", cfg.MaxPositionSize)

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

	// Initialize sports sniper
	log.Println("initializing sports sniper strategy...")
	sniper, err := strategy.NewSportsSniper(cfg, w, tg)
	if err != nil {
		log.Fatalf("failed to initialize sniper: %v", err)
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
	log.Println("starting sports sniper strategy...")

	// Run the sniper
	if err := sniper.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("sniper error: %v", err)
	}

	log.Println("shutdown complete")
}
