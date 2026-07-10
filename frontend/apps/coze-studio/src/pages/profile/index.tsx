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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive page orchestrator. */

import { useEffect, useMemo, useState, type FormEvent } from 'react';

import type { UserInfo } from '@coze-foundation/account-base';

import { getProfile, updateProfile } from './service';

import './index.less';

const getDisplayName = (profile: UserInfo | null): string =>
  profile?.name || profile?.screen_name || '未命名用户';

const getUniqueName = (profile: UserInfo | null): string =>
  profile?.app_user_info?.user_unique_name || '-';

const getEditableUniqueName = (profile: UserInfo | null): string =>
  profile?.app_user_info?.user_unique_name || '';

const getAvatarText = (name: string): string => {
  const text = name.trim();
  return text ? text.slice(0, 1).toUpperCase() : 'U';
};

const ProfilePage = () => {
  const [profile, setProfile] = useState<UserInfo | null>(null);
  const [name, setName] = useState('');
  const [uniqueName, setUniqueName] = useState('');
  const [description, setDescription] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  const hydrateProfile = (nextProfile: UserInfo) => {
    setProfile(nextProfile);
    setName(nextProfile.name || nextProfile.screen_name || '');
    setUniqueName(getEditableUniqueName(nextProfile));
    setDescription(nextProfile.description || '');
  };

  useEffect(() => {
    let cancelled = false;

    const loadProfile = async () => {
      setLoading(true);
      setError('');
      try {
        const nextProfile = await getProfile();
        if (!cancelled) {
          hydrateProfile(nextProfile);
        }
      } catch (_err) {
        if (!cancelled) {
          setError('个人资料加载失败，请稍后重试');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    void loadProfile();

    return () => {
      cancelled = true;
    };
  }, []);

  const displayName = getDisplayName(profile);
  const avatarText = useMemo(() => getAvatarText(displayName), [displayName]);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    const nextName = name.trim();
    const nextUniqueName = uniqueName.trim();
    const nextDescription = description.trim();

    setSuccess('');
    if (!nextName) {
      setError('请输入昵称');
      return;
    }
    if (!nextUniqueName) {
      setError('请输入用户名');
      return;
    }

    setSaving(true);
    setError('');
    try {
      const payload = {
        name: nextName,
        description: nextDescription,
        ...(nextUniqueName !== getEditableUniqueName(profile)
          ? { user_unique_name: nextUniqueName }
          : {}),
      };
      const nextProfile = await updateProfile({
        ...payload,
      });
      hydrateProfile(nextProfile);
      setSuccess('保存成功');
    } catch (_err) {
      setError('保存失败，请稍后重试');
    } finally {
      setSaving(false);
    }
  };

  return (
    <main className="profile-page">
      <div className="profile-page__shell">
        <div className="profile-page__eyebrow">Account</div>
        <h1 className="profile-page__title">个人中心</h1>
        <p className="profile-page__subtitle">
          管理当前账号的基础资料。这里先完成一期闭环：查看账号信息，并编辑昵称和个人简介。
        </p>

        {loading ? (
          <section className="profile-page__loading">
            正在加载个人资料...
          </section>
        ) : profile ? (
          <section className="profile-page__grid">
            <aside className="profile-page__card profile-page__summary">
              <div className="profile-page__avatar">{avatarText}</div>
              <h2 className="profile-page__name">{displayName}</h2>
              <p className="profile-page__email">
                {profile.email || '未绑定邮箱'}
              </p>
              <div className="profile-page__meta">
                <div className="profile-page__meta-row">
                  <span className="profile-page__meta-label">用户 ID</span>
                  <span>{profile.user_id_str || '-'}</span>
                </div>
                <div className="profile-page__meta-row">
                  <span className="profile-page__meta-label">用户名</span>
                  <span>{getUniqueName(profile)}</span>
                </div>
              </div>
            </aside>

            <form
              className="profile-page__card profile-page__form"
              onSubmit={handleSubmit}
            >
              <h2 className="profile-page__section-title">基础资料</h2>
              <label className="profile-page__field">
                <span className="profile-page__label">昵称</span>
                <input
                  aria-label="个人昵称"
                  className="profile-page__input"
                  maxLength={64}
                  value={name}
                  onChange={event => setName(event.target.value)}
                />
              </label>

              <label className="profile-page__field">
                <span className="profile-page__label">用户名</span>
                <input
                  aria-label="个人用户名"
                  className="profile-page__input"
                  maxLength={64}
                  value={uniqueName}
                  onChange={event => setUniqueName(event.target.value)}
                />
              </label>

              <label className="profile-page__field">
                <span className="profile-page__label">个人简介</span>
                <textarea
                  aria-label="个人简介"
                  className="profile-page__textarea"
                  maxLength={240}
                  placeholder="写一句让团队成员更容易认识你的介绍"
                  value={description}
                  onChange={event => setDescription(event.target.value)}
                />
              </label>
              <p className="profile-page__hint">
                邮箱、头像和密码仍沿用现有账号设置能力；这一页先承接产品内个人资料入口。
              </p>

              <div
                className={
                  error
                    ? 'profile-page__status profile-page__status--error'
                    : 'profile-page__status'
                }
              >
                {error || success}
              </div>

              <div className="profile-page__actions">
                <button
                  aria-label="保存个人资料"
                  className="profile-page__button"
                  disabled={saving}
                  type="submit"
                >
                  {saving ? '保存中...' : '保存个人资料'}
                </button>
              </div>
            </form>
          </section>
        ) : (
          <section className="profile-page__empty">
            {error || '暂时无法读取个人资料'}
          </section>
        )}
      </div>
    </main>
  );
};

export default ProfilePage;
