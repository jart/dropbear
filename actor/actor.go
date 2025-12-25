package actor

import (
	"log"
	"sync/atomic"
)

// Message is the interface all actor messages must implement.
// Empty interface allows any type to be a message.
type Message interface{}

// Handler processes incoming messages and produces outgoing messages.
// This is the core computation unit in the actor model.
type Handler[In, Out any] interface {
	// Handle processes an incoming message and returns zero or more output messages.
	Handle(msg In) []Out
}

// HandlerFunc is a function adapter for Handler.
type HandlerFunc[In, Out any] func(msg In) []Out

func (f HandlerFunc[In, Out]) Handle(msg In) []Out {
	return f(msg)
}

// Actor is a message-processing unit with its own mailbox.
// Messages are received via the mailbox and dispatched to a handler.
type Actor[In, Out any] struct {
	ID       uint64
	mailbox  *MutexMailbox[In]
	handler  Handler[In, Out]
	router   Router[Out]
	running  atomic.Bool
	capacity int
}

// Router delivers output messages to their destinations.
type Router[T any] interface {
	Route(msg T)
}

// RouterFunc is a function adapter for Router.
type RouterFunc[T any] func(msg T)

func (f RouterFunc[T]) Route(msg T) {
	f(msg)
}

// NullRouter discards all messages (useful for testing).
type NullRouter[T any] struct{}

func (NullRouter[T]) Route(msg T) {}

// Config configures actor creation.
type Config struct {
	ID       uint64
	Capacity int // mailbox capacity, default 1000
}

// NewActor creates a new actor with the given handler and optional router.
func NewActor[In, Out any](handler Handler[In, Out], router Router[Out], cfg Config) *Actor[In, Out] {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 1000
	}
	return &Actor[In, Out]{
		ID:       cfg.ID,
		mailbox:  NewMutexMailbox[In](capacity),
		handler:  handler,
		router:   router,
		capacity: capacity,
	}
}

// Send adds a message to the actor's mailbox.
func (a *Actor[In, Out]) Send(msg In) {
	a.mailbox.Push(msg)
}

// Step processes a single message from the mailbox.
// Returns true if a message was processed, false if mailbox was empty.
// This is useful for single-threaded or event-loop based processing.
func (a *Actor[In, Out]) Step() bool {
	result := a.mailbox.Pop()
	if result == nil {
		return false
	}

	// Log dropped messages if any
	if result.DroppedSinceLastRead > 0 {
		log.Printf("[actor %d] dropped %d messages", a.ID, result.DroppedSinceLastRead)
	}

	// Process message and route outputs
	outputs := a.handler.Handle(result.Message)
	for _, out := range outputs {
		if a.router != nil {
			a.router.Route(out)
		}
	}

	return true
}

// Run processes messages continuously until Stop is called.
// This should be called in its own goroutine.
func (a *Actor[In, Out]) Run() {
	a.running.Store(true)
	for a.running.Load() {
		if !a.Step() {
			// No message available, could add backoff here
			// For now, just spin (caller should use Step() for control)
		}
	}
}

// Stop signals the actor to stop processing.
func (a *Actor[In, Out]) Stop() {
	a.running.Store(false)
}

// IsRunning returns true if the actor is running.
func (a *Actor[In, Out]) IsRunning() bool {
	return a.running.Load()
}

// Len returns the number of messages in the mailbox.
func (a *Actor[In, Out]) Len() int {
	return a.mailbox.Len()
}

// IsEmpty returns true if the mailbox is empty.
func (a *Actor[In, Out]) IsEmpty() bool {
	return a.mailbox.IsEmpty()
}

// Drain processes all messages currently in the mailbox.
// Returns the number of messages processed.
func (a *Actor[In, Out]) Drain() int {
	count := 0
	for a.Step() {
		count++
	}
	return count
}
