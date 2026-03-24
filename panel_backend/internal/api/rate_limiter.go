package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiterMiddleware creates a rate limiting middleware for public endpoints
// It limits requests per IP address to prevent abuse
func RateLimiterMiddleware() gin.HandlerFunc {
	var (
		clients = make(map[string]*clientLimiter)
		mu      sync.RWMutex
	)

	// Cleanup stale entries every minute
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			mu.Lock()
			for ip, client := range clients {
				if time.Since(client.lastSeen) > 10*time.Minute {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.RLock()
		client, exists := clients[ip]
		mu.RUnlock()

		if !exists {
			mu.Lock()
			// Check again in case another goroutine created it
			if client, exists = clients[ip]; !exists {
				// Allow 10 requests per second with burst of 20
				client = &clientLimiter{
					limiter:  rate.NewLimiter(rate.Every(100*time.Millisecond), 20),
					lastSeen: time.Now(),
				}
				clients[ip] = client
			}
			mu.Unlock()
		}

		// Update last seen time
		mu.Lock()
		client.lastSeen = time.Now()
		mu.Unlock()

		// Check if request is allowed
		if !client.limiter.Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, please try again later",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}
