package runtime

import (
	"context"
	"time"
)

type Options struct {
	BaseURL string
	Debug   bool
	MCPURL  string
	Pretty  bool
	Timeout time.Duration
}

type optionsKey struct{}

func WithOptions(ctx context.Context, options *Options) context.Context {
	return context.WithValue(ctx, optionsKey{}, options)
}

func FromContext(ctx context.Context) *Options {
	options, ok := ctx.Value(optionsKey{}).(*Options)
	if !ok || options == nil {
		return &Options{Timeout: 30 * time.Second}
	}

	return options
}
