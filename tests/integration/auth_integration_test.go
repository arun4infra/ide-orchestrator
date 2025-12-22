package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/auth"
)

func TestAuthenticationIntegration(t *testing.T) {
	// Set required environment variable for JWT manager
	t.Setenv("JWT_SECRET", "test-secret-key-for-auth-integration-tests")

	// Initialize JWT manager (no database needed for JWT tests)
	jwtManager, err := auth.NewJWTManager()
	require.NoError(t, err)

	// Setup Gin router for HTTP testing
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	api := router.Group("/api")
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	protected := api.Group("")
	protected.Use(auth.RequireAuth(jwtManager))
	protected.GET("/protected", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{
			"user_id": userID,
			"email":   username, // The middleware sets username, but we call it email in response for consistency
			"message": "Access granted",
		})
	})

	t.Run("JWT Token Generation and Validation", func(t *testing.T) {
		userID := "test-user-123"
		username := "test@example.com"

		// Generate token
		token, err := jwtManager.GenerateToken(context.Background(), userID, username, []string{}, 24*time.Hour)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		// Validate token
		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, username, claims.Username)
		assert.True(t, claims.ExpiresAt.After(time.Now()))
	})

	t.Run("Authentication Required", func(t *testing.T) {
		// Test without token
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response["error"], "Missing authorization header")
	})

	t.Run("Invalid Token Formats", func(t *testing.T) {
		testCases := []struct {
			name   string
			header string
		}{
			{"Missing Bearer prefix", "invalid-token"},
			{"Empty Bearer", "Bearer "},
			{"Invalid JWT format", "Bearer invalid.jwt.token"},
			{"Malformed header", "NotBearer token"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
				req.Header.Set("Authorization", tc.header)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}
	})

	t.Run("Valid Token Access", func(t *testing.T) {
		userID := "valid-user-123"
		username := "valid@example.com"

		// Generate valid token
		token, err := jwtManager.GenerateToken(context.Background(), userID, username, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Test access with valid token
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, userID, response["user_id"])
		assert.Equal(t, username, response["email"])
		assert.Equal(t, "Access granted", response["message"])
	})

	t.Run("Token Reuse Validation", func(t *testing.T) {
		userID := "reuse-user-123"
		username := "reuse@example.com"

		// Generate token
		token, err := jwtManager.GenerateToken(context.Background(), userID, username, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Use the same token multiple times - should work
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, userID, response["user_id"])
			assert.Equal(t, username, response["email"])
		}
	})

	t.Run("Concurrent Token Usage", func(t *testing.T) {
		userID := "concurrent-user-123"
		username := "concurrent@example.com"

		// Generate token
		token, err := jwtManager.GenerateToken(context.Background(), userID, username, []string{}, 24*time.Hour)
		require.NoError(t, err)

		const numRequests = 5
		results := make(chan int, numRequests)

		// Make multiple concurrent requests with the same token
		for i := 0; i < numRequests; i++ {
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				results <- w.Code
			}()
		}

		// Collect results - all should succeed
		for i := 0; i < numRequests; i++ {
			select {
			case statusCode := <-results:
				assert.Equal(t, http.StatusOK, statusCode)
			case <-time.After(3 * time.Second):
				t.Fatal("Timeout waiting for concurrent requests")
			}
		}
	})

	t.Run("Public Endpoints No Auth Required", func(t *testing.T) {
		// Health endpoint should be accessible without authentication
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "healthy", response["status"])
	})
}