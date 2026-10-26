package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/torreirow/parsh/config"
)

func TestCacheTTL(t *testing.T) {
	cache := &completionCache{
		items: make(map[string]*cacheEntry),
	}

	// Add entry
	cache.set("test:us-east-1", []string{"val1", "val2"})

	// Immediately retrieve - should work
	result := cache.get("test:us-east-1", 1*time.Second)
	if result == nil {
		t.Error("Should retrieve fresh entry")
	}

	// Wait for TTL to expire
	time.Sleep(1100 * time.Millisecond)

	// Retrieve again - should be expired
	result = cache.get("test:us-east-1", 1*time.Second)
	if result != nil {
		t.Error("Should not retrieve expired entry")
	}
}

func TestLRUEviction(t *testing.T) {
	cache := &completionCache{
		items: make(map[string]*cacheEntry),
	}

	// Fill cache to max
	for i := 0; i < maxCacheEntries; i++ {
		cache.set(fmt.Sprintf("key%d:us-east-1", i), []string{"val"})
	}

	if len(cache.items) != maxCacheEntries {
		t.Errorf("Cache should have %d entries, got %d", maxCacheEntries, len(cache.items))
	}

	// Access first entry to make it recently used
	_ = cache.get("key0:us-east-1", 1*time.Hour)

	// Add one more entry - should evict LRU (not key0)
	cache.set("new:us-east-1", []string{"new"})

	// key0 should still exist (was recently accessed)
	if cache.get("key0:us-east-1", 1*time.Hour) == nil {
		t.Error("Recently accessed entry should not be evicted")
	}

	// Some other key should have been evicted
	if len(cache.items) > maxCacheEntries {
		t.Error("Cache exceeded max entries")
	}
}

func TestInvalidatePathAndParents(t *testing.T) {
	// Setup cache
	memoryCache.items = make(map[string]*cacheEntry)
	memoryCache.set("/dev:us-east-1", []string{"app/", "db/"})
	memoryCache.set("/dev/app:us-east-1", []string{"url", "domain"})
	memoryCache.set("/prod:us-east-1", []string{"app/"})

	// Invalidate /dev/app
	invalidatePathAndParents("/dev/app", "us-east-1")

	// /dev/app should be gone
	if memoryCache.get("/dev/app:us-east-1", 1*time.Hour) != nil {
		t.Error("/dev/app should be invalidated")
	}

	// Parent /dev should be gone
	if memoryCache.get("/dev:us-east-1", 1*time.Hour) != nil {
		t.Error("/dev should be invalidated")
	}

	// Unrelated /prod should still exist
	if memoryCache.get("/prod:us-east-1", 1*time.Hour) == nil {
		t.Error("/prod should not be invalidated")
	}
}

func TestRequestRateLimiting(t *testing.T) {
	tracker := &requestTracker{
		recentRequests: make([]time.Time, 0, 100),
	}

	// Make 9 requests - should all be allowed
	for i := 0; i < 9; i++ {
		if !tracker.allowRequest() {
			t.Error("Request should be allowed")
		}
		tracker.recordRequest()
	}

	// 10th request should still be allowed
	if !tracker.allowRequest() {
		t.Error("10th request should be allowed")
	}
	tracker.recordRequest()

	// 11th request should be blocked
	if tracker.allowRequest() {
		t.Error("11th request should be blocked")
	}

	// Wait 1 second
	time.Sleep(1 * time.Second)

	// Should be allowed again
	if !tracker.allowRequest() {
		t.Error("Request should be allowed after cooldown")
	}
}

func TestThrottleBackoff(t *testing.T) {
	tracker := &requestTracker{
		recentRequests: make([]time.Time, 0, 100),
	}

	// Initially not throttled
	if tracker.isThrottled() {
		t.Error("Should not be throttled initially")
	}

	// Mark as throttled
	tracker.markThrottled()

	// Should now be throttled
	if !tracker.isThrottled() {
		t.Error("Should be throttled after marking")
	}

	// Wait 5 seconds
	time.Sleep(5 * time.Second)

	// Should no longer be throttled
	if tracker.isThrottled() {
		t.Error("Should not be throttled after backoff period")
	}
}

