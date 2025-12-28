package mock

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
)

// MockDeepAgentsServer provides a mock implementation of the deepagents-runtime service
type MockDeepAgentsServer struct {
	server     *httptest.Server
	allEvents  []map[string]interface{}
	threadState map[string]interface{}
	upgrader   websocket.Upgrader
}

// NewMockDeepAgentsServer creates a new mock server with test data
func NewMockDeepAgentsServer() (*MockDeepAgentsServer, error) {
	mock := &MockDeepAgentsServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for testing
			},
		},
	}

	// Load test data
	if err := mock.loadTestData(); err != nil {
		return nil, fmt.Errorf("failed to load test data: %w", err)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/invoke", mock.handleInvoke)
	mux.HandleFunc("/state/", mock.handleState)
	mux.HandleFunc("/stream/", mock.handleStream)

	mock.server = httptest.NewServer(mux)
	return mock, nil
}

// URL returns the mock server URL
func (m *MockDeepAgentsServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockDeepAgentsServer) Close() {
	m.server.Close()
}

// loadTestData reads the test data files
func (m *MockDeepAgentsServer) loadTestData() error {
	// Load all_events.json
	eventsPath := filepath.Join("tests", "testdata", "all_events.json")
	eventsData, err := ioutil.ReadFile(eventsPath)
	if err != nil {
		return fmt.Errorf("failed to read all_events.json: %w", err)
	}

	if err := json.Unmarshal(eventsData, &m.allEvents); err != nil {
		return fmt.Errorf("failed to parse all_events.json: %w", err)
	}

	// Load thread_state.json
	statePath := filepath.Join("tests", "testdata", "thread_state.json")
	stateData, err := ioutil.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read thread_state.json: %w", err)
	}

	if err := json.Unmarshal(stateData, &m.threadState); err != nil {
		return fmt.Errorf("failed to parse thread_state.json: %w", err)
	}

	return nil
}

// handleInvoke handles POST /invoke requests
func (m *MockDeepAgentsServer) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return the static thread ID from our test data
	threadID := m.threadState["thread_id"].(string)
	
	response := map[string]interface{}{
		"thread_id": threadID,
		"status":    "started",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleState handles GET /state/{thread_id} requests
func (m *MockDeepAgentsServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract thread ID from URL path
	threadID := strings.TrimPrefix(r.URL.Path, "/state/")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	// Return the thread state data
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m.threadState)
}

// handleStream handles GET /stream/{thread_id} WebSocket requests
func (m *MockDeepAgentsServer) handleStream(w http.ResponseWriter, r *http.Request) {
	// Extract thread ID from URL path
	threadID := strings.TrimPrefix(r.URL.Path, "/stream/")
	if threadID == "" {
		http.Error(w, "Thread ID required", http.StatusBadRequest)
		return
	}

	// Upgrade to WebSocket
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Stream all events sequentially
	for _, event := range m.allEvents {
		if err := conn.WriteJSON(event); err != nil {
			log.Printf("Failed to write WebSocket message: %v", err)
			break
		}
	}

	// Close the connection after streaming all events
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}