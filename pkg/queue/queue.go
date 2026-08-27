package queue

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const (
	PrioritiesKey = "job_queue:priorities"
	ListPrefix    = "job_queue:priority:"
	QueuedJobsSet = "queued_jobs"
	MinPriority   = -1000
	MaxPriority   = 1000
)

var enqueueScript = redis.NewScript(`
if redis.call("SADD", KEYS[3], ARGV[1]) == 1 then
  redis.call("RPUSH", KEYS[2], ARGV[1])
  redis.call("ZADD", KEYS[1], ARGV[2], ARGV[2])
  return 1
end
return 0
`)

var popScript = redis.NewScript(`
local tier = redis.call("ZREVRANGE", KEYS[1], 0, 0)[1]
if not tier then return "" end
local list = ARGV[1] .. tier
local job = redis.call("LPOP", list)
if not job then
  redis.call("ZREM", KEYS[1], tier)
  return ""
end
redis.call("SREM", KEYS[2], job)
if redis.call("LLEN", list) == 0 then
  redis.call("ZREM", KEYS[1], tier)
end
return job
`)

func ValidatePriority(priority int) error {
	if priority < MinPriority || priority > MaxPriority {
		return fmt.Errorf("priority must be between %d and %d", MinPriority, MaxPriority)
	}
	return nil
}

func Enqueue(ctx context.Context, client *redis.Client, jobID string, priority int) error {
	if err := ValidatePriority(priority); err != nil {
		return err
	}
	_, err := enqueueScript.Run(ctx, client, []string{PrioritiesKey, ListPrefix + fmt.Sprint(priority), QueuedJobsSet}, jobID, priority).Result()
	return err
}

func Pop(ctx context.Context, client *redis.Client) (string, error) {
	value, err := popScript.Run(ctx, client, []string{PrioritiesKey, QueuedJobsSet}, ListPrefix).Result()
	if err != nil {
		return "", err
	}
	jobID, ok := value.(string)
	if !ok || jobID == "" {
		return "", redis.Nil
	}
	return jobID, nil
}
