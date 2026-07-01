# P1-A-004 Sidebar Recent Tasks Pagination

## Scope

This evidence verifies the Coze `我的任务` sidebar behavior requested for P1:
bounded recent-task rendering, scroll-to-load-more, visible loading affordance,
and immediate insertion of new task threads without refreshing the page.

## Source Contract

- Component:
  `frontend/apps/coze-studio/src/components/workspace-sub-menu/workspace-task-list.tsx`
- API:
  `GET /api/workbench/task_threads?space_id=:space_id&page=:page&page_size=20`
- Backend handler:
  `backend/api/handler/coze/workbench_thread_service.go`
- Immediate insertion event:
  `coze:workspace-task-thread-upsert`
- Loading affordance:
  `加载更多任务...`

The sidebar requests page 1 with `page_size=20`, observes a sentinel inside
the sidebar list with `IntersectionObserver`, and requests the next page when
the sentinel becomes visible. New or updated task threads are merged through
the upsert event: full upserts move the task to the top, while title/status
patches update an existing row without inserting partial rows.

## Browser Evidence

- Page:
  `http://localhost:8080/space/7656275718757679104/tasks/7657504771049259008`
- Screenshot after scroll/load-more:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/screenshots/coze/P1-A-004-sidebar-recent-tasks-pagination.png`
- Safe DOM summary:
  `docs/superpowers/evidence/2026-06-28-deerflow-task-detail-parity/notes/P1-A-004-sidebar-recent-tasks-summary.json`

Safe browser result:

- Before scrolling, the sidebar rendered 20 recent tasks.
- The sidebar list was independently scrollable and had a load-more sentinel.
- After scrolling the sidebar list toward the bottom, the visible task row
  count increased to 40.
- The summary stores only counts, dimensions, endpoint shape, and redaction
  notes; task titles, prompt text, and model completions are not retained.

## Automated Verification

Command:

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx -t "loads more recent tasks|prepends a newly created task thread|patches an existing recent task title"
```

Result: passed, 3 tests.

Covered behavior:

- `loads more recent tasks when the sidebar reaches the bottom`
- `prepends a newly created task thread without waiting for a page refresh`
- `patches an existing recent task title without inserting a partial task`

## Remaining Gap

No open P1-A-004 gap remains. DeerFlow names this area as recent chats, while
Coze keeps the required prototype vocabulary `我的任务`; the verified behavior
is pagination and immediate update parity rather than identical menu labels.
