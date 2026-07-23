package cost

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/ganjar/ecorouter/internal/config"
)

// ModelPrice is per-1M-token USD pricing for a model.
type ModelPrice struct {
	InputPer1M  float64 `toml:"input_per_1m"`
	OutputPer1M float64 `toml:"output_per_1m"`
}

type priceFile struct {
	// key format: "provider/model" or just "model"
	Prices map[string]ModelPrice `toml:"prices"`
}

var (
	loadOnce sync.Once
	loaded   map[string]ModelPrice
)

// PricingPath returns the path to the user-editable pricing file.
func PricingPath() string {
	return filepath.Join(config.DataDir(), "pricing.toml")
}

func loadPrices() map[string]ModelPrice {
	loadOnce.Do(func() {
		loaded = map[string]ModelPrice{}
		b, err := os.ReadFile(PricingPath())
		if err != nil {
			return
		}
		var pf priceFile
		if _, err := toml.Decode(string(b), &pf); err != nil {
			return
		}
		for k, v := range pf.Prices {
			loaded[k] = v
		}
	})
	return loaded
}

// ReloadPrices forces a re-read (used by pricing wizard after save).
func ReloadPrices() {
	loadOnce = sync.Once{}
	loaded = nil
	loadPrices()
}

// SavePrices persists the user's price table.
func SavePrices(m map[string]ModelPrice) error {
	if err := config.EnsureDirs(); err != nil {
		return err
	}
	f, err := os.OpenFile(PricingPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	pf := priceFile{Prices: m}
	if err := toml.NewEncoder(f).Encode(pf); err != nil {
		return err
	}
	ReloadPrices()
	return nil
}

// GetPrices returns a copy of the loaded price table.
func GetPrices() map[string]ModelPrice {
	m := loadPrices()
	out := make(map[string]ModelPrice, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// PricingFileExists reports whether pricing.toml is present on disk.
func PricingFileExists() bool {
	_, err := os.Stat(PricingPath())
	return err == nil
}
