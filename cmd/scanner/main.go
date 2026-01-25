package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/gamma"
)

const (
	version = "0.1.0"
	banner  = `
 ____   ___  _  __   ____   ____    _    _   _ _   _ _____ ____
|  _ \ / _ \| | \ \ / /  \/  |/ ___|  / \  | \ | | \ | | ____|  _ \
| |_) | | | | |  \ V /| |\/| |\___ \ / _ \ |  \| |  \| |  _| | |_) |
|  __/| |_| | |___| | | |  | | ___) / ___ \| |\  | |\  | |___|  _ <
|_|    \___/|_____|_| |_|  |_||____/_/   \_\_| \_|_| \_|_____|_| \_\

Market Scanner v%s
Finds active 15-minute BTC/ETH markets on Polymarket
`
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[scanner] ")

	fmt.Printf(banner, version)
	fmt.Println(strings.Repeat("-", 70))

	cfg, err := config.LoadMinimal()
	if err != nil {
		log.Printf("warning: failed to load config: %v", err)
		log.Println("continuing with defaults...")
	}

	_ = cfg

	log.Println("initializing gamma client...")
	client := gamma.NewClient()

	log.Println("searching for active 15-minute BTC/ETH markets...")
	markets, err := client.GetActiveUpDownMarkets()
	if err != nil {
		log.Fatalf("failed to fetch markets: %v", err)
	}

	if len(markets) == 0 {
		log.Println("no active 15-minute markets found")
		os.Exit(0)
	}

	fmt.Println()
	printHeader()

	for _, market := range markets {
		printMarket(market)
	}

	fmt.Println()
	log.Printf("found %d active market(s)", len(markets))
}

func printHeader() {
	fmt.Printf("%-60s | %-10s | %-10s | %-12s\n",
		"Question", "Yes Price", "No Price", "Time Left")
	fmt.Println(strings.Repeat("-", 100))
}

func printMarket(market gamma.Market) {
	question := truncate(market.Question, 58)

	yesPrice := "N/A"
	noPrice := "N/A"

	if yesToken := market.GetYesToken(); yesToken != nil {
		yesPrice = fmt.Sprintf("$%.4f", yesToken.Price)
	}

	if noToken := market.GetNoToken(); noToken != nil {
		noPrice = fmt.Sprintf("$%.4f", noToken.Price)
	}

	timeLeft := "unknown"
	if endTime, err := market.EndTime(); err == nil {
		remaining := time.Until(endTime)
		if remaining > 0 {
			timeLeft = formatDuration(remaining)
		} else {
			timeLeft = "ended"
		}
	}

	fmt.Printf("%-60s | %-10s | %-10s | %-12s\n",
		question, yesPrice, noPrice, timeLeft)

	fmt.Printf("  Condition ID: %s\n", market.ConditionID)

	if yesToken := market.GetYesToken(); yesToken != nil {
		fmt.Printf("  Yes Token ID: %s\n", yesToken.TokenID)
	}

	if noToken := market.GetNoToken(); noToken != nil {
		fmt.Printf("  No Token ID:  %s\n", noToken.TokenID)
	}

	fmt.Println()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		return "ended"
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
