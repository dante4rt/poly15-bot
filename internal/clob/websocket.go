package clob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"

	// Reconnection settings
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	backoffFactor  = 2

	// Keepalive settings
	pingInterval = 30 * time.Second
	pongTimeout  = 10 * time.Second
	writeTimeout = 10 * time.Second
)

// MarketUpdate represents a real-time market data update.
type MarketUpdate struct {
	TokenID string
	BestBid float64
	BestAsk float64
	BidSize float64
	AskSize float64
}

// WSClient is a WebSocket client for real-time market data.
type WSClient struct {
	conn       *websocket.Conn
	url        string
	subscribed map[string]bool
	handlers   []func(update MarketUpdate)
	done       chan struct{}
	mu         sync.RWMutex
	connMu     sync.Mutex
}

// wsMessage represents an outbound WebSocket message.
type wsMessage struct {
	Type    string   `json:"type"`
	Channel string   `json:"channel,omitempty"`
	Markets []string `json:"markets,omitempty"`
}

// wsEvent represents an inbound WebSocket event.
type wsEvent struct {
	EventType string     `json:"event_type"`
	Market    string     `json:"market"`
	Price     string     `json:"price,omitempty"`
	Side      string     `json:"side,omitempty"`
	Bids      [][]string `json:"bids,omitempty"`
	Asks      [][]string `json:"asks,omitempty"`
}

// NewWSClient creates a new WebSocket client.
func NewWSClient() *WSClient {
	return &WSClient{
		url:        wsURL,
		subscribed: make(map[string]bool),
		handlers:   make([]func(update MarketUpdate), 0),
		done:       make(chan struct{}),
	}
}

// Connect establishes a WebSocket connection.
func (c *WSClient) Connect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn != nil {
		return nil
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	c.conn = conn
	return nil
}

// Subscribe subscribes to market updates for the given token IDs.
func (c *WSClient) Subscribe(tokenIDs ...string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return errors.New("not connected")
	}

	msg := wsMessage{
		Type:    "subscribe",
		Channel: "market",
		Markets: tokenIDs,
	}

	if err := c.writeJSON(msg); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	c.mu.Lock()
	for _, id := range tokenIDs {
		c.subscribed[id] = true
	}
	c.mu.Unlock()

	return nil
}

// Unsubscribe unsubscribes from market updates for the given token IDs.
func (c *WSClient) Unsubscribe(tokenIDs ...string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return errors.New("not connected")
	}

	msg := wsMessage{
		Type:    "unsubscribe",
		Channel: "market",
		Markets: tokenIDs,
	}

	if err := c.writeJSON(msg); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}

	c.mu.Lock()
	for _, id := range tokenIDs {
		delete(c.subscribed, id)
	}
	c.mu.Unlock()

	return nil
}

// OnUpdate registers a callback handler for market updates.
func (c *WSClient) OnUpdate(handler func(MarketUpdate)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, handler)
}

// Run starts the main WebSocket loop with automatic reconnection.
// Note: WebSocket is optional - REST polling is used as primary price source.
func (c *WSClient) Run(ctx context.Context) error {
	backoff := initialBackoff
	failureCount := 0
	loggedDisabled := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		default:
		}

		if err := c.Connect(); err != nil {
			failureCount++
			if failureCount == 1 {
				log.Printf("[ws] connection failed (using REST polling): %v", err)
			}
			if !c.sleep(ctx, backoff) {
				return ctx.Err()
			}
			backoff = c.nextBackoff(backoff)
			continue
		}

		// Resubscribe to previously subscribed markets
		if err := c.resubscribe(); err != nil {
			c.closeConnection()
			failureCount++
			continue
		}

		// Run the read loop
		err := c.readLoop(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			failureCount++
			// Only log after first successful connection that then fails
			if failureCount == 1 {
				log.Printf("[ws] disconnected (using REST polling): %v", err)
			} else if !loggedDisabled && failureCount >= 3 {
				log.Printf("[ws] unstable, disabled (REST polling only)")
				loggedDisabled = true
			}
		}

		c.closeConnection()

		if !c.sleep(ctx, backoff) {
			return ctx.Err()
		}
		backoff = c.nextBackoff(backoff)
	}
}

// Close gracefully closes the WebSocket connection.
func (c *WSClient) Close() error {
	close(c.done)
	return c.closeConnection()
}

// readLoop reads messages from the WebSocket connection.
func (c *WSClient) readLoop(ctx context.Context) error {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return errors.New("not connected")
	}

	// Set up pong handler
	conn.SetPongHandler(func(appData string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout + pingInterval))
	})

	// Start ping routine
	pingDone := make(chan struct{})
	go c.pingLoop(ctx, pingDone)
	defer close(pingDone)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		default:
		}

		// Set read deadline
		if err := conn.SetReadDeadline(time.Now().Add(pongTimeout + pingInterval)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("failed to read message: %w", err)
		}

		c.handleMessage(message)
	}
}

