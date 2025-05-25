package bus

import (
	"context"
	"fmt"
	"sync"
)

// Handle represents a subscription handle that can be used to unsubscribe
type Handle[T any] struct {
	bus      *EventBus[T]
	topic    string
	handler  *eventHandler[T]
	priority Priority
	filter   EventFilter[T]
	ctx      context.Context
	mu       sync.Mutex
}

// Unsubscribe removes this specific subscription
func (h *Handle[T]) Unsubscribe() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.handler == nil {
		return fmt.Errorf("handle already unsubscribed")
	}

	h.bus.lock.Lock()
	defer h.bus.lock.Unlock()

	if handlers, ok := h.bus.handlers[h.topic]; ok {
		for i, handler := range handlers {
			if handler == h.handler {
				// Log unsubscription
				if h.bus.logger != nil {
					h.bus.logger.Debug("Unsubscribing handler from topic '%s'", h.topic)
				}
				h.bus.removeHandler(h.topic, i)
				h.handler = nil
				// Note: removeHandler already calls DecrementSubscribers, so we don't call it again
				return nil
			}
		}
	}

	return fmt.Errorf("handler not found for topic %s", h.topic)
}

// IsActive returns whether this handle is still active
func (h *Handle[T]) IsActive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.handler != nil
}

// eventHandler represents an internal event handler
type eventHandler[T any] struct {
	callBack      func(T)
	flagOnce      bool
	async         bool
	transactional bool
	priority      Priority
	filter        EventFilter[T]
	ctx           context.Context
	sync.Mutex    // lock for an event handler - useful for running async callbacks serially
}
