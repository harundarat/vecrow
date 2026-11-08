package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/harundarat/vecrow/backend/internal/infrastructure/internal/retry"
)

// Config configures the Redis connection.
type Config struct {
	// URL in the form redis://[user:password@]host:port/db (rediss:// for TLS).
	URL string
	// StartupTimeout bounds how long OpenRedis waits for the server.
	StartupTimeout time.Duration
}

// Redis implements Store on top of a Redis server.
type Redis struct {
	client *goredis.Client
}

var _ Store = (*Redis)(nil)

// OpenRedis connects to Redis and waits until the server answers a ping.
func OpenRedis(ctx context.Context, cfg Config, log *slog.Logger) (*Redis, error) {
	log = log.With(slog.String("component", Name))

	opts, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		// The parse error may echo the URL, including its password.
		return nil, errors.New("cache: invalid CACHE_URL (expected redis://[user:password@]host:port/db)")
	}
	goredis.SetLogger(redisLogger{log: log})

	client := goredis.NewClient(opts)
	ping := func(ctx context.Context) error { return client.Ping(ctx).Err() }
	if err := retry.Until(ctx, log, Name, cfg.StartupTimeout, ping); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("cache: %w", err)
	}

	log.InfoContext(ctx, "cache connected",
		slog.String("addr", opts.Addr),
		slog.Int("db", opts.DB),
		slog.Int("pool_size", client.Options().PoolSize),
	)
	return &Redis{client: client}, nil
}

func (r *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cache: get: %w", err)
	}
	return b, nil
}

func (r *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := r.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set: %w", err)
	}
	return nil
}

func (r *Redis) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	ok, err := r.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("cache: setnx: %w", err)
	}
	return ok, nil
}

func (r *Redis) GetDel(ctx context.Context, key string) ([]byte, error) {
	b, err := r.client.GetDel(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cache: getdel: %w", err)
	}
	return b, nil
}

func (r *Redis) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache: delete: %w", err)
	}
	return nil
}

func (r *Redis) Name() string { return Name }

func (r *Redis) Check(ctx context.Context) error { return r.client.Ping(ctx).Err() }

func (r *Redis) Close() error { return r.client.Close() }

// redisLogger routes the go-redis client's internal messages (reconnects,
// pool issues) to slog.
type redisLogger struct {
	log *slog.Logger
}

func (l redisLogger) Printf(ctx context.Context, format string, v ...any) {
	l.log.WarnContext(ctx, "cache client message", slog.String("detail", fmt.Sprintf(format, v...)))
}
