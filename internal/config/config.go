package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// Default paths follow the PRD; overridden for local/dev via ECO_HOME.
func DataDir() string {
	if h := os.Getenv("ECO_HOME"); h != "" {
		return h
	}
	if os.Getuid() == 0 {
		return "/var/lib/ecorouter"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ecorouter")
}

func DefaultConfigPath() string {
	if p := os.Getenv("ECO_CONFIG"); p != "" {
		return p
	}
	if os.Getuid() == 0 {
		return "/etc/ecorouter/config.toml"
	}
	return filepath.Join(DataDir(), "config.toml")
}

func SecretsPath() string {
	return filepath.Join(DataDir(), "secrets.toml")
}

func DBPath() string {
	return filepath.Join(DataDir(), "ecorouter.db")
}

func PIDPath() string {
	return filepath.Join(DataDir(), "eco.pid")
}

func LogPath() string {
	return filepath.Join(DataDir(), "eco.log")
}

// Config is the full on-disk configuration (no secrets).
type Config struct {
	Server    ServerConfig              `toml:"server"`
	Security  SecurityConfig            `toml:"security"`
	Access    AccessConfig              `toml:"access"`
	Providers map[string]ProviderConfig `toml:"providers"`
	Routes    map[string]RouteConfig    `toml:"routes"`
	Savers    map[string]SaverConfig    `toml:"savers"`
	Defaults  DefaultsConfig            `toml:"defaults"`
	Health    HealthConfig              `toml:"health"`

	path string `toml:"-"`
	mu   sync.RWMutex
}

type ServerConfig struct {
	Port      int    `toml:"port"`
	Host      string `toml:"host"`
	Domain    string `toml:"domain"`
	TimeoutMs int    `toml:"timeout_ms"`
}

type SecurityConfig struct {
	RequireTLS       bool   `toml:"require_tls"`
	MaxBodyBytes     int64  `toml:"max_body_bytes"`
	GlobalRate       string `toml:"global_rate"`
	AuthFailLockout  string `toml:"auth_fail_lockout"`
	GlobalDailyCap   float64 `toml:"global_daily_cap"`
}

type AccessConfig struct {
	Allow []string `toml:"allow"`
	Deny  []string `toml:"deny"`
}

type ProviderConfig struct {
	Type    string `toml:"type"` // openai | anthropic | ollama
	BaseURL string `toml:"base_url"`
	// Models is a cached catalog; keys live in secrets store.
	Models []string `toml:"models,omitempty"`
}

type RouteConfig struct {
	Mode         string   `toml:"mode"` // single | fallback | round
	Models       []string `toml:"models"`
	Via          string   `toml:"via"`
	ViaRequired  bool     `toml:"via_required"`
	NoVia        bool     `toml:"no_via"`
}

type SaverConfig struct {
	URL string `toml:"url"`
}

type DefaultsConfig struct {
	ActiveRoute  string `toml:"active_route"`
	SaverDefault string `toml:"saver_default"`
}

type HealthConfig struct {
	Window         int     `toml:"window"`
	ErrorThreshold float64 `toml:"error_threshold"`
	MinRequests    int     `toml:"min_requests"`
	CooldownMs     int     `toml:"cooldown_ms"`
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port:      8080,
			Host:      "127.0.0.1",
			Domain:    "",
			TimeoutMs: 30000,
		},
		Security: SecurityConfig{
			RequireTLS:      true,
			MaxBodyBytes:    10 * 1024 * 1024,
			GlobalRate:      "600/min",
			AuthFailLockout: "5/1m -> 15m",
			GlobalDailyCap:  0,
		},
		Access: AccessConfig{
			Allow: []string{},
			Deny:  []string{},
		},
		Providers: map[string]ProviderConfig{},
		Routes:    map[string]RouteConfig{},
		Savers:    map[string]SaverConfig{},
		Defaults:  DefaultsConfig{},
		Health: HealthConfig{
			Window:         20,
			ErrorThreshold: 0.5,
			MinRequests:    5,
			CooldownMs:     60000,
		},
	}
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Providers == nil {
		cfg.Providers = map[string]ProviderConfig{}
	}
	if cfg.Routes == nil {
		cfg.Routes = map[string]RouteConfig{}
	}
	if cfg.Savers == nil {
		cfg.Savers = map[string]SaverConfig{}
	}
	return cfg, nil
}

func (c *Config) Path() string {
	if c.path != "" {
		return c.path
	}
	return DefaultConfigPath()
}

func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

func (c *Config) saveLocked() error {
	path := c.Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(DataDir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	return enc.Encode(c)
}

// EnsureDirs creates data directories with tight permissions.
func EnsureDirs() error {
	return os.MkdirAll(DataDir(), 0o700)
}

// ParseRate parses "60/min" or "10/s" into (count, per-second interval).
func ParseRate(s string) (limit float64, burst int, err error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "0" {
		return 0, 0, nil
	}
	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid rate %q (want N/min or N/s)", s)
	}
	var n float64
	if _, err := fmt.Sscanf(parts[0], "%f", &n); err != nil {
		return 0, 0, fmt.Errorf("invalid rate count: %w", err)
	}
	switch parts[1] {
	case "s", "sec", "second":
		return n, int(n), nil
	case "m", "min", "minute":
		return n / 60.0, int(n), nil
	case "h", "hour":
		return n / 3600.0, int(n), nil
	default:
		return 0, 0, fmt.Errorf("invalid rate unit %q", parts[1])
	}
}

// Clone returns a shallow-ish copy safe for read use.
func (c *Config) Clone() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := *c
	out.Providers = make(map[string]ProviderConfig, len(c.Providers))
	for k, v := range c.Providers {
		out.Providers[k] = v
	}
	out.Routes = make(map[string]RouteConfig, len(c.Routes))
	for k, v := range c.Routes {
		out.Routes[k] = v
	}
	out.Savers = make(map[string]SaverConfig, len(c.Savers))
	for k, v := range c.Savers {
		out.Savers[k] = v
	}
	out.Access.Allow = append([]string{}, c.Access.Allow...)
	out.Access.Deny = append([]string{}, c.Access.Deny...)
	return &out
}
