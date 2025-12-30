"""
Refinement workflow integration tests.

Tests complete refinement workflow including WebSocket streaming,
proposal approval/rejection, and database persistence validation.
"""

import pytest
from httpx import AsyncClient
import time
import json
import asyncio
from websockets import connect as ws_connect


@pytest.mark.asyncio
async def test_complete_refinement_workflow(
    test_client: AsyncClient,
    test_db,
    jwt_manager,
    mock_deepagents_server
):
    """Test complete refinement workflow from creation to database persistence."""
    # Create test user
    user_email = f"refinement-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    # Step 1: Create workflow
    workflow_data = {
        "name": "Refinement Test Workflow",
        "description": "Workflow for testing refinements",
        "specification": {
            "nodes": [
                {"id": "start", "type": "start", "data": {"label": "Start"}},
                {"id": "end", "type": "end", "data": {"label": "End"}}
            ],
            "edges": [
                {"id": "start-to-end", "source": "start", "target": "end"}
            ]
        }
    }
    
    response = await test_client.post(
        "/api/workflows",
        json=workflow_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 201
    workflow_id = response.json()["id"]
    
    # Step 2: Create refinement
    refinement_data = {
        "instructions": "Add error handling to the workflow",
        "context": "The current workflow lacks proper error handling mechanisms"
    }
    
    response = await test_client.post(
        f"/api/workflows/{workflow_id}/refinements",
        json=refinement_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 202
    refinement_response = response.json()
    thread_id = refinement_response["thread_id"]
    assert thread_id is not None
    
    # Step 3: Wait for processing to complete
    await asyncio.sleep(0.5)
    
    # Step 4: Validate that data was persisted to the database
    conn = test_db.connect()
    with conn.cursor() as cur:
        cur.execute(
            "SELECT id, generated_files, status FROM proposals WHERE thread_id = %s",
            (thread_id,)
        )
        result = cur.fetchone()
    
    assert result is not None, "Proposal should be created in database"
    proposal_id = result["id"]
    generated_files = result["generated_files"]
    status = result["status"]
    
    # Verify the proposal status
    assert status == "completed"
    
    # Verify the generated files match our mock data
    db_files = json.loads(generated_files) if isinstance(generated_files, str) else generated_files
    
    # Check that key files from our mock data are present
    assert "/definition.json" in db_files
    assert "/THE_SPEC/requirements.md" in db_files
    assert "/THE_CAST/GreetingAgent.md" in db_files
    
    # Verify the definition.json content structure
    definition_file = db_files["/definition.json"]
    assert "content" in definition_file
    content = definition_file["content"]
    
    # Parse the JSON content to verify it's valid
    if isinstance(content, list):
        json_content = "".join(content)
    else:
        json_content = content
    
    definition_json = json.loads(json_content)
    assert definition_json["name"] == "GreetingWorkflow"
    assert definition_json["version"] == "1.0.0"


@pytest.mark.asyncio
async def test_websocket_streaming(
    test_client: AsyncClient,
    test_db,
    jwt_manager,
    mock_deepagents_server
):
    """Test WebSocket streaming of refinement progress."""
    # Create test user
    user_email = f"websocket-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    # Get the WebSocket URL
    ws_url = f"ws://localhost:8000/api/ws/refinements/test-thread-123"
    
    # Connect to WebSocket with authentication
    async with ws_connect(
        ws_url,
        extra_headers={"Authorization": f"Bearer {token}"}
    ) as websocket:
        # Read WebSocket messages
        messages = []
        
        try:
            while True:
                message = await asyncio.wait_for(websocket.recv(), timeout=5.0)
                msg_data = json.loads(message)
                messages.append(msg_data)
                
                # Check for end event
                if msg_data.get("event_type") == "end":
                    break
        except asyncio.TimeoutError:
            pass
        
        # Verify we received messages
        assert len(messages) > 0
        
        # Verify message structure
        for msg in messages:
            assert "event_type" in msg
            assert "data" in msg
        
        # Should have at least one state update and one end event
        has_state_update = any(msg["event_type"] == "on_state_update" for msg in messages)
        has_end_event = any(msg["event_type"] == "end" for msg in messages)
        
        assert has_state_update, "Should have received state update event"
        assert has_end_event, "Should have received end event"


@pytest.mark.asyncio
async def test_proposal_approval(test_client: AsyncClient, test_db, jwt_manager):
    """Test proposal approval endpoint."""
    user_email = f"approval-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    # Test approving a non-existent proposal
    response = await test_client.post(
        "/api/refinements/non-existent-proposal/approve",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    # Should return 404 for non-existent proposal
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_proposal_rejection(test_client: AsyncClient, test_db, jwt_manager):
    """Test proposal rejection endpoint."""
    user_email = f"rejection-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    # Test rejecting a non-existent proposal
    response = await test_client.post(
        "/api/refinements/non-existent-proposal/reject",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    # Should return 404 for non-existent proposal
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_refinement_validation(test_client: AsyncClient, test_db, jwt_manager):
    """Test refinement request validation."""
    user_email = f"validation-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    workflow_id = test_db.create_test_workflow(
        user_id,
        "Validation Test Workflow",
        "For testing refinement validation"
    )
    
    # Test invalid refinement (missing instructions)
    invalid_data = {
        "context": "Missing instructions"
    }
    
    response = await test_client.post(
        f"/api/workflows/{workflow_id}/refinements",
        json=invalid_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 400
    
    # Test refinement on non-existent workflow
    valid_data = {
        "instructions": "Valid instructions",
        "context": "Valid context"
    }
    
    response = await test_client.post(
        "/api/workflows/non-existent-workflow/refinements",
        json=valid_data,
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 404


@pytest.mark.asyncio
async def test_spec_engine_invoke(mock_deepagents_server):
    """Test direct integration with mock Spec Engine invoke endpoint."""
    import httpx
    
    spec_engine_url = mock_deepagents_server
    
    invoke_data = {
        "job_id": "test-job-123",
        "trace_id": "test-trace-123",
        "agent_definition": {
            "nodes": [],
            "edges": []
        },
        "input_payload": {
            "instructions": "Test refinement",
            "context": "Test context"
        }
    }
    
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{spec_engine_url}/invoke",
            json=invoke_data
        )
    
    assert response.status_code == 200
    data = response.json()
    assert data["thread_id"] == "test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574"
    assert data["status"] == "started"


@pytest.mark.asyncio
async def test_spec_engine_state(mock_deepagents_server):
    """Test Spec Engine state retrieval."""
    import httpx
    
    spec_engine_url = mock_deepagents_server
    thread_id = "test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574"
    
    async with httpx.AsyncClient() as client:
        response = await client.get(f"{spec_engine_url}/state/{thread_id}")
    
    assert response.status_code == 200
    data = response.json()
    assert data["thread_id"] == thread_id
    assert data["status"] == "completed"
    assert "result" in data
    assert "generated_files" in data
    
    # Verify generated files structure
    generated_files = data["generated_files"]
    assert "/definition.json" in generated_files
    assert "/THE_SPEC/requirements.md" in generated_files
    assert "/THE_CAST/GreetingAgent.md" in generated_files


@pytest.mark.asyncio
async def test_spec_engine_websocket_streaming(mock_deepagents_server):
    """Test WebSocket streaming from Spec Engine."""
    ws_url = mock_deepagents_server.replace("http://", "ws://")
    ws_url = f"{ws_url}/stream/test-job-63e8fa1b-60cb-454b-8815-96f1b4cb4574"
    
    async with ws_connect(ws_url) as websocket:
        # Read WebSocket messages
        messages = []
        
        try:
            while True:
                message = await asyncio.wait_for(websocket.recv(), timeout=5.0)
                msg_data = json.loads(message)
                messages.append(msg_data)
                
                # Check for end event
                if msg_data.get("event_type") == "end":
                    break
        except asyncio.TimeoutError:
            pass
        
        # Verify we received messages
        assert len(messages) > 0
        
        # Verify message structure
        for msg in messages:
            assert "event_type" in msg
            assert "data" in msg
        
        # Should have at least one state update and one end event
        has_state_update = any(msg["event_type"] == "on_state_update" for msg in messages)
        has_end_event = any(msg["event_type"] == "end" for msg in messages)
        
        assert has_state_update, "Should have received state update event"
        assert has_end_event, "Should have received end event"
