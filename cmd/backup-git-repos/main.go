// Command backup-git-repos mirrors git repositories out of self-hosted
// forges. See the root package for what it does; this file wires signal
// handling, the concrete forge adapters, and hands off to cobra.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	backup "github.com/alrayyes/backup-git-repos"
	"github.com/alrayyes/backup-git-repos/internal/forgejo"
	"github.com/alrayyes/backup-git-repos/internal/github"
	"github.com/alrayyes/backup-git-repos/internal/gitlab"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := backup.NewRootCommand(version, newRunner).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// newRunner is the composition root: it's the one place that knows about
// every forge adapter, wiring the concrete client for a forge's kind into a
// backup.Runner.
func newRunner(fc backup.ForgeConfig) (backup.Runner, error) {
	switch fc.Kind {
	case "forgejo":
		client, err := forgejo.New(fc.URL, fc.Token)
		if err != nil {
			return backup.Runner{}, fmt.Errorf("forge %q: %w", fc.Name, err)
		}
		return backup.Runner{Lister: client, Mirrorer: backup.Mirror{}, Remoter: client}, nil
	case "gitlab":
		client, err := gitlab.New(fc.URL, fc.Token)
		if err != nil {
			return backup.Runner{}, fmt.Errorf("forge %q: %w", fc.Name, err)
		}
		return backup.Runner{Lister: client, Mirrorer: backup.Mirror{}, Remoter: client}, nil
	case "github":
		client, err := github.New(fc.URL, fc.Token)
		if err != nil {
			return backup.Runner{}, fmt.Errorf("forge %q: %w", fc.Name, err)
		}
		return backup.Runner{Lister: client, Mirrorer: backup.Mirror{}, Remoter: client}, nil
	default:
		return backup.Runner{}, fmt.Errorf("forge %q: %w", fc.Name, &backup.UnknownKindError{Kind: fc.Kind})
	}
}
