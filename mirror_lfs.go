package backup

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrGitLFSNotFound means git-lfs isn't on PATH. It's only ever returned
// once a repository is found to use Git LFS -- detecting that needs
// nothing but git itself, so a machine backing up nothing but ordinary
// repositories never needs git-lfs installed at all.
var ErrGitLFSNotFound = errors.New("git-lfs not found on PATH")

// syncLFS fetches Git LFS content into dir's mirror over the same
// authenticated remote the clone or update just used. Detecting whether the
// repository uses LFS at all is pure git plumbing -- it costs nothing and
// needs no git-lfs binary for the common case of a repository that never
// used LFS, which is what keeps this from turning into an unconditional
// network call on every mirror.
func (m Mirror) syncLFS(ctx context.Context, git string, r Remote, dir string) error {
	uses, err := usesLFS(ctx, git, dir)
	if err != nil {
		return fmt.Errorf("mirror lfs %s: %w", r.CloneURL, err)
	}
	if !uses {
		return nil
	}

	gitLFS, err := m.gitLFSPath()
	if err != nil {
		return fmt.Errorf("mirror lfs %s: %w", r.CloneURL, err)
	}

	env, err := credentialEnv(r)
	if err != nil {
		return fmt.Errorf("mirror lfs %s: %w", r.CloneURL, err)
	}

	//nolint:gosec // argv, not a shell -- gitLFS and dir never pass through shell interpretation
	cmd := exec.CommandContext(ctx, gitLFS, "fetch", "--all")
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mirror lfs %s: %w: %s", r.CloneURL, err, out)
	}
	return nil
}

func (m Mirror) gitLFSPath() (string, error) {
	if m.GitLFSPath != "" {
		return m.GitLFSPath, nil
	}
	p, err := exec.LookPath("git-lfs")
	if err != nil {
		return "", ErrGitLFSNotFound
	}
	return p, nil
}

// usesLFS reports whether any ref in dir's mirror tracks Git LFS, by
// grepping every ref's tip tree for a .gitattributes entry naming the lfs
// filter -- at any depth, since git lfs track writes one at the repository
// root by default but a monorepo can equally set it per-directory. This is
// ordinary git plumbing -- no git-lfs binary, no network -- so it costs
// nothing for a repository that never used LFS, and it checks every branch
// and tag rather than just whichever one HEAD points at.
func usesLFS(ctx context.Context, git, dir string) (bool, error) {
	refs, err := refTips(ctx, git, dir)
	if err != nil {
		return false, err
	}
	if len(refs) == 0 {
		return false, nil
	}

	args := append([]string{"-C", dir, "grep", "--quiet", "-e", "filter=lfs"}, refs...)
	args = append(args, "--", ":(glob)**/.gitattributes")

	//nolint:gosec // argv, not a shell -- git, dir and refs never pass through shell interpretation
	cmd := exec.CommandContext(ctx, git, args...)
	cmd.Env = gitEnv()
	switch err := cmd.Run(); {
	case err == nil:
		return true, nil
	case isGrepNoMatch(err):
		return false, nil
	default:
		return false, fmt.Errorf("check lfs usage: %w", err)
	}
}

// isGrepNoMatch reports whether err is git grep's own "found nothing" exit
// status (1), as opposed to a real failure such as a corrupt repository.
func isGrepNoMatch(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

// refTips lists the commit each ref in dir currently points at.
func refTips(ctx context.Context, git, dir string) ([]string, error) {
	//nolint:gosec // argv, not a shell -- git and dir never pass through shell interpretation
	cmd := exec.CommandContext(ctx, git, "-C", dir, "for-each-ref", "--format=%(objectname)")
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list refs: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}
