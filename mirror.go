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
// zero value works: it resolves git and git-lfs from PATH.
type Mirror struct {
	GitPath    string
	GitLFSPath string
}

// Sync creates a bare mirror at dir if it doesn't exist yet, or refreshes an
// existing one otherwise. A refresh prunes refs the remote no longer has.
// Either way, once the ordinary git objects are in place, it fetches any
// Git LFS content the repository uses -- LFS objects live outside git's own
// object store, so a clone or a remote update never brings them along on
// their own.
func (m Mirror) Sync(ctx context.Context, r Remote, dir string) error {
	git, err := m.gitPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		if err := m.update(ctx, git, r, dir); err != nil {
			return err
		}
	} else if err := m.clone(ctx, git, r, dir); err != nil {
		return err
	}

	return m.syncLFS(ctx, git, r, dir)
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
	env := append(gitEnv(), "GIT_TERMINAL_PROMPT=0")
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

// gitEnv is the process environment with any inherited GIT_DIR,
// GIT_WORK_TREE or GIT_INDEX_FILE stripped. Git sets those for a hook's own
// process -- this project's own pre-push included -- and left in place
// they override every "-C dir" a git subprocess here is given, silently
// pointing a clone, an update, or an LFS check at whatever repository ran
// the hook instead of the one actually being mirrored.
func gitEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="),
			strings.HasPrefix(kv, "GIT_INDEX_FILE="):
			continue
		}
		filtered = append(filtered, kv)
	}

	return filtered
}
