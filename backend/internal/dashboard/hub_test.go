package dashboard

import (
	"sync"
	"testing"
)

func TestHubIsolationAndSlowConsumer(t *testing.T) {
	hub := NewHub()
	a, stopA, _ := hub.Subscribe("a")
	defer stopA()
	b, stopB, _ := hub.Subscribe("b")
	defer stopB()
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Add(1)
		go func(n int64) { defer wg.Done(); hub.Publish("a", Snapshot{Date: "2026-01-01", PaidSales: n}) }(i)
	}
	wg.Wait()
	if got := <-a; got.PaidSales != 100 {
		t.Fatalf("latest snapshot lost: %+v", got)
	}
	select {
	case <-b:
		t.Fatal("cross merchant event")
	default:
	}
	stopA()
	stopA()
	if len(hub.Merchants()) != 1 {
		t.Fatal("unsubscribe was not idempotent")
	}
}
