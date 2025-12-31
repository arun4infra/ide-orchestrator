"""
Refinement Rejected Lifecycle Integration Test - "Undo Path"

Focus: No data leakage and draft state preservation
Tests the complete refinement rejection flow with emphasis on:
- Draft content remains completely unchanged after rejection
- No data leakage from proposal to draft
- State machine transitions (processing → completed → resolved)
- Runtime cleanup verification
- Isolation between proposal and draft data
"""

import pytest
from httpx import AsyncClient

from .shared.fixtures import (
    refinement_test_context,
    sample_initial_draft_content,
    sample_generated_files_rejected,
    sample_refinement_request_rejected
)
from .shared.database_helpers import (
    create_test_workflow_with_draft,
    get_draft_content_by_workflow
)
from .shared.mock_helpers import (
    create_mock_deepagents_client,
    patch_deepagents_client,
    simulate_proposal_completion_via_stream,
    setup_cleanup_tracking
)
from .shared.assertions import (
    assert_refinement_response_valid,
    assert_proposal_state,
    assert_draft_content_unchanged,
    assert_runtime_cleanup_called,
    assert_context_metadata_persisted,
    assert_rejection_response_valid
)


@pytest.mark.asyncio
async def test_refinement_rejected_lifecycle(
    test_client: AsyncClient,
    refinement_test_context,
    sample_initial_draft_content,
    sample_generated_files_rejected,
    sample_refinement_request_rejected
):
    """
    Test complete refinement rejection lifecycle with data isolation validation.
    
    This test validates the "Undo Path" where:
    1. User initiates refinement request
    2. WebSocket streaming processes and updates proposal
    3. User rejects the proposal
    4. Draft content remains completely unchanged
    5. System performs cleanup and marks proposal as resolved
    
    Focus: No data leakage and draft state preservation
    """
    user_id, token, database_url, workflow_service, proposal_service, draft_service = refinement_test_context
    
    # Step 1: Setup workflow and draft using shared utility
    workflow_id, draft_id = await create_test_workflow_with_draft(
        user_id=user_id,
        workflow_name="Rejection Test Workflow",
        draft_content=sample_initial_draft_content,
        database_url=database_url,
        draft_name="Rejected Test Draft",
        draft_description="Draft for rejected lifecycle testing"
    )
    
    # Step 2: Capture baseline draft content for comparison
    baseline_draft_content = await get_draft_content_by_workflow(workflow_id, database_url)
    assert baseline_draft_content == sample_initial_draft_content, "Baseline content mismatch"
    
    # Step 3: Setup cleanup tracking to verify Requirement 4.5
    with setup_cleanup_tracking():
        # Step 4: Trigger refinement request using shared mock helper
        mock_client = create_mock_deepagents_client("rejected")
        
        with patch_deepagents_client(mock_client):
            response = await test_client.post(
                f"/api/workflows/{workflow_id}/refinements",
                json=sample_refinement_request_rejected,
                headers={"Authorization": f"Bearer {token}"}
            )
        
        # Validate: Response contains thread_id and proposal_id; status is processing
        refinement_data = assert_refinement_response_valid(response, expected_status=202)
        proposal_id = refinement_data["proposal_id"]
        thread_id = refinement_data["thread_id"]
        
        # Step 5: Verify initial proposal state
        await assert_proposal_state(
            proposal_id=proposal_id,
            expected_status="processing",
            database_url=database_url,
            has_files=False
        )
        
        # Step 6: Verify context metadata persistence (Requirement 7.1)
        await assert_context_metadata_persisted(
            proposal_id=proposal_id,
            expected_context_file_path=sample_refinement_request_rejected["context_file_path"],
            expected_context_selection=sample_refinement_request_rejected["context_selection"],
            database_url=database_url
        )
        
        # Step 7: Simulate WebSocket streaming with different content
        # This tests that proposal gets updated but draft remains isolated
        await simulate_proposal_completion_via_stream(
            proposal_service=proposal_service,
            proposal_id=proposal_id,
            thread_id=thread_id,
            generated_files=sample_generated_files_rejected
        )
        
        # Step 8: Validate proposal completion state (has different content)
        await assert_proposal_state(
            proposal_id=proposal_id,
            expected_status="completed",
            database_url=database_url,
            has_files=True
        )
        
        # Step 9: Critical - Verify draft content is still unchanged
        await assert_draft_content_unchanged(
            workflow_id=workflow_id,
            baseline_content=baseline_draft_content,
            database_url=database_url
        )
        
        # Step 10: Reject the proposal through production API
        response = await test_client.post(
            f"/api/refinements/{proposal_id}/reject",
            headers={"Authorization": f"Bearer {token}"}
        )
        
        # Validate: Rejection response structure
        rejection_data = assert_rejection_response_valid(response)
        assert rejection_data["proposal_id"] == proposal_id
        
        # Step 11: Validate final proposal resolution state
        await assert_proposal_state(
            proposal_id=proposal_id,
            expected_status="resolved",
            database_url=database_url,
            has_files=True,
            expected_resolution="rejected"
        )
        
        # Step 12: Critical - Verify draft content is STILL unchanged (no data leakage)
        await assert_draft_content_unchanged(
            workflow_id=workflow_id,
            baseline_content=baseline_draft_content,
            database_url=database_url
        )
        
        # Step 13: Verify runtime cleanup was called (Requirement 4.5)
        assert_runtime_cleanup_called(thread_id)


