package weather

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	openMeteoBaseURL = "https://api.open-meteo.com/v1"
	defaultTimeout   = 30 * time.Second
)

// Client fetches weather data from Open-Meteo (free, no auth required).
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new weather API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: defaultTimeout},
		baseURL:    openMeteoBaseURL,
	}
}

// Forecast represents weather forecast data for a location.
type Forecast struct {
	Location    string
	Latitude    float64
	Longitude   float64
	Date        time.Time
	TempHigh    float64 // Celsius
	TempLow     float64 // Celsius
	TempMean    float64 // Celsius (average)
	RainProb    float64 // 0-100 (percentage)
	SnowProb    float64 // 0-100 (percentage)
	Snowfall    float64 // cm
	WindSpeed   float64 // km/h max
	Humidity    int     // 0-100 (percentage)
	CloudCover  int     // 0-100 (percentage)
	UVIndex     float64
}

// CelsiusToFahrenheit converts Celsius to Fahrenheit.
func CelsiusToFahrenheit(c float64) float64 {
	return c*9/5 + 32
}

// FahrenheitToCelsius converts Fahrenheit to Celsius.
func FahrenheitToCelsius(f float64) float64 {
	return (f - 32) * 5 / 9
}

// TempHighF returns high temperature in Fahrenheit.
func (f *Forecast) TempHighF() float64 {
	return CelsiusToFahrenheit(f.TempHigh)
}

// TempLowF returns low temperature in Fahrenheit.
func (f *Forecast) TempLowF() float64 {
	return CelsiusToFahrenheit(f.TempLow)
}

// GetForecast fetches weather forecast for a location and date.
func (c *Client) GetForecast(loc *Location, date time.Time) (*Forecast, error) {
	// Open-Meteo forecast endpoint
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", loc.Latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", loc.Longitude))
	params.Set("daily", "temperature_2m_max,temperature_2m_min,precipitation_probability_max,snowfall_sum,wind_speed_10m_max,relative_humidity_2m_mean,cloud_cover_mean,uv_index_max")
	params.Set("temperature_unit", "celsius")
	params.Set("timezone", loc.TimezoneID)
	params.Set("forecast_days", "7") // Get 7 days of forecasts

	endpoint := fmt.Sprintf("%s/forecast?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open-Meteo API returned status %d", resp.StatusCode)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Open-Meteo response: %w", err)
	}

	// Find the forecast for the requested date
	targetDate := date.Format("2006-01-02")
	for i, d := range data.Daily.Time {
		if d == targetDate {
			return c.buildForecast(loc, data, i, date)
		}
	}

	return nil, fmt.Errorf("no forecast available for %s", targetDate)
}

// GetForecastRange fetches forecasts for multiple days.
func (c *Client) GetForecastRange(loc *Location, days int) ([]*Forecast, error) {
	if days < 1 {
		days = 1
	}
	if days > 7 {
		days = 7 // Open-Meteo free tier limit
	}

	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", loc.Latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", loc.Longitude))
	params.Set("daily", "temperature_2m_max,temperature_2m_min,precipitation_probability_max,snowfall_sum,wind_speed_10m_max,relative_humidity_2m_mean,cloud_cover_mean,uv_index_max")
	params.Set("temperature_unit", "celsius")
	params.Set("timezone", loc.TimezoneID)
	params.Set("forecast_days", fmt.Sprintf("%d", days))

	endpoint := fmt.Sprintf("%s/forecast?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open-Meteo API returned status %d", resp.StatusCode)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Open-Meteo response: %w", err)
	}

	forecasts := make([]*Forecast, 0, len(data.Daily.Time))
	for i := range data.Daily.Time {
		date, err := time.Parse("2006-01-02", data.Daily.Time[i])
		if err != nil {
			continue
		}
		f, err := c.buildForecast(loc, data, i, date)
		if err != nil {
			continue
		}
		forecasts = append(forecasts, f)
	}

	return forecasts, nil
}

func (c *Client) buildForecast(loc *Location, data openMeteoResponse, idx int, date time.Time) (*Forecast, error) {
	if idx >= len(data.Daily.TemperatureMax) || idx >= len(data.Daily.TemperatureMin) {
		return nil, fmt.Errorf("index out of range")
	}

	forecast := &Forecast{
		Location:  loc.Name,
		Latitude:  loc.Latitude,
		Longitude: loc.Longitude,
		Date:      date,
		TempHigh:  data.Daily.TemperatureMax[idx],
		TempLow:   data.Daily.TemperatureMin[idx],
		TempMean:  (data.Daily.TemperatureMax[idx] + data.Daily.TemperatureMin[idx]) / 2,
	}

	// Optional fields with bounds checking
	if idx < len(data.Daily.PrecipitationProbMax) {
		forecast.RainProb = data.Daily.PrecipitationProbMax[idx]
	}
	if idx < len(data.Daily.SnowfallSum) {
		forecast.Snowfall = data.Daily.SnowfallSum[idx]
		// Estimate snow probability based on snowfall amount
		if forecast.Snowfall > 0 {
			forecast.SnowProb = 100
		} else if forecast.TempHigh < 2 && forecast.RainProb > 50 {
			forecast.SnowProb = forecast.RainProb * 0.5 // Rough estimate
		}
	}
	if idx < len(data.Daily.WindSpeedMax) {
		forecast.WindSpeed = data.Daily.WindSpeedMax[idx]
	}
	if idx < len(data.Daily.HumidityMean) {
		forecast.Humidity = int(data.Daily.HumidityMean[idx])
	}
	if idx < len(data.Daily.CloudCoverMean) {
		forecast.CloudCover = int(data.Daily.CloudCoverMean[idx])
	}
	if idx < len(data.Daily.UVIndexMax) {
		forecast.UVIndex = data.Daily.UVIndexMax[idx]
	}

	return forecast, nil
}

// Open-Meteo API response types
type openMeteoResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
	Daily     struct {
		Time                 []string  `json:"time"`
		TemperatureMax       []float64 `json:"temperature_2m_max"`
		TemperatureMin       []float64 `json:"temperature_2m_min"`
		PrecipitationProbMax []float64 `json:"precipitation_probability_max"`
		SnowfallSum          []float64 `json:"snowfall_sum"`
		WindSpeedMax         []float64 `json:"wind_speed_10m_max"`
		HumidityMean         []float64 `json:"relative_humidity_2m_mean"`
		CloudCoverMean       []float64 `json:"cloud_cover_mean"`
		UVIndexMax           []float64 `json:"uv_index_max"`
	} `json:"daily"`
}
