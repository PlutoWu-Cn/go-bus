package bus

import (
	"sync"
	"testing"
)

type BenchEvent struct {
	ID   int
	Data string
}

func BenchmarkSyncPublish(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	bus.SubscribeWithHandle("bench.sync", func(event BenchEvent) {
		// 简单的处理
		_ = event.ID * 2
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bus.Publish("bench.sync", BenchEvent{
				ID:   i,
				Data: "test data",
			})
			i++
		}
	})
}

func BenchmarkAsyncPublish(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	bus.SubscribeAsync("bench.async", func(event BenchEvent) {
		// 简单的处理
		_ = event.ID * 2
	}, false)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bus.Publish("bench.async", BenchEvent{
				ID:   i,
				Data: "test data",
			})
			i++
		}
	})
	
	bus.WaitAsync()
}

func BenchmarkMultipleSubscribers(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	// 创建多个订阅者
	numSubscribers := 10
	for i := 0; i < numSubscribers; i++ {
		bus.SubscribeWithHandle("bench.multi", func(event BenchEvent) {
			_ = event.ID * 2
		})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			bus.Publish("bench.multi", BenchEvent{
				ID:   i,
				Data: "test data",
			})
			i++
		}
	})
}

func BenchmarkWithPriority(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	// 不同优先级的订阅者
	bus.SubscribeWithPriority("bench.priority", func(event BenchEvent) {
		_ = event.ID * 2
	}, PriorityCritical)
	
	bus.SubscribeWithPriority("bench.priority", func(event BenchEvent) {
		_ = event.ID * 3
	}, PriorityNormal)
	
	bus.SubscribeWithPriority("bench.priority", func(event BenchEvent) {
		_ = event.ID * 4
	}, PriorityLow)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.priority", BenchEvent{
			ID:   i,
			Data: "test data",
		})
	}
}

func BenchmarkWithFilter(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	// 过滤器只处理偶数ID
	bus.SubscribeWithFilter("bench.filter", func(event BenchEvent) {
		_ = event.ID * 2
	}, func(topic string, event BenchEvent) bool {
		return event.ID%2 == 0
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.filter", BenchEvent{
			ID:   i,
			Data: "test data",
		})
	}
}

func BenchmarkConcurrentSubscribeUnsubscribe(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			handle := bus.SubscribeWithHandle("bench.concurrent", func(event BenchEvent) {
				_ = event.ID * 2
			})
			
			// 发布一个事件
			bus.Publish("bench.concurrent", BenchEvent{
				ID:   1,
				Data: "test",
			})
			
			// 取消订阅
			handle.Unsubscribe()
		}
	})
}

func BenchmarkMemoryUsage(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	bus.SubscribeWithHandle("bench.memory", func(event BenchEvent) {
		// 最小处理
	})

	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		bus.Publish("bench.memory", BenchEvent{
			ID:   i,
			Data: "test data",
		})
	}
}

// 对比基准：传统的channel实现
func BenchmarkChannelBaseline(b *testing.B) {
	ch := make(chan BenchEvent, 1000)
	var wg sync.WaitGroup
	
	// 启动消费者
	wg.Add(1)
	go func() {
		defer wg.Done()
		for event := range ch {
			_ = event.ID * 2
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ch <- BenchEvent{
				ID:   i,
				Data: "test data",
			}
			i++
		}
	})
	
	close(ch)
	wg.Wait()
} 