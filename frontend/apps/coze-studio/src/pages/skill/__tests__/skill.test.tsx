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

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { workbenchSkill } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockListSkills = vi.hoisted(() => vi.fn());
const mockTestRunSkill = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  importSkill: vi.fn(),
  listSkills: mockListSkills,
  testRunSkill: mockTestRunSkill,
}));

import SkillPage from '../index';

const skill = {
  id: 'skill-1',
  space_id: 'space-1',
  name: 'Ping Skill',
  description: '',
  type: workbenchSkill.SkillType.Script,
  version: '1.0.0',
  enabled: true,
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions: '{}',
  created_at: 1717000000000,
  updated_at: 1717000300000,
};

describe('SkillPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockListSkills.mockResolvedValue({
      data: { skills: [skill] },
      code: 0,
      msg: '',
    });
    mockTestRunSkill.mockReset();
  });

  it('clears stale test output when a later test run fails', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockTestRunSkill
      .mockResolvedValueOnce({
        data: { output: 'pong' },
        code: 0,
        msg: '',
      })
      .mockRejectedValueOnce(new Error('boom'));

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const runButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('试运行'),
    ) as HTMLButtonElement;

    await act(async () => {
      runButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('pong');

    await act(async () => {
      runButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('boom');
    expect(container.textContent).not.toContain('pong');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
