// Command minimal wires the core package into a runnable HTTP service.
// It demonstrates:
//   - loading yaml config through config.ViperProvider
//   - using application/command + application/query in-memory buses
//   - registering use-case handlers
//   - serving a tiny REST-over-HTTP surface via a stdlib adapter
//   - the bootstrap.App lifecycle with graceful shutdown
//
// The example deliberately avoids real infra (Kafka, Postgres, etc.) so it
// can be run without external services. Production services should provide
// their own adapters in an infra layer.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/slam0504/go-ddd-core/application/command"
	"github.com/slam0504/go-ddd-core/application/query"
	"github.com/slam0504/go-ddd-core/bootstrap"
	"github.com/slam0504/go-ddd-core/config"
	"github.com/slam0504/go-ddd-core/pkg/idgen"

	apporder "github.com/slam0504/go-ddd-core/examples/minimal/application/order"
	orderdom "github.com/slam0504/go-ddd-core/examples/minimal/domain/order"
	"github.com/slam0504/go-ddd-core/examples/minimal/infra/httpsrv"
	"github.com/slam0504/go-ddd-core/examples/minimal/infra/memrepo"
	"github.com/slam0504/go-ddd-core/examples/minimal/infra/slogger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = filepath.Join("examples", "minimal", "config.yaml")
	}

	provider, err := config.NewViperProvider(config.ViperOptions{
		Sources: []config.Source{
			{Type: "file", Path: cfgPath},
			{Type: "env", EnvPrefix: "APP"},
		},
	})
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	var cfg config.Root
	if err := provider.Load(ctx, &cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	log := slogger.New(os.Stdout, slog.LevelInfo)
	repo := memrepo.NewOrderRepository()
	ids := idgen.UUIDv7()

	cmdBus := command.NewInMemoryBus()
	qryBus := query.NewInMemoryBus()

	command.Register[apporder.PlaceOrderCommand, apporder.PlaceOrderResult](
		cmdBus,
		apporder.NewPlaceOrderHandler(repo, nil, "order.events", ids),
	)
	query.Register[apporder.GetOrderQuery, apporder.OrderView](
		qryBus,
		apporder.NewGetOrderHandler(repo),
	)

	httpServer := httpsrv.New(cfg.HTTP.Addr, cfg.HTTP.ShutdownTimeout)
	registerRoutes(httpServer.Mux(), cmdBus, qryBus)

	app := bootstrap.New(bootstrap.Options{
		Config:          provider,
		Logger:          log,
		ShutdownTimeout: cfg.HTTP.ShutdownTimeout,
	})
	app.Use(bootstrap.ModuleFunc{
		ModuleName: "http",
		StartFn: func(ctx context.Context, a *bootstrap.App) error {
			return httpServer.Start(ctx)
		},
		StopFn: func(ctx context.Context) error {
			return httpServer.Stop(ctx)
		},
	})

	return app.Run(ctx)
}

func registerRoutes(mux *http.ServeMux, cmdBus *command.InMemoryBus, qryBus *query.InMemoryBus) {
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			OrderID    string           `json:"order_id"`
			CustomerID string           `json:"customer_id"`
			Items      []orderdom.Item `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res, err := cmdBus.Dispatch(r.Context(), apporder.PlaceOrderCommand{
			OrderID:    body.OrderID,
			CustomerID: body.CustomerID,
			Items:      body.Items,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	})

	mux.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		res, err := qryBus.Dispatch(r.Context(), apporder.GetOrderQuery{OrderID: r.PathValue("id")})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
