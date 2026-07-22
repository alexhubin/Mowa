package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type callEventBroker struct {
	mu          sync.Mutex
	subscribers map[string]map[chan struct{}]struct{}
}

func newCallEventBroker() *callEventBroker {
	return &callEventBroker{subscribers: make(map[string]map[chan struct{}]struct{})}
}

func (b *callEventBroker) subscribe(userID string) (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = make(map[chan struct{}]struct{})
	}
	b.subscribers[userID][updates] = struct{}{}
	b.mu.Unlock()

	return updates, func() {
		b.mu.Lock()
		delete(b.subscribers[userID], updates)
		if len(b.subscribers[userID]) == 0 {
			delete(b.subscribers, userID)
		}
		b.mu.Unlock()
	}
}

func (b *callEventBroker) notify(userIDs ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, userID := range userIDs {
		for updates := range b.subscribers[userID] {
			select {
			case updates <- struct{}{}:
			default:
			}
		}
	}
}

func (s *Server) streamCallEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Поток событий недоступен")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")

	updates, unsubscribe := s.callEvents.subscribe(currentUser(r).ID)
	defer unsubscribe()
	keepAlive := time.NewTicker(25 * time.Second)
	defer keepAlive.Stop()
	controller := http.NewResponseController(w)
	writeDeadline := func() { _ = controller.SetWriteDeadline(time.Now().Add(35 * time.Second)) }

	writeEvent := func() bool {
		writeDeadline()
		if _, err := fmt.Fprint(w, "event: calls\ndata: {}\n\n"); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !writeEvent() {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			if !writeEvent() {
				return
			}
		case <-keepAlive.C:
			writeDeadline()
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
