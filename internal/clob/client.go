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
	"net/url"
	"strconv"
	"time"

	"golang.org/x/net/proxy"
)

const (
	baseURL            = "https://clob.polymarket.com"
	headerAPIKey       = "POLY_API_KEY"
	headerSignature    = "POLY_SIGNATURE"
	headerTimestamp    = "POLY_TIMESTAMP"
	headerPassphrase   = "POLY_PASSPHRASE"
	headerAddress      = "POLY_ADDRESS"
	defaultTimeout     = 30 * time.Second
)

// Client is the CLOB REST API client with HMAC authentication.
type Client struct {
	apiKey     string
	secret     string
	passphrase string
	address    string // wallet address
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new CLOB API client.
func NewClient(apiKey, secret, passphrase, address string) *Client {
	return &Client{
		apiKey:     apiKey,
		secret:     secret,
		passphrase: passphrase,
		address:    address,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
	}
}

// NewClientWithProxy creates a new CLOB API client with HTTP or SOCKS5 proxy support.
// proxyURL format: "user:pass@host:port" (defaults to HTTP proxy)
// For SOCKS5: prefix with "socks5://" e.g. "socks5://user:pass@host:port"
func NewClientWithProxy(apiKey, secret, passphrase, address, proxyURL string) (*Client, error) {
	var transport *http.Transport

	// Check if it's explicitly a SOCKS5 proxy
	if len(proxyURL) > 9 && proxyURL[:9] == "socks5://" {
		// SOCKS5 proxy
		proxyURL = proxyURL[9:] // Remove socks5:// prefix

		var auth *proxy.Auth
		var addr string

		if u, err := url.Parse("socks5://" + proxyURL); err == nil && u.User != nil {
			auth = &proxy.Auth{
				User: u.User.Username(),
			}
			if pass, ok := u.User.Password(); ok {
				auth.Password = pass
			}
			addr = u.Host
		} else {
			addr = proxyURL
		}

		dialer, err := proxy.SOCKS5("tcp", addr, auth, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}

		transport = &http.Transport{
			Dial: dialer.Dial,
		}
	} else {
		// HTTP/HTTPS proxy (default)
		proxyURLParsed, err := url.Parse("http://" + proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}

		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURLParsed),
		}
	}

	return &Client{
		apiKey:     apiKey,
		secret:     secret,
		passphrase: passphrase,
		address:    address,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
		},
		baseURL: baseURL,
	}, nil
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

	// Debug: read and log response
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	// Re-parse the body we already read
	var orderResp OrderResponse
	if err := json.Unmarshal(respBody, &orderResp); err != nil {
		return nil, fmt.Errorf("failed to decode order response: %w (body: %s)", err, string(respBody))
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

// GetBalanceAllowance fetches the balance and allowance for an asset type.
// assetType: "COLLATERAL" for USDC, "CONDITIONAL" for position tokens
// tokenID: required for CONDITIONAL, ignored for COLLATERAL
func (c *Client) GetBalanceAllowance(assetType AssetType, tokenID string) (*BalanceAllowanceResponse, error) {
	path := fmt.Sprintf("/balance-allowance?asset_type=%s", assetType)
	if assetType == AssetTypeConditional && tokenID != "" {
		path += fmt.Sprintf("&token_id=%s", tokenID)
	}

	resp, err := c.doRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	defer resp.Body.Close()

	// Read body for debugging
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var balance BalanceAllowanceResponse
	if err := json.Unmarshal(respBody, &balance); err != nil {
		return nil, fmt.Errorf("failed to decode balance: %w (body: %s)", err, string(respBody))
	}

	return &balance, nil
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

	// Browser-like headers to help bypass Cloudflare
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Origin", "https://polymarket.com")
	req.Header.Set("Referer", "https://polymarket.com/")

	// API authentication headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerAPIKey, c.apiKey)
	req.Header.Set(headerSignature, signature)
	req.Header.Set(headerTimestamp, timestamp)
	req.Header.Set(headerPassphrase, c.passphrase)
	req.Header.Set(headerAddress, c.address)

	return c.httpClient.Do(req)
}

// sign generates the HMAC-SHA256 signature for a request.
// Uses URL-safe base64 encoding per Polymarket CLOB spec.
func (c *Client) sign(timestamp, method, path string, body []byte) string {
	var bodyStr string
	if body != nil {
		bodyStr = string(body)
	}

	message := timestamp + method + path + bodyStr

	// Secret is URL-safe base64 encoded, decode it first
	secretBytes, err := base64.URLEncoding.DecodeString(c.secret)
	if err != nil {
		// Try standard base64 as fallback
		secretBytes, err = base64.StdEncoding.DecodeString(c.secret)
		if err != nil {
			// Fallback to raw secret
			secretBytes = []byte(c.secret)
		}
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(message))

	// Return URL-safe base64 encoded signature
	return base64.URLEncoding.EncodeToString(h.Sum(nil))
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
