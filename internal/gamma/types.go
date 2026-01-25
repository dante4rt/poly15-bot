package gamma

import (
	"strconv"
	"strings"
	"time"
)

// Market represents a prediction market from the Gamma API.
type Market struct {
	ConditionID  string  `json:"condition_id"`
	QuestionID   string  `json:"question_id"`
	Question     string  `json:"question"`
	Slug         string  `json:"slug"`
	EndDateISO   string  `json:"end_date_iso"`
	GameStartTime string `json:"game_start_time"`
	Active       bool    `json:"active"`
	Closed       bool    `json:"closed"`
	Tokens       []Token `json:"tokens"`
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

// EndTime parses the end time from EndDateISO or extracts from slug for 15M markets.
func (m *Market) EndTime() (time.Time, error) {
	// Try EndDateISO first
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
	// For 15M markets, extract from slug
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

// GetYesToken returns the "Yes" outcome token if present.
func (m *Market) GetYesToken() *Token {
	for i := range m.Tokens {
		if m.Tokens[i].Outcome == "Yes" {
			return &m.Tokens[i]
		}
	}
	return nil
}

// GetNoToken returns the "No" outcome token if present.
func (m *Market) GetNoToken() *Token {
	for i := range m.Tokens {
		if m.Tokens[i].Outcome == "No" {
			return &m.Tokens[i]
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
