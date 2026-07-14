// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @coze-arch/max-line-per-function -- The modal keeps loading, result states, and pagination together. */

import { useEffect, useState } from 'react';

import { Modal } from '@coze-arch/coze-design';

import { formatExecutionStatus, formatTimestamp } from './utils';
import {
  listScheduledTaskExecutions,
  type ScheduledTask,
  type ScheduledTaskExecution,
} from './service';

const PAGE_SIZE = 10;

interface ExecutionRecordsModalProps {
  open: boolean;
  spaceID: string;
  task?: ScheduledTask;
  onCancel: () => void;
}

const ExecutionRecordsModal = ({
  open,
  spaceID,
  task,
  onCancel,
}: ExecutionRecordsModalProps) => {
  const [executions, setExecutions] = useState<ScheduledTaskExecution[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    if (!open || !task) {
      return;
    }
    setLoading(true);
    setError('');
    try {
      const response = await listScheduledTaskExecutions({
        task_id: task.id,
        space_id: spaceID,
        page,
        page_size: PAGE_SIZE,
      });
      setExecutions(response.data?.executions || []);
      setTotal(response.data?.total || 0);
    } catch {
      setError('执行记录加载失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      setPage(1);
    }
  }, [open, task?.id]);

  useEffect(() => {
    void load();
  }, [open, task?.id, page]);

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <Modal
      visible={open}
      width={720}
      title={`执行记录${task ? ` · ${task.name}` : ''}`}
      footer={null}
      onCancel={onCancel}
      className="task-center-record-modal"
    >
      <div className="task-center-records">
        <header>
          <div>
            <strong>{total}</strong>
            <span>次执行</span>
          </div>
          <button type="button" disabled={loading} onClick={() => void load()}>
            刷新
          </button>
        </header>
        {loading ? (
          <div className="task-center-records__state">正在加载执行记录...</div>
        ) : null}
        {!loading && error ? (
          <div className="task-center-records__state task-center-records__state--error">
            {error}
          </div>
        ) : null}
        {!loading && !error && !executions.length ? (
          <div className="task-center-records__empty">
            <span aria-hidden="true">01</span>
            <strong>暂无执行记录</strong>
            <p>任务触发或手动执行后，运行结果会出现在这里。</p>
          </div>
        ) : null}
        {!loading && !error && executions.length ? (
          <div className="task-center-record-list">
            {executions.map(execution => (
              <article key={execution.id}>
                <div
                  className="task-center-record-list__status"
                  data-status={execution.status}
                />
                <div className="task-center-record-list__body">
                  <div className="task-center-record-list__title">
                    <strong>{formatExecutionStatus(execution.status)}</strong>
                    <span>
                      {execution.trigger_type === 'manual'
                        ? '手动触发'
                        : '定时触发'}
                    </span>
                    <time>{formatTimestamp(execution.created_at)}</time>
                  </div>
                  <div className="task-center-record-list__meta">
                    <span>
                      计划时间 {formatTimestamp(execution.scheduled_at)}
                    </span>
                    {execution.started_at ? (
                      <span>开始 {formatTimestamp(execution.started_at)}</span>
                    ) : null}
                    {execution.finished_at ? (
                      <span>完成 {formatTimestamp(execution.finished_at)}</span>
                    ) : null}
                  </div>
                  {execution.error_message ? (
                    <p className="task-center-record-list__error">
                      {execution.error_message}
                    </p>
                  ) : null}
                  <footer>
                    <code>#{execution.id}</code>
                    {execution.thread_id ? (
                      <a
                        href={`/space/${spaceID}/tasks/${execution.thread_id}`}
                      >
                        查看会话
                      </a>
                    ) : null}
                    {execution.workflow_execution_id ? (
                      <span>工作流执行 #{execution.workflow_execution_id}</span>
                    ) : null}
                  </footer>
                </div>
              </article>
            ))}
          </div>
        ) : null}
        {pages > 1 ? (
          <footer className="task-center-pagination">
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
          </footer>
        ) : null}
      </div>
    </Modal>
  );
};

export default ExecutionRecordsModal;
