package aievents

import (
	"strings"
	"sync"
	"time"
)

type Event struct {
	Type      string    `json:"type"`
	UserID    string    `json:"user_id"`
	JobID     string    `json:"job_id,omitempty"`
	MessageID int64     `json:"message_id,omitempty"`
	State     string    `json:"state,omitempty"`
	Error     string    `json:"error,omitempty"`
	At        time.Time `json:"at"`
}

type Bus struct {
	mu          sync.Mutex
	nextID      int
	subscribers map[string]map[int]chan Event
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[string]map[int]chan Event)}
}

func (b *Bus) Subscribe(userID string) (<-chan Event, func()) {
	userID = strings.TrimSpace(userID)
	ch := make(chan Event, 16)
	if b == nil || userID == "" {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	if b.subscribers[userID] == nil {
		b.subscribers[userID] = make(map[int]chan Event)
	}
	b.nextID++
	id := b.nextID
	b.subscribers[userID][id] = ch
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subscribers := b.subscribers[userID]
		if subscribers == nil {
			return
		}
		delete(subscribers, id)
		if len(subscribers) == 0 {
			delete(b.subscribers, userID)
		}
	}
	return ch, cancel
}

func (b *Bus) Publish(event Event) {
	event.UserID = strings.TrimSpace(event.UserID)
	if b == nil || event.UserID == "" {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[event.UserID] {
		select {
		case ch <- event:
		default:
		}
	}
}
