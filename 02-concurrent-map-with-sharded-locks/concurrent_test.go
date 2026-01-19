package concurrent

import (
	"fmt"
	"sync"
	"testing"
)

func TestShardedMap_BasicOperations(t *testing.T) {
	m := NewShardedMap[string, int](16)

	// Test Set and Get
	m.Set("foo", 42)
	val, ok := m.Get("foo")
	if !ok || val != 42 {
		t.Errorf("Get(foo) = %v, %v; want 42, true", val, ok)
	}

	// Test missing key
	_, ok = m.Get("bar")
	if ok {
		t.Error("Get(bar) should return false for missing key")
	}

	// Test overwrite
	m.Set("foo", 100)
	val, ok = m.Get("foo")
	if !ok || val != 100 {
		t.Errorf("Get(foo) after overwrite = %v, %v; want 100, true", val, ok)
	}

	// Test Delete
	m.Delete("foo")
	_, ok = m.Get("foo")
	if ok {
		t.Error("Get(foo) should return false after delete")
	}
}

func TestShardedMap_Keys(t *testing.T) {
	m := NewShardedMap[string, int](8)

	keys := []string{"a", "b", "c", "d", "e"}
	for i, k := range keys {
		m.Set(k, i)
	}

	result := m.Keys()
	if len(result) != len(keys) {
		t.Errorf("Keys() returned %d keys; want %d", len(result), len(keys))
	}

	// Check all keys are present
	keySet := make(map[string]bool)
	for _, k := range result {
		keySet[k] = true
	}
	for _, k := range keys {
		if !keySet[k] {
			t.Errorf("Keys() missing key %q", k)
		}
	}
}

func TestShardedMap_IntKeys(t *testing.T) {
	m := NewShardedMap[int, string](16)

	m.Set(1, "one")
	m.Set(2, "two")
	m.Set(1000000, "million")

	val, ok := m.Get(1)
	if !ok || val != "one" {
		t.Errorf("Get(1) = %v, %v; want one, true", val, ok)
	}

	val, ok = m.Get(1000000)
	if !ok || val != "million" {
		t.Errorf("Get(1000000) = %v, %v; want million, true", val, ok)
	}
}

// TestShardedMap_ConcurrentAccess tests for race conditions
// Run with: go test -race
func TestShardedMap_ConcurrentAccess(t *testing.T) {
	m := NewShardedMap[int, int](32)
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				key := id*1000 + j
				m.Set(key, key*2)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				m.Get(j)
			}
		}()
	}

	// Concurrent deleters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				m.Delete(id*100 + j)
			}
		}(i)
	}

	// Concurrent Keys() calls
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Keys()
		}()
	}

	wg.Wait()
}

// BenchmarkShardedMap_Set_1Shard benchmarks with 1 shard (high contention)
func BenchmarkShardedMap_Set_1Shard(b *testing.B) {
	m := NewShardedMap[int, int](1)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Set(i, i)
			i++
		}
	})
}

// BenchmarkShardedMap_Set_64Shards benchmarks with 64 shards (low contention)
func BenchmarkShardedMap_Set_64Shards(b *testing.B) {
	m := NewShardedMap[int, int](64)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Set(i, i)
			i++
		}
	})
}

// BenchmarkShardedMap_Get benchmarks read performance
func BenchmarkShardedMap_Get(b *testing.B) {
	m := NewShardedMap[int, int](64)
	// Pre-populate
	for i := 0; i < 10000; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Get(i % 10000)
			i++
		}
	})
}

// BenchmarkShardedMap_MixedReadWrite benchmarks 95% reads, 5% writes
func BenchmarkShardedMap_MixedReadWrite(b *testing.B) {
	m := NewShardedMap[int, int](64)
	// Pre-populate
	for i := 0; i < 10000; i++ {
		m.Set(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%20 == 0 { // 5% writes
				m.Set(i%10000, i)
			} else { // 95% reads
				m.Get(i % 10000)
			}
			i++
		}
	})
}

func ExampleShardedMap() {
	// Create a sharded map with 16 shards
	m := NewShardedMap[string, int](16)

	// Set values
	m.Set("users:alice", 100)
	m.Set("users:bob", 200)

	// Get a value
	if count, ok := m.Get("users:alice"); ok {
		fmt.Printf("Alice's count: %d\n", count)
	}

	// Output:
	// Alice's count: 100
}
