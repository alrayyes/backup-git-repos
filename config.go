package backup

import (
	"errors"
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// ErrMissingToken means a forge's token_env names an environment variable
// that isn't set.
var ErrMissingToken = errors.New("token environment variable not set")

// UnknownKindError means a forge's kind isn't one backup-git-repos knows how
// to talk to.
type UnknownKindError struct {
	Kind string
}

func (e *UnknownKindError) Error() string {
	return fmt.Sprintf("unknown forge kind %q", e.Kind)
}

// ForgeConfig is one forge entry from the config file. Token is resolved
// from the environment variable named by TokenEnv during LoadConfig, never
// read from the file itself.
type ForgeConfig struct {
	Name     string `yaml:"name"`
	Kind     string `yaml:"kind"`
	URL      string `yaml:"url"`
	TokenEnv string `yaml:"token_env"`
	Token    string `yaml:"-"`
}

// Config is the top-level shape of the YAML config file.
type Config struct {
	Dest   string        `yaml:"dest"`
	Forges []ForgeConfig `yaml:"forges"`
}

// knownForgeKinds are the values ForgeConfig.Kind is allowed to hold.
var knownForgeKinds = map[string]bool{
	"forgejo": true,
}

// LoadConfig reads and validates the config file at path: every forge's
// kind must be one this build supports, and every forge's token_env must
// name a set environment variable. Failing before touching the network
// beats a 401 partway through a run.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}

	for i, f := range cfg.Forges {
		if !knownForgeKinds[f.Kind] {
			return Config{}, fmt.Errorf("forge %q: %w", f.Name, &UnknownKindError{Kind: f.Kind})
		}

		token, ok := os.LookupEnv(f.TokenEnv)
		if !ok {
			return Config{}, fmt.Errorf("forge %q: %w: %s", f.Name, ErrMissingToken, f.TokenEnv)
		}
		cfg.Forges[i].Token = token
	}

	return cfg, nil
}
