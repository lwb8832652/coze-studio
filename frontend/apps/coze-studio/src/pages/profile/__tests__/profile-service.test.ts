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

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { getProfile, updateProfile } from '../service';

const mockUser = {
  user_id_str: '1001',
  name: '畅享AI',
  screen_name: '畅享AI',
  description: '团队管理员',
  email: 'admin@example.com',
  avatar_url: '',
};

const checkLogin = vi.hoisted(() => vi.fn());
const updateUserProfile = vi.hoisted(() => vi.fn());
const refreshUserInfo = vi.hoisted(() => vi.fn());
const getUserInfo = vi.hoisted(() => vi.fn());

vi.mock('@coze-foundation/account-adapter', () => ({
  getUserInfo,
  passportApi: {
    checkLogin,
    updateUserProfile,
  },
  refreshUserInfo,
}));

describe('profile service', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    checkLogin.mockResolvedValue(mockUser);
    getUserInfo.mockReturnValue(mockUser);
    refreshUserInfo.mockResolvedValue(undefined);
    updateUserProfile.mockResolvedValue({ code: 0, msg: 'ok' });
  });

  it('loads current account profile', async () => {
    await expect(getProfile()).resolves.toEqual(mockUser);
    expect(checkLogin).toHaveBeenCalledTimes(1);
  });

  it('updates editable profile fields then refreshes global account cache', async () => {
    await expect(
      updateProfile({
        name: '新昵称',
        description: '新的个人简介',
        user_unique_name: 'new_name',
      }),
    ).resolves.toEqual(mockUser);

    expect(updateUserProfile).toHaveBeenCalledWith({
      name: '新昵称',
      description: '新的个人简介',
      user_unique_name: 'new_name',
    });
    expect(refreshUserInfo).toHaveBeenCalledTimes(1);
    expect(getUserInfo).toHaveBeenCalledTimes(1);
    expect(checkLogin).not.toHaveBeenCalled();
  });
});
