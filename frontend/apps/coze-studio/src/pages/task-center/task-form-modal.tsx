// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- The form keeps dependent schedule fields together. */

import { useEffect, useMemo, useState } from 'react';

import { workbenchTask } from '@coze-studio/api-schema';
import { Modal } from '@coze-arch/coze-design';

import {
  formatScheduleSummary,
  parseTaskPayload,
  toLocalDateTimeInput,
  validateTaskForm,
} from './utils';
import type { TaskFormValues } from './types';
import {
  listScheduledTaskCronPresets,
  listScheduledTaskTargets,
  type ScheduledTask,
  type ScheduledTaskCronPreset,
  type ScheduledTaskTarget,
} from './service';

const browserTimezone =
  Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai';

const TIMEZONES = Array.from(
  new Set([
    browserTimezone,
    'Asia/Shanghai',
    'Asia/Hong_Kong',
    'Asia/Tokyo',
    'Europe/London',
    'America/New_York',
    'UTC',
  ]),
);

const initialValues = (task?: ScheduledTask): TaskFormValues => {
  const payload = task
    ? parseTaskPayload(task)
    : { message: '', agentVariables: '{}', workflowInputs: '{}' };
  return {
    name: task?.name || '',
    targetType:
      task?.target_type || workbenchTask.ScheduledTaskTargetType.Agent,
    targetID: task?.target_id || '',
    scheduleType:
      task?.schedule_type || workbenchTask.ScheduledTaskScheduleType.Daily,
    timezone: task?.timezone || browserTimezone,
    runOnceAt: toLocalDateTimeInput(task?.run_once_at),
    minute: task?.minute || 0,
    hour: task?.hour ?? 9,
    weekday: task?.weekday ?? 1,
    cronExpr: task?.cron_expr || '',
    message: payload.message,
    agentVariables: payload.agentVariables,
    workflowInputs: payload.workflowInputs,
    keepConversation: task?.keep_conversation ?? false,
    maxExecutions: task?.max_executions || 0,
  };
};

interface TaskFormModalProps {
  open: boolean;
  spaceID: string;
  task?: ScheduledTask;
  saving: boolean;
  onCancel: () => void;
  onSubmit: (values: TaskFormValues) => Promise<void>;
}

