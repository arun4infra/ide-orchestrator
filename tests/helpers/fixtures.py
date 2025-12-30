"""
Test fixtures and data builders for IDE Orchestrator tests.
"""

import json
from typing import Dict, Any, List


# Default test fixtures
DEFAULT_TEST_USER = {
    "email": "test@example.com",
    "password": "test-password-123"
}

DEFAULT_TEST_WORKFLOW = {
    "name": "Test Workflow",
    "description": "A test workflow for integration testing",
    "specification": {
        "nodes": [
            {
                "id": "start",
                "type": "start",
                "data": {"label": "Start Node"}
            },
            {
                "id": "end",
                "type": "end",
                "data": {"label": "End Node"}
            }
        ],
        "edges": [
            {
                "id": "start-to-end",
                "source": "start",
                "target": "end"
            }
        ]
    }
}

DEFAULT_TEST_REFINEMENT = {
    "instructions": "Add a processing node between start and end",
    "context": "This is a simple workflow that needs a processing step"
}


def create_single_agent_workflow(agent_name: str, prompt: str) -> Dict[str, Any]:
    """Create a single-agent workflow specification."""
    return {
        "type": "single-agent",
        "agent": {
            "name": agent_name,
            "prompt": prompt,
            "tools": []
        },
        "nodes": [
            {
                "id": "agent",
                "type": "agent",
                "data": {
                    "agent_name": agent_name,
                    "prompt": prompt
                }
            }
        ],
        "edges": []
    }


def create_multi_agent_workflow(agents: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Create a multi-agent workflow specification."""
    nodes = []
    edges = []
    
    for i, agent in enumerate(agents):
        node_id = agent["name"]
        nodes.append({
            "id": node_id,
            "type": "agent",
            "data": agent
        })
        
        # Connect agents in sequence
        if i > 0:
            prev_node_id = agents[i-1]["name"]
            edges.append({
                "id": f"{prev_node_id}-to-{node_id}",
                "source": prev_node_id,
                "target": node_id
            })
    
    return {
        "type": "multi-agent",
        "agents": agents,
        "nodes": nodes,
        "edges": edges
    }


def create_complex_workflow_spec() -> Dict[str, Any]:
    """Create a complex workflow for testing."""
    return {
        "type": "complex-workflow",
        "nodes": [
            {
                "id": "input",
                "type": "input",
                "data": {
                    "label": "User Input",
                    "schema": {
                        "type": "object",
                        "properties": {
                            "query": {"type": "string"}
                        }
                    }
                }
            },
            {
                "id": "analyzer",
                "type": "agent",
                "data": {
                    "agent_name": "Query Analyzer",
                    "prompt": "Analyze the user query and extract key information",
                    "tools": ["text_analysis", "entity_extraction"]
                }
            },
            {
                "id": "processor",
                "type": "agent",
                "data": {
                    "agent_name": "Data Processor",
                    "prompt": "Process the analyzed data and generate insights",
                    "tools": ["data_processing", "insight_generation"]
                }
            },
            {
                "id": "output",
                "type": "output",
                "data": {
                    "label": "Final Output",
                    "format": "json"
                }
            }
        ],
        "edges": [
            {"id": "input-to-analyzer", "source": "input", "target": "analyzer"},
            {"id": "analyzer-to-processor", "source": "analyzer", "target": "processor"},
            {"id": "processor-to-output", "source": "processor", "target": "output"}
        ]
    }


def create_test_login_request(email: str, password: str) -> Dict[str, str]:
    """Create a login request payload."""
    return {
        "email": email,
        "password": password
    }


def create_test_workflow_request(
    name: str,
    description: str,
    spec: Dict[str, Any]
) -> Dict[str, Any]:
    """Create a workflow creation request payload."""
    return {
        "name": name,
        "description": description,
        "specification": spec
    }


def create_test_refinement_request(instructions: str, context: str) -> Dict[str, str]:
    """Create a refinement request payload."""
    return {
        "instructions": instructions,
        "context": context
    }


def mock_spec_engine_response(thread_id: str, status: str) -> Dict[str, Any]:
    """Create a mock response from Spec Engine."""
    response = {
        "thread_id": thread_id,
        "status": status
    }
    
    if status == "completed":
        response["result"] = {
            "specification": create_single_agent_workflow(
                "Enhanced Agent",
                "You are an enhanced AI agent with improved capabilities"
            ),
            "changes": [
                "Added processing node",
                "Enhanced agent prompt",
                "Improved error handling"
            ]
        }
    
    return response


def to_json(obj: Any) -> str:
    """Convert object to JSON string."""
    return json.dumps(obj)


def from_json(json_str: str) -> Any:
    """Parse JSON string to object."""
    return json.loads(json_str)
