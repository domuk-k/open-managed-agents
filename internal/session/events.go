package session

import (
	"encoding/json"
	"sync"
)

type Event struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolUseEvent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type UserMessageEvent struct {
	Type    string         `json:"type"`
	Content []ContentBlock `json:"content"`
}

// EventBus manages per-session SSE event pub/sub.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

func (b *EventBus) Subscribe(sessionID string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 64)
	b.subscribers[sessionID] = append(b.subscribers[sessionID], ch)
	return ch
}

func (b *EventBus) Unsubscribe(sessionID string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[sessionID]
	for i, s := range subs {
		if s == ch {
			b.subscribers[sessionID] = append(subs[:i], subs[i+1:]...)
			close(s)
			break
		}
	}
}

func (b *EventBus) Emit(sessionID string, event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[sessionID] {
		select {
		case ch <- event:
		default:
			// drop if subscriber is slow
		}
	}
}
