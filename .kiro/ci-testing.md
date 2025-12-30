Based on our discussion, here is the complete, finalized set of requirements for refactoring the `ide-orchestrator` integration tests.

### 1. Core Objective
Decouple the `ide-orchestrator` integration tests from the live `deepagents-runtime` service while maintaining the "In-Cluster" testing architecture. The tests must validate that `ide-orchestrator` correctly processes realistic data and persists it to its own database.

### 2. Mocking Strategy
*   **Source of Truth:** Mocks must use actual JSON payloads (`all_events.json`) exported from the `deepagents-runtime` repository, not hardcoded/assumed string values.
*   **Hosting:** The mock server will run **inside the test process** using Go's `httptest` package (listening on `localhost`), rather than deploying a separate mock pod.
*   **Protocol Fidelity:**
    *   **HTTP:** Must handle `POST /invoke` (returning a thread ID) and `GET /state/{id}`.
    *   **WebSocket:** Must handle the upgrade at `GET /stream/{id}` and stream the events from `all_events.json` sequentially (no complex timing delays required).

### 3. Data Requirements
You will create a `tests/testdata/` directory containing:
*   **`all_events.json`**: The exact event stream captured from a real execution.
*   **`thread_state.json`**: A constructed JSON response representing the final state (matching the file data found in `all_events.json`).

### 4. Validation Scope
*   **Persistence is Key:** The test must query the **real** `ide-orchestrator` PostgreSQL database (specifically the `proposals` table) after the mock workflow completes.
*   **Success Criteria:** The data in the database (specifically `generated_files`) must match the data served by the mock.

### 5. Infrastructure & Configuration Changes
*   **Environment Variables:** The `ClusterConfig` must allow overriding `SPEC_ENGINE_URL` to point to the local `httptest` server, bypassing the default Kubernetes DNS logic.
*   **Docker Image:** The `Dockerfile.test` must copy the `tests/testdata/` directory so the test binary can read the JSON files at runtime.
*   **Platform Dependencies:** The `deepagents-runtime` service must be removed from the `external` dependencies list in `ci/config.yaml`.

### 6. Cleanup & Simplification (Bloat Removal)
*   **Delete Files:** `tests/integration/websocket_proxy_integration_test.go` (redundant once refinement test is fixed).
*   **Delete Code:**
    *   All inline/manual mock structs inside test files (replace with shared `tests/helpers/deepagents_mock.go`).
    *   `scripts/ci/setup-dependencies.sh` (deployment logic for the real service).
    *   Connectivity checks (e.g., `nc -z deepagents-runtime...`) in `run-test-job.sh`.