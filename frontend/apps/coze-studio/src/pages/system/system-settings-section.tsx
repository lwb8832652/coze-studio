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

import type { AdminBasicConfig, AdminKnowledgeConfig } from './service';

interface SystemSettingsSectionProps {
  basicConfig: AdminBasicConfig;
  basicConfigSaving?: boolean;
  basicConfigMessage?: string;
  knowledgeConfig: AdminKnowledgeConfig;
  onSaveBasicConfig?: (configuration: AdminBasicConfig) => void | Promise<void>;
}

export const SystemSettingsSection = ({
  basicConfig,
  basicConfigSaving = false,
  basicConfigMessage = '',
  knowledgeConfig,
  onSaveBasicConfig,
}: SystemSettingsSectionProps) => {
  const [serverHost, setServerHost] = useState('');
  const [adminEmails, setAdminEmails] = useState('');
  const [allowRegistrationEmail, setAllowRegistrationEmail] = useState('');
  const [disableUserRegistration, setDisableUserRegistration] = useState(false);
  const [codeRunnerType, setCodeRunnerType] = useState('0');
  const [validationMessage, setValidationMessage] = useState('');

  useEffect(() => {
    setServerHost(basicConfig.server_host || '');
    setAdminEmails(basicConfig.admin_emails || '');
    setAllowRegistrationEmail(basicConfig.allow_registration_email || '');
    setDisableUserRegistration(Boolean(basicConfig.disable_user_registration));
    setCodeRunnerType(String(basicConfig.code_runner_type ?? 0));
  }, [basicConfig]);

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
    if (!normalizedServerHost) {
      setValidationMessage('服务地址不能为空');
      return;
    }

    setValidationMessage('');
    try {
      await onSaveBasicConfig?.({
        admin_emails: adminEmails.trim(),
        allow_registration_email: allowRegistrationEmail.trim(),
        code_runner_type: Number(codeRunnerType),
        disable_user_registration: disableUserRegistration,
        server_host: normalizedServerHost,
      });
    } catch (error) {
      void error;
      // Parent component surfaces the request error as basicConfigMessage.
    }
  };

  return (
    <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>配置总览</h2>
          <p>集中查看并维护后台基础配置，知识库能力保持安全状态展示。</p>
          {basicConfigMessage ? <p>{basicConfigMessage}</p> : null}
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
            保存服务地址、注册策略和管理员白名单。保存时会复用当前完整配置，避免覆盖未知字段。
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
            <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
              代码执行环境
              <select
                aria-label="代码执行环境"
                className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                value={codeRunnerType}
                onChange={event => setCodeRunnerType(event.target.value)}
              >
                <option value="0">本地执行</option>
                <option value="1">沙盒执行</option>
              </select>
            </label>
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
            disabled={basicConfigSaving || !onSaveBasicConfig}
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
