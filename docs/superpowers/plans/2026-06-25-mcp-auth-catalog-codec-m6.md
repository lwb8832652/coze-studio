# MCP Auth Catalog Codec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional MySQL MCP auth codec so durable storage can encode
auth at rest while internal callers still receive raw auth.

**Architecture:** Keep `Catalog` unchanged and add a MySQL-specific codec hook.
`Upsert` encodes auth before building the persistence object; `Get` and `List`
decode before returning API structs. The default codec is passthrough.

**Tech Stack:** Go, GORM, existing Workbench MCP catalog.

---

### Task 1: Codec Contract

**Files:**
- Modify: `backend/application/mcptool/catalog_mysql_test.go`
- Modify: `backend/application/mcptool/catalog_mysql.go`

- [ ] **Step 1: Write failing tests**

Add tests that prove encoded storage does not contain raw secret text, `Get`
and `List` decode back to raw auth, and encode/decode failures are sanitized.

- [ ] **Step 2: Verify RED**

Run:

```bash
cd backend
go test ./application/mcptool -run 'TestMySQLCatalogEncodesAuthAtRest|TestMySQLCatalogSanitizesAuthCodecErrors' -count=1
```

Expected: build failure because codec APIs do not exist.

- [ ] **Step 3: Implement codec**

Add `MCPAuthCodec`, passthrough default, `MySQLCatalogOption`, and
`WithMySQLCatalogAuthCodec`. Use the codec in `Upsert`, `Get`, and `List`.

- [ ] **Step 4: Verify GREEN**

Run the same focused command and expect PASS.

### Task 2: Context And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [ ] **Step 1: Document M6.26**

Record codec scope and remaining KMS/OAuth/rotation gaps.

- [ ] **Step 2: Verify packages**

Run:

```bash
cd backend
go test ./domain/agentthread/repository ./domain/agentthread/service ./application/agentthread ./application/mcptool ./application -count=1
```

Expected: all packages pass.

- [ ] **Step 3: Verify Atlas and diff hygiene**

Run:

```bash
docker run --rm -v "$PWD":/work -w /work arigaio/atlas:0.35.0-community-alpine migrate validate --dir file://docker/atlas/migrations
git diff --check
```

Expected: Atlas exits 0 and `git diff --check` emits no output.
