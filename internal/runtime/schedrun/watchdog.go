package schedrun

import (
	"bufio"
	"io"
	"os"
	"strings"
	"time"
)

// GoLine is what the supervisor writes once the run is recorded as running.
const GoLine = "go"

// StartTimeout is how long a child waits to be released before it gives up.
const StartTimeout = time.Minute

// SupervisorPipe reports whether f is the pipe a supervisor keeps open for the
// child. A terminal or a file is not one: it would either never end or end at
// once, and neither says anything about a supervisor. A socket counts, as it
// does for the Studio host's own watchdog.
func SupervisorPipe(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&(os.ModeNamedPipe|os.ModeSocket) != 0
}

// AwaitGo blocks until the supervisor releases the run. It returns a channel
// that closes when the supervisor's end of the stream does, whether the
// supervisor exited or crashed; nothing else says that on every platform. The
// child does no work before AwaitGo returns, so a run the store already settled
// spends nothing.
func AwaitGo(stdin io.Reader, timeout time.Duration) (gone <-chan struct{}, err error) {
	br := bufio.NewReader(stdin)
	line := make(chan string, 1)
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		s, rerr := br.ReadString('\n')
		if rerr != nil {
			line <- ""
			return
		}
		line <- strings.TrimSpace(s)
		_, _ = io.Copy(io.Discard, br)
	}()
	select {
	case s := <-line:
		if s != GoLine {
			return nil, ErrParentGone
		}
		return ended, nil
	case <-time.After(timeout):
		return nil, ErrStartTimeout
	}
}
