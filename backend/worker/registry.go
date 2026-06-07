package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	workerKeyPrefix = "workers:"
	heartbeatTTL    = 6 * time.Second // a key expires if an instance stops heartbeating
	heartbeatEvery  = 2 * time.Second
)

// InstanceWorkers is one backend instance's worker snapshot — stored in Redis and
// returned to the dashboard.
type InstanceWorkers struct {
	Instance string         `json:"instance"`
	Workers  []WorkerStatus `json:"workers"`
}

// Registry makes the worker panel work across MANY backend instances. Each instance
// heartbeats its own worker state into Redis under "workers:<instanceID>"; any instance
// can then read back EVERY instance's state. So the dashboard shows all workers across
// all backends, no matter which instance served the request. A dead instance's key
// simply expires (heartbeatTTL) and it drops off the panel.
type Registry struct {
	rdb        *redis.Client
	pool       *Pool
	instanceID string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewRegistry(rdb *redis.Client, pool *Pool) *Registry {
	return &Registry{rdb: rdb, pool: pool, instanceID: pool.InstanceID()}
}

// Start begins heartbeating this instance's worker state to Redis.
func (r *Registry) Start() {
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.wg.Add(1)
	go r.loop()
	slog.Info("worker registry started", "instance", r.instanceID)
}

// Stop ends the heartbeat and removes this instance from the panel immediately.
func (r *Registry) Stop() {
	r.cancel()
	r.wg.Wait()
	_ = r.rdb.Del(context.Background(), workerKeyPrefix+r.instanceID).Err()
	slog.Info("worker registry stopped", "instance", r.instanceID)
}

func (r *Registry) loop() {
	defer r.wg.Done()
	t := time.NewTicker(heartbeatEvery)
	defer t.Stop()

	r.publish() // publish once immediately so we appear right away
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-t.C:
			r.publish()
		}
	}
}

// publish writes this instance's current worker snapshot to Redis with a TTL.
func (r *Registry) publish() {
	snap := InstanceWorkers{Instance: r.instanceID, Workers: r.pool.WorkerStates()}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	if err := r.rdb.Set(r.ctx, workerKeyPrefix+r.instanceID, data, heartbeatTTL).Err(); err != nil {
		slog.Error("worker registry publish", "err", err)
	}
}

// AllInstances reads every instance's snapshot from Redis, sorted by instance id.
// (Keys() is fine for a handful of instances; SCAN would be the move at large scale.)
func (r *Registry) AllInstances(ctx context.Context) ([]InstanceWorkers, error) {
	keys, err := r.rdb.Keys(ctx, workerKeyPrefix+"*").Result()
	if err != nil {
		return nil, err
	}

	out := make([]InstanceWorkers, 0, len(keys))
	for _, k := range keys {
		data, err := r.rdb.Get(ctx, k).Bytes()
		if err != nil {
			continue // key may have just expired
		}
		var iw InstanceWorkers
		if json.Unmarshal(data, &iw) == nil {
			out = append(out, iw)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Instance < out[j].Instance })
	return out, nil
}