func TestCacheStats(t *testing.T) {
	cache := &completionCache{
		items: make(map[string]*cacheEntry),
	}

	cache.set("key1:us-east-1", []string{"val1", "val2", "val3"})
	cache.set("key2:us-east-1", []string{"val4"})

	stats := cache.stats()

	if stats.Entries != 2 {
		t.Errorf("Expected 2 entries, got %d", stats.Entries)
	}

	if stats.TotalItems != 4 {
		t.Errorf("Expected 4 total items, got %d", stats.TotalItems)
	}
}

func TestPersistentCacheSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.db")

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = cachePath
	cfg.Default.CacheMaxSize = 10 // 10 MB
	cfg.Default.CacheCompression = true

	// Create cache
	cache, err := NewPersistentCache(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Add entries
	cache.set("key1:us-east-1", []string{"val1", "val2"})
	cache.set("key2:us-east-1", []string{"val3"})

	// Save
	if err := cache.save(); err != nil {
		t.Fatal(err)
	}

	// Verify file exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("Cache file should exist")
	}

	// Load into new cache instance
	cache2, err := NewPersistentCache(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Verify data loaded
	if result := cache2.get("key1:us-east-1", 1*time.Hour); result == nil {
		t.Error("Should retrieve key1")
	} else if len(result) != 2 {
		t.Errorf("key1 should have 2 values, got %d", len(result))
	}
}

func TestPersistentCacheMaxSize(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.db")

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = cachePath
	cfg.Default.CacheMaxSize = 1 // Only 1 MB max
	cfg.Default.CacheCompression = false

	cache, err := NewPersistentCache(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Add lots of data (more than 1 MB)
	largeValue := make([]string, 10000)
	for i := range largeValue {
		largeValue[i] = "this is a long string to fill up space"
	}

	for i := 0; i < 100; i++ {
		cache.set(fmt.Sprintf("key%d:us-east-1", i), largeValue)
	}

	// Try to save - should fail due to size
	err = cache.save()
	if err == nil {
		t.Error("Save should fail when exceeding max size")
	}
}

func TestPersistentCacheTTL(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.db")

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = cachePath
	cfg.Default.CacheMaxSize = 10

	cache, err := NewPersistentCache(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Add entry
	cache.set("key1:us-east-1", []string{"val1"})

	// Immediately retrieve - should work
	if result := cache.get("key1:us-east-1", 1*time.Second); result == nil {
		t.Error("Should retrieve fresh entry")
	}

	// Wait for TTL to expire
	time.Sleep(1100 * time.Millisecond)

	// Retrieve again - should be expired
	if result := cache.get("key1:us-east-1", 1*time.Second); result != nil {
		t.Error("Should not retrieve expired entry")
	}
}

func TestCacheCompression(t *testing.T) {
	tmpDir := t.TempDir()

	testData := make([]string, 1000)
	for i := range testData {
		testData[i] = "this is repetitive data that compresses well"
	}

	// Test without compression
	cfg1 := &config.Config{}
	cfg1.Default.CacheEnabled = true
	cfg1.Default.CacheLocation = filepath.Join(tmpDir, "uncompressed.db")
	cfg1.Default.CacheCompression = false

	cache1, _ := NewPersistentCache(cfg1)
	cache1.set("data:us-east-1", testData)
	_ = cache1.save()

	stat1, _ := os.Stat(cfg1.Default.CacheLocation)
	uncompressedSize := stat1.Size()

	// Test with compression
	cfg2 := &config.Config{}
	cfg2.Default.CacheEnabled = true
	cfg2.Default.CacheLocation = filepath.Join(tmpDir, "compressed.db")
	cfg2.Default.CacheCompression = true

	cache2, _ := NewPersistentCache(cfg2)
	cache2.set("data:us-east-1", testData)
	_ = cache2.save()

	stat2, _ := os.Stat(cfg2.Default.CacheLocation)
	compressedSize := stat2.Size()

	// Compression should reduce size significantly
	ratio := float64(compressedSize) / float64(uncompressedSize)
	if ratio > 0.5 {
		t.Errorf("Compression ratio = %.2f, expected < 0.5", ratio)
	}

	t.Logf("Compression: %d bytes -> %d bytes (ratio: %.2f)",
		uncompressedSize, compressedSize, ratio)
}

// ========== Benchmarks ==========

// BenchmarkCacheMemoryGet measures performance of in-memory cache lookups
func BenchmarkCacheMemoryGet(b *testing.B) {
	cache := &completionCache{
		items: make(map[string]*cacheEntry),
	}

	// Populate with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("/path/%d:us-east-1", i)
		values := []string{"param1", "param2", "param3"}
		cache.set(key, values)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Benchmark random lookups
		key := fmt.Sprintf("/path/%d:us-east-1", i%1000)
		cache.get(key, 1*time.Hour)
	}
}

// BenchmarkCachePersistentGet measures performance of persistent cache lookups
func BenchmarkCachePersistentGet(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench.db")

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = cachePath
	cfg.Default.CacheMaxSize = 50
	cfg.Default.CacheCompression = true

	cache, err := NewPersistentCache(cfg)
	if err != nil {
		b.Fatal(err)
	}

	// Populate with 1000 entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("/path/%d:us-east-1", i)
		values := []string{"param1", "param2", "param3"}
		cache.set(key, values)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("/path/%d:us-east-1", i%1000)
		cache.get(key, 1*time.Hour)
	}
}

