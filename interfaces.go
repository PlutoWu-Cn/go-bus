package bus

import (
	"context"
	"time"
)

// BusSubscriber defines subscription-related bus behavior
type BusSubscriber[T any] interface {
	Subscribe(topic string, fn func(T)) error
	SubscribeAsync(topic string, fn func(T), transactional bool) error
	SubscribeOnce(topic string, fn func(T)) error
	SubscribeOnceAsync(topic string, fn func(T)) error
	SubscribeWithHandle(topic string, fn func(T)) *Handle[T]
	SubscribeAsyncWithHandle(topic string, fn func(T), transactional bool) *Handle[T]
	SubscribeWithPriority(topic string, fn func(T), priority Priority) *Handle[T]
	SubscribeWithFilter(topic string, fn func(T), filter EventFilter[T]) *Handle[T]
	SubscribeWithContext(ctx context.Context, topic string, fn func(T)) *Handle[T]
}

// BusPublisher defines publishing-related bus behavior
type BusPublisher[T any] interface {
	Publish(topic string, event T)
	PublishWithContext(ctx context.Context, topic string, event T) error
	PublishWithTimeout(topic string, event T, timeout time.Duration) error
}

// BusController defines bus control behavior
type BusController interface {
	HasCallback(topic string) bool
	WaitAsync()
	GetMetrics() Metrics
	SetErrorHandler(handler ErrorHandler)
	AddMiddleware(middleware EventMiddleware[any])
	SetLogger(logger Logger)
	GetLogger() Logger
	GetTopics() []string
	GetSubscriberCount(topic string) int
	Close() error
}

// Bus englobes global (subscribe, publish, control) bus behavior
type Bus[T any] interface {
	BusController
	BusSubscriber[T]
	BusPublisher[T]
}
