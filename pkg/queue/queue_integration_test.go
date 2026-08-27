package queue

import (
	"context"
	"os"
	"testing"

	redisclient "github.com/redis/go-redis/v9"
)

func TestRedisQueuePrioritizesAndPreservesFIFO(t *testing.T) {
	url := os.Getenv("TRANSCODEX_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set TRANSCODEX_TEST_REDIS_URL to run Redis integration tests")
	}
	options, err := redisclient.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redisclient.NewClient(options)
	defer client.Close()
	ctx := context.Background()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	for _, job := range []struct {
		id       string
		priority int
	}{
		{id: "normal-old", priority: 0},
		{id: "high", priority: 10},
		{id: "normal-new", priority: 0},
	} {
		if err := Enqueue(ctx, client, job.id, job.priority); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"high", "normal-old", "normal-new"} {
		got, err := Pop(ctx, client)
		if err != nil || got != want {
			t.Fatalf("expected %q, got %q (err=%v)", want, got, err)
		}
	}
	if _, err := Pop(ctx, client); err == nil {
		t.Fatal("expected empty queue error")
	}
}