// BenchmarkCacheSaveUncompressed measures save performance without compression
func BenchmarkCacheSaveUncompressed(b *testing.B) {
	tmpDir := b.TempDir()

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = filepath.Join(tmpDir, "bench.db")
	cfg.Default.CacheMaxSize = 50
	cfg.Default.CacheCompression = false

	cache, err := NewPersistentCache(cfg)
	if err != nil {
		b.Fatal(err)
	}

	// Populate with realistic data
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("/app/config/%d:us-east-1", i)
		values := []string{"database.url", "api.key", "feature.flag"}
		cache.set(key, values)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cache.save(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCacheSaveCompressed measures save performance with compression
func BenchmarkCacheSaveCompressed(b *testing.B) {
	tmpDir := b.TempDir()

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = filepath.Join(tmpDir, "bench.db")
	cfg.Default.CacheMaxSize = 50
	cfg.Default.CacheCompression = true

	cache, err := NewPersistentCache(cfg)
	if err != nil {
		b.Fatal(err)
	}

	// Populate with realistic data
	for i := 0; i < 500; i++ {
		key := fmt.Sprintf("/app/config/%d:us-east-1", i)
		values := []string{"database.url", "api.key", "feature.flag"}
		cache.set(key, values)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cache.save(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompleterFilterAndLimit measures filtering performance with large result sets
func BenchmarkCompleterFilterAndLimit(b *testing.B) {
	// Create large dataset (500+ items)
	items := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = fmt.Sprintf("/app/service-%d/config/parameter-%d", i/10, i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Filter with prefix matching
		prefix := "/app/service-5"
		filterAndLimit(items, prefix, false)
	}
}

// BenchmarkCacheSetLRU measures performance of cache set with LRU eviction
func BenchmarkCacheSetLRU(b *testing.B) {
	cache := &completionCache{
		items: make(map[string]*cacheEntry),
	}

	// Fill cache to near max
	for i := 0; i < maxCacheEntries-10; i++ {
		cache.set(fmt.Sprintf("key%d:us-east-1", i), []string{"val"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will trigger LRU eviction on some iterations
		cache.set(fmt.Sprintf("new%d:us-east-1", i), []string{"val1", "val2"})
	}
}

// BenchmarkInvalidatePathAndParents measures cache invalidation performance
func BenchmarkInvalidatePathAndParents(b *testing.B) {
	// Setup cache with hierarchical paths
	memoryCache.items = make(map[string]*cacheEntry)
	for i := 0; i < 100; i++ {
		memoryCache.set(fmt.Sprintf("/app/service-%d:us-east-1", i), []string{"param"})
		memoryCache.set(fmt.Sprintf("/app/service-%d/config:us-east-1", i), []string{"val"})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		invalidatePathAndParents(fmt.Sprintf("/app/service-%d/config", i%100), "us-east-1")
	}
}

// BenchmarkPersistentCacheLoad measures cache loading performance
func BenchmarkPersistentCacheLoad(b *testing.B) {
	tmpDir := b.TempDir()
	cachePath := filepath.Join(tmpDir, "bench.db")

	cfg := &config.Config{}
	cfg.Default.CacheEnabled = true
	cfg.Default.CacheLocation = cachePath
	cfg.Default.CacheMaxSize = 50
	cfg.Default.CacheCompression = true

	// Create and save cache once
	cache, err := NewPersistentCache(cfg)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < 500; i++ {
		cache.set(fmt.Sprintf("/path/%d:us-east-1", i), []string{"val1", "val2"})
	}
	_ = cache.save()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Load cache from disk
		_, err := NewPersistentCache(cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}
