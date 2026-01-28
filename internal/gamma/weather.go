package gamma

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// WeatherTagID is the Gamma API tag ID for weather markets.
const WeatherTagID = "84"

// WeatherMarketType represents the type of weather market.
type WeatherMarketType string

const (
	WeatherTypeTempAbove    WeatherMarketType = "temp_above"
	WeatherTypeTempBelow    WeatherMarketType = "temp_below"
	WeatherTypeTempRange    WeatherMarketType = "temp_range"
	WeatherTypeSnow         WeatherMarketType = "snow"
	WeatherTypeRain         WeatherMarketType = "rain"
	WeatherTypePrecipitation WeatherMarketType = "precipitation"
	WeatherTypeGlobalTemp   WeatherMarketType = "global_temp"
	WeatherTypeUnknown      WeatherMarketType = "unknown"
)

// WeatherMarket represents a parsed weather market with extracted details.
type WeatherMarket struct {
	Market         Market
	MarketType     WeatherMarketType
	Location       string  // City name extracted from question
	Threshold      float64 // Temperature threshold in Fahrenheit (for temp markets)
	ThresholdUnits string  // "F" or "C"
	ResolutionDate time.Time
	YesTokenID     string
	NoTokenID      string
	YesPrice       float64
	NoPrice        float64
}

// WeatherEvent represents a weather-related event with its markets.
type WeatherEvent struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Slug    string   `json:"slug"`
	Active  bool     `json:"active"`
	Closed  bool     `json:"closed"`
	Markets []Market `json:"markets"`
}

// GetWeatherMarkets retrieves active weather-related markets using tag_id filtering.
// This uses the events endpoint with tag_id=84 (weather) which is the only
// working filter method in the Gamma API.
func (c *Client) GetWeatherMarkets() ([]Market, error) {
	now := time.Now()

	// Fetch weather events using tag_id=84
	events, err := c.GetWeatherEvents()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch weather events: %w", err)
	}

	// Extract markets from events
	marketMap := make(map[string]Market)
	for _, event := range events {
		for _, market := range event.Markets {
			// Skip inactive or closed markets
			if !market.Active || market.Closed {
				continue
			}

			// Check end time is in the future
			endTime, err := market.EndTime()
			if err != nil || !endTime.After(now) {
				continue
			}

			// Filter for weather markets we can trade
			if isWeatherMarket(market) {
				marketMap[market.Slug] = market
			}
		}
	}

	result := make([]Market, 0, len(marketMap))
	for _, market := range marketMap {
		result = append(result, market)
	}

	return result, nil
}

// WeatherEventsPaginationResponse represents the paginated events response.
type WeatherEventsPaginationResponse struct {
	Data   []WeatherEvent `json:"data"`
	Offset int            `json:"offset"`
}

// GetWeatherEvents fetches weather events from the Gamma API using the pagination endpoint.
// This endpoint supports tag_slug=weather which returns all weather markets including
// daily temperature markets for specific cities.
func (c *Client) GetWeatherEvents() ([]WeatherEvent, error) {
	var allEvents []WeatherEvent
	offset := 0
	limit := 50

	for {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(limit))
		params.Set("active", "true")
		params.Set("archived", "false")
		params.Set("tag_slug", "weather")
		params.Set("closed", "false")
		params.Set("order", "startDate")
		params.Set("ascending", "false")
		params.Set("offset", strconv.Itoa(offset))

		endpoint := fmt.Sprintf("%s/events/pagination?%s", c.baseURL, params.Encode())

		resp, err := c.doGet(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch weather events: %w", err)
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		var paginatedResp WeatherEventsPaginationResponse
		if err := json.NewDecoder(resp.Body).Decode(&paginatedResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode weather events: %w", err)
		}
		resp.Body.Close()

		if len(paginatedResp.Data) == 0 {
			break
		}

		allEvents = append(allEvents, paginatedResp.Data...)

		// If we got fewer than limit, we've reached the end
		if len(paginatedResp.Data) < limit {
			break
		}

		offset += limit

		// Safety limit to avoid infinite loops
		if offset > 500 {
			break
		}
	}

	return allEvents, nil
}

