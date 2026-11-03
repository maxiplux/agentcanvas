// Package hub is the SSE subscriber registry (§4.1, §12.2).
package hub

import "sync"

// Buffer is how many events a subscriber may lag before it is dropped.
const Buffer = 4

// Sub is one subscriber. C is closed when the hub drops the subscriber.
type Sub struct {
	C chan []byte
}

// Hub fans state events out to subscribers.
type Hub struct {
	mu     sync.Mutex
	subs   map[*Sub]struct{}
	closed bool
}

func New() *Hub { return &Hub{subs: map[*Sub]struct{}{}} }

// Subscribe registers a new subscriber.
func (h *Hub) Subscribe() *Sub {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := &Sub{C: make(chan []byte, Buffer)}
	if h.closed {
		close(s.C)
		return s
	}
	h.subs[s] = struct{}{}
	return s
}

// Unsubscribe removes a subscriber (safe to call twice).
func (h *Hub) Unsubscribe(s *Sub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.subs[s]; ok {
		delete(h.subs, s)
		close(s.C)
	}
}

// Publish sends data to every subscriber without blocking; a full
// subscriber is dropped and must reconnect.
func (h *Hub) Publish(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.C <- data:
		default:
			delete(h.subs, s)
			close(s.C)
		}
	}
}

// Len is the number of live subscribers.
func (h *Hub) Len() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

// Close drops every subscriber and refuses new ones.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		delete(h.subs, s)
		close(s.C)
	}
}
