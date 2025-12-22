package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/auth"
	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/gateway"
	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/orchestration"
	"github.com/bizmatters/agent-builder/ide-orchestrator/tests/helpers"
)

func TestAuthenticationIntegration(t *testing.T) {
	// Setup test environment with real infrastructure
	testDB := helpers.NewTestDatabase(t)
	defer testDB.Close()

	// Use real deepagents-runtime service (no mocking)
	config := SetupInClusterEnvironment()
	t.Logf("Using real infrastructure - Database: %s, SpecEngine: %s", config.DatabaseURL, config.SpecEngineURL)
	
	// Initialize services with real connections
	specEngineClient := orchestration.NewSpecEngineClient(testDB.Pool)
	orchestrationService := orchestration.NewService(testDB.Pool, specEngineClient)
	
	jwtManager, err := auth.NewJWTManager()
	require.NoError(t, err)

	gatewayHandler := gateway.NewHandler(orchestrationService, jwtManager, testDB.Pool)

	// Setup Gin router for HTTP testing
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	api := router.Group("/api")
	api.POST("/auth/login", gatewayHandler.Login)
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	protected := api.Group("")
	protected.Use(auth.RequireAuth(jwtManager))
	protected.POST("/workflows", gatewayHandler.CreateWorkflow)
	protected.GET("/protected", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{
			"user_id": userID,
			"email":   username, // The middleware sets username, but we call it email in response for consistency
			"message": "Access granted",
		})
	})

	// NOTE: All JWT-specific validation tests have been moved to jwt_validation_integration_test.go
	// This file now focuses on authentication flow integration tests that are not JWT-specific

	t.Run("Authentication Flow Integration", func(t *testing.T) {
		// Test the complete authentication flow without duplicating JWT validation logic
		userEmail := fmt.Sprintf("auth-flow-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Generate token for authentication flow testing
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Test that authentication middleware properly integrates with the application
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, userID, response["user_id"])
		assert.Equal(t, userEmail, response["email"])
		assert.Equal(t, "Access granted", response["message"])
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