// ParseWeatherMarket extracts weather market details from a generic market.
func ParseWeatherMarket(market Market) *WeatherMarket {
	if !isWeatherMarket(market) {
		return nil
	}

	wm := &WeatherMarket{
		Market:     market,
		MarketType: classifyWeatherMarket(market),
		Location:   extractLocation(market.Question),
	}

	// Extract threshold from question
	wm.Threshold, wm.ThresholdUnits = extractThreshold(market.Question)

	// Parse resolution date
	endTime, err := market.EndTime()
	if err == nil {
		wm.ResolutionDate = endTime
	}

	// Get token info
	yesToken := market.GetYesToken()
	noToken := market.GetNoToken()
	if yesToken != nil {
		wm.YesTokenID = yesToken.TokenID
		wm.YesPrice = yesToken.Price
	}
	if noToken != nil {
		wm.NoTokenID = noToken.TokenID
		wm.NoPrice = noToken.Price
	}

	return wm
}

// isWeatherMarket checks if a market is weather-related and tradeable.
// Since markets are already filtered by tag_id=84 (weather), this function
// focuses on excluding non-tradeable markets rather than strict keyword matching.
func isWeatherMarket(market Market) bool {
	if !market.Active || market.Closed {
		return false
	}

	// Skip 15-min crypto markets
	if market.Is15MinMarket() {
		return false
	}

	question := strings.ToLower(market.Question)

	// Exclude long-term climate/yearly markets that aren't suitable for quick trading
	excludePatterns := []string{
		"hottest year on record",
		"arctic sea ice",
		"earthquake",
		"volcano",
		"hurricane",
		"meteor",
		"tsunami",
	}
	for _, pattern := range excludePatterns {
		if strings.Contains(question, pattern) {
			return false
		}
	}

	// Must have a recognizable city name - we need a location for forecasts
	if !hasCityName(question) {
		return false
	}

	// Accept markets with temperature, precipitation, or weather indicators
	weatherIndicators := []string{
		"temperature",
		"highest temperature",
		"lowest temperature",
		"degrees",
		"ºc",
		"ºf",
		"°c",
		"°f",
		"snow",
		"rain",
		"precipitation",
		"inches",
	}

	for _, indicator := range weatherIndicators {
		if strings.Contains(question, indicator) {
			return true
		}
	}

	return false
}

// hasCityName checks if the question contains a known city name.
func hasCityName(question string) bool {
	cities := []string{
		// US Cities
		"nyc", "new york", "chicago", "miami", "denver", "seattle",
		"los angeles", "boston", "dallas", "houston", "phoenix",
		"philadelphia", "san francisco", "atlanta", "washington",
		"las vegas", "san diego", "minneapolis", "detroit",
		// International Cities
		"toronto", "seoul", "tokyo", "london", "paris", "berlin",
		"sydney", "melbourne", "auckland", "wellington",
		"buenos aires", "sao paulo", "mexico city",
		"ankara", "istanbul", "moscow", "beijing", "shanghai",
		"hong kong", "singapore", "mumbai", "delhi", "dubai",
		"cairo", "cape town", "johannesburg",
	}
	for _, city := range cities {
		if strings.Contains(question, city) {
			return true
		}
	}
	return false
}

