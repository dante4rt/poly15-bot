package gamma

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Market represents a prediction market from the Gamma API.
type Market struct {
	ConditionID   string  `json:"condition_id"`
	ConditionId   string  `json:"conditionId"` // Also check camelCase
	QuestionID    string  `json:"question_id"`
	Question      string  `json:"question"`
	Slug          string  `json:"slug"`
	EndDateISO    string  `json:"end_date_iso"`
	EndDate       string  `json:"endDate"`
	GameStartTime string  `json:"game_start_time"`
	Active        bool    `json:"active"`
	Closed        bool    `json:"closed"`
	Tokens        []Token `json:"tokens"`
	// 15M markets use JSON-encoded strings
	ClobTokenIDs  string `json:"clobTokenIds"`
	Outcomes      string `json:"outcomes"`
	OutcomePrices string `json:"outcomePrices"`
	// Gamma's indicative prices (more accurate than CLOB order book)
	BestBid float64 `json:"bestBid"`
	BestAsk float64 `json:"bestAsk"`
}

// GetConditionID returns the condition ID (handles both field names)
func (m *Market) GetConditionID() string {
	if m.ConditionID != "" {
		return m.ConditionID
	}
	return m.ConditionId
}

// ParseClobTokenIDs parses the JSON-encoded clobTokenIds string
func (m *Market) ParseClobTokenIDs() []string {
	var ids []string
	json.Unmarshal([]byte(m.ClobTokenIDs), &ids)
	return ids
}

// ParseOutcomes parses the JSON-encoded outcomes string
func (m *Market) ParseOutcomes() []string {
	var outcomes []string
	json.Unmarshal([]byte(m.Outcomes), &outcomes)
	return outcomes
}

// ParseOutcomePrices parses the JSON-encoded outcomePrices string
func (m *Market) ParseOutcomePrices() []float64 {
	var priceStrs []string
	json.Unmarshal([]byte(m.OutcomePrices), &priceStrs)
	prices := make([]float64, len(priceStrs))
	for i, s := range priceStrs {
		prices[i], _ = strconv.ParseFloat(s, 64)
	}
	return prices
}

// Is15MinMarket returns true if this is a 15-minute up/down market.
func (m *Market) Is15MinMarket() bool {
	return strings.Contains(m.Slug, "-updown-15m-")
}

// ExtractEndTimeFromSlug extracts unix timestamp from slug like "btc-updown-15m-1737801900"
func (m *Market) ExtractEndTimeFromSlug() (time.Time, error) {
	parts := strings.Split(m.Slug, "-")
	if len(parts) < 4 {
		return time.Time{}, nil
	}
	// Last part should be unix timestamp
	ts, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

// Token represents a tradeable outcome token within a market.
type Token struct {
	TokenID string  `json:"token_id"`
	Outcome string  `json:"outcome"`
	Price   float64 `json:"price,string"`
}

// EndTime parses the end time from various fields or extracts from slug for 15M markets.
func (m *Market) EndTime() (time.Time, error) {
	// Try EndDate (used by 15M markets)
	if m.EndDate != "" {
		t, err := time.Parse(time.RFC3339, m.EndDate)
		if err == nil {
			return t, nil
		}
	}
	// Try EndDateISO
	if m.EndDateISO != "" {
		t, err := time.Parse(time.RFC3339, m.EndDateISO)
		if err == nil {
			return t, nil
		}
	}
	// Try GameStartTime
	if m.GameStartTime != "" {
		t, err := time.Parse(time.RFC3339, m.GameStartTime)
		if err == nil {
			return t, nil
		}
	}
	// For 15M markets, extract from slug as fallback
	if m.Is15MinMarket() {
		return m.ExtractEndTimeFromSlug()
	}
	return time.Time{}, nil
}

// IsExpiringSoon returns true if the market ends within the given duration.
func (m *Market) IsExpiringSoon(within time.Duration) bool {
	endTime, err := m.EndTime()
	if err != nil {
		return false
	}
	return time.Until(endTime) <= within && time.Until(endTime) > 0
}

// GetYesToken returns the "Yes" or "Up" outcome token if present.
func (m *Market) GetYesToken() *Token {
	// Check standard tokens array first
	for i := range m.Tokens {
		o := strings.ToLower(m.Tokens[i].Outcome)
		if o == "yes" || o == "up" {
			return &m.Tokens[i]
		}
	}
	// Check 15M market format (JSON-encoded strings)
	tokenIDs := m.ParseClobTokenIDs()
	outcomes := m.ParseOutcomes()
	prices := m.ParseOutcomePrices()
	if len(tokenIDs) >= 2 && len(outcomes) >= 2 {
		for i, outcome := range outcomes {
			o := strings.ToLower(outcome)
			if o == "yes" || o == "up" {
				price := 0.0
				if i < len(prices) {
					price = prices[i]
				}
				return &Token{
					TokenID: tokenIDs[i],
					Outcome: outcome,
					Price:   price,
				}
			}
		}
	}
	return nil
}

// GetNoToken returns the "No" or "Down" outcome token if present.
func (m *Market) GetNoToken() *Token {
	// Check standard tokens array first
	for i := range m.Tokens {
		o := strings.ToLower(m.Tokens[i].Outcome)
		if o == "no" || o == "down" {
			return &m.Tokens[i]
		}
	}
	// Check 15M market format (JSON-encoded strings)
	tokenIDs := m.ParseClobTokenIDs()
	outcomes := m.ParseOutcomes()
	prices := m.ParseOutcomePrices()
	if len(tokenIDs) >= 2 && len(outcomes) >= 2 {
		for i, outcome := range outcomes {
			o := strings.ToLower(outcome)
			if o == "no" || o == "down" {
				price := 0.0
				if i < len(prices) {
					price = prices[i]
				}
				return &Token{
					TokenID: tokenIDs[i],
					Outcome: outcome,
					Price:   price,
				}
			}
		}
	}
	return nil
}

// SearchParams holds query parameters for market search.
type SearchParams struct {
	Query  string
	Active bool
	Closed bool
	Limit  int
	Offset int
}
