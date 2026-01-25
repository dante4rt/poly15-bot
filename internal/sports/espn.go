package sports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	espnNFLScoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard"
	espnNBAScoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/scoreboard"
)

// ESPNClient fetches live sports data from ESPN's free API.
type ESPNClient struct {
	httpClient *http.Client
}

// NewESPNClient creates a new ESPN API client.
func NewESPNClient() *ESPNClient {
	return &ESPNClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Game represents a live sports game.
type Game struct {
	ID           string
	Name         string
	ShortName    string
	HomeTeam     Team
	AwayTeam     Team
	Status       GameStatus
	Quarter      int    // 1-4 for NFL, 1-4 for NBA
	TimeRemaining string // e.g., "2:30" or "Final"
	StartTime    time.Time
}

// Team represents a team in a game.
type Team struct {
	ID           string
	Name         string
	Abbreviation string
	Score        int
	IsWinner     bool
}

// GameStatus represents the current status of a game.
type GameStatus string

const (
	StatusScheduled  GameStatus = "scheduled"
	StatusInProgress GameStatus = "in_progress"
	StatusFinal      GameStatus = "final"
	StatusDelayed    GameStatus = "delayed"
	StatusPostponed  GameStatus = "postponed"
)

// WinProbability estimates the probability that the leading team wins.
// Based on lead size and time remaining.
func (g *Game) WinProbability() float64 {
	if g.Status == StatusFinal {
		return 1.0 // Game is over, winner is 100% certain
	}

	if g.Status != StatusInProgress {
		return 0.5 // Game hasn't started
	}

	lead := abs(g.HomeTeam.Score - g.AwayTeam.Score)

	// Simple model based on lead and quarter
	// NFL: 7 points = 1 TD, 14 = 2 TDs, 21 = 3 TDs
	switch g.Quarter {
	case 4:
		if lead >= 21 {
			return 0.99 // 3+ TD lead in 4th = virtually certain
		}
		if lead >= 14 {
			return 0.95 // 2+ TD lead in 4th = very likely
		}
		if lead >= 7 {
			return 0.80 // 1 TD lead in 4th
		}
		return 0.60
	case 3:
		if lead >= 21 {
			return 0.95
		}
		if lead >= 14 {
			return 0.85
		}
		return 0.65
	case 2:
		if lead >= 21 {
			return 0.85
		}
		return 0.60
	default:
		return 0.55
	}
}

// Leader returns the team that is currently winning.
func (g *Game) Leader() *Team {
	if g.HomeTeam.Score > g.AwayTeam.Score {
		return &g.HomeTeam
	} else if g.AwayTeam.Score > g.HomeTeam.Score {
		return &g.AwayTeam
	}
	return nil // Tied
}

// IsTied returns true if the game is tied.
func (g *Game) IsTied() bool {
	return g.HomeTeam.Score == g.AwayTeam.Score
}

// PointDifferential returns the absolute point difference.
func (g *Game) PointDifferential() int {
	return abs(g.HomeTeam.Score - g.AwayTeam.Score)
}

// Winner returns the winning team if the game is final.
func (g *Game) Winner() *Team {
	if g.Status != StatusFinal {
		return nil
	}
	return g.Leader()
}

// GetNFLGames fetches current NFL games from ESPN.
func (c *ESPNClient) GetNFLGames() ([]Game, error) {
	return c.getGames(espnNFLScoreboardURL)
}

// GetNBAGames fetches current NBA games from ESPN.
func (c *ESPNClient) GetNBAGames() ([]Game, error) {
	return c.getGames(espnNBAScoreboardURL)
}

func (c *ESPNClient) getGames(url string) ([]Game, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ESPN data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ESPN API returned status %d", resp.StatusCode)
	}

	var data espnResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse ESPN response: %w", err)
	}

	games := make([]Game, 0, len(data.Events))
	for _, event := range data.Events {
		game, err := parseEvent(event)
		if err != nil {
			continue
		}
		games = append(games, game)
	}

	return games, nil
}

func parseEvent(event espnEvent) (Game, error) {
	if len(event.Competitions) == 0 {
		return Game{}, fmt.Errorf("no competitions in event")
	}

	comp := event.Competitions[0]
	if len(comp.Competitors) < 2 {
		return Game{}, fmt.Errorf("not enough competitors")
	}

	game := Game{
		ID:        event.ID,
		Name:      event.Name,
		ShortName: event.ShortName,
		Status:    parseStatus(event.Status.Type.Name),
		Quarter:   event.Status.Period,
		TimeRemaining: event.Status.DisplayClock,
	}

	// Parse start time
	if t, err := time.Parse(time.RFC3339, event.Date); err == nil {
		game.StartTime = t
	}

	// Parse teams
	for _, c := range comp.Competitors {
		team := Team{
			ID:           c.Team.ID,
			Name:         c.Team.DisplayName,
			Abbreviation: c.Team.Abbreviation,
			Score:        parseInt(c.Score),
			IsWinner:     c.Winner,
		}

		if c.HomeAway == "home" {
			game.HomeTeam = team
		} else {
			game.AwayTeam = team
		}
	}

	return game, nil
}

func parseStatus(status string) GameStatus {
	status = strings.ToLower(status)
	switch {
	case strings.Contains(status, "final"):
		return StatusFinal
	case strings.Contains(status, "progress"), strings.Contains(status, "halftime"):
		return StatusInProgress
	case strings.Contains(status, "scheduled"), strings.Contains(status, "pre"):
		return StatusScheduled
	case strings.Contains(status, "delayed"):
		return StatusDelayed
	case strings.Contains(status, "postponed"):
		return StatusPostponed
	default:
		return StatusScheduled
	}
}

func parseInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ESPN API response types
type espnResponse struct {
	Events []espnEvent `json:"events"`
}

type espnEvent struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	ShortName    string           `json:"shortName"`
	Date         string           `json:"date"`
	Status       espnStatus       `json:"status"`
	Competitions []espnCompetition `json:"competitions"`
}

type espnStatus struct {
	Type         espnStatusType `json:"type"`
	Period       int            `json:"period"`
	DisplayClock string         `json:"displayClock"`
}

type espnStatusType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type espnCompetition struct {
	Competitors []espnCompetitor `json:"competitors"`
}

type espnCompetitor struct {
	Team     espnTeam `json:"team"`
	Score    string   `json:"score"`
	HomeAway string   `json:"homeAway"`
	Winner   bool     `json:"winner"`
}

type espnTeam struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Abbreviation string `json:"abbreviation"`
}
