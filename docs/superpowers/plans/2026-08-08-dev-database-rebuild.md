# Dev Database Rebuild Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Recreate the configured shared remote dev MySQL database from the repository migration directory and restore Workbench task creation without using a local MySQL database.

**Architecture:** Treat the ignored Debug environment as the only source of the remote dev target, fail closed unless the host is non-loopback and the database is exactly `opencoze`, then rebuild the database from an empty schema with Atlas forward migrations. Stop the backend during the destructive window, verify schema facts before restart, and perform UI acceptance against the same remote database.

**Tech Stack:** MySQL 8-compatible remote dev database, Atlas Community 1.2.3, Docker client containers, Go/Hertz backend, React frontend, Codex in-app browser.

---

### Task 1: Freeze and validate the target

**Files:**
- Read: `docker/.env.debug`
- Read: `docker/atlas/migrations/atlas.sum`

- [ ] **Step 1: Validate the remote target without printing credentials**

Load `docker/.env.debug` in a subshell and assert that `MYSQL_HOST` is neither empty nor
`localhost`, `127.0.0.1`, or `::1`; assert that `MYSQL_DATABASE` equals `opencoze`; assert that
`MYSQL_PORT`, `MYSQL_USER`, and `MYSQL_PASSWORD` are non-empty. Print only the host class,
port, and database name.

- [ ] **Step 2: Record the failing schema assertions**

Use a MySQL client container to query `information_schema`. Expected RED result before rebuild:
`agent_run_attempts`, Journal snapshot tables, `scheduled_tasks`, `notification_outbox`,
`announcements`, and `im_channel_configs` are missing while Atlas revisions claim the Journal
migrations are applied.

- [ ] **Step 3: Validate the migration directory**

Run the pinned Atlas image from `docs/superpowers/runbooks/local-debug-and-test.md` with
`migrate validate --dir file:///migrations`. Expected result: checksum and migration validation
pass before any database deletion.

### Task 2: Recreate the shared dev database

**Files:**
- Read: `docker/.env.debug`
- Read: `docker/atlas/migrations/*.sql`

- [ ] **Step 1: Stop the backend writer**

Stop the currently running backend process on port `8888` and verify the port no longer accepts
connections. Keep the frontend process unchanged.

- [ ] **Step 2: Resolve and verify the exact destructive target**

Read the configured server identity and current database name through a read-only connection.
Abort unless the server is reachable, `MYSQL_DATABASE=opencoze`, and the target is not a
loopback host.

- [ ] **Step 3: Drop and recreate only `opencoze`**

Using the configured remote MySQL administrative credential, run exactly:

```sql
DROP DATABASE IF EXISTS `opencoze`;
CREATE DATABASE `opencoze`
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;
```

Do not enumerate, drop, or modify any other database.

- [ ] **Step 4: Apply all repository migrations to the empty database**

Percent-encode the username and password in memory, construct an Atlas URL without printing it,
and run the pinned Atlas image with `migrate apply --url "$ATLAS_URL" --dir file:///migrations`.
Expected result: every repository migration is applied once and a fresh
`atlas_schema_revisions` history is created.

### Task 3: Verify schema and restart the application

**Files:**
- Read: `docker/.env.debug`
- Read: `docker/atlas/opencoze_latest_schema.hcl`

- [ ] **Step 1: Verify GREEN schema assertions**

Query `information_schema` and require all previously missing tables plus the canonical
Thread/Run tables and model-management tables to exist. Verify `agent_run_events` contains its
Journal columns and Atlas reports no pending migration.

- [ ] **Step 2: Confirm no local MySQL service is used**

Verify the backend Debug configuration still points to the non-loopback shared dev host. Do not
start a local MySQL container or change `MYSQL_HOST` to localhost.

- [ ] **Step 3: Restart the backend**

Start the backend with `APP_ENV=debug` and the repository Debug launcher, then verify port `8888`
and inspect fresh startup output. Expected result: no `table doesn't exist` errors for Journal,
scheduled task, notification, announcement, IM, or model-management tables.

### Task 4: Acceptance test Workbench task creation

**Files:**
- Read: `docs/superpowers/runbooks/local-debug-and-test.md`

- [ ] **Step 1: Initialize the empty dev environment**

Use the standard test account and system-administrator bootstrap path. Recreate required model
configuration through the system management UI if the empty database has no active model.

- [ ] **Step 2: Run the simple task regression**

In the in-app browser, open
`http://localhost:8080/space/7666420680379858944/chats/new` after selecting or creating a valid
space, submit a simple task, and verify that no `Internal server error` alert appears and a Thread
and Run are persisted.

- [ ] **Step 3: Run the Journal regression**

Enable projection/UI through the Basic Configuration CAS API only after dependencies are ready,
submit a Pro or Ultra root task, and verify one `agent_run_attempts` record is created with the
expected Run identity. Keep snapshots and checkpoint recovery disabled.

- [ ] **Step 4: Record final evidence**

Record the Atlas status, required table checks, backend health, browser URL/account/space,
visible task result, and any unverified items without including credentials or DSNs.

