package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	eventKeepAlive    = 25 * time.Second
	eventWriteTimeout = 35 * time.Second
)

type eventBroker struct {
	mu          sync.Mutex
	subscribers map[string]map[chan struct{}]struct{}
}

func newEventBroker() *eventBroker {
	return &eventBroker{subscribers: make(map[string]map[chan struct{}]struct{})}
}

func (b *eventBroker) subscribe(topic string) (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subscribers[topic] == nil {
		b.subscribers[topic] = make(map[chan struct{}]struct{})
	}
	b.subscribers[topic][updates] = struct{}{}
	b.mu.Unlock()

	return updates, func() {
		b.mu.Lock()
		delete(b.subscribers[topic], updates)
		if len(b.subscribers[topic]) == 0 {
			delete(b.subscribers, topic)
		}
		b.mu.Unlock()
	}
}

func (b *eventBroker) notify(topics ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, topic := range topics {
		for updates := range b.subscribers[topic] {
			select {
			case updates <- struct{}{}:
			default:
			}
		}
	}
}

func streamEvents(w http.ResponseWriter, r *http.Request, broker *eventBroker, topic, eventName string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Event stream unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")

	updates, unsubscribe := broker.subscribe(topic)
	defer unsubscribe()
	keepAlive := time.NewTicker(eventKeepAlive)
	defer keepAlive.Stop()
	controller := http.NewResponseController(w)
	writeDeadline := func() { _ = controller.SetWriteDeadline(time.Now().Add(eventWriteTimeout)) }

	write := func(payload string) bool {
		writeDeadline()
		if _, err := fmt.Fprint(w, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	event := fmt.Sprintf("event: %s\ndata: {}\n\n", eventName)
	if !write(event) {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-updates:
			if !write(event) {
				return
			}
		case <-keepAlive.C:
			if !write(": keepalive\n\n") {
				return
			}
		}
	}
}
