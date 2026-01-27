package weather

import (
	"math"
)

// TempDistribution represents a probability distribution for temperature forecasts.
// Weather forecasts have inherent uncertainty - we model this as a normal distribution
// centered on the forecast with standard deviation based on forecast horizon.
type TempDistribution struct {
	Mean   float64 // Forecast temperature (Celsius)
	StdDev float64 // Standard deviation (uncertainty)
	Low    float64 // Minimum (TempLow from forecast)
	High   float64 // Maximum (TempHigh from forecast)
}

// NewTempDistribution creates a distribution from a forecast.
// The standard deviation is estimated based on forecast accuracy research:
// - Same day forecasts: ~1.5C std dev
// - 1 day out: ~2.0C std dev
// - 2-3 days out: ~2.5C std dev
// - 4-7 days out: ~3.5C std dev
func NewTempDistribution(forecast *Forecast, daysAhead int) *TempDistribution {
	// Estimate standard deviation based on forecast horizon
	var stdDev float64
	switch {
	case daysAhead <= 0:
		stdDev = 1.5 // Same day - very accurate
	case daysAhead == 1:
		stdDev = 2.0 // Tomorrow - good accuracy
	case daysAhead <= 3:
		stdDev = 2.5 // 2-3 days - moderate accuracy
	default:
		stdDev = 3.5 // 4+ days - lower accuracy
	}

	return &TempDistribution{
		Mean:   forecast.TempMean,
		StdDev: stdDev,
		Low:    forecast.TempLow,
		High:   forecast.TempHigh,
	}
}

// NewHighTempDistribution creates a distribution for high temperature.
func NewHighTempDistribution(forecast *Forecast, daysAhead int) *TempDistribution {
	var stdDev float64
	switch {
	case daysAhead <= 0:
		stdDev = 1.2 // Same day highs are quite accurate
	case daysAhead == 1:
		stdDev = 1.8
	case daysAhead <= 3:
		stdDev = 2.2
	default:
		stdDev = 3.0
	}

	return &TempDistribution{
		Mean:   forecast.TempHigh,
		StdDev: stdDev,
		Low:    forecast.TempLow,
		High:   forecast.TempHigh + 2*stdDev, // Allow for outliers
	}
}

// NewLowTempDistribution creates a distribution for low temperature.
func NewLowTempDistribution(forecast *Forecast, daysAhead int) *TempDistribution {
	var stdDev float64
	switch {
	case daysAhead <= 0:
		stdDev = 1.2
	case daysAhead == 1:
		stdDev = 1.8
	case daysAhead <= 3:
		stdDev = 2.2
	default:
		stdDev = 3.0
	}

	return &TempDistribution{
		Mean:   forecast.TempLow,
		StdDev: stdDev,
		Low:    forecast.TempLow - 2*stdDev,
		High:   forecast.TempHigh,
	}
}

// ProbAbove calculates the probability that the actual temperature will be above the threshold.
// Uses the cumulative distribution function (CDF) of the normal distribution.
func (d *TempDistribution) ProbAbove(threshold float64) float64 {
	// P(X > threshold) = 1 - CDF(threshold)
	return 1 - normalCDF(threshold, d.Mean, d.StdDev)
}

// ProbBelow calculates the probability that the actual temperature will be below the threshold.
func (d *TempDistribution) ProbBelow(threshold float64) float64 {
	// P(X < threshold) = CDF(threshold)
	return normalCDF(threshold, d.Mean, d.StdDev)
}

// ProbBetween calculates the probability that temperature is between low and high.
func (d *TempDistribution) ProbBetween(low, high float64) float64 {
	return normalCDF(high, d.Mean, d.StdDev) - normalCDF(low, d.Mean, d.StdDev)
}

// normalCDF computes the cumulative distribution function of a normal distribution.
// Uses the error function approximation.
func normalCDF(x, mean, stdDev float64) float64 {
	if stdDev <= 0 {
		if x < mean {
			return 0
		}
		return 1
	}
	z := (x - mean) / (stdDev * math.Sqrt2)
	return 0.5 * (1 + erf(z))
}

// erf approximates the error function using Horner's method.
// Approximation from Abramowitz and Stegun.
func erf(x float64) float64 {
	// Save the sign of x
	sign := 1.0
	if x < 0 {
		sign = -1
		x = -x
	}

	// Constants for approximation
	a1 := 0.254829592
	a2 := -0.284496736
	a3 := 1.421413741
	a4 := -1.453152027
	a5 := 1.061405429
	p := 0.3275911

	// Approximation formula
	t := 1.0 / (1.0 + p*x)
	y := 1.0 - (((((a5*t+a4)*t)+a3)*t+a2)*t+a1)*t*math.Exp(-x*x)

	return sign * y
}

// EdgeCalculator computes trading edge based on forecast vs market price.
type EdgeCalculator struct{}

// NewEdgeCalculator creates a new edge calculator.
func NewEdgeCalculator() *EdgeCalculator {
	return &EdgeCalculator{}
}

// CalculateEdge computes the edge and expected value.
// Edge = Our Probability - Market Price (as probability)
// EV = (Win Prob * Payout) - (Loss Prob * Stake)
// For binary markets: Payout = 1/Price - 1, Stake = 1
func (ec *EdgeCalculator) CalculateEdge(ourProb, marketPrice float64) (edge float64, ev float64, shouldBet bool) {
	// Edge is the difference between our probability and market implied probability
	edge = ourProb - marketPrice

	// Expected value calculation
	// If we buy at marketPrice, we pay marketPrice and get 1 if we win
	// EV = ourProb * (1 - marketPrice) - (1 - ourProb) * marketPrice
	// Simplified: EV = ourProb - marketPrice = edge
	ev = edge

	// We only bet when edge exceeds minimum threshold
	// This is set by the strategy, but basic sanity check here
	shouldBet = edge > 0.05 // 5% minimum edge

	return edge, ev, shouldBet
}

// CalculateKellyFraction computes optimal bet size using Kelly Criterion.
// Kelly = (p * b - q) / b
// where p = probability of winning, q = probability of losing (1-p)
// b = odds received on the bet (payout ratio)
func (ec *EdgeCalculator) CalculateKellyFraction(ourProb, marketPrice float64) float64 {
	if marketPrice <= 0 || marketPrice >= 1 {
		return 0
	}

	p := ourProb
	q := 1 - ourProb
	b := (1 - marketPrice) / marketPrice // Payout odds

	kelly := (p*b - q) / b

	// Never bet more than 25% Kelly for safety
	if kelly > 0.25 {
		kelly = 0.25
	}

	// Don't bet if negative Kelly
	if kelly < 0 {
		return 0
	}

	return kelly
}

// RainProbability calculates probability of rain/precipitation based on forecast.
func RainProbability(forecast *Forecast) float64 {
	// Open-Meteo gives direct precipitation probability
	return forecast.RainProb / 100.0
}

// SnowProbability calculates probability of snow based on forecast.
func SnowProbability(forecast *Forecast) float64 {
	// If we have snowfall data, calculate probability
	if forecast.Snowfall > 0 {
		return 1.0 // Definite snow predicted
	}

	// If temp is below freezing and precipitation likely, estimate snow chance
	if forecast.TempHigh < 2 && forecast.RainProb > 30 {
		// Temperature-based snow probability adjustment
		tempFactor := 1.0
		if forecast.TempHigh > 0 {
			tempFactor = (2 - forecast.TempHigh) / 2 // Linear decrease as temp approaches 2C
		}
		return (forecast.RainProb / 100.0) * tempFactor
	}

	return 0
}
