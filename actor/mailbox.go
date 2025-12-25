// Package actor provides a message-passing actor framework with mailboxes.
// Ported from havequick/platform/mailbox/mailbox.zig patterns.
package actor

import "sync"

// ReadResult contains a message and the count of messages dropped since the last read.
// This allows actors to detect backpressure and log overflow conditions.
type ReadResult[T any] struct {
	Message              T
	DroppedSinceLastRead uint64
}

// Mailbox is a ring buffer for actor messages with overflow tracking.
// Fixed-capacity ring buffer that drops messages when full and tracks
// the count of dropped messages.
type Mailbox[T any] struct {
	buffer       []T
	readIdx      int
	writeIdx     int
	count        int
	droppedCount uint64
	capacity     int
}

// NewMailbox creates a new mailbox with the given capacity.
func NewMailbox[T any](capacity int) *Mailbox[T] {
	return &Mailbox[T]{
		buffer:   make([]T, capacity),
		capacity: capacity,
	}
}

// Push adds a message to the mailbox.
// If the mailbox is full, the message is dropped and droppedCount is incremented.
func (m *Mailbox[T]) Push(msg T) {
	if m.count >= m.capacity {
		m.droppedCount++
		return
	}
	m.buffer[m.writeIdx] = msg
	m.writeIdx = (m.writeIdx + 1) % m.capacity
	m.count++
}

// Pop removes and returns the next message from the mailbox.
// Returns nil if the mailbox is empty.
// The returned ReadResult includes the dropped_since_last_read count,
// which is reset to 0 after being reported.
func (m *Mailbox[T]) Pop() *ReadResult[T] {
	if m.count == 0 {
		return nil
	}
	msg := m.buffer[m.readIdx]
	m.readIdx = (m.readIdx + 1) % m.capacity
	m.count--
	dropped := m.droppedCount
	m.droppedCount = 0
	return &ReadResult[T]{
		Message:              msg,
		DroppedSinceLastRead: dropped,
	}
}

// Len returns the current number of messages in the mailbox.
func (m *Mailbox[T]) Len() int {
	return m.count
}

// IsEmpty returns true if the mailbox has no messages.
func (m *Mailbox[T]) IsEmpty() bool {
	return m.count == 0
}

// MutexMailbox wraps a Mailbox with a mutex for thread-safe access.
// Use this when multiple goroutines need to push to the same mailbox.
type MutexMailbox[T any] struct {
	inner *Mailbox[T]
	mu    sync.Mutex
}

// NewMutexMailbox creates a new thread-safe mailbox with the given capacity.
func NewMutexMailbox[T any](capacity int) *MutexMailbox[T] {
	return &MutexMailbox[T]{
		inner: NewMailbox[T](capacity),
	}
}

// Push adds a message to the mailbox (thread-safe).
// If the mailbox is full, the message is dropped and droppedCount is incremented.
func (m *MutexMailbox[T]) Push(msg T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inner.Push(msg)
}

// Pop removes and returns the next message from the mailbox (thread-safe).
// Returns nil if the mailbox is empty.
func (m *MutexMailbox[T]) Pop() *ReadResult[T] {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.Pop()
}

// Len returns the current number of messages in the mailbox (thread-safe).
func (m *MutexMailbox[T]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.Len()
}

// IsEmpty returns true if the mailbox has no messages (thread-safe).
func (m *MutexMailbox[T]) IsEmpty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inner.IsEmpty()
}
