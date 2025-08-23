package events

import (
	"log"
	"sync"
)

type Event struct {
	StreamKey string
	Timestamp int
	Value     any
}

type EventHub struct {
	mu      sync.Mutex
	subs    map[int]chan Event
	next    int
	last    Event
	hasLast bool

	events chan Event
}

func NewHub() *EventHub {
	h := &EventHub{
		subs:   map[int]chan Event{},
		events: make(chan Event, 128),
	}
	go h.run()
	return h
}

// Subscribe returns (id, read-only channel, cancel)
func (h *EventHub) Subscribe() (int, <-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.next
	h.next++

	ch := make(chan Event, 16)
	// push the last event immediately, if we have one
	if h.hasLast {
		select {
		case ch <- h.last:
		default:
			// should be room in a fresh buffer, but keep it non-blocking
		}
	}
	h.subs[id] = ch

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			close(c)
			delete(h.subs, id)
		}
	}

	return id, ch, cancel
}

func (h *EventHub) run() {
	for event := range h.events {
		// record last + snapshot subscribers under lock
		h.mu.Lock()
		h.last = event
		h.hasLast = true

		subs := make([]chan Event, 0, len(h.subs))
		for _, ch := range h.subs {
			subs = append(subs, ch)
		}
		h.mu.Unlock()

		// fan out without holding the lock
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
				// non-blocking: drop if subscriber is slow
				log.Printf("eventhub: subscriber channel full; dropping event")
			}
		}
	}

	// events channel closed: close all subscriber chans
	h.mu.Lock()
	for id, ch := range h.subs {
		close(ch)
		delete(h.subs, id)
	}
	h.mu.Unlock()
}

// Broadcast Non-blocking: enqueue or drop if hub queue is full
func (h *EventHub) Broadcast(event Event) {
	select {
	case h.events <- event:
	default:
		log.Printf("eventhub: hub queue full; dropping event")
	}
}

// Close stops the hub and closes all subscriber channels.
func (h *EventHub) Close() {
	close(h.events)
}
