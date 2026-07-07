package bus

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// EventBus - box for handlers and callbacks.
type EventBus[T any] struct {
	handlers     map[string][]*eventHandler[T]
	middlewares  []EventMiddleware[any]
	errorHandler ErrorHandler
	metrics      Metrics
	logger       Logger
	lock         sync.RWMutex
	wg           sync.WaitGroup
	closed       bool
	closeCh      chan struct{}
}

// Option defines a functional option for EventBus
type Option[T any] func(*EventBus[T])

// WithMetrics allows custom Metrics implementation
func WithMetrics[T any](metrics Metrics) Option[T] {
	return func(b *EventBus[T]) {
		if metrics == nil {
			return
		}
		b.metrics = metrics
	}
}

// WithLogger sets a custom logger for the EventBus
func WithLogger[T any](logger Logger) Option[T] {
	return func(b *EventBus[T]) {
		b.logger = logger
	}
}

// WithErrorHandler sets a custom error handler for the EventBus
func WithErrorHandler[T any](handler ErrorHandler) Option[T] {
	return func(b *EventBus[T]) {
		b.errorHandler = handler
	}
}

// WithMiddleware adds a middleware to the EventBus
func WithMiddleware[T any](middleware EventMiddleware[any]) Option[T] {
	return func(b *EventBus[T]) {
		if middleware == nil {
			return
		}
		b.middlewares = append(b.middlewares, middleware)
	}
}

// NewTyped returns new EventBus with empty handlers for the specified type.
func NewTyped[T any](opts ...Option[T]) Bus[T] {
	b := &EventBus[T]{
		handlers:    make(map[string][]*eventHandler[T]),
		middlewares: make([]EventMiddleware[any], 0),
		metrics:     &DefaultMetrics{},
		logger:      NewDefaultLogger(),
		lock:        sync.RWMutex{},
		wg:          sync.WaitGroup{},
		closed:      false,
		closeCh:     make(chan struct{}),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(b)
	}
	if b.metrics == nil {
		b.metrics = &DefaultMetrics{}
	}
	return Bus[T](b)
}

// New returns new EventBus with empty handlers (for compatibility, uses any type).
func New(opts ...Option[any]) Bus[any] {
	return NewTyped[any](opts...)
}

// doSubscribe handles the subscription logic and is utilized by the public Subscribe functions
func (bus *EventBus[T]) doSubscribe(topic string, fn func(T), handler *eventHandler[T]) error {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if err := bus.addHandlerLocked(topic, fn, handler); err != nil {
		return err
	}
	return nil
}

// doSubscribeWithHandle handles the subscription logic and returns a handle
func (bus *EventBus[T]) doSubscribeWithHandle(topic string, fn func(T), handler *eventHandler[T]) *Handle[T] {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if bus.addHandlerLocked(topic, fn, handler) != nil {
		return nil
	}

	return &Handle[T]{
		bus:      bus,
		topic:    topic,
		handler:  handler,
		priority: handler.priority,
		filter:   handler.filter,
		ctx:      handler.ctx,
	}
}

