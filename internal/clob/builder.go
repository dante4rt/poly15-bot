package clob

import (
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/dantezy/polymarket-sniper/internal/wallet"
	"github.com/ethereum/go-ethereum/common"
)

const (
	// USDC has 6 decimals on Polygon
	usdcDecimals = 6
	// Default fee rate in basis points (0 = no fee)
	defaultFeeRateBps = 0
	// Default expiration (1 hour from now)
	defaultExpirationSeconds = 3600
	// Tick size for prices (0.001)
	tickSize = 0.001
	// Price decimal precision (tick size = 10^-priceDecimals)
	priceDecimals = 3
)

// OrderBuilder constructs and signs orders for the CLOB.
type OrderBuilder struct {
	signer           *wallet.Signer   // Standard CTF Exchange signer
	negRiskSigner    *wallet.Signer   // Neg Risk CTF Exchange signer
	maker            common.Address   // The maker/funder address (proxy wallet if set, else EOA)
	signerAddr       common.Address   // The EOA that signs orders
	apiKey           string           // API key used as owner for orders
	nonce            *big.Int
	signatureType    uint8            // 0=EOA, 1=POLY_PROXY, 2=GNOSIS_SAFE
}

// NewOrderBuilder creates a new OrderBuilder with the given wallet and API key.
// This creates an EOA-mode builder (signature type 0).
func NewOrderBuilder(w *wallet.Wallet, apiKey string) *OrderBuilder {
	signer := wallet.NewSigner(w)
	negRiskSigner := wallet.NewSignerWithConfig(w, wallet.ChainID, wallet.NegRiskExchangeContract)
	return &OrderBuilder{
		signer:        signer,
		negRiskSigner: negRiskSigner,
		maker:         w.Address(),
		signerAddr:    w.Address(),
		apiKey:        apiKey,
		nonce:         big.NewInt(0),
		signatureType: wallet.SignatureTypeEOA, // Type 0
	}
}

// NewOrderBuilderWithProxy creates an OrderBuilder that uses a Polymarket proxy wallet.
// The proxyWalletAddress is the Gnosis Safe address that holds the user's funds.
// signatureType should be:
//   - 1 (POLY_PROXY) for Magic Link email/Google login accounts
//   - 2 (GNOSIS_SAFE) for browser wallet (MetaMask) connected accounts
func NewOrderBuilderWithProxy(w *wallet.Wallet, apiKey string, proxyWalletAddress common.Address, signatureType int) *OrderBuilder {
	signer := wallet.NewSigner(w)
	negRiskSigner := wallet.NewSignerWithConfig(w, wallet.ChainID, wallet.NegRiskExchangeContract)

	// Validate signature type, default to GNOSIS_SAFE if invalid
	sigType := uint8(signatureType)
	if sigType > 2 {
		sigType = wallet.SignatureTypePolyGnosis // Default to type 2
	}

	return &OrderBuilder{
		signer:        signer,
		negRiskSigner: negRiskSigner,
		maker:         proxyWalletAddress, // The proxy wallet is the maker/funder
		signerAddr:    w.Address(),        // The EOA signs the orders
		apiKey:        apiKey,
		nonce:         big.NewInt(0),
		signatureType: sigType,
	}
}

// NewOrderBuilderWithConfig creates an OrderBuilder with custom chain configuration.
// Use this for testnet deployments.
func NewOrderBuilderWithConfig(w *wallet.Wallet, apiKey string, chainID int64, exchangeAddress common.Address) *OrderBuilder {
	signer := wallet.NewSignerWithConfig(w, chainID, exchangeAddress)
	return &OrderBuilder{
		signer: signer,
		maker:  w.Address(),
		apiKey: apiKey,
		nonce:  big.NewInt(0),
	}
}

// SetNonce sets the nonce for subsequent orders.
// The CLOB uses nonce for order cancellation groups.
func (b *OrderBuilder) SetNonce(nonce *big.Int) {
	b.nonce = new(big.Int).Set(nonce)
}

// Address returns the wallet address used for orders.
func (b *OrderBuilder) Address() common.Address {
	return b.maker
}

// BuildParams holds parameters for building an order.
type BuildParams struct {
	TokenID     string
	Side        OrderSide
	Price       float64   // Price in range [0, 1]
	Size        float64   // Size in USDC
	OrderType   OrderType
	Expiration  int64     // Unix timestamp, 0 for default
	FeeRateBps  int       // Fee rate in basis points, -1 for default
	NegRisk     bool      // True if market uses Neg Risk CTF Exchange
}

