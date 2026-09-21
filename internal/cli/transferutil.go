package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// wallClock is the production Clock for transfer.Client keepalives.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
func (wallClock) NewTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// sigwinchChannel delivers struct{} on SIGWINCH so transfer.Connect can
// re-query the terminal size. The returned stop tears down the goroutine.
func sigwinchChannel(stdoutFd int) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sigCh:
				select {
				case ch <- struct{}{}:
				default:
				}
			case <-done:
				return
			}
		}
	}()
	stop := func() {
		signal.Stop(sigCh)
		close(done)
		close(ch)
	}
	return ch, stop
}
