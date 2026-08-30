package backup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrGitNotFound means git isn't on PATH.
var ErrGitNotFound = errors.New("git not found on PATH")

// SSHKey is a private key Mirror.Sync authenticates a clone with over SSH,
// instead of the Authorization header a token-based Remote carries.
// Passphrase, if the key needs one, is already resolved to its value --
// see ForgeConfig.SSHKeyPassphraseEnv in config.go -- so Mirror never
// touches the environment variable name itself, only the value. Leave it
// empty for a key with no passphrase, or one already unlocked in an SSH
// agent: ssh tries the agent for a matching identity before ever asking for
// a passphrase itself.
type SSHKey struct {
	Path       string
	Passphrase string
}

// Remote is a repository's clone URL and its credential: either the value
// of the Authorization header to send for an HTTPS clone, or an SSHKey for
// one over SSH -- never both, since a Remoter builds one shape or the
// other for a given forge, not a mix of the two. Keeping the HTTPS
// credential in a header rather than the URL is what keeps it out of the
// mirror's own .git/config -- a URL embedded there would leave the token
// in the clear in every mirror on disk; the SSH key's passphrase gets the
// same treatment in credentialEnv, passed through the environment rather
// than ever touching disk or argv.
type Remote struct {
	CloneURL   string
	AuthHeader string
	SSHKey     *SSHKey
}

// Mirror keeps a bare mirror clone of a repository up to date on disk. The
// zero value works: it resolves git from PATH.
type Mirror struct {
	GitPath string
}

// Sync creates a bare mirror at dir if it doesn't exist yet, or refreshes an
// existing one otherwise. A refresh prunes refs the remote no longer has.
func (m Mirror) Sync(ctx context.Context, r Remote, dir string) error {
	git, err := m.gitPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		return m.update(ctx, git, r, dir)
	}

	return m.clone(ctx, git, r, dir)
}

// clone clones into dir+".partial" and only renames it into place once the
// clone succeeds, so a SIGINT mid-clone never leaves a half-built directory
// that the next run mistakes for an existing mirror.
func (m Mirror) clone(ctx context.Context, git string, r Remote, dir string) error {
	partial := dir + ".partial"
	if err := os.RemoveAll(partial); err != nil {
		return fmt.Errorf("mirror clone %s: %w", r.CloneURL, err)
	}

	env, cleanup, err := credentialEnv(r)
	if err != nil {
		return fmt.Errorf("mirror clone %s: %w", r.CloneURL, err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, git, "clone", "--mirror", r.CloneURL, partial) //nolint:gosec // argv, not a shell -- git and r.CloneURL never pass through shell interpretation
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mirror clone %s: %w: %s", r.CloneURL, err, out)
	}

	if err := os.Rename(partial, dir); err != nil {
		return fmt.Errorf("mirror clone %s: %w", r.CloneURL, err)
	}
	return nil
}

func (m Mirror) update(ctx context.Context, git string, r Remote, dir string) error {
	env, cleanup, err := credentialEnv(r)
	if err != nil {
		return fmt.Errorf("mirror update %s: %w", r.CloneURL, err)
	}
	defer cleanup()

	cmd := exec.CommandContext(ctx, git, "-C", dir, "remote", "update", "--prune") //nolint:gosec // argv, not a shell -- git and dir never pass through shell interpretation
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mirror update %s: %w: %s", r.CloneURL, err, out)
	}
	return nil
}

func (m Mirror) gitPath() (string, error) {
	if m.GitPath != "" {
		return m.GitPath, nil
	}
	p, err := exec.LookPath("git")
	if err != nil {
		return "", ErrGitNotFound
	}
	return p, nil
}

// sshPassphraseEnvVar is the name Mirror sets in the git subprocess's own
// environment for the askpass script written by writeAskpassScript to read
// the resolved passphrase back out of. Never the name from
// ForgeConfig.SSHKeyPassphraseEnv itself -- that only ever exists in the
// parent process's environment, and nothing here forwards it.
const sshPassphraseEnvVar = "BACKUP_GIT_REPOS_SSH_KEY_PASSPHRASE" //nolint:gosec // this is the name of an env var, not a credential value

