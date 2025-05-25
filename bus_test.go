package bus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type TestEvent struct {
	ID    string
	Value int
}

func TestBasicPublishSubscribe(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var received TestEvent
	var wg sync.WaitGroup
	wg.Add(1)

	handle := bus.SubscribeWithHandle("test.topic", func(event TestEvent) {
		received = event
		wg.Done()
	})
	defer handle.Unsubscribe()

	bus.Publish("test.topic", TestEvent{ID: "test1", Value: 42})
	wg.Wait()

	if received.ID != "test1" || received.Value != 42 {
		t.Errorf("Expected ID='test1', Value=42, got ID='%s', Value=%d", received.ID, received.Value)
	}
}

func TestPriorityHandling(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var executionOrder []string
	var mu sync.Mutex

	// Low priority
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "low")
		mu.Unlock()
	}, PriorityLow)

	// High priority
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "high")
		mu.Unlock()
	}, PriorityHigh)

	// Normal priority
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "normal")
		mu.Unlock()
	}, PriorityNormal)

	// Critical priority
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "critical")
		mu.Unlock()
	}, PriorityCritical)

	bus.Publish("priority.test", TestEvent{ID: "priority", Value: 1})
	bus.WaitAsync()

	expected := []string{"critical", "high", "normal", "low"}
	mu.Lock()
	defer mu.Unlock()

	if len(executionOrder) != len(expected) {
		t.Fatalf("Expected %d handlers, got %d", len(expected), len(executionOrder))
	}

	for i, exp := range expected {
		if executionOrder[i] != exp {
			t.Errorf("Expected order[%d]='%s', got '%s'", i, exp, executionOrder[i])
		}
	}
}

