package weather

// Location represents a city with GPS coordinates.
type Location struct {
	Name         string   // Display name
	Aliases      []string // Alternative names (NYC, New York, etc.)
	Latitude     float64
	Longitude    float64
	TimezoneID   string // IANA timezone (e.g., "America/New_York")
}

// AllCities contains all cities we track for weather markets (US + International).
var AllCities = []Location{
	// US Cities
	{
		Name:       "New York",
		Aliases:    []string{"NYC", "New York City", "NY", "Manhattan"},
		Latitude:   40.7128,
		Longitude:  -74.0060,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "Los Angeles",
		Aliases:    []string{"LA", "L.A."},
		Latitude:   34.0522,
		Longitude:  -118.2437,
		TimezoneID: "America/Los_Angeles",
	},
	{
		Name:       "Chicago",
		Aliases:    []string{"Chi-Town"},
		Latitude:   41.8781,
		Longitude:  -87.6298,
		TimezoneID: "America/Chicago",
	},
	{
		Name:       "Miami",
		Aliases:    []string{"Miami Beach"},
		Latitude:   25.7617,
		Longitude:  -80.1918,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "Denver",
		Aliases:    []string{"Mile High City"},
		Latitude:   39.7392,
		Longitude:  -104.9903,
		TimezoneID: "America/Denver",
	},
	{
		Name:       "Seattle",
		Aliases:    []string{"SEA"},
		Latitude:   47.6062,
		Longitude:  -122.3321,
		TimezoneID: "America/Los_Angeles",
	},
	{
		Name:       "Atlanta",
		Aliases:    []string{"ATL"},
		Latitude:   33.7490,
		Longitude:  -84.3880,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "Dallas",
		Aliases:    []string{"DFW"},
		Latitude:   32.7767,
		Longitude:  -96.7970,
		TimezoneID: "America/Chicago",
	},
	{
		Name:       "Houston",
		Aliases:    []string{},
		Latitude:   29.7604,
		Longitude:  -95.3698,
		TimezoneID: "America/Chicago",
	},
	{
		Name:       "Phoenix",
		Aliases:    []string{},
		Latitude:   33.4484,
		Longitude:  -112.0740,
		TimezoneID: "America/Phoenix",
	},
	{
		Name:       "Philadelphia",
		Aliases:    []string{"Philly"},
		Latitude:   39.9526,
		Longitude:  -75.1652,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "San Francisco",
		Aliases:    []string{"SF"},
		Latitude:   37.7749,
		Longitude:  -122.4194,
		TimezoneID: "America/Los_Angeles",
	},
	{
		Name:       "Boston",
		Aliases:    []string{},
		Latitude:   42.3601,
		Longitude:  -71.0589,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "Washington",
		Aliases:    []string{"Washington DC", "DC"},
		Latitude:   38.9072,
		Longitude:  -77.0369,
		TimezoneID: "America/New_York",
	},
	{
		Name:       "Las Vegas",
		Aliases:    []string{"Vegas"},
		Latitude:   36.1699,
		Longitude:  -115.1398,
		TimezoneID: "America/Los_Angeles",
	},
	{
		Name:       "San Diego",
		Aliases:    []string{},
		Latitude:   32.7157,
		Longitude:  -117.1611,
		TimezoneID: "America/Los_Angeles",
	},
	{
		Name:       "Minneapolis",
		Aliases:    []string{},
		Latitude:   44.9778,
		Longitude:  -93.2650,
		TimezoneID: "America/Chicago",
	},
	{
		Name:       "Detroit",
		Aliases:    []string{},
		Latitude:   42.3314,
		Longitude:  -83.0458,
		TimezoneID: "America/Detroit",
	},
	// International Cities
	{
		Name:       "Toronto",
		Aliases:    []string{},
		Latitude:   43.6532,
		Longitude:  -79.3832,
		TimezoneID: "America/Toronto",
	},
	{
		Name:       "Seoul",
		Aliases:    []string{},
		Latitude:   37.5665,
		Longitude:  126.9780,
		TimezoneID: "Asia/Seoul",
	},
	{
		Name:       "Tokyo",
		Aliases:    []string{},
		Latitude:   35.6762,
		Longitude:  139.6503,
		TimezoneID: "Asia/Tokyo",
	},
	{
		Name:       "London",
		Aliases:    []string{},
		Latitude:   51.5074,
		Longitude:  -0.1278,
		TimezoneID: "Europe/London",
	},
	{
		Name:       "Paris",
		Aliases:    []string{},
		Latitude:   48.8566,
		Longitude:  2.3522,
		TimezoneID: "Europe/Paris",
	},
	{
		Name:       "Berlin",
		Aliases:    []string{},
		Latitude:   52.5200,
		Longitude:  13.4050,
		TimezoneID: "Europe/Berlin",
	},
	{
		Name:       "Sydney",
		Aliases:    []string{},
		Latitude:   -33.8688,
		Longitude:  151.2093,
		TimezoneID: "Australia/Sydney",
	},
	{
		Name:       "Melbourne",
		Aliases:    []string{},
		Latitude:   -37.8136,
		Longitude:  144.9631,
		TimezoneID: "Australia/Melbourne",
	},
	{
		Name:       "Auckland",
		Aliases:    []string{},
		Latitude:   -36.8485,
		Longitude:  174.7633,
		TimezoneID: "Pacific/Auckland",
	},
	{
		Name:       "Wellington",
		Aliases:    []string{},
		Latitude:   -41.2865,
		Longitude:  174.7762,
		TimezoneID: "Pacific/Auckland",
	},
	{
		Name:       "Buenos Aires",
		Aliases:    []string{},
		Latitude:   -34.6037,
		Longitude:  -58.3816,
		TimezoneID: "America/Argentina/Buenos_Aires",
	},
	{
		Name:       "Sao Paulo",
		Aliases:    []string{"São Paulo"},
		Latitude:   -23.5505,
		Longitude:  -46.6333,
		TimezoneID: "America/Sao_Paulo",
	},
	{
		Name:       "Mexico City",
		Aliases:    []string{},
		Latitude:   19.4326,
		Longitude:  -99.1332,
		TimezoneID: "America/Mexico_City",
	},
	{
		Name:       "Ankara",
		Aliases:    []string{},
		Latitude:   39.9334,
		Longitude:  32.8597,
		TimezoneID: "Europe/Istanbul",
	},
	{
		Name:       "Istanbul",
		Aliases:    []string{},
		Latitude:   41.0082,
		Longitude:  28.9784,
		TimezoneID: "Europe/Istanbul",
	},
	{
		Name:       "Moscow",
		Aliases:    []string{},
		Latitude:   55.7558,
		Longitude:  37.6173,
		TimezoneID: "Europe/Moscow",
	},
	{
		Name:       "Beijing",
		Aliases:    []string{},
		Latitude:   39.9042,
		Longitude:  116.4074,
		TimezoneID: "Asia/Shanghai",
	},
	{
		Name:       "Shanghai",
		Aliases:    []string{},
		Latitude:   31.2304,
		Longitude:  121.4737,
		TimezoneID: "Asia/Shanghai",
	},
	{
		Name:       "Hong Kong",
		Aliases:    []string{},
		Latitude:   22.3193,
		Longitude:  114.1694,
		TimezoneID: "Asia/Hong_Kong",
	},
	{
		Name:       "Singapore",
		Aliases:    []string{},
		Latitude:   1.3521,
		Longitude:  103.8198,
		TimezoneID: "Asia/Singapore",
	},
	{
		Name:       "Mumbai",
		Aliases:    []string{"Bombay"},
		Latitude:   19.0760,
		Longitude:  72.8777,
		TimezoneID: "Asia/Kolkata",
	},
	{
		Name:       "Delhi",
		Aliases:    []string{"New Delhi"},
		Latitude:   28.6139,
		Longitude:  77.2090,
		TimezoneID: "Asia/Kolkata",
	},
	{
		Name:       "Dubai",
		Aliases:    []string{},
		Latitude:   25.2048,
		Longitude:  55.2708,
		TimezoneID: "Asia/Dubai",
	},
	{
		Name:       "Cairo",
		Aliases:    []string{},
		Latitude:   30.0444,
		Longitude:  31.2357,
		TimezoneID: "Africa/Cairo",
	},
	{
		Name:       "Cape Town",
		Aliases:    []string{},
		Latitude:   -33.9249,
		Longitude:  18.4241,
		TimezoneID: "Africa/Johannesburg",
	},
	{
		Name:       "Johannesburg",
		Aliases:    []string{},
		Latitude:   -26.2041,
		Longitude:  28.0473,
		TimezoneID: "Africa/Johannesburg",
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
