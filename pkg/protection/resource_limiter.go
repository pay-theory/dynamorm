package protection

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ResourceLimits defines configurable resource limits
type ResourceLimits struct {
	// HTTP request limits
	MaxRequestBodySize int64         `json:"max_request_body_size" yaml:"max_request_body_size"`
	MaxRequestTimeout  time.Duration `json:"max_request_timeout" yaml:"max_request_timeout"`
	MaxConcurrentReq   int           `json:"max_concurrent_requests" yaml:"max_concurrent_requests"`

	// Batch operation limits
	MaxBatchSize       int     `json:"max_batch_size" yaml:"max_batch_size"`
	MaxConcurrentBatch int     `json:"max_concurrent_batch" yaml:"max_concurrent_batch"`
	BatchRateLimit     float64 `json:"batch_rate_limit" yaml:"batch_rate_limit"`

	// Memory limits
	MaxMemoryMB          int64         `json:"max_memory_mb" yaml:"max_memory_mb"`
	MemoryCheckInterval  time.Duration `json:"memory_check_interval" yaml:"memory_check_interval"`
	MemoryPanicThreshold float64       `json:"memory_panic_threshold" yaml:"memory_panic_threshold"`

	// Rate limiting
	RequestsPerSecond float64 `json:"requests_per_second" yaml:"requests_per_second"`
	BurstSize         int     `json:"burst_size" yaml:"burst_size"`
}

// DefaultResourceLimits returns secure default limits
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		// HTTP limits
		MaxRequestBodySize: 10 * 1024 * 1024, // 10MB
		MaxRequestTimeout:  30 * time.Second,
		MaxConcurrentReq:   100,

		// Batch limits
		MaxBatchSize:       25, // DynamoDB batch limit
		MaxConcurrentBatch: 10,
		BatchRateLimit:     100, // ops per second

		// Memory limits
		MaxMemoryMB:          500, // 500MB default
		MemoryCheckInterval:  5 * time.Second,
		MemoryPanicThreshold: 0.9, // 90% of max memory

		// Rate limiting
		RequestsPerSecond: 1000, // 1000 RPS
		BurstSize:         50,
	}
}

// ResourceProtector provides resource protection and monitoring
type ResourceProtector struct {
	config ResourceLimits

	// Rate limiters
	globalLimiter *SimpleLimiter
	batchLimiter  *SimpleLimiter

	// Concurrency controls
	requestSemaphore chan struct{}
	batchSemaphore   chan struct{}

	// Memory monitoring
	memoryMonitor *MemoryMonitor

	// Statistics
	stats *ResourceStats
	mu    sync.RWMutex
}

// ResourceStats tracks resource usage statistics
type ResourceStats struct {
	// Request stats
	TotalRequests      int64 `json:"total_requests"`
	RejectedRequests   int64 `json:"rejected_requests"`
	ConcurrentRequests int64 `json:"concurrent_requests"`
	MaxConcurrentReq   int64 `json:"max_concurrent_requests"`

	// Batch stats
	TotalBatchOps      int64 `json:"total_batch_operations"`
	RejectedBatchOps   int64 `json:"rejected_batch_operations"`
	ConcurrentBatchOps int64 `json:"concurrent_batch_operations"`
	MaxConcurrentBatch int64 `json:"max_concurrent_batch"`

	// Memory stats
	CurrentMemoryMB int64 `json:"current_memory_mb"`
	PeakMemoryMB    int64 `json:"peak_memory_mb"`
	MemoryAlerts    int64 `json:"memory_alerts"`

	// Rate limiting stats
	RateLimitHits   int64     `json:"rate_limit_hits"`
	LastStatsUpdate time.Time `json:"last_stats_update"`
}

// MemoryMonitor monitors memory usage
type MemoryMonitor struct {
	limits        ResourceLimits
	alertCallback func(MemoryAlert)
	stopChan      chan struct{}
	stats         *ResourceStats
	running       bool
	mu            sync.Mutex
}

// MemoryAlert represents a memory usage alert
type MemoryAlert struct {
	Type         string    `json:"type"`
	CurrentMB    int64     `json:"current_mb"`
	LimitMB      int64     `json:"limit_mb"`
	UsagePercent float64   `json:"usage_percent"`
	Timestamp    time.Time `json:"timestamp"`
	Severity     string    `json:"severity"`
}

// NewResourceProtector creates a new resource protector
func NewResourceProtector(config ResourceLimits) *ResourceProtector {
	rp := &ResourceProtector{
		config:           config,
		globalLimiter:    NewSimpleLimiter(config.RequestsPerSecond, config.BurstSize),
		batchLimiter:     NewSimpleLimiter(config.BatchRateLimit, int(config.BatchRateLimit/10)),
		requestSemaphore: make(chan struct{}, config.MaxConcurrentReq),
		batchSemaphore:   make(chan struct{}, config.MaxConcurrentBatch),
		stats:            &ResourceStats{LastStatsUpdate: time.Now()},
	}

	// Initialize memory monitor
	rp.memoryMonitor = &MemoryMonitor{
		limits:   config,
		stopChan: make(chan struct{}),
		stats:    rp.stats,
	}

	return rp
}