func TestEventFiltering(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var filteredEvents []TestEvent
	var allEvents []TestEvent
	var mu sync.Mutex

	// Filter: only process events with Value > 10
	bus.SubscribeWithFilter("filter.test", func(event TestEvent) {
		mu.Lock()
		filteredEvents = append(filteredEvents, event)
		mu.Unlock()
	}, func(topic string, event TestEvent) bool {
		return event.Value > 10
	})

	// Process all events
	bus.SubscribeWithHandle("filter.test", func(event TestEvent) {
		mu.Lock()
		allEvents = append(allEvents, event)
		mu.Unlock()
	})

	// Publish test events
	testEvents := []TestEvent{
		{ID: "e1", Value: 5},  // Should not be processed by filter
		{ID: "e2", Value: 15}, // Should be processed by filter
		{ID: "e3", Value: 3},  // Should not be processed by filter
		{ID: "e4", Value: 25}, // Should be processed by filter
	}

	for _, event := range testEvents {
		bus.Publish("filter.test", event)
	}
	bus.WaitAsync()

	mu.Lock()
	defer mu.Unlock()

	if len(allEvents) != 4 {
		t.Errorf("Expected 4 events in allEvents, got %d", len(allEvents))
	}

	if len(filteredEvents) != 2 {
		t.Errorf("Expected 2 events in filteredEvents, got %d", len(filteredEvents))
	}

	// Verify filtered events
	for _, event := range filteredEvents {
		if event.Value <= 10 {
			t.Errorf("Filtered event should have Value > 10, got %d", event.Value)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	var processedCount int32

	handle := bus.SubscribeWithContext(ctx, "context.test", func(event TestEvent) {
		atomic.AddInt32(&processedCount, 1)
	})
	defer handle.Unsubscribe()

	// Publish first event (should be processed)
	bus.Publish("context.test", TestEvent{ID: "before_cancel", Value: 1})

	// Cancel context
	cancel()

	// Wait briefly to ensure cancellation takes effect
	time.Sleep(10 * time.Millisecond)

	// Publish second event (should be skipped)
	bus.Publish("context.test", TestEvent{ID: "after_cancel", Value: 2})

	bus.WaitAsync()

	// Should only process one event
	if count := atomic.LoadInt32(&processedCount); count != 1 {
		t.Errorf("Expected 1 processed event, got %d", count)
	}
}

func TestErrorHandling(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var errorCount int32
	var normalCount int32

	// Set error handler
	bus.SetErrorHandler(func(err *EventError) {
		atomic.AddInt32(&errorCount, 1)
	})

	// Subscribe a handler that will panic
	bus.SubscribeWithHandle("error.test", func(event TestEvent) {
		if event.Value < 0 {
			panic("negative value")
		}
		atomic.AddInt32(&normalCount, 1)
	})

	// Publish normal event
	bus.Publish("error.test", TestEvent{ID: "normal", Value: 5})

	// Publish event that will cause panic
	bus.Publish("error.test", TestEvent{ID: "panic", Value: -1})

	bus.WaitAsync()

	if count := atomic.LoadInt32(&normalCount); count != 1 {
		t.Errorf("Expected 1 normal event, got %d", count)
	}

	if count := atomic.LoadInt32(&errorCount); count != 1 {
		t.Errorf("Expected 1 error, got %d", count)
	}
}

func TestMetrics(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	// Subscribe handlers
	handle1 := bus.SubscribeWithHandle("metrics.test", func(event TestEvent) {
		// Normal processing
	})
	handle2 := bus.SubscribeWithHandle("metrics.test", func(event TestEvent) {
		// Normal processing
	})

	defer func() {
		handle1.Unsubscribe()
		handle2.Unsubscribe()
	}()

	// Check initial subscriber count
	if count := bus.GetSubscriberCount("metrics.test"); count != 2 {
		t.Errorf("Expected 2 subscribers, got %d", count)
	}

	// Publish events
	for i := 0; i < 3; i++ {
		bus.Publish("metrics.test", TestEvent{ID: fmt.Sprintf("event%d", i), Value: i})
	}

	bus.WaitAsync()

	// Check metrics
	metrics := bus.GetMetrics()
	published, processed, failed, subscribers := metrics.GetStats()

	if published < 3 {
		t.Errorf("Expected at least 3 published events, got %d", published)
	}

	if processed < 6 { // 3 events * 2 handlers
		t.Errorf("Expected at least 6 processed events, got %d", processed)
	}

	if failed != 0 {
		t.Errorf("Expected 0 failed events, got %d", failed)
	}

	if subscribers != 2 {
		t.Errorf("Expected 2 active subscribers, got %d", subscribers)
	}
}

func TestMiddleware(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var middlewareCalled int32
	var handlerCalled int32

	// Add middleware
	bus.AddMiddleware(func(topic string, event interface{}, next func()) error {
		atomic.AddInt32(&middlewareCalled, 1)
		next()
		return nil
	})

	// Subscribe handler
	bus.SubscribeWithHandle("middleware.test", func(event TestEvent) {
		atomic.AddInt32(&handlerCalled, 1)
	})

	// Publish event
	bus.Publish("middleware.test", TestEvent{ID: "test", Value: 1})
	bus.WaitAsync()

	if count := atomic.LoadInt32(&middlewareCalled); count != 1 {
		t.Errorf("Expected middleware to be called 1 time, got %d", count)
	}

	if count := atomic.LoadInt32(&handlerCalled); count != 1 {
		t.Errorf("Expected handler to be called 1 time, got %d", count)
	}
}

func TestPublishWithTimeout(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	// Subscribe a synchronous handler that will block for a long time
	bus.SubscribeWithHandle("timeout.test", func(event TestEvent) {
		// Simulate blocking operation
		time.Sleep(100 * time.Millisecond)
	})

	// Test timeout publish (timeout shorter than processing time)
	start := time.Now()
	err := bus.PublishWithTimeout("timeout.test", TestEvent{ID: "timeout", Value: 1}, 50*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	// Verify it returns around timeout time
	if elapsed > 80*time.Millisecond {
		t.Errorf("Expected timeout around 50ms, but took %v", elapsed)
	}
}

func TestHandleUnsubscribe(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var count int32

	handle := bus.SubscribeWithHandle("unsubscribe.test", func(event TestEvent) {
		atomic.AddInt32(&count, 1)
	})

	// Publish first event
	bus.Publish("unsubscribe.test", TestEvent{ID: "first", Value: 1})

	// Unsubscribe
	err := handle.Unsubscribe()
	if err != nil {
		t.Errorf("Unexpected error during unsubscribe: %v", err)
	}

	// Publish second event (should not be processed)
	bus.Publish("unsubscribe.test", TestEvent{ID: "second", Value: 2})

	bus.WaitAsync()

	if finalCount := atomic.LoadInt32(&count); finalCount != 1 {
		t.Errorf("Expected 1 processed event after unsubscribe, got %d", finalCount)
	}

	// Check handle status
	if handle.IsActive() {
		t.Error("Handle should not be active after unsubscribe")
	}

	// Unsubscribing again should return error
	err = handle.Unsubscribe()
	if err == nil {
		t.Error("Expected error when unsubscribing already unsubscribed handle")
	}
}

func TestBusClose(t *testing.T) {
	bus := NewTyped[TestEvent]()

	var processed int32

	// Subscribe handler
	bus.SubscribeWithHandle("close.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
	})

	// Publish event
	bus.Publish("close.test", TestEvent{ID: "before_close", Value: 1})

	// Close bus
	err := bus.Close()
	if err != nil {
		t.Errorf("Unexpected error during close: %v", err)
	}

	// Try to publish again (should fail)
	err = bus.PublishWithContext(context.Background(), "close.test", TestEvent{ID: "after_close", Value: 2})
	if err == nil {
		t.Error("Expected error when publishing to closed bus")
	}

	// Try to subscribe (should fail)
	err = bus.Subscribe("close.test", func(event TestEvent) {})
	if err == nil {
		t.Error("Expected error when subscribing to closed bus")
	}

	// Closing again should return error
	err = bus.Close()
	if err == nil {
		t.Error("Expected error when closing already closed bus")
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var processed int64
	var wg sync.WaitGroup

	// Create multiple subscribers
	numSubscribers := 10
	for i := 0; i < numSubscribers; i++ {
		bus.SubscribeWithHandle("concurrent.test", func(event TestEvent) {
			atomic.AddInt64(&processed, 1)
		})
	}

	// Concurrent event publishing
	numPublishers := 5
	eventsPerPublisher := 100

	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				bus.Publish("concurrent.test", TestEvent{
					ID:    fmt.Sprintf("p%d-e%d", publisherID, j),
					Value: j,
				})
			}
		}(i)
	}

	wg.Wait()
	bus.WaitAsync()

	expectedProcessed := int64(numPublishers * eventsPerPublisher * numSubscribers)
	actualProcessed := atomic.LoadInt64(&processed)

	if actualProcessed != expectedProcessed {
		t.Errorf("Expected %d processed events, got %d", expectedProcessed, actualProcessed)
	}
}

