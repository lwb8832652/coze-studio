# MCP Stdio Secret Projection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Project stdio MCP runtime credentials from internal server auth into
process env only at execution time.

**Architecture:** Extend the stdio transport parser with an `auth_env` mapping
and resolve mapped values from `MCPToolServer.Auth` before policy validation.
Keep projection inside the transport boundary so existing static policy,
sandbox, audit, health, and output offload behavior remains unchanged.

**Tech Stack:** Go, existing `ADKMCPRuntimeStdioTransport`, existing
Workbench MCP catalog raw-auth resolver.

---

### Task 1: Transport Projection Contract

**Files:**
- Modify: `backend/application/agentthread/adk_mcp_stdio_transport_test.go`
- Modify: `backend/application/agentthread/adk_mcp_stdio_transport.go`

- [ ] **Step 1: Write failing tests**

Add tests that prove `auth_env` resolves string auth fields into env, auth
values override public config env collisions, and projection failures are
sanitized.

- [ ] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/agentthread -run 'TestADKMCPRuntimeStdioTransportProjectsAuthEnv|TestADKMCPRuntimeStdioTransportRejectsInvalidAuthEnvSafely' -count=1
```

Expected: build or assertion failure because projection code does not exist.

- [ ] **Step 3: Implement projection**

Add bounded `auth_env` parsing, auth JSON parsing, dot-path string lookup, env
name validation, collision override, and fixed sanitized errors.

- [ ] **Step 4: Verify GREEN**

Run the same focused test command and expect PASS.

### Task 2: Context And Regression Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [ ] **Step 1: Document M6.25**

Record the `auth_env` contract, safety rules, and remaining gaps.

- [ ] **Step 2: Verify packages**

Run:

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1
```

Expected: all packages pass.

- [ ] **Step 3: Verify migrations and diff hygiene**

Run:

```bash
docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations
git diff --check
```

Expected: Atlas exits 0 and `git diff --check` emits no output.
