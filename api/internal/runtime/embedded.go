package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alicebob/miniredis/v2"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/redis/go-redis/v9"

	"github.com/dfradehubs/agentgram-api/internal/config"
)

// Resources holds in-process stores started for laptop mode.
type Resources struct {
	Redis    *miniredis.Miniredis
	Postgres *embeddedpostgres.EmbeddedPostgres
}

// Close shuts down embedded stores.
func (r *Resources) Close() {
	if r == nil {
		return
	}
	if r.Redis != nil {
		r.Redis.Close()
	}
	if r.Postgres != nil {
		_ = r.Postgres.Stop()
	}
}

// StartRedis starts miniredis when redis.embedded is set. The config Addr is rewritten.
func StartRedis(cfg *config.Config) (*miniredis.Miniredis, *redis.Client, error) {
	if !cfg.Redis.Embedded {
		rdb := redis.NewClient(&redis.Options{
			Addr:         cfg.Redis.Addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		})
		return nil, rdb, nil
	}
	mr, err := miniredis.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("start embedded redis: %w", err)
	}
	cfg.Redis.Addr = mr.Addr()
	rdb := redis.NewClient(&redis.Options{
		Addr:     mr.Addr(),
		PoolSize: cfg.Redis.PoolSize,
	})
	return mr, rdb, nil
}

// StartPostgres starts a private PostgreSQL when database.embedded is true.
// Data lives under $AGENTGRAM_DATA_DIR or ./data/postgres.
func StartPostgres(cfg *config.Config) (*embeddedpostgres.EmbeddedPostgres, error) {
	if !cfg.Database.Embedded {
		return nil, nil
	}
	dataDir := os.Getenv("AGENTGRAM_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(".", "data")
	}
	runtimeDir := filepath.Join(dataDir, "postgres")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return nil, fmt.Errorf("create postgres data dir: %w", err)
	}

	port := cfg.Database.Port
	if port == 0 || port == 5432 {
		port = 55432
	}
	user := cfg.Database.User
	if user == "" {
		user = "agentgram"
	}
	pass := cfg.Database.Password
	if pass == "" {
		pass = "agentgram"
	}
	name := cfg.Database.DBName
	if name == "" {
		name = "agentgram"
	}

	ep := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Username(user).
		Password(pass).
		Database(name).
		Port(uint32(port)).
		RuntimePath(runtimeDir).
		StartTimeout(60 * time.Second))
	if err := ep.Start(); err != nil {
		return nil, fmt.Errorf("start embedded postgres: %w", err)
	}

	cfg.Database.Host = "localhost"
	cfg.Database.Port = port
	cfg.Database.User = user
	cfg.Database.Password = pass
	cfg.Database.DBName = name
	cfg.Database.SSLMode = "disable"
	return ep, nil
}
