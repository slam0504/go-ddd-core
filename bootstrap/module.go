package bootstrap

import "context"

// Module is a unit of application composition. Modules can publish values
// into the App container during Start and tear them down in Stop. Stop is
// called in reverse Start order.
type Module interface {
	Name() string
	Start(ctx context.Context, app *App) error
	Stop(ctx context.Context) error
}

// ModuleFunc adapts a pair of start/stop callbacks to the Module interface.
type ModuleFunc struct {
	ModuleName string
	StartFn    func(ctx context.Context, app *App) error
	StopFn     func(ctx context.Context) error
}

func (m ModuleFunc) Name() string { return m.ModuleName }

func (m ModuleFunc) Start(ctx context.Context, app *App) error {
	if m.StartFn == nil {
		return nil
	}
	return m.StartFn(ctx, app)
}

func (m ModuleFunc) Stop(ctx context.Context) error {
	if m.StopFn == nil {
		return nil
	}
	return m.StopFn(ctx)
}
