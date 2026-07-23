package cost

import (
	"fmt"
	"strings"
)

// Estimate returns USD cost for token counts, or nil if the model is unpriced.
// Estimates only — never claimed as exact billing.
func Estimate(model string, tokensIn, tokensOut int) *float64 {
	prices := loadPrices()

	// try exact match first
	if p, ok := prices[model]; ok {
		c := calc(p, tokensIn, tokensOut)
		return &c
	}

	// try bare name (strip provider/)
	bare := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		bare = model[i+1:]
	}
	if p, ok := prices[bare]; ok {
		c := calc(p, tokensIn, tokensOut)
		return &c
	}

	// try prefix match (e.g. "gpt-4o" matches "gpt-4o-2024-11-20")
	for k, p := range prices {
		kb := k
		if i := strings.LastIndex(k, "/"); i >= 0 {
			kb = k[i+1:]
		}
		if strings.HasPrefix(bare, kb) {
			c := calc(p, tokensIn, tokensOut)
			return &c
		}
	}

	// unpriced
	return nil
}

func calc(p ModelPrice, in, out int) float64 {
	return (float64(in)/1_000_000.0)*p.InputPer1M + (float64(out)/1_000_000.0)*p.OutputPer1M
}

// Format returns a display string; unpriced → "unpriced" (never "$0" for unknown).
func Format(c *float64) string {
	if c == nil {
		return "unpriced"
	}
	if *c > 0 && *c < 0.0001 {
		return "<$0.0001"
	}
	return fmt.Sprintf("$%.4f", *c)
}
