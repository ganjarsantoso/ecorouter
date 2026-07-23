package secrets

import (
	"fmt"
	"os"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/ganjar/ecorouter/internal/config"
)

// Store holds provider API keys. File is 0600; never written to public config.
type Store struct {
	Providers map[string]string `toml:"providers"` // name -> api key
	path      string
	mu        sync.RWMutex
}

func Load(path string) (*Store, error) {
	if path == "" {
		path = config.SecretsPath()
	}
	s := &Store{
		Providers: map[string]string{},
		path:      path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if _, err := toml.Decode(string(data), s); err != nil {
		return nil, fmt.Errorf("parse secrets: %w", err)
	}
	if s.Providers == nil {
		s.Providers = map[string]string{}
	}
	return s, nil
}

func (s *Store) Get(provider string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.Providers[provider]
	return k, ok
}

func (s *Store) Set(provider, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Providers == nil {
		s.Providers = map[string]string{}
	}
	s.Providers[provider] = key
	return s.saveLocked()
}

func (s *Store) Delete(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Providers, provider)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(config.DataDir(), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(s)
}