// BuildOrder creates a signed order request.
func (b *OrderBuilder) BuildOrder(params BuildParams) (*OrderRequest, error) {
	if params.Price <= 0 || params.Price >= 1 {
		return nil, fmt.Errorf("price must be between 0 and 1 exclusive, got %f", params.Price)
	}
	if params.Size <= 0 {
		return nil, fmt.Errorf("size must be positive, got %f", params.Size)
	}

	// Generate random salt for order uniqueness
	salt, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Calculate amounts using integer math to avoid float precision issues.
	// Polymarket requirements:
	// - Price must be at tick size (0.001) - implicit price from amounts must match
	// - BUY orders: makerAmount (USDC) calculated from size*price, takerAmount = size (tokens)
	// - SELL orders: makerAmount = size (tokens), takerAmount (USDC) calculated from size*price
	// - All amounts in wei (6 decimals)

	// CRITICAL: For implicit price to be at tick size, we must:
	// 1. Round price to tick size FIRST
	// 2. Calculate amounts based on rounded price
	// This ensures makerAmount/takerAmount = rounded_price (at tick size)

	// Round price to tick size (0.001)
	priceRounded := roundToTickSize(params.Price)

	// Convert to integer representations for precise calculation
	// priceInt = rounded_price * 1000 (milli-units, guaranteed integer since price is at tick)
	// sizeInt = floor(size * 100) (centi-units, 2 decimal precision)
	priceInt := int64(math.Round(priceRounded * 1000))
	sizeInt := int64(math.Floor(params.Size * 100))

	// sizeWei = sizeInt * 10000 (convert centi-units to wei)
	sizeWei := sizeInt * 10000

	var makerAmount, takerAmount *big.Int

	if params.Side == OrderSideBuy {
		// Buying tokens: pay USDC, receive tokens
		// costWei = sizeWei * priceInt / 1000
		// This ensures costWei / sizeWei = priceInt / 1000 = priceRounded
		costWei := (sizeWei * priceInt) / 1000
		makerAmount = big.NewInt(costWei)
		takerAmount = big.NewInt(sizeWei)
	} else {
		// Selling tokens: pay tokens, receive USDC
		makerAmount = big.NewInt(sizeWei)
		proceedsWei := (sizeWei * priceInt) / 1000
		takerAmount = big.NewInt(proceedsWei)
	}

	// Set expiration
	// For GTC and FOK orders, expiration must be 0
	// Only GTD orders use a non-zero expiration
	var expiration int64
	if params.OrderType == OrderTypeGTD {
		expiration = params.Expiration
		if expiration == 0 {
			expiration = time.Now().Unix() + defaultExpirationSeconds
		}
	} else {
		expiration = 0
	}

	// Set fee rate
	feeRate := params.FeeRateBps
	if feeRate < 0 {
		feeRate = defaultFeeRateBps
	}

	// Parse token ID
	tokenIDBig, ok := new(big.Int).SetString(params.TokenID, 10)
	if !ok {
		return nil, fmt.Errorf("invalid token ID: %s", params.TokenID)
	}

	// Use the signature type configured for this builder
	// Type 0 (EOA): Standalone wallet
	// Type 1 (POLY_PROXY): Polymarket email/Google login
	// Type 2 (GNOSIS_SAFE): Browser wallet (MetaMask) connected to Polymarket
	sigType := b.signatureType

	// Build the order struct for signing
	// For proxy wallet: maker = proxy wallet, signer = EOA
	// For EOA: maker = signer = EOA
	order := &wallet.Order{
		Salt:          salt,
		Maker:         b.maker,
		Signer:        b.signerAddr,
		Taker:         common.Address{}, // Any taker
		TokenID:       tokenIDBig,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Expiration:    big.NewInt(expiration),
		Nonce:         new(big.Int).Set(b.nonce),
		FeeRateBps:    big.NewInt(int64(feeRate)),
		Side:          sideToUint8(params.Side),
		SignatureType: sigType,
	}

	// Sign the order using the appropriate signer (standard vs neg risk exchange)
	var signature string
	if params.NegRisk {
		signature, err = b.negRiskSigner.SignOrder(order)
	} else {
		signature, err = b.signer.SignOrder(order)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to sign order: %w", err)
	}

	// Convert to API order format
	// Note: Polymarket expects lowercase addresses (not checksummed)
	apiOrder := Order{
		Salt:          salt.Int64(),
		Maker:         strings.ToLower(b.maker.Hex()),
		Signer:        strings.ToLower(b.signerAddr.Hex()),
		Taker:         strings.ToLower(common.Address{}.Hex()), // zero address for any taker
		TokenID:       params.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Expiration:    strconv.FormatInt(expiration, 10),
		Nonce:         b.nonce.String(),
		FeeRateBps:    strconv.Itoa(feeRate),
		Side:          string(params.Side),
		SignatureType: int(sigType),
		Signature:     signature,
	}

	return &OrderRequest{
		Order:     apiOrder,
		Owner:     b.apiKey, // API key is used as owner
		OrderType: string(params.OrderType),
	}, nil
}

