/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines */

import { useNavigate, useParams } from 'react-router-dom';
import {
  type ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { workbenchSkill } from '@coze-studio/api-schema';
import { useSpaceStore } from '@coze-foundation/space-store';
import {
  Button,
  Empty,
  Input,
  Modal,
  Spin,
  Toast,
} from '@coze-arch/coze-design';

import { createSkillCreationNavigationState } from '../workbench/skill-creation-intent';
import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import {
  createSkill,
  deleteSkill,
  exportSkill,
  exportSkillVersion,
  getSkill,
  importSkill,
  listSkills,
  listSkillVersionResources,
  listSkillVersions,
  rollbackSkillVersion,
  testRunSkill,
  updateSkill,
  updateSkillVersionContent,
  updateSkillVersionResource,
} from './service';

import './skill-management.less';

type Skill = workbenchSkill.Skill;
type SkillVersion = workbenchSkill.SkillVersion;
type SkillResource = workbenchSkill.SkillResource;

const PAGE_SIZE = 12;
const DIRECTORY_MARKER = '.coze-folder';
const ICON_TONES = ['lime', 'ocean', 'sunset', 'cobalt', 'rose', 'slate'];
export const SKILL_IMPORT_ACCEPT = '.skill,.md';

export const isSupportedSkillImportFile = (fileName: string) => {
  const lowerName = fileName.trim().toLowerCase();
  return lowerName.endsWith('.skill') || lowerName === 'skill.md';
};

const timestampToDate = (value: number) => {
  const timestamp = Number(value);
  return new Date(timestamp < 10_000_000_000 ? timestamp * 1000 : timestamp);
};

const errorText = (error: unknown, fallback: string) => {
  const apiMessage = (error as { response?: { data?: { msg?: unknown } } })
    ?.response?.data?.msg;
  const message =
    typeof apiMessage === 'string' && apiMessage.trim()
      ? apiMessage.trim()
      : error instanceof Error && error.message
        ? error.message
        : fallback;
  if (message.includes('does not support direct test run')) {
    return '当前技能类型不支持直接试运行，请在任务中启用该技能后验证';
  }
  return message;
};

const bytesToBase64 = (bytes: Uint8Array) => {
  let binary = '';
  bytes.forEach(byte => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary);
};

const textToBase64 = (value: string) =>
  bytesToBase64(new TextEncoder().encode(value));

const base64ToText = (value: string) => {
  const bytes = Uint8Array.from(atob(value), char => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
};

const downloadFile = (
  fileName: string,
  content: string,
  contentType = 'application/zip',
) => {
  const normalized = content.startsWith('base64:') ? content.slice(7) : content;
  let blob: Blob;
  try {
    const bytes = Uint8Array.from(atob(normalized), char => char.charCodeAt(0));
    blob = new Blob([bytes], { type: contentType });
  } catch {
    blob = new Blob([content], { type: contentType });
  }
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  URL.revokeObjectURL(url);
};

const readImportContent = async (file: File) => {
  if (file.name.toLowerCase() === 'skill.md') {
    return file.text();
  }
  return `base64:${bytesToBase64(new Uint8Array(await file.arrayBuffer()))}`;
};

const skillTypeText = (type: workbenchSkill.SkillType) => {
  if (type === workbenchSkill.SkillType.CustomSkill) {
    return '自定义技能';
  }
  if (type === workbenchSkill.SkillType.DeerSkill) {
    return '内置技能';
  }
  if (type === workbenchSkill.SkillType.PublicSkill) {
    return '公共技能';
  }
  return type === workbenchSkill.SkillType.Script ? '脚本技能' : '工作流技能';
};

const isReadonlySkill = (type: workbenchSkill.SkillType) =>
  type === workbenchSkill.SkillType.DeerSkill ||
  type === workbenchSkill.SkillType.PublicSkill;

const SkillAvatar = ({
  skill,
  large = false,
}: {
  skill: Skill;
  large?: boolean;
}) => {
  const tone = skill.icon_uri?.replace('skill-icon://', '') || 'lime';
  if (skill.icon_uri?.startsWith('http')) {
    return (
      <img
        className={`skill-avatar ${large ? 'is-large' : ''}`}
        src={skill.icon_uri}
        alt=""
      />
    );
  }
  return (
    <span
      className={`skill-avatar tone-${tone} ${large ? 'is-large' : ''}`}
      aria-hidden="true"
    >
      {skill.name.trim().slice(0, 1).toUpperCase() || 'S'}
    </span>
  );
};

const CreateSkillModal = ({
  visible,
  submitting,
  onCancel,
  onSubmit,
}: {
  visible: boolean;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: (value: {
    name: string;
    description: string;
    usage: string;
    icon: string;
  }) => void;
}) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [usage, setUsage] = useState('');
  const [icon, setIcon] = useState(ICON_TONES[0]);

  useEffect(() => {
    if (!visible) {
      setName('');
      setDescription('');
      setUsage('');
      setIcon(ICON_TONES[0]);
    }
  }, [visible]);

  return (
    <Modal
      title="手动创建技能"
      visible={visible}
      okText="创建并编辑"
      cancelText="取消"
      confirmLoading={submitting}
      okButtonProps={{ disabled: !name.trim() || !description.trim() }}
      onCancel={onCancel}
      onOk={() =>
        onSubmit({
          name: name.trim(),
          description: description.trim(),
          usage: usage.trim(),
          icon,
        })
      }
    >
      <div className="skill-form-stack">
        <label>
          <span>技能图标</span>
          <div className="skill-icon-picker">
            {ICON_TONES.map(tone => (
              <button
                className={`skill-icon-choice tone-${tone} ${
                  icon === tone ? 'is-active' : ''
                }`}
                key={tone}
                type="button"
                onClick={() => setIcon(tone)}
                aria-label={`选择 ${tone} 图标`}
              >
                {name.trim().slice(0, 1).toUpperCase() || 'S'}
              </button>
            ))}
          </div>
        </label>
        <label>
          <span>技能名称</span>
          <Input
            value={name}
            maxLength={60}
            showClear
            placeholder="例如：周报整理助手"
            onChange={setName}
          />
        </label>
        <label>
          <span>技能说明</span>
          <textarea
            value={description}
            maxLength={500}
            placeholder="说明这个技能能解决什么问题"
            onChange={event => setDescription(event.target.value)}
          />
        </label>
        <label>
          <span>使用场景</span>
          <textarea
            value={usage}
            maxLength={500}
            placeholder="例如：每周五汇总项目进展并生成结构化周报"
            onChange={event => setUsage(event.target.value)}
          />
        </label>
      </div>
    </Modal>
  );
};

const SkillListPage = ({ spaceID }: { spaceID: string }) => {
  const navigate = useNavigate();
  const importInputRef = useRef<HTMLInputElement>(null);
  const spaces = useSpaceStore(state => state.spaceList);
  const [skills, setSkills] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [createVisible, setCreateVisible] = useState(false);
  const [copySkill, setCopySkill] = useState<Skill>();
  const [copyTargetSpaceID, setCopyTargetSpaceID] = useState(spaceID);
  const [query, setQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [page, setPage] = useState(1);

  const loadSkills = useCallback(async () => {
    setLoading(true);
    try {
      const response = await listSkills({ space_id: spaceID });
      setSkills(response.data?.skills ?? []);
    } catch (error) {
      Toast.error(errorText(error, '技能列表加载失败'));
    } finally {
      setLoading(false);
    }
  }, [spaceID]);

  useEffect(() => {
    void loadSkills();
  }, [loadSkills]);

  useEffect(() => {
    setPage(1);
  }, [query, typeFilter, statusFilter]);

  const filteredSkills = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return skills.filter(skill => {
      const matchesQuery =
        !normalized ||
        skill.name.toLowerCase().includes(normalized) ||
        skill.description.toLowerCase().includes(normalized) ||
        skill.usage_scenarios?.toLowerCase().includes(normalized);
      const matchesType =
        typeFilter === 'all' ||
        (typeFilter === 'custom' &&
          skill.type === workbenchSkill.SkillType.CustomSkill) ||
        (typeFilter === 'public' &&
          skill.type !== workbenchSkill.SkillType.CustomSkill);
      const matchesStatus =
        statusFilter === 'all' ||
        (statusFilter === 'enabled' && skill.enabled) ||
        (statusFilter === 'disabled' && !skill.enabled);
      return matchesQuery && matchesType && matchesStatus;
    });
  }, [query, skills, statusFilter, typeFilter]);

  const visibleSkills = filteredSkills.slice(
    (page - 1) * PAGE_SIZE,
    page * PAGE_SIZE,
  );
  const totalPages = Math.max(1, Math.ceil(filteredSkills.length / PAGE_SIZE));

  const handleCreate = async (value: {
    name: string;
    description: string;
    usage: string;
    icon: string;
  }) => {
    setSubmitting(true);
    try {
      const response = await createSkill({
        space_id: spaceID,
        name: value.name,
        description: value.description,
        usage_scenarios: value.usage,
        icon_uri: `skill-icon://${value.icon}`,
        type: workbenchSkill.SkillType.CustomSkill,
        version: '1.0.0',
        enabled: true,
        input_schema: '{}',
        output_schema: '{}',
        executor: '{}',
        permissions: '{}',
      });
      if (!response.data) {
        throw new Error(response.msg || '创建技能失败');
      }
      setCreateVisible(false);
      Toast.success('技能已创建');
      navigate(`/space/${spaceID}/skill/${response.data.id}`);
    } catch (error) {
      Toast.error(errorText(error, '创建技能失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleImport = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) {
      return;
    }
    if (!isSupportedSkillImportFile(file.name)) {
      Toast.error('仅支持 .skill 或 SKILL.md');
      return;
    }
    setSubmitting(true);
    try {
      const response = await importSkill({
        space_id: spaceID,
        file_name: file.name,
        content: await readImportContent(file),
      });
      Toast.success('技能导入成功');
      if (response.data) {
        navigate(`/space/${spaceID}/skill/${response.data.id}`);
      } else {
        await loadSkills();
      }
    } catch (error) {
      Toast.error(errorText(error, '技能导入失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleExport = async (skill: Skill) => {
    try {
      const response = await exportSkill({ skill_id: skill.id });
      if (!response.data) {
        throw new Error(response.msg || '导出失败');
      }
      downloadFile(response.data.file_name, response.data.content);
      Toast.success('导出已开始');
    } catch (error) {
      Toast.error(errorText(error, '技能导出失败'));
    }
  };

  const handleCopy = async () => {
    if (!copySkill || !copyTargetSpaceID) {
      return;
    }
    setSubmitting(true);
    try {
      const exported = await exportSkill({ skill_id: copySkill.id });
      if (!exported.data) {
        throw new Error(exported.msg || '读取技能包失败');
      }
      const imported = await importSkill({
        space_id: copyTargetSpaceID,
        file_name: exported.data.file_name,
        content: exported.data.content,
      });
      setCopySkill(undefined);
      Toast.success('技能已复制到目标工作空间');
      if (copyTargetSpaceID === spaceID) {
        await loadSkills();
      } else if (imported.data) {
        navigate(`/space/${copyTargetSpaceID}/skill/${imported.data.id}`);
      }
    } catch (error) {
      Toast.error(errorText(error, '复制技能失败'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = (skill: Skill) => {
    Modal.warning({
      title: `删除“${skill.name}”？`,
      content: '删除后该技能将从当前工作空间移除，历史任务不会被删除。',
      okText: '删除',
      cancelText: '取消',
      onOk: async () => {
        try {
          await deleteSkill({ skill_id: skill.id });
          setSkills(current => current.filter(item => item.id !== skill.id));
          Toast.success('技能已删除');
        } catch (error) {
          Toast.error(errorText(error, '删除技能失败'));
        }
      },
    });
  };

  return (
    <main
      className="skill-management-page newx-menu-page"
      data-testid="skill-management-page"
    >
      <WorkspacePageTopBar />
      <section className="skill-page-hero">
        <div>
          <span className="skill-eyebrow">SKILL WORKSPACE</span>
          <h1>技能管理</h1>
          <p>集中创建、导入和维护工作空间技能，让团队能力持续沉淀并可复用。</p>
        </div>
        <div className="skill-hero-actions">
          <Button
            className="skill-secondary-button"
            color="secondary"
            onClick={() => importInputRef.current?.click()}
            disabled={submitting}
          >
            导入技能
          </Button>
          <Button
            className="skill-secondary-button"
            color="secondary"
            onClick={() =>
              navigate(`/space/${spaceID}/chats/new`, {
                state: createSkillCreationNavigationState(),
              })
            }
          >
            自动创建
          </Button>
          <Button
            className="skill-primary-button"
            onClick={() => setCreateVisible(true)}
          >
            <span aria-hidden="true">+</span> 手动创建
          </Button>
          <input
            ref={importInputRef}
            hidden
            type="file"
            accept={SKILL_IMPORT_ACCEPT}
            onChange={handleImport}
          />
        </div>
      </section>

      <section className="skill-toolbar" aria-label="技能筛选">
        <Input
          className="skill-search"
          value={query}
          showClear
          placeholder="搜索技能名称、说明或使用场景"
          onChange={setQuery}
        />
        <select
          value={typeFilter}
          onChange={event => setTypeFilter(event.target.value)}
          aria-label="技能类型"
        >
          <option value="all">全部类型</option>
          <option value="custom">自定义技能</option>
          <option value="public">公共与内置技能</option>
        </select>
        <select
          value={statusFilter}
          onChange={event => setStatusFilter(event.target.value)}
          aria-label="技能状态"
        >
          <option value="all">全部状态</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已停用</option>
        </select>
        <span className="skill-result-count">
          共 {filteredSkills.length} 个技能
        </span>
      </section>

      {loading ? (
        <div className="skill-loading">
          <Spin size="large" />
        </div>
      ) : visibleSkills.length ? (
        <section className="skill-card-grid">
          {visibleSkills.map(skill => (
            <article
              className="skill-card"
              key={skill.id}
              data-testid="skill-card"
            >
              <button
                className="skill-card-main"
                type="button"
                onClick={() => navigate(`/space/${spaceID}/skill/${skill.id}`)}
              >
                <div className="skill-card-heading">
                  <SkillAvatar skill={skill} />
                  <div>
                    <h2>{skill.name}</h2>
                    <span>
                      {skillTypeText(skill.type)} · v{skill.version}
                    </span>
                  </div>
                  <i
                    className={`skill-status-dot ${
                      skill.enabled ? 'is-enabled' : ''
                    }`}
                    aria-label={skill.enabled ? '已启用' : '已停用'}
                  />
                </div>
                <p>{skill.description || '暂无技能说明'}</p>
                <div className="skill-card-scenario">
                  <strong>使用场景</strong>
                  <span>{skill.usage_scenarios || '进入详情补充使用场景'}</span>
                </div>
              </button>
              <footer>
                <span>
                  更新于{' '}
                  {timestampToDate(skill.updated_at).toLocaleDateString()}
                </span>
                <details className="skill-action-menu">
                  <summary aria-label="更多操作">···</summary>
                  <div>
                    <button
                      type="button"
                      onClick={() =>
                        navigate(`/space/${spaceID}/skill/${skill.id}`)
                      }
                    >
                      {isReadonlySkill(skill.type) ? '查看' : '编辑'}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setCopyTargetSpaceID(spaceID);
                        setCopySkill(skill);
                      }}
                    >
                      复制到空间
                    </button>
                    <button
                      type="button"
                      onClick={() => void handleExport(skill)}
                    >
                      导出
                    </button>
                    {isReadonlySkill(skill.type) ? null : (
                      <button
                        className="is-danger"
                        type="button"
                        onClick={() => handleDelete(skill)}
                      >
                        删除
                      </button>
                    )}
                  </div>
                </details>
              </footer>
            </article>
          ))}
        </section>
      ) : (
        <div className="skill-empty">
          <Empty
            title="没有找到匹配的技能"
            description="调整筛选条件，或创建一个新技能开始沉淀团队能力。"
          />
        </div>
      )}

      {totalPages > 1 ? (
        <nav className="skill-pagination" aria-label="技能分页">
          <button
            type="button"
            disabled={page === 1}
            onClick={() => setPage(current => current - 1)}
          >
            上一页
          </button>
          <span>
            {page} / {totalPages}
          </span>
          <button
            type="button"
            disabled={page === totalPages}
            onClick={() => setPage(current => current + 1)}
          >
            下一页
          </button>
        </nav>
      ) : null}

      <CreateSkillModal
        visible={createVisible}
        submitting={submitting}
        onCancel={() => setCreateVisible(false)}
        onSubmit={handleCreate}
      />

      <Modal
        title="复制技能到工作空间"
        visible={Boolean(copySkill)}
        okText="复制"
        cancelText="取消"
        confirmLoading={submitting}
        onCancel={() => setCopySkill(undefined)}
        onOk={() => void handleCopy()}
      >
        <div className="skill-form-stack">
          <p className="skill-modal-hint">
            将“{copySkill?.name}
            ”完整复制到目标工作空间，包含当前声明和资源文件。
          </p>
          <label>
            <span>目标工作空间</span>
            <select
              value={copyTargetSpaceID}
              onChange={event => setCopyTargetSpaceID(event.target.value)}
            >
              {(spaces ?? []).map(space => (
                <option key={space.id} value={space.id}>
                  {space.name || '个人空间'}
                </option>
              ))}
            </select>
          </label>
        </div>
      </Modal>
    </main>
  );
};

type InspectorTab = 'overview' | 'versions' | 'run';

const SkillDetailPage = ({
  spaceID,
  skillID,
}: {
  spaceID: string;
  skillID: string;
}) => {
  const navigate = useNavigate();
  const uploadRef = useRef<HTMLInputElement>(null);
  const [skill, setSkill] = useState<Skill>();
  const [versions, setVersions] = useState<SkillVersion[]>([]);
  const [resources, setResources] = useState<SkillResource[]>([]);
  const [activeVersion, setActiveVersion] = useState<SkillVersion>();
  const [activePath, setActivePath] = useState('SKILL.md');
  const [editorValue, setEditorValue] = useState('');
  const [savedValue, setSavedValue] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [tab, setTab] = useState<InspectorTab>('overview');
  const [resourceModal, setResourceModal] = useState<'file' | 'directory'>();
  const [resourcePath, setResourcePath] = useState('');
  const [renameResource, setRenameResource] = useState<SkillResource>();
  const [renamePath, setRenamePath] = useState('');
  const [runInput, setRunInput] = useState('');
  const [runOutput, setRunOutput] = useState('');
  const [running, setRunning] = useState(false);
  const dirty = editorValue !== savedValue;

  const loadResources = useCallback(
    async (version: SkillVersion) => {
      const response = await listSkillVersionResources({
        skill_id: skillID,
        version_id: version.id,
      });
      setResources(response.data?.resources ?? []);
    },
    [skillID],
  );

  const loadDetail = useCallback(async () => {
    setLoading(true);
    try {
      const [skillResponse, versionsResponse] = await Promise.all([
        getSkill({ skill_id: skillID }),
        listSkillVersions({ skill_id: skillID }),
      ]);
      if (!skillResponse.data) {
        throw new Error(skillResponse.msg || '技能不存在');
      }
      const nextVersions = versionsResponse.data?.versions ?? [];
      const latest = nextVersions[0];
      setSkill(skillResponse.data);
      setVersions(nextVersions);
      setActiveVersion(latest);
      if (latest) {
        setActivePath('SKILL.md');
        setEditorValue(latest.skill_md);
        setSavedValue(latest.skill_md);
        await loadResources(latest);
      }
    } catch (error) {
      Toast.error(errorText(error, '技能详情加载失败'));
    } finally {
      setLoading(false);
    }
  }, [loadResources, skillID]);

  useEffect(() => {
    void loadDetail();
  }, [loadDetail]);

  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (dirty) {
        event.preventDefault();
      }
    };
    window.addEventListener('beforeunload', beforeUnload);
    return () => window.removeEventListener('beforeunload', beforeUnload);
  }, [dirty]);

  const confirmDiscard = () =>
    !dirty || window.confirm('当前文件尚未保存，确定放弃修改吗？');

  const openFile = (path: string) => {
    if (!confirmDiscard()) {
      return;
    }
    setActivePath(path);
    const value =
      path === 'SKILL.md'
        ? (activeVersion?.skill_md ?? '')
        : base64ToText(
            resources.find(resource => resource.path === path)
              ?.content_base64 ?? '',
          );
    setEditorValue(value);
    setSavedValue(value);
  };

  const refreshAfterMutation = async (version: SkillVersion) => {
    setActiveVersion(version);
    setVersions(current => [
      version,
      ...current.filter(item => item.id !== version.id),
    ]);
    await loadResources(version);
  };

  const saveFile = async () => {
    if (!activeVersion) {
      return;
    }
    setSaving(true);
    try {
      const response =
        activePath === 'SKILL.md'
          ? await updateSkillVersionContent({
              skill_id: skillID,
              version_id: activeVersion.id,
              skill_md: editorValue,
            })
          : await updateSkillVersionResource({
              skill_id: skillID,
              version_id: activeVersion.id,
              path: activePath,
              content_base64: textToBase64(editorValue),
              operation: workbenchSkill.SkillResourceOperation.Upsert,
            });
      if (!response.data) {
        throw new Error(response.msg || '保存失败');
      }
      setSavedValue(editorValue);
      await refreshAfterMutation(response.data);
      Toast.success('文件已保存并生成新版本');
    } catch (error) {
      Toast.error(errorText(error, '文件保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const createResource = async () => {
    if (!activeVersion || !resourceModal || !resourcePath.trim()) {
      return;
    }
    try {
      const response = await updateSkillVersionResource({
        skill_id: skillID,
        version_id: activeVersion.id,
        path: resourcePath.trim(),
        content_base64: '',
        operation:
          resourceModal === 'directory'
            ? workbenchSkill.SkillResourceOperation.CreateDirectory
            : workbenchSkill.SkillResourceOperation.Upsert,
      });
      if (!response.data) {
        throw new Error(response.msg || '创建失败');
      }
      await refreshAfterMutation(response.data);
      if (resourceModal === 'file') {
        setActivePath(resourcePath.trim());
        setEditorValue('');
        setSavedValue('');
      }
      const createdType = resourceModal;
      setResourceModal(undefined);
      setResourcePath('');
      Toast.success(createdType === 'file' ? '文件已创建' : '文件夹已创建');
    } catch (error) {
      Toast.error(errorText(error, '创建资源失败'));
    }
  };

  const handleUpload = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!activeVersion || !file) {
      return;
    }
    try {
      const response = await updateSkillVersionResource({
        skill_id: skillID,
        version_id: activeVersion.id,
        path: file.name,
        content_base64: bytesToBase64(new Uint8Array(await file.arrayBuffer())),
        operation: workbenchSkill.SkillResourceOperation.Upsert,
      });
      if (!response.data) {
        throw new Error(response.msg || `上传 ${file.name} 失败`);
      }
      await refreshAfterMutation(response.data);
      Toast.success(`已上传 ${file.name}`);
    } catch (error) {
      Toast.error(errorText(error, '文件上传失败'));
    }
  };

  const applyRename = async () => {
    if (!activeVersion || !renameResource || !renamePath.trim()) {
      return;
    }
    try {
      const response = await updateSkillVersionResource({
        skill_id: skillID,
        version_id: activeVersion.id,
        path: renameResource.path,
        content_base64: '',
        operation: workbenchSkill.SkillResourceOperation.Move,
        target_path: renamePath.trim(),
      });
      if (!response.data) {
        throw new Error(response.msg || '重命名失败');
      }
      await refreshAfterMutation(response.data);
      if (
        activePath === renameResource.path ||
        activePath.startsWith(`${renameResource.path}/`)
      ) {
        setActivePath(
          `${renamePath.trim()}${activePath.slice(renameResource.path.length)}`,
        );
      }
      setRenameResource(undefined);
      Toast.success('资源已重命名');
    } catch (error) {
      Toast.error(errorText(error, '重命名失败'));
    }
  };

  const removeResource = (resource: SkillResource) => {
    if (!activeVersion) {
      return;
    }
    Modal.warning({
      title: `删除“${resource.path}”？`,
      content: '保存后会生成一个新版本，可通过版本历史恢复。',
      okText: '删除',
      cancelText: '取消',
      onOk: async () => {
        try {
          const response = await updateSkillVersionResource({
            skill_id: skillID,
            version_id: activeVersion.id,
            path: resource.path,
            content_base64: '',
            operation: workbenchSkill.SkillResourceOperation.Delete,
          });
          if (!response.data) {
            throw new Error(response.msg || '删除失败');
          }
          await refreshAfterMutation(response.data);
          if (
            activePath === resource.path ||
            activePath.startsWith(`${resource.path}/`)
          ) {
            openFile('SKILL.md');
          }
          Toast.success('资源已删除');
        } catch (error) {
          Toast.error(errorText(error, '删除资源失败'));
        }
      },
    });
  };

  const saveOverview = async () => {
    if (!skill) {
      return;
    }
    const expectedVersionID = versions[0]?.id;
    if (!expectedVersionID) {
      Toast.error('缺少当前最新版本，请刷新后重试');
      return;
    }
    setSaving(true);
    try {
      const response = await updateSkill({
        id: skill.id,
        space_id: skill.space_id,
        name: skill.name.trim(),
        description: skill.description.trim(),
        type: skill.type,
        version: skill.version,
        enabled: skill.enabled,
        input_schema: skill.input_schema,
        output_schema: skill.output_schema,
        executor: skill.executor,
        permissions: skill.permissions,
        icon_uri: skill.icon_uri,
        usage_scenarios: skill.usage_scenarios,
        expected_version_id: expectedVersionID,
      });
      if (!response.data) {
        throw new Error(response.msg || '资料保存失败');
      }
      await loadDetail();
      Toast.success('技能资料已保存');
    } catch (error) {
      Toast.error(errorText(error, '资料保存失败'));
    } finally {
      setSaving(false);
    }
  };

  const runSkill = async () => {
    if (!runInput.trim()) {
      Toast.warning('请输入测试内容');
      return;
    }
    setRunning(true);
    setRunOutput('');
    try {
      const response = await testRunSkill({
        skill_id: skillID,
        input: runInput.trim(),
      });
      setRunOutput(response.data?.output || '运行任务已提交');
    } catch (error) {
      setRunOutput(errorText(error, '试运行失败'));
    } finally {
      setRunning(false);
    }
  };

  const selectVersion = async (version: SkillVersion) => {
    if (!confirmDiscard()) {
      return;
    }
    setActiveVersion(version);
    setActivePath('SKILL.md');
    setEditorValue(version.skill_md);
    setSavedValue(version.skill_md);
    await loadResources(version);
  };

  const rollback = (version: SkillVersion) => {
    Modal.warning({
      title: `回滚到 v${version.version}？`,
      content: '回滚不会覆盖历史版本，而是基于该快照创建一个新的当前版本。',
      okText: '确认回滚',
      cancelText: '取消',
      onOk: async () => {
        try {
          const expectedVersionID = versions[0]?.id;
          if (!expectedVersionID) {
            throw new Error('缺少当前最新版本，请刷新后重试');
          }
          await rollbackSkillVersion({
            skill_id: skillID,
            version_id: version.id,
            expected_version_id: expectedVersionID,
          });
          Toast.success('已创建回滚版本');
          await loadDetail();
        } catch (error) {
          Toast.error(errorText(error, '版本回滚失败'));
        }
      },
    });
  };

  const exportVersion = async (version: SkillVersion) => {
    try {
      const response = await exportSkillVersion({
        skill_id: skillID,
        version_id: version.id,
      });
      if (!response.data) {
        throw new Error(response.msg || '导出失败');
      }
      downloadFile(
        response.data.file_name,
        response.data.content_base64,
        response.data.content_type,
      );
    } catch (error) {
      Toast.error(errorText(error, '版本导出失败'));
    }
  };

  if (loading) {
    return (
      <div className="skill-detail-loading">
        <Spin size="large" />
      </div>
    );
  }
  if (!skill) {
    return (
      <div className="skill-empty">
        <Empty title="技能不存在或无权访问" />
      </div>
    );
  }

  const readOnly = isReadonlySkill(skill.type);

  const visibleResources = resources.filter(
    resource => !resource.path.endsWith(`/${DIRECTORY_MARKER}`),
  );
  const directoryResources = resources
    .filter(resource => resource.path.endsWith(`/${DIRECTORY_MARKER}`))
    .map(resource => ({
      directory: resource.path.slice(0, -`/${DIRECTORY_MARKER}`.length),
      marker: resource,
    }));

  return (
    <main className="skill-detail-page" data-testid="skill-detail-page">
      <header className="skill-detail-header">
        <button
          className="skill-back-button"
          type="button"
          onClick={() =>
            confirmDiscard() && navigate(`/space/${spaceID}/skill`)
          }
        >
          ←
        </button>
        <SkillAvatar skill={skill} large />
        <div className="skill-detail-title">
          <div>
            <h1>{skill.name}</h1>
            <span className={skill.enabled ? 'is-enabled' : ''}>
              {skill.enabled ? '已启用' : '已停用'}
            </span>
          </div>
          <p>{skill.description}</p>
        </div>
        <div className="skill-detail-actions">
          {skill.development_thread_id && !readOnly ? (
            <Button
              onClick={() =>
                navigate(
                  `/space/${spaceID}/chats/${skill.development_thread_id}`,
                )
              }
            >
              继续 AI 共创
            </Button>
          ) : null}
          <Button
            onClick={() => void saveFile()}
            disabled={readOnly || !dirty || saving}
            loading={saving}
          >
            保存文件
          </Button>
          <Button
            className="skill-primary-button"
            onClick={() => setTab('run')}
          >
            试运行
          </Button>
        </div>
      </header>

      <section className="skill-workbench">
        <aside className="skill-file-explorer">
          <div className="skill-panel-heading">
            <div>
              <strong>技能文件</strong>
              <span>
                {activeVersion ? `v${activeVersion.version}` : '暂无版本'}
              </span>
            </div>
            {readOnly ? (
              <span>
                {skill.type === workbenchSkill.SkillType.DeerSkill
                  ? '内置技能为只读'
                  : '公共技能为只读'}
              </span>
            ) : (
              <details className="skill-action-menu">
                <summary aria-label="新建资源">+</summary>
                <div>
                  <button
                    type="button"
                    onClick={() => setResourceModal('file')}
                  >
                    新建文件
                  </button>
                  <button
                    type="button"
                    onClick={() => setResourceModal('directory')}
                  >
                    新建文件夹
                  </button>
                  <button
                    type="button"
                    onClick={() => uploadRef.current?.click()}
                  >
                    上传文件
                  </button>
                </div>
              </details>
            )}
          </div>
          <input
            ref={uploadRef}
            hidden
            type="file"
            title="为保证原子保存，一次仅上传一个文件"
            onChange={handleUpload}
          />
          <div className="skill-file-tree">
            <button
              className={activePath === 'SKILL.md' ? 'is-active' : ''}
              type="button"
              onClick={() => openFile('SKILL.md')}
            >
              <span className="skill-file-icon is-primary">M</span>
              <span>SKILL.md</span>
            </button>
            {directoryResources.map(({ directory, marker }) => {
              const managedDirectory = { ...marker, path: directory };
              return (
                <div
                  className="skill-resource-row skill-directory-row"
                  key={marker.id}
                >
                  <div className="skill-directory-label">
                    <span>▸</span>
                    <span title={directory}>{directory}</span>
                  </div>
                  {readOnly ? null : (
                    <details className="skill-action-menu">
                      <summary aria-label={`${directory} 操作`}>···</summary>
                      <div>
                        <button
                          type="button"
                          onClick={() => {
                            setRenameResource(managedDirectory);
                            setRenamePath(directory);
                          }}
                        >
                          重命名
                        </button>
                        <button
                          className="is-danger"
                          type="button"
                          onClick={() => removeResource(managedDirectory)}
                        >
                          删除
                        </button>
                      </div>
                    </details>
                  )}
                </div>
              );
            })}
            {visibleResources.map(resource => (
              <div
                className={`skill-resource-row ${
                  activePath === resource.path ? 'is-active' : ''
                }`}
                key={resource.id}
              >
                <button type="button" onClick={() => openFile(resource.path)}>
                  <span className="skill-file-icon">
                    {resource.path.split('.').pop()?.slice(0, 2).toUpperCase()}
                  </span>
                  <span title={resource.path}>{resource.path}</span>
                </button>
                {readOnly ? null : (
                  <details className="skill-action-menu">
                    <summary aria-label={`${resource.path} 操作`}>···</summary>
                    <div>
                      <button
                        type="button"
                        onClick={() => {
                          setRenameResource(resource);
                          setRenamePath(resource.path);
                        }}
                      >
                        重命名
                      </button>
                      <button
                        className="is-danger"
                        type="button"
                        onClick={() => removeResource(resource)}
                      >
                        删除
                      </button>
                    </div>
                  </details>
                )}
              </div>
            ))}
          </div>
        </aside>

        <section className="skill-editor-panel">
          <header>
            <div>
              <strong>{activePath}</strong>
              {dirty ? <span>未保存</span> : <small>已保存</small>}
            </div>
            <span>{editorValue.split('\n').length} 行</span>
          </header>
          <textarea
            className="skill-code-editor"
            value={editorValue}
            spellCheck={false}
            readOnly={readOnly}
            onChange={event => setEditorValue(event.target.value)}
            aria-label={`${activePath} 编辑器`}
          />
          <footer>
            <span>UTF-8</span>
            <span>{activePath === 'SKILL.md' ? 'Markdown' : '文本文件'}</span>
          </footer>
        </section>

        <aside className="skill-inspector">
          <nav>
            <button
              className={tab === 'overview' ? 'is-active' : ''}
              type="button"
              onClick={() => setTab('overview')}
            >
              概览
            </button>
            <button
              className={tab === 'versions' ? 'is-active' : ''}
              type="button"
              onClick={() => setTab('versions')}
            >
              版本
            </button>
            <button
              className={tab === 'run' ? 'is-active' : ''}
              type="button"
              onClick={() => setTab('run')}
            >
              试运行
            </button>
          </nav>
          {tab === 'overview' ? (
            <div className="skill-inspector-body skill-form-stack">
              <label>
                <span>技能名称</span>
                <Input
                  value={skill.name}
                  disabled={readOnly}
                  onChange={name => setSkill({ ...skill, name })}
                />
              </label>
              <label>
                <span>技能说明</span>
                <textarea
                  value={skill.description}
                  readOnly={readOnly}
                  onChange={event =>
                    setSkill({ ...skill, description: event.target.value })
                  }
                />
              </label>
              <label>
                <span>使用场景</span>
                <textarea
                  value={skill.usage_scenarios ?? ''}
                  readOnly={readOnly}
                  onChange={event =>
                    setSkill({ ...skill, usage_scenarios: event.target.value })
                  }
                />
              </label>
              <label className="skill-toggle-row">
                <span>
                  <strong>启用技能</strong>
                  <small>停用后不会出现在可调用技能中</small>
                </span>
                <input
                  type="checkbox"
                  checked={skill.enabled}
                  disabled={readOnly}
                  onChange={event =>
                    setSkill({ ...skill, enabled: event.target.checked })
                  }
                />
              </label>
              <Button
                className="skill-primary-button"
                loading={saving}
                disabled={readOnly}
                onClick={() => void saveOverview()}
              >
                保存资料
              </Button>
            </div>
          ) : null}
          {tab === 'versions' ? (
            <div className="skill-inspector-body skill-version-list">
              {versions.map((version, index) => (
                <article
                  className={
                    activeVersion?.id === version.id ? 'is-active' : ''
                  }
                  key={version.id}
                >
                  <button
                    type="button"
                    onClick={() => void selectVersion(version)}
                  >
                    <span>
                      v{version.version}
                      {index === 0 ? <i>当前</i> : null}
                    </span>
                    <small>
                      {timestampToDate(version.created_at).toLocaleString()}
                    </small>
                  </button>
                  <div>
                    <button
                      type="button"
                      onClick={() => void exportVersion(version)}
                    >
                      导出
                    </button>
                    {readOnly ? null : (
                      <button type="button" onClick={() => rollback(version)}>
                        回滚
                      </button>
                    )}
                  </div>
                </article>
              ))}
            </div>
          ) : null}
          {tab === 'run' ? (
            <div className="skill-inspector-body skill-run-panel">
              <label>
                <span>测试输入</span>
                <textarea
                  value={runInput}
                  placeholder="输入一个真实任务，验证技能输出"
                  onChange={event => setRunInput(event.target.value)}
                />
              </label>
              <Button
                className="skill-primary-button"
                loading={running}
                disabled={!runInput.trim()}
                onClick={() => void runSkill()}
              >
                开始运行
              </Button>
              <div className="skill-run-output">
                <span>运行结果</span>
                <pre>{runOutput || '运行后将在这里展示输出。'}</pre>
              </div>
            </div>
          ) : null}
        </aside>
      </section>

      <Modal
        title={resourceModal === 'directory' ? '新建文件夹' : '新建文件'}
        visible={Boolean(resourceModal)}
        okText="创建"
        cancelText="取消"
        okButtonProps={{ disabled: !resourcePath.trim() }}
        onCancel={() => {
          setResourceModal(undefined);
          setResourcePath('');
        }}
        onOk={() => void createResource()}
      >
        <div className="skill-form-stack">
          <label>
            <span>路径</span>
            <Input
              value={resourcePath}
              placeholder={
                resourceModal === 'directory'
                  ? 'references/templates'
                  : 'references/guide.md'
              }
              onChange={setResourcePath}
            />
          </label>
        </div>
      </Modal>
      <Modal
        title="重命名资源"
        visible={Boolean(renameResource)}
        okText="保存"
        cancelText="取消"
        okButtonProps={{
          disabled:
            !renamePath.trim() || renamePath.trim() === renameResource?.path,
        }}
        onCancel={() => setRenameResource(undefined)}
        onOk={() => void applyRename()}
      >
        <div className="skill-form-stack">
          <label>
            <span>新路径</span>
            <Input value={renamePath} onChange={setRenamePath} />
          </label>
        </div>
      </Modal>
    </main>
  );
};

export default function SkillManagementPage() {
  const { space_id = '', skill_id } = useParams<{
    space_id: string;
    skill_id?: string;
  }>();
  return skill_id ? (
    <SkillDetailPage spaceID={space_id} skillID={skill_id} />
  ) : (
    <SkillListPage spaceID={space_id} />
  );
}
