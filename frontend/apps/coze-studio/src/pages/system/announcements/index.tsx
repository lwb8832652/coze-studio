// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines -- Announcement management keeps its bounded workflow in one section. */

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  cancelAdminAnnouncement,
  createAdminAnnouncement,
  listAdminAnnouncementAuditEvents,
  listAdminAnnouncements,
  listAdminUsers,
  listAdminWorkspaces,
  publishAdminAnnouncement,
  replayAdminAnnouncements,
  scheduleAdminAnnouncement,
  updateAdminAnnouncement,
  AdminAnnouncementRouteType,
  type AdminAnnouncement,
  type AdminAnnouncementAudienceType,
  type AdminAnnouncementAuditEvent,
  type AdminAnnouncementDraftPayload,
  type AdminAnnouncementSeverity,
  type AdminAnnouncementStatus,
  type AdminUser,
  type AdminWorkspace,
} from '../service';

const statusLabels: Record<string, string> = {
  cancelled: '已取消',
  completed: '投影完成',
  draft: '草稿',
  failed: '投影失败',
  idle: '待发布',
  projecting: '写入通知中',
  published: '已发布',
  scheduled: '计划发布',
  snapshotting: '快照受众中',
};

const routeLabels: Record<AdminAnnouncementRouteType, string> = {
  [AdminAnnouncementRouteType.None]: '不跳转',
  [AdminAnnouncementRouteType.WorkspaceHome]: '工作空间首页',
  [AdminAnnouncementRouteType.SystemAnnouncements]: '系统公告管理',
};

const actionLabels: Record<string, string> = {
  cancelled: '取消',
  created: '创建草稿',
  projection_failed: '投影失败',
  projection_completed: '投影完成',
  publish_requested: '请求发布',
  published: '发布完成',
  replay_started: '开始重放',
  schedule_replayed: '计划任务触发',
  scheduled: '设置计划',
  updated: '更新草稿',
};

const formatTime = (value?: string) =>
  value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-';

const formatScheduleInput = (value?: string) => {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) {
    return '';
  }
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16);
};

const createIdempotencyKey = () => {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  return `announcement.${Date.now()}.${Math.random().toString(36).slice(2)}`;
};

const validateDraft = (draft: AdminAnnouncementDraftPayload) => {
  if (!draft.title.trim() || [...draft.title.trim()].length > 128) {
    return '标题需为 1 至 128 个字符';
  }
  if (!draft.body.trim() || [...draft.body.trim()].length > 512) {
    return '正文需为 1 至 512 个字符';
  }
  if (/[<>]/.test(`${draft.title}${draft.body}`)) {
    return '标题和正文不能包含 HTML 标签';
  }
  if (
    draft.route.type === AdminAnnouncementRouteType.WorkspaceHome &&
    !/^[1-9]\d*$/.test(draft.route.space_id ?? '')
  ) {
    return '工作空间跳转必须选择有效的工作空间 ID';
  }
  if (draft.audience.type !== 'all' && draft.audience.target_ids.length === 0) {
    return '请至少选择一个目标用户或工作空间';
  }
  return '';
};

const safeErrorCode = (value?: string) =>
  value && /^[a-z0-9_]{1,64}$/.test(value) ? value : '';

const projectionPresentation = (announcement: AdminAnnouncement) => {
  if (announcement.projection_status === 'failed') {
    return {
      state: 'projection_retry_pending',
      text: '投影失败，等待重试',
    };
  }
  if (
    announcement.projection_status === 'snapshotting' ||
    announcement.projection_status === 'projecting'
  ) {
    return {
      state: 'projection_pending',
      text: '正常投递中',
    };
  }
  return {
    state: announcement.projection_status,
    text: statusLabels[announcement.projection_status],
  };
};

type FeedbackTone = 'info' | 'warning' | 'error';

const feedbackToneClassNames: Record<FeedbackTone, string> = {
  info: 'text-[#245bdb]',
  warning: 'text-[#ad6800]',
  error: 'text-[#c42b1c]',
};

const publicationFeedback = (
  announcement: AdminAnnouncement,
  deferred: boolean,
  responseErrorCode?: string,
): { text: string; tone: FeedbackTone } => {
  const projection = projectionPresentation(announcement);
  const safeResponseErrorCode = safeErrorCode(responseErrorCode);
  const errorCode =
    safeResponseErrorCode === 'projection_retry_pending'
      ? safeResponseErrorCode
      : safeErrorCode(announcement.last_error_code);
  if (
    projection.state === 'projection_retry_pending' ||
    safeResponseErrorCode === 'projection_retry_pending'
  ) {
    return {
      text: errorCode
        ? `公告已发布，但通知投影失败，等待后台重试；错误代码：${errorCode}`
        : '公告已发布，但通知投影失败，等待后台重试',
      tone: 'error',
    };
  }
  if (projection.state === 'projection_pending') {
    return {
      text:
        announcement.status === 'published'
          ? '公告已发布，通知正在后台投递'
          : '发布请求已持久化，受众快照正在后台准备',
      tone: 'info',
    };
  }
  return {
    text: deferred
      ? announcement.status === 'published'
        ? '公告已发布，通知投递将在后台继续'
        : '发布请求已持久化，受众快照将在后台继续'
      : '公告已发布',
    tone: 'info',
  };
};

