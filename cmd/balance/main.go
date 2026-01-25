package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/clob"
	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
)

const (
	version = "0.1.0"
	banner  = `
 ____    _    _        _    _   _  ____ _____
| __ )  / \  | |      / \  | \ | |/ ___| ____|
|  _ \ / _ \ | |     / _ \ |  \| | |   |  _|
| |_) / ___ \| |___ / ___ \| |\  | |___| |___
|____/_/   \_\_____/_/   \_\_| \_|\____|_____|

Polymarket Balance Checker v%s
Check USDC balance and allowance on Polymarket
`
)

// DataAPIPosition represents a position from the Data API.
type DataAPIPosition struct {
	ProxyWallet  string  `json:"proxyWallet"`
	Asset        string  `json:"asset"`
	ConditionID  string  `json:"conditionId"`
	Size         float64 `json:"size"`
	AvgPrice     float64 `json:"avgPrice"`
	CurrentValue float64 `json:"currentValue"`
	CashPnl      float64 `json:"cashPnl"`
	Title        string  `json:"title"`
	Outcome      string  `json:"outcome"`
}

// DataAPIValue represents the holdings value from the Data API.
type DataAPIValue struct {
	User  string  `json:"user"`
	Value float64 `json:"value"`
}

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[balance] ")

	fmt.Printf(banner, version)
	fmt.Println(strings.Repeat("-", 60))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("failed to create wallet: %v", err)
	}

	walletAddr := w.AddressHex()
	log.Printf("EOA wallet:     %s", walletAddr)

	if cfg.ProxyWalletAddress != "" {
		log.Printf("Proxy wallet:   %s", cfg.ProxyWalletAddress)
	}

	fmt.Println(strings.Repeat("-", 60))

	// Create CLOB client - always authenticate with EOA
	var client *clob.Client
	if cfg.ProxyURL != "" {
		client, err = clob.NewClientWithProxy(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr, cfg.ProxyURL)
		if err != nil {
			log.Fatalf("failed to create CLOB client: %v", err)
		}
	} else {
		client = clob.NewClient(cfg.CLOBApiKey, cfg.CLOBSecret, cfg.CLOBPassphrase, walletAddr)
	}

	// Check on-chain USDC balance (most reliable)
	targetWallet := walletAddr
	if cfg.ProxyWalletAddress != "" {
		targetWallet = cfg.ProxyWalletAddress
	}

	log.Printf("Checking on-chain USDC balance for %s...", truncateAddr(targetWallet))
	onChainBalance, err := getOnChainUSDCBalance(targetWallet)
	if err != nil {
		log.Printf("On-chain query error: %v", err)
	} else {
		log.Printf("USDC Balance (on-chain): $%.2f", onChainBalance)
	}

	// Also try CLOB API (may fail for proxy wallets)
	log.Println("Fetching balance from CLOB API...")
	balance, err := client.GetBalanceAllowance(clob.AssetTypeCollateral, "")
	if err != nil {
		log.Printf("CLOB API: %v (expected for proxy wallets)", err)
	} else {
		balanceFloat := parseUSDCBalance(balance.Balance)
		allowanceFloat := parseUSDCBalance(balance.Allowance)
		log.Printf("CLOB Balance:   $%.2f", balanceFloat)
		log.Printf("CLOB Allowance: $%.2f", allowanceFloat)
	}

	fmt.Println(strings.Repeat("-", 60))

	// Check positions via public Data API
	targetAddr := walletAddr
	if cfg.ProxyWalletAddress != "" {
		targetAddr = cfg.ProxyWalletAddress
	}

	log.Printf("Fetching holdings from Data API for %s...", truncateAddr(targetAddr))

	// Get total holdings value
	value, err := getHoldingsValue(targetAddr)
	if err != nil {
		log.Printf("Data API error: %v", err)
	} else if len(value) > 0 {
		log.Printf("Total Holdings Value: $%.2f", value[0].Value)
	} else {
		log.Println("No holdings found")
	}

	// Get positions
	positions, err := getPositions(targetAddr)
	if err != nil {
		log.Printf("Positions error: %v", err)
	} else if len(positions) > 0 {
		fmt.Println(strings.Repeat("-", 60))
		log.Printf("Open Positions (%d):", len(positions))
		for _, p := range positions {
			log.Printf("  %s [%s]: %.2f shares @ $%.2f = $%.2f (P&L: $%.2f)",
				truncateStr(p.Title, 30), p.Outcome, p.Size, p.AvgPrice, p.CurrentValue, p.CashPnl)
		}
	}

	fmt.Println(strings.Repeat("-", 60))

	// Explain proxy wallet if not configured
	if cfg.ProxyWalletAddress == "" {
		log.Println("TIP: If you deposited via Polymarket UI, your USDC is in your proxy wallet.")
		log.Println("Find your proxy wallet address in Polymarket settings and add to .env:")
		log.Println("  PROXY_WALLET_ADDRESS=0x...")
	}
}

