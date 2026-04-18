package options

import (
	"dropbear/black76"
	"dropbear/clocky"
	"dropbear/decimal"
	"testing"
)

func makeStrike(price decimal.Decimal, callBid, callAsk, putBid, putAsk decimal.Decimal) *Strike {
	s := &Strike{Price: price}
	s.Call = &Option{
		Class:  'C',
		Strike: s,
		Bid:    callBid,
		Ask:    callAsk,
		Got:    GotBid | GotAsk,
	}
	s.Put = &Option{
		Class:  'P',
		Strike: s,
		Bid:    putBid,
		Ask:    putAsk,
		Got:    GotBid | GotAsk,
	}
	return s
}

func linkStrikes(strikes ...*Strike) {
	for i := 1; i < len(strikes); i++ {
		strikes[i].Prev = strikes[i-1]
		strikes[i-1].Next = strikes[i]
	}
}

// TestProbabilityNonNegative verifies that the Breeden-Litzenberger butterfly
// produces non-negative probabilities for a well-behaved (convex) option chain.
func TestProbabilityNonNegative(t *testing.T) {
	// Simulate a convex call/put curve with uniform $5 strike spacing.
	// Call prices decrease with strike (convex), put prices increase (convex).
	strikes := []*Strike{
		makeStrike(decimal.FromInt(5900), decimal.Parse("110"), decimal.Parse("112"), decimal.Parse("2.0"), decimal.Parse("2.4")),
		makeStrike(decimal.FromInt(5905), decimal.Parse("106"), decimal.Parse("108"), decimal.Parse("2.5"), decimal.Parse("2.9")),
		makeStrike(decimal.FromInt(5910), decimal.Parse("102"), decimal.Parse("104"), decimal.Parse("3.2"), decimal.Parse("3.6")),
		makeStrike(decimal.FromInt(5915), decimal.Parse("98.5"), decimal.Parse("100.5"), decimal.Parse("4.0"), decimal.Parse("4.4")),
		makeStrike(decimal.FromInt(5920), decimal.Parse("95.2"), decimal.Parse("97.2"), decimal.Parse("5.0"), decimal.Parse("5.4")),
		makeStrike(decimal.FromInt(5925), decimal.Parse("92.1"), decimal.Parse("94.1"), decimal.Parse("6.2"), decimal.Parse("6.6")),
		makeStrike(decimal.FromInt(5930), decimal.Parse("89.2"), decimal.Parse("91.2"), decimal.Parse("7.6"), decimal.Parse("8.0")),
	}
	linkStrikes(strikes...)
	for _, s := range strikes[1 : len(strikes)-1] {
		prob := s.Probability()
		if prob.IsNegative() {
			t.Errorf("strike %s: probability is negative: %s", s.Price, prob)
		}
	}
}

// TestProbabilityZeroWhenMissingNeighbors verifies edge strikes return zero.
func TestProbabilityZeroWhenMissingNeighbors(t *testing.T) {
	strikes := []*Strike{
		makeStrike(decimal.FromInt(5900), decimal.Parse("110"), decimal.Parse("112"), decimal.Parse("2.0"), decimal.Parse("2.4")),
		makeStrike(decimal.FromInt(5905), decimal.Parse("106"), decimal.Parse("108"), decimal.Parse("2.5"), decimal.Parse("2.9")),
		makeStrike(decimal.FromInt(5910), decimal.Parse("102"), decimal.Parse("104"), decimal.Parse("3.2"), decimal.Parse("3.6")),
	}
	linkStrikes(strikes...)
	if prob := strikes[0].Probability(); !prob.IsZero() {
		t.Errorf("first strike should have zero probability, got %s", prob)
	}
	if prob := strikes[2].Probability(); !prob.IsZero() {
		t.Errorf("last strike should have zero probability, got %s", prob)
	}
}

// TestProbabilityZeroWhenNotReady verifies a strike without call or put returns zero.
func TestProbabilityZeroWhenNotReady(t *testing.T) {
	s := &Strike{Price: decimal.FromInt(5900)}
	s.Call = &Option{Class: 'C', Strike: s}
	// no Put
	if prob := s.Probability(); !prob.IsZero() {
		t.Errorf("not-ready strike should have zero probability, got %s", prob)
	}
}

