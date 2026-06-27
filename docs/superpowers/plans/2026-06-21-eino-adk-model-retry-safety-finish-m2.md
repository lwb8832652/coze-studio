# Eino ADK Model Retry Safety Finish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable explicit Eino ADK model retry for task runs and classify safety finish reasons in Coze ADK events.

**Architecture:** Add a Coze config adapter that returns `adk.ModelRetryConfig` and pass it into `ChatModelAgentConfig`. Keep finish classification in `MapADKEvent`, where Eino runtime events become Coze-owned public events.

**Tech Stack:** Go, Eino ADK v0.9.9, existing `agentthread` tests.

---

### Task 1: Retry Config Adapter

**Files:**
- Create: `backend/application/agentthread/adk_model_reliability.go`
- Test: `backend/application/agentthread/adk_model_reliability_test.go`

- [x] Write tests for disabled default config, valid retry config, and invalid retry limits.
- [x] Run `cd backend && go test -gcflags="all=-l -N" ./application/agentthread -run 'TestADKModelReliability'`.
- [x] Implement parsing and `adk.ModelRetryConfig` construction.
- [x] Re-run the focused reliability tests.

### Task 2: Agent Factory Retry Wiring

**Files:**
- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Test: `backend/application/agentthread/adk_agent_factory_test.go`

- [x] Write a failing test proving configured retry makes a flaky model succeed on the second call.
- [x] Write a failing test proving exhausted retries surface `adk.RetryExhaustedError`.
- [x] Pass the parsed retry config into `adk.ChatModelAgentConfig`.
- [x] Re-run the agent factory retry tests.

### Task 3: Safety Finish Classification

**Files:**
- Modify: `backend/application/agentthread/adk_event_mapper.go`
- Test: `backend/application/agentthread/adk_event_mapper_test.go`

- [x] Write a failing test for `finish_reason:"content_filter"` mapping to `model.safety_finish`.
- [x] Implement finish classification payloads.
- [x] Re-run the mapper test plus existing message mapping tests.

### Task 4: Docs And Verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`

- [x] Document retry/safety finish rules and failover boundary.
- [x] Update roadmap evidence to mark M2.13a complete and leave failover candidate selection open.
- [x] Run `cd backend && go test -gcflags="all=-l -N" ./application/agentthread`.
- [x] Run `git diff --check`.
