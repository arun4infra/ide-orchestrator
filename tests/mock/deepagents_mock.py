"""
Mock DeepAgents Runtime Server for testing.

Provides HTTP and WebSocket endpoints that simulate the deepagents-runtime service
using test data from testdata/ directory.
"""

import json
from pathlib import Path
from typing import Dict, Any, List
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import JSONResponse
import uvicorn


class MockDeepAgentsServer:
    """Mock implementation of deepagents-runtime service."""
    
    def __init__(self, testdata_dir: Path = None):
        """
        Initialize mock server with test data.
        
        Args:
            testdata_dir: Path to testdata directory containing all_events.json and thread_state.json
        """
        if testdata_dir is None:
            testdata_dir = Path(__file__).parent.parent / "testdata"
        
        self.testdata_dir = testdata_dir
        self.all_events: List[Dict[str, Any]] = []
        self.thread_state: Dict[str, Any] = {}
        
        # Load test data
        self._load_test_data()
        
        # Create FastAPI app
        self.app = FastAPI()
        self._setup_routes()
    
    def _load_test_data(self):
        """Load test data from JSON files."""
        # Load all_events.json
        events_path = self.testdata_dir / "all_events.json"
        if events_path.exists():
            with open(events_path, 'r') as f:
                self.all_events = json.load(f)
        else:
            raise FileNotFoundError(f"Test data not found: {events_path}")
        
        # Load thread_state.json
        state_path = self.testdata_dir / "thread_state.json"
        if state_path.exists():
            with open(state_path, 'r') as f:
                self.thread_state = json.load(f)
        else:
            raise FileNotFoundError(f"Test data not found: {state_path}")
    
    def _setup_routes(self):
        """Setup FastAPI routes."""
        
        @self.app.post("/invoke")
        async def invoke():
            """Handle POST /invoke requests."""
            thread_id = self.thread_state.get("thread_id", "test-thread-id")
            return JSONResponse({
                "thread_id": thread_id,
                "status": "started"
            })
        
        @self.app.get("/state/{thread_id}")
        async def get_state(thread_id: str):
            """Handle GET /state/{thread_id} requests."""
            return JSONResponse(self.thread_state)
        
        @self.app.websocket("/stream/{thread_id}")
        async def stream_events(websocket: WebSocket, thread_id: str):
            """Handle WebSocket /stream/{thread_id} requests."""
            await websocket.accept()
            
            try:
                # Stream all events sequentially
                for event in self.all_events:
                    await websocket.send_json(event)
                
                # Close connection after streaming all events
                await websocket.close()
            except WebSocketDisconnect:
                pass
    
    def run(self, host: str = "127.0.0.1", port: int = 8000):
        """
        Run the mock server.
        
        Args:
            host: Host to bind to
            port: Port to bind to
        """
        uvicorn.run(self.app, host=host, port=port)
    
    def get_app(self) -> FastAPI:
        """Get the FastAPI application instance."""
        return self.app


def create_mock_server(testdata_dir: Path = None) -> MockDeepAgentsServer:
    """
    Create a mock deepagents server instance.
    
    Args:
        testdata_dir: Path to testdata directory
        
    Returns:
        MockDeepAgentsServer instance
    """
    return MockDeepAgentsServer(testdata_dir)
