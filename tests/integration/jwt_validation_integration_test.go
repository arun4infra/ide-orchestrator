package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestJWTValidationIntegration consolidates all JWT token validation tests
// This includes token generation, validation, edge cases, and authentication flow tests
func TestJWTValidationIntegration(t *testing.T) {
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
	api.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

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

	// ============================================================
	// Section 1: Basic JWT Token Generation and Validation
	// ============================================================

	t.Run("Basic Token Generation and Validation", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-basic-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Generate token
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		// Validate token
		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, userEmail, claims.Username)
		assert.True(t, claims.ExpiresAt.After(time.Now()))
	})

	t.Run("Token with Roles", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-roles-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		roles := []string{"admin", "user", "editor"}
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, roles, 24*time.Hour)
		require.NoError(t, err)

		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, roles, claims.Roles)
	})

	// ============================================================
	// Section 2: JWT Edge Cases
	// ============================================================

	t.Run("Empty User ID", func(t *testing.T) {
		// Test JWT generation with empty user ID
		token, err := jwtManager.GenerateToken(context.Background(), "", "test@example.com", []string{}, 24*time.Hour)
		// Should either fail or generate token with empty user ID
		if err == nil {
			claims, err := jwtManager.ValidateToken(context.Background(), token)
			require.NoError(t, err)
			assert.Equal(t, "", claims.UserID)
		}
	})

	t.Run("Empty Username", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-empty-username-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Test JWT generation with empty username
		token, err := jwtManager.GenerateToken(context.Background(), userID, "", []string{}, 24*time.Hour)
		// Should either fail or generate token with empty username
		if err == nil {
			claims, err := jwtManager.ValidateToken(context.Background(), token)
			require.NoError(t, err)
			assert.Equal(t, "", claims.Username)
		}
	})

	t.Run("Special Characters in Claims", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-special-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Test JWT with special characters
		specialUsername := "user!@#$%^&*()_+-=[]{}|;':\",./<>?"
		token, err := jwtManager.GenerateToken(context.Background(), userID, specialUsername, []string{}, 24*time.Hour)
		require.NoError(t, err)

		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, specialUsername, claims.Username)
	})

	t.Run("Very Long Claims", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-long-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Test JWT with very long user ID and username (1000+ chars)
		longUsername := strings.Repeat("a", 1000) + "@example.com"
		token, err := jwtManager.GenerateToken(context.Background(), userID, longUsername, []string{}, 24*time.Hour)
		require.NoError(t, err)

		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, longUsername, claims.Username)
	})

	t.Run("Unicode Characters in Claims", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-unicode-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Test JWT with unicode characters
		unicodeUsername := "用户名@例子.com"
		token, err := jwtManager.GenerateToken(context.Background(), userID, unicodeUsername, []string{}, 24*time.Hour)
		require.NoError(t, err)

		claims, err := jwtManager.ValidateToken(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, unicodeUsername, claims.Username)
	})

	// ============================================================
	// Section 3: Token Expiration Tests
	// ============================================================

	t.Run("Expired Token Handling", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-expired-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Generate token with very short expiration (1 millisecond)
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 1*time.Millisecond)
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(10 * time.Millisecond)

		// Try to use expired token
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should be rejected due to expiration
		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"].(string), "token")
	})

	t.Run("Token Near Expiration", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-near-expiry-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		// Generate token with 5 second expiration (enough time to make request)
		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 5*time.Second)
		require.NoError(t, err)

		// Use token immediately - should work
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	// ============================================================
	// Section 4: Malformed Token Validation
	// ============================================================

	t.Run("Malformed Token Formats", func(t *testing.T) {
		testCases := []struct {
			name   string
			header string
		}{
			{"Missing Bearer prefix", "invalid-token"},
			{"Empty Bearer", "Bearer "},
			{"Invalid JWT format", "Bearer invalid.jwt.token"},
			{"Malformed header", "NotBearer token"},
			{"Only Bearer keyword", "Bearer"},
			{"Double Bearer", "Bearer Bearer token"},
			{"Lowercase bearer", "bearer valid-token"},
			{"Extra spaces", "Bearer  token"},
			{"Tab separator", "Bearer\ttoken"},
			{"Empty string", ""},
			{"Just spaces", "   "},
			{"Random base64", "Bearer " + "YWJjZGVm"},
			{"Incomplete JWT segments", "Bearer header.payload"},
			{"Too many JWT segments", "Bearer a.b.c.d"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
				if tc.header != "" {
					req.Header.Set("Authorization", tc.header)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusUnauthorized, w.Code)
			})
		}
	})

	// ============================================================
	// Section 5: Authentication Required Tests
	// ============================================================

	t.Run("Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Missing authorization header")
	})

	t.Run("Public Endpoints No Auth Required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])
	})

	// ============================================================
	// Section 6: Valid Token Access Tests
	// ============================================================

	t.Run("Valid Token Access", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-valid-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

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

	t.Run("Token Reuse Validation", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-reuse-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// Use the same token multiple times - should work (JWT is stateless)
		for i := 0; i < 5; i++ {
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
		}
	})

	t.Run("Concurrent Token Usage", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-concurrent-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		const numRequests = 10
		results := make(chan int, numRequests)
		userIDs := make(chan string, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				results <- w.Code

				if w.Code == http.StatusOK {
					var response map[string]interface{}
					json.Unmarshal(w.Body.Bytes(), &response)
					if uid, ok := response["user_id"].(string); ok {
						userIDs <- uid
					}
				}
			}()
		}

		// Collect results - all should succeed
		for i := 0; i < numRequests; i++ {
			select {
			case statusCode := <-results:
				assert.Equal(t, http.StatusOK, statusCode)
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent requests")
			}
		}

		// Verify all requests returned the same user ID
		for i := 0; i < numRequests; i++ {
			select {
			case returnedUserID := <-userIDs:
				assert.Equal(t, userID, returnedUserID)
			case <-time.After(1 * time.Second):
				break
			}
		}
	})

	// ============================================================
	// Section 7: Token Claims Extraction Tests
	// ============================================================

	t.Run("Token Claims Extraction with Workflow Creation", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-claims-%d@example.com", time.Now().UnixNano())
		userID := testDB.CreateTestUser(t, userEmail, "hashed-password")

		token, err := jwtManager.GenerateToken(context.Background(), userID, userEmail, []string{}, 24*time.Hour)
		require.NoError(t, err)

		workflowReq := map[string]interface{}{
			"name":        "JWT Claims Test Workflow",
			"description": "Testing claims extraction",
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

		assert.NotEmpty(t, response["id"])
		assert.Equal(t, "JWT Claims Test Workflow", response["name"])

		// Verify workflow is associated with correct user
		workflowID := response["id"].(string)
		var dbUserID string
		err = testDB.Pool.QueryRow(context.Background(),
			"SELECT created_by_user_id FROM workflows WHERE id = $1",
			workflowID).Scan(&dbUserID)
		require.NoError(t, err)
		assert.Equal(t, userID, dbUserID)
	})

	// ============================================================
	// Section 8: User Access Control Tests
	// ============================================================

	t.Run("User Access Control - Own Resources Only", func(t *testing.T) {
		userEmail1 := fmt.Sprintf("jwt-user1-%d@example.com", time.Now().UnixNano())
		userID1 := testDB.CreateTestUser(t, userEmail1, "hashed-password")

		userEmail2 := fmt.Sprintf("jwt-user2-%d@example.com", time.Now().UnixNano())
		userID2 := testDB.CreateTestUser(t, userEmail2, "hashed-password")

		token1, err := jwtManager.GenerateToken(context.Background(), userID1, userEmail1, []string{}, 24*time.Hour)
		require.NoError(t, err)

		token2, err := jwtManager.GenerateToken(context.Background(), userID2, userEmail2, []string{}, 24*time.Hour)
		require.NoError(t, err)

		// User 1 creates a workflow
		workflowReq := map[string]interface{}{
			"name":        "User 1 Workflow",
			"description": "Testing access control",
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

		// User 2 cannot access User 1's workflow
		req = httptest.NewRequest(http.MethodGet, "/api/workflows/"+workflowID, nil)
		req.Header.Set("Authorization", "Bearer "+token2)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	// ============================================================
	// Section 9: Login Integration Tests
	// ============================================================

	t.Run("Login Integration with Database", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-login-%d@example.com", time.Now().UnixNano())
		testPassword := "test-password-123"

		hashedPassword, err := testDB.HashPassword(testPassword)
		require.NoError(t, err)

		userID := testDB.CreateTestUser(t, userEmail, hashedPassword)

		// Test successful login
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

		// Test the returned token works
		token := response["token"].(string)
		req = httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Login with Wrong Password", func(t *testing.T) {
		userEmail := fmt.Sprintf("jwt-login-fail-%d@example.com", time.Now().UnixNano())
		testPassword := "correct-password"

		hashedPassword, err := testDB.HashPassword(testPassword)
		require.NoError(t, err)

		testDB.CreateTestUser(t, userEmail, hashedPassword)

		loginReq := map[string]interface{}{
			"email":    userEmail,
			"password": "wrong-password",
		}
		loginBody, _ := json.Marshal(loginReq)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Login with Non-Existent User", func(t *testing.T) {
		loginReq := map[string]interface{}{
			"email":    "nonexistent@example.com",
			"password": "any-password",
		}
		loginBody, _ := json.Marshal(loginReq)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(loginBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}
