package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type SortedSetCounter struct {
	client *redis.Client
}

const (
	sortedSetMax = "+inf"
	sortedSetMin = "-inf"
)

var _ Strategy = &SortedSetCounter{}

func NewSortedSetCounterStrategy(client *redis.Client) *SortedSetCounter {
	return &SortedSetCounter{client: client}
}

func (s *SortedSetCounter) Run(ctx context.Context, r Request) (Result, error) {
	var (
		now       = time.Now().UTC()
		expiresAt = now.Add(r.Duration)
		minimum   = now.Add(-r.Duration)
		res       = Result{
			State:     Deny,
			ExpiresAt: expiresAt,
		}
	)

	// If we already have more requests than allowed per key,
	// we can deny the request immediately
	c, err := s.client.ZCount(ctx, r.Key, strconv.FormatInt(minimum.UnixMilli(), 10), sortedSetMax).Uint64()
	if err == nil && c >= r.Limit {
		res.TotalRequests = c
		return res, nil
	}

	p := s.client.Pipeline()

	// we remove all already expired requests (below the low timestamp)
	removeOldest := p.ZRemRangeByScore(ctx, r.Key, "0", strconv.FormatInt(minimum.UnixMilli(), 10))

	// we add the current request
	add := p.ZAdd(ctx, r.Key, &redis.Z{
		Score:  float64(now.UnixMilli()),
		Member: uuid.New().String(),
	})

	// then count how many non expired requests there are
	count := p.ZCount(ctx, r.Key, sortedSetMin, sortedSetMax)

	if _, err = p.Exec(ctx); err != nil {
		return res, fmt.Errorf("failed to execute sorted set pipeline for key %q: %w", r.Key, err)
	}

	if err = removeOldest.Err(); err != nil {
		return res, fmt.Errorf("failed to remove oldest items for key %q: %w", r.Key, err)
	}

	if err = add.Err(); err != nil {
		return res, fmt.Errorf("failed to add item for key %q: %w", r.Key, err)
	}

	total, err := count.Result()
	if err != nil {
		return res, fmt.Errorf("failed to count items for key %q: %w", r.Key, err)
	}

	res.TotalRequests = uint64(total)

	if res.TotalRequests > r.Limit {
		return res, nil
	}

	res.State = Allow

	return res, nil
}