// classifyWeatherMarket determines the type of weather market.
func classifyWeatherMarket(market Market) WeatherMarketType {
	question := strings.ToLower(market.Question)

	// Snow markets
	if strings.Contains(question, "snow") {
		return WeatherTypeSnow
	}

	// Precipitation markets (inches of rain, etc.)
	if strings.Contains(question, "precipitation") || (strings.Contains(question, "inches") && !strings.Contains(question, "snow")) {
		return WeatherTypePrecipitation
	}

	// Rain markets
	if strings.Contains(question, "rain") {
		return WeatherTypeRain
	}

	// Global temperature increase markets (ºC anomaly)
	if strings.Contains(question, "global temperature") || strings.Contains(question, "temperature increase") {
		return WeatherTypeGlobalTemp
	}

	// Daily high/low temperature range markets (e.g., "highest temperature in NYC be between 20-21°F")
	if strings.Contains(question, "highest temperature") || strings.Contains(question, "lowest temperature") {
		// Check for specific range (bucket markets like "8°C")
		if strings.Contains(question, "between") {
			return WeatherTypeTempRange
		}
		// Check for "below" threshold markets
		if strings.Contains(question, "or below") || strings.Contains(question, "or lower") ||
			strings.Contains(question, "below") || strings.Contains(question, "under") ||
			strings.Contains(question, "lower than") {
			return WeatherTypeTempBelow
		}
		// Check for "above" threshold markets
		if strings.Contains(question, "or higher") || strings.Contains(question, "or above") ||
			strings.Contains(question, "above") || strings.Contains(question, "exceed") ||
			strings.Contains(question, "higher than") || strings.Contains(question, "at least") {
			return WeatherTypeTempAbove
		}
		// If just a temperature value with no direction indicator, it's a bucket/range market
		// e.g., "Will the highest temperature in London be 8°C on January 28?"
		return WeatherTypeTempRange
	}

	// Temperature range markets (between X and Y)
	if strings.Contains(question, "between") && (strings.Contains(question, "ºc") ||
		strings.Contains(question, "°c") || strings.Contains(question, "ºf") ||
		strings.Contains(question, "°f") || strings.Contains(question, "degrees")) {
		return WeatherTypeTempRange
	}

	// Temperature threshold markets
	if strings.Contains(question, "temperature") || strings.Contains(question, "degrees") ||
		strings.Contains(question, "ºc") || strings.Contains(question, "ºf") ||
		strings.Contains(question, "°c") || strings.Contains(question, "°f") {
		if strings.Contains(question, "above") || strings.Contains(question, "exceed") ||
			strings.Contains(question, "higher than") || strings.Contains(question, "at least") ||
			strings.Contains(question, "more than") {
			return WeatherTypeTempAbove
		}
		if strings.Contains(question, "below") || strings.Contains(question, "under") ||
			strings.Contains(question, "lower than") || strings.Contains(question, "drop to") ||
			strings.Contains(question, "less than") {
			return WeatherTypeTempBelow
		}
	}

	return WeatherTypeUnknown
}

// extractLocation extracts city name from market question.
func extractLocation(question string) string {
	question = strings.ToLower(question)

	// Check for cities (US and international)
	cities := []struct {
		name    string
		aliases []string
	}{
		// US Cities
		{"New York", []string{"nyc", "new york city", "new york", "manhattan"}},
		{"Los Angeles", []string{"los angeles", "la", "l.a."}},
		{"Chicago", []string{"chicago"}},
		{"Miami", []string{"miami"}},
		{"Denver", []string{"denver"}},
		{"Seattle", []string{"seattle"}},
		{"Boston", []string{"boston"}},
		{"Dallas", []string{"dallas"}},
		{"Houston", []string{"houston"}},
		{"Phoenix", []string{"phoenix"}},
		{"Philadelphia", []string{"philadelphia", "philly"}},
		{"San Francisco", []string{"san francisco", "sf"}},
		{"Atlanta", []string{"atlanta"}},
		{"Washington", []string{"washington dc", "washington d.c.", "washington, d.c.", "dc", "d.c.", "washington"}},
		{"Las Vegas", []string{"las vegas"}},
		{"San Diego", []string{"san diego"}},
		{"Minneapolis", []string{"minneapolis"}},
		{"Detroit", []string{"detroit"}},
		// International Cities
		{"Toronto", []string{"toronto"}},
		{"Seoul", []string{"seoul"}},
		{"Tokyo", []string{"tokyo"}},
		{"London", []string{"london"}},
		{"Paris", []string{"paris"}},
		{"Berlin", []string{"berlin"}},
		{"Sydney", []string{"sydney"}},
		{"Melbourne", []string{"melbourne"}},
		{"Auckland", []string{"auckland"}},
		{"Wellington", []string{"wellington"}},
		{"Buenos Aires", []string{"buenos aires"}},
		{"Sao Paulo", []string{"são paulo", "sao paulo"}},
		{"Mexico City", []string{"mexico city"}},
		{"Ankara", []string{"ankara"}},
		{"Istanbul", []string{"istanbul"}},
		{"Moscow", []string{"moscow"}},
		{"Beijing", []string{"beijing"}},
		{"Shanghai", []string{"shanghai"}},
		{"Hong Kong", []string{"hong kong"}},
		{"Singapore", []string{"singapore"}},
		{"Mumbai", []string{"mumbai"}},
		{"Delhi", []string{"delhi"}},
		{"Dubai", []string{"dubai"}},
		{"Cairo", []string{"cairo"}},
		{"Cape Town", []string{"cape town"}},
		{"Johannesburg", []string{"johannesburg"}},
	}

	for _, city := range cities {
		for _, alias := range city.aliases {
			if strings.Contains(question, alias) {
				return city.name
			}
		}
	}

	return "Unknown"
}

