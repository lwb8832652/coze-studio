/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { useCallback, useEffect, useMemo, useState } from 'react';

import { IconCozRefresh } from '@coze-arch/coze-design/icons';
import { Button, Spin, Tag } from '@coze-arch/coze-design';

import {
  getWorkbenchRuntimeDoctor,
  type RuntimeDoctorCheck,
  type WorkbenchRuntimeDoctorData,
} from './service';

const RUNTIME_DOCTOR_PENDING_MESSAGE = '后端深度检查待接入';

const sanitizeRuntimeDoctorText = (value?: string) => {
  const trimmed = value?.trim() ?? '';
  if (!trimmed) {
    return '';
  }

  if (
    /api[_-]?key|secret|credential|authorization|bearer|tool_arguments|checkpoint|provider_raw/i.test(
      trimmed,
    )
  ) {
    return '诊断信息已隐藏';
  }

  return trimmed;
};

const statusLabel = (status?: string) => {
  switch (status) {
    case 'ready':
      return '正常';
    case 'warning':
      return '需要关注';
    case 'error':
      return '异常';
    case 'disabled':
      return '未启用';
    case 'pending':
      return '待接入';
    default:
      return status || '未知';
  }
};

const statusColor = (status?: string) => {
  switch (status) {
    case 'ready':
      return 'green';
    case 'warning':
      return 'amber';
    case 'error':
      return 'red';
    case 'disabled':
      return 'grey';
    case 'pending':
      return 'light-blue';
    default:
      return 'grey';
  }
};

const findCheck = (
  checks: RuntimeDoctorCheck[],
  match: (check: RuntimeDoctorCheck) => boolean,
) => checks.find(check => match(check));

const deferredCheck = (name: string, category: string): RuntimeDoctorCheck => ({
  name,
  category,
  status: 'pending',
  message: RUNTIME_DOCTOR_PENDING_MESSAGE,
});

const RuntimeDoctorCard = ({
  detail,
  meta,
  status,
  title,
}: {
  detail?: string;
  meta?: string;
  status?: string;
  title: string;
}) => (
  <li className="coze-prototype-runtime-doctor-card" data-status={status}>
    <span className="coze-prototype-runtime-doctor-card-title-row">
      <span className="coze-prototype-runtime-doctor-card-title">{title}</span>
      <Tag color={statusColor(status)} size="small" type="light">
        {statusLabel(status)}
      </Tag>
    </span>
    {meta ? (
      <span className="coze-prototype-runtime-doctor-card-meta">{meta}</span>
    ) : null}
    {detail ? (
      <span className="coze-prototype-runtime-doctor-card-detail">
        {sanitizeRuntimeDoctorText(detail)}
      </span>
    ) : null}
  </li>
);

const RuntimeDoctorCheckRow = ({ check }: { check: RuntimeDoctorCheck }) => (
  <li
    className="coze-prototype-runtime-doctor-check-row"
    data-status={check.status}
  >
    <span className="coze-prototype-runtime-doctor-check-main">
      <span className="coze-prototype-runtime-doctor-check-title">
        {check.name}
      </span>
      <span className="coze-prototype-runtime-doctor-check-meta">
        {check.category}
      </span>
      {check.message ? (
        <span className="coze-prototype-runtime-doctor-check-detail">
          {sanitizeRuntimeDoctorText(check.message)}
        </span>
      ) : null}
    </span>
    <Tag color={statusColor(check.status)} size="small" type="light">
      {statusLabel(check.status)}
    </Tag>
  </li>
);