@pytest.mark.asyncio
async def test_refinement_rejected_data_isolation(
    test_client: AsyncClient,
    refinement_test_context,
    sample_initial_draft_content,
    sample_generated_files_rejected,
    sample_refinement_request_rejected
):
    """
    Test data isolation between proposal and draft during rejection.
    
    Focus: Validates that proposal content never leaks into draft
    - Proposal can have completely different content
    - Draft remains isolated throughout the entire process
    - Multiple rejection cycles don't cause data corruption
    """
    user_id, token, database_url, workflow_service, proposal_service, draft_service = refinement_test_context
    
    # Setup workflow and draft
    workflow_id, draft_id = await create_test_workflow_with_draft(
        user_id=user_id,
        workflow_name="Data Isolation Test Workflow",
        draft_content=sample_initial_draft_content,
        database_url=database_url
    )
    
    # Capture original content
    original_content = await get_draft_content_by_workflow(workflow_id, database_url)
    
    # Create first proposal with different content
    mock_client_1 = create_mock_deepagents_client("isolation_1")
    
    with patch_deepagents_client(mock_client_1):
        response = await test_client.post(
            f"/api/workflows/{workflow_id}/refinements",
            json=sample_refinement_request_rejected,
            headers={"Authorization": f"Bearer {token}"}
        )
    
    refinement_data_1 = assert_refinement_response_valid(response)
    proposal_id_1 = refinement_data_1["proposal_id"]
    thread_id_1 = refinement_data_1["thread_id"]
    
    # Complete first proposal with radically different content
    different_content_1 = {
        "main.py": {
            "content": "# Completely different content 1\nprint('This should never appear in draft')",
            "type": "markdown"
        },
        "config.json": {
            "content": '{"completely": "different", "version": "999.0"}',
            "type": "json"
        }
    }
    
    await simulate_proposal_completion_via_stream(
        proposal_service=proposal_service,
        proposal_id=proposal_id_1,
        thread_id=thread_id_1,
        generated_files=different_content_1
    )
    
    # Verify draft is still unchanged
    await assert_draft_content_unchanged(
        workflow_id=workflow_id,
        baseline_content=original_content,
        database_url=database_url
    )
    
    # Reject first proposal
    await test_client.post(
        f"/api/refinements/{proposal_id_1}/reject",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    # Verify draft is STILL unchanged after rejection
    await assert_draft_content_unchanged(
        workflow_id=workflow_id,
        baseline_content=original_content,
        database_url=database_url
    )
    
    # Create second proposal with even more different content
    mock_client_2 = create_mock_deepagents_client("isolation_2")
    
    with patch_deepagents_client(mock_client_2):
        response = await test_client.post(
            f"/api/workflows/{workflow_id}/refinements",
            json=sample_refinement_request_rejected,
            headers={"Authorization": f"Bearer {token}"}
        )
    
    refinement_data_2 = assert_refinement_response_valid(response)
    proposal_id_2 = refinement_data_2["proposal_id"]
    thread_id_2 = refinement_data_2["thread_id"]
    
    # Complete second proposal with even more different content
    different_content_2 = {
        "main.py": {
            "content": "# Even more different content 2\nprint('This should also never appear in draft')",
            "type": "markdown"
        },
        "config.json": {
            "content": '{"even_more": "different", "version": "888.0"}',
            "type": "json"
        },
        "new_file.py": {
            "content": "# This new file should never appear in draft",
            "type": "markdown"
        }
    }
    
    await simulate_proposal_completion_via_stream(
        proposal_service=proposal_service,
        proposal_id=proposal_id_2,
        thread_id=thread_id_2,
        generated_files=different_content_2
    )
    
    # Verify draft is STILL unchanged after second proposal
    await assert_draft_content_unchanged(
        workflow_id=workflow_id,
        baseline_content=original_content,
        database_url=database_url
    )
    
    # Reject second proposal
    await test_client.post(
        f"/api/refinements/{proposal_id_2}/reject",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    # Final verification: draft content is completely unchanged
    await assert_draft_content_unchanged(
        workflow_id=workflow_id,
        baseline_content=original_content,
        database_url=database_url
    )
    
    # Verify both proposals are resolved as rejected
    await assert_proposal_state(
        proposal_id=proposal_id_1,
        expected_status="resolved",
        database_url=database_url,
        expected_resolution="rejected"
    )
    
    await assert_proposal_state(
        proposal_id=proposal_id_2,
        expected_status="resolved",
        database_url=database_url,
        expected_resolution="rejected"
    )