// Package httpsrv is a minimal net/http adapter satisfying transport/http.Server.
// It is only for the example; production services should use a fully featured
// mux adapter (chi, gin, echo, etc.) in their own infra layer.
package httpsrv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Server wraps net/http.Server and exposes the ServeMux for route registration.
type Server struct {
	addr            string
	mux             *http.ServeMux
	srv             *http.Server
	shutdownTimeout time.Duration
	errCh           chan error
}

// New constructs a Server bound to addr.
func New(addr string, shutdownTimeout time.Duration) *Server {
	mux := http.NewServeMux()
	return &Server{
		addr:            addr,
		mux:             mux,
		srv:             &http.Server{Addr: addr, Handler: mux},
		shutdownTimeout: shutdownTimeout,
		errCh:           make(chan error, 1),
	}
}

// Mux exposes the underlying mux for route registration.
func (s *Server) Mux() *http.ServeMux { return s.mux }

// Start begins listening in a goroutine. Listener failures (port in use,
// bind errors) are surfaced both via stderr and the Err channel so callers
// can react during the lifecycle.
func (s *Server) Start(_ context.Context) error {
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "httpsrv: listen on %s failed: %v\n", s.addr, err)
			s.errCh <- err
			return
		}
		close(s.errCh)
	}()
	return nil
}

// Err returns a channel that receives the first fatal listener error and is
// then closed. Closed without a value means shutdown completed cleanly.
func (s *Server) Err() <-chan error { return s.errCh }

// Stop gracefully shuts the server down.
func (s *Server) Stop(ctx context.Context) error {
	sctx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()
	return s.srv.Shutdown(sctx)
}

// Addr returns the bound address.
func (s *Server) Addr() string { return s.addr }