// TestProbabilityNonUniformStrikes verifies correctness with non-uniform strike spacing.
func TestProbabilityNonUniformStrikes(t *testing.T) {
	// $5 left gap, $10 right gap — non-uniform spacing
	strikes := []*Strike{
		makeStrike(decimal.FromInt(5900), decimal.Parse("110"), decimal.Parse("112"), decimal.Parse("2.0"), decimal.Parse("2.4")),
		makeStrike(decimal.FromInt(5905), decimal.Parse("106"), decimal.Parse("108"), decimal.Parse("2.5"), decimal.Parse("2.9")),
		makeStrike(decimal.FromInt(5915), decimal.Parse("98.5"), decimal.Parse("100.5"), decimal.Parse("4.0"), decimal.Parse("4.4")),
	}
	linkStrikes(strikes...)
	prob := strikes[1].Probability()
	if prob.IsNegative() {
		t.Errorf("non-uniform strike probability is negative: %s", prob)
	}
	if prob.IsZero() {
		t.Errorf("non-uniform strike probability should be non-zero")
	}
}

// TestProbabilitySumApproximatesOne verifies that probabilities across a chain
// sum to something reasonable (less than 1, since tails are excluded).
func TestProbabilitySumApproximatesOne(t *testing.T) {
	// Generate option prices from Black-76 to ensure proper convexity.
	// F=6000, sigma=0.15, T=1/365 (one day), r=0, uniform $5 strikes.
	F := 6000.0
	sigma := 0.15
	E := clocky.Day
	spread := decimal.Parse("0.2")
	halfSpread := spread.Half()
	var strikes []*Strike
	for k := 5900; k <= 6100; k += 5 {
		K := float64(k)
		callMid := decimal.FromFloat64(max(black76.CallPrice(F, K, 0, E, sigma), 0.05))
		putMid := decimal.FromFloat64(max(black76.PutPrice(F, K, 0, E, sigma), 0.05))
		strikes = append(strikes, makeStrike(
			decimal.FromInt(k),
			callMid.Sub(halfSpread), callMid.Add(halfSpread),
			putMid.Sub(halfSpread), putMid.Add(halfSpread),
		))
	}
	linkStrikes(strikes...)
	var sum decimal.Decimal
	for _, s := range strikes[1 : len(strikes)-1] {
		prob := s.Probability()
		if prob.IsNegative() {
			t.Errorf("strike %s: negative probability %s", s.Price, prob)
		}
		sum = sum.Add(prob)
	}
	t.Logf("probability sum across interior strikes: %s", sum)
	if !sum.IsPositive() {
		t.Errorf("probability sum should be positive, got %s", sum)
	}
	if sum.Cmp(decimal.One) > 0 {
		t.Errorf("probability sum should not exceed 1, got %s", sum)
	}
}

// TestProbabilityNoQuotes verifies zero is returned when quotes are missing.
func TestProbabilityNoQuotes(t *testing.T) {
	strikes := []*Strike{
		makeStrike(decimal.FromInt(5900), decimal.Parse("110"), decimal.Parse("112"), decimal.Parse("2.0"), decimal.Parse("2.4")),
		makeStrike(decimal.FromInt(5905), decimal.Parse("106"), decimal.Parse("108"), decimal.Parse("2.5"), decimal.Parse("2.9")),
		makeStrike(decimal.FromInt(5910), decimal.Parse("102"), decimal.Parse("104"), decimal.Parse("3.2"), decimal.Parse("3.6")),
	}
	linkStrikes(strikes...)
	// Clear Got flags so HasQuotes returns false
	for _, s := range strikes {
		s.Call.Got = 0
		s.Put.Got = 0
	}
	prob := strikes[1].Probability()
	if !prob.IsZero() {
		t.Errorf("expected zero probability with no quotes, got %s", prob)
	}
}

// now benchmark Probability()
func BenchmarkProbability(b *testing.B) {
	strikes := []*Strike{
		makeStrike(decimal.FromInt(5900), decimal.Parse("110"), decimal.Parse("112"), decimal.Parse("2.0"), decimal.Parse("2.4")),
		makeStrike(decimal.FromInt(5905), decimal.Parse("106"), decimal.Parse("108"), decimal.Parse("2.5"), decimal.Parse("2.9")),
		makeStrike(decimal.FromInt(5910), decimal.Parse("102"), decimal.Parse("104"), decimal.Parse("3.2"), decimal.Parse("3.6")),
	}
	linkStrikes(strikes...)
	for b.Loop() {
		strikes[1].Probability()
	}
}
