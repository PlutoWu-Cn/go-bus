package bus

import "sync"

// Metrics defines the monitoring interface for the event bus
type Metrics interface {
	IncrementPublished()
	IncrementProcessed()
	IncrementFailed()
	IncrementSubscribers()
	DecrementSubscribers()
	GetStats() (published, processed, failed int64, activeSubscribers int32)
}

// DefaultMetrics is the default implementation of the Metrics interface
type DefaultMetrics struct {
	PublishedEvents   int64
	ProcessedEvents   int64
	FailedEvents      int64
	ActiveSubscribers int32
	mu                sync.RWMutex
}

func (m *DefaultMetrics) IncrementPublished() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PublishedEvents++
}

func (m *DefaultMetrics) IncrementProcessed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProcessedEvents++
}

func (m *DefaultMetrics) IncrementFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailedEvents++
}

func (m *DefaultMetrics) IncrementSubscribers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveSubscribers++
}

func (m *DefaultMetrics) DecrementSubscribers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveSubscribers--
}

func (m *DefaultMetrics) GetStats() (published, processed, failed int64, activeSubscribers int32) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.PublishedEvents, m.ProcessedEvents, m.FailedEvents, m.ActiveSubscribers
}