const TaskFormModal = ({
  open,
  spaceID,
  task,
  saving,
  onCancel,
  onSubmit,
}: TaskFormModalProps) => {
  const [values, setValues] = useState<TaskFormValues>(() =>
    initialValues(task),
  );
  const [targets, setTargets] = useState<ScheduledTaskTarget[]>([]);
  const [presets, setPresets] = useState<ScheduledTaskCronPreset[]>([]);
  const [targetKeyword, setTargetKeyword] = useState('');
  const [targetLoading, setTargetLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');

  useEffect(() => {
    if (!open) {
      return;
    }
    setValues(initialValues(task));
    setTargetKeyword('');
    setErrorMessage('');
    void listScheduledTaskCronPresets({ timezone: browserTimezone })
      .then(response => setPresets(response.data || []))
      .catch(() => setPresets([]));
  }, [open, task]);

  useEffect(() => {
    if (!open || !spaceID) {
      return;
    }
    const timer = window.setTimeout(() => {
      setTargetLoading(true);
      void listScheduledTaskTargets({
        space_id: spaceID,
        target_type: values.targetType,
        keyword: targetKeyword.trim() || undefined,
        page: 1,
        page_size: 50,
      })
        .then(response => setTargets(response.data?.targets || []))
        .catch(() => setTargets([]))
        .finally(() => setTargetLoading(false));
    }, 180);
    return () => window.clearTimeout(timer);
  }, [open, spaceID, targetKeyword, values.targetType]);

  const selectedTarget = targets.find(target => target.id === values.targetID);
  const scheduleSummary = useMemo(
    () => formatScheduleSummary(values),
    [values],
  );

  const update = <K extends keyof TaskFormValues>(
    key: K,
    value: TaskFormValues[K],
  ) => setValues(current => ({ ...current, [key]: value }));

  const submit = async () => {
    const error = validateTaskForm(values);
    if (error) {
      setErrorMessage(error);
      return;
    }
    setErrorMessage('');
    await onSubmit(values);
  };

  return (
    <Modal
      visible={open}
      width={760}
      title={task ? '编辑定时任务' : '创建定时任务'}
      footer={null}
      onCancel={onCancel}
      className="task-center-form-modal"
      closeOnEsc={!saving}
      maskClosable={false}
    >
      <div className="task-center-form">
        <section className="task-center-form__section">
          <div className="task-center-form__section-heading">
            <span>01</span>
            <div>
              <h3>基本信息</h3>
              <p>为任务命名，并选择已发布的 Agent 或工作流。</p>
            </div>
          </div>
          <label className="task-center-field task-center-field--full">
            <span>任务名称</span>
            <input
              value={values.name}
              maxLength={100}
              placeholder="例如：每日行业简报"
              onChange={event => update('name', event.target.value)}
            />
            <small>{values.name.length}/100</small>
          </label>
          <div className="task-center-form__grid">
            <label className="task-center-field">
              <span>目标类型</span>
              <select
                value={values.targetType}
                onChange={event => {
                  update('targetType', Number(event.target.value));
                  update('targetID', '');
                }}
              >
                <option value={workbenchTask.ScheduledTaskTargetType.Agent}>
                  Agent
                </option>
                <option value={workbenchTask.ScheduledTaskTargetType.Workflow}>
                  工作流
                </option>
              </select>
            </label>
            <label className="task-center-field">
              <span>搜索目标</span>
              <input
                value={targetKeyword}
                placeholder="按名称搜索"
                onChange={event => setTargetKeyword(event.target.value)}
              />
            </label>
          </div>
          <label className="task-center-field task-center-field--full">
            <span className="task-center-field__label-row">
              <span>执行目标</span>
              <a
                href={`/space/${spaceID}/develop`}
                target="_blank"
                rel="noreferrer"
              >
                + 新建执行目标
              </a>
            </span>
            <select
              value={values.targetID}
              disabled={targetLoading}
              onChange={event => update('targetID', event.target.value)}
            >
              <option value="">
                {targetLoading ? '正在加载...' : '请选择已发布目标'}
              </option>
              {targets.map(target => (
                <option key={target.id} value={target.id}>
                  {target.name}
                </option>
              ))}
            </select>
            {selectedTarget?.input_schema ? (
              <small className="task-center-field__schema">
                目标参数约束：{selectedTarget.input_schema}
              </small>
            ) : null}
          </label>
        </section>

        <section className="task-center-form__section">
          <div className="task-center-form__section-heading">
            <span>02</span>
            <div>
              <h3>调度规则</h3>
              <p>按本地业务时区配置，服务端会统一计算下一次执行时间。</p>
            </div>
          </div>
          <div className="task-center-schedule-tabs" role="tablist">
            {[
              [workbenchTask.ScheduledTaskScheduleType.Once, '仅一次'],
              [workbenchTask.ScheduledTaskScheduleType.Hourly, '每小时'],
              [workbenchTask.ScheduledTaskScheduleType.Daily, '每天'],
              [workbenchTask.ScheduledTaskScheduleType.Weekly, '每周'],
              [workbenchTask.ScheduledTaskScheduleType.Cron, '自定义'],
            ].map(([type, label]) => (
              <button
                type="button"
                role="tab"
                aria-selected={values.scheduleType === type}
                data-active={values.scheduleType === type}
                key={type}
                onClick={() => update('scheduleType', Number(type))}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="task-center-form__grid task-center-form__grid--schedule">
            <label className="task-center-field">
              <span>时区</span>
              <select
                value={values.timezone}
                onChange={event => update('timezone', event.target.value)}
              >
                {TIMEZONES.map(timezone => (
                  <option key={timezone} value={timezone}>
                    {timezone}
                  </option>
                ))}
              </select>
            </label>
            {values.scheduleType ===
            workbenchTask.ScheduledTaskScheduleType.Once ? (
              <label className="task-center-field">
                <span>执行时间</span>
                <input
                  type="datetime-local"
                  value={values.runOnceAt}
                  min={toLocalDateTimeInput(Math.floor(Date.now() / 1000) + 60)}
                  onChange={event => update('runOnceAt', event.target.value)}
                />
              </label>
            ) : null}
            {values.scheduleType ===
            workbenchTask.ScheduledTaskScheduleType.Weekly ? (
              <label className="task-center-field">
                <span>星期</span>
                <select
                  value={values.weekday}
                  onChange={event =>
                    update('weekday', Number(event.target.value))
                  }
                >
                  {['周日', '周一', '周二', '周三', '周四', '周五', '周六'].map(
                    (label, value) => (
                      <option key={label} value={value}>
                        {label}
                      </option>
                    ),
                  )}
                </select>
              </label>
            ) : null}
            {[
              workbenchTask.ScheduledTaskScheduleType.Daily,
              workbenchTask.ScheduledTaskScheduleType.Weekly,
            ].includes(values.scheduleType) ? (
              <label className="task-center-field">
                <span>小时</span>
                <input
                  type="number"
                  min={0}
                  max={23}
                  value={values.hour}
                  onChange={event => update('hour', Number(event.target.value))}
                />
              </label>
            ) : null}
            {[
              workbenchTask.ScheduledTaskScheduleType.Hourly,
              workbenchTask.ScheduledTaskScheduleType.Daily,
              workbenchTask.ScheduledTaskScheduleType.Weekly,
            ].includes(values.scheduleType) ? (
              <label className="task-center-field">
                <span>分钟</span>
                <input
                  type="number"
                  min={0}
                  max={59}
                  value={values.minute}
                  onChange={event =>
                    update('minute', Number(event.target.value))
                  }
                />
              </label>
            ) : null}
          </div>
          {values.scheduleType ===
          workbenchTask.ScheduledTaskScheduleType.Cron ? (
            <div className="task-center-cron-editor">
              <label className="task-center-field task-center-field--full">
                <span>Cron 表达式</span>
                <input
                  value={values.cronExpr}
                  placeholder="分 时 日 月 周，例如 0 9 * * 1-5"
                  onChange={event => update('cronExpr', event.target.value)}
                />
              </label>
              <div className="task-center-cron-presets">
                {presets
                  .filter(
                    preset =>
                      preset.schedule_type ===
                      workbenchTask.ScheduledTaskScheduleType.Cron,
                  )
                  .map(preset => (
                    <button
                      type="button"
                      key={preset.id}
                      onClick={() => update('cronExpr', preset.cron_expr || '')}
                    >
                      {preset.label}
                    </button>
                  ))}
              </div>
            </div>
          ) : null}
          <div className="task-center-schedule-preview">
            <span>下一规则</span>
            <strong>{scheduleSummary}</strong>
            <small>{values.timezone}</small>
          </div>
        </section>

        <section className="task-center-form__section">
          <div className="task-center-form__section-heading">
            <span>03</span>
            <div>
              <h3>任务内容</h3>
              <p>输入会在服务端校验后传递给已发布目标。</p>
            </div>
          </div>
          {values.targetType === workbenchTask.ScheduledTaskTargetType.Agent ? (
            <>
              <label className="task-center-field task-center-field--full">
                <span>任务指令</span>
                <textarea
                  rows={5}
                  value={values.message}
                  placeholder="描述希望 Agent 定时完成的工作"
                  onChange={event => update('message', event.target.value)}
                />
              </label>
              <label className="task-center-field task-center-field--full">
                <span>变量（JSON，可选）</span>
                <textarea
                  rows={3}
                  spellCheck={false}
                  value={values.agentVariables}
                  onChange={event =>
                    update('agentVariables', event.target.value)
                  }
                />
              </label>
              <label className="task-center-check-field">
                <input
                  type="checkbox"
                  checked={values.keepConversation}
                  onChange={event =>
                    update('keepConversation', event.target.checked)
                  }
                />
                <span>
                  <strong>保持连续会话</strong>
                  <small>后续执行复用此任务专属会话，保留上下文。</small>
                </span>
              </label>
            </>
          ) : (
            <label className="task-center-field task-center-field--full">
              <span>工作流输入（JSON）</span>
              <textarea
                rows={7}
                spellCheck={false}
                value={values.workflowInputs}
                placeholder={'{\n  "city": "武汉"\n}'}
                onChange={event => update('workflowInputs', event.target.value)}
              />
            </label>
          )}
          {values.scheduleType !==
          workbenchTask.ScheduledTaskScheduleType.Once ? (
            <label className="task-center-field task-center-field--compact">
              <span>最大执行次数</span>
              <input
                type="number"
                min={0}
                value={values.maxExecutions}
                onChange={event =>
                  update('maxExecutions', Number(event.target.value))
                }
              />
              <small>填 0 表示不限制</small>
            </label>
          ) : null}
        </section>

        {errorMessage ? (
          <div role="alert" className="task-center-form__error">
            {errorMessage}
          </div>
        ) : null}
        <footer className="task-center-form__footer">
          <button type="button" onClick={onCancel} disabled={saving}>
            取消
          </button>
          <button
            type="button"
            className="task-center-primary-button"
            disabled={saving}
            onClick={() => void submit()}
          >
            {saving ? '正在保存...' : task ? '保存修改' : '创建并启用'}
          </button>
        </footer>
      </div>
    </Modal>
  );
};

export default TaskFormModal;
