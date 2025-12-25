package actor

import (
	"sync"
	"testing"
)

type incrementHandler struct {
	processed int
}

func (h *incrementHandler) Handle(msg int) []int {
	h.processed++
	return []int{msg + 1}
}

type collectRouter struct {
	mu       sync.Mutex
	messages []int
}

func (r *collectRouter) Route(msg int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *collectRouter) Get() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int{}, r.messages...)
}

func TestActor_SendAndStep(t *testing.T) {
	handler := &incrementHandler{}
	router := &collectRouter{}
	actor := NewActor[int, int](handler, router, Config{ID: 1})

	// Send messages
	actor.Send(1)
	actor.Send(2)
	actor.Send(3)

	if actor.Len() != 3 {
		t.Errorf("expected 3 messages, got %d", actor.Len())
	}

	// Process one message
	if !actor.Step() {
		t.Error("expected Step to return true")
	}
	if handler.processed != 1 {
		t.Errorf("expected 1 processed, got %d", handler.processed)
	}

	msgs := router.Get()
	if len(msgs) != 1 || msgs[0] != 2 {
		t.Errorf("expected [2], got %v", msgs)
	}

	// Process remaining
	actor.Drain()
	if handler.processed != 3 {
		t.Errorf("expected 3 processed, got %d", handler.processed)
	}

	msgs = router.Get()
	if len(msgs) != 3 {
		t.Errorf("expected 3 routed, got %d", len(msgs))
	}
}

func TestActor_EmptyStep(t *testing.T) {
	handler := &incrementHandler{}
	actor := NewActor[int, int](handler, NullRouter[int]{}, Config{ID: 1})

	if actor.Step() {
		t.Error("expected Step to return false on empty mailbox")
	}
	if handler.processed != 0 {
		t.Error("expected no messages processed")
	}
}

func TestActor_Drain(t *testing.T) {
	handler := &incrementHandler{}
	router := &collectRouter{}
	actor := NewActor[int, int](handler, router, Config{ID: 1})

	// Send 10 messages
	for i := 0; i < 10; i++ {
		actor.Send(i)
	}

	// Drain all
	count := actor.Drain()
	if count != 10 {
		t.Errorf("expected 10 drained, got %d", count)
	}
	if !actor.IsEmpty() {
		t.Error("expected empty after drain")
	}

	msgs := router.Get()
	if len(msgs) != 10 {
		t.Errorf("expected 10 routed, got %d", len(msgs))
	}
}

func TestActor_HandlerFunc(t *testing.T) {
	// Test using HandlerFunc adapter
	handler := HandlerFunc[int, int](func(msg int) []int {
		return []int{msg * 2}
	})
	router := &collectRouter{}
	actor := NewActor[int, int](handler, router, Config{ID: 1})

	actor.Send(5)
	actor.Step()

	msgs := router.Get()
	if len(msgs) != 1 || msgs[0] != 10 {
		t.Errorf("expected [10], got %v", msgs)
	}
}

func TestActor_NoOutputs(t *testing.T) {
	// Handler that produces no outputs
	handler := HandlerFunc[int, int](func(msg int) []int {
		return nil
	})
	router := &collectRouter{}
	actor := NewActor[int, int](handler, router, Config{ID: 1})

	actor.Send(1)
	actor.Step()

	msgs := router.Get()
	if len(msgs) != 0 {
		t.Errorf("expected no messages, got %v", msgs)
	}
}

func TestActor_MultipleOutputs(t *testing.T) {
	// Handler that produces multiple outputs
	handler := HandlerFunc[int, int](func(msg int) []int {
		return []int{msg, msg * 2, msg * 3}
	})
	router := &collectRouter{}
	actor := NewActor[int, int](handler, router, Config{ID: 1})

	actor.Send(5)
	actor.Step()

	msgs := router.Get()
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
	expected := []int{5, 10, 15}
	for i, m := range expected {
		if msgs[i] != m {
			t.Errorf("expected msgs[%d]=%d, got %d", i, m, msgs[i])
		}
	}
}

func TestActor_NilRouter(t *testing.T) {
	handler := HandlerFunc[int, int](func(msg int) []int {
		return []int{msg + 1}
	})
	actor := NewActor[int, int](handler, nil, Config{ID: 1})

	actor.Send(1)
	// Should not panic with nil router
	actor.Step()
}

func TestActor_DefaultCapacity(t *testing.T) {
	handler := HandlerFunc[int, int](func(msg int) []int { return nil })
	actor := NewActor[int, int](handler, nil, Config{ID: 1})

	if actor.capacity != 1000 {
		t.Errorf("expected default capacity 1000, got %d", actor.capacity)
	}
}

func TestActor_CustomCapacity(t *testing.T) {
	handler := HandlerFunc[int, int](func(msg int) []int { return nil })
	actor := NewActor[int, int](handler, nil, Config{ID: 1, Capacity: 50})

	if actor.capacity != 50 {
		t.Errorf("expected capacity 50, got %d", actor.capacity)
	}
}

type stringMsg struct {
	Text string
}

type upperHandler struct{}

func (h upperHandler) Handle(msg stringMsg) []stringMsg {
	return []stringMsg{{Text: "UPPER:" + msg.Text}}
}

func TestActor_CustomMessageTypes(t *testing.T) {
	router := &struct {
		mu   sync.Mutex
		msgs []stringMsg
	}{}
	routerFn := RouterFunc[stringMsg](func(msg stringMsg) {
		router.mu.Lock()
		defer router.mu.Unlock()
		router.msgs = append(router.msgs, msg)
	})

	actor := NewActor[stringMsg, stringMsg](upperHandler{}, routerFn, Config{ID: 1})

	actor.Send(stringMsg{Text: "hello"})
	actor.Step()

	router.mu.Lock()
	if len(router.msgs) != 1 || router.msgs[0].Text != "UPPER:hello" {
		t.Errorf("expected UPPER:hello, got %v", router.msgs)
	}
	router.mu.Unlock()
}

func BenchmarkActor_SendStep(b *testing.B) {
	handler := HandlerFunc[int, int](func(msg int) []int {
		return []int{msg + 1}
	})
	actor := NewActor[int, int](handler, NullRouter[int]{}, Config{ID: 1, Capacity: 10000})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		actor.Send(i)
		actor.Step()
	}
}

func BenchmarkActor_Batch(b *testing.B) {
	handler := HandlerFunc[int, int](func(msg int) []int {
		return []int{msg + 1}
	})
	actor := NewActor[int, int](handler, NullRouter[int]{}, Config{ID: 1, Capacity: 10000})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Send batch
		for j := 0; j < 100; j++ {
			actor.Send(j)
		}
		// Drain batch
		actor.Drain()
	}
}
