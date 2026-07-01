// Package resolver turns a one-click-hoster share-page URL into something
// the HTTP engine can actually download, without scraping/protocol logic
// leaking into the engine itself. New hosts plug in by implementing
// Resolver, the same way download engines plug into task.Task — see
// TODO.md Phase 2 for the per-host difficulty breakdown and rationale.
package resolver

import (
	"context"
	"errors"
)

// Result is what a Resolver hands back to the HTTP engine.
type Result struct {
	// URLs are direct, fetchable download URLs, in preference order. Most
	// resolvers return exactly one; a host with multiple mirrors could return more.
	URLs []string
}

// Resolver turns a share-page URL into a Result. CanResolve reports whether
// a given URL belongs to this resolver at all, so a registry can dispatch
// without every resolver needing to know about every other one.
type Resolver interface {
	Name() string
	CanResolve(rawURL string) bool
	Resolve(ctx context.Context, rawURL string) (Result, error)
}

var ErrNotSupported = errors.New("resolver: no resolver matches this URL")

// Registry dispatches a URL to whichever registered Resolver claims it.
type Registry struct {
	resolvers []Resolver
}

func NewRegistry(resolvers ...Resolver) *Registry {
	return &Registry{resolvers: resolvers}
}

func (r *Registry) Resolve(ctx context.Context, rawURL string) (Result, error) {
	for _, res := range r.resolvers {
		if res.CanResolve(rawURL) {
			return res.Resolve(ctx, rawURL)
		}
	}
	return Result{}, ErrNotSupported
}
