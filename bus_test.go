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

	// 低优先级
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "low")
		mu.Unlock()
	}, PriorityLow)

	// 高优先级
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "high")
		mu.Unlock()
	}, PriorityHigh)

	// 普通优先级
	bus.SubscribeWithPriority("priority.test", func(event TestEvent) {
		mu.Lock()
		executionOrder = append(executionOrder, "normal")
		mu.Unlock()
	}, PriorityNormal)

	// 关键优先级
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

	// 过滤器：只处理 Value > 10 的事件
	bus.SubscribeWithFilter("filter.test", func(event TestEvent) {
		mu.Lock()
		filteredEvents = append(filteredEvents, event)
		mu.Unlock()
	}, func(topic string, event TestEvent) bool {
		return event.Value > 10
	})

	// 处理所有事件
	bus.SubscribeWithHandle("filter.test", func(event TestEvent) {
		mu.Lock()
		allEvents = append(allEvents, event)
		mu.Unlock()
	})

	// 发布测试事件
	testEvents := []TestEvent{
		{ID: "e1", Value: 5},   // 不应该被过滤器处理
		{ID: "e2", Value: 15},  // 应该被过滤器处理
		{ID: "e3", Value: 3},   // 不应该被过滤器处理
		{ID: "e4", Value: 25},  // 应该被过滤器处理
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

	// 验证过滤的事件
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

	// 发布第一个事件（应该被处理）
	bus.Publish("context.test", TestEvent{ID: "before_cancel", Value: 1})
	
	// 取消上下文
	cancel()
	
	// 短暂等待确保取消生效
	time.Sleep(10 * time.Millisecond)
	
	// 发布第二个事件（应该被跳过）
	bus.Publish("context.test", TestEvent{ID: "after_cancel", Value: 2})
	
	bus.WaitAsync()

	// 应该只处理一个事件
	if count := atomic.LoadInt32(&processedCount); count != 1 {
		t.Errorf("Expected 1 processed event, got %d", count)
	}
}

func TestErrorHandling(t *testing.T) {
	bus := NewTyped[TestEvent]()
	defer bus.Close()

	var errorCount int32
	var normalCount int32

	// 设置错误处理器
	bus.SetErrorHandler(func(err *EventError) {
		atomic.AddInt32(&errorCount, 1)
	})

	// 订阅一个会产生 panic 的处理器
	bus.SubscribeWithHandle("error.test", func(event TestEvent) {
		if event.Value < 0 {
			panic("negative value")
		}
		atomic.AddInt32(&normalCount, 1)
	})

	// 发布正常事件
	bus.Publish("error.test", TestEvent{ID: "normal", Value: 5})
	
	// 发布会导致 panic 的事件
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

	// 订阅处理器
	handle1 := bus.SubscribeWithHandle("metrics.test", func(event TestEvent) {
		// 正常处理
	})
	handle2 := bus.SubscribeWithHandle("metrics.test", func(event TestEvent) {
		// 正常处理
	})

	defer func() {
		handle1.Unsubscribe()
		handle2.Unsubscribe()
	}()

	// 检查初始订阅者数量
	if count := bus.GetSubscriberCount("metrics.test"); count != 2 {
		t.Errorf("Expected 2 subscribers, got %d", count)
	}

	// 发布事件
	for i := 0; i < 3; i++ {
		bus.Publish("metrics.test", TestEvent{ID: fmt.Sprintf("event%d", i), Value: i})
	}
	
	bus.WaitAsync()

	// 检查指标
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

	// 添加中间件
	bus.AddMiddleware(func(topic string, event interface{}, next func()) error {
		atomic.AddInt32(&middlewareCalled, 1)
		next()
		return nil
	})

	// 订阅处理器
	bus.SubscribeWithHandle("middleware.test", func(event TestEvent) {
		atomic.AddInt32(&handlerCalled, 1)
	})

	// 发布事件
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

	// 订阅一个同步处理器，但会阻塞较长时间
	bus.SubscribeWithHandle("timeout.test", func(event TestEvent) {
		// 模拟阻塞操作
		time.Sleep(100 * time.Millisecond)
	})

	// 测试超时发布（超时时间比处理时间短）
	start := time.Now()
	err := bus.PublishWithTimeout("timeout.test", TestEvent{ID: "timeout", Value: 1}, 50*time.Millisecond)
	elapsed := time.Since(start)
	
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	
	// 验证确实在超时时间附近返回
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

	// 发布第一个事件
	bus.Publish("unsubscribe.test", TestEvent{ID: "first", Value: 1})
	
	// 取消订阅
	err := handle.Unsubscribe()
	if err != nil {
		t.Errorf("Unexpected error during unsubscribe: %v", err)
	}

	// 发布第二个事件（应该不被处理）
	bus.Publish("unsubscribe.test", TestEvent{ID: "second", Value: 2})
	
	bus.WaitAsync()

	if finalCount := atomic.LoadInt32(&count); finalCount != 1 {
		t.Errorf("Expected 1 processed event after unsubscribe, got %d", finalCount)
	}

	// 检查 handle 状态
	if handle.IsActive() {
		t.Error("Handle should not be active after unsubscribe")
	}

	// 再次取消订阅应该返回错误
	err = handle.Unsubscribe()
	if err == nil {
		t.Error("Expected error when unsubscribing already unsubscribed handle")
	}
}

func TestBusClose(t *testing.T) {
	bus := NewTyped[TestEvent]()

	var processed int32

	// 订阅处理器
	bus.SubscribeWithHandle("close.test", func(event TestEvent) {
		atomic.AddInt32(&processed, 1)
	})

	// 发布事件
	bus.Publish("close.test", TestEvent{ID: "before_close", Value: 1})
	
	// 关闭总线
	err := bus.Close()
	if err != nil {
		t.Errorf("Unexpected error during close: %v", err)
	}

	// 尝试再次发布（应该失败）
	err = bus.PublishWithContext(context.Background(), "close.test", TestEvent{ID: "after_close", Value: 2})
	if err == nil {
		t.Error("Expected error when publishing to closed bus")
	}

	// 尝试订阅（应该失败）
	err = bus.Subscribe("close.test", func(event TestEvent) {})
	if err == nil {
		t.Error("Expected error when subscribing to closed bus")
	}

	// 再次关闭应该返回错误
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

	// 创建多个订阅者
	numSubscribers := 10
	for i := 0; i < numSubscribers; i++ {
		bus.SubscribeWithHandle("concurrent.test", func(event TestEvent) {
			atomic.AddInt64(&processed, 1)
		})
	}

	// 并发发布事件
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