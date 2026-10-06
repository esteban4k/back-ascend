// Package container is the composition root: the only place that knows
// which adapters are in use. Nothing in `domain/` or `application/` changes
// when an adapter is swapped.
package container

import (
	"context"
	"fmt"
	"time"

	"ascend/internal/application"
	"ascend/internal/application/ports"
	"ascend/internal/infrastructure/config"
	"ascend/internal/infrastructure/memory"
	"ascend/internal/infrastructure/seed"
	"ascend/internal/infrastructure/sqlite"
	"ascend/internal/infrastructure/system"
)

// Store is what the composition root needs from a persistence adapter.
type Store interface {
	Ports(clock ports.Clock, ids ports.IDGenerator) ports.Ports
	Load(ctx context.Context, data seed.Dataset) error
	Empty(ctx context.Context) (bool, error)
	Close() error
}

// Container holds the wired application.
type Container struct {
	UseCases *application.UseCases
	// DataSource describes the storage in use.
	DataSource string
	// Reset restores the initial data.
	Reset func(ctx context.Context) error
	// Ping checks the storage is reachable.
	Ping  func(ctx context.Context) error
	Close func() error
}

// Build opens the configured store, seeds it when empty and binds the use cases.
func Build(ctx context.Context, cfg config.Config) (*Container, error) {
	clock := system.NewClock(cfg.Location)
	var (
		store      Store
		dataSource string
		ping       = func(context.Context) error { return nil }
	)
	switch cfg.Store {
	case config.StoreMemory:
		store, dataSource = memory.New(), "Memory · process lifetime"
	default:
		db, err := sqlite.Open(ctx, cfg.DBPath)
		if err != nil {
			return nil, err
		}
		store, dataSource, ping = db, "SQLite · "+cfg.DBPath, db.Ping
	}

	initial := func() seed.Dataset {
		if cfg.Seed == config.SeedBlank {
			return seed.Blank(clock.Now(), cfg.UserName)
		}
		return seed.Demo(clock.Now())
	}
	empty, err := store.Empty(ctx)
	if err != nil {
		store.Close()
		return nil, err
	}
	if empty {
		if err := store.Load(ctx, initial()); err != nil {
			store.Close()
			return nil, fmt.Errorf("seed %s data: %w", cfg.Seed, err)
		}
	}

	c := &Container{
		UseCases:   application.New(store.Ports(clock, system.UUIDGenerator{})),
		DataSource: dataSource,
		Ping:       ping,
		Close:      store.Close,
	}
	if cfg.AllowReset {
		c.Reset = func(ctx context.Context) error { return store.Load(ctx, initial()) }
	}
	return c, nil
}

// ForTest wires an in-memory store loaded with `data` and a fixed clock.
// Ids are sequential (`id-1`, `id-2`, …) so assertions are predictable.
func ForTest(data seed.Dataset, now time.Time) (*application.UseCases, ports.Ports) {
	store := memory.New()
	_ = store.Load(context.Background(), data) // the memory store never fails to load
	p := store.Ports(system.FixedClock{At: now}, &sequence{})
	return application.New(p), p
}

type sequence struct{ n int }

func (s *sequence) Next() string {
	s.n++
	return fmt.Sprintf("id-%d", s.n)
}
