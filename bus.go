package bus

import (
	"context"
	"fmt"
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
		opt(b)
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

	if bus.closed {
		return fmt.Errorf("event bus is closed")
	}

	// Insert handler based on priority
	handlers := bus.handlers[topic]
	inserted := false

	for i, h := range handlers {
		if handler.priority > h.priority {
			// Insert before this handler
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

	// Log subscription
	if bus.logger != nil {
		bus.logger.Debug("Handler subscribed to topic '%s' with priority %v", topic, handler.priority)
	}

	return nil
}

// doSubscribeWithHandle handles the subscription logic and returns a handle
func (bus *EventBus[T]) doSubscribeWithHandle(topic string, fn func(T), handler *eventHandler[T]) *Handle[T] {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if bus.closed {
		return nil
	}

	// Insert handler based on priority (same logic as doSubscribe)
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

	// Log subscription
	if bus.logger != nil {
		bus.logger.Debug("Handler subscribed to topic '%s' with priority %v (with handle)", topic, handler.priority)
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
	bus.PublishWithContext(context.Background(), topic, event)
}

// PublishWithContext publishes an event with context
func (bus *EventBus[T]) PublishWithContext(ctx context.Context, topic string, event T) error {
	bus.lock.RLock()
	if bus.closed {
		bus.lock.RUnlock()
		return fmt.Errorf("event bus is closed")
	}
	logger := bus.logger
	errorHandler := bus.errorHandler
	metrics := bus.metrics
	closeCh := bus.closeCh
	middlewares := append([]EventMiddleware[any](nil), bus.middlewares...)
	handlers := append([]*eventHandler[T](nil), bus.handlers[topic]...)
	bus.lock.RUnlock()

	// Log event publishing
	if logger != nil {
		logger.Debug("Publishing event to topic '%s'", topic)
	}

	metrics.IncrementPublished()

	dispatch := func() error {
		return bus.dispatch(ctx, topic, event, handlers, closeCh, logger, errorHandler, metrics)
	}
	dispatchErr, middlewareErr := runMiddlewares(middlewares, topic, event, dispatch)
	if middlewareErr != nil {
		if errorHandler != nil {
			errorHandler(&EventError{
				Topic: topic,
				Event: event,
				Err:   middlewareErr,
			})
		}
		return middlewareErr
	}
	return dispatchErr
}

func runMiddlewares(middlewares []EventMiddleware[any], topic string, event any, dispatch func() error) (dispatchErr, middlewareErr error) {
	var run func(int)
	run = func(i int) {
		if middlewareErr != nil {
			return
		}
		if i == len(middlewares) {
			dispatchErr = dispatch()
			return
		}
		if err := middlewares[i](topic, event, func() { run(i + 1) }); err != nil {
			middlewareErr = err
		}
	}
	run(0)
	return dispatchErr, middlewareErr
}

func (bus *EventBus[T]) dispatch(ctx context.Context, topic string, event T, handlers []*eventHandler[T], closeCh <-chan struct{}, logger Logger, errorHandler ErrorHandler, metrics Metrics) error {
	_, hasDeadline := ctx.Deadline()

	for _, handler := range handlers {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-closeCh:
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

		if handler.flagOnce && !bus.removeHandler(topic, handler) {
			continue
		}

		if !handler.async {
			if hasDeadline {
				done := make(chan error, 1)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							if errorHandler != nil {
								errorHandler(&EventError{
									Topic:   topic,
									Event:   event,
									Handler: handler.callBack,
									Err:     fmt.Errorf("panic: %v", r),
								})
							}
							metrics.IncrementFailed()
							done <- fmt.Errorf("panic: %v", r)
						} else {
							metrics.IncrementProcessed()
							done <- nil
						}
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
				case <-closeCh:
					return fmt.Errorf("event bus is closed")
				}
			} else {
				bus.doPublish(handler, topic, event, logger, errorHandler, metrics)
			}
			continue
		}

		if !bus.addAsync() {
			return fmt.Errorf("event bus is closed")
		}
		if handler.transactional {
			handler.Lock()
		}
		go bus.doPublishAsync(handler, topic, event, logger, errorHandler, metrics)
	}

	return nil
}

func (bus *EventBus[T]) addAsync() bool {
	bus.lock.RLock()
	defer bus.lock.RUnlock()
	if bus.closed {
		return false
	}
	bus.wg.Add(1)
	return true
}

func (bus *EventBus[T]) removeHandler(topic string, handler *eventHandler[T]) bool {
	bus.lock.Lock()
	defer bus.lock.Unlock()

	if handlers, ok := bus.handlers[topic]; ok {
		for i, h := range handlers {
			if h == handler {
				return bus.removeHandlerAt(topic, i)
			}
		}
	}
	return false
}

// PublishWithTimeout publishes an event with timeout
func (bus *EventBus[T]) PublishWithTimeout(topic string, event T, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return bus.PublishWithContext(ctx, topic, event)
}

func (bus *EventBus[T]) doPublish(handler *eventHandler[T], topic string, event T, logger Logger, errorHandler ErrorHandler, metrics Metrics) {
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Error("Handler panic for topic '%s': %v", topic, r)
			}
			if errorHandler != nil {
				errorHandler(&EventError{
					Topic:   topic,
					Event:   event,
					Handler: handler.callBack,
					Err:     fmt.Errorf("panic: %v", r),
				})
			}
			metrics.IncrementFailed()
			return
		}
		if logger != nil {
			logger.Debug("Handler executed successfully for topic '%s'", topic)
		}
		metrics.IncrementProcessed()
	}()

	handler.callBack(event)
}

func (bus *EventBus[T]) doPublishAsync(handler *eventHandler[T], topic string, event T, logger Logger, errorHandler ErrorHandler, metrics Metrics) {
	defer bus.wg.Done()
	defer func() {
		if handler.transactional {
			handler.Unlock()
		}
	}()

	bus.doPublish(handler, topic, event, logger, errorHandler, metrics)
}

func (bus *EventBus[T]) removeHandlerAt(topic string, idx int) bool {
	if _, ok := bus.handlers[topic]; !ok {
		return false
	}
	l := len(bus.handlers[topic])

	if !(0 <= idx && idx < l) {
		return false
	}

	copy(bus.handlers[topic][idx:], bus.handlers[topic][idx+1:])
	bus.handlers[topic][l-1] = nil
	bus.handlers[topic] = bus.handlers[topic][:l-1]
	bus.metrics.DecrementSubscribers()

	// Log handler removal
	if bus.logger != nil {
		bus.logger.Debug("Handler removed from topic '%s'", topic)
	}
	return true
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
	bus.handlers = make(map[string][]*eventHandler[T])
	bus.lock.Unlock()

	// Wait for all async operations to complete
	bus.wg.Wait()

	return nil
}
