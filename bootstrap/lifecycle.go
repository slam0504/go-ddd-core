package bootstrap

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Hook is a unit of startup or shutdown work registered against the App.
type Hook struct {
	Name string
	Run  func(ctx context.Context) error
}

// Lifecycle coordinates ordered startup and reverse-order shutdown.
// Registration order determines execution order on start; shutdown runs
// in the reverse order to mirror dependency direction.
type Lifecycle struct {
	starts []Hook
	stops  []Hook
}

// Append adds a paired start/stop hook. Either may be nil.
func (l *Lifecycle) Append(start, stop Hook) {
	if start.Run != nil {
		l.starts = append(l.starts, start)
	}
	if stop.Run != nil {
		l.stops = append(l.stops, stop)
	}
}

// Start runs all registered start hooks in order, stopping on the first error.
func (l *Lifecycle) Start(ctx context.Context) error {
	for _, h := range l.starts {
		if err := h.Run(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop runs all registered stop hooks in reverse order, accumulating errors.
func (l *Lifecycle) Stop(ctx context.Context) error {
	var errs []error
	for i := len(l.stops) - 1; i >= 0; i-- {
		if err := l.stops[i].Run(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// WaitForSignal blocks until SIGINT, SIGTERM, or ctx cancellation. It
// returns the triggering signal or nil if ctx was cancelled first.
func WaitForSignal(ctx context.Context) os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(ch)

	select {
	case <-ctx.Done():
		return nil
	case s := <-ch:
		return s
	}
}

// ShutdownContext returns a context with the given timeout used for graceful
// shutdown. If timeout is zero, it returns ctx and a no-op cancel.
func ShutdownContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}
