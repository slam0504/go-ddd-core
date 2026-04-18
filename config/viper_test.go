package config_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/slam0504/go-ddd-core/config"
)

const sampleYAML = `
app:
  name: svc
  env: test
  version: 0.0.1
http:
  addr: ":9000"
  read_timeout: 3s
messaging:
  driver: kafka
  brokers: [localhost:9092]
`

func writeYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleYAML), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestViperProvider_LoadYAML(t *testing.T) {
	p, err := config.NewViperProvider(config.ViperOptions{
		Sources: []config.Source{{Type: "file", Path: writeYAML(t)}},
	})
	if err != nil {
		t.Fatalf("NewViperProvider: %v", err)
	}

	var cfg config.Root
	if err := p.Load(context.Background(), &cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.App.Name != "svc" || cfg.App.Env != "test" {
		t.Fatalf("app = %+v", cfg.App)
	}
	if cfg.HTTP.Addr != ":9000" || cfg.HTTP.ReadTimeout.String() != "3s" {
		t.Fatalf("http = %+v", cfg.HTTP)
	}
	if cfg.Messaging.Driver != "kafka" || len(cfg.Messaging.Brokers) != 1 {
		t.Fatalf("messaging = %+v", cfg.Messaging)
	}
}

func TestViperProvider_GetPath(t *testing.T) {
	p, err := config.NewViperProvider(config.ViperOptions{
		Sources: []config.Source{{Type: "file", Path: writeYAML(t)}},
	})
	if err != nil {
		t.Fatalf("NewViperProvider: %v", err)
	}

	if got := p.Get("messaging.driver"); got != "kafka" {
		t.Fatalf("Get = %v, want kafka", got)
	}
}

func TestViperProvider_UnknownSourceType(t *testing.T) {
	_, err := config.NewViperProvider(config.ViperOptions{
		Sources: []config.Source{{Type: "nonsense"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown source type")
	}
}
