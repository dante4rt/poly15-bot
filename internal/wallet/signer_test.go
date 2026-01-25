package wallet

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Test private key (DO NOT use in production)
const testPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func TestNewWalletFromHex(t *testing.T) {
	tests := []struct {
		name        string
		hexKey      string
		wantAddress string
		wantErr     bool
	}{
		{
			name:        "valid key without prefix",
			hexKey:      testPrivateKey,
			wantAddress: "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			wantErr:     false,
		},
		{
			name:        "valid key with 0x prefix",
			hexKey:      "0x" + testPrivateKey,
			wantAddress: "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
			wantErr:     false,
		},
		{
			name:    "invalid key - too short",
			hexKey:  "abc123",
			wantErr: true,
		},
		{
			name:    "invalid key - not hex",
			hexKey:  "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet, err := NewWalletFromHex(tt.hexKey)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if wallet.AddressHex() != tt.wantAddress {
				t.Errorf("address mismatch: got %s, want %s", wallet.AddressHex(), tt.wantAddress)
			}
		})
	}
}

func TestSignOrder(t *testing.T) {
	wallet, err := NewWalletFromHex(testPrivateKey)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	signer := NewSigner(wallet)

	order := &Order{
		Salt:          big.NewInt(12345),
		Maker:         wallet.Address(),
		Signer:        wallet.Address(),
		Taker:         common.Address{}, // Any taker
		TokenID:       big.NewInt(100),
		MakerAmount:   big.NewInt(1000000), // 1 USDC (6 decimals)
		TakerAmount:   big.NewInt(500000),  // 0.5 outcome tokens
		Expiration:    big.NewInt(0),       // No expiration
		Nonce:         big.NewInt(1),
		FeeRateBps:    big.NewInt(0),
		Side:          SideBuy,
		SignatureType: SignatureTypeEOA,
	}

	sig, err := signer.SignOrder(order)
	if err != nil {
		t.Fatalf("failed to sign order: %v", err)
	}

	// Verify signature format
	if !strings.HasPrefix(sig, "0x") {
		t.Error("signature should have 0x prefix")
	}

	// Signature should be 65 bytes = 130 hex chars + "0x"
	if len(sig) != 132 {
		t.Errorf("invalid signature length: got %d, want 132", len(sig))
	}

	// Parse and verify V value is 27 or 28
	_, _, v, err := ParseSignature(sig)
	if err != nil {
		t.Fatalf("failed to parse signature: %v", err)
	}
	if v != 27 && v != 28 {
		t.Errorf("V value should be 27 or 28, got %d", v)
	}
}

func TestSignOrderRecovery(t *testing.T) {
	wallet, err := NewWalletFromHex(testPrivateKey)
	if err != nil {
		t.Fatalf("failed to create wallet: %v", err)
	}

	signer := NewSigner(wallet)

	order := &Order{
		Salt:          big.NewInt(67890),
		Maker:         wallet.Address(),
		Signer:        wallet.Address(),
		Taker:         common.Address{},
		TokenID:       big.NewInt(200),
		MakerAmount:   big.NewInt(2000000),
		TakerAmount:   big.NewInt(1000000),
		Expiration:    big.NewInt(1735689600), // Some future timestamp
		Nonce:         big.NewInt(2),
		FeeRateBps:    big.NewInt(100), // 1%
		Side:          SideSell,
		SignatureType: SignatureTypeEOA,
	}

	sigRaw, err := signer.SignOrderRaw(order)
	if err != nil {
		t.Fatalf("failed to sign order: %v", err)
	}

	// Get the order hash
	orderHash, err := signer.GetOrderHash(order)
	if err != nil {
		t.Fatalf("failed to get order hash: %v", err)
	}

	// Recover the public key from signature
	// Need to adjust V back for recovery
	sigForRecovery := make([]byte, 65)
	copy(sigForRecovery, sigRaw)
	if sigForRecovery[64] >= 27 {
		sigForRecovery[64] -= 27
	}

	recoveredPubKey, err := crypto.SigToPub(orderHash.Bytes(), sigForRecovery)
	if err != nil {
		t.Fatalf("failed to recover public key: %v", err)
	}

	recoveredAddress := crypto.PubkeyToAddress(*recoveredPubKey)
	if recoveredAddress != wallet.Address() {
		t.Errorf("recovered address mismatch: got %s, want %s",
			recoveredAddress.Hex(), wallet.AddressHex())
	}
}

