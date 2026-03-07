package fetchlock

import (
	"sync"
	"testing"
)

func TestNew_ReturnsNonNil(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestGet_SameSlugReturnsSameMutex(t *testing.T) {
	m := New()
	mu1 := m.Get("alpha")
	mu2 := m.Get("alpha")
	if mu1 != mu2 {
		t.Fatal("Get() returned different mutexes for the same slug")
	}
}

func TestGet_DifferentSlugsReturnDifferentMutexes(t *testing.T) {
	m := New()
	mu1 := m.Get("alpha")
	mu2 := m.Get("beta")
	if mu1 == mu2 {
		t.Fatal("Get() returned the same mutex for different slugs")
	}
}

func TestGet_ConcurrentAccessIsSafe(t *testing.T) {
	m := New()
	const goroutines = 100
	slugs := []string{"a", "b", "c", "d", "e"}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			slug := slugs[i%len(slugs)]
			mu := m.Get(slug)
			if mu == nil {
				t.Error("Get() returned nil under concurrent access")
			}
		}(i)
	}

	wg.Wait()

	// Verify each slug still maps to a single consistent mutex.
	for _, slug := range slugs {
		mu1 := m.Get(slug)
		mu2 := m.Get(slug)
		if mu1 != mu2 {
			t.Errorf("mutex for slug %q is inconsistent after concurrent access", slug)
		}
	}
}
