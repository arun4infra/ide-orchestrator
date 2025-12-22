package integration

import (
	"bytes"
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

// TestAuthDatabaseIntegration tests critical auth validations that require database access
// NOTE: JWT-specific validation tests have been moved to jwt_validation_integration_test.go
// This file focuses on database-specific authentication integration tests
func TestAuthDatabaseIntegration(t *testing.T) {
	// Setup test environment with real infrastructure
	testDB := helpers.NewTestDatabase(t)
	defer testDB.Close()

	// Use real deepagents-runtime service (no mocking)
	config := SetupInClusterEnvironment()
	t.Logf("Using real infrastructure - Database: %s, SpecEngine: %s", config.DatabaseURL, config.SpecEngineURL)

	// Initialize services
	specEngineClient := orchestration.NewSpecEngineClient(testDB.Pool)
	orchestrationService := orchestration.NewService(testDB.Pool, specEngineClient)
	
	jwtManager, err := auth.NewJWTManager()
	require.NoError(t, err)

	gatewayHandler := gateway.NewHandler(orchestrationService, jwtManager, testDB.Pool)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	api := router.Group("/api")
	api.POST("/auth/login", gatewayHandler.Login)

	protected := api.Group("")
	protected.Use(auth.RequireAuth(jwtManager))
	protected.POST("/workflows", gatewayHandler.CreateWorkflow)
	protected.GET("/workflows/:id", gatewayHandler.GetWorkflow)
	protected.GET("/protected", func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")
		c.JSON(http.StatusOK, gin.H{
			"user_id": userID,
			"email":   username,
			"message": "Access granted",
		})
	})

	t.Run("Database User Authentication Integration", func(t *testing.T) {
		// Create real user in database
		userEmail := fmt.Sprintf("db-auth-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")
		
		// Generate token for real user
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Test that database user can access protected endpoints
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

	t.Run("Database User Workflow Creation", func(t *testing.T) {
		// Create real user in database
		userEmail := fmt.Sprintf("db-workflow-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")
		
		// Generate token for real user
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Create workflow to test database integration with authentication
		workflowReq := map[string]interface{}{
			"name":        "Database Integration Workflow",
			"description": "Testing database integration with authentication",
		}
		workflowBody, _ := json.Marshal(workflowReq)

		req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBuffer(workflowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		// Verify the workflow was created with correct user context
		assert.NotEmpty(t, response["id"])
		assert.Equal(t, "Database Integration Workflow", response["name"])
		
		// Verify the workflow is associated with the correct user in database
		workflowID := response["id"].(string)
		var dbUserID string
		err = testDB.Pool.QueryRow(context.Background(), 
			"SELECT created_by_user_id FROM workflows WHERE id = $1", 
			workflowID).Scan(&dbUserID)
		require.NoError(t, err)
		assert.Equal(t, userID, dbUserID)
	})

	t.Run("Database User Access Control", func(t *testing.T) {
		// Create two different users in database
		userEmail1 := fmt.Sprintf("user1-db-%d@example.com", time.Now().UnixNano())
		userID1 := testDB.CreateTestUser(t, userEmail1, "hashed-password")
		
		userEmail2 := fmt.Sprintf("user2-db-%d@example.com", time.Now().UnixNano())
		userID2 := testDB.CreateTestUser(t, userEmail2, "hashed-password")

		// Generate tokens for both users
		token1, err := jwtManager.GenerateToken(context.Background(), userID1, userEmail1, []string{}, 24*time.Hour)
		require.NoError(t, err)
		
		token2, err := jwtManager.GenerateToken(context.Background(), userID2, userEmail2, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// User 1 creates a workflow
		workflowReq := map[string]interface{}{
			"name":        "User 1 Database Workflow",
			"description": "Testing database-level access control",
		}
		workflowBody, _ := json.Marshal(workflowReq)

		req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBuffer(workflowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token1)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		workflowID := response["id"].(string)

		// User 1 can access their own workflow
		req = httptest.NewRequest(http.MethodGet, "/api/workflows/"+workflowID, nil)
		req.Header.Set("Authorization", "Bearer "+token1)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// User 2 cannot access User 1's workflow (database-level access control)
		req = httptest.NewRequest(http.MethodGet, "/api/workflows/"+workflowID, nil)
		req.Header.Set("Authorization", "Bearer "+token2)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Database Login Integration", func(t *testing.T) {
		// Create real user in database with known password
		userEmail := fmt.Sprintf("login-db-%d@example.com", time.Now().UnixNano())
		testPassword := "test-password-123"
		
		// Hash the password properly for storage
		hashedPassword, err := testDB.HashPassword(testPassword)
		require.NoError(t, err)
		
		userID := testDB.CreateTestUser(t, userEmail, hashedPassword)

		// Test successful login with database user
		loginReq := map[string]interface{}{
			"email":    userEmail,
			"password": testPassword,
		}
		loginBody, _ := json.Marshal(loginReq)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.NotEmpty(t, response["token"])
		assert.Equal(t, userID, response["user_id"])

		// Test the returned token works with database
		token := response["token"].(string)
		req = httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Test failed login with wrong password
		loginReq["password"] = "wrong-password"
		loginBody, _ = json.Marshal(loginReq)

		req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Database User Session Persistence", func(t *testing.T) {
		// Test that database users maintain session state correctly
		userEmail := fmt.Sprintf("session-db-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")
		
		// Generate token for real user
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Make multiple requests to verify session persistence
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Verify consistent user identity across requests
			assert.Equal(t, userID, response["user_id"])
			assert.Equal(t, userEmail, response["email"])
		}
	})
}