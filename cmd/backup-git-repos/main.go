// Command backup-git-repos mirrors git repositories out of self-hosted
// forges. See the root package for what it does; this file only wires
// signal handling and hands off to cobra.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	backup "github.com/alrayyes/backup-git-repos"
)

var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := backup.NewRootCommand(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
