"""
Mock DeepAgents Runtime Server and Client for testing.

Provides HTTP and WebSocket endpoints that simulate the deepagents-runtime service
using test data from testdata/ directory, plus a mock client that can be used
in integration tests.
"""

import json
import asyncio
import time
from pathlib import Path
from typing import Dict, Any, List, Optional
from unittest.mock import AsyncMock
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import JSONResponse
import uvicorn


class MockDeepAgentsServer:
    """Mock implementation of deepagents-runtime service."""
    
    def __init__(self, testdata_dir: Path = None, scenario: str = "approved"):
        """
        Initialize mock server with test data.
        
        Args:
            testdata_dir: Path to testdata directory containing test data files
            scenario: Test scenario to load data for ("approved", "rejected", "isolation_1")
        """
        if testdata_dir is None:
            testdata_dir = Path(__file__).parent.parent / "testdata"
        
        self.testdata_dir = testdata_dir
        self.scenario = scenario
        self.all_events: List[Dict[str, Any]] = []
        self.thread_state: Dict[str, Any] = {}
        
        # Load test data based on scenario
        self._load_test_data()
        
        # Create FastAPI app
        self.app = FastAPI()
        self._setup_routes()
    
    def _load_test_data(self):
        """Load test data from JSON files based on scenario."""
        # Map scenarios to file names
        scenario_files = {
            "approved": ("all_events.json", "thread_state.json"),
            "rejected": ("rejection_events.json", "rejection_state.json"),
            "isolation_1": ("isolation_events_1.json", "isolation_state_1.json")
        }
        
        if self.scenario not in scenario_files:
            raise ValueError(f"Unknown scenario: {self.scenario}. Available: {list(scenario_files.keys())}")
        
        events_file, state_file = scenario_files[self.scenario]
        
        # Load events file
        events_path = self.testdata_dir / events_file
        with open(events_path, 'r') as f:
            self.all_events = json.load(f)
        
        # Load state file
        state_path = self.testdata_dir / state_file
        with open(state_path, 'r') as f:
            self.thread_state = json.load(f)
    
    def _setup_routes(self):
        """Setup FastAPI routes."""
        
        @self.app.post("/invoke")
        async def invoke():
            """Handle POST /invoke requests."""
            return JSONResponse({
                "thread_id": self.thread_state["thread_id"],
                "status": "started"
            })
        
        @self.app.get("/state/{thread_id}")
        async def get_state(thread_id: str):
            """Handle GET /state/{thread_id} requests."""
            state = self.thread_state.copy()
            state["thread_id"] = thread_id
            return JSONResponse(state)
        
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


def create_mock_server(testdata_dir: Path = None, scenario: str = "approved") -> MockDeepAgentsServer:
    """
    Create a mock deepagents server instance.
    
    Args:
        testdata_dir: Path to testdata directory
        scenario: Test scenario to load data for ("approved", "rejected", "isolation_1")
        
    Returns:
        MockDeepAgentsServer instance
    """
    return MockDeepAgentsServer(testdata_dir, scenario)