// pingLoop sends periodic ping messages to keep the connection alive.
func (c *WSClient) pingLoop(ctx context.Context, done <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-c.done:
			return
		case <-ticker.C:
			c.connMu.Lock()
			conn := c.conn
			c.connMu.Unlock()

			if conn == nil {
				return
			}

			if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				log.Printf("Failed to set write deadline for ping: %v", err)
				return
			}

			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// handleMessage processes an incoming WebSocket message.
func (c *WSClient) handleMessage(data []byte) {
	var event wsEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to unmarshal WebSocket message: %v", err)
		return
	}

	var update MarketUpdate
	update.TokenID = event.Market

	switch event.EventType {
	case "price_change":
		update = c.handlePriceChange(event)
	case "book":
		update = c.handleBookUpdate(event)
	default:
		// Ignore unknown event types
		return
	}

	if update.TokenID == "" {
		return
	}

	c.notifyHandlers(update)
}

// handlePriceChange processes a price change event.
func (c *WSClient) handlePriceChange(event wsEvent) MarketUpdate {
	update := MarketUpdate{
		TokenID: event.Market,
	}

	price, err := strconv.ParseFloat(event.Price, 64)
	if err != nil {
		log.Printf("Failed to parse price %s: %v", event.Price, err)
		return update
	}

	switch event.Side {
	case "buy":
		update.BestBid = price
	case "sell":
		update.BestAsk = price
	}

	return update
}

// handleBookUpdate processes a book update event.
func (c *WSClient) handleBookUpdate(event wsEvent) MarketUpdate {
	update := MarketUpdate{
		TokenID: event.Market,
	}

	// Parse best bid (first entry in bids array)
	if len(event.Bids) > 0 && len(event.Bids[0]) >= 2 {
		price, err := strconv.ParseFloat(event.Bids[0][0], 64)
		if err == nil {
			update.BestBid = price
		}
		size, err := strconv.ParseFloat(event.Bids[0][1], 64)
		if err == nil {
			update.BidSize = size
		}
	}

	// Parse best ask (first entry in asks array)
	if len(event.Asks) > 0 && len(event.Asks[0]) >= 2 {
		price, err := strconv.ParseFloat(event.Asks[0][0], 64)
		if err == nil {
			update.BestAsk = price
		}
		size, err := strconv.ParseFloat(event.Asks[0][1], 64)
		if err == nil {
			update.AskSize = size
		}
	}

	return update
}

// notifyHandlers calls all registered handlers with the update.
func (c *WSClient) notifyHandlers(update MarketUpdate) {
	c.mu.RLock()
	handlers := make([]func(MarketUpdate), len(c.handlers))
	copy(handlers, c.handlers)
	c.mu.RUnlock()

	for _, handler := range handlers {
		handler(update)
	}
}

// resubscribe resubscribes to all previously subscribed markets.
func (c *WSClient) resubscribe() error {
	c.mu.RLock()
	tokenIDs := make([]string, 0, len(c.subscribed))
	for id := range c.subscribed {
		tokenIDs = append(tokenIDs, id)
	}
	c.mu.RUnlock()

	if len(tokenIDs) == 0 {
		return nil
	}

	msg := wsMessage{
		Type:    "subscribe",
		Channel: "market",
		Markets: tokenIDs,
	}

	return c.writeJSON(msg)
}

// writeJSON writes a JSON message to the WebSocket connection.
func (c *WSClient) writeJSON(v interface{}) error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return errors.New("not connected")
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	return c.conn.WriteJSON(v)
}

// closeConnection closes the current WebSocket connection.
func (c *WSClient) closeConnection() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	if c.conn == nil {
		return nil
	}

	// Send close message
	err := c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)

	c.conn.Close()
	c.conn = nil

	return err
}

// nextBackoff calculates the next backoff duration.
func (c *WSClient) nextBackoff(current time.Duration) time.Duration {
	next := current * backoffFactor
	if next > maxBackoff {
		return maxBackoff
	}
	return next
}

// sleep waits for the specified duration or until context is cancelled.
func (c *WSClient) sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-c.done:
		return false
	case <-timer.C:
		return true
	}
}

// GetSubscribedMarkets returns a copy of the currently subscribed market IDs.
func (c *WSClient) GetSubscribedMarkets() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	markets := make([]string, 0, len(c.subscribed))
	for id := range c.subscribed {
		markets = append(markets, id)
	}
	return markets
}

// IsConnected returns whether the client is currently connected.
func (c *WSClient) IsConnected() bool {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.conn != nil
}
