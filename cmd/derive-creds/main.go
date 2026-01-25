package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	baseURL = "https://clob.polymarket.com"
)

// ClobAuthDomain EIP-712 constants
const (
	clobDomainName = "ClobAuthDomain"
	clobVersion    = "1"
	authMessage    = "This message attests that I control the given wallet"
)

var (
	clobAuthDomainTypeHash = crypto.Keccak256Hash(
		[]byte("EIP712Domain(string name,string version,uint256 chainId)"),
	)
	clobAuthTypeHash = crypto.Keccak256Hash(
		[]byte("ClobAuth(address address,string timestamp,uint256 nonce,string message)"),
	)
)

// ApiCreds holds the derived API credentials
type ApiCreds struct {
	ApiKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

func main() {
	fmt.Println("Polymarket API Credential Derivation Tool")
	fmt.Println("==========================================")

	// Load config (only need private key)
	cfg, err := config.LoadWithPrivateKey()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize wallet
	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("Failed to create wallet: %v", err)
	}

	fmt.Printf("Wallet address: %s\n", w.AddressHex())
	fmt.Printf("Chain ID: %d\n", cfg.PolygonChainID)
	fmt.Println()

	// Derive API credentials
	creds, err := deriveApiKey(w, int64(cfg.PolygonChainID))
	if err != nil {
		log.Fatalf("Failed to derive API credentials: %v", err)
	}

	fmt.Println("Successfully derived API credentials!")
	fmt.Println()
	fmt.Println("Add these to your .env file:")
	fmt.Println("-----------------------------")
	fmt.Printf("CLOB_API_KEY=%s\n", creds.ApiKey)
	fmt.Printf("CLOB_SECRET=%s\n", creds.Secret)
	fmt.Printf("CLOB_PASSPHRASE=%s\n", creds.Passphrase)
}

func deriveApiKey(w *wallet.Wallet, chainID int64) (*ApiCreds, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := 0

	// Build EIP-712 signature
	signature, err := buildClobAuthSignature(w, chainID, timestamp, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to sign auth message: %w", err)
	}

	// Make request to derive API key
	url := baseURL + "/auth/derive-api-key"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("POLY_ADDRESS", w.AddressHex())
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_NONCE", strconv.Itoa(nonce))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var creds ApiCreds
	if err := json.Unmarshal(body, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}

	return &creds, nil
}

func buildClobAuthSignature(w *wallet.Wallet, chainID int64, timestamp string, nonce int) (string, error) {
	// Compute domain separator
	domainSeparator := computeClobAuthDomainSeparator(chainID)

	// Compute struct hash
	structHash := computeClobAuthStructHash(w.AddressHex(), timestamp, nonce)

	// Compute EIP-712 digest
	digest := crypto.Keccak256Hash(
		[]byte{0x19, 0x01},
		domainSeparator[:],
		structHash[:],
	)

	// Sign
	signature, err := w.Sign(digest.Bytes())
	if err != nil {
		return "", err
	}

	// Adjust V value from 0/1 to 27/28
	if signature[64] < 27 {
		signature[64] += 27
	}

	return "0x" + hex.EncodeToString(signature), nil
}

func computeClobAuthDomainSeparator(chainID int64) [32]byte {
	nameHash := crypto.Keccak256Hash([]byte(clobDomainName))
	versionHash := crypto.Keccak256Hash([]byte(clobVersion))

	chainIDBig := big.NewInt(chainID)
	chainIDBytes := make([]byte, 32)
	chainIDBig.FillBytes(chainIDBytes)

	return crypto.Keccak256Hash(
		clobAuthDomainTypeHash.Bytes(),
		nameHash.Bytes(),
		versionHash.Bytes(),
		chainIDBytes,
	)
}

func computeClobAuthStructHash(address, timestamp string, nonce int) [32]byte {
	// Address type: 20-byte address left-padded to 32 bytes
	addr := common.HexToAddress(address)
	addressPadded := make([]byte, 32)
	copy(addressPadded[12:], addr.Bytes())

	// String types: keccak256 hash of the string
	timestampHash := crypto.Keccak256Hash([]byte(timestamp))
	messageHash := crypto.Keccak256Hash([]byte(authMessage))

	// Uint type: uint256 padded to 32 bytes
	nonceBig := big.NewInt(int64(nonce))
	nonceBytes := make([]byte, 32)
	nonceBig.FillBytes(nonceBytes)

	return crypto.Keccak256Hash(
		clobAuthTypeHash.Bytes(),
		addressPadded,
		timestampHash.Bytes(),
		nonceBytes,
		messageHash.Bytes(),
	)
}
