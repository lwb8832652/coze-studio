# M2.18 Eino ADK Web Tools Plan

## Checklist

- [x] Add tests for disabled config, host allowlist, private-IP rejection,
      bounded fetch output, redirect policy, search backend, and default
      provider wiring.
- [x] Implement `ADKWebToolCatalog`, safe `web_fetch`, and backend-driven
      `web_search`.
- [x] Wire disabled-by-default Web catalog into `NewDefaultADKToolProvider`.
- [x] Update `AGENTS.md` and the DeerFlow parity roadmap.
- [x] Pass focused tests, full `application/agentthread` tests, and
      `git diff --check`.
