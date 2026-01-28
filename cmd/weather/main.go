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
 __        _______    _  _____ _   _ _____ ____
 \ \      / / ____|  / \|_   _| | | | ____|  _ \
  \ \ /\ / /|  _|   / _ \ | | | |_| |  _| | |_) |
   \ V  V / | |___ / ___ \| | |  _  | |___|  _ <
    \_/\_/  |_____/_/   \_\_| |_| |_|_____|_| \_\

 ____  _   _ ___ ____  _____ ____
/ ___|| \ | |_ _|  _ \| ____|  _ \
\___ \|  \| || || |_) |  _| | |_) |
 ___) | |\  || ||  __/| |___|  _ <
|____/|_| \_|___|_|   |_____|_| \_\

Weather Temperature Sniper v0.1.0
Exploit mispricings between forecasts and Polymarket odds
`

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[weather] ")

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
	if cfg.WeatherBalance > 0 {
		log.Printf("balance:          $%.2f (from WEATHER_BALANCE)", cfg.WeatherBalance)
	} else {
		log.Printf("balance:          (will try API, fallback: $%.2f)", cfg.WeatherBankroll)
	}
	log.Printf("bet percent:      %.0f%%", cfg.WeatherBetPercent*100)
	log.Printf("min edge:         %.0f%%", cfg.WeatherMinEdge*100)
	log.Printf("min confidence:   %.0f%%", cfg.WeatherMinConfidence*100)
	log.Printf("max position:     $%.2f", cfg.WeatherMaxPosition)
	log.Printf("daily loss limit: $%.2f", cfg.WeatherDailyLossLimit)
	log.Printf("min volume:       $%.0f", cfg.WeatherMinVolume)
	log.Printf("max spread:       %.0f%%", cfg.WeatherMaxSpread*100)
	log.Printf("bid discount:     %.0f%%", cfg.WeatherBidDiscount*100)

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

	// Initialize weather sniper
	log.Println("initializing weather sniper...")
	sniper, err := strategy.NewWeatherSniper(cfg, w, tg)
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
	log.Println("starting weather sniper...")
	log.Println("strategy: exploit forecast/market price divergence")
	log.Println("focus: US major cities temperature markets")
	log.Println("data source: Open-Meteo (free, no auth)")
	fmt.Println(strings.Repeat("-", 60))

	// Send startup notification
	if tg != nil {
		tg.SendMessage(fmt.Sprintf("Weather Sniper Started [%s]\n\n"+
			"Bankroll: $%.2f\n"+
			"Min Edge: %.0f%%\n"+
			"Min Confidence: %.0f%%\n"+
			"Daily Loss Limit: $%.2f",
			mode, cfg.WeatherBankroll,
			cfg.WeatherMinEdge*100,
			cfg.WeatherMinConfidence*100,
			cfg.WeatherDailyLossLimit))
	}

	// Run the sniper
	if err := sniper.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("sniper error: %v", err)
	}

	log.Println("shutdown complete")
}
