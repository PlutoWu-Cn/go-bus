package bus

import "sync"

// EventMetrics provides monitoring capabilities
type EventMetrics struct {
	PublishedEvents   int64
	ProcessedEvents   int64
	FailedEvents      int64
	ActiveSubscribers int32
	mu                sync.RWMutex
}

func (m *EventMetrics) IncrementPublished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PublishedEvents++
}

func (m *EventMetrics) IncrementProcessed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProcessedEvents++
}

func (m *EventMetrics) IncrementFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailedEvents++
}

func (m *EventMetrics) IncrementSubscribers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveSubscribers++
}

func (m *EventMetrics) DecrementSubscribers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveSubscribers--
}

func (m *EventMetrics) GetStats() (published, processed, failed int64, activeSubscribers int32) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PublishedEvents, m.ProcessedEvents, m.FailedEvents, m.ActiveSubscribers
} 