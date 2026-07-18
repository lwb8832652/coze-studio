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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */

import { useEffect, useMemo, useState } from 'react';

import type {
  AdminBasicConfig,
  AdminBasicConfigPatch,
  AdminKnowledgeConfig,
} from './service';

interface SystemSettingsSectionProps {
  basicConfig: AdminBasicConfig | null;
  basicConfigLoading?: boolean;
  basicConfigLoadError?: string;
  basicConfigRefreshRequired?: boolean;
  basicConfigSaving?: boolean;
  basicConfigMessage?: string;
  knowledgeConfig: AdminKnowledgeConfig;
  onReloadBasicConfig?: () => void | Promise<void>;
  onSaveBasicConfig?: (
    configuration: AdminBasicConfigPatch,
  ) => void | Promise<void>;
}

export const SystemSettingsSection = ({
  basicConfig,
  basicConfigLoading = false,
  basicConfigLoadError = '',
  basicConfigRefreshRequired = false,
  basicConfigSaving = false,
  basicConfigMessage = '',
  knowledgeConfig,
  onReloadBasicConfig,
  onSaveBasicConfig,
}: SystemSettingsSectionProps) => {
  const [serverHost, setServerHost] = useState('');
  const [adminEmails, setAdminEmails] = useState('');
  const [allowRegistrationEmail, setAllowRegistrationEmail] = useState('');
  const [disableUserRegistration, setDisableUserRegistration] = useState(false);
  const [validationMessage, setValidationMessage] = useState('');

  useEffect(() => {
    if (!basicConfig) {
      return;
    }
    setServerHost(basicConfig.server_host || '');
    setAdminEmails(basicConfig.admin_emails || '');
    setAllowRegistrationEmail(basicConfig.allow_registration_email || '');
    setDisableUserRegistration(Boolean(basicConfig.disable_user_registration));
  }, [basicConfig]);

  const normalizedCurrent = useMemo(
    () => ({
      adminEmails: (basicConfig?.admin_emails || '').trim(),
      allowRegistrationEmail: (
        basicConfig?.allow_registration_email || ''
      ).trim(),
      disableUserRegistration: Boolean(basicConfig?.disable_user_registration),
      serverHost: (basicConfig?.server_host || '').trim(),
    }),
    [basicConfig],
  );

  const basicConfigDirty =
    serverHost.trim() !== normalizedCurrent.serverHost ||
    adminEmails.trim() !== normalizedCurrent.adminEmails ||
    allowRegistrationEmail.trim() !==
      normalizedCurrent.allowRegistrationEmail ||
    disableUserRegistration !== normalizedCurrent.disableUserRegistration;

  const configuredKnowledgeItems = useMemo(
    () =>
      [
        knowledgeConfig.embedding_config,
        knowledgeConfig.rerank_config,
        knowledgeConfig.ocr_config,
        knowledgeConfig.parser_config,
      ].filter(Boolean).length,
    [knowledgeConfig],
  );

  const submitBasicConfig = async () => {
    const normalizedServerHost = serverHost.trim();
    if (
      normalizedServerHost !== normalizedCurrent.serverHost &&
      !normalizedServerHost
    ) {
      setValidationMessage('服务地址不能为空');
      return;
    }

    setValidationMessage('');
    const patch: AdminBasicConfigPatch = {};
    if (normalizedServerHost !== normalizedCurrent.serverHost) {
      patch.server_host = normalizedServerHost;
    }
    if (adminEmails.trim() !== normalizedCurrent.adminEmails) {
      patch.admin_emails = adminEmails.trim();
    }
    if (
      allowRegistrationEmail.trim() !== normalizedCurrent.allowRegistrationEmail
    ) {
      patch.allow_registration_email = allowRegistrationEmail.trim();
    }
    if (disableUserRegistration !== normalizedCurrent.disableUserRegistration) {
      patch.disable_user_registration = disableUserRegistration;
    }
    if (Object.keys(patch).length === 0) {
      setValidationMessage('未检测到需要保存的变更');
      return;
    }
    try {
      await onSaveBasicConfig?.(patch);
    } catch (error) {
      void error;
      // Parent component surfaces the request error as basicConfigMessage.
    }
  };

  if (basicConfigLoading) {
    return (
      <section className="coze-prototype-workspace-settings-list">
        <article
          className="coze-prototype-workspace-settings-row"
          role="status"
        >
          <div>
            <h2>系统基础配置</h2>
            <p>正在加载系统基础配置...</p>
          </div>
          <span>加载中</span>
        </article>
      </section>
    );
  }

  if (basicConfigLoadError || !basicConfig) {
    return (
      <section className="coze-prototype-workspace-settings-list">
        <article className="coze-prototype-workspace-settings-row" role="alert">
          <div>
            <h2>系统基础配置加载失败</h2>
            <p>{basicConfigLoadError || '系统基础配置不可用'}</p>
            <button
              aria-label="重试加载系统基础配置"
              className="mt-[10px] h-[34px] rounded-[8px] bg-[#1d2333] px-[14px] text-[13px] text-white disabled:opacity-60"
              disabled={!onReloadBasicConfig}
              type="button"
              onClick={() => void onReloadBasicConfig?.()}
            >
              重新加载
            </button>
          </div>
          <span>加载失败</span>
        </article>
      </section>
    );
  }

  return (
    <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>配置总览</h2>
          <p>集中查看并维护后台基础配置，知识库能力保持安全状态展示。</p>
          {basicConfigMessage ? <p>{basicConfigMessage}</p> : null}
          {basicConfigRefreshRequired && onReloadBasicConfig ? (
            <button
              aria-label="刷新系统基础配置"
              className="mt-[8px] h-[32px] rounded-[8px] border border-[#4d75e8] px-[12px] text-[12px] font-medium text-[#3159c9]"
              type="button"
              onClick={() => void onReloadBasicConfig()}
            >
              刷新配置
            </button>
          ) : null}
          {validationMessage ? <p>{validationMessage}</p> : null}
          <div className="mt-[12px] grid gap-[10px] md:grid-cols-3">
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">服务地址</p>
              <strong className="text-[15px] text-[#1d2333]">
                {basicConfig.server_host ? '已配置' : '未配置'}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">注册策略</p>
              <strong className="text-[15px] text-[#1d2333]">
                {basicConfig.disable_user_registration ? '已关闭' : '允许注册'}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">知识库能力</p>
              <strong className="text-[15px] text-[#1d2333]">
                {configuredKnowledgeItems} / 4 已配置
              </strong>
            </div>
          </div>
        </div>
        <span>系统配置</span>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <h2>基础配置</h2>
          <p>
            保存服务地址、注册策略和管理员白名单。仅提交本次变更字段，并使用版本号避免并发覆盖。
          </p>
          <div className="mt-[12px] grid gap-[10px] md:grid-cols-2">
            <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
              服务地址
              <input
                aria-label="系统服务地址"
                className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                placeholder="http://localhost:8888"
                value={serverHost}
                onChange={event => setServerHost(event.target.value)}
              />
            </label>
            <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
              管理员邮箱
              <input
                aria-label="系统管理员邮箱"
                className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                placeholder="admin@example.com，多个邮箱用英文逗号分隔"
                value={adminEmails}
                onChange={event => setAdminEmails(event.target.value)}
              />
            </label>
            <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
              允许注册邮箱
              <input
                aria-label="允许注册邮箱"
                className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                placeholder="留空表示不限制邮箱白名单"
                value={allowRegistrationEmail}
                onChange={event =>
                  setAllowRegistrationEmail(event.target.value)
                }
              />
            </label>
            <div
              className="rounded-[12px] border border-[#d8e4fb] bg-[#f4f8ff] px-[14px] py-[12px] md:col-span-2"
              role="status"
            >
              <p className="m-0 text-[13px] font-medium text-[#1d2333]">
                配置已迁移到 Sandbox 管理
              </p>
              <p className="mb-0 mt-[4px] text-[12px] text-[#687385]">
                旧代码运行器配置仅作为只读迁移来源，不再从系统基础配置写入。
              </p>
              <a
                className="mt-[8px] inline-flex h-[30px] items-center rounded-[8px] border border-[#4d75e8] px-[10px] text-[12px] font-medium text-[#3159c9] no-underline"
                href="/system/sandbox"
              >
                前往 Sandbox 管理
              </a>
            </div>
            <label className="flex items-center gap-[8px] text-[13px] text-[#4d566a]">
              <input
                aria-label="关闭用户注册"
                checked={disableUserRegistration}
                type="checkbox"
                onChange={event =>
                  setDisableUserRegistration(event.target.checked)
                }
              />
              关闭用户注册
            </label>
          </div>
          <button
            aria-label="保存系统基础配置"
            className="mt-[12px] h-[34px] rounded-[8px] bg-[#1d2333] px-[14px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
            disabled={
              basicConfigSaving || !onSaveBasicConfig || !basicConfigDirty
            }
            type="button"
            onClick={() => void submitBasicConfig()}
          >
            {basicConfigSaving ? '保存中...' : '保存基础配置'}
          </button>
        </div>
        <span>可保存</span>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>知识库配置</h2>
          <p>
            内置模型 ID：
            {knowledgeConfig.builtin_model_id ?? '未配置'}
          </p>
          <p>
            Embedding：
            {knowledgeConfig.embedding_config ? '已配置' : '未配置'}
          </p>
          <p>
            Rerank：
            {knowledgeConfig.rerank_config ? '已配置' : '未配置'}
          </p>
          <p>
            OCR：
            {knowledgeConfig.ocr_config ? '已配置' : '未配置'}
          </p>
          <p>
            Parser：
            {knowledgeConfig.parser_config ? '已配置' : '未配置'}
          </p>
        </div>
        <span>安全展示</span>
      </article>
    </section>
  );
};
