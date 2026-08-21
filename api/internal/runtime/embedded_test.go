package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/dfradehubs/agentgram-api/internal/config"
)

func TestStartRedis_Embedded(t *testing.T) {
	cfg := &config.Config{Redis: config.RedisConfig{Embedded: true, PoolSize: 4}}
	mr, rdb, err := StartRedis(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if cfg.Redis.Addr == "" {
		t.Fatal("expected addr to be set")
	}
}
