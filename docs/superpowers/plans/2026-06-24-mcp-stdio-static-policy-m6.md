# MCP Stdio Static Policy M6 Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a concrete static policy gate for stdio MCP transport calls.

**Architecture:** `ADKMCPRuntimeStdioStaticPolicy` implements
`ADKMCPRuntimeStdioPolicy` and validates parsed sandbox calls with exact
command allow-lists, isolated working-directory prefixes, env key projection,
and deterministic args/env budgets. It returns fixed sanitized errors and does
not execute anything.

**Tech Stack:** Go, Coze-owned ADK MCP runtime executor, MCP stdio transport
boundary.

---

## Result

M6.9 provides the first reusable policy implementation that can be mounted
behind `ADKMCPRuntimeStdioTransport` before a real sandbox exists.

## Completed

- Added `ADKMCPRuntimeStdioStaticPolicyOptions`.
- Added `ADKMCPRuntimeStdioStaticPolicy` implementing
  `ADKMCPRuntimeStdioPolicy`.
- Added exact command allow-list enforcement.
- Added absolute working-directory prefix isolation with cleaned path
  containment.
- Added argument count and total-byte budgets.
- Added environment key allow-list plus env count/value budgets.
- Kept default options fail-closed.
- Added focused tests for allowed config, default deny, command deny, required
  and isolated workdirs, prefix escape, arg budgets, env allow-list, env
  budgets, and transport-before-sandbox policy denial.

## Verification

- `go test ./application/agentthread -run 'TestADKMCPRuntimeStdioStaticPolicy|TestADKMCPRuntimeStdioTransportUsesStaticPolicy' -count=1`

## Remaining

Concrete sandbox execution, isolated workdir creation and cleanup, env secret
projection, process/session limits, Eino MCP stdio adapter invocation, audit
persistence, health classification, output offload, production bootstrap
wiring, frontend policy controls, and browser E2E remain open M6 or production
acceptance work.
