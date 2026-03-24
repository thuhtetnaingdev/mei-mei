package services

import (
	"context"
	"log"
	"sync"
	"time"
)

// UserClassificationScheduler handles daily user classification
type UserClassificationScheduler struct {
	service     *UserClassificationService
	interval    time.Duration
	initialDelay time.Duration

	mu        sync.RWMutex
	isRunning bool
}

// NewUserClassificationScheduler creates a new user classification scheduler
func NewUserClassificationScheduler(service *UserClassificationService, interval time.Duration) *UserClassificationScheduler {
	return &UserClassificationScheduler{
		service:     service,
		interval:    interval,
		initialDelay: 30 * time.Second, // Initial delay to allow system to stabilize
	}
}

// Start begins the daily classification loop
func (s *UserClassificationScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	s.isRunning = true
	s.mu.Unlock()

	go func() {
		log.Printf("[user-classification-scheduler] started with interval %v", s.interval)

		// Initial delay to allow system to stabilize
		select {
		case <-time.After(s.initialDelay):
		case <-ctx.Done():
			return
		}

		// Run initial classification
		log.Printf("[user-classification-scheduler] running initial classification")
		s.service.ClassifyAllUsers()

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[user-classification-scheduler] context cancelled, stopping")
				s.mu.Lock()
				s.isRunning = false
				s.mu.Unlock()
				return
			case <-ticker.C:
				log.Printf("[user-classification-scheduler] running scheduled classification")
				s.service.ClassifyAllUsers()
			}
		}
	}()
}

// GetStatus returns the current status of the scheduler
func (s *UserClassificationScheduler) GetStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"schedulerActive": s.isRunning,
		"interval":        s.interval.String(),
		"nextRunAt":       time.Now().Add(s.interval).Format(time.RFC3339),
	}
}

// Stop stops the scheduler
func (s *UserClassificationScheduler) Stop() {
	s.mu.Lock()
	s.isRunning = false
	s.mu.Unlock()
	log.Printf("[user-classification-scheduler] stopped")
}
