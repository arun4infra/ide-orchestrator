"""
Pytest configuration and fixtures for IDE Orchestrator integration tests.
"""

import pytest
import pytest_asyncio
from httpx import AsyncClient
import os
from pathlib import Path

# Import test helpers
from tests.helpers.database import TestDatabase
from tests.integration.cluster_config import setup_in_cluster_environment
from tests.mock.deepagents_mock import create_mock_server


@pytest.fixture(scope="session")
def cluster_config():
    """Setup in-cluster environment configuration."""
    config = setup_in_cluster_environment()
    print(f"\nUsing infrastructure - Database: {config.database_url}, SpecEngine: {config.spec_engine_url}")
    return config


@pytest.fixture(scope="function")
def test_db():
    """
    Provide test database instance with automatic cleanup.
    
    Uses transaction-based isolation for test data.
    """
    db = TestDatabase()
    yield db
    db.close()


@pytest_asyncio.fixture(scope="function")
async def test_client(app):
    """
    Provide async HTTP test client.
    
    Args:
        app: FastAPI application instance
        
    Yields:
        AsyncClient for making HTTP requests
    """
    async with AsyncClient(app=app, base_url="http://test") as client:
        yield client


@pytest.fixture(scope="session")
def mock_deepagents_server():
    """
    Provide mock deepagents-runtime server URL.
    
    The mock server should be started separately or as part of test setup.
    This fixture just returns the URL.
    """
    # Check for mock URL override
    mock_url = os.getenv("MOCK_SPEC_ENGINE_URL")
    if mock_url:
        return mock_url
    
    # Default mock server URL
    return "http://localhost:8001"


@pytest.fixture(scope="function")
def jwt_manager():
    """
    Provide JWT manager instance for token generation/validation.
    
    This fixture will be implemented once the auth module is created.
    """
    # TODO: Import and return actual JWTManager instance
    # from api.auth import JWTManager
    # return JWTManager()
    raise NotImplementedError("JWTManager not yet implemented")


@pytest.fixture(scope="function")
def app():
    """
    Provide FastAPI application instance.
    
    This fixture will be implemented once the API module is created.
    """
    # TODO: Import and return actual FastAPI app
    # from api.main import app
    # return app
    raise NotImplementedError("FastAPI app not yet implemented")


# Pytest configuration
def pytest_configure(config):
    """Configure pytest with custom markers."""
    config.addinivalue_line(
        "markers", "asyncio: mark test as async"
    )
    config.addinivalue_line(
        "markers", "integration: mark test as integration test"
    )
