package cost

import (
	"fmt"
	"strings"
)

// Per-1M-token USD list prices (approximate; unpriced models return nil).
// Estimates only — never claimed as exact billing.
var prices = map[string]struct {
	In  float64
	Out float64
}{
	"gpt-4o":                  {In: 2.50, Out: 10.00},
	"gpt-4o-mini":             {In: 0.15, Out: 0.60},
	"gpt-4.1":                 {In: 2.00, Out: 8.00},
	"gpt-4.1-mini":            {In: 0.40, Out: 1.60},
	"gpt-4.1-nano":            {In: 0.10, Out: 0.40},
	"o1":                      {In: 15.00, Out: 60.00},
	"o1-mini":                 {In: 1.10, Out: 4.40},
	"o3-mini":                 {In: 1.10, Out: 4.40},
	"claude-3-5-sonnet":       {In: 3.00, Out: 15.00},
	"claude-3-5-haiku":        {In: 0.80, Out: 4.00},
	"claude-3-opus":           {In: 15.00, Out: 75.00},
	"claude-sonnet-4":         {In: 3.00, Out: 15.00},
	"claude-opus-4":           {In: 15.00, Out: 75.00},
	"claude-3-haiku-20240307": {In: 0.25, Out: 1.25},
}

// Estimate returns USD cost for token counts, or nil if the model is unpriced.
func Estimate(model string, tokensIn, tokensOut int) *float64 {
	name := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		name = model[i+1:]
	}
	p, ok := prices[name]
	if !ok {
		for k, v := range prices {
			if strings.HasPrefix(name, k) {
				p = v
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil
	}
	c := (float64(tokensIn)/1_000_000.0)*p.In + (float64(tokensOut)/1_000_000.0)*p.Out
	return &c
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
