package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Source describes one configuration source. Multiple sources are merged in
// order: later sources override earlier ones.
type Source struct {
	// Type is one of "file", "env".
	Type string
	// Path is the file path (for file sources).
	Path string
	// EnvPrefix is the prefix for env-based overrides (for env sources).
	EnvPrefix string
	// Watch enables live reload for file sources.
	Watch bool
}

// ViperOptions configures a ViperProvider.
type ViperOptions struct {
	Sources []Source
	// EnvKeyReplacer maps config keys to env var names. Defaults to "." -> "_".
	EnvKeyReplacer *strings.Replacer
}

// ViperProvider is the default Provider backed by spf13/viper. It supports
// yaml files and environment variable overrides.
type ViperProvider struct {
	v         *viper.Viper
	listeners []func()
}

// NewViperProvider builds a ViperProvider from opts. The returned provider
// has already read all configured sources.
func NewViperProvider(opts ViperOptions) (*ViperProvider, error) {
	v := viper.New()
	if opts.EnvKeyReplacer == nil {
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	} else {
		v.SetEnvKeyReplacer(opts.EnvKeyReplacer)
	}

	p := &ViperProvider{v: v}

	for _, src := range opts.Sources {
		switch src.Type {
		case "file":
			v.SetConfigFile(src.Path)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("config: read file %q: %w", src.Path, err)
			}
			if src.Watch {
				v.OnConfigChange(func(_ fsnotify.Event) {
					for _, fn := range p.listeners {
						fn()
					}
				})
				v.WatchConfig()
			}
		case "env":
			if src.EnvPrefix != "" {
				v.SetEnvPrefix(src.EnvPrefix)
			}
			v.AutomaticEnv()
		default:
			return nil, fmt.Errorf("config: unknown source type %q", src.Type)
		}
	}

	return p, nil
}

// Load unmarshals the merged configuration into dest.
func (p *ViperProvider) Load(_ context.Context, dest any) error {
	if err := p.v.Unmarshal(dest); err != nil {
		return fmt.Errorf("config: unmarshal: %w", err)
	}
	return nil
}

// Get returns the raw value at path.
func (p *ViperProvider) Get(path string) any { return p.v.Get(path) }

// OnChange registers a callback invoked after a file-source reload.
func (p *ViperProvider) OnChange(fn func()) {
	p.listeners = append(p.listeners, fn)
}

// Viper exposes the underlying viper.Viper for advanced cases. Avoid using
// it from domain/application code — prefer the Provider interface.
func (p *ViperProvider) Viper() *viper.Viper { return p.v }
