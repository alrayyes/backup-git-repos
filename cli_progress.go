package backup

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// isTerminalWriter reports whether w is a terminal a progress bar could
// safely redraw a line on. Anything that isn't a real *os.File -- a
// bytes.Buffer in a test, a pipe, a redirected file -- answers false, since
// term.IsTerminal needs a file descriptor to ask the OS about.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// newProgressReporter returns a Progress callback that redraws a single
// "label: done/total" line on w using a carriage return, finishing with a
// newline once done reaches total. When tty is false it returns a no-op:
// a carriage-return-redrawn line is unreadable once piped or redirected
// into a file, so nothing is written there beyond the run's existing
// summary line.
func newProgressReporter(w io.Writer, label string, tty bool) func(done, total int) {
	if !tty {
		return func(int, int) {}
	}

	var mu sync.Mutex
	return func(done, total int) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(w, "\r%s: %d/%d", label, done, total)
		if done >= total {
			_, _ = fmt.Fprintln(w)
		}
	}
}
