package dashboard

import "sync"

type Hub struct {
	mu      sync.Mutex
	clients map[string]map[chan Snapshot]struct{}
	count   int
}

func NewHub() *Hub { return &Hub{clients: make(map[string]map[chan Snapshot]struct{})} }

func (h *Hub) Subscribe(merchantID string) (<-chan Snapshot, func(), bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.count >= 1000 || len(h.clients[merchantID]) >= 100 {
		return nil, nil, false
	}
	if h.clients[merchantID] == nil {
		h.clients[merchantID] = make(map[chan Snapshot]struct{})
	}
	ch := make(chan Snapshot, 1)
	h.clients[merchantID][ch] = struct{}{}
	h.count++
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.clients[merchantID], ch)
			h.count--
			if len(h.clients[merchantID]) == 0 {
				delete(h.clients, merchantID)
			}
		})
	}
	return ch, cancel, true
}

func (h *Hub) Publish(merchantID string, snapshot Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients[merchantID] {
		select {
		case old := <-ch:
			if old.Date > snapshot.Date || (old.Date == snapshot.Date && old.PaidSales > snapshot.PaidSales) {
				ch <- old
				continue
			}
		default:
		}
		ch <- snapshot
	}
}

func (h *Hub) Merchants() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]string, 0, len(h.clients))
	for id := range h.clients {
		result = append(result, id)
	}
	return result
}
