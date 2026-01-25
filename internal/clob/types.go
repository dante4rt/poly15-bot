package clob

// OrderBook represents the current state of bids and asks for a token.
type OrderBook struct {
	Bids []PriceLevel `json:"bids"`
	Asks []PriceLevel `json:"asks"`
	Hash string       `json:"hash"`
}

// PriceLevel represents a single price level in the order book.
type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// Order represents a signed order on the CLOB.
type Order struct {
	Salt          int64  `json:"salt"`          // integer, not string
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	Side          string `json:"side"` // "BUY" or "SELL"
	SignatureType int    `json:"signatureType"`
	Signature     string `json:"signature"`
}

// OrderSide represents the side of an order.
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// OrderType represents the type of order execution.
type OrderType string

const (
	OrderTypeFOK OrderType = "FOK" // Fill or Kill
	OrderTypeGTC OrderType = "GTC" // Good Till Cancelled
	OrderTypeGTD OrderType = "GTD" // Good Till Date
)

// OrderRequest represents a request to create a new order.
type OrderRequest struct {
	Order     Order  `json:"order"`
	Owner     string `json:"owner"`     // wallet address
	OrderType string `json:"orderType"`
}

// OrderResponse represents the response from order creation.
type OrderResponse struct {
	Success bool   `json:"success"`
	OrderID string `json:"orderID"`
	Error   string `json:"error,omitempty"`
}

// CancelOrderRequest represents a request to cancel an order.
type CancelOrderRequest struct {
	OrderID string `json:"orderID"`
}

// OpenOrdersResponse represents the response from fetching open orders.
type OpenOrdersResponse struct {
	Orders []Order `json:"orders"`
}

// APIError represents an error response from the CLOB API.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return e.Message
}

// AssetType represents the type of asset for balance queries.
type AssetType string

const (
	AssetTypeCollateral  AssetType = "COLLATERAL"  // USDC balance
	AssetTypeConditional AssetType = "CONDITIONAL" // Position tokens
)

// BalanceAllowanceResponse represents the balance and allowance for an asset.
type BalanceAllowanceResponse struct {
	Balance   string `json:"balance"`
	Allowance string `json:"allowance"`
}
