package memory

import (
	"sync"
	"time"

	"github.com/johncui/PAIM/pkg/model"
)

// SensoryBuffer is an in-memory TTL buffer for short-lived sensory memories.
type SensoryBuffer struct {
	mu       sync.Mutex
	items    []bufferItem
	capacity int
	ttl      time.Duration
}

type bufferItem struct {
	at    time.Time
	input model.SensoryInput
}

func NewSensoryBuffer(capacity int, ttl time.Duration) *SensoryBuffer {
	return &SensoryBuffer{capacity: capacity, ttl: ttl}
}

// Add pushes a new item, evicting the oldest if capacity exceeded.
func (b *SensoryBuffer) Add(input model.SensoryInput) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.items = append(b.items, bufferItem{at: time.Now(), input: input})
	if len(b.items) > b.capacity {
		b.items = b.items[len(b.items)-b.capacity:]
	}
}

// Snapshot returns non-expired items.
func (b *SensoryBuffer) Snapshot() []model.SensoryInput {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-b.ttl)
	var filtered []bufferItem
	for _, item := range b.items {
		if item.at.After(cutoff) {
			filtered = append(filtered, item)
		}
	}
	b.items = filtered

	outputs := make([]model.SensoryInput, len(filtered))
	for i, item := range filtered {
		outputs[i] = item.input
	}
	return outputs
}

// Clear removes all items.
func (b *SensoryBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = nil
}
