"""
DeepAgents mock setup utilities for refinement integration tests.

Provides WebSocket stream simulation and mock client management
to test the complete refinement workflow including stream processing.
"""

import time
import json
import asyncio
from unittest.mock import patch, AsyncMock, MagicMock
from typing import Dict, Any, List, Optional

# from tests.mock.deepagents_mock import MockDeepAgentsRuntimeClient  # Commented out to use real client


class WebSocketStreamSimulator:
    """
    Simulates WebSocket stream sequences for testing stream processing.
    
    This class emits events that the Orchestrator's proxy logic must catch
    to prove Requirement 3.1 (WebSocket proxy with event extraction) works.
    """
    
    def __init__(self, thread_id: str, generated_files: Dict[str, Any]):
        self.thread_id = thread_id
        self.generated_files = generated_files
        self.events = []
        self._build_event_sequence()
    
    def _build_event_sequence(self):
        """Build realistic event sequence with on_state_update containing files."""
        # Initial state update (no files yet)
        self.events.append({
            "event_type": "on_state_update",
            "data": {
                "messages": "Starting refinement process...",
                "thread_id": self.thread_id
            }
        })
        
        # LLM stream events (simulating processing)
        for i in range(3):
            self.events.append({
                "event_type": "on_llm_stream", 
                "data": {
                    "raw_event": f"Processing step {i+1}...",
                    "thread_id": self.thread_id
                }
            })
        
        # Critical: Final state update with files (this should trigger DB update)
        self.events.append({
            "event_type": "on_state_update",
            "data": {
                "messages": "Refinement completed successfully",
                "thread_id": self.thread_id,
                "files": self.generated_files  # This is what the proxy should extract
            }
        })
        
        # End event
        self.events.append({
            "event_type": "end",
            "data": {
                "thread_id": self.thread_id
            }
        })
    
    async def stream_events(self, websocket_mock):
        """
        Stream events to simulate WebSocket communication.
        
        This method should be called by the WebSocket proxy to simulate
        receiving events from deepagents-runtime.
        """
        for event in self.events:
            # Simulate network delay
            await asyncio.sleep(0.1)
            # Yield event (this is what the proxy should receive)
            yield event


def create_mock_deepagents_client(thread_id_suffix: str = "") -> None:
    """
    Create configured mock DeepAgents client with thread_id.
    
    NOTE: MockDeepAgentsRuntimeClient is commented out, so tests will use real client.
    This function now returns None to indicate no mocking.
    
    Args:
        thread_id_suffix: Suffix to append to thread_id for uniqueness
        
    Returns:
        None (no mock client, use real client)
    """
    print(f"[DEBUG] create_mock_deepagents_client called with suffix: {thread_id_suffix}")
    print("[DEBUG] MockDeepAgentsRuntimeClient is commented out, using real client")
    return None


def create_websocket_stream_simulator(
    thread_id: str, 
    generated_files: Dict[str, Any]
) -> WebSocketStreamSimulator:
    """
    Create WebSocket stream simulator for testing stream processing.
    
    Args:
        thread_id: Thread ID for the simulation
        generated_files: Files to include in final on_state_update event
        
    Returns:
        WebSocketStreamSimulator instance
    """
    return WebSocketStreamSimulator(thread_id, generated_files)


async def simulate_proposal_completion_via_stream(
    proposal_service,
    proposal_id: str,
    thread_id: str,
    generated_files: Dict[str, Any]
):
    """
    Simulate proposal completion via WebSocket stream processing.
    
    This simulates the hybrid event processing where the WebSocket proxy
    extracts files from streaming events and updates the database.
    
    Args:
        proposal_service: ProposalService instance
        proposal_id: Proposal ID
        thread_id: Thread ID
        generated_files: Generated files to include in stream
    """
    # Create stream simulator
    simulator = create_websocket_stream_simulator(thread_id, generated_files)
    
    # Simulate the WebSocket proxy extracting files from the final on_state_update
    final_state_event = None
    for event in simulator.events:
        if event["event_type"] == "on_state_update" and "files" in event.get("data", {}):
            final_state_event = event
    
    if final_state_event:
        # This simulates what the WebSocket proxy should do:
        # Extract files from the stream and update the proposal
        extracted_files = final_state_event["data"]["files"]
        proposal_service.update_proposal_results(
            proposal_id=proposal_id,
            status="completed",
            audit_trail_json="{}",
            generated_files=extracted_files
        )


def patch_deepagents_client(mock_client):
    """
    Context manager for patching DeepAgents client.
    
    Since MockDeepAgentsRuntimeClient is commented out, this returns a no-op context manager.
    
    Args:
        mock_client: Mock client (will be None)
    """
    print(f"[DEBUG] patch_deepagents_client called with mock_client: {mock_client}")
    if mock_client is None:
        print("[DEBUG] No mock client provided, using real DeepAgentsRuntimeClient")
        # Return a no-op context manager
        from contextlib import nullcontext
        return nullcontext()
    else:
        return patch('services.deepagents_client.DeepAgentsRuntimeClient', return_value=mock_client)


class RuntimeCleanupTracker:
    """
    Tracks calls to deepagents-runtime cleanup to verify Requirement 4.5.
    
    This ensures that upon Resolution (Approve/Reject), the system actually
    tells the deepagents-runtime to delete its checkpoints.
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
    
    def get_cleanup_calls_for_thread(self, thread_id: str) -> List[Dict[str, Any]]:
        """Get all cleanup calls for specific thread_id."""
        return [call for call in self.cleanup_calls if call["thread_id"] == thread_id]


# Global cleanup tracker instance
_cleanup_tracker = RuntimeCleanupTracker()


def get_cleanup_tracker() -> RuntimeCleanupTracker:
    """Get the global cleanup tracker instance."""
    return _cleanup_tracker


def mock_deepagents_cleanup_call(thread_id: str, success: bool = True):
    """
    Mock a deepagents-runtime cleanup call.
    
    This should be called by the orchestration service when a proposal
    is resolved to simulate cleanup of deepagents-runtime checkpoints.
    """
    print(f"[DEBUG] Mock cleanup called for thread_id: {thread_id}, success: {success}")
    _cleanup_tracker.record_cleanup_call(thread_id, success)
    return success


def setup_cleanup_tracking():
    """
    Set up cleanup tracking by patching the cleanup method.
    
    This patches the deepagents client cleanup method to track calls.
    """
    async def mock_cleanup(self, thread_id: str):
        print(f"[DEBUG] Mock async cleanup called for thread_id: {thread_id}")
        result = mock_deepagents_cleanup_call(thread_id, True)
        print(f"[DEBUG] Mock cleanup result: {result}")
        return result
    
    # Patch the real client to ensure cleanup tracking works
    from services.deepagents_client import DeepAgentsRuntimeClient
    print("[DEBUG] Setting up cleanup tracking patch for real DeepAgentsRuntimeClient")
    return patch.object(DeepAgentsRuntimeClient, 'cleanup_thread_data', mock_cleanup)