// SecureBodyReader provides secure HTTP body reading with size limits
func (rp *ResourceProtector) SecureBodyReader(r *http.Request) ([]byte, error) {
	// Check rate limit first
	if !rp.globalLimiter.Allow() {
		atomic.AddInt64(&rp.stats.RateLimitHits, 1)
		return nil, &ProtectionError{
			Type:   "RateLimitExceeded",
			Detail: "Request rate limit exceeded",
		}
	}

	// Acquire request semaphore
	select {
	case rp.requestSemaphore <- struct{}{}:
		defer func() { <-rp.requestSemaphore }()
	default:
		atomic.AddInt64(&rp.stats.RejectedRequests, 1)
		return nil, &ProtectionError{
			Type:   "ConcurrencyLimitExceeded",
			Detail: fmt.Sprintf("Maximum concurrent requests (%d) exceeded", rp.config.MaxConcurrentReq),
		}
	}

	// Update stats
	current := atomic.AddInt64(&rp.stats.ConcurrentRequests, 1)
	defer atomic.AddInt64(&rp.stats.ConcurrentRequests, -1)
	atomic.AddInt64(&rp.stats.TotalRequests, 1)

	// Update max concurrent requests
	for {
		max := atomic.LoadInt64(&rp.stats.MaxConcurrentReq)
		if current <= max || atomic.CompareAndSwapInt64(&rp.stats.MaxConcurrentReq, max, current) {
			break
		}
	}

	// Limit request body size
	body := http.MaxBytesReader(nil, r.Body, rp.config.MaxRequestBodySize)

	// Add timeout context
	ctx, cancel := context.WithTimeout(r.Context(), rp.config.MaxRequestTimeout)
	defer cancel()

	// Read with timeout
	done := make(chan struct{})
	var bodyBytes []byte
	var err error

	go func() {
		defer close(done)
		bodyBytes, err = io.ReadAll(body)
	}()

	select {
	case <-done:
		return bodyBytes, err
	case <-ctx.Done():
		atomic.AddInt64(&rp.stats.RejectedRequests, 1)
		return nil, &ProtectionError{
			Type:   "RequestTimeout",
			Detail: fmt.Sprintf("Request timeout after %v", rp.config.MaxRequestTimeout),
		}
	}
}

// BatchLimiter provides batch operation protection
type BatchLimiter struct {
	protector *ResourceProtector
}

// GetBatchLimiter returns a batch limiter
func (rp *ResourceProtector) GetBatchLimiter() *BatchLimiter {
	return &BatchLimiter{protector: rp}
}

// AcquireBatch acquires permission for a batch operation
func (bl *BatchLimiter) AcquireBatch(ctx context.Context, batchSize int) error {
	// Validate batch size
	if batchSize > bl.protector.config.MaxBatchSize {
		atomic.AddInt64(&bl.protector.stats.RejectedBatchOps, 1)
		return &ProtectionError{
			Type:   "BatchSizeExceeded",
			Detail: fmt.Sprintf("Batch size %d exceeds maximum %d", batchSize, bl.protector.config.MaxBatchSize),
		}
	}

	// Check batch rate limit
	if !bl.protector.batchLimiter.Allow() {
		atomic.AddInt64(&bl.protector.stats.RejectedBatchOps, 1)
		return &ProtectionError{
			Type:   "BatchRateLimitExceeded",
			Detail: "Batch rate limit exceeded",
		}
	}

	// Acquire batch semaphore
	select {
	case bl.protector.batchSemaphore <- struct{}{}:
		// Successfully acquired
	case <-ctx.Done():
		atomic.AddInt64(&bl.protector.stats.RejectedBatchOps, 1)
		return ctx.Err()
	default:
		atomic.AddInt64(&bl.protector.stats.RejectedBatchOps, 1)
		return &ProtectionError{
			Type:   "BatchConcurrencyExceeded",
			Detail: fmt.Sprintf("Maximum concurrent batch operations (%d) exceeded", bl.protector.config.MaxConcurrentBatch),
		}
	}

	// Update stats
	current := atomic.AddInt64(&bl.protector.stats.ConcurrentBatchOps, 1)
	atomic.AddInt64(&bl.protector.stats.TotalBatchOps, 1)

	// Update max concurrent batch ops
	for {
		max := atomic.LoadInt64(&bl.protector.stats.MaxConcurrentBatch)
		if current <= max || atomic.CompareAndSwapInt64(&bl.protector.stats.MaxConcurrentBatch, max, current) {
			break
		}
	}

	return nil
}

// ReleaseBatch releases batch operation permission
func (bl *BatchLimiter) ReleaseBatch() {
	atomic.AddInt64(&bl.protector.stats.ConcurrentBatchOps, -1)
	<-bl.protector.batchSemaphore
}

// StartMemoryMonitoring starts memory monitoring
func (rp *ResourceProtector) StartMemoryMonitoring(alertCallback func(MemoryAlert)) {
	rp.memoryMonitor.alertCallback = alertCallback
	rp.memoryMonitor.Start()
}

