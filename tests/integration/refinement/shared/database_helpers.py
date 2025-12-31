"""
Database helper utilities for refinement integration tests.

Provides reusable database operations for workflow, draft, and proposal
management to ensure consistent data setup and validation patterns.
"""

import uuid
import json
import psycopg
from psycopg.rows import dict_row
from typing import Dict, Any, Tuple, Optional


async def create_test_workflow_with_draft(
    user_id: str,
    workflow_name: str,
    draft_content: Dict[str, str],
    database_url: str,
    draft_name: Optional[str] = None,
    draft_description: Optional[str] = None
) -> Tuple[str, str]:
    """
    Create workflow and initial draft in single operation.
    
    Args:
        user_id: User ID who owns the workflow
        workflow_name: Name of the workflow
        draft_content: Dictionary of file_path -> content
        database_url: Database connection URL
        draft_name: Optional draft name (defaults to workflow name + " Draft")
        draft_description: Optional draft description
        
    Returns:
        Tuple of (workflow_id, draft_id)
    """
    if draft_name is None:
        draft_name = f"{workflow_name} Draft"
    if draft_description is None:
        draft_description = f"Draft for {workflow_name}"
    
    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            # Create workflow
            cur.execute(
                """
                INSERT INTO workflows (name, description, created_by_user_id, created_at, updated_at)
                VALUES (%s, %s, %s, NOW(), NOW())
                RETURNING id
                """,
                (workflow_name, f"Testing workflow: {workflow_name}", user_id)
            )
            workflow_result = cur.fetchone()
            workflow_id = str(workflow_result["id"])
            
            # Create draft
            draft_id = str(uuid.uuid4())
            cur.execute(
                """
                INSERT INTO drafts (id, workflow_id, name, description, created_by_user_id, created_at, updated_at)
                VALUES (%s, %s, %s, %s, %s, NOW(), NOW())
                RETURNING id
                """,
                (draft_id, workflow_id, draft_name, draft_description, user_id)
            )
            
            # Insert draft files
            for file_path, content in draft_content.items():
                file_type = "json" if file_path.endswith(".json") else "markdown"
                cur.execute(
                    """
                    INSERT INTO draft_specification_files (draft_id, file_path, content, file_type, created_at)
                    VALUES (%s, %s, %s, %s, NOW())
                    """,
                    (draft_id, file_path, content, file_type)
                )
            
            conn.commit()
    
    return workflow_id, draft_id


async def get_draft_content_by_workflow(workflow_id: str, database_url: str) -> Dict[str, str]:
    """
    Retrieve draft content as dictionary by workflow_id.
    
    Args:
        workflow_id: Workflow ID
        database_url: Database connection URL
        
    Returns:
        Dictionary of file_path -> content
    """
    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT dsf.file_path, dsf.content 
                FROM draft_specification_files dsf
                JOIN drafts d ON dsf.draft_id = d.id
                WHERE d.workflow_id = %s
                ORDER BY dsf.file_path
                """,
                (workflow_id,)
            )
            return {row["file_path"]: row["content"] for row in cur.fetchall()}


async def get_proposal_by_id(proposal_id: str, database_url: str) -> Optional[Dict[str, Any]]:
    """
    Get proposal with all fields via direct database query.
    
    Args:
        proposal_id: Proposal ID
        database_url: Database connection URL
        
    Returns:
        Proposal dictionary or None if not found
    """
    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                SELECT id, draft_id, thread_id, user_prompt, context_file_path,
                       context_selection, status, generated_files, resolution,
                       created_at, completed_at, resolved_at, created_by_user_id, resolved_by_user_id
                FROM proposals
                WHERE id = %s
                """,
                (proposal_id,)
            )
            result = cur.fetchone()
            if result:
                result = dict(result)
                # Convert UUID objects to strings
                for key, value in result.items():
                    if hasattr(value, 'hex'):
                        result[key] = str(value)
                return result
            return None


async def verify_proposal_resolution(
    proposal_id: str, 
    expected_resolution: str, 
    database_url: str
) -> bool:
    """
    Verify proposal resolution state and timestamps.
    
    Args:
        proposal_id: Proposal ID
        expected_resolution: Expected resolution ("approved" or "rejected")
        database_url: Database connection URL
        
    Returns:
        True if proposal has expected resolution and proper timestamps
    """
    proposal = await get_proposal_by_id(proposal_id, database_url)
    if not proposal:
        return False
    
    return (
        proposal["status"] == "resolved" and
        proposal["resolution"] == expected_resolution and
        proposal["resolved_at"] is not None and
        proposal["resolved_by_user_id"] is not None
    )


async def get_proposal_generated_files(proposal_id: str, database_url: str) -> Optional[Dict[str, Any]]:
    """
    Get proposal generated files with proper JSON parsing.
    
    Args:
        proposal_id: Proposal ID
        database_url: Database connection URL
        
    Returns:
        Generated files dictionary or None if not found
    """
    with psycopg.connect(database_url, row_factory=dict_row) as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT generated_files FROM proposals WHERE id = %s",
                (proposal_id,)
            )
            result = cur.fetchone()
            if result and result["generated_files"]:
                generated_files = result["generated_files"]
                if isinstance(generated_files, str):
                    return json.loads(generated_files)
                return generated_files
            return None


async def verify_context_persistence(
    proposal_id: str,
    expected_context_file_path: Optional[str],
    expected_context_selection: Optional[str],
    database_url: str
) -> bool:
    """
    Verify that context_file_path and context_selection are correctly persisted.
    
    Args:
        proposal_id: Proposal ID
        expected_context_file_path: Expected context file path
        expected_context_selection: Expected context selection
        database_url: Database connection URL
        
    Returns:
        True if context fields match expected values
    """
    proposal = await get_proposal_by_id(proposal_id, database_url)
    if not proposal:
        return False
    
    return (
        proposal["context_file_path"] == expected_context_file_path and
        proposal["context_selection"] == expected_context_selection
    )