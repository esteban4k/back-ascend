// Package config reads the server configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Store selects the persistence adapter.
type Store string

const (
	StoreSQLite Store = "sqlite"
	StoreMemory Store = "memory"
)

// Seed selects the data loaded into an empty store.
type Seed string

const (
	// SeedDemo loads ~6 months of realistic history, like the web client's mock.
	SeedDemo Seed = "demo"
	// SeedBlank creates only the user, the default life domains and the origin milestone.
	SeedBlank Seed = "blank"
)

// Config is everything the server needs to start.
type Config struct {
	Addr           string
	Store          Store
	DBPath         string
	Location       *time.Location
	Seed           Seed
	UserName       string
	AllowReset     bool
	AllowedOrigins []string
	APIToken       string
	LogLevel       slog.Level
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

// FromEnv reads the configuration. See README.md for every variable.
func FromEnv() (Config, error) {
	c := Config{
		Addr:     env("ASCEND_ADDR", ":8080"),
		Store:    Store(strings.ToLower(env("ASCEND_STORE", string(StoreSQLite)))),
		DBPath:   env("ASCEND_DB_PATH", "data/ascend.db"),
		Seed:     Seed(strings.ToLower(env("ASCEND_SEED", string(SeedDemo)))),
		UserName: env("ASCEND_USER_NAME", ""),
		APIToken: env("ASCEND_API_TOKEN", ""),
	}
	if port := env("PORT", ""); port != "" {
		c.Addr = ":" + port
	}

	switch c.Store {
	case StoreSQLite, StoreMemory:
	default:
		return Config{}, fmt.Errorf("ASCEND_STORE must be %q or %q, got %q", StoreSQLite, StoreMemory, c.Store)
	}
	switch c.Seed {
	case SeedDemo, SeedBlank:
	default:
		return Config{}, fmt.Errorf("ASCEND_SEED must be %q or %q, got %q", SeedDemo, SeedBlank, c.Seed)
	}

	tz := env("ASCEND_TIMEZONE", "Local")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return Config{}, fmt.Errorf("ASCEND_TIMEZONE: %w", err)
	}
	c.Location = loc

	c.AllowReset, err = strconv.ParseBool(env("ASCEND_ALLOW_RESET", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("ASCEND_ALLOW_RESET: %w", err)
	}

	for _, o := range strings.Split(env("ASCEND_CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}

	if err := c.LogLevel.UnmarshalText([]byte(env("ASCEND_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("ASCEND_LOG_LEVEL: %w", err)
	}
	return c, nil
}
