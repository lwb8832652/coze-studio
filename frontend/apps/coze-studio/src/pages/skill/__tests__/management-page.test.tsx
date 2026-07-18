/*
 * Copyright 2025 coze-dev Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 */

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror UI package names. */

import type { ReactNode } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbenchSkill } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() =>
  vi.fn(() => ({ space_id: '1', skill_id: undefined as string | undefined })),
);
const mockNavigate = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());
const mockGetSkill = vi.hoisted(() => vi.fn());
const mockCreateSkill = vi.hoisted(() => vi.fn());
const mockListVersions = vi.hoisted(() => vi.fn());
const mockListResources = vi.hoisted(() => vi.fn());
const mockTestRunSkill = vi.hoisted(() => vi.fn());
const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (selector: (state: unknown) => unknown) =>
    selector({ spaceList: [{ id: '1', name: '测试空间' }] }),
}));

vi.mock('@coze-arch/coze-design', () => {
  const Modal = Object.assign(
    ({
      visible,
      title,
      children,
      okText,
      cancelText,
      onOk,
      onCancel,
    }: {
      visible?: boolean;
      title?: ReactNode;
      children?: ReactNode;
      okText?: ReactNode;
      cancelText?: ReactNode;
      onOk?: () => void;
      onCancel?: () => void;
    }) =>
      visible ? (
        <div role="dialog">
          <h2>{title}</h2>
          {children}
          <button type="button" onClick={onCancel}>
            {cancelText}
          </button>
          <button type="button" onClick={onOk}>
            {okText}
          </button>
        </div>
      ) : null,
    { warning: vi.fn() },
  );

  return {
    Button: ({
      children,
      disabled,
      onClick,
    }: {
      children?: ReactNode;
      disabled?: boolean;
      onClick?: () => void;
    }) => (
      <button type="button" disabled={disabled} onClick={onClick}>
        {children}
      </button>
    ),
    Empty: ({
      title,
      description,
    }: {
      title?: ReactNode;
      description?: ReactNode;
    }) => (
      <div>
        {title}
        {description}
      </div>
    ),
    Input: ({
      value,
      placeholder,
      disabled,
      onChange,
    }: {
      value?: string;
      placeholder?: string;
      disabled?: boolean;
      onChange?: (value: string) => void;
    }) => (
      <input
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        onChange={event => onChange?.(event.target.value)}
      />
    ),
    Modal,
    Spin: () => <div>loading</div>,
    Toast: {
      success: mockToastSuccess,
      error: mockToastError,
      warning: vi.fn(),
    },
  };
});

vi.mock('../service', () => ({
  listSkills: mockListSkills,
  getSkill: mockGetSkill,
  createSkill: mockCreateSkill,
  importSkill: vi.fn(),
  updateSkill: vi.fn(),
  deleteSkill: vi.fn(),
  exportSkill: vi.fn(),
  listSkillVersions: mockListVersions,
  listSkillVersionResources: mockListResources,
  updateSkillVersionContent: vi.fn(),
  updateSkillVersionResource: vi.fn(),
  exportSkillVersion: vi.fn(),
  rollbackSkillVersion: vi.fn(),
  testRunSkill: mockTestRunSkill,
}));

import SkillManagementPage, {
  isSupportedSkillImportFile,
  SKILL_IMPORT_ACCEPT,
} from '../management-page';

const skill = {
  id: '101',
  space_id: '1',
  name: '周报助手',
  description: '整理团队周报',
  usage_scenarios: '每周五汇总项目进展',
  icon_uri: 'skill-icon://lime',
  type: workbenchSkill.SkillType.CustomSkill,
  version: '1.0.0',
  enabled: true,
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions: '{}',
  created_at: 1,
  updated_at: 1_783_935_988_000,
};

const version = {
  id: '201',
  skill_id: '101',
  version: '1.0.0',
  skill_md: '# 周报助手',
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions: '{}',
  created_at: 1,
};

