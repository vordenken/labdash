package server

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Hub fans out server-sent events to connected browsers.
type Hub struct {
	mu      sync.Mutex
	clients map[chan event]struct{}
}

type event struct {
	name string
	data []byte
}

func NewHub() *Hub { return &Hub{clients: map[chan event]struct{}{}} }

// Broadcast sends a JSON event to all connected clients. Slow clients drop
// events instead of blocking the broadcaster.
func (h *Hub) Broadcast(name string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	ev := event{name: name, data: data}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (h *Hub) subscribe() (chan event, func()) {
	ch := make(chan event, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}
}

// ServeHTTP streams events to one client. initial is sent first so the
// client has a full state before deltas arrive.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request, initial []event) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch, unsub := h.subscribe()
	defer unsub()

	write := func(ev event) bool {
		if _, err := w.Write([]byte("event: " + ev.name + "\ndata: ")); err != nil {
			return false
		}
		if _, err := w.Write(ev.data); err != nil {
			return false
		}
		if _, err := w.Write([]byte("\n\n")); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	for _, ev := range initial {
		if !write(ev) {
			return
		}
	}
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			fl.Flush()
		case ev := <-ch:
			if !write(ev) {
				return
			}
		}
	}
}
