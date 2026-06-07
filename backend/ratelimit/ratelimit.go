// Package ratelimit implements a per-client sliding-window rate limiter backed by
// Redis. Each client gets at most `max` requests per `window`; the (max+1)th within
// the window is denied.
//
// We use the "sliding window log" technique: every request is recorded in a Redis
// sorted set keyed by the client, with the request's timestamp as the score. To
// decide a new request we prune entries older than the window, count what's left, and
// admit only if we're under the limit. Unlike a fixed-window counter, there's no
// bucket boundary a client can exploit to burst 2x.
package ratelimit

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// slidingWindowScript does the whole decision ATOMICALLY inside Redis, so two
// simultaneous requests can't both read "count = max-1" and both get admitted. A Lua
// script in Redis runs to completion with nothing else interleaved.
//
//	KEYS[1] = the per-client key
//	ARGV[1] = now (ms)        ARGV[2] = window (ms)
//	ARGV[3] = max requests    ARGV[4] = a unique member for this request
//
// Returns the remaining allowance (>= 0) if admitted, or -1 if denied.
var slidingWindowScript = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local max    = tonumber(ARGV[3])
local member = ARGV[4]

-- slide the window forward: drop timestamps older than (now - window)
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)

-- how many requests are in the current window?
local count = redis.call('ZCARD', key)
if count < max then
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window)   -- auto-clean idle clients
    return max - count - 1               -- remaining after admitting this one
end
return -1                                -- over the limit: deny
`)

// allowNScript is the weighted version: it tries to consume N tokens at once (a batch
// of N tasks). All-or-nothing — if the N wouldn't fit under the limit, none are added.
// ARGV[5] is a unique base; we append :1..:n so every member is distinct.
var allowNScript = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local max    = tonumber(ARGV[3])
local n      = tonumber(ARGV[4])
local base   = ARGV[5]

redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
local count = redis.call('ZCARD', key)
if count + n <= max then
    for i = 1, n do
        redis.call('ZADD', key, now, base .. ':' .. i)
    end
    redis.call('PEXPIRE', key, window)
    return max - count - n          -- remaining after admitting the batch
end
return -1                           -- batch rejected
`)

// Limiter holds the Redis client and the policy (max requests per window).
type Limiter struct {
	rdb    *redis.Client
	max    int
	window time.Duration
}

// NewLimiter builds a limiter. max < 1 is clamped to 1.
func NewLimiter(rdb *redis.Client, max int, window time.Duration) *Limiter {
	if max < 1 {
		max = 1
	}
	return &Limiter{rdb: rdb, max: max, window: window}
}

// Allow reports whether a request identified by id (we use client_id) is within the
// limit right now. remaining is how many more requests the client may make in the
// current window (0 means this was the last allowed one). A non-nil error means Redis
// itself failed — the caller decides how to handle that (we fail open).
func (l *Limiter) Allow(ctx context.Context, id string) (allowed bool, remaining int, err error) {
	now := time.Now().UnixMilli()
	// The member must be unique per request, or ZADD would overwrite a same-timestamp
	// entry and undercount. timestamp + random gives uniqueness.
	member := fmt.Sprintf("%d-%d", now, rand.Int63())
	key := "ratelimit:" + id

	res, err := slidingWindowScript.Run(ctx, l.rdb,
		[]string{key},
		now, l.window.Milliseconds(), l.max, member,
	).Int()
	if err != nil {
		return false, 0, err
	}
	if res < 0 {
		return false, 0, nil // denied
	}
	return true, res, nil
}

// AllowN tries to consume n tokens at once (a batch of n tasks). All-or-nothing: if the
// n wouldn't fit under the limit, none are consumed and it's denied. remaining is how
// many more tokens are left in the window.
func (l *Limiter) AllowN(ctx context.Context, id string, n int) (allowed bool, remaining int, err error) {
	if n < 1 {
		n = 1
	}
	now := time.Now().UnixMilli()
	base := fmt.Sprintf("%d-%d", now, rand.Int63())
	key := "ratelimit:" + id

	res, err := allowNScript.Run(ctx, l.rdb,
		[]string{key},
		now, l.window.Milliseconds(), l.max, n, base,
	).Int()
	if err != nil {
		return false, 0, err
	}
	if res < 0 {
		return false, 0, nil // the batch was rejected
	}
	return true, res, nil
}

// Max returns the configured ceiling (for response headers / messages).
func (l *Limiter) Max() int { return l.max }