export const TaskRuntimeDoctorSection = ({
  spaceId,
}: {
  spaceId?: string;
}) => {
  const [data, setData] = useState<WorkbenchRuntimeDoctorData>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadRuntimeDoctor = useCallback(async () => {
    if (!spaceId) {
      setData(undefined);
      setError('');
      return;
    }

    setLoading(true);
    setError('');
    try {
      const response = await getWorkbenchRuntimeDoctor({ space_id: spaceId });
      setData(response.data);
    } catch (err) {
      setData(undefined);
      setError(err instanceof Error ? err.message : '加载运行诊断失败');
    } finally {
      setLoading(false);
    }
  }, [spaceId]);

  useEffect(() => {
    void loadRuntimeDoctor();
  }, [loadRuntimeDoctor]);

  const checks = data?.checks ?? [];
  const derivedChecks = useMemo(
    () => ({
      memory:
        findCheck(checks, check => check.category === 'memory') ??
        deferredCheck('memory.runtime_retrieval', 'memory'),
      model:
        findCheck(checks, check => check.category === 'model') ??
        deferredCheck('model.runtime_config', 'model'),
      skill:
        findCheck(checks, check =>
          ['skill', 'skills'].includes(check.category),
        ) ?? deferredCheck('skills.runtime_load', 'skills'),
    }),
    [checks],
  );

  if (!spaceId) {
    return null;
  }

  return (
    <section
      className="coze-prototype-runtime-doctor-panel"
      data-testid="task-runtime-doctor-panel"
    >
      <div className="coze-prototype-runtime-doctor-header">
        <h2>运行诊断</h2>
        <Tag color={statusColor(data?.status)} size="small" type="light">
          {statusLabel(data?.status)}
        </Tag>
        <Button
          aria-label="刷新运行诊断"
          icon={<IconCozRefresh />}
          loading={loading}
          size="small"
          theme="borderless"
          type="tertiary"
          onClick={() => void loadRuntimeDoctor()}
        >
          刷新
        </Button>
      </div>
      {error ? (
        <div className="coze-prototype-runtime-doctor-error" role="alert">
          {sanitizeRuntimeDoctorText(error)}
        </div>
      ) : null}
      <Spin spinning={loading}>
        {data ? (
          <>
            <ol className="coze-prototype-runtime-doctor-card-grid">
              <RuntimeDoctorCard
                title="Eino ADK"
                status={data.runtime.eino_adk_enabled ? 'ready' : 'disabled'}
                meta={
                  data.runtime.eino_adk_enabled ? '已启用' : '未启用'
                }
                detail={`默认模式 ${data.runtime.default_mode || '未配置'}`}
              />
              <RuntimeDoctorCard
                title="Web Fetch"
                status={data.web_tools.web_fetch.status}
                meta={data.web_tools.web_fetch.configured ? '已配置' : '未配置'}
                detail={data.web_tools.web_fetch.message}
              />
              <RuntimeDoctorCard
                title="Web Search"
                status={data.web_tools.web_search.status}
                meta={
                  data.web_tools.web_search.configured ? '已配置' : '未配置'
                }
                detail={data.web_tools.web_search.message}
              />
              <RuntimeDoctorCard
                title="MCP 工具"
                status={data.mcp_tools.status}
                meta={`总计 ${data.mcp_tools.total_servers} · 启用 ${data.mcp_tools.enabled_servers}`}
                detail={`健康 ${data.mcp_tools.healthy_servers} · 异常 ${data.mcp_tools.unhealthy_servers} · 未知 ${data.mcp_tools.unknown_servers}`}
              />
              <RuntimeDoctorCard
                title="模型配置"
                status={derivedChecks.model.status}
                detail={derivedChecks.model.message}
              />
              <RuntimeDoctorCard
                title="Skill 检查"
                status={derivedChecks.skill.status}
                detail={derivedChecks.skill.message}
              />
              <RuntimeDoctorCard
                title="记忆检查"
                status={derivedChecks.memory.status}
                detail={derivedChecks.memory.message}
              />
            </ol>
            {checks.length ? (
              <ol className="coze-prototype-runtime-doctor-check-list">
                {checks.map(check => (
                  <RuntimeDoctorCheckRow
                    key={`${check.category}:${check.name}`}
                    check={check}
                  />
                ))}
              </ol>
            ) : null}
          </>
        ) : (
          <div className="coze-prototype-runtime-doctor-empty">
            {loading ? '加载运行诊断中...' : '暂无运行诊断'}
          </div>
        )}
      </Spin>
    </section>
  );
};
