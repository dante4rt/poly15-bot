package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"math/big"

	"github.com/dantezy/polymarket-sniper/internal/config"
	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func main() {
	cfg, err := config.LoadWithPrivateKey()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	w, err := wallet.NewWalletFromHex(cfg.PrivateKey)
	if err != nil {
		log.Fatalf("Failed to create wallet: %v", err)
	}

	signer := wallet.NewSigner(w)

	// Create a test order with known values
	testOrder := &wallet.Order{
		Salt:          big.NewInt(12345),
		Maker:         w.Address(),
		Signer:        w.Address(),
		Taker:         common.Address{},
		TokenID:       big.NewInt(123456789),
		MakerAmount:   big.NewInt(1000000), // 1 USDC
		TakerAmount:   big.NewInt(1000000), // 1 token
		Expiration:    big.NewInt(0),
		Nonce:         big.NewInt(0),
		FeeRateBps:    big.NewInt(0),
		Side:          wallet.SideBuy,
		SignatureType: wallet.SignatureTypeEOA,
	}

	// Print expected type hashes
	domainTypeHash := crypto.Keccak256Hash(
		[]byte("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
	)
	fmt.Printf("EIP712Domain type hash: 0x%s\n", hex.EncodeToString(domainTypeHash.Bytes()))

	orderTypeHash := crypto.Keccak256Hash(
		[]byte("Order(uint256 salt,address maker,address signer,address taker,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint256 expiration,uint256 nonce,uint256 feeRateBps,uint8 side,uint8 signatureType)"),
	)
	fmt.Printf("Order type hash: 0x%s\n", hex.EncodeToString(orderTypeHash.Bytes()))

	// Print name and version hashes
	nameHash := crypto.Keccak256Hash([]byte("Polymarket CTF Exchange"))
	fmt.Printf("Name hash: 0x%s\n", hex.EncodeToString(nameHash.Bytes()))

	versionHash := crypto.Keccak256Hash([]byte("1"))
	fmt.Printf("Version hash: 0x%s\n", hex.EncodeToString(versionHash.Bytes()))

	// Print domain separator
	fmt.Printf("\nDomain separator: 0x%s\n", hex.EncodeToString(signer.DomainSeparator().Bytes()))

	// Print order hash
	orderHash, err := signer.GetOrderHash(testOrder)
	if err != nil {
		log.Fatalf("Failed to get order hash: %v", err)
	}
	fmt.Printf("Order hash (digest): 0x%s\n", hex.EncodeToString(orderHash.Bytes()))

	// Sign the order
	sig, err := signer.SignOrder(testOrder)
	if err != nil {
		log.Fatalf("Failed to sign order: %v", err)
	}
	fmt.Printf("Signature: %s\n", sig)

	// Print order details
	fmt.Printf("\nOrder details:\n")
	fmt.Printf("  Salt: %s\n", testOrder.Salt.String())
	fmt.Printf("  Maker: %s\n", testOrder.Maker.Hex())
	fmt.Printf("  Signer: %s\n", testOrder.Signer.Hex())
	fmt.Printf("  Taker: %s\n", testOrder.Taker.Hex())
	fmt.Printf("  TokenID: %s\n", testOrder.TokenID.String())
	fmt.Printf("  MakerAmount: %s\n", testOrder.MakerAmount.String())
	fmt.Printf("  TakerAmount: %s\n", testOrder.TakerAmount.String())
	fmt.Printf("  Expiration: %s\n", testOrder.Expiration.String())
	fmt.Printf("  Nonce: %s\n", testOrder.Nonce.String())
	fmt.Printf("  FeeRateBps: %s\n", testOrder.FeeRateBps.String())
	fmt.Printf("  Side: %d\n", testOrder.Side)
	fmt.Printf("  SignatureType: %d\n", testOrder.SignatureType)

	// Verify signature recovery
	fmt.Printf("\n--- Verification ---\n")
	sigBytes, _ := hex.DecodeString(sig[2:]) // Remove 0x prefix
	fmt.Printf("Signature length: %d bytes\n", len(sigBytes))
	fmt.Printf("V value: %d\n", sigBytes[64])
}
