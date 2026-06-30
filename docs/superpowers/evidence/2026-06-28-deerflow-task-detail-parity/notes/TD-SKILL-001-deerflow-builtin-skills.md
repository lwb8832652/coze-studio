# TD-SKILL-001 DeerFlow Builtin Skills

## DeerFlow Source Baseline

- Source root: `/Users/liuwenbo/code/BuildingAI/deer-flow/skills/public`
- Count: 22 public builtin skill directories.
- Count: 91 total files.
- Backend loading reference:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/packages/harness/deerflow/skills/storage/skill_storage.py`
- Skill API reference:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/backend/app/gateway/routers/skills.py`
- Settings UI reference:
  `/Users/liuwenbo/code/BuildingAI/deer-flow/frontend/src/components/workspace/settings/skill-settings-page.tsx`

DeerFlow discovers builtin public skills from the filesystem. Public/custom
skills default to enabled when `extensions_config.json` does not explicitly
override a skill state.

## Coze Implementation

- Embedded bundle root:
  `backend/application/skill/builtin_deerflow/public`
- Sync adapter:
  `backend/application/skill/deerflow_builtin_skills.go`
- Tests:
  `backend/application/skill/deerflow_builtin_skills_test.go`

The Coze adapter lazily syncs missing DeerFlow builtin skills per workspace
when Skill lists are read. Each imported skill is stored as `deer_skill` and
keeps the original `SKILL.md` snapshot plus support resources from scripts,
templates, references, assets, evals, and LICENSE files.

Idempotency uses the `SKILL.md` frontmatter `name`, not the directory name.
This matters for `vercel-deploy-claimable`, whose imported skill name is
`vercel-deploy`.

## Verification

```bash
cd backend
go test ./application/skill ./domain/skill/... ./application/agentthread -run 'Skill|skill' -count=1
go test ./api/router/coze -run 'Skill|skill' -count=1
```

Both commands passed on 2026-06-29.

Browser/API validation after rebuilding and restarting the Go backend on
2026-06-29:

- `GET /api/workbench/skills?space_id=7656275718757679104` returned `code=0`
  and 22 enabled builtin `deer_skill` records.
- Expected migrated names were present: `academic-paper-review`, `bootstrap`,
  `chart-visualization`, `deep-research`, `skill-creator`, and
  `vercel-deploy`.
- The DeerFlow directory name `vercel-deploy-claimable` did not leak into the
  imported skill name.
- Coze Skill page rendered the imported builtin skills after reload.
- Screenshot:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/TD-SKILL-001-skill-page-builtins.png`

## Remaining Acceptance

- Compare Coze Skill page screenshot against DeerFlow Settings Skills list and
  track visual/function deltas under `TD-SKILL-002`.
- Verify selecting one builtin Skill from the composer writes the expected
  `enable_skills` run config and runtime can load its latest `SKILL.md`
  under `TD-SKILL-003` and `TD-SKILL-005`.
