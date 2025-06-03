package bus

import (
	"sync/atomic"
	"testing"
)

// CustomMetrics is a custom implementation of the Metrics interface
type CustomMetrics struct {
	published   int64
	processed   int64
	failed      int64
	subscribers int32
}

func (m *CustomMetrics) IncrementPublished() {
	atomic.AddInt64(&m.published, 1)
}

func (m *CustomMetrics) IncrementProcessed() {
	atomic.AddInt64(&m.processed, 1)
}

func (m *CustomMetrics) IncrementFailed() {
	atomic.AddInt64(&m.failed, 1)
}

func (m *CustomMetrics) IncrementSubscribers() {
	atomic.AddInt32(&m.subscribers, 1)
}

func (m *CustomMetrics) DecrementSubscribers() {
	atomic.AddInt32(&m.subscribers, -1)
}

func (m *CustomMetrics) GetStats() (published, processed, failed int64, activeSubscribers int32) {
	return atomic.LoadInt64(&m.published),
		atomic.LoadInt64(&m.processed),
		atomic.LoadInt64(&m.failed),
		atomic.LoadInt32(&m.subscribers)
}

func TestCustomMetricsImplementation(t *testing.T) {
	// Create custom metrics
	customMetrics := &CustomMetrics{}

	// Create a bus with custom metrics using WithMetrics option
	bus := New(WithMetrics[any](customMetrics))
	defer bus.Close()

	// Subscribe to an event
	handle := bus.SubscribeWithHandle("test.topic", func(event any) {
		// Normal processing
	})
	defer handle.Unsubscribe()

	// Check subscriber count
	published, processed, failed, subscribers := customMetrics.GetStats()
	if subscribers != 1 {
		t.Errorf("Expected 1 subscriber, got %d", subscribers)
	}

	// Publish an event
	bus.Publish("test.topic", "test event")
	bus.WaitAsync()

	// Check metrics
	published, processed, failed, subscribers = customMetrics.GetStats()
	if published != 1 {
		t.Errorf("Expected 1 published event, got %d", published)
	}
	if processed != 1 {
		t.Errorf("Expected 1 processed event, got %d", processed)
	}
	if failed != 0 {
		t.Errorf("Expected 0 failed events, got %d", failed)
	}

	// Unsubscribe
	handle.Unsubscribe()

	// Check subscriber count after unsubscribe
	published, processed, failed, subscribers = customMetrics.GetStats()
	if subscribers != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", subscribers)
	}
}

func TestWithOptions(t *testing.T) {
	// Test various Option combinations

	// Custom metrics
	customMetrics := &CustomMetrics{}

	// Custom logger
	customLogger := NewDefaultLogger()

	// Error handler
	var errorCount int
	errorHandler := func(err *EventError) {
		errorCount++
	}

	// Middleware
	middleware := func(topic string, event any, next func()) error {
		next()
		return nil
	}

	// Create bus with all options
	bus := New(
		WithMetrics[any](customMetrics),
		WithLogger[any](customLogger),
		WithErrorHandler[any](errorHandler),
		WithMiddleware[any](middleware),
	)
	defer bus.Close()

	// Test publish and subscribe
	bus.Subscribe("test.options", func(event any) {
		// Normal processing
	})

	bus.Publish("test.options", "test event")
	bus.WaitAsync()

	// Verify metrics are being used correctly
	published, processed, _, _ := customMetrics.GetStats()
	if published != 1 {
		t.Errorf("Expected 1 published event, got %d", published)
	}
	if processed != 1 {
		t.Errorf("Expected 1 processed event, got %d", processed)
	}

	// Get metrics from bus and verify it's our custom implementation
	metricsFromBus := bus.GetMetrics()
	if metricsFromBus != customMetrics {
		t.Errorf("Expected metrics from bus to be our custom metrics instance")
	}
}
