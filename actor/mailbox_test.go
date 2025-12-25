package actor

import (
	"sync"
	"testing"
)

type testMessage struct {
	ID    int
	Value int
}

func TestMailbox_PushPop(t *testing.T) {
	mb := NewMailbox[testMessage](4)

	// Push messages
	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200})

	// Pop first message
	result := mb.Pop()
	if result == nil {
		t.Fatal("expected message, got nil")
	}
	if result.Message.ID != 1 || result.Message.Value != 100 {
		t.Errorf("expected {1, 100}, got {%d, %d}", result.Message.ID, result.Message.Value)
	}
	if result.DroppedSinceLastRead != 0 {
		t.Errorf("expected 0 dropped, got %d", result.DroppedSinceLastRead)
	}

	// Pop second message
	result = mb.Pop()
	if result == nil {
		t.Fatal("expected message, got nil")
	}
	if result.Message.ID != 2 || result.Message.Value != 200 {
		t.Errorf("expected {2, 200}, got {%d, %d}", result.Message.ID, result.Message.Value)
	}

	// Should be empty
	result = mb.Pop()
	if result != nil {
		t.Errorf("expected nil, got message")
	}
}

func TestMailbox_Empty(t *testing.T) {
	mb := NewMailbox[testMessage](10)
	if !mb.IsEmpty() {
		t.Error("new mailbox should be empty")
	}
	if mb.Len() != 0 {
		t.Errorf("new mailbox len should be 0, got %d", mb.Len())
	}
	result := mb.Pop()
	if result != nil {
		t.Error("pop from empty mailbox should return nil")
	}
}

func TestMailbox_Overflow(t *testing.T) {
	mb := NewMailbox[testMessage](2)

	// Fill to capacity
	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200})

	// These should be dropped
	mb.Push(testMessage{ID: 3, Value: 300})
	mb.Push(testMessage{ID: 4, Value: 400})
	mb.Push(testMessage{ID: 5, Value: 500})

	// Pop first - should report 3 dropped
	result := mb.Pop()
	if result == nil {
		t.Fatal("expected message, got nil")
	}
	if result.Message.ID != 1 {
		t.Errorf("expected ID 1, got %d", result.Message.ID)
	}
	if result.DroppedSinceLastRead != 3 {
		t.Errorf("expected 3 dropped, got %d", result.DroppedSinceLastRead)
	}

	// Pop second - dropped count should be reset
	result = mb.Pop()
	if result == nil {
		t.Fatal("expected message, got nil")
	}
	if result.Message.ID != 2 {
		t.Errorf("expected ID 2, got %d", result.Message.ID)
	}
	if result.DroppedSinceLastRead != 0 {
		t.Errorf("expected 0 dropped after reset, got %d", result.DroppedSinceLastRead)
	}
}

func TestMailbox_DroppedCounterReset(t *testing.T) {
	mb := NewMailbox[testMessage](1)

	// Fill mailbox
	mb.Push(testMessage{ID: 1, Value: 100})

	// Drop some messages
	mb.Push(testMessage{ID: 2, Value: 200})
	mb.Push(testMessage{ID: 3, Value: 300})

	// Pop - should report 2 dropped
	result := mb.Pop()
	if result == nil {
		t.Fatal("expected message")
	}
	if result.DroppedSinceLastRead != 2 {
		t.Errorf("expected 2 dropped, got %d", result.DroppedSinceLastRead)
	}

	// Add more messages
	mb.Push(testMessage{ID: 4, Value: 400})
	mb.Push(testMessage{ID: 5, Value: 500}) // This one drops

	// Pop - should report 1 dropped (not 3)
	result = mb.Pop()
	if result == nil {
		t.Fatal("expected message")
	}
	if result.Message.ID != 4 {
		t.Errorf("expected ID 4, got %d", result.Message.ID)
	}
	if result.DroppedSinceLastRead != 1 {
		t.Errorf("expected 1 dropped, got %d", result.DroppedSinceLastRead)
	}
}

