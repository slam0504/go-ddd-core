package bootstrap

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/slam0504/go-ddd-core/config"
	"github.com/slam0504/go-ddd-core/ports/logger"
)

// App is the runtime container wiring configuration, logger, adapters, and
// modules together. Services construct an App, attach modules, then call Run.
type App struct {
	cfg       config.Provider
	log       logger.Logger
	registry  *AdapterRegistry
	lifecycle *Lifecycle

	shutdownTimeout time.Duration

	mu      sync.RWMutex
	modules []Module
	values  map[string]any
}

// Options configures App construction.
type Options struct {
	Config          config.Provider
	Logger          logger.Logger
	Registry        *AdapterRegistry
	ShutdownTimeout time.Duration
}

// New builds an App from opts. If Registry is nil, a fresh one is created.
func New(opts Options) *App {
	reg := opts.Registry
	if reg == nil {
		reg = NewAdapterRegistry()
	}
	timeout := opts.ShutdownTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &App{
		cfg:             opts.Config,
		log:             opts.Logger,
		registry:        reg,
		lifecycle:       &Lifecycle{},
		shutdownTimeout: timeout,
		values:          map[string]any{},
	}
}

// Config exposes the injected configuration Provider.
func (a *App) Config() config.Provider { return a.cfg }

// Logger exposes the injected Logger.
func (a *App) Logger() logger.Logger { return a.log }

// Registry exposes the AdapterRegistry so modules can register or resolve
// adapters during Start.
func (a *App) Registry() *AdapterRegistry { return a.registry }

// Lifecycle exposes the underlying lifecycle so modules can append hooks
// that are not tied to a specific Module.
func (a *App) Lifecycle() *Lifecycle { return a.lifecycle }

// Set publishes a named value into the container. Modules typically set the
// services they produce (e.g. "http.server") so later modules can retrieve them.
func (a *App) Set(key string, value any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.values[key] = value
}

// Get retrieves a named value from the container.
func (a *App) Get(key string) (any, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	v, ok := a.values[key]
	return v, ok
}

// Use appends modules to the App. Modules start in registration order.
func (a *App) Use(modules ...Module) *App {
	a.modules = append(a.modules, modules...)
	return a
}

// Start boots all modules in order. Callers typically prefer Run which also
// blocks on signals and orchestrates shutdown.
func (a *App) Start(ctx context.Context) error {
	for _, m := range a.modules {
		name := m.Name()
		if err := m.Start(ctx, a); err != nil {
			return fmt.Errorf("bootstrap: start module %q: %w", name, err)
		}
		mod := m
		a.lifecycle.Append(Hook{}, Hook{
			Name: name,
			Run:  func(sctx context.Context) error { return mod.Stop(sctx) },
		})
	}
	return a.lifecycle.Start(ctx)
}

// Stop runs lifecycle shutdown within the configured timeout.
func (a *App) Stop(parent context.Context) error {
	ctx, cancel := ShutdownContext(parent, a.shutdownTimeout)
	defer cancel()
	return a.lifecycle.Stop(ctx)
}

// Run starts the App and blocks until SIGINT/SIGTERM or ctx cancellation,
// then performs a graceful shutdown.
func (a *App) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		_ = a.Stop(ctx)
		return err
	}
	WaitForSignal(ctx)
	return a.Stop(ctx)
}
