package backup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/goccy/go-yaml"
)

// ErrMissingToken means a forge's token_env names an environment variable
// that isn't set.
var ErrMissingToken = errors.New("token environment variable not set")

// ErrAmbiguousToken means a forge set both token and token_env. Only one can
// win silently, and picking one without saying so is how a token meant to
// override the other quietly gets ignored -- config validation fails
// instead, the same as issue #9's SSH-key-vs-token rule.
var ErrAmbiguousToken = errors.New("set exactly one of token or token_env")

// ErrTokenCommandFailed means a forge's token_command exited non-zero.
// LoadConfig never falls back to an empty credential silently -- a
// password manager that's locked or misconfigured has to fail the run, not
// authenticate as nothing.
var ErrTokenCommandFailed = errors.New("token_command failed")

// UnknownKindError means a forge's kind isn't one backup-git-repos knows how
// to talk to.
type UnknownKindError struct {
	Kind string
}

func (e *UnknownKindError) Error() string {
	return fmt.Sprintf("unknown forge kind %q", e.Kind)
}

// ForgeConfig is one forge entry from the config file. Token is resolved
// during LoadConfig, either from the environment variable named by
// TokenEnv or copied straight from TokenLiteral -- never read as-is from
// the struct the YAML unmarshals into, so every other field reaching a
// Runner has gone through the same validation regardless of which form the
// file used.
type ForgeConfig struct {
	Name         string `yaml:"name"`
	Kind         string `yaml:"kind"`
	URL          string `yaml:"url"`
	TokenEnv     string `yaml:"token_env"`
	TokenLiteral string `yaml:"token"`
	TokenCommand string `yaml:"token_command"`
	Token        string `yaml:"-"`

	// SkipMirrors excludes repositories a forge reports as mirrors of an
	// external upstream from both listing and mirroring. Only the forgejo
	// adapter interprets it today; other kinds ignore it.
	SkipMirrors bool `yaml:"skip_mirrors"`
}

// Config is the top-level shape of the YAML config file.
type Config struct {
	Dest   string        `yaml:"dest"`
	Forges []ForgeConfig `yaml:"forges"`
}

// knownForgeKinds are the values ForgeConfig.Kind is allowed to hold.
var knownForgeKinds = map[string]bool{
	"forgejo": true,
	"gitlab":  true,
	"github":  true,
}

// LoadConfig reads and validates the config file at path: every forge's
// kind must be one this build supports, and every forge's token_env must
// name a set environment variable. Failing before touching the network
// beats a 401 partway through a run.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the file the caller explicitly named via --config/-c; reading it is the whole point
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

		token, err := resolveToken(f)
		if err != nil {
			return Config{}, fmt.Errorf("forge %q: %w", f.Name, err)
		}
		cfg.Forges[i].Token = token
	}

	return cfg, nil
}

// resolveToken returns f's token from whichever of TokenCommand,
// TokenLiteral or TokenEnv it set. TokenCommand wins over the other two
// unconditionally -- someone who configured it did so on purpose, and a
// lingering literal or token_env in the same entry shouldn't silently
// override it.
func resolveToken(f ForgeConfig) (string, error) {
	if f.TokenCommand != "" {
		return runTokenCommand(f.TokenCommand)
	}

	if f.TokenLiteral != "" && f.TokenEnv != "" {
		return "", ErrAmbiguousToken
	}

	if f.TokenLiteral != "" {
		return f.TokenLiteral, nil
	}

	token, ok := os.LookupEnv(f.TokenEnv)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrMissingToken, f.TokenEnv)
	}

	return token, nil
}

// runTokenCommand runs command through the shell -- so a pipeline or a
// quoted argument works unmodified, the same as restic's
// --password-command -- and returns its stdout with exactly one trailing
// newline trimmed. A non-zero exit is ErrTokenCommandFailed rather than an
// empty token: a locked password manager or a typo'd command has to stop
// the run, not authenticate as nothing.
func runTokenCommand(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).Output() //nolint:gosec // command is this config's own trusted setting, the whole point of the feature
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrTokenCommandFailed, command, err)
	}

	return strings.TrimSuffix(string(out), "\n"), nil
}