func (bus *EventBus[T]) addHandlerLocked(topic string, fn func(T), handler *eventHandler[T]) error {
	if fn == nil {
		return fmt.Errorf("event handler is nil")
	}
	if bus.closed {
		return fmt.Errorf("event bus is closed")
	}
	if handler.ctx == nil {
		handler.ctx = context.Background()
	}
	handler.topic = topic

	handlers := bus.handlers[topic]
	inserted := false

	for i, h := range handlers {
		if handler.priority > h.priority {
			handlers = append(handlers[:i], append([]*eventHandler[T]{handler}, handlers[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		handlers = append(handlers, handler)
	}

	bus.handlers[topic] = handlers
	bus.metrics.IncrementSubscribers()

	if bus.logger != nil {
		bus.logger.Debug("Handler subscribed to topic '%s' with priority %v", topic, handler.priority)
	}
	return nil
}

// Subscribe subscribes to a topic.
func (bus *EventBus[T]) Subscribe(topic string, fn func(T)) error {
	return bus.doSubscribe(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         false,
		transactional: false,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeWithHandle subscribes to a topic and returns a handle for unsubscription.
func (bus *EventBus[T]) SubscribeWithHandle(topic string, fn func(T)) *Handle[T] {
	return bus.doSubscribeWithHandle(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         false,
		transactional: false,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeWithPriority subscribes to a topic with specified priority
func (bus *EventBus[T]) SubscribeWithPriority(topic string, fn func(T), priority Priority) *Handle[T] {
	return bus.doSubscribeWithHandle(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         false,
		transactional: false,
		priority:      priority,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeWithFilter subscribes to a topic with an event filter
func (bus *EventBus[T]) SubscribeWithFilter(topic string, fn func(T), filter EventFilter[T]) *Handle[T] {
	return bus.doSubscribeWithHandle(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         false,
		transactional: false,
		priority:      PriorityNormal,
		filter:        filter,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeWithContext subscribes to a topic with context for cancellation
func (bus *EventBus[T]) SubscribeWithContext(ctx context.Context, topic string, fn func(T)) *Handle[T] {
	return bus.doSubscribeWithHandle(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         false,
		transactional: false,
		priority:      PriorityNormal,
		ctx:           ctx,
		Mutex:         sync.Mutex{},
	})
}

// SubscribeAsync subscribes to a topic with an asynchronous callback
func (bus *EventBus[T]) SubscribeAsync(topic string, fn func(T), transactional bool) error {
	return bus.doSubscribe(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         true,
		transactional: transactional,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeAsyncWithHandle subscribes to a topic with an asynchronous callback and returns a handle.
func (bus *EventBus[T]) SubscribeAsyncWithHandle(topic string, fn func(T), transactional bool) *Handle[T] {
	return bus.doSubscribeWithHandle(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      false,
		async:         true,
		transactional: transactional,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeOnce subscribes to a topic once. Handler will be removed after executing.
func (bus *EventBus[T]) SubscribeOnce(topic string, fn func(T)) error {
	return bus.doSubscribe(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      true,
		async:         false,
		transactional: false,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// SubscribeOnceAsync subscribes to a topic once with an asynchronous callback
func (bus *EventBus[T]) SubscribeOnceAsync(topic string, fn func(T)) error {
	return bus.doSubscribe(topic, fn, &eventHandler[T]{
		callBack:      fn,
		flagOnce:      true,
		async:         true,
		transactional: false,
		priority:      PriorityNormal,
		ctx:           context.Background(),
		Mutex:         sync.Mutex{},
	})
}

// HasCallback returns true if exists any callback subscribed to the topic.
func (bus *EventBus[T]) HasCallback(topic string) bool {
	bus.lock.RLock()
	defer bus.lock.RUnlock()
	_, ok := bus.handlers[topic]
	if ok {
		return len(bus.handlers[topic]) > 0
	}
	return false
}

// Publish executes callback defined for a topic.
func (bus *EventBus[T]) Publish(topic string, event T) {
	_ = bus.PublishWithContext(context.Background(), topic, event)
}

// PublishWithContext publishes an event with context
func (bus *EventBus[T]) PublishWithContext(ctx context.Context, topic string, event T) error {
	bus.lock.RLock()
	if bus.closed {
		errorHandler := bus.errorHandler
		bus.lock.RUnlock()
		err := fmt.Errorf("event bus is closed")
		if errorHandler != nil {
			errorHandler(&EventError{
				Topic: topic,
				Event: event,
				Err:   err,
			})
		}
		return err
	}
	handlers := append([]*eventHandler[T](nil), bus.handlers[topic]...)
	if topic != "*" {
		if wildcardHandlers := bus.handlers["*"]; len(wildcardHandlers) > 0 {
			handlers = append(handlers, wildcardHandlers...)
			sort.SliceStable(handlers, func(i, j int) bool {
				return handlers[i].priority > handlers[j].priority
			})
		}
	}
	middlewares := append([]EventMiddleware[any](nil), bus.middlewares...)
	errorHandler := bus.errorHandler
	bus.lock.RUnlock()

	// Log event publishing
	if bus.logger != nil {
		bus.logger.Debug("Publishing event to topic '%s'", topic)
	}

	bus.metrics.IncrementPublished()

	runHandlers := func() error {
		return bus.publishHandlers(ctx, topic, event, handlers)
	}
	var publishErr error
	next := func() {
		publishErr = runHandlers()
	}

	for i := len(middlewares) - 1; i >= 0; i-- {
		middleware := middlewares[i]
		prev := next
		next = func() {
			if publishErr == nil {
				if err := middleware(topic, event, prev); err != nil {
					publishErr = err
				}
			}
		}
	}
	next()

	if publishErr != nil {
		if errorHandler != nil {
			errorHandler(&EventError{
				Topic: topic,
				Event: event,
				Err:   publishErr,
			})
		}
		return publishErr
	}

	return nil
}

func (bus *EventBus[T]) publishHandlers(ctx context.Context, topic string, event T, handlers []*eventHandler[T]) error {
	_, hasDeadline := ctx.Deadline()

	for _, handler := range handlers {
		handler := handler

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-bus.closeCh:
			return fmt.Errorf("event bus is closed")
		default:
		}

		if handler.ctx != nil {
			select {
			case <-handler.ctx.Done():
				continue
			default:
			}
		}

		if handler.filter != nil && !handler.filter(topic, event) {
			continue
		}

		if handler.flagOnce {
			if !handler.ranOnce.CompareAndSwap(false, true) {
				continue
			}
			bus.removeHandler(handler.topic, handler)
		}

		if !handler.async {
			if !hasDeadline {
				bus.doPublish(handler, topic, event)
				continue
			}

			done := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						if bus.errorHandler != nil {
							bus.errorHandler(&EventError{
								Topic:   topic,
								Event:   event,
								Handler: handler.callBack,
								Err:     fmt.Errorf("panic: %v", r),
							})
						}
						bus.metrics.IncrementFailed()
						done <- fmt.Errorf("panic: %v", r)
						return
					}
					bus.metrics.IncrementProcessed()
					done <- nil
				}()
				handler.callBack(event)
			}()

			select {
			case err := <-done:
				if err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			case <-bus.closeCh:
				return fmt.Errorf("event bus is closed")
			}
			continue
		}

		bus.lock.RLock()
		if bus.closed {
			bus.lock.RUnlock()
			return fmt.Errorf("event bus is closed")
		}
		bus.wg.Add(1)
		bus.lock.RUnlock()

		if handler.transactional {
			handler.Lock()
		}
		go bus.doPublishAsync(handler, topic, event)
	}
	return nil
}

// PublishWithTimeout limits how long PublishWithContext waits; it cannot interrupt a running handler.
func (bus *EventBus[T]) PublishWithTimeout(topic string, event T, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return bus.PublishWithContext(ctx, topic, event)
}

func (bus *EventBus[T]) doPublish(handler *eventHandler[T], topic string, event T) {
	defer func() {
		if r := recover(); r != nil {
			if bus.logger != nil {
				bus.logger.Error("Handler panic for topic '%s': %v", topic, r)
			}
			if bus.errorHandler != nil {
				bus.errorHandler(&EventError{
					Topic:   topic,
					Event:   event,
					Handler: handler.callBack,
					Err:     fmt.Errorf("panic: %v", r),
				})
			}
			bus.metrics.IncrementFailed()
			return
		}
		if bus.logger != nil {
			bus.logger.Debug("Handler executed successfully for topic '%s'", topic)
		}
		bus.metrics.IncrementProcessed()
	}()

	handler.callBack(event)
}

func (bus *EventBus[T]) doPublishAsync(handler *eventHandler[T], topic string, event T) {
	defer bus.wg.Done()
	defer func() {
		if handler.transactional {
			handler.Unlock()
		}
	}()

	bus.doPublish(handler, topic, event)
}

func (bus *EventBus[T]) removeHandler(topic string, target *eventHandler[T]) bool {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if _, ok := bus.handlers[topic]; !ok {
		return false
	}

	for idx, handler := range bus.handlers[topic] {
		if handler != target {
			continue
		}
		l := len(bus.handlers[topic])
		copy(bus.handlers[topic][idx:], bus.handlers[topic][idx+1:])
		bus.handlers[topic][l-1] = nil
		bus.handlers[topic] = bus.handlers[topic][:l-1]
		if len(bus.handlers[topic]) == 0 {
			delete(bus.handlers, topic)
		}
		bus.metrics.DecrementSubscribers()

		// Log handler removal
		if bus.logger != nil {
			bus.logger.Debug("Handler removed from topic '%s'", topic)
		}
		return true
	}
	return false
}

// WaitAsync waits for all async callbacks to complete
func (bus *EventBus[T]) WaitAsync() {
	bus.wg.Wait()
}

// GetMetrics returns the current metrics
func (bus *EventBus[T]) GetMetrics() Metrics {
	return bus.metrics
}

// SetErrorHandler sets the error handler for the bus
func (bus *EventBus[T]) SetErrorHandler(handler ErrorHandler) {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	bus.errorHandler = handler
}

// AddMiddleware adds middleware to the bus
func (bus *EventBus[T]) AddMiddleware(middleware EventMiddleware[any]) {
	if middleware == nil {
		return
	}

	bus.lock.Lock()
	defer bus.lock.Unlock()
	bus.middlewares = append(bus.middlewares, middleware)
}

// SetLogger sets the logger for the bus
func (bus *EventBus[T]) SetLogger(logger Logger) {
	bus.lock.Lock()
	defer bus.lock.Unlock()
	bus.logger = logger
}

// GetLogger returns the current logger
func (bus *EventBus[T]) GetLogger() Logger {
	bus.lock.RLock()
	defer bus.lock.RUnlock()
	return bus.logger
}

// GetTopics returns all topics that have subscribers
func (bus *EventBus[T]) GetTopics() []string {
	bus.lock.RLock()
	defer bus.lock.RUnlock()

	topics := make([]string, 0, len(bus.handlers))
	for topic := range bus.handlers {
		topics = append(topics, topic)
	}
	return topics
}

// GetSubscriberCount returns the number of subscribers for a topic
func (bus *EventBus[T]) GetSubscriberCount(topic string) int {
	bus.lock.RLock()
	defer bus.lock.RUnlock()

	if handlers, ok := bus.handlers[topic]; ok {
		return len(handlers)
	}
	return 0
}

// Close gracefully shuts down the event bus
func (bus *EventBus[T]) Close() error {
	bus.lock.Lock()

	if bus.closed {
		bus.lock.Unlock()
		return fmt.Errorf("event bus already closed")
	}

	bus.closed = true
	close(bus.closeCh)
	bus.lock.Unlock()

	// Wait for all async operations to complete
	bus.wg.Wait()

	// Clear all handlers
	bus.lock.Lock()
	subscriberCount := 0
	for _, handlers := range bus.handlers {
		subscriberCount += len(handlers)
	}
	bus.handlers = make(map[string][]*eventHandler[T])
	bus.lock.Unlock()

	for i := 0; i < subscriberCount; i++ {
		bus.metrics.DecrementSubscribers()
	}

	return nil
}