func parseUSDCBalance(balanceStr string) float64 {
	if balanceStr == "" {
		return 0
	}
	balance := new(big.Int)
	balance.SetString(balanceStr, 10)
	// USDC has 6 decimals
	divisor := new(big.Int).SetInt64(1e6)
	result := new(big.Float).Quo(
		new(big.Float).SetInt(balance),
		new(big.Float).SetInt(divisor),
	)
	f, _ := result.Float64()
	return f
}

func getHoldingsValue(address string) ([]DataAPIValue, error) {
	url := fmt.Sprintf("https://data-api.polymarket.com/value?user=%s", address)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var value []DataAPIValue
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func getPositions(address string) ([]DataAPIPosition, error) {
	url := fmt.Sprintf("https://data-api.polymarket.com/positions?user=%s&limit=50", address)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var positions []DataAPIPosition
	if err := json.NewDecoder(resp.Body).Decode(&positions); err != nil {
		return nil, err
	}
	return positions, nil
}

func truncateAddr(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// getOnChainUSDCBalance queries the USDC.e contract on Polygon for the balance.
// This is the most reliable way to check balance, especially for proxy wallets.
func getOnChainUSDCBalance(address string) (float64, error) {
	// USDC.e contract on Polygon
	const usdcContract = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
	const polygonRPC = "https://polygon-rpc.com"

	// balanceOf(address) function selector
	const balanceOfSelector = "0x70a08231"

	// Pad the address to 32 bytes (remove 0x prefix, left-pad with zeros)
	addr := strings.TrimPrefix(strings.ToLower(address), "0x")
	paddedAddr := fmt.Sprintf("%064s", addr)

	// Build the eth_call request
	callData := balanceOfSelector + paddedAddr

	requestBody := fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"method": "eth_call",
		"params": [{"to": "%s", "data": "%s"}, "latest"],
		"id": 1
	}`, usdcContract, callData)

	req, err := http.NewRequest(http.MethodPost, polygonRPC, strings.NewReader(requestBody))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("RPC request failed: %w", err)
	}
	defer resp.Body.Close()

	var rpcResponse struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rpcResponse); err != nil {
		return 0, fmt.Errorf("failed to decode response: %w", err)
	}

	if rpcResponse.Error != nil {
		return 0, fmt.Errorf("RPC error: %s", rpcResponse.Error.Message)
	}

	// Parse hex result to big.Int
	result := strings.TrimPrefix(rpcResponse.Result, "0x")
	if result == "" || result == "0" {
		return 0, nil
	}

	balance := new(big.Int)
	balance.SetString(result, 16)

	// USDC has 6 decimals
	divisor := new(big.Int).SetInt64(1e6)
	balanceFloat := new(big.Float).Quo(
		new(big.Float).SetInt(balance),
		new(big.Float).SetInt(divisor),
	)

	f, _ := balanceFloat.Float64()
	return f, nil
}

func init() {
	// Suppress unused import error
	_ = os.Getenv
}
