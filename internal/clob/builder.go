package clob

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
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
)

// OrderBuilder constructs and signs orders for the CLOB.
type OrderBuilder struct {
	signer *wallet.Signer
	maker  common.Address
	nonce  *big.Int
}

// NewOrderBuilder creates a new OrderBuilder with the given wallet.
func NewOrderBuilder(w *wallet.Wallet) *OrderBuilder {
	signer := wallet.NewSigner(w)
	return &OrderBuilder{
		signer: signer,
		maker:  w.Address(),
		nonce:  big.NewInt(0),
	}
}

// NewOrderBuilderWithConfig creates an OrderBuilder with custom chain configuration.
// Use this for testnet deployments.
func NewOrderBuilderWithConfig(w *wallet.Wallet, chainID int64, exchangeAddress common.Address) *OrderBuilder {
	signer := wallet.NewSignerWithConfig(w, chainID, exchangeAddress)
	return &OrderBuilder{
		signer: signer,
		maker:  w.Address(),
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
	Price       float64  // Price in range [0, 1]
	Size        float64  // Size in USDC
	OrderType   OrderType
	Expiration  int64    // Unix timestamp, 0 for default
	FeeRateBps  int      // Fee rate in basis points, -1 for default
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

	// Calculate amounts
	// For BUY: makerAmount = USDC to spend, takerAmount = tokens to receive
	// For SELL: makerAmount = tokens to sell, takerAmount = USDC to receive
	var makerAmount, takerAmount *big.Int

	sizeWei := floatToUSDCWei(params.Size)

	if params.Side == OrderSideBuy {
		// Buying tokens: pay USDC, receive tokens
		// makerAmount = size * price (USDC paying)
		// takerAmount = size (tokens receiving)
		priceWei := floatToUSDCWei(params.Size * params.Price)
		makerAmount = priceWei
		takerAmount = sizeWei
	} else {
		// Selling tokens: pay tokens, receive USDC
		// makerAmount = size (tokens selling)
		// takerAmount = size * price (USDC receiving)
		makerAmount = sizeWei
		priceWei := floatToUSDCWei(params.Size * params.Price)
		takerAmount = priceWei
	}

	// Set expiration
	expiration := params.Expiration
	if expiration == 0 {
		expiration = time.Now().Unix() + defaultExpirationSeconds
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

	// Build the order struct for signing
	order := &wallet.Order{
		Salt:          salt,
		Maker:         b.maker,
		Signer:        b.maker,
		Taker:         common.Address{}, // Any taker
		TokenID:       tokenIDBig,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Expiration:    big.NewInt(expiration),
		Nonce:         new(big.Int).Set(b.nonce),
		FeeRateBps:    big.NewInt(int64(feeRate)),
		Side:          sideToUint8(params.Side),
		SignatureType: wallet.SignatureTypeEOA,
	}

	// Sign the order
	signature, err := b.signer.SignOrder(order)
	if err != nil {
		return nil, fmt.Errorf("failed to sign order: %w", err)
	}

	// Convert to API order format
	apiOrder := Order{
		Maker:         b.maker.Hex(),
		TokenID:       params.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Side:          string(params.Side),
		Expiration:    strconv.FormatInt(expiration, 10),
		Nonce:         b.nonce.String(),
		FeeRateBps:    strconv.Itoa(feeRate),
		Salt:          salt.String(),
		SignatureType: int(wallet.SignatureTypeEOA),
		Signature:     signature,
	}

	return &OrderRequest{
		Order:     apiOrder,
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
func (b *OrderBuilder) BuildGTCBuyOrder(tokenID string, price, size float64) (*OrderRequest, error) {
	return b.BuildOrder(BuildParams{
		TokenID:    tokenID,
		Side:       OrderSideBuy,
		Price:      price,
		Size:       size,
		OrderType:  OrderTypeGTC,
		FeeRateBps: defaultFeeRateBps,
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
func generateSalt() (*big.Int, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(bytes), nil
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