func TestDomainSeparator(t *testing.T) {
	wallet, _ := NewWalletFromHex(testPrivateKey)
	signer := NewSigner(wallet)

	// Domain separator should be deterministic
	domainSep := signer.DomainSeparator()
	if domainSep == (common.Hash{}) {
		t.Error("domain separator should not be zero")
	}

	// Creating another signer with same config should produce same domain separator
	signer2 := NewSigner(wallet)
	if signer.DomainSeparator() != signer2.DomainSeparator() {
		t.Error("domain separators should match for same configuration")
	}
}

func TestValidateOrder(t *testing.T) {
	tests := []struct {
		name    string
		order   *Order
		wantErr bool
	}{
		{
			name:    "nil order",
			order:   nil,
			wantErr: true,
		},
		{
			name: "missing salt",
			order: &Order{
				TokenID:       big.NewInt(1),
				MakerAmount:   big.NewInt(1),
				TakerAmount:   big.NewInt(1),
				Expiration:    big.NewInt(0),
				Nonce:         big.NewInt(0),
				FeeRateBps:    big.NewInt(0),
				Side:          SideBuy,
				SignatureType: SignatureTypeEOA,
			},
			wantErr: true,
		},
		{
			name: "invalid side",
			order: &Order{
				Salt:          big.NewInt(1),
				TokenID:       big.NewInt(1),
				MakerAmount:   big.NewInt(1),
				TakerAmount:   big.NewInt(1),
				Expiration:    big.NewInt(0),
				Nonce:         big.NewInt(0),
				FeeRateBps:    big.NewInt(0),
				Side:          5, // Invalid
				SignatureType: SignatureTypeEOA,
			},
			wantErr: true,
		},
		{
			name: "valid order",
			order: &Order{
				Salt:          big.NewInt(1),
				TokenID:       big.NewInt(1),
				MakerAmount:   big.NewInt(1),
				TakerAmount:   big.NewInt(1),
				Expiration:    big.NewInt(0),
				Nonce:         big.NewInt(0),
				FeeRateBps:    big.NewInt(0),
				Side:          SideBuy,
				SignatureType: SignatureTypeEOA,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrder(tt.order)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOrder() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPadTo32Bytes(t *testing.T) {
	tests := []struct {
		name  string
		value *big.Int
		want  int
	}{
		{
			name:  "nil value",
			value: nil,
			want:  32,
		},
		{
			name:  "small value",
			value: big.NewInt(255),
			want:  32,
		},
		{
			name:  "large value",
			value: new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil),
			want:  32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padTo32Bytes(tt.value)
			if len(result) != tt.want {
				t.Errorf("padTo32Bytes() length = %d, want %d", len(result), tt.want)
			}
		})
	}
}

func TestCustomChainConfig(t *testing.T) {
	wallet, _ := NewWalletFromHex(testPrivateKey)

	// Test with Mumbai testnet config
	testnetChainID := int64(80001)
	testnetExchange := common.HexToAddress("0x1234567890123456789012345678901234567890")

	signer := NewSignerWithConfig(wallet, testnetChainID, testnetExchange)

	// Should have different domain separator than mainnet
	mainnetSigner := NewSigner(wallet)
	if signer.DomainSeparator() == mainnetSigner.DomainSeparator() {
		t.Error("testnet and mainnet domain separators should differ")
	}
}
