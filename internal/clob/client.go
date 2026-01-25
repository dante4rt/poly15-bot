package clob

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

	// Proxy rotation support
	proxyURLs    []string
	currentProxy int
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
	if strings.HasPrefix(proxyURL, "socks5://") {
		// SOCKS5 proxy - parse the full URL
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SOCKS5 proxy URL: %w", err)
		}

		var auth *proxy.Auth
		if u.User != nil {
			auth = &proxy.Auth{
				User: u.User.Username(),
			}
			if pass, ok := u.User.Password(); ok {
				auth.Password = pass
			}
		}

		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
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

// NewClientWithProxyRotation creates a CLOB client that rotates through multiple proxies on failure.
func NewClientWithProxyRotation(apiKey, secret, passphrase, address string, proxyURLs []string) (*Client, error) {
	if len(proxyURLs) == 0 {
		return NewClient(apiKey, secret, passphrase, address), nil
	}

	// Create client with first proxy, store all proxies for rotation
	client, err := NewClientWithProxy(apiKey, secret, passphrase, address, proxyURLs[0])
	if err != nil {
		return nil, err
	}

	client.proxyURLs = proxyURLs
	client.currentProxy = 0

	return client, nil
}

// rotateProxy switches to the next proxy in the list
func (c *Client) rotateProxy() error {
	if len(c.proxyURLs) <= 1 {
		return fmt.Errorf("no more proxies to rotate")
	}

	prevProxy := c.currentProxy
	c.currentProxy = (c.currentProxy + 1) % len(c.proxyURLs)
	proxyURL := c.proxyURLs[c.currentProxy]

	log.Printf("[clob] rotating proxy %d -> %d (of %d)", prevProxy+1, c.currentProxy+1, len(c.proxyURLs))

	var transport *http.Transport

	if strings.HasPrefix(proxyURL, "socks5://") {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return fmt.Errorf("failed to parse SOCKS5 proxy URL: %w", err)
		}

		var auth *proxy.Auth
		if u.User != nil {
			auth = &proxy.Auth{User: u.User.Username()}
			if pass, ok := u.User.Password(); ok {
				auth.Password = pass
			}
		}

		dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
		if err != nil {
			return fmt.Errorf("failed to create SOCKS5 dialer: %w", err)
		}

		transport = &http.Transport{Dial: dialer.Dial}
	} else {
		proxyURLParsed, err := url.Parse("http://" + proxyURL)
		if err != nil {
			return fmt.Errorf("failed to parse proxy URL: %w", err)
		}
		transport = &http.Transport{Proxy: http.ProxyURL(proxyURLParsed)}
	}

	c.httpClient = &http.Client{
		Timeout:   defaultTimeout,
		Transport: transport,
	}

	return nil
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
	resp, err := c.doRequest(http.MethodGet, "/data/orders", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}
	defer resp.Body.Close()

	// Read body for debugging
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	// API returns object wrapper: {"orders": [...]}
	var response OpenOrdersResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to decode orders: %w (body: %s)", err, string(respBody))
	}

	return response.Orders, nil
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

// doRequest performs an authenticated HTTP request with automatic proxy rotation on 403.
func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	maxRetries := len(c.proxyURLs)
	if maxRetries == 0 {
		maxRetries = 1 // At least one attempt without proxy rotation
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := c.doRequestOnce(method, path, body)
		if err != nil {
			lastErr = err
			// Network error - try rotating proxy
			if len(c.proxyURLs) > 1 {
				if rotateErr := c.rotateProxy(); rotateErr == nil {
					continue
				}
			}
			return nil, err
		}

		// Check for Cloudflare 403 block
		if resp.StatusCode == http.StatusForbidden && len(c.proxyURLs) > 1 {
			log.Printf("[clob] got 403 (Cloudflare block), rotating proxy...")
			resp.Body.Close()
			if rotateErr := c.rotateProxy(); rotateErr == nil {
				continue // Retry with new proxy
			}
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all proxies failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all proxies returned 403")
}

// doRequestOnce performs a single authenticated HTTP request.
func (c *Client) doRequestOnce(method, path string, body []byte) (*http.Response, error) {
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
