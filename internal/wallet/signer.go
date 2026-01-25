package wallet

import (
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Polymarket CLOB EIP-712 Domain Constants
const (
	DomainName = "Polymarket CTF Exchange"
	ChainID    = 137 // Polygon Mainnet
)

// Polymarket CTF Exchange contract address on Polygon
var ExchangeContract = common.HexToAddress("0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E")

// Order side constants
const (
	SideBuy  uint8 = 0
	SideSell uint8 = 1
)

// Signature type constants
const (
	SignatureTypeEOA   uint8 = 0
	SignatureTypePoly  uint8 = 1
	SignatureTypePolyGnosis uint8 = 2
)

var (
	ErrInvalidOrder = errors.New("invalid order parameters")

	// EIP-712 type hashes (pre-computed for gas efficiency)
	eip712DomainTypeHash = crypto.Keccak256Hash(
		[]byte("EIP712Domain(string name,uint256 chainId,address verifyingContract)"),
	)

	orderTypeHash = crypto.Keccak256Hash(
		[]byte("Order(uint256 salt,address maker,address signer,address taker,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint256 expiration,uint256 nonce,uint256 feeRateBps,uint8 side,uint8 signatureType)"),
	)
)

// Order represents a Polymarket CLOB order for EIP-712 signing.
type Order struct {
	Salt          *big.Int       // Random salt for uniqueness
	Maker         common.Address // Order creator address
	Signer        common.Address // Signing address (can differ from maker for delegated signing)
	Taker         common.Address // Counterparty (0x0 for any taker)
	TokenID       *big.Int       // Conditional token ID
	MakerAmount   *big.Int       // Amount maker is selling
	TakerAmount   *big.Int       // Amount maker wants to receive
	Expiration    *big.Int       // Unix timestamp expiration (0 for no expiration)
	Nonce         *big.Int       // Nonce for order cancellation
	FeeRateBps    *big.Int       // Fee rate in basis points
	Side          uint8          // 0 = Buy, 1 = Sell
	SignatureType uint8          // 0 = EOA, 1 = Poly, 2 = PolyGnosis
}

// Signer handles EIP-712 typed data signing for Polymarket orders.
type Signer struct {
	wallet          *Wallet
	domainSeparator common.Hash
	chainID         *big.Int
	exchangeAddress common.Address
}

// NewSigner creates a new Signer with the default Polymarket domain.
func NewSigner(wallet *Wallet) *Signer {
	return NewSignerWithConfig(wallet, ChainID, ExchangeContract)
}

// NewSignerWithConfig creates a new Signer with custom domain configuration.
// Use this for testnet or custom deployments.
func NewSignerWithConfig(wallet *Wallet, chainID int64, exchangeAddress common.Address) *Signer {
	chainIDBig := big.NewInt(chainID)
	domainSeparator := computeDomainSeparator(DomainName, chainIDBig, exchangeAddress)

	return &Signer{
		wallet:          wallet,
		domainSeparator: domainSeparator,
		chainID:         chainIDBig,
		exchangeAddress: exchangeAddress,
	}
}

// SignOrder signs a Polymarket order using EIP-712 typed data signing.
// Returns the signature as a hex string with "0x" prefix.
func (s *Signer) SignOrder(order *Order) (string, error) {
	if err := validateOrder(order); err != nil {
		return "", err
	}

	structHash := hashOrder(order)
	digest := computeEIP712Digest(s.domainSeparator, structHash)

	signature, err := s.wallet.Sign(digest.Bytes())
	if err != nil {
		return "", err
	}

	// Adjust V value from 0/1 to 27/28 for Ethereum compatibility
	if signature[64] < 27 {
		signature[64] += 27
	}

	return "0x" + hex.EncodeToString(signature), nil
}

// SignOrderRaw signs a Polymarket order and returns the raw 65-byte signature.
func (s *Signer) SignOrderRaw(order *Order) ([]byte, error) {
	if err := validateOrder(order); err != nil {
		return nil, err
	}

	structHash := hashOrder(order)
	digest := computeEIP712Digest(s.domainSeparator, structHash)

	signature, err := s.wallet.Sign(digest.Bytes())
	if err != nil {
		return nil, err
	}

	// Adjust V value from 0/1 to 27/28 for Ethereum compatibility
	if signature[64] < 27 {
		signature[64] += 27
	}

	return signature, nil
}

// GetOrderHash returns the EIP-712 digest hash for an order without signing.
// Useful for order identification and verification.
func (s *Signer) GetOrderHash(order *Order) (common.Hash, error) {
	if err := validateOrder(order); err != nil {
		return common.Hash{}, err
	}

	structHash := hashOrder(order)
	return computeEIP712Digest(s.domainSeparator, structHash), nil
}

// DomainSeparator returns the cached EIP-712 domain separator.
func (s *Signer) DomainSeparator() common.Hash {
	return s.domainSeparator
}

// Wallet returns the underlying wallet.
func (s *Signer) Wallet() *Wallet {
	return s.wallet
}

// computeDomainSeparator calculates the EIP-712 domain separator.
func computeDomainSeparator(name string, chainID *big.Int, verifyingContract common.Address) common.Hash {
	nameHash := crypto.Keccak256Hash([]byte(name))

	return crypto.Keccak256Hash(
		eip712DomainTypeHash.Bytes(),
		nameHash.Bytes(),
		padTo32Bytes(chainID),
		padAddress(verifyingContract),
	)
}

// hashOrder computes the EIP-712 struct hash for an Order.
func hashOrder(order *Order) common.Hash {
	return crypto.Keccak256Hash(
		orderTypeHash.Bytes(),
		padTo32Bytes(order.Salt),
		padAddress(order.Maker),
		padAddress(order.Signer),
		padAddress(order.Taker),
		padTo32Bytes(order.TokenID),
		padTo32Bytes(order.MakerAmount),
		padTo32Bytes(order.TakerAmount),
		padTo32Bytes(order.Expiration),
		padTo32Bytes(order.Nonce),
		padTo32Bytes(order.FeeRateBps),
		padUint8(order.Side),
		padUint8(order.SignatureType),
	)
}

// computeEIP712Digest computes the final EIP-712 digest.
// Format: keccak256("\x19\x01" + domainSeparator + structHash)
func computeEIP712Digest(domainSeparator, structHash common.Hash) common.Hash {
	return crypto.Keccak256Hash(
		[]byte{0x19, 0x01},
		domainSeparator.Bytes(),
		structHash.Bytes(),
	)
}

// validateOrder performs basic validation on order fields.
func validateOrder(order *Order) error {
	if order == nil {
		return ErrInvalidOrder
	}
	if order.Salt == nil || order.TokenID == nil || order.MakerAmount == nil ||
		order.TakerAmount == nil || order.Expiration == nil || order.Nonce == nil ||
		order.FeeRateBps == nil {
		return ErrInvalidOrder
	}
	if order.Side > 1 {
		return ErrInvalidOrder
	}
	if order.SignatureType > 2 {
		return ErrInvalidOrder
	}
	return nil
}

// padTo32Bytes pads a big.Int to 32 bytes (left-padded with zeros).
func padTo32Bytes(value *big.Int) []byte {
	if value == nil {
		return make([]byte, 32)
	}
	bytes := value.Bytes()
	if len(bytes) >= 32 {
		return bytes[:32]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(bytes):], bytes)
	return padded
}

// padAddress pads an address to 32 bytes (left-padded with zeros).
func padAddress(addr common.Address) []byte {
	padded := make([]byte, 32)
	copy(padded[12:], addr.Bytes())
	return padded
}

// padUint8 pads a uint8 to 32 bytes (left-padded with zeros).
func padUint8(value uint8) []byte {
	padded := make([]byte, 32)
	padded[31] = value
	return padded
}

// ParseSignature parses a hex signature string into its components (r, s, v).
func ParseSignature(sigHex string) (r, s *big.Int, v uint8, err error) {
	sigHex = strings.TrimPrefix(sigHex, "0x")
	if len(sigHex) != 130 {
		return nil, nil, 0, errors.New("invalid signature length")
	}

	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, nil, 0, err
	}

	r = new(big.Int).SetBytes(sigBytes[0:32])
	s = new(big.Int).SetBytes(sigBytes[32:64])
	v = sigBytes[64]

	return r, s, v, nil
}