// TestNew tests the New() function for compatibility
func TestNew(t *testing.T) {
	bus := New()
	defer bus.Close()

	var received interface{}
	var wg sync.WaitGroup
	wg.Add(1)

	handle := bus.SubscribeWithHandle("new.test", func(event interface{}) {
		received = event
		wg.Done()
	})
	defer handle.Unsubscribe()

	testData := map[string]interface{}{"key": "value", "number": 42}
	bus.Publish("new.test", testData)
	wg.Wait()

	if received == nil {
		t.Error("Expected to receive event, got nil")
	}

	receivedMap, ok := received.(map[string]interface{})
	if !ok {
		t.Error("Expected received event to be map[string]interface{}")
	} else {
		if receivedMap["key"] != "value" || receivedMap["number"] != 42 {
			t.Errorf("Expected received data to match sent data, got %v", receivedMap)
		}
	}
}

// TestSubscribeAsync tests asynchronous subscription
func TestSubscribeAsync(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var processed int32
	var wg sync.WaitGroup

	// Test non-transactional async
	err := bus.SubscribeAsync("async.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
		wg.Done()
	}, false)

	if err != nil {
		t.Errorf("Unexpected error in SubscribeAsync: %v", err)
	}

	wg.Add(1)
	bus.Publish("async.test", TestEvent{ID: "async1", Value: 1})
	wg.Wait()

	if count := atomic.LoadInt32(&processed); count != 1 {
		t.Errorf("Expected 1 processed event, got %d", count)
	}

	// Test transactional async
	err = bus.SubscribeAsync("async.transactional", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
		wg.Done()
	}, true)

	if err != nil {
		t.Errorf("Unexpected error in SubscribeAsync with transactional: %v", err)
	}

	wg.Add(1)
	bus.Publish("async.transactional", TestEvent{ID: "async2", Value: 2})
	wg.Wait()

	if count := atomic.LoadInt32(&processed); count != 2 {
		t.Errorf("Expected 2 processed events, got %d", count)
	}
}

// TestSubscribeAsyncWithHandle tests asynchronous subscription with handle
func TestSubscribeAsyncWithHandle(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var processed int32
	var wg sync.WaitGroup

	// Test non-transactional async with handle
	handle := bus.SubscribeAsyncWithHandle("async.handle.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
		wg.Done()
	}, false)

	if handle == nil {
		t.Error("Expected handle to be returned, got nil")
	}
	defer handle.Unsubscribe()

	wg.Add(1)
	bus.Publish("async.handle.test", TestEvent{ID: "async_handle1", Value: 1})
	wg.Wait()

	if count := atomic.LoadInt32(&processed); count != 1 {
		t.Errorf("Expected 1 processed event, got %d", count)
	}

	// Test transactional async with handle
	handle2 := bus.SubscribeAsyncWithHandle("async.handle.transactional", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
		wg.Done()
	}, true)

	if handle2 == nil {
		t.Error("Expected handle2 to be returned, got nil")
	}
	defer handle2.Unsubscribe()

	wg.Add(1)
	bus.Publish("async.handle.transactional", TestEvent{ID: "async_handle2", Value: 2})
	wg.Wait()

	if count := atomic.LoadInt32(&processed); count != 2 {
		t.Errorf("Expected 2 processed events, got %d", count)
	}
}

