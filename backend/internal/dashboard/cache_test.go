package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRedisNeverReplacesNewerSnapshot(t *testing.T) {
	if os.Getenv("MOVANK_INTEGRATION") != "1" {
		t.Skip("requires Redis")
	}
	service := New(nil, os.Getenv("REDIS_ADDR"))
	defer service.Close()
	ctx := context.Background()
	key := fmt.Sprintf("test:dashboard:%d", time.Now().UnixNano())
	defer service.cache.Del(ctx, key)
	var wg sync.WaitGroup
	for i := int64(1); i <= 50; i++ {
		wg.Add(1)
		go func(n int64) {
			defer wg.Done()
			data, _ := json.Marshal(Snapshot{Date: "2026-01-01", Currency: "COP", PaidSales: n, TotalMinor: n * 100})
			if err := saveSnapshot.Run(ctx, service.cache, []string{key}, n, string(data)).Err(); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	data, err := service.cache.Get(ctx, key).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var result Snapshot
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.PaidSales != 50 || result.TotalMinor != 5000 {
		t.Fatalf("stale cache: %+v", result)
	}
	ttl := service.cache.TTL(ctx, key).Val()
	if ttl <= 0 || ttl > 30*time.Second {
		t.Fatalf("unexpected TTL %s", ttl)
	}
}
