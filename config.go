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

// ErrAmbiguousToken means a forge set both token and token_env. Only one can
// win silently, and picking one without saying so is how a token meant to
// override the other quietly gets ignored -- config validation fails
// instead, the same as issue #9's SSH-key-vs-token rule.
var ErrAmbiguousToken = errors.New("set exactly one of token or token_env")

// ErrAmbiguousCredential means a forge set ssh_key alongside token or
// token_env. The two are alternative ways to authenticate the same clone,
// not layers that stack -- see Remote in mirror.go -- so, the same as
// ErrAmbiguousToken above, silently preferring one over the other is worse
// than failing before a run ever touches the network.
var ErrAmbiguousCredential = errors.New("set exactly one of ssh_key or token/token_env")

// ErrMissingSSHKeyPassphrase means a forge's ssh_key_passphrase_env names an
// environment variable that isn't set.
var ErrMissingSSHKeyPassphrase = errors.New("ssh key passphrase environment variable not set")

// ErrSSHHostNotSupported means a github forge set ssh_host. github.com
// always clones over SSH from its own fixed host, so the field would
// otherwise be silently ignored rather than doing what it looks like it
// does -- the same "fail loud, not quiet" reasoning as ErrAmbiguousToken.
var ErrSSHHostNotSupported = errors.New("ssh_host is only supported for gitlab and forgejo forges")

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
	Token        string `yaml:"-"`

	// SSHKeyPath, when set, clones this forge's repositories over SSH with
	// this private key instead of HTTPS with a token -- mutually exclusive
	// with TokenEnv/TokenLiteral, enforced in LoadConfig. SSHKeyPassphraseEnv
	// names an environment variable holding the key's passphrase, for a key
	// that needs one and isn't already unlocked in an SSH agent; left empty,
	// the key is assumed to need no passphrase or to already be in an agent.
	SSHKeyPath          string `yaml:"ssh_key"`
	SSHKeyPassphraseEnv string `yaml:"ssh_key_passphrase_env"`
	SSHKeyPassphrase    string `yaml:"-"`

	// SSHHost overrides the host[:port] a gitlab or forgejo entry clones
	// over SSH from, for an instance that serves it somewhere other than
	// the default -- port 22 on url's own host. Only those two kinds read
	// it; github always clones over SSH from github.com itself.
	SSHHost string `yaml:"ssh_host"`

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
		if f.Kind == "github" && f.SSHHost != "" {
			return Config{}, fmt.Errorf("forge %q: %w", f.Name, ErrSSHHostNotSupported)
		}

		resolved, err := resolveCredential(f)
		if err != nil {
			return Config{}, fmt.Errorf("forge %q: %w", f.Name, err)
		}
		cfg.Forges[i] = resolved
	}

	return cfg, nil
}

// resolveCredential validates f's credential fields and returns f with
// whichever one it set resolved to an actual value: Token from
// TokenLiteral/TokenEnv, or SSHKeyPassphrase from SSHKeyPassphraseEnv --
// never both, and never read as bare TokenEnv/SSHKeyPassphraseEnv names
// past this point, so every other place a ForgeConfig reaches has gone
// through the same validation regardless of which form the file used.
func resolveCredential(f ForgeConfig) (ForgeConfig, error) {
	usesSSHKey := f.SSHKeyPath != ""
	usesToken := f.TokenLiteral != "" || f.TokenEnv != ""
	if usesSSHKey && usesToken {
		return ForgeConfig{}, ErrAmbiguousCredential
	}

	if usesSSHKey {
		if f.SSHKeyPassphraseEnv == "" {
			return f, nil
		}
		passphrase, ok := os.LookupEnv(f.SSHKeyPassphraseEnv)
		if !ok {
			return ForgeConfig{}, fmt.Errorf("%w: %s", ErrMissingSSHKeyPassphrase, f.SSHKeyPassphraseEnv)
		}
		f.SSHKeyPassphrase = passphrase
		return f, nil
	}

	if f.TokenLiteral != "" && f.TokenEnv != "" {
		return ForgeConfig{}, ErrAmbiguousToken
	}
	if f.TokenLiteral != "" {
		f.Token = f.TokenLiteral
		return f, nil
	}

	token, ok := os.LookupEnv(f.TokenEnv)
	if !ok {
		return ForgeConfig{}, fmt.Errorf("%w: %s", ErrMissingToken, f.TokenEnv)
	}
	f.Token = token
	return f, nil
}
