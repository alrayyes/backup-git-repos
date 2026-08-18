package backup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrGitNotFound means git isn't on PATH.
var ErrGitNotFound = errors.New("git not found on PATH")

// Remote is a repository's clone URL and, where the forge needs one, the
// value of the Authorization header to send with it. Keeping the credential
// in a header rather than the URL is what keeps it out of the mirror's own
// .git/config -- a URL embedded there would leave the token in the clear in
// every mirror on disk.
type Remote struct {
	CloneURL   string
	AuthHeader string
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

	env, err := credentialEnv(r)
	if err != nil {
		return fmt.Errorf("mirror clone %s: %w", r.CloneURL, err)
	}

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
	env, err := credentialEnv(r)
	if err != nil {
		return fmt.Errorf("mirror update %s: %w", r.CloneURL, err)
	}

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

// credentialEnv passes the Authorization header through the environment
// rather than the command line or the remote URL, so it never gets
// persisted to disk. The header is scoped to the remote's own scheme and
// host, so it's never sent anywhere else even if git follows a redirect.
func credentialEnv(r Remote) ([]string, error) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if r.AuthHeader == "" {
		return env, nil
	}

	u, err := url.Parse(r.CloneURL)
	if err != nil {
		return nil, fmt.Errorf("parse clone url: %w", err)
	}

	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http."+u.Scheme+"://"+u.Host+"/.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: "+r.AuthHeader,
	), nil
}
