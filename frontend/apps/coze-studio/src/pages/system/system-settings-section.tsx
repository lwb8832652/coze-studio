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

/* eslint-disable max-lines -- Site and runtime settings intentionally share the existing revisioned system form. */

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */

import { useEffect, useMemo, useRef, useState } from 'react';

import { useCommonConfigStore } from '@coze-foundation/global-store';

import { verifySiteAssetPreview } from './site-asset-preview';
import { uploadAdminSiteAsset } from './service';
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
  const publicSiteConfig = useCommonConfigStore(state => state.siteConfig);
  const publicSiteConfigRef = useRef(publicSiteConfig);
  const [serverHost, setServerHost] = useState('');
  const [siteName, setSiteName] = useState('');
  const [siteDescription, setSiteDescription] = useState('');
  const [siteLogoURI, setSiteLogoURI] = useState('');
  const [faviconURI, setFaviconURI] = useState('');
  const [siteLogoPreview, setSiteLogoPreview] = useState('');
  const [faviconPreview, setFaviconPreview] = useState('');
  const [uploadingAsset, setUploadingAsset] = useState<'logo' | 'favicon' | ''>(
    '',
  );
  const [adminEmails, setAdminEmails] = useState('');
  const [allowRegistrationEmail, setAllowRegistrationEmail] = useState('');
  const [disableUserRegistration, setDisableUserRegistration] = useState(false);
  const [validationMessage, setValidationMessage] = useState('');

  useEffect(() => {
    publicSiteConfigRef.current = publicSiteConfig;
  }, [publicSiteConfig]);

  useEffect(() => {
    if (!basicConfig) {
      return;
    }
    const currentPublicSiteConfig = publicSiteConfigRef.current;
    setServerHost(basicConfig.server_host || '');
    setSiteName(basicConfig.site_name || 'NewX AI');
    setSiteDescription(basicConfig.site_description || '');
    setSiteLogoURI(basicConfig.site_logo_uri || '');
    setFaviconURI(basicConfig.favicon_uri || '');
    setSiteLogoPreview(
      basicConfig.site_logo_uri ? currentPublicSiteConfig.siteLogoUrl : '',
    );
    setFaviconPreview(
      basicConfig.favicon_uri ? currentPublicSiteConfig.faviconUrl : '',
    );
    setAdminEmails(basicConfig.admin_emails || '');
    setAllowRegistrationEmail(basicConfig.allow_registration_email || '');
    setDisableUserRegistration(Boolean(basicConfig.disable_user_registration));
  }, [basicConfig]);

  useEffect(() => {
    if (!basicConfig) {
      return;
    }
    if (siteLogoURI === (basicConfig.site_logo_uri || '')) {
      setSiteLogoPreview(
        basicConfig.site_logo_uri ? publicSiteConfig.siteLogoUrl : '',
      );
    }
    if (faviconURI === (basicConfig.favicon_uri || '')) {
      setFaviconPreview(
        basicConfig.favicon_uri ? publicSiteConfig.faviconUrl : '',
      );
    }
  }, [
    basicConfig,
    faviconURI,
    publicSiteConfig.faviconUrl,
    publicSiteConfig.siteLogoUrl,
    siteLogoURI,
  ]);

  const normalizedCurrent = useMemo(
    () => ({
      adminEmails: (basicConfig?.admin_emails || '').trim(),
      allowRegistrationEmail: (
        basicConfig?.allow_registration_email || ''
      ).trim(),
      disableUserRegistration: Boolean(basicConfig?.disable_user_registration),
      serverHost: (basicConfig?.server_host || '').trim(),
      siteName: (basicConfig?.site_name || 'NewX AI').trim(),
      siteDescription: (basicConfig?.site_description || '').trim(),
      siteLogoURI: (basicConfig?.site_logo_uri || '').trim(),
      faviconURI: (basicConfig?.favicon_uri || '').trim(),
    }),
    [basicConfig],
  );

  const siteConfigDirty =
    serverHost.trim() !== normalizedCurrent.serverHost ||
    siteName.trim() !== normalizedCurrent.siteName ||
    siteDescription.trim() !== normalizedCurrent.siteDescription ||
    siteLogoURI.trim() !== normalizedCurrent.siteLogoURI ||
    faviconURI.trim() !== normalizedCurrent.faviconURI;
  const systemBasicConfigDirty =
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

  const submitBasicConfig = async (scope: 'site' | 'system') => {
    const normalizedServerHost = serverHost.trim();
    const normalizedSiteName = siteName.trim();
    const normalizedSiteDescription = siteDescription.trim();
    if (scope === 'site' && !normalizedSiteName) {
      setValidationMessage('站点名称不能为空');
      return;
    }
    if (scope === 'site' && normalizedSiteName.length > 64) {
      setValidationMessage('站点名称不能超过 64 个字符');
      return;
    }
    if (scope === 'site' && normalizedSiteDescription.length > 240) {
      setValidationMessage('站点介绍不能超过 240 个字符');
      return;
    }
    if (
      scope === 'site' &&
      normalizedServerHost !== normalizedCurrent.serverHost &&
      !normalizedServerHost
    ) {
      setValidationMessage('服务地址不能为空');
      return;
    }

    setValidationMessage('');
    const patch: AdminBasicConfigPatch = {};
    if (scope === 'site') {
      if (normalizedServerHost !== normalizedCurrent.serverHost) {
        patch.server_host = normalizedServerHost;
      }
      if (normalizedSiteName !== normalizedCurrent.siteName) {
        patch.site_name = normalizedSiteName;
      }
      if (normalizedSiteDescription !== normalizedCurrent.siteDescription) {
        patch.site_description = normalizedSiteDescription;
      }
      if (siteLogoURI.trim() !== normalizedCurrent.siteLogoURI) {
        patch.site_logo_uri = siteLogoURI.trim();
      }
      if (faviconURI.trim() !== normalizedCurrent.faviconURI) {
        patch.favicon_uri = faviconURI.trim();
      }
    } else {
      if (adminEmails.trim() !== normalizedCurrent.adminEmails) {
        patch.admin_emails = adminEmails.trim();
      }
      if (
        allowRegistrationEmail.trim() !==
        normalizedCurrent.allowRegistrationEmail
      ) {
        patch.allow_registration_email = allowRegistrationEmail.trim();
      }
      if (
        disableUserRegistration !== normalizedCurrent.disableUserRegistration
      ) {
        patch.disable_user_registration = disableUserRegistration;
      }
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

  const uploadSiteAsset = async (kind: 'logo' | 'favicon', file?: File) => {
    if (!file) {
      return;
    }
    setValidationMessage('');
    setUploadingAsset(kind);
    try {
      const asset = await uploadAdminSiteAsset(kind, file);
      try {
        await verifySiteAssetPreview(asset.url);
      } catch (error) {
        void error;
        setValidationMessage('上传资源不可访问，请重试或检查对象存储');
        return;
      }
      if (kind === 'logo') {
        setSiteLogoURI(asset.uri);
        setSiteLogoPreview(asset.url);
      } else {
        setFaviconURI(asset.uri);
        setFaviconPreview(asset.url);
      }
    } catch (error) {
      void error;
      setValidationMessage(
        kind === 'logo'
          ? '站点 Logo 上传失败，请使用 32-2048px 的 PNG、JPEG 或 WebP 图片'
          : '浏览器图标上传失败，请使用 16-512px 的正方形 PNG、JPEG 或 WebP 图片',
      );
    } finally {
      setUploadingAsset('');
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
      <article className="coze-prototype-workspace-settings-row coze-prototype-site-settings-card">
        <div className="w-full">
          <div className="coze-prototype-site-settings-header">
            <div>
              <h2>站点配置</h2>
              <p>
                配置用户实际可见的产品名称、访问地址和品牌资源。保存后会同步到浏览器标题、登录注册页和工作区。
              </p>
            </div>
            <span className="coze-prototype-site-settings-badge">全局生效</span>
          </div>
          {validationMessage ? (
            <p className="mt-[8px] text-[12px] text-[#d53b3e]" role="alert">
              {validationMessage}
            </p>
          ) : null}
          <div className="coze-prototype-site-settings-field-grid">
            <label className="coze-prototype-site-settings-field">
              站点名称
              <input
                aria-label="站点名称"
                className="h-[38px] rounded-[8px] border border-[#d8dde8] px-[11px] text-[13px] outline-none transition focus:border-[#24904b] focus:ring-2 focus:ring-[#24904b]/15"
                maxLength={64}
                placeholder="NewX AI"
                value={siteName}
                onChange={event => setSiteName(event.target.value)}
              />
            </label>
            <label className="coze-prototype-site-settings-field">
              站点访问地址
              <input
                aria-label="站点访问地址"
                className="h-[38px] rounded-[8px] border border-[#d8dde8] px-[11px] text-[13px] outline-none transition focus:border-[#24904b] focus:ring-2 focus:ring-[#24904b]/15"
                placeholder="http://localhost:8888"
                value={serverHost}
                onChange={event => setServerHost(event.target.value)}
              />
            </label>
          </div>
          <label className="coze-prototype-site-settings-field coze-prototype-site-settings-description">
            站点介绍
            <textarea
              aria-label="站点介绍"
              className="min-h-[86px] w-full resize-y rounded-[8px] border border-[#d8dde8] px-[11px] py-[9px] text-[13px] outline-none transition focus:border-[#24904b] focus:ring-2 focus:ring-[#24904b]/15"
              maxLength={240}
              placeholder="说明站点面向的用户和核心能力"
              value={siteDescription}
              onChange={event => setSiteDescription(event.target.value)}
            />
            <span className="self-end text-[11px] text-[#8b95a7]">
              {siteDescription.length}/240
            </span>
          </label>
          <div className="coze-prototype-site-settings-assets">
            {[
              {
                kind: 'logo' as const,
                title: '站点 Logo',
                hint: 'PNG、JPEG 或 WebP，32-2048px，最大 2MB',
                preview: siteLogoPreview,
                configured: Boolean(siteLogoURI.trim()),
              },
              {
                kind: 'favicon' as const,
                title: '浏览器地址栏图标',
                hint: '正方形 PNG、JPEG 或 WebP，16-512px，最大 512KB',
                preview: faviconPreview,
                configured: Boolean(faviconURI.trim()),
              },
            ].map(asset => (
              <div
                key={asset.kind}
                className="coze-prototype-site-settings-asset"
              >
                <div className="coze-prototype-site-settings-asset-copy">
                  <strong>{asset.title}</strong>
                  <span>{asset.hint}</span>
                </div>
                <div className="coze-prototype-site-settings-asset-body">
                  <div className="coze-prototype-site-settings-preview">
                    {asset.preview ? (
                      <img
                        alt={asset.title}
                        src={asset.preview}
                        onError={() => {
                          if (asset.kind === 'logo') {
                            setSiteLogoPreview('');
                          } else {
                            setFaviconPreview('');
                          }
                          setValidationMessage('资源不可用，请重新上传');
                        }}
                      />
                    ) : (
                      <span>
                        {asset.configured ? '资源不可用，请重新上传' : '未配置'}
                      </span>
                    )}
                  </div>
                  <div className="coze-prototype-site-settings-asset-actions">
                    <label className="coze-prototype-site-settings-upload">
                      <input
                        accept="image/png,image/jpeg,image/webp"
                        aria-label={`上传${asset.title}`}
                        className="hidden"
                        disabled={Boolean(uploadingAsset)}
                        type="file"
                        onChange={event => {
                          const file = event.currentTarget.files?.[0];
                          event.currentTarget.value = '';
                          void uploadSiteAsset(asset.kind, file);
                        }}
                      />
                      {uploadingAsset === asset.kind
                        ? '上传中...'
                        : asset.preview
                          ? '替换'
                          : asset.configured
                            ? '重新上传'
                            : '上传'}
                    </label>
                    {asset.configured ? (
                      <button
                        className="coze-prototype-site-settings-remove"
                        type="button"
                        onClick={() => {
                          if (asset.kind === 'logo') {
                            setSiteLogoURI('');
                            setSiteLogoPreview('');
                          } else {
                            setFaviconURI('');
                            setFaviconPreview('');
                          }
                        }}
                      >
                        移除
                      </button>
                    ) : null}
                  </div>
                </div>
              </div>
            ))}
          </div>
          <div className="coze-prototype-site-settings-footer">
            <p>仅保存当前卡片的变更，品牌图片会安全存储并按需生成访问地址。</p>
            <button
              aria-label="保存站点配置"
              className="coze-prototype-site-settings-save"
              disabled={
                basicConfigSaving ||
                Boolean(uploadingAsset) ||
                !onSaveBasicConfig ||
                !siteConfigDirty
              }
              type="button"
              onClick={() => void submitBasicConfig('site')}
            >
              {basicConfigSaving ? '保存中...' : '保存配置'}
            </button>
          </div>
        </div>
      </article>

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
              basicConfigSaving || !onSaveBasicConfig || !systemBasicConfigDirty
            }
            type="button"
            onClick={() => void submitBasicConfig('system')}
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