// TestSubscribeOnce tests one-time subscription
func TestSubscribeOnce(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var processed int32

	err := bus.SubscribeOnce("once.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
	})

	if err != nil {
		t.Errorf("Unexpected error in SubscribeOnce: %v", err)
	}

	// Publish first event (should be processed)
	bus.Publish("once.test", TestEvent{ID: "once1", Value: 1})
	bus.WaitAsync()

	// Publish second event (should NOT be processed)
	bus.Publish("once.test", TestEvent{ID: "once2", Value: 2})
	bus.WaitAsync()

	if count := atomic.LoadInt32(&processed); count != 1 {
		t.Errorf("Expected 1 processed event (once only), got %d", count)
	}

	// Verify no callback exists for the topic anymore
	if bus.HasCallback("once.test") {
		t.Error("Expected no callback after SubscribeOnce execution")
	}
}

// TestSubscribeOnceAsync tests one-time asynchronous subscription
func TestSubscribeOnceAsync(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var processed int32
	var wg sync.WaitGroup

	err := bus.SubscribeOnceAsync("once.async.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
		wg.Done()
	})

	if err != nil {
		t.Errorf("Unexpected error in SubscribeOnceAsync: %v", err)
	}

	// Publish first event (should be processed)
	wg.Add(1)
	bus.Publish("once.async.test", TestEvent{ID: "once_async1", Value: 1})
	wg.Wait()

	// Publish second event (should NOT be processed)
	bus.Publish("once.async.test", TestEvent{ID: "once_async2", Value: 2})
	bus.WaitAsync()

	if count := atomic.LoadInt32(&processed); count != 1 {
		t.Errorf("Expected 1 processed event (once only), got %d", count)
	}

	// Verify no callback exists for the topic anymore
	if bus.HasCallback("once.async.test") {
		t.Error("Expected no callback after SubscribeOnceAsync execution")
	}
}

// TestHasCallback tests the HasCallback function
func TestHasCallback(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	// Initially no callback
	if bus.HasCallback("callback.test") {
		t.Error("Expected no callback initially")
	}

	// Subscribe a handler
	handle := bus.SubscribeWithHandle("callback.test", func(event TestEvent) {})
	defer handle.Unsubscribe()

	// Now should have callback
	if !bus.HasCallback("callback.test") {
		t.Error("Expected callback after subscription")
	}

	// Unsubscribe
	handle.Unsubscribe()

	// Should not have callback anymore
	if bus.HasCallback("callback.test") {
		t.Error("Expected no callback after unsubscription")
	}

	// Test with non-existent topic
	if bus.HasCallback("non.existent.topic") {
		t.Error("Expected no callback for non-existent topic")
	}
}

// TestGetTopics tests the GetTopics function
func TestGetTopics(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	// Initially no topics
	topics := bus.GetTopics()
	if len(topics) != 0 {
		t.Errorf("Expected 0 topics initially, got %d", len(topics))
	}

	// Subscribe to multiple topics
	handle1 := bus.SubscribeWithHandle("topic1", func(event TestEvent) {})
	handle2 := bus.SubscribeWithHandle("topic2", func(event TestEvent) {})
	handle3 := bus.SubscribeWithHandle("topic3", func(event TestEvent) {})

	defer handle1.Unsubscribe()
	defer handle2.Unsubscribe()
	defer handle3.Unsubscribe()

	// Get topics
	topics = bus.GetTopics()
	if len(topics) != 3 {
		t.Errorf("Expected 3 topics, got %d", len(topics))
	}

	// Check if all topics are present
	topicMap := make(map[string]bool)
	for _, topic := range topics {
		topicMap[topic] = true
	}

	expectedTopics := []string{"topic1", "topic2", "topic3"}
	for _, expected := range expectedTopics {
		if !topicMap[expected] {
			t.Errorf("Expected topic '%s' to be present", expected)
		}
	}

	// Note: The current implementation doesn't remove empty topic entries from the map
	// when all handlers are unsubscribed, so topics remain in GetTopics() result
	// This is the actual behavior of the current implementation
}

