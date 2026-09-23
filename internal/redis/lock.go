package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Locker provides atomic distributed locking across horizontally scaled worker instances.
type Locker struct {
	rdb      *redis.Client
	workerID string
}

// NewLocker creates a distributed lock manager.
func NewLocker(rdb *redis.Client, workerID string) *Locker {
	return &Locker{
		rdb:      rdb,
		workerID: workerID,
	}
}

// Acquire attempts to acquire a lock with a specific TTL.
// Returns true if the lock was acquired, false if held by another instance.
func (l *Locker) Acquire(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	if l.rdb == nil {
		return true, nil
	}
	// CRITICAL(atomic-lock): SET key worker_id NX PX ttl ensures atomic mutual exclusion without race conditions.
	return l.rdb.SetNX(ctx, "lock:"+lockKey, l.workerID, ttl).Result()
}

// Release safely releases the lock only if the current worker is the active holder.
func (l *Locker) Release(ctx context.Context, lockKey string) error {
	if l.rdb == nil {
		return nil
	}

	// CRITICAL(safe-release): Lua script ensures a worker never unlocks a key if its TTL expired
	// and was acquired by another replica.
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	return script.Run(ctx, l.rdb, []string{"lock:" + lockKey}, l.workerID).Err()
}
