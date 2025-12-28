package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/auth"
	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/gateway"
	"github.com/bizmatters/agent-builder/ide-orchestrator/internal/orchestration"
	"github.com/bizmatters/agent-builder/ide-orchestrator/tests/helpers"
	"github.com/bizmatters/agent-builder/ide-orchestrator/tests/mock"
)

func TestRefinementIntegration(t *testing.T) {
	// Setup mock deepagents-runtime server
	mockServer, err := mock.NewMockDeepAgentsServer()
	require.NoError(t, err)
	defer mockServer.Close()

	// Set mock URL environment variable
	os.Setenv("MOCK_SPEC_ENGINE_URL", mockServer.URL())
	defer os.Unsetenv("MOCK_SPEC_ENGINE_URL")

	// Setup test environment with real infrastructure
	testDB := helpers.NewTestDatabase(t)
	defer testDB.Close()

	// Use transaction-based isolation instead of table cleanup
	txCtx, rollback := testDB.BeginTransaction(t)
	defer rollback()

	// Get configuration (will use mock URL due to environment variable)
	config := SetupInClusterEnvironment()
	t.Logf("Using mock infrastructure - Database: %s, SpecEngine: %s", config.DatabaseURL, config.SpecEngineURL)

	// Initialize services
	specEngineClient := orchestration.NewSpecEngineClient(testDB.Pool)
	orchestrationService := orchestration.NewService(testDB.Pool, specEngineClient)
	
	jwtManager, err := auth.NewJWTManager()
	require.NoError(t, err)

	gatewayHandler := gateway.NewHandler(orchestrationService, jwtManager, testDB.Pool)
	wsProxy := gateway.NewWebSocketProxy(testDB.Pool, mockServer.URL())

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	
	api := router.Group("/api")
	protected := api.Group("")
	protected.Use(auth.RequireAuth(jwtManager))
	
	protected.POST("/workflows", gatewayHandler.CreateWorkflow)
	protected.POST("/workflows/:id/refinements", gatewayHandler.CreateRefinement)
	protected.POST("/refinements/:proposalId/approve", gatewayHandler.ApproveProposal)
	protected.POST("/refinements/:proposalId/reject", gatewayHandler.RejectProposal)
	protected.GET("/ws/refinements/:thread_id", wsProxy.StreamRefinement)

	t.Run("Complete Refinement Workflow", func(t *testing.T) {
		// Setup test data
		userID := testDB.CreateTestUser(t, "refinement@example.com", "hashed-password")
		token, err := jwtManager.GenerateToken(
			context.Background(),
			userID, 
			"refinement@example.com",
			[]string{"user"},
			24*time.Hour,
		)
		require.NoError(t, err)

		// Step 1: Create workflow
		workflowReq := helpers.CreateTestWorkflowRequest(
			"Refinement Test Workflow",
			"Workflow for testing refinements",
			helpers.DefaultTestWorkflow.Specification,
		)
		workflowBody, _ := json.Marshal(workflowReq)

		req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBuffer(workflowBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusCreated, w.Code)

		var workflowResponse map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &workflowResponse)
		require.NoError(t, err)

		workflowID := workflowResponse["id"].(string)

		// Step 2: Create refinement
		refinementReq := helpers.CreateTestRefinementRequest(
			"Add error handling to the workflow",
			"The current workflow lacks proper error handling mechanisms",
		)
		refinementBody, _ := json.Marshal(refinementReq)

		req = httptest.NewRequest(
			http.MethodPost,
			"/api/workflows/"+workflowID+"/refinements",
			bytes.NewBuffer(refinementBody),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		var refinementResponse map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &refinementResponse)
		require.NoError(t, err)

		threadID := refinementResponse["thread_id"].(string)
		assert.NotEmpty(t, threadID)

		// Step 3: Wait for processing to complete and validate database persistence
		time.Sleep(500 * time.Millisecond)

		// Step 4: Validate that data was persisted to the database
		// Query the proposals table to verify the generated_files match our mock data
		var proposalID string
		var generatedFiles json.RawMessage
		var status string
		
		err = testDB.Pool.QueryRow(txCtx, 
			"SELECT id, generated_files, status FROM proposals WHERE thread_id = $1", 
			threadID,
		).Scan(&proposalID, &generatedFiles, &status)
		require.NoError(t, err, "Proposal should be created in database")

		// Verify the proposal status
		assert.Equal(t, "completed", status)

		// Verify the generated files match our mock data
		var dbFiles map[string]interface{}
		err = json.Unmarshal(generatedFiles, &dbFiles)
		require.NoError(t, err)

		// Check that key files from our mock data are present
		assert.Contains(t, dbFiles, "/definition.json")
		assert.Contains(t, dbFiles, "/THE_SPEC/requirements.md")
		assert.Contains(t, dbFiles, "/THE_CAST/GreetingAgent.md")

		// Verify the definition.json content structure
		definitionFile, ok := dbFiles["/definition.json"].(map[string]interface{})
		require.True(t, ok)
		content, ok := definitionFile["content"].([]interface{})
		require.True(t, ok)
		
		// Parse the JSON content to verify it's valid
		var definitionJSON map[string]interface{}
		jsonContent := strings.Join(func() []string {
			result := make([]string, len(content))
			for i, v := range content {
				result[i] = v.(string)
			}
			return result
		}(), "")
		err = json.Unmarshal([]byte(jsonContent), &definitionJSON)
		require.NoError(t, err)
		
		assert.Equal(t, "GreetingWorkflow", definitionJSON["name"])
		assert.Equal(t, "1.0.0", definitionJSON["version"])
	})

	t.Run("WebSocket Streaming", func(t *testing.T) {
		// Setup test data
		userID := testDB.CreateTestUser(t, "websocket@example.com", "hashed-password")
		token, err := jwtManager.GenerateToken(
			context.Background(),
			userID,
			"websocket@example.com", 
			[]string{"user"},
			24*time.Hour,
		)
		require.NoError(t, err)

		// Create a test server for WebSocket testing
		testServer := httptest.NewServer(router)
		defer testServer.Close()

		// Convert HTTP URL to WebSocket URL
		wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/api/ws/refinements/test-thread-123"

		// Set up WebSocket connection with authentication
		header := http.Header{}
		header.Set("Authorization", "Bearer "+token)

		dialer := websocket.Dialer{}
		conn, _, err := dialer.Dial(wsURL, header)
		require.NoError(t, err)
		defer conn.Close()

		// Set up mock thread state - not needed as mock server handles this automatically
		// The mock server will stream events from all_events.json

		// Read WebSocket messages
		messages := make([]map[string]interface{}, 0)
		timeout := time.After(5 * time.Second)

		for {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for WebSocket messages")
			default:
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				var message map[string]interface{}
				err := conn.ReadJSON(&message)
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						break
					}
					continue
				}

				messages = append(messages, message)

				// Check for end event
				if eventType, ok := message["event_type"].(string); ok && eventType == "end" {
					goto done
				}
			}
		}

	done:
		// Verify we received messages
		assert.Greater(t, len(messages), 0)

		// Verify message structure
		for _, msg := range messages {
			assert.Contains(t, msg, "event_type")
			assert.Contains(t, msg, "data")
		}

		// Should have at least one state update and one end event
		hasStateUpdate := false
		hasEndEvent := false
		for _, msg := range messages {
			eventType := msg["event_type"].(string)
			if eventType == "on_state_update" {
				hasStateUpdate = true
			}
			if eventType == "end" {
				hasEndEvent = true
			}
		}

		assert.True(t, hasStateUpdate, "Should have received state update event")
		assert.True(t, hasEndEvent, "Should have received end event")
	})

	t.Run("Proposal Approval", func(t *testing.T) {
		// This test would require implementing the proposal approval endpoints
		// For now, we'll test the basic structure

		userID := testDB.CreateTestUser(t, "approval@example.com", "hashed-password")
		token, err := jwtManager.GenerateToken(
			context.Background(),
			userID,
			"approval@example.com",
			[]string{"user"},
			24*time.Hour,
		)
		require.NoError(t, err)

		// Test approving a non-existent proposal
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/refinements/non-existent-proposal/approve",
			nil,
		)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return 404 for non-existent proposal
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Proposal Rejection", func(t *testing.T) {
		userID := testDB.CreateTestUser(t, "rejection@example.com", "hashed-password")
		token, err := jwtManager.GenerateToken(
			context.Background(),
			userID,
			"rejection@example.com",
			[]string{"user"},
			24*time.Hour,
		)
		require.NoError(t, err)

		// Test rejecting a non-existent proposal
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/refinements/non-existent-proposal/reject",
			nil,
		)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should return 404 for non-existent proposal
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("Refinement Validation", func(t *testing.T) {
		userID := testDB.CreateTestUser(t, "validation@example.com", "hashed-password")
		token, err := jwtManager.GenerateToken(
			context.Background(),
			userID,
			"validation@example.com",
			[]string{"user"},
			24*time.Hour,
		)
		require.NoError(t, err)

		workflowID := testDB.CreateTestWorkflow(
			t,
			userID,
			"Validation Test Workflow",
			"For testing refinement validation",
		)

		// Test invalid refinement (missing instructions)
		invalidReq := map[string]interface{}{
			"context": "Missing instructions",
		}
		invalidBody, _ := json.Marshal(invalidReq)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/workflows/"+workflowID+"/refinements",
			bytes.NewBuffer(invalidBody),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Test refinement on non-existent workflow
		validReq := helpers.CreateTestRefinementRequest(
			"Valid instructions",
			"Valid context",
		)
		validBody, _ := json.Marshal(validReq)

		req = httptest.NewRequest(
			http.MethodPost,
			"/api/workflows/non-existent-workflow/refinements",
			bytes.NewBuffer(validBody),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSpecEngineIntegration(t *testing.T) {
	// Setup mock deepagents-runtime server
	mockServer, err := mock.NewMockDeepAgentsServer()
	require.NoError(t, err)
	defer mockServer.Close()

	// Test direct integration with mock Spec Engine
	specEngineURL := mockServer.URL()

	t.Run("Spec Engine Invoke", func(t *testing.T) {
		invokeReq := map[string]interface{}{
			"job_id":     "test-job-123",
			"trace_id":   "test-trace-123",
			"agent_definition": helpers.DefaultTestWorkflow.Specification,
			"input_payload": map[string]interface{}{
				"instructions": "Test refinement",
				"context":      "Test context",
			},
		}
		invokeBody, _ := json.Marshal(invokeReq)

		resp, err := http.Post(
			specEngineURL+"/invoke",
			"application/json",
			bytes.NewBuffer(invokeBody),
		)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var invokeResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&invokeResponse)
		require.NoError(t, err)

		assert.Equal(t, "test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574", invokeResponse["thread_id"])
		assert.Equal(t, "started", invokeResponse["status"])
	})

	t.Run("Spec Engine State", func(t *testing.T) {
		// Check state using the thread ID from our test data
		threadID := "test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574"
		
		resp, err := http.Get(specEngineURL + "/state/" + threadID)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var stateResponse map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&stateResponse)
		require.NoError(t, err)

		assert.Equal(t, threadID, stateResponse["thread_id"])
		assert.Equal(t, "completed", stateResponse["status"])
		assert.NotNil(t, stateResponse["result"])
		assert.NotNil(t, stateResponse["generated_files"])

		// Verify generated files structure
		generatedFiles, ok := stateResponse["generated_files"].(map[string]interface{})
		require.True(t, ok)
		
		assert.Contains(t, generatedFiles, "/definition.json")
		assert.Contains(t, generatedFiles, "/THE_SPEC/requirements.md")
		assert.Contains(t, generatedFiles, "/THE_CAST/GreetingAgent.md")
	})

	t.Run("WebSocket Streaming", func(t *testing.T) {
		// Convert HTTP URL to WebSocket URL
		wsURL := "ws" + strings.TrimPrefix(specEngineURL, "http") + "/stream/test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574"

		dialer := websocket.Dialer{}
		conn, _, err := dialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer conn.Close()

		// Read WebSocket messages
		messages := make([]map[string]interface{}, 0)
		timeout := time.After(5 * time.Second)

		for {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for WebSocket messages")
			default:
				conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				var message map[string]interface{}
				err := conn.ReadJSON(&message)
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
						goto done
					}
					continue
				}

				messages = append(messages, message)

				// Check for end event
				if eventType, ok := message["event_type"].(string); ok && eventType == "end" {
					goto done
				}
			}
		}

	done:
		// Verify we received messages
		assert.Greater(t, len(messages), 0)

		// Verify message structure
		for _, msg := range messages {
			assert.Contains(t, msg, "event_type")
			assert.Contains(t, msg, "data")
		}

		// Should have at least one state update and one end event
		hasStateUpdate := false
		hasEndEvent := false
		for _, msg := range messages {
			eventType := msg["event_type"].(string)
			if eventType == "on_state_update" {
				hasStateUpdate = true
			}
			if eventType == "end" {
				hasEndEvent = true
			}
		}

		assert.True(t, hasStateUpdate, "Should have received state update event")
		assert.True(t, hasEndEvent, "Should have received end event")
	})
}