// extractThreshold extracts temperature threshold from market question.
// Returns threshold value and units ("F" or "C").
func extractThreshold(question string) (float64, string) {
	// Patterns to match temperature thresholds
	patterns := []struct {
		regex *regexp.Regexp
		unit  string
	}{
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*°?\s*[fF]`), "F"},
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*degrees?\s*[fF]`), "F"},
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*°?\s*[cC]`), "C"},
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*degrees?\s*[cC]`), "C"},
		{regexp.MustCompile(`above\s*(\d+(?:\.\d+)?)`), "F"}, // Assume F for US markets
		{regexp.MustCompile(`below\s*(\d+(?:\.\d+)?)`), "F"},
		{regexp.MustCompile(`(\d+(?:\.\d+)?)\s*degrees`), "F"},
	}

	for _, p := range patterns {
		matches := p.regex.FindStringSubmatch(question)
		if len(matches) > 1 {
			val, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				return val, p.unit
			}
		}
	}

	return 0, ""
}

// GetThresholdCelsius returns the threshold in Celsius.
func (wm *WeatherMarket) GetThresholdCelsius() float64 {
	if wm.ThresholdUnits == "F" {
		return (wm.Threshold - 32) * 5 / 9
	}
	return wm.Threshold
}

// GetThresholdFahrenheit returns the threshold in Fahrenheit.
func (wm *WeatherMarket) GetThresholdFahrenheit() float64 {
	if wm.ThresholdUnits == "C" {
		return wm.Threshold*9/5 + 32
	}
	return wm.Threshold
}

// DaysUntilResolution returns the number of days until market resolves.
func (wm *WeatherMarket) DaysUntilResolution() float64 {
	return time.Until(wm.ResolutionDate).Hours() / 24
}

// HasGoodLiquidity checks if the market has enough volume for trading.
func (wm *WeatherMarket) HasGoodLiquidity(minVolume float64) bool {
	return wm.Market.GetVolume24hr() >= minVolume
}

// GetRangeBoundsCelsius returns the temperature range bounds for bucket markets.
// For "8°C" bucket → (7.5, 8.5)
// For "6°C or below" → (-100, 6.5)
// For "12°C or higher" → (11.5, 100)
func (wm *WeatherMarket) GetRangeBoundsCelsius() (low, high float64) {
	threshold := wm.GetThresholdCelsius()

	switch wm.MarketType {
	case WeatherTypeTempBelow:
		// "X or below" means temp ≤ X
		return -100, threshold + 0.5
	case WeatherTypeTempAbove:
		// "X or higher" means temp ≥ X
		return threshold - 0.5, 100
	case WeatherTypeTempRange:
		// Bucket market: "8°C" means 7.5 ≤ temp < 8.5
		return threshold - 0.5, threshold + 0.5
	default:
		return threshold - 0.5, threshold + 0.5
	}
}

// IsBucketMarket returns true if this is a specific temperature bucket (e.g., "8°C")
// rather than a threshold market (e.g., "above 32°F").
func (wm *WeatherMarket) IsBucketMarket() bool {
	return wm.MarketType == WeatherTypeTempRange
}
