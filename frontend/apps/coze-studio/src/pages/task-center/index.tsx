// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines, complexity -- The page coordinates the complete task lifecycle. */

import { useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { workbenchTask } from '@coze-studio/api-schema';
import { Modal, Toast } from '@coze-arch/coze-design';

import {
  buildTaskPayload,
  formatScheduleSummary,
  formatTaskStatus,
  formatTimestamp,
} from './utils';
import type { TaskFormValues } from './types';
import TaskFormModal from './task-form-modal';
import {
  createScheduledTask,
  deleteScheduledTask,
  disableScheduledTask,
  enableScheduledTask,
  executeScheduledTask,
  listScheduledTasks,
  updateScheduledTask,
  type ScheduledTask,
} from './service';
import ExecutionRecordsModal from './execution-records-modal';
import './index.less';

const DEFAULT_PAGE_SIZE = 20;

const targetTypeLabel = (type: workbenchTask.ScheduledTaskTargetType) =>
  type === workbenchTask.ScheduledTaskTargetType.Workflow ? '工作流' : 'Agent';

const taskStatusTone = (task: ScheduledTask) => {
  if (task.status === workbenchTask.ScheduledTaskStatus.Disabled) {
    return 'disabled';
  }
  if (task.status === workbenchTask.ScheduledTaskStatus.Completed) {
    return 'completed';
  }
  if (
    task.latest_execution_status ===
      workbenchTask.ScheduledTaskExecutionStatus.Queued ||
    task.latest_execution_status ===
      workbenchTask.ScheduledTaskExecutionStatus.Running
  ) {
    return 'running';
  }
  if (
    task.latest_execution_status ===
      workbenchTask.ScheduledTaskExecutionStatus.Failed ||
    task.latest_execution_status ===
      workbenchTask.ScheduledTaskExecutionStatus.Canceled
  ) {
    return 'failed';
  }
  return 'success';
};

const TaskCenterPage = () => {
  const { space_id: spaceID = '' } = useParams();
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [keywordDraft, setKeywordDraft] = useState('');
  const [keyword, setKeyword] = useState('');
  const [targetType, setTargetType] = useState(0);
  const [status, setStatus] = useState(0);
  const [formOpen, setFormOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<ScheduledTask>();
  const [saving, setSaving] = useState(false);
  const [deletingTask, setDeletingTask] = useState<ScheduledTask>();
  const [recordsTask, setRecordsTask] = useState<ScheduledTask>();
  const [actionTaskID, setActionTaskID] = useState('');

  const loadTasks = useCallback(async () => {
    if (!spaceID) {
      return;
    }
    setLoading(true);
    setError('');
    try {
      const request: workbenchTask.ListScheduledTasksRequest = {
        space_id: spaceID,
        page,
        page_size: pageSize,
      };
      if (keyword) {
        request.keyword = keyword;
      }
      if (targetType) {
        request.target_type = targetType;
      }
      if (status) {
        request.status = status;
      }
      const response = await listScheduledTasks(request);
      setTasks(response.data?.tasks || []);
      setTotal(response.data?.total || 0);
    } catch {
      setError('任务列表加载失败，请检查服务状态后重试');
    } finally {
      setLoading(false);
    }
  }, [keyword, page, pageSize, spaceID, status, targetType]);

  useEffect(() => {
    void loadTasks();
  }, [loadTasks]);

  const pages = Math.max(1, Math.ceil(total / pageSize));
  const enabledCount = useMemo(
    () =>
      tasks.filter(
        task => task.status === workbenchTask.ScheduledTaskStatus.Enabled,
      ).length,
    [tasks],
  );

  const resetPageAndFilter = (setter: () => void) => {
    setPage(1);
    setter();
  };

  const clearFilters = () => {
    setKeyword('');
    setKeywordDraft('');
    setTargetType(0);
    setStatus(0);
    setPage(1);
  };

  const handleSave = async (values: TaskFormValues) => {
    if (!spaceID) {
      return;
    }
    setSaving(true);
    try {
      const common = {
        space_id: spaceID,
        name: values.name.trim(),
        target_type: values.targetType,
        target_id: values.targetID,
        schedule_type: values.scheduleType,
        timezone: values.timezone,
        payload: buildTaskPayload(values),
        keep_conversation:
          values.targetType === workbenchTask.ScheduledTaskTargetType.Agent
            ? values.keepConversation
            : false,
        max_executions:
          values.scheduleType === workbenchTask.ScheduledTaskScheduleType.Once
            ? 1
            : values.maxExecutions,
        cron_expr:
          values.scheduleType === workbenchTask.ScheduledTaskScheduleType.Cron
            ? values.cronExpr.trim()
            : undefined,
        run_once_at:
          values.scheduleType === workbenchTask.ScheduledTaskScheduleType.Once
            ? Math.floor(new Date(values.runOnceAt).getTime() / 1000)
            : undefined,
        minute: values.minute,
        hour: values.hour,
        weekday: values.weekday,
      };
      if (editingTask) {
        await updateScheduledTask({
          ...common,
          task_id: editingTask.id,
          version: editingTask.version,
        });
        Toast.success({ content: '任务已更新' });
      } else {
        await createScheduledTask(common);
        Toast.success({ content: '任务已创建并启用' });
      }
      setFormOpen(false);
      setEditingTask(undefined);
      await loadTasks();
    } catch {
      Toast.error({ content: '任务保存失败，请检查配置后重试' });
    } finally {
      setSaving(false);
    }
  };

  const runTaskAction = async (
    task: ScheduledTask,
    action: 'execute' | 'enable' | 'disable',
  ) => {
    if (!spaceID) {
      return;
    }
    setActionTaskID(task.id);
    try {
      const request = { task_id: task.id, space_id: spaceID };
      if (action === 'execute') {
        await executeScheduledTask(request);
        Toast.success({ content: '任务已加入执行队列' });
      } else if (action === 'enable') {
        await enableScheduledTask(request);
        Toast.success({ content: '任务已启用' });
      } else {
        await disableScheduledTask(request);
        Toast.success({ content: '任务已停用' });
      }
      await loadTasks();
    } catch {
      Toast.error({ content: '操作失败，请稍后重试' });
    } finally {
      setActionTaskID('');
    }
  };

  const handleDelete = async () => {
    if (!spaceID || !deletingTask) {
      return;
    }
    setActionTaskID(deletingTask.id);
    try {
      await deleteScheduledTask({
        task_id: deletingTask.id,
        space_id: spaceID,
      });
      Toast.success({ content: '任务已删除' });
      setDeletingTask(undefined);
      await loadTasks();
    } catch {
      Toast.error({ content: '任务删除失败，请稍后重试' });
    } finally {
      setActionTaskID('');
    }
  };

  return (
    <main className="task-center-page">
      <div className="task-center-page__ambient" aria-hidden="true" />
      <header className="task-center-hero">
        <div>
          <span className="task-center-hero__eyebrow">AUTOMATION</span>
          <h1>任务中心</h1>
          <p>让 Agent 与工作流按计划稳定执行，结果与上下文全程可追踪。</p>
        </div>
        <button
          type="button"
          className="task-center-primary-button task-center-hero__create"
          onClick={() => {
            setEditingTask(undefined);
            setFormOpen(true);
          }}
        >
          <span aria-hidden="true">+</span>
          新建任务
        </button>
      </header>

      <section className="task-center-overview" aria-label="任务概览">
        <article>
          <span>任务总数</span>
          <strong>{total}</strong>
          <small>当前工作空间</small>
        </article>
        <article>
          <span>本页运行中</span>
          <strong>{enabledCount}</strong>
          <small>自动调度已开启</small>
        </article>
        <article>
          <span>调度引擎</span>
          <strong className="task-center-overview__online">正常</strong>
          <small>多实例安全触发</small>
        </article>
      </section>

      <section className="task-center-panel">
        <div className="task-center-toolbar">
          <div className="task-center-toolbar__filters">
            <label className="task-center-search">
              <span>搜索</span>
              <input
                value={keywordDraft}
                placeholder="搜索任务或执行目标"
                onChange={event => setKeywordDraft(event.target.value)}
                onKeyDown={event => {
                  if (event.key === 'Enter') {
                    resetPageAndFilter(() => setKeyword(keywordDraft.trim()));
                  }
                }}
              />
              {keywordDraft ? (
                <button
                  type="button"
                  onClick={() => {
                    setKeywordDraft('');
                    resetPageAndFilter(() => setKeyword(''));
                  }}
                >
                  清空
                </button>
              ) : null}
            </label>
            <select
              aria-label="目标类型"
              value={targetType}
              onChange={event =>
                resetPageAndFilter(() =>
                  setTargetType(Number(event.target.value)),
                )
              }
            >
              <option value={0}>全部类型</option>
              <option value={workbenchTask.ScheduledTaskTargetType.Agent}>
                Agent
              </option>
              <option value={workbenchTask.ScheduledTaskTargetType.Workflow}>
                工作流
              </option>
            </select>
            <select
              aria-label="任务状态"
              value={status}
              onChange={event =>
                resetPageAndFilter(() => setStatus(Number(event.target.value)))
              }
            >
              <option value={0}>全部状态</option>
              <option value={workbenchTask.ScheduledTaskStatus.Enabled}>
                运行中
              </option>
              <option value={workbenchTask.ScheduledTaskStatus.Disabled}>
                已停用
              </option>
              <option value={workbenchTask.ScheduledTaskStatus.Completed}>
                已完成
              </option>
            </select>
            <button
              type="button"
              className="task-center-toolbar__search-button"
              onClick={() =>
                resetPageAndFilter(() => setKeyword(keywordDraft.trim()))
              }
            >
              查询
            </button>
            <button
              type="button"
              className="task-center-toolbar__reset-button"
              disabled={!keyword && !keywordDraft && !targetType && !status}
              onClick={clearFilters}
            >
              重置
            </button>
          </div>
          <button
            type="button"
            className="task-center-toolbar__refresh"
            disabled={loading}
            onClick={() => void loadTasks()}
          >
            刷新
          </button>
        </div>

        {loading ? (
          <div className="task-center-loading" aria-label="正在加载任务">
            {[1, 2, 3].map(item => (
              <span key={item} />
            ))}
          </div>
        ) : null}
        {!loading && error ? (
          <div className="task-center-state task-center-state--error">
            <strong>任务暂时无法加载</strong>
            <p>{error}</p>
            <button type="button" onClick={() => void loadTasks()}>
              重新加载
            </button>
          </div>
        ) : null}
        {!loading && !error && !tasks.length ? (
          <div className="task-center-state task-center-state--empty">
            <div aria-hidden="true">
              <span>00</span>
              <i />
            </div>
            <strong>
              {keyword ? '没有匹配的任务' : '让第一项工作自动运行'}
            </strong>
            <p>
              {keyword
                ? '调整关键词或筛选条件后再次查询。'
                : '创建任务，选择 Agent 或工作流，再设置清晰的执行计划。'}
            </p>
            <button
              type="button"
              onClick={() => {
                if (keyword || targetType || status) {
                  clearFilters();
                } else {
                  setFormOpen(true);
                }
              }}
            >
              {keyword || targetType || status ? '清除筛选' : '创建第一个任务'}
            </button>
          </div>
        ) : null}
        {!loading && !error && tasks.length ? (
          <div className="task-center-table-wrap">
            <table className="task-center-table">
              <thead>
                <tr>
                  <th>任务</th>
                  <th>执行目标</th>
                  <th>调度规则</th>
                  <th>下次执行</th>
                  <th>最近执行</th>
                  <th>状态</th>
                  <th>执行次数</th>
                  <th>创建人</th>
                  <th>创建时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {tasks.map(task => {
                  const busy = actionTaskID === task.id;
                  const enabled =
                    task.status === workbenchTask.ScheduledTaskStatus.Enabled;
                  return (
                    <tr key={task.id}>
                      <td>
                        <div className="task-center-table__task">
                          <span>{task.name.slice(0, 1).toUpperCase()}</span>
                          <div>
                            <strong>{task.name}</strong>
                            <small>
                              更新于 {formatTimestamp(task.updated_at)}
                            </small>
                          </div>
                        </div>
                      </td>
                      <td>
                        <div className="task-center-table__target">
                          <span data-type={task.target_type}>
                            {targetTypeLabel(task.target_type)}
                          </span>
                          <strong>{task.target_name}</strong>
                        </div>
                      </td>
                      <td>
                        <div className="task-center-table__schedule">
                          <strong>{formatScheduleSummary(task)}</strong>
                          <small>{task.timezone}</small>
                        </div>
                      </td>
                      <td>
                        <span className="task-center-table__next">
                          {task.status ===
                          workbenchTask.ScheduledTaskStatus.Completed
                            ? '已结束'
                            : formatTimestamp(task.next_execution_at)}
                        </span>
                      </td>
                      <td>
                        <span className="task-center-table__next">
                          {formatTimestamp(task.latest_execution_at)}
                        </span>
                      </td>
                      <td>
                        <span
                          className="task-center-status"
                          data-status={taskStatusTone(task)}
                        >
                          <i />
                          {formatTaskStatus(task)}
                        </span>
                      </td>
                      <td>
                        <span className="task-center-table__creator">
                          {task.creator_name ||
                            `用户 #${task.creator_id.slice(-6)}`}
                        </span>
                      </td>
                      <td>
                        <span className="task-center-table__next">
                          {formatTimestamp(task.created_at)}
                        </span>
                      </td>
                      <td>
                        <strong className="task-center-table__count">
                          {task.execution_count}
                          {task.max_executions
                            ? ` / ${task.max_executions}`
                            : ''}
                        </strong>
                      </td>
                      <td>
                        <div className="task-center-table__actions">
                          <button
                            type="button"
                            disabled={
                              busy ||
                              task.status ===
                                workbenchTask.ScheduledTaskStatus.Completed
                            }
                            onClick={() => void runTaskAction(task, 'execute')}
                          >
                            立即执行
                          </button>
                          {task.status !==
                          workbenchTask.ScheduledTaskStatus.Completed ? (
                            <button
                              type="button"
                              disabled={busy}
                              onClick={() =>
                                void runTaskAction(
                                  task,
                                  enabled ? 'disable' : 'enable',
                                )
                              }
                            >
                              {enabled ? '停用' : '启用'}
                            </button>
                          ) : null}
                          <button
                            type="button"
                            onClick={() => {
                              setEditingTask(task);
                              setFormOpen(true);
                            }}
                          >
                            编辑
                          </button>
                          <button
                            type="button"
                            onClick={() => setRecordsTask(task)}
                          >
                            执行记录
                          </button>
                          <button
                            type="button"
                            className="task-center-table__danger"
                            onClick={() => setDeletingTask(task)}
                          >
                            删除
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}

        {!loading && !error && total > 0 ? (
          <footer className="task-center-panel__footer">
            <span>共 {total} 项任务</span>
            <div className="task-center-panel__paging">
              <label>
                每页
                <select
                  aria-label="每页任务数"
                  value={pageSize}
                  onChange={event => {
                    setPageSize(Number(event.target.value));
                    setPage(1);
                  }}
                >
                  <option value={10}>10</option>
                  <option value={20}>20</option>
                  <option value={50}>50</option>
                </select>
              </label>
              <div className="task-center-pagination">
                <button
                  type="button"
                  disabled={page <= 1}
                  onClick={() => setPage(current => current - 1)}
                >
                  上一页
                </button>
                <span>
                  {page} / {pages}
                </span>
                <button
                  type="button"
                  disabled={page >= pages}
                  onClick={() => setPage(current => current + 1)}
                >
                  下一页
                </button>
              </div>
            </div>
          </footer>
        ) : null}
      </section>

      <TaskFormModal
        open={formOpen}
        spaceID={spaceID}
        task={editingTask}
        saving={saving}
        onCancel={() => {
          if (!saving) {
            setFormOpen(false);
            setEditingTask(undefined);
          }
        }}
        onSubmit={handleSave}
      />
      <ExecutionRecordsModal
        open={Boolean(recordsTask)}
        spaceID={spaceID}
        task={recordsTask}
        onCancel={() => setRecordsTask(undefined)}
      />
      <Modal
        visible={Boolean(deletingTask)}
        title="删除定时任务"
        okText={actionTaskID ? '正在删除...' : '确认删除'}
        cancelText="取消"
        okButtonProps={{ disabled: Boolean(actionTaskID) }}
        cancelButtonProps={{ disabled: Boolean(actionTaskID) }}
        onOk={() => void handleDelete()}
        onCancel={() => setDeletingTask(undefined)}
      >
        <div className="task-center-delete-confirm">
          <strong>{deletingTask?.name}</strong>
          <p>
            删除后任务及页面内执行记录将不可恢复。已产生的底层审计数据仍按系统安全策略保留。
          </p>
        </div>
      </Modal>
    </main>
  );
};

export default TaskCenterPage;