// StopMemoryMonitoring stops memory monitoring
func (rp *ResourceProtector) StopMemoryMonitoring() {
	rp.memoryMonitor.Stop()
}

// Start starts the memory monitor
func (mm *MemoryMonitor) Start() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.running {
		return
	}

	mm.running = true
	go mm.monitorLoop()
}

// Stop stops the memory monitor
func (mm *MemoryMonitor) Stop() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if !mm.running {
		return
	}

	mm.running = false
	close(mm.stopChan)
}

// monitorLoop runs the memory monitoring loop
func (mm *MemoryMonitor) monitorLoop() {
	ticker := time.NewTicker(mm.limits.MemoryCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mm.checkMemory()
		case <-mm.stopChan:
			return
		}
	}
}

// checkMemory checks current memory usage
func (mm *MemoryMonitor) checkMemory() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	currentMB := int64(memStats.Alloc / 1024 / 1024)
	atomic.StoreInt64(&mm.stats.CurrentMemoryMB, currentMB)

	// Update peak memory
	for {
		peak := atomic.LoadInt64(&mm.stats.PeakMemoryMB)
		if currentMB <= peak || atomic.CompareAndSwapInt64(&mm.stats.PeakMemoryMB, peak, currentMB) {
			break
		}
	}

	// Check against limits
	usagePercent := float64(currentMB) / float64(mm.limits.MaxMemoryMB)

	if usagePercent >= mm.limits.MemoryPanicThreshold {
		atomic.AddInt64(&mm.stats.MemoryAlerts, 1)

		alert := MemoryAlert{
			Type:         "MemoryThresholdExceeded",
			CurrentMB:    currentMB,
			LimitMB:      mm.limits.MaxMemoryMB,
			UsagePercent: usagePercent * 100,
			Timestamp:    time.Now(),
			Severity:     mm.determineSeverity(usagePercent),
		}

		if mm.alertCallback != nil {
			mm.alertCallback(alert)
		}

		// Force garbage collection if memory usage is very high
		if usagePercent >= 0.95 {
			runtime.GC()
		}
	}
}

// determineSeverity determines alert severity based on usage
func (mm *MemoryMonitor) determineSeverity(usagePercent float64) string {
	switch {
	case usagePercent >= 0.95:
		return "CRITICAL"
	case usagePercent >= 0.9:
		return "HIGH"
	case usagePercent >= 0.8:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// GetStats returns current resource usage statistics
func (rp *ResourceProtector) GetStats() ResourceStats {
	rp.mu.RLock()
	defer rp.mu.RUnlock()

	stats := *rp.stats
	stats.LastStatsUpdate = time.Now()
	return stats
}

// ProtectionError represents a resource protection error
type ProtectionError struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

func (e *ProtectionError) Error() string {
	return fmt.Sprintf("resource protection: %s - %s", e.Type, e.Detail)
}

// IsResourceProtectionError checks if an error is a resource protection error
func IsResourceProtectionError(err error) bool {
	_, ok := err.(*ProtectionError)
	return ok
}

// GetResourceProtectionType returns the type of resource protection error
func GetResourceProtectionType(err error) string {
	if protErr, ok := err.(*ProtectionError); ok {
		return protErr.Type
	}
	return ""
}

// HealthCheck performs a resource protection health check
func (rp *ResourceProtector) HealthCheck() map[string]any {
	stats := rp.GetStats()

	health := map[string]any{
		"status": "healthy",
		"checks": map[string]any{
			"memory": map[string]any{
				"status":        "ok",
				"current_mb":    stats.CurrentMemoryMB,
				"limit_mb":      rp.config.MaxMemoryMB,
				"usage_percent": float64(stats.CurrentMemoryMB) / float64(rp.config.MaxMemoryMB) * 100,
			},
			"concurrency": map[string]any{
				"status":              "ok",
				"concurrent_requests": stats.ConcurrentRequests,
				"max_requests":        rp.config.MaxConcurrentReq,
				"concurrent_batches":  stats.ConcurrentBatchOps,
				"max_batches":         rp.config.MaxConcurrentBatch,
			},
			"rate_limiting": map[string]any{
				"status":           "ok",
				"rate_limit_hits":  stats.RateLimitHits,
				"requests_per_sec": rp.config.RequestsPerSecond,
			},
		},
		"timestamp": time.Now(),
	}

	// Check for unhealthy conditions
	memoryUsage := float64(stats.CurrentMemoryMB) / float64(rp.config.MaxMemoryMB)
	if memoryUsage > 0.9 {
		health["status"] = "degraded"
		health["checks"].(map[string]any)["memory"].(map[string]any)["status"] = "warning"
	}

	if stats.ConcurrentRequests >= int64(float64(rp.config.MaxConcurrentReq)*0.9) {
		health["status"] = "degraded"
		health["checks"].(map[string]any)["concurrency"].(map[string]any)["status"] = "warning"
	}

	return health
}
