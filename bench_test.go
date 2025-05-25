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
		// Simple processing
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
		// Simple processing
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

	// Create multiple subscribers
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

	// Subscribers with different priorities
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

	// Filter only processes even IDs
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

			// Publish an event
			bus.Publish("bench.concurrent", BenchEvent{
				ID:   1,
				Data: "test",
			})

			// Unsubscribe
			handle.Unsubscribe()
		}
	})
}

func BenchmarkMemoryUsage(b *testing.B) {
	bus := NewTyped[BenchEvent]()
	defer bus.Close()

	bus.SubscribeWithHandle("bench.memory", func(event BenchEvent) {
		// Minimal processing
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

// Baseline comparison: traditional channel implementation
func BenchmarkChannelBaseline(b *testing.B) {
	ch := make(chan BenchEvent, 1000)
	var wg sync.WaitGroup

	// Start consumer
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
