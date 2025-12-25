package indicators

import (
	"dropbear/clocky"
	"dropbear/decimal"
	"math"
)

// Intensity implements the Avellaneda-Stoikov trading intensity model.
// It fits λ(δ) = α * exp(-κ * δ) where δ is distance from mid-price.
//
// This tells us about market microstructure:
//   - α (Alpha): baseline intensity - how much trading happens near mid-price
//   - κ (Koopa): decay rate - how fast activity drops away from mid
//
// High κ = tight liquidity (activity concentrated near mid)
// Low κ = dispersed liquidity (activity spread across price levels)
//
// Uses O(1) updates with fixed bins and exponential decay.
// Trades may be added out-of-order with proper decay handling.
// Regression is computed incrementally from decayed bin statistics.
type Intensity struct {
	Alpha     decimal.Decimal
	Kappa     decimal.Decimal
	halfLife  float64
	midPrice  float64
	lastDecay clocky.Time
	binVolume [numBins]float64
}

const (
	numBins       = 5000  // support five bins per basis point
	maxDelta      = 0.01  // recognize trades within 1% of mid-price
	minVolume     = 10000 // minimum USD volume for reliable estimate
	minBins       = 5     // minimum filled bins for reliable regression
	minVolumeBin  = 1e-8  // minimum quantity per trade (filters dust)
	decayInterval = clocky.Second
	binWidth      = maxDelta / numBins
	ln2           = 0.6931471805599453
)

// NewIntensity creates a new trading intensity indicator.
// halfLife specifies the exponential decay half-life (e.g., 5 minutes).
// After one half-life, old samples have half the weight of new ones.
func NewIntensity(halfLife clocky.Duration) *Intensity {
	return &Intensity{
		halfLife: float64(halfLife),
	}
}

// SetMidPrice updates the reference mid-price for distance calculations.
func (ti *Intensity) SetMidPrice(mid decimal.Decimal) {
	ti.midPrice = mid.Float64()
}

// AddTrade records a trade execution for intensity.
func (ti *Intensity) AddTrade(time clocky.Time, price, quantity decimal.Decimal) {

	// calculate distance from market
	if ti.midPrice <= 0 {
		return
	}
	delta := price.Float64() - ti.midPrice
	if delta < 0 {
		delta = -delta
	}
	delta /= ti.midPrice

	// ignore suspicious trades
	if delta >= maxDelta {
		return
	}

	// apply exponential decay
	// batching doubles performance without changing backtests
	newVolume := quantity.Float64()
	if ti.lastDecay == 0 {
		ti.lastDecay = time
	} else if time >= ti.lastDecay.Add(decayInterval) {
		dt := float64(time - ti.lastDecay)
		decay := math.Exp(-ln2 * dt / ti.halfLife)
		for i := range ti.binVolume {
			ti.binVolume[i] *= decay
		}
		ti.lastDecay = time
	} else if time < ti.lastDecay {
		// if trade is a blast from the past
		// then we must apply retroactive decay
		dt := float64(ti.lastDecay - time)
		decay := math.Exp(-ln2 * dt / ti.halfLife)
		newVolume *= decay
	}
	if newVolume < minVolumeBin {
		return
	}

	// add volume to appropriate bin (in USD value, not raw quantity)
	bin := int(delta / binWidth)
	usdValue := newVolume * price.Float64()
	ti.binVolume[bin] += usdValue

	// compute regression stats from bins
	var sumX, sumY, sumXY, sumXX, count float64
	for i := range numBins {
		if ti.binVolume[i] < minVolumeBin {
			continue
		}
		x := float64(i)*binWidth + binWidth/2 // bin center
		y := math.Log(ti.binVolume[i])
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
		count++
	}

	// if only only one bin is filled
	// that means all trades are very close to the midpoint
	// assume very high koopa (will be clamped in usage anyway)
	if count < 2 {
		if count == 1 {
			v := ti.totalVolume()
			if v > minVolume {
				ti.Kappa = decimal.FromInt(100000)
				ti.Alpha = decimal.FromFloat64(v)
			}
		}
		return
	}

	// linear regression: y = b + m*x
	meanX := sumX / count
	meanY := sumY / count
	varX := sumXX/count - meanX*meanX
	covXY := sumXY/count - meanX*meanY
	if varX < 1e-12 {
		return
	}
	m := covXY / varX
	b := meanY - m*meanX

	// λ(δ) = α * e^(-κ*δ), so log(λ) = log(α) - κ*δ
	// κ = -m, α = e^b
	kappa := -m
	alpha := math.Exp(b)
	if kappa < 0 {
		kappa = 0
	}

	ti.Kappa = decimal.FromFloat64(kappa)
	ti.Alpha = decimal.FromFloat64(alpha)
}

// IsReady returns true when there's enough data for a reliable estimate.
func (ti *Intensity) IsReady() bool {
	if ti.Kappa.IsZero() {
		return false
	}
	if ti.totalVolume() < minVolume {
		return false
	}
	if ti.filledBins() < minBins {
		return false
	}
	return true
}

// OptimalSpread returns the Avellaneda-Stoikov optimal half-spread.
func (ti *Intensity) OptimalSpread() decimal.Decimal {
	if ti.Kappa.IsZero() {
		return decimal.Zero
	}
	return decimal.One.Div(ti.Kappa)
}

func (ti *Intensity) totalVolume() float64 {
	total := 0.0
	for i := range numBins {
		total += ti.binVolume[i]
	}
	return total
}

func (ti *Intensity) filledBins() int {
	count := 0
	for i := range numBins {
		if ti.binVolume[i] > minVolumeBin {
			count++
		}
	}
	return count
}
