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

const (
	version = "0.1.0"
	banner  = `
 ____   ___  _  __   ____  _   _ ___ ____  _____ ____
|  _ \ / _ \| | \ \ / /  \/  |/ ___| ___ \|_   _|  _ \
| |_) | | | | |  \ V /| |\/| |\__ \   ) | | | | |_) |
|  __/| |_| | |___| | | |  | |___) |  / /  | | |  __/
|_|    \___/|_____|_| |_|  |_|____/____|   |_| |_|

Sniper Bot v%s
Automated trading for 15-minute BTC/ETH markets
`
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[sniper] ")

	fmt.Printf(banner, version)
	fmt.Println(strings.Repeat("-", 60))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	printConfig(cfg)

	log.Println("initializing wallet...")
	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("failed to create wallet: %v", err)
	}
	log.Printf("wallet address: %s", w.AddressHex())

	log.Println("initializing telegram bot...")
	bot, err := telegram.NewBot(cfg.TelegramBotToken, cfg.TelegramChatID)
	if err != nil {
		log.Fatalf("failed to create telegram bot: %v", err)
	}
	bot.SetDryRun(cfg.DryRun)

	log.Println("initializing sniper strategy...")
	sniper, err := strategy.NewSniper(cfg, w, bot)
	if err != nil {
		log.Fatalf("failed to create sniper: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("received signal: %v, initiating shutdown...", sig)
		cancel()
	}()

	if err := bot.NotifyStarted(); err != nil {
		log.Printf("warning: failed to send startup notification: %v", err)
	}

	log.Println("starting sniper strategy...")
	fmt.Println(strings.Repeat("-", 60))

	if err := sniper.Run(ctx); err != nil && err != context.Canceled {
		log.Printf("strategy error: %v", err)
		bot.NotifyError(err)
	}

	log.Println("shutting down...")

	if err := bot.NotifyStopped(); err != nil {
		log.Printf("warning: failed to send shutdown notification: %v", err)
	}

	log.Println("shutdown complete")
	os.Exit(0)
}

func printConfig(cfg *config.Config) {
	mode := "LIVE"
	if cfg.DryRun {
		mode = "DRY RUN"
	}

	telegramStatus := "disabled"
	if cfg.HasTelegram() {
		telegramStatus = "enabled"
	}

	log.Printf("mode:             %s", mode)
	log.Printf("chain ID:         %d", cfg.PolygonChainID)
	log.Printf("max position:     $%.2f", cfg.MaxPositionSize)
	log.Printf("snipe price:      %.2f", cfg.SnipePrice)
	log.Printf("trigger seconds:  %d", cfg.TriggerSeconds)
	log.Printf("telegram:         %s", telegramStatus)
	fmt.Println(strings.Repeat("-", 60))
}
