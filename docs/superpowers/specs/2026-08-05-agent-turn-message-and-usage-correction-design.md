# Agent Turn Message And Usage Correction Design

## Background

Task `7670452654723563520` proved two independent projection defects:

- one successful resumed run exposed a non-final `message.completed` event and
  the durable final assistant message as two public chat replies;
- each provider model invocation was recorded once by the provider callback and
  once by the enclosing billing guard callback, doubling public token totals.

The run itself executed once. Its model/tool loop is valid and must not change.

## Product Contract

- One user turn creates one top-level execution run and yields at most one
  public assistant reply.
- The latest durable assistant message is authoritative for a completed run.
  Non-final assistant events and any obsolete duplicate durable messages from
  that run are not projected as standalone chat messages.
- When a run has no durable assistant message, canonical history may expose at
  most the latest non-empty assistant event. This preserves interrupted human
  clarification and live/failed-run compatibility without creating multiple
  replies.
- Tool calls, artifacts, checkpoints, retries, and model execution are not
  modified by this projection rule.
- One provider model invocation contributes token usage once. Nested wrappers
  may observe the same invocation but must not create additional usage rows.
- Billing reservation remains before the provider call and is not bypassed.

## Backend Design

### Canonical message projection

`loadCanonicalThreadMessagesWithBudget` will distinguish durable assistant
messages from event-only assistant projections by run ID.

1. If a run has durable assistant messages, retain only the latest durable
   message and discard all event-only assistant messages for that run from the
   canonical Message response.
2. Otherwise retain only the latest non-empty event-only assistant message for
   the run.
3. User messages and durable assistant messages keep their existing order and
   identifiers.
4. Run events remain unchanged, so Journal continues to render execution
   actions and snapshots.

### Nested usage callback deduplication

`ADKUsageBridge` will track parent/child chat-model callback identities.

1. A nested provider callback records the usage row and associates its usage
   fingerprint with the parent wrapper callback.
2. When the parent callback reports the same fingerprint, it is recognized as
   the same invocation and skipped.
3. The fingerprint is propagated through additional wrapper levels.
4. The normal event-usage suppression remains in place, so a provider callback
   and its `message.completed` event still produce one row.
5. Separate sequential or parallel calls keep their distinct callback lineage
   and remain independently billable.

## Compatibility And Risk Boundaries

- Interrupted clarification remains visible when no durable reply exists.
- Successful task history changes only by removing intermediate assistant
  events that were incorrectly promoted to chat messages.
- No IDL, database schema, queue, worker, model prompt, tool, artifact, or
  billing-reservation contract changes.
- Historical duplicate token rows are not mutated in this change. New runs use
  corrected accounting; historical cleanup requires a separate data decision.

## Verification

- Canonical handler tests cover different intermediate/final content, duplicate
  durable messages, and the event-only fallback.
- ADK usage tests cover nested billing-wrapper/provider callbacks plus event
  usage.
- Existing agentthread and canonical handler suites remain green.
- The task detail frontend suite verifies that canonical one-reply data keeps
  the accepted Journal layout without the temporary display-only grouping.
- An existing tool-producing task refreshed through the corrected projection
  verifies one final reply while its artifact remains visible. A fresh
  low-cost task verifies one visible assistant turn and non-doubled token
  totals without spending another full tool-execution run.
