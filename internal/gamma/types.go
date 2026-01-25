package gamma

import "time"

// Market represents a prediction market from the Gamma API.
type Market struct {
	ConditionID string  `json:"condition_id"`
	QuestionID  string  `json:"question_id"`
	Question    string  `json:"question"`
	EndDateISO  string  `json:"end_date_iso"`
	Active      bool    `json:"active"`
	Closed      bool    `json:"closed"`
	Tokens      []Token `json:"tokens"`
}

// Token represents a tradeable outcome token within a market.
type Token struct {
	TokenID string  `json:"token_id"`
	Outcome string  `json:"outcome"`
	Price   float64 `json:"price,string"`
}

// EndTime parses the EndDateISO field into a time.Time value.
func (m *Market) EndTime() (time.Time, error) {
	return time.Parse(time.RFC3339, m.EndDateISO)
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
