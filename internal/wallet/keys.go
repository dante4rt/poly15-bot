package wallet

import (
	"crypto/ecdsa"
	"errors"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrInvalidPrivateKey = errors.New("invalid private key format")
	ErrNilPrivateKey     = errors.New("private key is nil")
)

// Wallet holds the private key and derived address for signing operations.
type Wallet struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

// NewWalletFromHex creates a new Wallet from a hex-encoded private key.
// The hex string can optionally include the "0x" prefix.
func NewWalletFromHex(hexKey string) (*Wallet, error) {
	cleanKey := strings.TrimPrefix(hexKey, "0x")
	cleanKey = strings.TrimPrefix(cleanKey, "0X")

	if len(cleanKey) != 64 {
		return nil, ErrInvalidPrivateKey
	}

	privateKey, err := crypto.HexToECDSA(cleanKey)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, ErrInvalidPrivateKey
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	return &Wallet{
		privateKey: privateKey,
		address:    address,
	}, nil
}

// Address returns the Ethereum address derived from the private key.
func (w *Wallet) Address() common.Address {
	return w.address
}

// AddressHex returns the Ethereum address as a checksummed hex string.
func (w *Wallet) AddressHex() string {
	return w.address.Hex()
}

// PrivateKey returns the underlying ECDSA private key.
// Use with caution - prefer using Sign methods instead.
func (w *Wallet) PrivateKey() *ecdsa.PrivateKey {
	return w.privateKey
}

// Sign signs the provided hash with the wallet's private key.
// Returns a 65-byte signature in [R || S || V] format.
func (w *Wallet) Sign(hash []byte) ([]byte, error) {
	if w.privateKey == nil {
		return nil, ErrNilPrivateKey
	}

	signature, err := crypto.Sign(hash, w.privateKey)
	if err != nil {
		return nil, err
	}

	return signature, nil
}
