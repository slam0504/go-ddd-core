// Package config defines the configuration contract and shared schema
// consumed by core modules. Downstream services extend these structs with
// their own fields via struct embedding.
package config

import "time"

// Root is the canonical config shape the bootstrap layer understands.
// Services may wrap this struct or mount it under a section of their own
// schema — the Provider is generic over the destination type.
type Root struct {
	App           App           `mapstructure:"app"`
	Logger        Logger        `mapstructure:"logger"`
	Observability Observability `mapstructure:"observability"`
	HTTP          HTTP          `mapstructure:"http"`
	GRPC          GRPC          `mapstructure:"grpc"`
	GraphQL       GraphQL       `mapstructure:"graphql"`
	Database      Database      `mapstructure:"database"`
	Cache         Cache         `mapstructure:"cache"`
	Messaging     Messaging     `mapstructure:"messaging"`
	Storage       Storage       `mapstructure:"storage"`
}

// App holds application identity.
type App struct {
	Name        string `mapstructure:"name"`
	Env         string `mapstructure:"env"`
	Version     string `mapstructure:"version"`
	InstanceID  string `mapstructure:"instance_id"`
	Description string `mapstructure:"description"`
}

// Logger selects the logger adapter and its behaviour.
type Logger struct {
	Driver string `mapstructure:"driver"` // e.g. "slog", "zap"
	Level  string `mapstructure:"level"`  // debug/info/warn/error
	Format string `mapstructure:"format"` // json/text
}

// Observability chooses the OTel exporter adapter.
type Observability struct {
	Driver       string  `mapstructure:"driver"` // e.g. "otlp", "noop"
	Endpoint     string  `mapstructure:"endpoint"`
	Insecure     bool    `mapstructure:"insecure"`
	SampleRatio  float64 `mapstructure:"sample_ratio"`
	ServiceName  string  `mapstructure:"service_name"`
	MetricPrefix string  `mapstructure:"metric_prefix"`
}

// HTTP configures the inbound HTTP server adapter.
type HTTP struct {
	Driver          string        `mapstructure:"driver"` // e.g. "stdlib", "chi"
	Addr            string        `mapstructure:"addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// GRPC configures the inbound gRPC server adapter.
type GRPC struct {
	Driver          string        `mapstructure:"driver"`
	Addr            string        `mapstructure:"addr"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// GraphQL configures the GraphQL adapter.
type GraphQL struct {
	Driver     string `mapstructure:"driver"` // e.g. "gqlgen", "graphql-go"
	Path       string `mapstructure:"path"`   // HTTP path to mount
	Playground bool   `mapstructure:"playground"`
}

// Database selects and configures a persistence adapter.
type Database struct {
	Driver          string        `mapstructure:"driver"` // postgres/mysql/...
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// Cache selects and configures a cache adapter.
type Cache struct {
	Driver    string        `mapstructure:"driver"` // redis/memory/...
	Endpoint  string        `mapstructure:"endpoint"`
	Namespace string        `mapstructure:"namespace"`
	TTL       time.Duration `mapstructure:"ttl"`
}

// Messaging selects a watermill driver (kafka, nats, redis, etc.).
type Messaging struct {
	Driver        string            `mapstructure:"driver"`
	Brokers       []string          `mapstructure:"brokers"`
	ConsumerGroup string            `mapstructure:"consumer_group"`
	Options       map[string]string `mapstructure:"options"`
}

// Storage selects an object storage adapter.
type Storage struct {
	Driver   string `mapstructure:"driver"` // s3/gcs/filesystem
	Endpoint string `mapstructure:"endpoint"`
	Bucket   string `mapstructure:"bucket"`
	Region   string `mapstructure:"region"`
}
