package weather

// PredictabilityTier indicates how reliable weather forecasts are for a location.
// Based on model coverage, terrain complexity, and maritime/lake effects.
type PredictabilityTier string

const (
	TierS PredictabilityTier = "S" // Best: stable patterns, excellent model coverage
	TierA PredictabilityTier = "A" // Good: reliable models, some variability
	TierB PredictabilityTier = "B" // Moderate: variable weather, decent models
	TierC PredictabilityTier = "C" // Poor: complex terrain, limited models
	TierD PredictabilityTier = "D" // Avoid: unpredictable, poor model coverage
)

// TierMultiplier returns the scoring multiplier for a tier.
func (t PredictabilityTier) TierMultiplier() float64 {
	switch t {
	case TierS:
		return 2.0 // Strong preference
	case TierA:
		return 1.5
	case TierB:
		return 1.0
	case TierC:
		return 0.5 // Penalize
	case TierD:
		return 0.1 // Heavy penalty
	default:
		return 0.5
	}
}

// Location represents a city with GPS coordinates.
type Location struct {
	Name       string   // Display name
	Aliases    []string // Alternative names (NYC, New York, etc.)
	Latitude   float64
	Longitude  float64
	TimezoneID string             // IANA timezone (e.g., "America/New_York")
	Tier       PredictabilityTier // Forecast reliability tier
}