func TestMailbox_RingBufferWrapAround(t *testing.T) {
	mb := NewMailbox[testMessage](3)

	// Fill the buffer
	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200})
	mb.Push(testMessage{ID: 3, Value: 300})

	// Pop two messages
	mb.Pop()
	mb.Pop()

	// Add two more (should wrap around)
	mb.Push(testMessage{ID: 4, Value: 400})
	mb.Push(testMessage{ID: 5, Value: 500})

	// Pop remaining in order
	result := mb.Pop()
	if result.Message.ID != 3 {
		t.Errorf("expected ID 3, got %d", result.Message.ID)
	}

	result = mb.Pop()
	if result.Message.ID != 4 {
		t.Errorf("expected ID 4, got %d", result.Message.ID)
	}

	result = mb.Pop()
	if result.Message.ID != 5 {
		t.Errorf("expected ID 5, got %d", result.Message.ID)
	}

	// Should be empty
	result = mb.Pop()
	if result != nil {
		t.Error("expected nil after draining")
	}
}

func TestMailbox_SingleCapacity(t *testing.T) {
	mb := NewMailbox[testMessage](1)

	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200}) // Dropped

	result := mb.Pop()
	if result == nil {
		t.Fatal("expected message")
	}
	if result.Message.ID != 1 {
		t.Errorf("expected ID 1, got %d", result.Message.ID)
	}
	if result.DroppedSinceLastRead != 1 {
		t.Errorf("expected 1 dropped, got %d", result.DroppedSinceLastRead)
	}

	result = mb.Pop()
	if result != nil {
		t.Error("expected nil after draining single-capacity mailbox")
	}
}

func TestMutexMailbox_Basic(t *testing.T) {
	mb := NewMutexMailbox[testMessage](4)

	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200})

	if mb.Len() != 2 {
		t.Errorf("expected len 2, got %d", mb.Len())
	}
	if mb.IsEmpty() {
		t.Error("expected not empty")
	}

	result := mb.Pop()
	if result == nil || result.Message.ID != 1 {
		t.Error("expected first message with ID 1")
	}

	result = mb.Pop()
	if result == nil || result.Message.ID != 2 {
		t.Error("expected second message with ID 2")
	}

	if !mb.IsEmpty() {
		t.Error("expected empty after draining")
	}
	if mb.Pop() != nil {
		t.Error("expected nil from empty mailbox")
	}
}

func TestMutexMailbox_Overflow(t *testing.T) {
	mb := NewMutexMailbox[testMessage](2)

	mb.Push(testMessage{ID: 1, Value: 100})
	mb.Push(testMessage{ID: 2, Value: 200})
	mb.Push(testMessage{ID: 3, Value: 300}) // Dropped
	mb.Push(testMessage{ID: 4, Value: 400}) // Dropped

	result := mb.Pop()
	if result == nil {
		t.Fatal("expected message")
	}
	if result.Message.ID != 1 {
		t.Errorf("expected ID 1, got %d", result.Message.ID)
	}
	if result.DroppedSinceLastRead != 2 {
		t.Errorf("expected 2 dropped, got %d", result.DroppedSinceLastRead)
	}

	result = mb.Pop()
	if result == nil || result.Message.ID != 2 {
		t.Error("expected second message with ID 2")
	}
	if result.DroppedSinceLastRead != 0 {
		t.Errorf("expected 0 dropped after reset, got %d", result.DroppedSinceLastRead)
	}
}

func TestMutexMailbox_ConcurrentProducers(t *testing.T) {
	mb := NewMutexMailbox[int](1000)
	const messagesPerThread = 100
	const numThreads = 4

	var wg sync.WaitGroup
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := 0; j < messagesPerThread; j++ {
				mb.Push(base + j)
			}
		}(i * messagesPerThread)
	}
	wg.Wait()

	// Should have all messages (capacity 1000 > 400 total)
	if mb.Len() != numThreads*messagesPerThread {
		t.Errorf("expected %d messages, got %d", numThreads*messagesPerThread, mb.Len())
	}

	// Drain and count
	count := 0
	for mb.Pop() != nil {
		count++
	}
	if count != numThreads*messagesPerThread {
		t.Errorf("expected to drain %d messages, got %d", numThreads*messagesPerThread, count)
	}
}

func BenchmarkMailbox_PushPop(b *testing.B) {
	mb := NewMailbox[testMessage](1000)
	msg := testMessage{ID: 1, Value: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Push(msg)
		mb.Pop()
	}
}

func BenchmarkMutexMailbox_PushPop(b *testing.B) {
	mb := NewMutexMailbox[testMessage](1000)
	msg := testMessage{ID: 1, Value: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mb.Push(msg)
		mb.Pop()
	}
}
