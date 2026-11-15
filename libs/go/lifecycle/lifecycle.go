// Package lifecycle runs a service's long-lived components (servers,
// consumers, workers) and shuts them down gracefully (spec 8.2).
//
// Order of events on SIGTERM:
//  1. readiness flips to "not ready", so Kubernetes stops routing traffic;
//  2. every component's Shutdown is called with one shared deadline: servers
//     stop accepting and finish in-flight requests;
//  3. Run returns once all components have stopped, or when the deadline
//     passes.
//
// If any component fails, the others are shut down the same way and Run
// returns that error, so the process exits non-zero and is restarted.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// Component is a long-running part of a service.
type Component interface {
	// Name is used in logs.
	Name() string
	// Run blocks until the component stops. It must return nil after a
	// successful Shutdown.
	Run(ctx context.Context) error
	// Shutdown stops accepting new work and waits for in-flight work, until
	// ctx expires.
	Shutdown(ctx context.Context) error
}

// Readiness is shared by the admin /readyz endpoint and the gRPC health
// service. It reports not-ready from the moment shutdown starts.
type Readiness struct {
	draining atomic.Bool
}

// Draining reports whether shutdown has started.
func (r *Readiness) Draining() bool { return r.draining.Load() }

func (r *Readiness) startDraining() { r.draining.Store(true) }

// Runner runs components until a signal, a failure or context cancellation.
type Runner struct {
	Log             *slog.Logger
	ShutdownTimeout time.Duration
	Readiness       *Readiness
	// OnDrain is called right after readiness flips (e.g. set gRPC health
	// to NOT_SERVING). Optional.
	OnDrain func()
}

// Run starts all components and blocks until they have stopped. It listens
// for SIGTERM and SIGINT.
func (r Runner) Run(ctx context.Context, components ...Component) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return r.run(ctx, components...)
}

func (r Runner) run(ctx context.Context, components ...Component) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, c := range components {
		g.Go(func() error {
			r.Log.Info("component starting", slog.String("component", c.Name()))
			// Run gets a context that is not cancelled on shutdown: stopping
			// is driven by Shutdown, so in-flight work is not cut off.
			if err := c.Run(context.WithoutCancel(gctx)); err != nil {
				return fmt.Errorf("%s: %w", c.Name(), err)
			}
			r.Log.Info("component stopped", slog.String("component", c.Name()))
			return nil
		})
	}

	g.Go(func() error {
		<-gctx.Done() // signal, parent cancellation, or a component failed
		r.Log.Info("shutting down", slog.Duration("timeout", r.ShutdownTimeout))
		if r.Readiness != nil {
			r.Readiness.startDraining()
		}
		if r.OnDrain != nil {
			r.OnDrain()
		}
		sctx, cancel := context.WithTimeout(context.WithoutCancel(gctx), r.ShutdownTimeout)
		defer cancel()

		// Shut down concurrently: all components share one deadline.
		var sg errgroup.Group
		for _, c := range components {
			sg.Go(func() error {
				if err := c.Shutdown(sctx); err != nil {
					return fmt.Errorf("shutdown %s: %w", c.Name(), err)
				}
				return nil
			})
		}
		return sg.Wait() //nolint:wrapcheck // already wrapped per component
	})

	err := g.Wait()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err //nolint:wrapcheck // already wrapped per component
}