// credentialEnv builds the environment a git subprocess runs with: the
// Authorization header for an HTTPS Remote, passed through the environment
// rather than the command line or the remote URL so it never gets persisted
// to disk (scoped to the remote's own scheme and host, so it's never sent
// anywhere else even if git follows a redirect) -- or, for an SSH Remote,
// GIT_SSH_COMMAND and, for a passphrase-protected key, an askpass script.
// The returned cleanup func removes anything credentialEnv wrote to disk --
// only ever the askpass script -- and must run once the subprocess it was
// built for has exited, not before.
func credentialEnv(r Remote) ([]string, func(), error) {
	noop := func() {}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if r.SSHKey != nil {
		return sshCredentialEnv(env, *r.SSHKey)
	}

	if r.AuthHeader == "" {
		return env, noop, nil
	}

	u, err := url.Parse(r.CloneURL)
	if err != nil {
		return nil, noop, fmt.Errorf("parse clone url: %w", err)
	}

	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http."+u.Scheme+"://"+u.Host+"/.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: "+r.AuthHeader,
	), noop, nil
}

// sshCredentialEnv points git at a specific private key over
// GIT_SSH_COMMAND, and never lets a run block waiting on a prompt: a key
// needing no passphrase gets BatchMode=yes, which fails outright rather
// than asking for one (from a terminal or otherwise) if it turns out to
// need one after all. A passphrase-protected key can't use BatchMode --
// that would also disable the SSH_ASKPASS fallback below -- so it relies on
// SSH_ASKPASS_REQUIRE=force instead, which makes ssh use the askpass
// script unconditionally rather than only when it detects no controlling
// terminal. Either way, StrictHostKeyChecking=accept-new keeps a first
// connection to an unknown host from blocking on a yes/no prompt too.
func sshCredentialEnv(env []string, key SSHKey) ([]string, func(), error) {
	noop := func() {}
	sshCmd := "ssh -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -i " + shellQuote(key.Path)

	if key.Passphrase == "" {
		return append(env, "GIT_SSH_COMMAND="+sshCmd+" -o BatchMode=yes"), noop, nil
	}

	askpass, cleanup, err := writeAskpassScript()
	if err != nil {
		return nil, noop, fmt.Errorf("ssh credential: %w", err)
	}

	return append(env,
		"GIT_SSH_COMMAND="+sshCmd,
		"SSH_ASKPASS="+askpass,
		"SSH_ASKPASS_REQUIRE=force",
		sshPassphraseEnvVar+"="+key.Passphrase,
	), cleanup, nil
}

// writeAskpassScript writes a script that does nothing but print
// sshPassphraseEnvVar's value to stdout -- the protocol ssh invokes
// SSH_ASKPASS under -- to a private temp file, and returns a cleanup func
// that removes it. The passphrase itself never appears in the script or on
// any argv; it reaches the script only through the environment
// sshCredentialEnv also sets, the same way the HTTPS path above keeps a
// token out of argv and disk.
func writeAskpassScript() (path string, cleanup func(), err error) {
	noop := func() {}

	f, err := os.CreateTemp("", "backup-git-repos-askpass-*")
	if err != nil {
		return "", noop, fmt.Errorf("create askpass script: %w", err)
	}
	cleanup = func() { _ = os.Remove(f.Name()) }

	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$%s\"\n", sshPassphraseEnvVar)
	if _, err := f.WriteString(script); err != nil {
		_ = f.Close()
		cleanup()
		return "", noop, fmt.Errorf("write askpass script: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("close askpass script: %w", err)
	}
	if err := os.Chmod(f.Name(), 0o700); err != nil { //nolint:gosec // the askpass protocol execs this file directly, so it needs the owner execute bit -- 0600 alone wouldn't run
		cleanup()
		return "", noop, fmt.Errorf("chmod askpass script: %w", err)
	}

	return f.Name(), cleanup, nil
}

// shellQuote wraps s in single quotes for GIT_SSH_COMMAND, which git hands
// to a shell rather than exec'ing directly -- unlike the argv-based git
// invocations elsewhere in this file, an unquoted key path containing a
// space or shell metacharacter would break here.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
