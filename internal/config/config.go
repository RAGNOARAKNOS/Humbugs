// Package config loads the list of Royal Mint coins Humbugs should track.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Coin is one tracked product. Name is an optional human label; the canonical
// name and SKU are discovered from the page on the first scrape.
type Coin struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Config is the top-level coins.yaml document.
type Config struct {
	Coins []Coin `yaml:"coins"`
}

// Load reads and parses a coins.yaml file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	for i, c := range cfg.Coins {
		if c.URL == "" {
			return nil, fmt.Errorf("config %q: coin #%d is missing a url", path, i+1)
		}
	}
	return &cfg, nil
}
