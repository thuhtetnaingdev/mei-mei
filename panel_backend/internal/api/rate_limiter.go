package api

import (
	"fmt"
	"net/http"
	"strings"
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

// LoginRateLimiterMiddleware creates a rate limiting middleware specifically for login endpoints
// It limits login attempts to prevent brute force attacks (OWASP A07)
// Limit: 5 attempts per minute per IP address using sliding window
func LoginRateLimiterMiddleware() gin.HandlerFunc {
	type loginAttempt struct {
		timestamp time.Time
		count     int
	}

	var (
		attempts = make(map[string]*loginAttempt)
		mu       sync.RWMutex
	)

	// Cleanup old entries every 30 seconds
	go func() {
		for {
			time.Sleep(30 * time.Second)
			mu.Lock()
			for ip, attempt := range attempts {
				if time.Since(attempt.timestamp) > 2*time.Minute {
					delete(attempts, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := getClientIP(c)
		now := time.Now()

		mu.Lock()
		attempt, exists := attempts[ip]
		
		if !exists || now.Sub(attempt.timestamp) > time.Minute {
			// Reset counter if more than a minute has passed
			attempts[ip] = &loginAttempt{
				timestamp: now,
				count:     1,
			}
			mu.Unlock()
			c.Next()
			return
		}

		if attempt.count >= 5 {
			// Rate limit exceeded
			retryAfter := int(60 - now.Sub(attempt.timestamp).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}

			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "too many login attempts, please try again later",
				"retry_after": retryAfter,
			})
			c.Abort()
			mu.Unlock()
			return
		}

		attempt.count++
		mu.Unlock()
		c.Next()
	}
}

// getClientIP extracts the client IP address from the request,
// considering X-Forwarded-For header for proxied requests
func getClientIP(c *gin.Context) string {
	// Check X-Forwarded-For header first (for proxied requests)
	xff := c.GetHeader("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first one
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	xri := c.GetHeader("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to ClientIP() which checks RemoteAddr
	return c.ClientIP()
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type loginClientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}
