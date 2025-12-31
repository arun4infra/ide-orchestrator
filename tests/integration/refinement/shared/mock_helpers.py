"""
DeepAgents mock setup utilities for refinement integration tests.

Provides simple HTTP mock for deepagents-runtime endpoints to test
the actual production code paths without complex WebSocket logic.
"""

import time
import json
import asyncio
import os
from typing import Dict, Any, List, Optional
from pathlib import Path

# Import for in-process HTTP mock server
import pytest_httpserver


class DeepAgentsMockServer:
    """
    Simple HTTP mock server for deepagents-runtime endpoints.
    
    This follows the integration testing pattern by mocking only the
    external deepagents-runtime HTTP endpoints that the production
    DeepAgentsRuntimeClient calls.
    """
    
    def __init__(self, scenario: str = "approved"):
        self.scenario = scenario
        self.http_server = None
        self.http_port = None
        self.test_data = {}
        self.thread_states = {}
        self._load_test_data()
        
    def _load_test_data(self):
        """Load real test data from testdata directory."""
        testdata_dir = Path(__file__).parent.parent.parent.parent / "testdata"
        
        scenario_files = {
            "approved": "thread_state.json",
            "rejected": "rejection_state.json", 
            "isolation_1": "isolation_state_1.json"
        }
        
        if self.scenario in scenario_files:
            state_path = testdata_dir / scenario_files[self.scenario]
            if state_path.exists():
                with open(state_path, 'r') as f:
                    self.test_data = json.load(f)
    
    async def start(self):
        """Start the HTTP mock server."""
        # Start HTTP server
        self.http_server = pytest_httpserver.HTTPServer(host="127.0.0.1", port=0)
        self.http_server.start()
        self.http_port = self.http_server.port
        
        # Setup HTTP endpoints
        self._setup_http_endpoints()
        
        print(f"[DEBUG] Mock deepagents-runtime started on port: {self.http_port}")
        
        # Set environment variable for production code to use mock
        os.environ["DEEPAGENTS_RUNTIME_URL"] = f"http://127.0.0.1:{self.http_port}"
        print(f"[DEBUG] Set DEEPAGENTS_RUNTIME_URL to: {os.environ['DEEPAGENTS_RUNTIME_URL']}")
    
    def _setup_http_endpoints(self):
        """Setup HTTP endpoints that match deepagents-runtime API."""
        
        def invoke_handler(request):
            """Handle POST /invoke requests."""
            thread_id = f"test-thread-{int(time.time() * 1000000)}"
            self.thread_states[thread_id] = {
                "status": "running",
                "generated_files": {}
            }
            
            print(f"[DEBUG] Mock invoke handler called, created thread_id: {thread_id}")
            
            # Simulate processing completion after a short delay
            asyncio.create_task(self._complete_processing(thread_id))
            
            return pytest_httpserver.Response(
                json.dumps({"thread_id": thread_id}),
                status=200,
                headers={"Content-Type": "application/json"}
            )
        
        def state_handler(request):
            """Handle GET /state/{thread_id} requests."""
            thread_id = request.path_info.split('/')[-1]
            
            print(f"[DEBUG] Mock state handler called for thread_id: {thread_id}")
            
            if thread_id in self.thread_states:
                state = self.thread_states[thread_id]
                print(f"[DEBUG] Returning state for {thread_id}: {state['status']}")
                return pytest_httpserver.Response(
                    json.dumps(state),
                    status=200,
                    headers={"Content-Type": "application/json"}
                )
            else:
                print(f"[DEBUG] Thread {thread_id} not found in states")
                return pytest_httpserver.Response("Not found", status=404)
        
        # Register endpoints
        self.http_server.expect_request("/invoke", method="POST").respond_with_handler(invoke_handler)
        self.http_server.expect_request("/state/*", method="GET").respond_with_handler(state_handler)
    
    async def _complete_processing(self, thread_id: str):
        """Simulate processing completion after a delay."""
        print(f"[DEBUG] Starting processing simulation for thread_id: {thread_id}")
        await asyncio.sleep(1)  # Simulate processing time
        
        if thread_id in self.thread_states:
            self.thread_states[thread_id] = {
                "status": "completed",
                "generated_files": self.test_data.get("generated_files", {}),
                "result": "Processing completed successfully"
            }
            print(f"[DEBUG] Mock processing completed for thread_id: {thread_id}")
    
    async def stop(self):
        """Stop the mock server and cleanup."""
        if self.http_server:
            self.http_server.stop()
            print(f"[DEBUG] HTTP server stopped")
        
        # Clean up environment variable
        if "DEEPAGENTS_RUNTIME_URL" in os.environ:
            del os.environ["DEEPAGENTS_RUNTIME_URL"]
            print(f"[DEBUG] Cleaned up DEEPAGENTS_RUNTIME_URL environment variable")
        
        print(f"[DEBUG] Mock deepagents-runtime stopped")


