package events

import "sync"

type Event struct {
	StreamKey string
	Timestamp int
	Value     any
}

type EventHub struct {
	mu   sync.Mutex
	subs map[int]chan *Event
	next int
	last *Event
}

func NewHub() *EventHub {
	return &EventHub{subs: map[int]chan *Event{}, last: &Event{}}
}

func (h *EventHub) Subscribe() (int, <-chan *Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.next
	h.next++
	ch := make(chan *Event, 16)
	if h.last != nil {
		ch <- h.copy(h.last)
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

func (h *EventHub) Broadcast(event *Event) {
	h.mu.Lock()
	h.last = event
	for _, ch := range h.subs {
		select {
		case ch <- h.copy(event):
		default:
		}
	}
	h.mu.Unlock()
}

func (h *EventHub) copy(e *Event) *Event {
	return &Event{e.StreamKey, e.Timestamp, e.Value}
}
