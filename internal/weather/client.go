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

// WeatherModel represents a specific weather prediction model.
type WeatherModel string

const (
	ModelBestMatch WeatherModel = ""                     // Open-Meteo default best match
	ModelECMWF     WeatherModel = "ecmwf_ifs04"          // ECMWF global - #1 worldwide
	ModelGFS       WeatherModel = "gfs_seamless"         // NOAA GFS - good global coverage
	ModelHRRR      WeatherModel = "gfs_hrrr"             // NOAA HRRR - best for US, 3km resolution
	ModelICON      WeatherModel = "icon_seamless"        // DWD ICON - best for Europe
	ModelICONEU    WeatherModel = "icon_eu"              // DWD ICON-EU - 7km Europe
	ModelUKMO      WeatherModel = "ukmo_seamless"        // UK Met Office - best for London
	ModelGEM       WeatherModel = "gem_seamless"         // Environment Canada - best for Toronto
	ModelKMA       WeatherModel = "jma_seamless"         // Korea/Japan regional
	ModelAROME     WeatherModel = "meteofrance_seamless" // Météo-France AROME
)

// ModelForecast contains a forecast from a specific model.
type ModelForecast struct {
	Model    WeatherModel
	Forecast *Forecast
}

// ConsensusForecast contains forecasts from multiple models with agreement metrics.
type ConsensusForecast struct {
	Location       string
	Date           time.Time
	Models         []ModelForecast
	AvgTempHigh    float64 // Average high across models
	AvgTempLow     float64 // Average low across models
	TempHighSpread float64 // Max - Min high temp (model disagreement)
	TempLowSpread  float64 // Max - Min low temp (model disagreement)
	Agreement      float64 // 0-1, how much models agree (1 = perfect agreement)
}

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
	Location   string
	Latitude   float64
	Longitude  float64
	Date       time.Time
	TempHigh   float64 // Celsius
	TempLow    float64 // Celsius
	TempMean   float64 // Celsius (average)
	RainProb   float64 // 0-100 (percentage)
	SnowProb   float64 // 0-100 (percentage)
	Snowfall   float64 // cm
	WindSpeed  float64 // km/h max
	Humidity   int     // 0-100 (percentage)
	CloudCover int     // 0-100 (percentage)
	UVIndex    float64
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

// GetForecastWithModel fetches forecast using a specific weather model.
func (c *Client) GetForecastWithModel(loc *Location, date time.Time, model WeatherModel) (*Forecast, error) {
	params := url.Values{}
	params.Set("latitude", fmt.Sprintf("%.4f", loc.Latitude))
	params.Set("longitude", fmt.Sprintf("%.4f", loc.Longitude))
	params.Set("daily", "temperature_2m_max,temperature_2m_min,precipitation_probability_max,snowfall_sum,wind_speed_10m_max,relative_humidity_2m_mean,cloud_cover_mean,uv_index_max")
	params.Set("temperature_unit", "celsius")
	params.Set("timezone", loc.TimezoneID)
	params.Set("forecast_days", "7")

	// Add model parameter if specified
	if model != ModelBestMatch && model != "" {
		params.Set("models", string(model))
	}

	endpoint := fmt.Sprintf("%s/forecast?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forecast: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open-Meteo API returned status %d for model %s", resp.StatusCode, model)
	}

	var data openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Open-Meteo response: %w", err)
	}

	targetDate := date.Format("2006-01-02")
	for i, d := range data.Daily.Time {
		if d == targetDate {
			return c.buildForecast(loc, data, i, date)
		}
	}

	return nil, fmt.Errorf("no forecast available for %s", targetDate)
}

// GetConsensusForecast fetches forecasts from multiple models and computes agreement.
func (c *Client) GetConsensusForecast(loc *Location, date time.Time) (*ConsensusForecast, error) {
	models := loc.GetPreferredModels()
	if len(models) == 0 {
		// Default to ECMWF + GFS if no specific models
		models = []WeatherModel{ModelECMWF, ModelGFS}
	}

	consensus := &ConsensusForecast{
		Location: loc.Name,
		Date:     date,
		Models:   make([]ModelForecast, 0, len(models)),
	}

	var tempHighSum, tempLowSum float64
	var tempHighMin, tempHighMax float64 = 999, -999
	var tempLowMin, tempLowMax float64 = 999, -999
	successCount := 0

	for _, model := range models {
		forecast, err := c.GetForecastWithModel(loc, date, model)
		if err != nil {
			// Log but continue - some models may not have data for all locations
			continue
		}

		consensus.Models = append(consensus.Models, ModelForecast{
			Model:    model,
			Forecast: forecast,
		})

		tempHighSum += forecast.TempHigh
		tempLowSum += forecast.TempLow

		if forecast.TempHigh < tempHighMin {
			tempHighMin = forecast.TempHigh
		}
		if forecast.TempHigh > tempHighMax {
			tempHighMax = forecast.TempHigh
		}
		if forecast.TempLow < tempLowMin {
			tempLowMin = forecast.TempLow
		}
		if forecast.TempLow > tempLowMax {
			tempLowMax = forecast.TempLow
		}

		successCount++
	}

	if successCount == 0 {
		return nil, fmt.Errorf("no models returned data for %s on %s", loc.Name, date.Format("2006-01-02"))
	}

	// Calculate averages and spreads
	consensus.AvgTempHigh = tempHighSum / float64(successCount)
	consensus.AvgTempLow = tempLowSum / float64(successCount)
	consensus.TempHighSpread = tempHighMax - tempHighMin
	consensus.TempLowSpread = tempLowMax - tempLowMin

	// Calculate agreement score (1.0 = perfect agreement, 0.0 = high disagreement)
	// Agreement decreases as spread increases
	// A spread of 0°C = 1.0 agreement
	// A spread of 5°C = 0.5 agreement
	// A spread of 10°C+ = 0.0 agreement
	maxSpread := consensus.TempHighSpread
	if consensus.TempLowSpread > maxSpread {
		maxSpread = consensus.TempLowSpread
	}
	consensus.Agreement = 1.0 - (maxSpread / 10.0)
	if consensus.Agreement < 0 {
		consensus.Agreement = 0
	}

	return consensus, nil
}

// BestForecast returns the most reliable forecast from consensus.
// Uses average of models when they agree, primary model when they disagree.
func (cf *ConsensusForecast) BestForecast() *Forecast {
	if len(cf.Models) == 0 {
		return nil
	}

	// If good agreement, return average
	if cf.Agreement >= 0.7 {
		f := cf.Models[0].Forecast
		return &Forecast{
			Location:  cf.Location,
			Latitude:  f.Latitude,
			Longitude: f.Longitude,
			Date:      cf.Date,
			TempHigh:  cf.AvgTempHigh,
			TempLow:   cf.AvgTempLow,
			TempMean:  (cf.AvgTempHigh + cf.AvgTempLow) / 2,
			RainProb:  f.RainProb,
			SnowProb:  f.SnowProb,
			Snowfall:  f.Snowfall,
			WindSpeed: f.WindSpeed,
			Humidity:  f.Humidity,
		}
	}

	// If disagreement, return first (primary) model
	return cf.Models[0].Forecast
}