describe('Nuwax parity skill management', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    vi.clearAllMocks();
    mockUseParams.mockReturnValue({ space_id: '1', skill_id: undefined });
    mockListSkills.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { skills: [skill] },
    });
    mockCreateSkill.mockResolvedValue({ code: 0, msg: 'success', data: skill });
    mockGetSkill.mockResolvedValue({ code: 0, msg: 'success', data: skill });
    mockListVersions.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { versions: [version] },
    });
    mockListResources.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        resources: [
          {
            id: '301',
            skill_id: '101',
            version_id: '201',
            path: 'references/guide.md',
            content_base64: 'IyDkvb/nlKjmiYvlhow=',
            size: 14,
            sha256: 'hash',
            created_at: 1,
          },
          {
            id: '302',
            skill_id: '101',
            version_id: '201',
            path: 'references/.coze-folder',
            content_base64: 'LmNvemUtZm9sZGVy',
            size: 12,
            sha256: 'folder-hash',
            created_at: 1,
          },
        ],
      },
    });
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(() => root.unmount());
    container.remove();
  });

  const renderPage = async () => {
    await act(async () => {
      root.render(<SkillManagementPage />);
      await Promise.resolve();
    });
  };

  it('shows both creation paths and workspace skills', async () => {
    await renderPage();

    expect(container.textContent).toContain('周报助手');
    expect(container.textContent).toContain('使用 AI 创建');
    expect(container.textContent).toContain('手动创建');
    expect(container.textContent).toContain('每周五汇总项目进展');
    expect(container.textContent).toContain('2026');
    expect(container.textContent).not.toContain('58461');
  });

  it('opens AI skill creation in the shared workbench home', async () => {
    await renderPage();

    const createWithAIButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent === '使用 AI 创建',
    ) as HTMLButtonElement;

    act(() => {
      createWithAIButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/1/chats/new', {
      state: {
        workbenchIntent: 'create_skill',
        initialMessage:
          '我想创建一个技能，请先询问我技能用途、使用场景和期望输出。',
      },
    });
  });

  it('accepts only importable skill formats and exposes single-file upload', async () => {
    expect(isSupportedSkillImportFile('project.skill')).toBe(true);
    expect(isSupportedSkillImportFile('SKILL.md')).toBe(true);
    expect(isSupportedSkillImportFile('project.zip')).toBe(false);
    expect(SKILL_IMPORT_ACCEPT).toBe('.skill,.md');

    mockUseParams.mockReturnValue({ space_id: '1', skill_id: '101' });
    await renderPage();
    const upload = container.querySelector<HTMLInputElement>(
      'input[title="为保证原子保存，一次仅上传一个文件"]',
    );
    expect(upload).not.toBeNull();
    expect(upload?.multiple).toBe(false);
  });

  it('submits the Nuwax manual-create metadata', async () => {
    await renderPage();
    const manualButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('手动创建'),
    );
    await act(() => manualButton?.click());

    const nameInput = container.querySelector<HTMLInputElement>(
      'input[placeholder="例如：周报整理助手"]',
    );
    const description = container.querySelector<HTMLTextAreaElement>(
      'textarea[placeholder="说明这个技能能解决什么问题"]',
    );
    expect(nameInput).not.toBeNull();
    expect(description).not.toBeNull();
    act(() => {
      Simulate.change(nameInput as HTMLInputElement, {
        target: { value: '会议纪要助手' },
      });
      Simulate.change(description as HTMLTextAreaElement, {
        target: { value: '整理会议纪要' },
      });
    });
    const createButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '创建并编辑',
    );
    await act(async () => {
      createButton?.click();
      await Promise.resolve();
    });

    expect(mockCreateSkill).toHaveBeenCalledWith(
      expect.objectContaining({
        space_id: '1',
        name: '会议纪要助手',
        description: '整理会议纪要',
        type: workbenchSkill.SkillType.CustomSkill,
      }),
    );
  });

  it('opens the file, version and run workbench', async () => {
    mockUseParams.mockReturnValue({ space_id: '1', skill_id: '101' });
    await renderPage();

    expect(
      container.querySelector('[data-testid="skill-detail-page"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain('SKILL.md');
    expect(container.textContent).toContain('references/guide.md');
    expect(
      container.querySelector('[aria-label="references 操作"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain('版本');
    expect(container.textContent).toContain('试运行');
  });

  it('exposes only safe list actions for builtin skills', async () => {
    mockListSkills.mockResolvedValueOnce({
      code: 0,
      msg: 'success',
      data: {
        skills: [{ ...skill, type: workbenchSkill.SkillType.DeerSkill }],
      },
    });

    await renderPage();

    const card = container.querySelector('[data-testid="skill-card"]');
    expect(card?.textContent).toContain('查看');
    expect(card?.textContent).not.toContain('编辑');
    expect(card?.textContent).not.toContain('删除');
    expect(card?.textContent).toContain('复制到空间');
    expect(card?.textContent).toContain('导出');
  });

  it('renders builtin skill details as read-only', async () => {
    mockUseParams.mockReturnValue({ space_id: '1', skill_id: '101' });
    mockGetSkill.mockResolvedValueOnce({
      code: 0,
      msg: 'success',
      data: { ...skill, type: workbenchSkill.SkillType.DeerSkill },
    });

    await renderPage();

    expect(
      container.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="SKILL.md 编辑器"]',
      )?.readOnly,
    ).toBe(true);
    expect(
      container.querySelector<HTMLInputElement>('input[value="周报助手"]')
        ?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLInputElement>('input[type="checkbox"]')
        ?.disabled,
    ).toBe(true);
    const saveOverview = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存资料',
    );
    expect(saveOverview?.disabled).toBe(true);
    expect(container.textContent).toContain('内置技能为只读');
    expect(container.textContent).not.toContain('新建资源');
  });

  it('shows an actionable message when direct test run is unsupported', async () => {
    mockUseParams.mockReturnValue({ space_id: '1', skill_id: '101' });
    mockTestRunSkill.mockRejectedValueOnce({
      response: {
        data: {
          msg: 'skill type custom_skill does not support direct test run',
        },
      },
    });
    await renderPage();

    const runButtons = Array.from(container.querySelectorAll('button')).filter(
      button => button.textContent === '试运行',
    );
    act(() => runButtons[0]?.click());
    const input = container.querySelector(
      'textarea[placeholder="输入一个真实任务，验证技能输出"]',
    );
    act(() => {
      Simulate.change(input as HTMLTextAreaElement, {
        target: { value: '生成一份周报' },
      });
    });
    const startButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '开始运行',
    );
    await act(async () => {
      startButton?.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain(
      '当前技能类型不支持直接试运行，请在任务中启用该技能后验证',
    );
  });
});
