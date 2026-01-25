package clob

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	baseURL            = "https://clob.polymarket.com"
	headerAPIKey       = "POLY-API-KEY"
	headerSignature    = "POLY-SIGNATURE"
	headerTimestamp    = "POLY-TIMESTAMP"
	headerPassphrase   = "POLY-PASSPHRASE"
	defaultTimeout     = 30 * time.Second
)

// Client is the CLOB REST API client with HMAC authentication.
type Client struct {
	apiKey     string
	secret     string
	passphrase string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new CLOB API client.
func NewClient(apiKey, secret, passphrase string) *Client {
	return &Client{
		apiKey:     apiKey,
		secret:     secret,
		passphrase: passphrase,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
	}
}

// WithHTTPClient sets a custom HTTP client.
func (c *Client) WithHTTPClient(client *http.Client) *Client {
	c.httpClient = client
	return c
}

// WithBaseURL sets a custom base URL (useful for testing).
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

// GetOrderBook fetches the order book for a given token.
func (c *Client) GetOrderBook(tokenID string) (*OrderBook, error) {
	path := fmt.Sprintf("/book?token_id=%s", tokenID)

	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var orderBook OrderBook
	if err := json.NewDecoder(resp.Body).Decode(&orderBook); err != nil {
		return nil, fmt.Errorf("failed to decode order book: %w", err)
	}

	return &orderBook, nil
}

// CreateOrder submits a new order to the CLOB.
func (c *Client) CreateOrder(order *OrderRequest) (*OrderResponse, error) {
	body, err := json.Marshal(order)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order: %w", err)
	}

	resp, err := c.doRequest(http.MethodPost, "/order", body)
	if err != nil {
		return nil, fmt.Errorf("failed to create order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		return nil, fmt.Errorf("failed to decode order response: %w", err)
	}

	return &orderResp, nil
}

// CancelOrder cancels an existing order by ID.
func (c *Client) CancelOrder(orderID string) error {
	body, err := json.Marshal(CancelOrderRequest{OrderID: orderID})
	if err != nil {
		return fmt.Errorf("failed to marshal cancel request: %w", err)
	}

	resp, err := c.doRequest(http.MethodDelete, "/order", body)
	if err != nil {
		return fmt.Errorf("failed to cancel order: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// GetOpenOrders fetches all open orders for the authenticated user.
func (c *Client) GetOpenOrders() ([]Order, error) {
	resp, err := c.doRequest(http.MethodGet, "/orders", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var orders []Order
	if err := json.NewDecoder(resp.Body).Decode(&orders); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %w", err)
	}

	return orders, nil
}

// doRequest performs an authenticated HTTP request.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	url := c.baseURL + path
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	signature := c.sign(timestamp, method, path, body)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerAPIKey, c.apiKey)
	req.Header.Set(headerSignature, signature)
	req.Header.Set(headerTimestamp, timestamp)
	req.Header.Set(headerPassphrase, c.passphrase)

	return c.httpClient.Do(req)
}

// sign generates the HMAC-SHA256 signature for a request.
func (c *Client) sign(timestamp, method, path string, body []byte) string {
	var bodyStr string
	if body != nil {
		bodyStr = string(body)
	}

	message := timestamp + method + path + bodyStr

	h := hmac.New(sha256.New, []byte(c.secret))
	h.Write([]byte(message))

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// parseError extracts error information from a failed response.
func (c *Client) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	apiErr.Code = resp.StatusCode
	return &apiErr
}