// BuildFOKOrder creates a Fill-Or-Kill order for sniping.
func (b *OrderBuilder) BuildFOKOrder(tokenID string, side OrderSide, price, size float64) (*OrderRequest, error) {
	return b.BuildOrder(BuildParams{
		TokenID:    tokenID,
		Side:       side,
		Price:      price,
		Size:       size,
		OrderType:  OrderTypeFOK,
		FeeRateBps: defaultFeeRateBps,
	})
}

// BuildFOKBuyOrder is a helper for the common case: buy at a price, fill or kill.
func (b *OrderBuilder) BuildFOKBuyOrder(tokenID string, price, size float64) (*OrderRequest, error) {
	return b.BuildFOKOrder(tokenID, OrderSideBuy, price, size)
}

// BuildFOKSellOrder is a helper for selling with fill or kill.
func (b *OrderBuilder) BuildFOKSellOrder(tokenID string, price, size float64) (*OrderRequest, error) {
	return b.BuildFOKOrder(tokenID, OrderSideSell, price, size)
}

// BuildGTCBuyOrder creates a good-till-cancelled buy order.
// negRisk should be true if the market uses the Neg Risk CTF Exchange.
func (b *OrderBuilder) BuildGTCBuyOrder(tokenID string, price, size float64, negRisk bool) (*OrderRequest, error) {
	return b.BuildOrder(BuildParams{
		TokenID:    tokenID,
		Side:       OrderSideBuy,
		Price:      price,
		Size:       size,
		OrderType:  OrderTypeGTC,
		FeeRateBps: defaultFeeRateBps,
		NegRisk:    negRisk,
	})
}

// BuildGTCSellOrder creates a good-till-cancelled sell order.
func (b *OrderBuilder) BuildGTCSellOrder(tokenID string, price, size float64) (*OrderRequest, error) {
	return b.BuildOrder(BuildParams{
		TokenID:    tokenID,
		Side:       OrderSideSell,
		Price:      price,
		Size:       size,
		OrderType:  OrderTypeGTC,
		FeeRateBps: defaultFeeRateBps,
	})
}

// generateSalt generates a cryptographically random salt for order uniqueness.
// Returns a random int64 in range [0, 2^32) to match official Polymarket implementation.
func generateSalt() (*big.Int, error) {
	// Generate random number in range [0, 2^32) as per official go-order-utils
	maxInt := big.NewInt(1 << 32) // 2^32
	salt, err := rand.Int(rand.Reader, maxInt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

// roundToTickSize rounds a price to the nearest tick size (0.001).
// This ensures the price is valid for Polymarket's tick size rules.
func roundToTickSize(price float64) float64 {
	return math.Round(price/tickSize) * tickSize
}

// floatToUSDCWei converts a float USDC amount to wei (6 decimals).
func floatToUSDCWei(amount float64) *big.Int {
	// Multiply by 10^6 for USDC decimals
	multiplier := new(big.Float).SetInt(big.NewInt(1e6))
	amountFloat := new(big.Float).SetFloat64(amount)
	result := new(big.Float).Mul(amountFloat, multiplier)

	wei, _ := result.Int(nil)
	return wei
}

// truncateToDecimals truncates (rounds down) a float to the specified number of decimal places.
// This matches Polymarket's amount calculation logic.
func truncateToDecimals(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Floor(value*multiplier) / multiplier
}

// sideToUint8 converts OrderSide to uint8 for signing.
func sideToUint8(side OrderSide) uint8 {
	if side == OrderSideBuy {
		return wallet.SideBuy
	}
	return wallet.SideSell
}

// CalculateCost returns the USDC cost for a buy order as a scaled integer string.
// For example, CalculateCost(0.65, 100) returns "65000000" (65 USDC in 6-decimal format).
func CalculateCost(price, size float64) string {
	cost := price * size
	wei := floatToUSDCWei(cost)
	return wei.String()
}

// CalculateProceeds returns the USDC proceeds for a sell order as a scaled integer string.
func CalculateProceeds(price, size float64) string {
	return CalculateCost(price, size)
}

// CalculateShares returns the number of shares in scaled integer format.
// For example, CalculateShares(100) returns "100000000" (100 shares in 6-decimal format).
func CalculateShares(size float64) string {
	wei := floatToUSDCWei(size)
	return wei.String()
}
