package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	version = "0.1.0"
	banner  = `
 _   _ ____  ____   ____      _    ____  ____  ____   _____     _______
| | | / ___||  _ \ / ___|    / \  |  _ \|  _ \|  _ \ / _ \ \   / / ____|
| | | \___ \| | | | |       / _ \ | |_) | |_) | |_) | | | \ \ / /|  _|
| |_| |___) | |_| | |___   / ___ \|  __/|  __/|  _ <| |_| |\ V / | |___
 \___/|____/|____/ \____| /_/   \_\_|   |_|   |_| \_\\___/  \_/  |_____|

USDC Approval Tool v%s
One-time USDC approval for Polymarket CTF Exchange
`
)

var (
	usdcAddress     = common.HexToAddress("0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174")
	ctfExchange     = common.HexToAddress("0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E")
	maxUint256      = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	erc20ApproveABI = `[{"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]`
)

func main() {
	log.SetFlags(log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[approve] ")

	fmt.Printf(banner, version)
	fmt.Println(strings.Repeat("-", 70))

	cfg, err := config.LoadWithPrivateKey()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Println("initializing wallet...")
	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("failed to create wallet: %v", err)
	}

	log.Printf("wallet address: %s", w.AddressHex())
	log.Printf("USDC contract:  %s", usdcAddress.Hex())
	log.Printf("spender (CTF):  %s", ctfExchange.Hex())
	log.Printf("amount:         MAX (2^256 - 1)")
	log.Printf("chain ID:       %d", cfg.PolygonChainID)
	log.Printf("RPC URL:        %s", cfg.PolygonRPCURL)
	fmt.Println(strings.Repeat("-", 70))

	if !confirmAction() {
		log.Println("operation cancelled by user")
		os.Exit(0)
	}

	log.Println("connecting to Polygon RPC...")
	client, err := ethclient.Dial(cfg.PolygonRPCURL)
	if err != nil {
		log.Fatalf("failed to connect to RPC: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log.Println("fetching account nonce...")
	nonce, err := client.PendingNonceAt(ctx, w.Address())
	if err != nil {
		log.Fatalf("failed to get nonce: %v", err)
	}

	log.Println("fetching gas price...")
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		log.Fatalf("failed to get gas price: %v", err)
	}

	callData, err := buildApproveCallData(ctfExchange, maxUint256)
	if err != nil {
		log.Fatalf("failed to build call data: %v", err)
	}

	gasLimit := uint64(60000)

	tx := types.NewTransaction(
		nonce,
		usdcAddress,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		callData,
	)

	chainID := big.NewInt(int64(cfg.PolygonChainID))
	signedTx, err := signTransaction(tx, w, chainID)
	if err != nil {
		log.Fatalf("failed to sign transaction: %v", err)
	}

	log.Println("sending transaction...")
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		log.Fatalf("failed to send transaction: %v", err)
	}

	txHash := signedTx.Hash().Hex()
	fmt.Println(strings.Repeat("-", 70))
	log.Printf("transaction submitted successfully")
	log.Printf("tx hash: %s", txHash)
	log.Printf("view on PolygonScan: https://polygonscan.com/tx/%s", txHash)
	fmt.Println(strings.Repeat("-", 70))

	log.Println("waiting for confirmation (this may take a minute)...")

	receipt, err := waitForReceipt(ctx, client, signedTx.Hash())
	if err != nil {
		log.Printf("warning: failed to get receipt: %v", err)
		log.Println("transaction may still be pending, check PolygonScan for status")
		os.Exit(0)
	}

	if receipt.Status == types.ReceiptStatusSuccessful {
		log.Printf("transaction confirmed in block %d", receipt.BlockNumber.Uint64())
		log.Println("USDC approval successful - you can now trade on Polymarket")
	} else {
		log.Fatalf("transaction failed - check PolygonScan for details")
	}
}

func confirmAction() bool {
	fmt.Println()
	fmt.Println("This will approve the Polymarket CTF Exchange to spend your USDC.")
	fmt.Println("This is a one-time operation required before trading.")
	fmt.Println()
	fmt.Print("Do you want to proceed? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("failed to read input: %v", err)
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "yes" || input == "y"
}

func buildApproveCallData(spender common.Address, amount *big.Int) ([]byte, error) {
	parsedABI, err := abi.JSON(strings.NewReader(erc20ApproveABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("approve", spender, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to pack call data: %w", err)
	}

	return data, nil
}

func signTransaction(tx *types.Transaction, w *wallet.Wallet, chainID *big.Int) (*types.Transaction, error) {
	signer := types.NewEIP155Signer(chainID)
	txHash := signer.Hash(tx)

	signature, err := w.Sign(txHash.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	if signature[64] < 27 {
		signature[64] += 27
	}

	signedTx, err := tx.WithSignature(signer, signature)
	if err != nil {
		return nil, fmt.Errorf("failed to attach signature: %w", err)
	}

	return signedTx, nil
}

func waitForReceipt(ctx context.Context, client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			receipt, err := client.TransactionReceipt(ctx, txHash)
			if err != nil {
				continue
			}
			return receipt, nil
		}
	}
}
