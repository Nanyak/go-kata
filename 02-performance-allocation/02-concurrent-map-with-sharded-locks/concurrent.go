package concurrent

import (
	"fmt"
	"hash/fnv"
	"sync"
)

// ShardedMap is a generic concurrent map with sharded locks for high-throughput access.
// It distributes keys across multiple shards to reduce lock contention.
type ShardedMap[K comparable, V any] struct {
	// TODO: Add fields
	// - shards: slice of maps for each shard
	shards []shard[K, V]
	// - numShards: number of shards
	numShards int
}

// shard represents a single shard containing a map and its lock.
type shard[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]V
}

// NewShardedMap creates a new ShardedMap with the specified number of shards.
// A higher shard count reduces contention but increases memory overhead.
func NewShardedMap[K comparable, V any](numShards int) *ShardedMap[K, V] {
	// TODO: Implement
	// - Validate numShards (minimum 1)
	if numShards < 1 {
		panic("numShards must be at least 1")
	}
	m := &ShardedMap[K, V]{
		numShards: numShards,
		shards:    make([]shard[K, V], numShards),
	}
	// - Initialize all shards with empty maps
	for i := 0; i < numShards; i++ {
		// Initialize each shard's map
		m.shards[i] = shard[K, V]{data: make(map[K]V)}
	}
	// - Return the initialized ShardedMap
	return m
	
}

// getShard returns the shard index for the given key using FNV-64 hashing.
// This must be deterministic and distribute keys evenly across shards.
func (m *ShardedMap[K, V]) getShard(key K) int {
	// TODO: Implement
	// - Hash the key using fnv64
	hash := hashKey(key)
	// - Return hash % 
	return int(hash % uint64(m.numShards))
	// - Hint: Use hash/fnv package and handle different key types
}

// hashKey computes the FNV-64 hash of a key.
// This is the core hashing function used for shard selection.
func hashKey[K comparable](key K) uint64 {
	// TODO: Implement
	// - Create fnv64a hasher
	h := fnv.New64a() // Use this hasher
	// - Convert key to bytes efficiently (avoid allocations)
	h.Write([]byte(fmt.Sprintf("%v", key))) // Simple conversion; optimize as needed
	// - Return the hash value
	return h.Sum64()
}

// Get retrieves a value from the map.
// Returns the value and true if found, zero value and false otherwise.
// Uses RLock for read optimization.
func (m *ShardedMap[K, V]) Get(key K) (V, bool) {
	// TODO: Implement
	// - Determine the shard for this key
	shardIndex := m.getShard(key)
	// - Acquire read lock (RLock)
	m.shards[shardIndex].mu.RLock()
	// - Look up value in shard's map
	value, ok := m.shards[shardIndex].data[key]
	// - Release lock and return result
	m.shards[shardIndex].mu.RUnlock()
	return value, ok
}

// Set inserts or updates a key-value pair in the map.
// Uses full Lock for write operations.
func (m *ShardedMap[K, V]) Set(key K, value V) {
	// TODO: Implement
	// - Determine the shard for this key
	shardIndex := m.getShard(key)
	// - Acquire write lock (Lock)
	m.shards[shardIndex].mu.Lock()
	// - Set the value in shard's map
	m.shards[shardIndex].data[key] = value
	// - Release lock
	m.shards[shardIndex].mu.Unlock()
}

// Delete removes a key from the map.
// No-op if the key doesn't exist.
func (m *ShardedMap[K, V]) Delete(key K) {
	// TODO: Implement
	// - Determine the shard for this key
	shardIndex := m.getShard(key)
	// - Acquire write lock (Lock)
	m.shards[shardIndex].mu.Lock()
	// - Delete the key from shard's map
	delete(m.shards[shardIndex].data, key)
	// - Release lock
	m.shards[shardIndex].mu.Unlock()
}

// Keys returns all keys in the map.
// The order of keys is not guaranteed.
// This operation acquires read locks on all shards sequentially.
func (m *ShardedMap[K, V]) Keys() []K {
	// TODO: Implement
	// - Pre-calculate total capacity to minimize allocations
	totalLen := 0
	for i := 0; i < m.numShards; i++ {
		m.shards[i].mu.RLock()
		totalLen += len(m.shards[i].data)
		m.shards[i].mu.RUnlock()
	}
	keys := make([]K, 0, totalLen)
	// - Iterate through all shards to collect keys
	for i := 0; i < m.numShards; i++ {
		m.shards[i].mu.RLock()
		for key := range m.shards[i].data {
			keys = append(keys, key)
		}
		m.shards[i].mu.RUnlock()
	}
	// - For each shard: acquire RLock, copy keys, release RLock
	// - Return collected keys
	return keys
	// - Note: This provides a snapshot, not a live view
}