// TestGetSubscriberCount tests the GetSubscriberCount function
func TestGetSubscriberCount(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	topic := "subscriber.count.test"

	// Initially no subscribers
	count := bus.GetSubscriberCount(topic)
	if count != 0 {
		t.Errorf("Expected 0 subscribers initially, got %d", count)
	}

	// Add subscribers
	handle1 := bus.SubscribeWithHandle(topic, func(event TestEvent) {})
	handle2 := bus.SubscribeWithHandle(topic, func(event TestEvent) {})
	handle3 := bus.SubscribeWithHandle(topic, func(event TestEvent) {})

	defer handle1.Unsubscribe()
	defer handle2.Unsubscribe()
	defer handle3.Unsubscribe()

	// Should have 3 subscribers
	count = bus.GetSubscriberCount(topic)
	if count != 3 {
		t.Errorf("Expected 3 subscribers, got %d", count)
	}

	// Unsubscribe one
	handle1.Unsubscribe()

	// Should have 2 subscribers
	count = bus.GetSubscriberCount(topic)
	if count != 2 {
		t.Errorf("Expected 2 subscribers after unsubscribe, got %d", count)
	}

	// Test non-existent topic
	count = bus.GetSubscriberCount("non.existent.topic")
	if count != 0 {
		t.Errorf("Expected 0 subscribers for non-existent topic, got %d", count)
	}
}

// TestEventError tests the EventError type
func TestEventError(t *testing.T) {
	err := &EventError{
		Topic: "test.topic",
		Event: TestEvent{ID: "test", Value: 42},
		Err:   fmt.Errorf("test error"),
	}

	errorString := err.Error()
	expectedSubstring := "event error in topic 'test.topic'"
	if !contains(errorString, expectedSubstring) {
		t.Errorf("Expected error string to contain '%s', got '%s'", expectedSubstring, errorString)
	}

	if !contains(errorString, "test error") {
		t.Errorf("Expected error string to contain 'test error', got '%s'", errorString)
	}
}

// TestAsyncErrorHandling tests error handling in async operations
func TestAsyncErrorHandling(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var errorCount int32
	var wg sync.WaitGroup

	// Set error handler
	bus.SetErrorHandler(func(err *EventError) {
		atomic.AddInt32(&errorCount, 1)
		wg.Done()
	})

	// Subscribe async handler that will panic
	bus.SubscribeAsync("async.error.test", func(event TestEvent) {
		if event.Value < 0 {
			panic("negative value in async handler")
		}
	}, false)

	// Publish event that will cause panic
	wg.Add(1)
	bus.Publish("async.error.test", TestEvent{ID: "error", Value: -1})
	wg.Wait()

	if count := atomic.LoadInt32(&errorCount); count != 1 {
		t.Errorf("Expected 1 error to be handled, got %d", count)
	}
}

// TestClosedBusOperations tests operations on a closed bus
func TestClosedBusOperations(t *testing.T) {
	bus := NewTyped[TestEvent]()

	// Close the bus
	err := bus.Close()
	if err != nil {
		t.Errorf("Unexpected error closing bus: %v", err)
	}

	// Test Subscribe on closed bus
	err = bus.Subscribe("closed.test", func(event TestEvent) {})
	if err == nil {
		t.Error("Expected error when subscribing to closed bus")
	}

	// Test SubscribeAsync on closed bus
	err = bus.SubscribeAsync("closed.test", func(event TestEvent) {}, false)
	if err == nil {
		t.Error("Expected error when subscribing async to closed bus")
	}

	// Test SubscribeOnce on closed bus
	err = bus.SubscribeOnce("closed.test", func(event TestEvent) {})
	if err == nil {
		t.Error("Expected error when subscribing once to closed bus")
	}

	// Test SubscribeOnceAsync on closed bus
	err = bus.SubscribeOnceAsync("closed.test", func(event TestEvent) {})
	if err == nil {
		t.Error("Expected error when subscribing once async to closed bus")
	}

	// Test doSubscribeWithHandle on closed bus (returns nil)
	handle := bus.SubscribeWithHandle("closed.test", func(event TestEvent) {})
	if handle != nil {
		t.Error("Expected nil handle when subscribing to closed bus")
	}

	// Test SubscribeAsyncWithHandle on closed bus (returns nil)
	handle = bus.SubscribeAsyncWithHandle("closed.test", func(event TestEvent) {}, false)
	if handle != nil {
		t.Error("Expected nil handle when subscribing async to closed bus")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