// AllCities contains all cities we track for weather markets (US + International).
// Tier assignments based on: model coverage, terrain complexity, maritime effects.
// Reference: UK Met Office UKV, HRRR, NAM, ECMWF, KMA coverage analysis.
var AllCities = []Location{
	// ═══════════════════════════════════════════════════════════════════
	// TIER S - Best predictability (flat terrain, stable patterns, excellent models)
	// ═══════════════════════════════════════════════════════════════════
	{
		Name:       "London",
		Aliases:    []string{},
		Latitude:   51.5074,
		Longitude:  -0.1278,
		TimezoneID: "Europe/London",
		Tier:       TierS, // UK Met Office UKV 1.5km - best in world
	},
	{
		Name:       "Dallas",
		Aliases:    []string{"DFW"},
		Latitude:   32.7767,
		Longitude:  -96.7970,
		TimezoneID: "America/Chicago",
		Tier:       TierS, // Flat terrain, HRRR excels
	},
	{
		Name:       "Atlanta",
		Aliases:    []string{"ATL"},
		Latitude:   33.7490,
		Longitude:  -84.3880,
		TimezoneID: "America/New_York",
		Tier:       TierS, // Inland, stable SE patterns, HRRR/NAM strong
	},
	{
		Name:       "Houston",
		Aliases:    []string{},
		Latitude:   29.7604,
		Longitude:  -95.3698,
		TimezoneID: "America/Chicago",
		Tier:       TierS, // Flat Texas, good models
	},
	{
		Name:       "Phoenix",
		Aliases:    []string{},
		Latitude:   33.4484,
		Longitude:  -112.0740,
		TimezoneID: "America/Phoenix",
		Tier:       TierS, // Desert = very predictable
	},
	{
		Name:       "Las Vegas",
		Aliases:    []string{"Vegas"},
		Latitude:   36.1699,
		Longitude:  -115.1398,
		TimezoneID: "America/Los_Angeles",
		Tier:       TierS, // Desert, stable patterns
	},
	// ═══════════════════════════════════════════════════════════════════
	// TIER A - Good predictability (reliable models, some variability)
	// ═══════════════════════════════════════════════════════════════════
	{
		Name:       "New York",
		Aliases:    []string{"NYC", "New York City", "NY", "Manhattan"},
		Latitude:   40.7128,
		Longitude:  -74.0060,
		TimezoneID: "America/New_York",
		Tier:       TierA, // Good coverage but coastal, nor'easters
	},
	{
		Name:       "Seoul",
		Aliases:    []string{},
		Latitude:   37.5665,
		Longitude:  126.9780,
		TimezoneID: "Asia/Seoul",
		Tier:       TierA, // KMA local model, continental
	},
	{
		Name:       "Chicago",
		Aliases:    []string{"Chi-Town"},
		Latitude:   41.8781,
		Longitude:  -87.6298,
		TimezoneID: "America/Chicago",
		Tier:       TierA, // Lake Michigan effect but good models
	},
	{
		Name:       "Philadelphia",
		Aliases:    []string{"Philly"},
		Latitude:   39.9526,
		Longitude:  -75.1652,
		TimezoneID: "America/New_York",
		Tier:       TierA, // East coast, good models
	},
	{
		Name:       "Boston",
		Aliases:    []string{},
		Latitude:   42.3601,
		Longitude:  -71.0589,
		TimezoneID: "America/New_York",
		Tier:       TierA, // Coastal but well-monitored
	},
	{
		Name:       "Washington",
		Aliases:    []string{"Washington DC", "DC"},
		Latitude:   38.9072,
		Longitude:  -77.0369,
		TimezoneID: "America/New_York",
		Tier:       TierA, // Inland east coast, good coverage
	},
	{
		Name:       "San Diego",
		Aliases:    []string{},
		Latitude:   32.7157,
		Longitude:  -117.1611,
		TimezoneID: "America/Los_Angeles",
		Tier:       TierA, // Stable Mediterranean climate
	},
	{
		Name:       "Berlin",
		Aliases:    []string{},
		Latitude:   52.5200,
		Longitude:  13.4050,
		TimezoneID: "Europe/Berlin",
		Tier:       TierA, // ICON-EU 7km, flat terrain
	},
	{
		Name:       "Paris",
		Aliases:    []string{},
		Latitude:   48.8566,
		Longitude:  2.3522,
		TimezoneID: "Europe/Paris",
		Tier:       TierA, // AROME 1.3km, good coverage
	},
	{
		Name:       "Tokyo",
		Aliases:    []string{},
		Latitude:   35.6762,
		Longitude:  139.6503,
		TimezoneID: "Asia/Tokyo",
		Tier:       TierA, // JMA excellent but maritime influence
	},
	// ═══════════════════════════════════════════════════════════════════
	// TIER B - Moderate predictability (variable weather, decent models)
	// ═══════════════════════════════════════════════════════════════════
	{
		Name:       "Seattle",
		Aliases:    []string{"SEA"},
		Latitude:   47.6062,
		Longitude:  -122.3321,
		TimezoneID: "America/Los_Angeles",
		Tier:       TierB, // Pacific maritime, Cascade mountains
	},
	{
		Name:       "Toronto",
		Aliases:    []string{},
		Latitude:   43.6532,
		Longitude:  -79.3832,
		TimezoneID: "America/Toronto",
		Tier:       TierB, // Lake Ontario effect, changeable
	},
	{
		Name:       "Denver",
		Aliases:    []string{"Mile High City"},
		Latitude:   39.7392,
		Longitude:  -104.9903,
		TimezoneID: "America/Denver",
		Tier:       TierB, // Mountain effects, rapid changes
	},
	{
		Name:       "Minneapolis",
		Aliases:    []string{},
		Latitude:   44.9778,
		Longitude:  -93.2650,
		TimezoneID: "America/Chicago",
		Tier:       TierB, // Continental extremes
	},
	{
		Name:       "Detroit",
		Aliases:    []string{},
		Latitude:   42.3314,
		Longitude:  -83.0458,
		TimezoneID: "America/Detroit",
		Tier:       TierB, // Great Lakes effect
	},
	{
		Name:       "San Francisco",
		Aliases:    []string{"SF"},
		Latitude:   37.7749,
		Longitude:  -122.4194,
		TimezoneID: "America/Los_Angeles",
		Tier:       TierB, // Microclimates, fog
	},
	{
		Name:       "Miami",
		Aliases:    []string{"Miami Beach"},
		Latitude:   25.7617,
		Longitude:  -80.1918,
		TimezoneID: "America/New_York",
		Tier:       TierB, // Tropical variability
	},
	{
		Name:       "Los Angeles",
		Aliases:    []string{"LA", "L.A."},
		Latitude:   34.0522,
		Longitude:  -118.2437,
		TimezoneID: "America/Los_Angeles",
		Tier:       TierB, // Marine layer, microclimates
	},
	// ═══════════════════════════════════════════════════════════════════
	// TIER C - Poor predictability (complex terrain, limited models)
	// ═══════════════════════════════════════════════════════════════════
	{
		Name:       "Buenos Aires",
		Aliases:    []string{},
		Latitude:   -34.6037,
		Longitude:  -58.3816,
		TimezoneID: "America/Argentina/Buenos_Aires",
		Tier:       TierC, // Southern hemisphere, limited models
	},
	{
		Name:       "Sao Paulo",
		Aliases:    []string{"São Paulo"},
		Latitude:   -23.5505,
		Longitude:  -46.6333,
		TimezoneID: "America/Sao_Paulo",
		Tier:       TierC, // Limited model coverage
	},
	{
		Name:       "Mexico City",
		Aliases:    []string{},
		Latitude:   19.4326,
		Longitude:  -99.1332,
		TimezoneID: "America/Mexico_City",
		Tier:       TierC, // High altitude, complex terrain
	},
	{
		Name:       "Sydney",
		Aliases:    []string{},
		Latitude:   -33.8688,
		Longitude:  151.2093,
		TimezoneID: "Australia/Sydney",
		Tier:       TierC, // Southern hemisphere, variable
	},
	{
		Name:       "Melbourne",
		Aliases:    []string{},
		Latitude:   -37.8136,
		Longitude:  144.9631,
		TimezoneID: "Australia/Melbourne",
		Tier:       TierC, // "Four seasons in one day"
	},
	{
		Name:       "Auckland",
		Aliases:    []string{},
		Latitude:   -36.8485,
		Longitude:  174.7633,
		TimezoneID: "Pacific/Auckland",
		Tier:       TierC, // Pacific maritime, variable
	},
	// ═══════════════════════════════════════════════════════════════════
	// TIER D - Avoid (unpredictable, poor model coverage)
	// ═══════════════════════════════════════════════════════════════════
	{
		Name:       "Wellington",
		Aliases:    []string{},
		Latitude:   -41.2865,
		Longitude:  174.7762,
		TimezoneID: "Pacific/Auckland",
		Tier:       TierD, // Windiest city, extreme variability
	},
	{
		Name:       "Ankara",
		Aliases:    []string{},
		Latitude:   39.9334,
		Longitude:  32.8597,
		TimezoneID: "Europe/Istanbul",
		Tier:       TierD, // Limited model coverage for Turkey
	},
	{
		Name:       "Istanbul",
		Aliases:    []string{},
		Latitude:   41.0082,
		Longitude:  28.9784,
		TimezoneID: "Europe/Istanbul",
		Tier:       TierD, // Bosphorus effects, complex
	},
	{
		Name:       "Moscow",
		Aliases:    []string{},
		Latitude:   55.7558,
		Longitude:  37.6173,
		TimezoneID: "Europe/Moscow",
		Tier:       TierD, // Limited western model access
	},
	{
		Name:       "Beijing",
		Aliases:    []string{},
		Latitude:   39.9042,
		Longitude:  116.4074,
		TimezoneID: "Asia/Shanghai",
		Tier:       TierD, // CMA models not accessible
	},
	{
		Name:       "Shanghai",
		Aliases:    []string{},
		Latitude:   31.2304,
		Longitude:  121.4737,
		TimezoneID: "Asia/Shanghai",
		Tier:       TierD, // Limited model access
	},
	{
		Name:       "Hong Kong",
		Aliases:    []string{},
		Latitude:   22.3193,
		Longitude:  114.1694,
		TimezoneID: "Asia/Hong_Kong",
		Tier:       TierD, // Typhoons, tropical
	},
	{
		Name:       "Singapore",
		Aliases:    []string{},
		Latitude:   1.3521,
		Longitude:  103.8198,
		TimezoneID: "Asia/Singapore",
		Tier:       TierD, // Tropical, daily thunderstorms
	},
	{
		Name:       "Mumbai",
		Aliases:    []string{"Bombay"},
		Latitude:   19.0760,
		Longitude:  72.8777,
		TimezoneID: "Asia/Kolkata",
		Tier:       TierD, // Monsoon, limited models
	},
	{
		Name:       "Delhi",
		Aliases:    []string{"New Delhi"},
		Latitude:   28.6139,
		Longitude:  77.2090,
		TimezoneID: "Asia/Kolkata",
		Tier:       TierD, // Monsoon effects, dust storms
	},
	{
		Name:       "Dubai",
		Aliases:    []string{},
		Latitude:   25.2048,
		Longitude:  55.2708,
		TimezoneID: "Asia/Dubai",
		Tier:       TierD, // Limited model coverage
	},
	{
		Name:       "Cairo",
		Aliases:    []string{},
		Latitude:   30.0444,
		Longitude:  31.2357,
		TimezoneID: "Africa/Cairo",
		Tier:       TierD, // Limited model coverage
	},
	{
		Name:       "Cape Town",
		Aliases:    []string{},
		Latitude:   -33.9249,
		Longitude:  18.4241,
		TimezoneID: "Africa/Johannesburg",
		Tier:       TierD, // Complex terrain, limited models
	},
	{
		Name:       "Johannesburg",
		Aliases:    []string{},
		Latitude:   -26.2041,
		Longitude:  28.0473,
		TimezoneID: "Africa/Johannesburg",
		Tier:       TierD, // Limited model coverage
	},
}

// USCities is an alias for backward compatibility.
var USCities = AllCities

// FindLocationByName finds a location by name or alias (case-insensitive).
func FindLocationByName(name string) *Location {
	nameLower := toLower(name)
	for i := range AllCities {
		if toLower(AllCities[i].Name) == nameLower {
			return &AllCities[i]
		}
		for _, alias := range AllCities[i].Aliases {
			if toLower(alias) == nameLower {
				return &AllCities[i]
			}
		}
	}
	return nil
}

// FindLocationInText searches for any location mention in text.
func FindLocationInText(text string) *Location {
	textLower := toLower(text)
	for i := range AllCities {
		if containsWord(textLower, toLower(AllCities[i].Name)) {
			return &AllCities[i]
		}
		for _, alias := range AllCities[i].Aliases {
			if containsWord(textLower, toLower(alias)) {
				return &AllCities[i]
			}
		}
	}
	return nil
}

// toLower converts string to lowercase.
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// containsWord checks if text contains the word (simple substring match).
func containsWord(text, word string) bool {
	return len(word) > 0 && len(text) >= len(word) && indexOf(text, word) >= 0
}

// indexOf returns index of substring or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
