"""
Reusable test fixtures for refinement integration tests.

Provides standardized test setup, data, and context management
to ensure consistent test environments across all refinement tests.
"""

import pytest
import time
import uuid
import psycopg
from psycopg.rows import dict_row
from typing import Dict, Any, Tuple, NamedTuple

from api.dependencies import get_database_url
from services.workflow_service import WorkflowService
from services.proposal_service import ProposalService
from services.draft_service import DraftService
from core.jwt_manager import JWTManager


class RefinementTestContext(NamedTuple):
    """Complete test context for refinement tests."""
    user_id: str
    token: str
    database_url: str
    workflow_service: WorkflowService
    proposal_service: ProposalService
    draft_service: DraftService


@pytest.fixture
async def refinement_test_context(jwt_manager: JWTManager, mock_deepagents_server) -> RefinementTestContext:
    """
    Complete test context with user, services, and token.
    
    Creates a test user and initializes all production services
    using the same dependency injection as production code.
    """
    # Set the mock deepagents server URL for this test
    import os
    os.environ["DEEPAGENTS_RUNTIME_URL"] = mock_deepagents_server
    print(f"[DEBUG] Set DEEPAGENTS_RUNTIME_URL to: {mock_deepagents_server}")
    
    # Use production dependency injection
    database_url = get_database_url()
    workflow_service = WorkflowService(database_url)
    proposal_service = ProposalService(database_url)
    draft_service = DraftService(database_url)
    
    # Create test user with unique email
    user_email = f"refinement-test-{int(time.time() * 1000000)}@example.com"
    
    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO users (name, email, hashed_password, created_at, updated_at)
                VALUES (%s, %s, %s, NOW(), NOW())
                RETURNING id
                """,
                ("Refinement Test User", user_email, "hashed-password")
            )
            user_result = cur.fetchone()
            user_id = str(user_result["id"])
    
    # Generate JWT token
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    return RefinementTestContext(
        user_id=user_id,
        token=token,
        database_url=database_url,
        workflow_service=workflow_service,
        proposal_service=proposal_service,
        draft_service=draft_service
    )


@pytest.fixture
def sample_initial_draft_content() -> Dict[str, str]:
    """Standard initial draft content for tests - matches real deepagents workflow structure."""
    return {
        "/user_request.md": "Create a simple hello world agent that greets users",
        "/orchestrator_plan.md": "# Initial Plan\nBasic orchestrator plan for hello world agent.",
        "/guardrail_assessment.md": "# Initial Guardrail Assessment\nBasic safety assessment.",
        "/impact_assessment.md": "# Initial Impact Assessment\nBasic impact analysis.",
        "/THE_SPEC/constitution.md": "# Initial Constitution\nBasic constitutional principles.",
        "/THE_SPEC/requirements.md": "# Initial Requirements\nBasic input schema requirements.",
        "/THE_SPEC/plan.md": "# Initial Plan\nBasic execution flow.",
        "/THE_CAST/OrchestratorAgent.md": "# Initial Orchestrator\nBasic orchestrator agent.",
        "/THE_CAST/GreetingAgent.md": "# Initial Greeting Agent\nBasic greeting agent.",
        "/definition.json": '{"name": "InitialWorkflow", "version": "0.1.0"}'
    }


@pytest.fixture
def sample_enhanced_draft_content() -> Dict[str, str]:
    """Enhanced draft content for approval tests - loaded from real test data."""
    # Load from actual test data
    from pathlib import Path
    import json
    
    testdata_dir = Path(__file__).parent.parent.parent.parent / "testdata"
    state_path = testdata_dir / "thread_state.json"
    
    with open(state_path, 'r') as f:
        state_data = json.load(f)
    
    # Extract content from generated files
    generated_files = state_data.get("generated_files", {})
    content = {}
    
    for file_path, file_data in generated_files.items():
        if isinstance(file_data, dict) and "content" in file_data:
            # Join content lines if it's a list
            if isinstance(file_data["content"], list):
                content[file_path] = "\n".join(file_data["content"])
            else:
                content[file_path] = file_data["content"]
        else:
            content[file_path] = str(file_data)
    
    return content


@pytest.fixture
def sample_generated_files_approved() -> Dict[str, Any]:
    """Standard generated files for approved proposal completion - loaded from real test data."""
    from pathlib import Path
    import json
    
    testdata_dir = Path(__file__).parent.parent.parent.parent / "testdata"
    state_path = testdata_dir / "thread_state.json"
    
    with open(state_path, 'r') as f:
        state_data = json.load(f)
    
    return state_data.get("generated_files", {})


@pytest.fixture
def sample_generated_files_rejected() -> Dict[str, Any]:
    """Standard generated files for rejected proposal completion - loaded from real test data."""
    from pathlib import Path
    import json
    
    testdata_dir = Path(__file__).parent.parent.parent.parent / "testdata"
    state_path = testdata_dir / "rejection_state.json"
    
    with open(state_path, 'r') as f:
        state_data = json.load(f)
    
    return state_data.get("generated_files", {})


@pytest.fixture
def sample_refinement_request_approved() -> Dict[str, Any]:
    """Standard refinement request for approval tests."""
    return {
        "instructions": "Add error handling and logging to the main function",
        "context_file_path": "/main.py",
        "context_selection": "Improve code quality and debugging capabilities"
    }


@pytest.fixture
def sample_refinement_request_rejected() -> Dict[str, Any]:
    """Standard refinement request for rejection tests."""
    return {
        "instructions": "Add database integration with SQLAlchemy",
        "context_file_path": "/config.json", 
        "context_selection": "Need to persist data in a database"
    }