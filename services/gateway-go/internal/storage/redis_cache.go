package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisCaseCache struct {
	client    redis.Cmdable
	keyPrefix string
}

// NewRedisCaseCache creates a Redis-backed case cache used by repository lookup.
func NewRedisCaseCache(client redis.Cmdable) CaseCache {
	return &redisCaseCache{
		client:    client,
		keyPrefix: "risk:case:",
	}
}

func (c *redisCaseCache) GetCase(ctx context.Context, caseID string) (Case, bool, error) {
	payload, err := c.client.Get(ctx, c.key(caseID)).Result()
	if err != nil {
		if err == redis.Nil {
			return Case{}, false, nil
		}
		return Case{}, false, err
	}

	var out Case
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return Case{}, false, err
	}
	return out, true, nil
}

func (c *redisCaseCache) SetCase(ctx context.Context, in Case, ttl time.Duration) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(in.CaseID), payload, ttl).Err()
}

func (c *redisCaseCache) key(caseID string) string {
	return fmt.Sprintf("%s%s", c.keyPrefix, caseID)
}
