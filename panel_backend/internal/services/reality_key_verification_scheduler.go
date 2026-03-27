package services

import (
	"context"
	"log"
	"sync"
	"time"
)

// RealityKeyVerificationScheduler handles periodic verification of REALITY keys
type RealityKeyVerificationScheduler struct {
	service      *RealityKeyVerificationService
	interval     time.Duration
	autoFix      bool
	initialDelay time.Duration

	mu        sync.RWMutex
	isRunning bool
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewRealityKeyVerificationScheduler creates a new RealityKeyVerificationScheduler
func NewRealityKeyVerificationScheduler(
	service *RealityKeyVerificationService,
	intervalHours int,
	autoFix bool,
) *RealityKeyVerificationScheduler {
	// Default: 6 hours
	if intervalHours <= 0 {
		intervalHours = 6
	}

	return &RealityKeyVerificationScheduler{
		service:      service,
		interval:     time.Duration(intervalHours) * time.Hour,
		autoFix:      autoFix,
		initialDelay: 30 * time.Second, // Initial delay to allow system to stabilize
	}
}

// Start begins the periodic verification loop
func (s *RealityKeyVerificationScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	s.isRunning = true
	s.mu.Unlock()

	go func() {
		log.Printf("[reality-key-verification] scheduler started (interval=%v, autoFix=%v)", s.interval, s.autoFix)

		// Initial delay to allow system to stabilize
		select {
		case <-time.After(s.initialDelay):
		case <-ctx.Done():
			return
		}

		// Run initial verification
		log.Printf("[reality-key-verification] running initial verification")
		s.runVerification()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[reality-key-verification] context cancelled, stopping")
				s.mu.Lock()
				s.isRunning = false
				s.mu.Unlock()
				return
			case <-ticker.C:
				s.runVerification()
			}
		}
	}()
}

// Stop stops the scheduler
func (s *RealityKeyVerificationScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	if s.cancel != nil {
		s.cancel()
	}
	s.isRunning = false
	log.Printf("[reality-key-verification] scheduler stopped")
}

// GetStatus returns the current status of the scheduler
func (s *RealityKeyVerificationScheduler) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"schedulerActive": s.isRunning,
		"interval":        s.interval.String(),
		"autoFixEnabled":  s.autoFix,
	}

	if s.isRunning {
		status["nextRunAt"] = time.Now().Add(s.interval).Format(time.RFC3339)
	}

	return status
}

// runVerification executes a verification cycle
func (s *RealityKeyVerificationScheduler) runVerification() {
	log.Println("[reality-key-verification] running scheduled verification")

	results, err := s.service.VerifyAllNodes()
	if err != nil {
		log.Printf("[reality-key-verification] verification failed: %v", err)
		return
	}

	mismatchCount := 0
	fixSuccessCount := 0
	fixFailureCount := 0

	for _, result := range results {
		if result.Status == "mismatch" {
			mismatchCount++
			log.Printf("[reality-key-verification] mismatch detected on node %s (ID=%d)", result.NodeName, result.NodeID)

			if s.autoFix {
				log.Printf("[reality-key-verification] auto-fixing node %s (ID=%d)", result.NodeName, result.NodeID)
				if err := s.service.AutoFixMismatch(result.NodeID); err != nil {
					log.Printf("[reality-key-verification] auto-fix failed for node %s: %v", result.NodeName, err)
					fixFailureCount++
				} else {
					log.Printf("[reality-key-verification] auto-fix succeeded for node %s", result.NodeName)
					fixSuccessCount++
				}
			}
		}
	}

	log.Printf("[reality-key-verification] verification completed: %d nodes checked, %d mismatches found, %d fixes succeeded, %d fixes failed",
		len(results), mismatchCount, fixSuccessCount, fixFailureCount)
}