interface EditorActionToken {
  editorKey: string;
  editorSessionSequence: number;
  mutationSequence: number;
  selectedID: string;
}

const announcementEditorKey = (announcementID: string) =>
  `announcement:${announcementID}`;

const draftEditorKey = (nonce: string) => `draft:${nonce}`;

export const AnnouncementsSection = () => {
  const [announcements, setAnnouncements] = useState<AdminAnnouncement[]>([]);
  const [selected, setSelected] = useState<AdminAnnouncement | null>(null);
  const [audits, setAudits] = useState<AdminAnnouncementAuditEvent[]>([]);
  const [auditLoading, setAuditLoading] = useState(false);
  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');
  const [severity, setSeverity] = useState<AdminAnnouncementSeverity>('info');
  const [routeType, setRouteType] = useState<AdminAnnouncementRouteType>(
    AdminAnnouncementRouteType.None,
  );
  const [routeSpaceID, setRouteSpaceID] = useState('');
  const [audienceType, setAudienceType] =
    useState<AdminAnnouncementAudienceType>('all');
  const [targetIDs, setTargetIDs] = useState<string[]>([]);
  const [candidateKeyword, setCandidateKeyword] = useState('');
  const [candidateUsers, setCandidateUsers] = useState<AdminUser[]>([]);
  const [candidateWorkspaces, setCandidateWorkspaces] = useState<
    AdminWorkspace[]
  >([]);
  const [scheduledAt, setScheduledAt] = useState('');
  const [loading, setLoading] = useState(true);
  const [candidatesLoading, setCandidatesLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [feedback, setFeedback] = useState<{
    text: string;
    tone: FeedbackTone;
  }>({ text: '', tone: 'info' });
  const message = feedback.text;
  const messageTone = feedback.tone;
  const setMessage = useCallback(
    (text: string) => setFeedback({ text, tone: 'info' }),
    [],
  );
  const setMessageWithTone = useCallback(
    (text: string, tone: FeedbackTone) => setFeedback({ text, tone }),
    [],
  );
  const [statusFilter, setStatusFilter] = useState<
    AdminAnnouncementStatus | ''
  >('');
  const createKey = useRef(createIdempotencyKey());
  const publishKeys = useRef<Record<string, string>>({});
  const selectedIDRef = useRef('');
  const editorDirtyRef = useRef(false);
  const editorSessionKeyRef = useRef(draftEditorKey(createKey.current));
  const editorSessionSequenceRef = useRef(0);
  const mutationSequenceRef = useRef(0);
  const listRequestSequence = useRef(0);
  const auditRequestSequence = useRef(0);
  const candidateRequestSequence = useRef(0);
  const statusFilterRef = useRef<AdminAnnouncementStatus | ''>('');
  const mountedRef = useRef(true);

  const hydrateEditor = useCallback((announcement: AdminAnnouncement) => {
    candidateRequestSequence.current += 1;
    editorDirtyRef.current = false;
    selectedIDRef.current = announcement.id;
    setSelected(announcement);
    setTitle(announcement.title);
    setBody(announcement.body);
    setSeverity(announcement.severity);
    setRouteType(announcement.route.type);
    setRouteSpaceID(announcement.route.space_id || '');
    setAudienceType(announcement.audience.type);
    setTargetIDs([...announcement.audience.target_ids]);
    setScheduledAt(formatScheduleInput(announcement.scheduled_at));
    setCandidateKeyword('');
    setCandidateUsers([]);
    setCandidateWorkspaces([]);
    setCandidatesLoading(false);
  }, []);

  const startEditorSession = (editorKey: string, selectedID: string) => {
    listRequestSequence.current += 1;
    auditRequestSequence.current += 1;
    candidateRequestSequence.current += 1;
    editorSessionKeyRef.current = editorKey;
    editorSessionSequenceRef.current += 1;
    mutationSequenceRef.current += 1;
    selectedIDRef.current = selectedID;
    setLoading(false);
    setAuditLoading(false);
    setCandidatesLoading(false);
  };

  const beginAction = (): EditorActionToken => {
    listRequestSequence.current += 1;
    auditRequestSequence.current += 1;
    candidateRequestSequence.current += 1;
    setLoading(false);
    setAuditLoading(false);
    setCandidatesLoading(false);
    return {
      editorKey: editorSessionKeyRef.current,
      editorSessionSequence: editorSessionSequenceRef.current,
      mutationSequence: ++mutationSequenceRef.current,
      selectedID: selectedIDRef.current,
    };
  };

  const isActionCurrent = (action: EditorActionToken) =>
    mountedRef.current &&
    action.editorKey === editorSessionKeyRef.current &&
    action.editorSessionSequence === editorSessionSequenceRef.current &&
    action.mutationSequence === mutationSequenceRef.current &&
    action.selectedID === selectedIDRef.current;

  const refresh = useCallback(
    async (filter = statusFilterRef.current, action?: EditorActionToken) => {
      const requestSequence = ++listRequestSequence.current;
      const selectedIDAtRequest = selectedIDRef.current;
      const editorKeyAtRequest = editorSessionKeyRef.current;
      const editorSessionAtRequest = editorSessionSequenceRef.current;
      const mutationAtRequest = mutationSequenceRef.current;
      const canCommit = () =>
        mountedRef.current &&
        requestSequence === listRequestSequence.current &&
        (!action ||
          (action.editorKey === editorSessionKeyRef.current &&
            action.editorSessionSequence === editorSessionSequenceRef.current &&
            action.mutationSequence === mutationSequenceRef.current &&
            action.selectedID === selectedIDRef.current));
      setLoading(true);
      setMessage('');
      try {
        const response = await listAdminAnnouncements({
          limit: 50,
          status: filter || undefined,
        });
        if (!canCommit()) {
          return;
        }
        setAnnouncements(response.announcements);
        if (
          !action &&
          selectedIDAtRequest &&
          selectedIDRef.current === selectedIDAtRequest &&
          editorSessionKeyRef.current === editorKeyAtRequest &&
          editorSessionSequenceRef.current === editorSessionAtRequest &&
          mutationSequenceRef.current === mutationAtRequest &&
          !editorDirtyRef.current
        ) {
          const latest = response.announcements.find(
            item => item.id === selectedIDAtRequest,
          );
          if (latest) {
            hydrateEditor(latest);
          }
        }
      } catch (error) {
        if (canCommit()) {
          setMessage(
            error instanceof Error ? error.message : '加载公告失败，请稍后重试',
          );
        }
      } finally {
        if (canCommit()) {
          setLoading(false);
        }
      }
    },
    [hydrateEditor, setMessage],
  );

  useEffect(() => {
    mountedRef.current = true;
    void refresh();
    return () => {
      mountedRef.current = false;
      listRequestSequence.current += 1;
      editorSessionSequenceRef.current += 1;
      mutationSequenceRef.current += 1;
      auditRequestSequence.current += 1;
      candidateRequestSequence.current += 1;
    };
  }, [refresh]);

  const loadCandidates = async () => {
    if (!mountedRef.current) {
      return;
    }
    const requestSequence = ++candidateRequestSequence.current;
    const editorKey = editorSessionKeyRef.current;
    const editorSession = editorSessionSequenceRef.current;
    const selectedID = selectedIDRef.current;
    const requestedAudience = audienceType;
    const keyword = candidateKeyword;
    const canCommit = () =>
      mountedRef.current &&
      requestSequence === candidateRequestSequence.current &&
      editorKey === editorSessionKeyRef.current &&
      editorSession === editorSessionSequenceRef.current &&
      selectedID === selectedIDRef.current;
    if (requestedAudience === 'all') {
      setCandidateUsers([]);
      setCandidateWorkspaces([]);
      setCandidatesLoading(false);
      return;
    }
    setCandidatesLoading(true);
    setMessage('');
    try {
      if (requestedAudience === 'users') {
        const result = await listAdminUsers({
          keyword: keyword || undefined,
          page: 1,
          size: 100,
        });
        if (canCommit()) {
          setCandidateUsers(result.users);
          setCandidateWorkspaces([]);
        }
      } else {
        const result = await listAdminWorkspaces({
          keyword: keyword || undefined,
          page: 1,
          size: 100,
        });
        if (canCommit()) {
          setCandidateWorkspaces(result.workspaces);
          setCandidateUsers([]);
        }
      }
    } catch {
      if (canCommit()) {
        setMessage('加载受众候选项失败，请缩小搜索范围后重试');
      }
    } finally {
      if (canCommit()) {
        setCandidatesLoading(false);
      }
    }
  };

  const loadAudits = async (
    announcement: AdminAnnouncement,
    action?: EditorActionToken,
  ) => {
    const announcementID = announcement.id;
    if (
      !mountedRef.current ||
      selectedIDRef.current !== announcementID ||
      (action && !isActionCurrent(action))
    ) {
      return;
    }
    const requestSequence = ++auditRequestSequence.current;
    setAudits([]);
    setAuditLoading(true);
    const canCommit = () =>
      mountedRef.current &&
      requestSequence === auditRequestSequence.current &&
      selectedIDRef.current === announcementID &&
      (!action || isActionCurrent(action));
    try {
      const response = await listAdminAnnouncementAuditEvents(announcementID);
      if (!canCommit()) {
        return;
      }
      setAudits(response.audit_events);
    } catch {
      if (!canCommit()) {
        return;
      }
      setAudits([]);
      setMessage('加载公告审计记录失败');
    } finally {
      if (canCommit()) {
        setAuditLoading(false);
      }
    }
  };

  const openAnnouncement = (announcement: AdminAnnouncement) => {
    startEditorSession(announcementEditorKey(announcement.id), announcement.id);
    hydrateEditor(announcement);
    setBusy(false);
    setMessage('');
    void loadAudits(announcement);
  };

  const startDraft = () => {
    const nextCreateKey = createIdempotencyKey();
    createKey.current = nextCreateKey;
    startEditorSession(draftEditorKey(nextCreateKey), '');
    editorDirtyRef.current = false;
    auditRequestSequence.current += 1;
    setSelected(null);
    setAudits([]);
    setAuditLoading(false);
    setBusy(false);
    setTitle('');
    setBody('');
    setSeverity('info');
    setRouteType(AdminAnnouncementRouteType.None);
    setRouteSpaceID('');
    setAudienceType('all');
    setTargetIDs([]);
    setScheduledAt('');
    setCandidateKeyword('');
    setCandidateUsers([]);
    setCandidateWorkspaces([]);
    setMessage('');
  };

  const draftPayload = (): AdminAnnouncementDraftPayload => ({
    audience: {
      target_ids: audienceType === 'all' ? [] : targetIDs,
      type: audienceType,
    },
    body: body.trim(),
    route: {
      type: routeType,
      space_id:
        routeType === AdminAnnouncementRouteType.WorkspaceHome
          ? routeSpaceID.trim()
          : undefined,
    },
    severity,
    title: title.trim(),
  });

  const persistCurrentDraft = async (action: EditorActionToken) => {
    const draft = draftPayload();
    const validationMessage = validateDraft(draft);
    if (validationMessage) {
      setMessage(validationMessage);
      return null;
    }
    const selectedAtRequest = selected;
    const announcement =
      selectedAtRequest &&
      (selectedAtRequest.status === 'draft' ||
        selectedAtRequest.status === 'scheduled') &&
      selectedAtRequest.projection_status === 'idle'
        ? (
            await updateAdminAnnouncement(
              selectedAtRequest.id,
              selectedAtRequest.version,
              draft,
            )
          ).announcement
        : (await createAdminAnnouncement(draft, createKey.current))
            .announcement;
    if (!isActionCurrent(action)) {
      return null;
    }
    if (!selectedAtRequest) {
      const persistedEditorKey = announcementEditorKey(announcement.id);
      editorSessionKeyRef.current = persistedEditorKey;
      selectedIDRef.current = announcement.id;
      action.editorKey = persistedEditorKey;
      action.selectedID = announcement.id;
    }
    hydrateEditor(announcement);
    return announcement;
  };

  const saveDraft = async () => {
    const action = beginAction();
    setBusy(true);
    setMessage('');
    try {
      const announcement = await persistCurrentDraft(action);
      if (!announcement || !isActionCurrent(action)) {
        return;
      }
      void loadAudits(announcement, action);
      await refresh(statusFilterRef.current, action);
      if (!isActionCurrent(action)) {
        return;
      }
      setMessage('草稿已保存');
    } catch (error) {
      if (isActionCurrent(action)) {
        setMessage(error instanceof Error ? error.message : '保存草稿失败');
      }
    } finally {
      if (isActionCurrent(action)) {
        setBusy(false);
      }
    }
  };

  const schedule = async () => {
    if (!selected || !scheduledAt) {
      setMessage('请先保存草稿并选择计划发布时间');
      return;
    }
    const parsed = new Date(scheduledAt);
    if (!Number.isFinite(parsed.getTime()) || parsed.getTime() <= Date.now()) {
      setMessage('计划发布时间必须晚于当前时间');
      return;
    }
    const action = beginAction();
    setBusy(true);
    setMessage('');
    try {
      const persisted = await persistCurrentDraft(action);
      if (!persisted || !isActionCurrent(action)) {
        return;
      }
      const response = await scheduleAdminAnnouncement(
        persisted.id,
        persisted.version,
        parsed.toISOString(),
      );
      if (!isActionCurrent(action)) {
        return;
      }
      hydrateEditor(response.announcement);
      void loadAudits(response.announcement, action);
      await refresh(statusFilterRef.current, action);
      if (!isActionCurrent(action)) {
        return;
      }
      setMessage('计划发布时间已保存');
    } catch (error) {
      if (isActionCurrent(action)) {
        setMessage(error instanceof Error ? error.message : '设置计划失败');
      }
    } finally {
      if (isActionCurrent(action)) {
        setBusy(false);
      }
    }
  };

  const publish = async () => {
    if (!selected) {
      setMessage('请先保存草稿');
      return;
    }
    const action = beginAction();
    setBusy(true);
    setMessage('');
    try {
      const persisted = await persistCurrentDraft(action);
      if (!persisted || !isActionCurrent(action)) {
        return;
      }
      const key =
        publishKeys.current[persisted.id] ??
        (publishKeys.current[persisted.id] = createIdempotencyKey());
      const response = await publishAdminAnnouncement(
        persisted.id,
        persisted.version,
        key,
      );
      if (!isActionCurrent(action)) {
        return;
      }
      hydrateEditor(response.announcement);
      void loadAudits(response.announcement, action);
      await refresh(statusFilterRef.current, action);
      if (!isActionCurrent(action)) {
        return;
      }
      const nextFeedback = publicationFeedback(
        response.announcement,
        response.deferred,
        response.error_code,
      );
      setMessageWithTone(nextFeedback.text, nextFeedback.tone);
    } catch (error) {
      if (isActionCurrent(action)) {
        setMessageWithTone(
          error instanceof Error && error.message === '草稿保存失败'
            ? '草稿保存失败'
            : '发布公告失败，请稍后重试',
          'error',
        );
      }
    } finally {
      if (isActionCurrent(action)) {
        setBusy(false);
      }
    }
  };

  const cancel = async () => {
    if (!selected) {
      return;
    }
    const action = beginAction();
    setBusy(true);
    setMessage('');
    try {
      const response = await cancelAdminAnnouncement(
        selected.id,
        selected.version,
      );
      if (!isActionCurrent(action)) {
        return;
      }
      hydrateEditor(response.announcement);
      void loadAudits(response.announcement, action);
      await refresh(statusFilterRef.current, action);
      if (!isActionCurrent(action)) {
        return;
      }
      setMessage('公告已取消');
    } catch (error) {
      if (isActionCurrent(action)) {
        setMessage(error instanceof Error ? error.message : '取消公告失败');
      }
    } finally {
      if (isActionCurrent(action)) {
        setBusy(false);
      }
    }
  };

  const replay = async () => {
    const action = beginAction();
    const replayTarget = selected;
    setBusy(true);
    setMessage('');
    try {
      const result = await replayAdminAnnouncements(
        action.selectedID || undefined,
      );
      if (!isActionCurrent(action)) {
        return;
      }
      const failedMessage =
        result.failed > 0 ? `，失败 ${result.failed} 个` : '';
      const errorCodes = Object.entries(result.error_codes ?? {})
        .filter(
          ([code, count]) =>
            Boolean(safeErrorCode(code)) &&
            Number.isInteger(count) &&
            count > 0,
        )
        .map(([code, count]) => `${code}(${count})`)
        .join('、');
      const replayMessage =
        result.deferred > 0
          ? `重放已持久化，${result.deferred} 个公告等待后台恢复${failedMessage}`
          : `重放完成：处理 ${result.processed} 个，完成 ${result.completed} 个${failedMessage}`;
      await refresh(statusFilterRef.current, action);
      if (!isActionCurrent(action)) {
        return;
      }
      if (replayTarget) {
        void loadAudits(replayTarget, action);
      }
      setMessage(
        errorCodes
          ? `${replayMessage}；错误代码：${errorCodes}。请检查失败公告后再次重放`
          : replayMessage,
      );
    } catch (error) {
      if (isActionCurrent(action)) {
        setMessage(error instanceof Error ? error.message : '重放失败');
      }
    } finally {
      if (isActionCurrent(action)) {
        setBusy(false);
      }
    }
  };

  const toggleTarget = (targetID: string) => {
    editorDirtyRef.current = true;
    setTargetIDs(current =>
      current.includes(targetID)
        ? current.filter(value => value !== targetID)
        : [...current, targetID],
    );
  };

  const editable =
    !selected ||
    ((selected.status === 'draft' || selected.status === 'scheduled') &&
      selected.projection_status === 'idle');
  const candidates =
    audienceType === 'users'
      ? candidateUsers.map(user => ({
          id: user.user_id,
          label: user.name || user.email || user.user_id,
          hint: user.email || user.user_unique_name || '',
        }))
      : candidateWorkspaces.map(workspace => ({
          id: workspace.id,
          label: workspace.name || workspace.id,
          hint: workspace.owner_name || `${workspace.total_member_num ?? 0} 人`,
        }));

  return (
    <div className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <div className="flex flex-wrap items-start justify-between gap-[12px]">
            <div>
              <h2>公告编辑与预览</h2>
              <p>
                内容只以纯文本进入现有通知中心，跳转仅支持服务端白名单目标。
              </p>
            </div>
            <button
              className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:opacity-50"
              disabled={busy}
              type="button"
              onClick={startDraft}
            >
              新建草稿
            </button>
          </div>

          <div className="mt-[14px] grid gap-[14px] xl:grid-cols-[minmax(0,1.2fr)_minmax(280px,0.8fr)]">
            <div className="grid gap-[12px]">
              <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                标题
                <input
                  aria-label="公告标题"
                  className="h-[36px] rounded-[8px] border border-[#d8dde8] px-[10px] outline-none"
                  disabled={!editable || busy}
                  maxLength={128}
                  value={title}
                  onChange={event => {
                    editorDirtyRef.current = true;
                    setTitle(event.target.value);
                  }}
                />
              </label>
              <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                正文
                <textarea
                  aria-label="公告正文"
                  className="min-h-[132px] resize-y rounded-[8px] border border-[#d8dde8] p-[10px] outline-none"
                  disabled={!editable || busy}
                  maxLength={512}
                  value={body}
                  onChange={event => {
                    editorDirtyRef.current = true;
                    setBody(event.target.value);
                  }}
                />
              </label>
              <div className="grid gap-[12px] md:grid-cols-2">
                <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                  级别
                  <select
                    aria-label="公告级别"
                    className="h-[36px] rounded-[8px] border border-[#d8dde8] px-[10px]"
                    disabled={!editable || busy}
                    value={severity}
                    onChange={event => {
                      editorDirtyRef.current = true;
                      setSeverity(
                        event.target.value as AdminAnnouncementSeverity,
                      );
                    }}
                  >
                    <option value="info">信息</option>
                    <option value="success">成功</option>
                    <option value="warning">警告</option>
                    <option value="error">重要</option>
                  </select>
                </label>
                <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                  站内跳转（可选）
                  <select
                    aria-label="公告跳转类型"
                    className="h-[36px] rounded-[8px] border border-[#d8dde8] px-[10px] outline-none"
                    disabled={!editable || busy}
                    value={routeType}
                    onChange={event => {
                      editorDirtyRef.current = true;
                      const next = Number(
                        event.target.value,
                      ) as AdminAnnouncementRouteType;
                      setRouteType(next);
                      if (next !== AdminAnnouncementRouteType.WorkspaceHome) {
                        setRouteSpaceID('');
                      }
                    }}
                  >
                    <option value={AdminAnnouncementRouteType.None}>
                      不跳转
                    </option>
                    <option value={AdminAnnouncementRouteType.WorkspaceHome}>
                      工作空间首页
                    </option>
                    <option
                      value={AdminAnnouncementRouteType.SystemAnnouncements}
                    >
                      系统公告管理
                    </option>
                  </select>
                </label>
              </div>
              {routeType === AdminAnnouncementRouteType.WorkspaceHome ? (
                <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                  跳转工作空间 ID
                  <input
                    aria-label="公告跳转工作空间"
                    className="h-[36px] rounded-[8px] border border-[#d8dde8] px-[10px] outline-none"
                    disabled={!editable || busy}
                    inputMode="numeric"
                    value={routeSpaceID}
                    onChange={event => {
                      editorDirtyRef.current = true;
                      setRouteSpaceID(event.target.value);
                    }}
                  />
                </label>
              ) : null}

              <label className="grid gap-[6px] text-[13px] text-[#4d566a]">
                目标受众
                <select
                  aria-label="公告目标受众"
                  className="h-[36px] rounded-[8px] border border-[#d8dde8] px-[10px]"
                  disabled={!editable || busy}
                  value={audienceType}
                  onChange={event => {
                    candidateRequestSequence.current += 1;
                    setCandidatesLoading(false);
                    editorDirtyRef.current = true;
                    setAudienceType(
                      event.target.value as AdminAnnouncementAudienceType,
                    );
                    setTargetIDs([]);
                    setCandidateUsers([]);
                    setCandidateWorkspaces([]);
                  }}
                >
                  <option value="all">全体未删除用户</option>
                  <option value="workspaces">指定工作空间成员</option>
                  <option value="users">指定用户</option>
                </select>
              </label>

              {audienceType !== 'all' ? (
                <div className="rounded-[10px] border border-[#e7ebf3] p-[12px]">
                  <div className="flex flex-wrap gap-[8px]">
                    <input
                      aria-label="搜索公告受众"
                      className="h-[32px] min-w-[220px] flex-1 rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px]"
                      disabled={!editable || busy}
                      placeholder="按名称或邮箱搜索，最多返回 100 项"
                      value={candidateKeyword}
                      onChange={event => {
                        candidateRequestSequence.current += 1;
                        setCandidatesLoading(false);
                        setCandidateKeyword(event.target.value);
                      }}
                    />
                    <button
                      className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
                      disabled={!editable || candidatesLoading || busy}
                      type="button"
                      onClick={() => void loadCandidates()}
                    >
                      {candidatesLoading ? '加载中...' : '查询'}
                    </button>
                  </div>
                  <div className="mt-[10px] grid max-h-[180px] gap-[6px] overflow-y-auto">
                    {candidates.map(candidate => (
                      <label
                        className="flex items-center gap-[8px] rounded-[8px] px-[8px] py-[6px] hover:bg-[#f8fafc]"
                        key={candidate.id}
                      >
                        <input
                          checked={targetIDs.includes(candidate.id)}
                          disabled={!editable || busy}
                          type="checkbox"
                          onChange={() => toggleTarget(candidate.id)}
                        />
                        <span className="text-[13px] text-[#1d2333]">
                          {candidate.label}
                        </span>
                        <span className="text-[12px] text-[#7a8496]">
                          {candidate.hint}
                        </span>
                      </label>
                    ))}
                    {!candidatesLoading && candidates.length === 0 ? (
                      <p className="m-0 text-[12px] text-[#7a8496]">
                        输入关键词后查询持久化用户或工作空间。
                      </p>
                    ) : null}
                  </div>
                  <p className="mb-0 text-[12px] text-[#7a8496]">
                    已选择 {targetIDs.length}{' '}
                    项；服务端会重新校验并解析实际用户。
                  </p>
                </div>
              ) : null}
            </div>

            <aside
              aria-label="公告预览"
              className="self-start rounded-[14px] border border-[#dfe5ed] bg-[#f8fafc] p-[16px]"
            >
              <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[#7a8496]">
                通知中心预览
              </span>
              <h3 className="mb-[8px] mt-[14px] text-[17px] text-[#1d2333]">
                {title.trim() || '公告标题'}
              </h3>
              <p className="whitespace-pre-wrap text-[13px] leading-[1.7] text-[#4d566a]">
                {body.trim() || '公告正文将在这里以纯文本显示。'}
              </p>
              <div className="mt-[16px] flex flex-wrap gap-[6px] text-[11px]">
                <span className="rounded-full bg-white px-[8px] py-[4px] text-[#4d566a]">
                  {severity}
                </span>
                <span className="rounded-full bg-white px-[8px] py-[4px] text-[#4d566a]">
                  {audienceType}
                </span>
                {routeType !== AdminAnnouncementRouteType.None ? (
                  <span className="rounded-full bg-white px-[8px] py-[4px] text-[#4d566a]">
                    {routeLabels[routeType]}
                    {routeSpaceID ? ` · ${routeSpaceID}` : ''}
                  </span>
                ) : null}
              </div>
            </aside>
          </div>

          <div className="mt-[14px] flex flex-wrap items-end gap-[8px]">
            <button
              className="h-[34px] rounded-[8px] bg-[#1d2333] px-[14px] text-[13px] text-white disabled:opacity-50"
              disabled={!editable || busy}
              type="button"
              onClick={() => void saveDraft()}
            >
              {busy ? '处理中...' : selected ? '保存修改' : '保存草稿'}
            </button>
            <input
              aria-label="公告计划发布时间"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px]"
              disabled={!selected || !editable || busy}
              type="datetime-local"
              value={scheduledAt}
              onChange={event => {
                editorDirtyRef.current = true;
                setScheduledAt(event.target.value);
              }}
            />
            <button
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:opacity-50"
              disabled={!selected || !editable || busy}
              type="button"
              onClick={() => void schedule()}
            >
              计划发布
            </button>
            <button
              className="h-[34px] rounded-[8px] border border-[#1d2333] px-[12px] text-[13px] disabled:opacity-50"
              disabled={!selected || !editable || busy}
              type="button"
              onClick={() => void publish()}
            >
              立即发布
            </button>
            <button
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:opacity-50"
              disabled={!selected || !editable || busy}
              type="button"
              onClick={() => void cancel()}
            >
              取消公告
            </button>
            <button
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:opacity-50"
              disabled={busy}
              type="button"
              onClick={() => void replay()}
            >
              重放待处理
            </button>
          </div>
          {message ? (
            <p
              aria-live="polite"
              className={`mb-0 mt-[10px] text-[13px] ${feedbackToneClassNames[messageTone]}`}
              data-feedback-tone={messageTone}
              role={messageTone === 'error' ? 'alert' : 'status'}
            >
              {message}
            </p>
          ) : null}
        </div>
        <span>{selected ? statusLabels[selected.status] : '新草稿'}</span>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <div className="flex flex-wrap items-start justify-between gap-[12px]">
            <div>
              <h2>公告记录</h2>
              <p>发布进度和收件人数来自持久化投影状态。</p>
            </div>
            <div className="flex gap-[8px]">
              <select
                aria-label="公告状态筛选"
                className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[8px] text-[12px]"
                disabled={busy}
                value={statusFilter}
                onChange={event => {
                  const next = event.target.value as
                    | AdminAnnouncementStatus
                    | '';
                  statusFilterRef.current = next;
                  setStatusFilter(next);
                  void refresh(next);
                }}
              >
                <option value="">全部状态</option>
                <option value="draft">草稿</option>
                <option value="scheduled">计划发布</option>
                <option value="published">已发布</option>
                <option value="cancelled">已取消</option>
              </select>
              <button
                className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px] disabled:opacity-50"
                disabled={busy}
                type="button"
                onClick={() => void refresh()}
              >
                刷新
              </button>
            </div>
          </div>
          {loading ? <p role="status">正在加载公告...</p> : null}
          <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
            <table className="w-full min-w-[860px] border-collapse text-left text-[13px]">
              <thead className="bg-[#f8fafc] text-[#687385]">
                <tr>
                  <th className="px-[12px] py-[9px] font-medium">标题</th>
                  <th className="px-[12px] py-[9px] font-medium">状态</th>
                  <th className="px-[12px] py-[9px] font-medium">受众</th>
                  <th className="px-[12px] py-[9px] font-medium">投影</th>
                  <th className="px-[12px] py-[9px] font-medium">更新时间</th>
                  <th className="px-[12px] py-[9px] font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {!loading && announcements.length === 0 ? (
                  <tr>
                    <td
                      className="px-[12px] py-[18px] text-[#7a8496]"
                      colSpan={6}
                    >
                      暂无公告。
                    </td>
                  </tr>
                ) : null}
                {announcements.map(announcement => {
                  const projection = projectionPresentation(announcement);
                  const errorCode = safeErrorCode(announcement.last_error_code);
                  return (
                    <tr
                      className="border-t border-[#edf0f5]"
                      key={announcement.id}
                    >
                      <td className="max-w-[280px] px-[12px] py-[10px] text-[#1d2333]">
                        {announcement.title}
                      </td>
                      <td className="px-[12px] py-[10px] text-[#4d566a]">
                        {statusLabels[announcement.status]}
                      </td>
                      <td className="px-[12px] py-[10px] text-[#4d566a]">
                        {announcement.audience.type}
                      </td>
                      <td className="px-[12px] py-[10px] text-[#4d566a]">
                        <span aria-label={`投影状态 ${projection.state}`}>
                          {projection.text}
                        </span>{' '}
                        · {announcement.projected_count}/
                        {announcement.recipient_count}
                        {errorCode ? ` · ${errorCode}` : ''}
                      </td>
                      <td className="px-[12px] py-[10px] text-[#4d566a]">
                        {formatTime(announcement.updated_at)}
                      </td>
                      <td className="px-[12px] py-[10px]">
                        <button
                          className="h-[28px] rounded-[7px] border border-[#d8dde8] px-[9px] text-[12px] disabled:opacity-50"
                          disabled={busy}
                          type="button"
                          onClick={() => openAnnouncement(announcement)}
                        >
                          查看
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
        <span>{announcements.length} 条</span>
      </article>

      {selected ? (
        <article className="coze-prototype-workspace-settings-row">
          <div className="w-full">
            <h2>审计历史</h2>
            <p>公告 ID {selected.id}，发布人和操作人均来自服务端认证上下文。</p>
            <div className="mt-[10px] grid gap-[8px]">
              {auditLoading ? (
                <p className="text-[13px] text-[#7a8496]" role="status">
                  正在加载审计记录...
                </p>
              ) : null}
              {!auditLoading && audits.length === 0 ? (
                <p className="text-[13px] text-[#7a8496]">暂无审计记录。</p>
              ) : null}
              {!auditLoading
                ? audits.map(audit => (
                    <div
                      className="flex flex-wrap items-center justify-between gap-[8px] rounded-[9px] border border-[#edf0f5] px-[11px] py-[9px]"
                      key={audit.id}
                    >
                      <div>
                        <strong className="text-[13px] text-[#1d2333]">
                          {actionLabels[audit.action] || audit.action}
                        </strong>
                        <span className="ml-[8px] text-[12px] text-[#7a8496]">
                          actor {audit.actor_id} ·{' '}
                          {formatTime(audit.created_at)}
                        </span>
                      </div>
                      <span className="text-[12px] text-[#4d566a]">
                        {audit.result}
                        {audit.error_code ? ` · ${audit.error_code}` : ''}
                      </span>
                    </div>
                  ))
                : null}
            </div>
          </div>
          <span>只读审计</span>
        </article>
      ) : null}
    </div>
  );
};
