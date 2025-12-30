"""
Authentication integration tests for IDE Orchestrator.

Tests authentication flow integration without duplicating JWT validation logic.
"""

import pytest
from httpx import AsyncClient
import time


@pytest.mark.asyncio
async def test_authentication_flow_integration(test_client: AsyncClient, test_db, jwt_manager):
    """Test complete authentication flow without duplicating JWT validation logic."""
    # Create test user
    user_email = f"auth-flow-{int(time.time() * 1000000)}@example.com"
    user_id = test_db.create_test_user(user_email, "hashed-password")
    
    # Generate token for authentication flow testing
    token = await jwt_manager.generate_token(user_id, user_email, [], 24 * 3600)
    
    # Test that authentication middleware properly integrates with the application
    response = await test_client.get(
        "/api/protected",
        headers={"Authorization": f"Bearer {token}"}
    )
    
    assert response.status_code == 200
    data = response.json()
    assert data["user_id"] == user_id
    assert data["email"] == user_email
    assert data["message"] == "Access granted"


@pytest.mark.asyncio
async def test_public_endpoints_no_auth_required(test_client: AsyncClient):
    """Health endpoint should be accessible without authentication."""
    response = await test_client.get("/api/health")
    
    assert response.status_code == 200
    data = response.json()
    assert data["status"] == "healthy"