def create_mock_deepagents_server(scenario: str = "approved") -> DeepAgentsMockServer:
    """
    Create simple HTTP mock server for deepagents-runtime.
    
    This follows the integration testing pattern by mocking only the
    external HTTP endpoints that the production code calls.
    
    Args:
        scenario: Test scenario to load data for
        
    Returns:
        DeepAgentsMockServer instance
    """
    print(f"[DEBUG] Creating mock deepagents server for scenario: {scenario}")
    return DeepAgentsMockServer(scenario)


async def wait_for_proposal_completion_via_orchestration(
    proposal_service,
    proposal_id: str,
    timeout: int = 30
):
    """
    Wait for proposal completion via the actual orchestration service processing.
    
    This follows the integration testing pattern by waiting for the real
    orchestration service to complete its deepagents-runtime processing.
    
    Args:
        proposal_service: ProposalService instance
        proposal_id: Proposal ID to monitor
        timeout: Maximum wait time in seconds
    """
    print(f"[DEBUG] Waiting for proposal completion via orchestration service for proposal_id: {proposal_id}")
    
    start_time = time.time()
    while time.time() - start_time < timeout:
        # Check proposal status through production service
        try:
            # Use production service to check status
            from .database_helpers import get_proposal_by_id
            from api.dependencies import get_database_url
            
            proposal = await get_proposal_by_id(proposal_id, get_database_url())
            if proposal and proposal["status"] == "completed":
                print(f"[DEBUG] Proposal {proposal_id} completed via orchestration service")
                return proposal
            elif proposal and proposal["status"] == "failed":
                print(f"[DEBUG] Proposal {proposal_id} failed")
                raise Exception(f"Proposal processing failed")
                
        except Exception as e:
            print(f"[DEBUG] Error checking proposal status: {e}")
        
        # Wait before next check
        await asyncio.sleep(0.5)
    
    raise TimeoutError(f"Proposal {proposal_id} did not complete within {timeout} seconds")


# Cleanup tracking for testing requirement 4.5
class RuntimeCleanupTracker:
    """
    Tracks calls to deepagents-runtime cleanup to verify Requirement 4.5.
    """
    
    def __init__(self):
        self.cleanup_calls = []
    
    def record_cleanup_call(self, thread_id: str, success: bool = True):
        """Record a cleanup call for verification."""
        self.cleanup_calls.append({
            "thread_id": thread_id,
            "success": success,
            "timestamp": time.time()
        })
    
    def was_cleanup_called(self, thread_id: str) -> bool:
        """Check if cleanup was called for specific thread_id."""
        return any(call["thread_id"] == thread_id for call in self.cleanup_calls)
    
    def get_cleanup_calls_for_thread(self, thread_id: str) -> list:
        """Get all cleanup calls for specific thread_id."""
        return [call for call in self.cleanup_calls if call["thread_id"] == thread_id]


# Global cleanup tracker instance
_cleanup_tracker = RuntimeCleanupTracker()


def get_cleanup_tracker() -> RuntimeCleanupTracker:
    """Get the global cleanup tracker instance."""
    return _cleanup_tracker


def mock_deepagents_cleanup_call(thread_id: str, success: bool = True):
    """Mock a deepagents-runtime cleanup call."""
    print(f"[DEBUG] Mock cleanup called for thread_id: {thread_id}, success: {success}")
    _cleanup_tracker.record_cleanup_call(thread_id, success)
    return success


def setup_cleanup_tracking():
    """Set up cleanup tracking by patching the cleanup method."""
    from unittest.mock import patch
    
    async def mock_cleanup(self, thread_id: str):
        print(f"[DEBUG] Mock async cleanup called for thread_id: {thread_id}")
        result = mock_deepagents_cleanup_call(thread_id, True)
        print(f"[DEBUG] Mock cleanup result: {result}")
        return result
    
    # Patch the real client to ensure cleanup tracking works
    from services.deepagents_client import DeepAgentsRuntimeClient
    print("[DEBUG] Setting up cleanup tracking patch for real DeepAgentsRuntimeClient")
    return patch.object(DeepAgentsRuntimeClient, 'cleanup_thread_data', mock_cleanup)