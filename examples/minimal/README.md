# minimal example

A runnable demonstration that wires `go-ddd-core` into a tiny HTTP service.

## What it shows

- Loading yaml + env config via `config.ViperProvider`
- A domain aggregate (`Order`) built on `domain.BaseAggregate`
- CQRS handlers using `application/command` and `application/query` with
  the bundled `InMemoryBus` implementations
- A stdlib-backed HTTP server adapter satisfying `transport/http`
- An `slog`-backed logger adapter satisfying `ports/logger`
- An in-memory repository satisfying `domain.Repository`
- Bootstrap lifecycle with graceful shutdown (SIGINT/SIGTERM)

## Run

```bash
go run ./examples/minimal/cmd
```

The server listens on `:8080`. Example requests:

```bash
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"order_id":"o1","customer_id":"c1","items":[{"sku":"a","quantity":2,"price_cents":500}]}'

curl http://localhost:8080/orders/o1
```

## Layout

```
examples/minimal/
├── cmd/main.go                      # wiring + HTTP routes
├── config.yaml                      # config consumed by ViperProvider
├── domain/order/                    # Order aggregate, events, repo contract
├── application/order/               # PlaceOrderHandler, GetOrderHandler
└── infra/
    ├── httpsrv/                     # transport/http adapter (stdlib)
    ├── memrepo/                     # in-memory repository
    └── slogger/                     # logger adapter (slog)
```

The `infra/` adapters are deliberately trivial. Production services would
replace them with real implementations (Kafka, Postgres, etc.) living in
their own repos.
