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

import { type FC, type FormEvent, useEffect, useMemo, useState } from 'react';

import { CozeBrand } from '@coze-studio/components/coze-brand';
import { I18n } from '@coze-arch/i18n';
import { IconCozEarth } from '@coze-arch/coze-design/icons';
import { Button, Input } from '@coze-arch/coze-design';

import { useLoginService } from './service';

import './index.less';

type AuthMode = 'login' | 'register';
type AuthLocale = 'zh-CN' | 'en';

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const MIN_PASSWORD_LENGTH = 6;

const AUTH_COPY = {
  'zh-CN': {
    brandName: 'Coze Studio',
    brandTitle: 'An open-source AI agent development platform',
    brandDescription: '提供完整的智能体开发、调试、监控、发布与团队协作能力。',
    loginTab: '密码登录',
    registerTab: '邮箱注册',
    welcome: '欢迎来到扣子-开源版',
    emailPlaceholder: '请输入邮箱',
    passwordPlaceholder: '请输入不少于 6 位的密码',
    confirmPasswordPlaceholder: '请再次输入密码',
    invalidEmail: '请输入有效的邮箱地址',
    invalidPassword: '密码至少需要 6 位',
    passwordMismatch: '两次输入的密码不一致',
    login: '登录',
    register: '注册',
    agreementPrefix: '已阅读并同意协议：',
    agreement: '开源协议',
    agreementRequired: '请先阅读并同意开源协议',
    footer: 'Powered by Coze Studio',
    language: '简体中文',
    switchLanguage: '切换至 English',
  },
  en: {
    brandName: 'Coze Studio',
    brandTitle: 'An open-source AI agent development platform',
    brandDescription:
      'A complete workspace for building, debugging, monitoring, publishing, and collaborating on AI agents.',
    loginTab: 'Password',
    registerTab: 'Sign up',
    welcome: 'Welcome to Coze Studio',
    emailPlaceholder: 'Enter your email',
    passwordPlaceholder: 'Enter a password with at least 6 characters',
    confirmPasswordPlaceholder: 'Enter your password again',
    invalidEmail: 'Enter a valid email address',
    invalidPassword: 'Password must contain at least 6 characters',
    passwordMismatch: 'The passwords do not match',
    login: 'Log in',
    register: 'Sign up',
    agreementPrefix: 'I have read and agree to the ',
    agreement: 'Open-source license',
    agreementRequired: 'Please read and agree to the open-source license',
    footer: 'Powered by Coze Studio',
    language: 'English',
    switchLanguage: '切换至简体中文',
  },
} as const;

const readInitialLocale = (): AuthLocale => {
  try {
    return localStorage.getItem('i18next') === 'en' ? 'en' : 'zh-CN';
  } catch (error) {
    void error;
    return 'zh-CN';
  }
};

// Login and registration intentionally share one stateful form so switching
// modes preserves the user's input and validation state.
// eslint-disable-next-line @coze-arch/max-line-per-function, complexity -- Both modes intentionally share one stateful form.
export const LoginPage: FC = () => {
  const [mode, setMode] = useState<AuthMode>('login');
  const [locale, setLocale] = useState<AuthLocale>(readInitialLocale);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [accepted, setAccepted] = useState(true);
  const [touched, setTouched] = useState({
    email: false,
    password: false,
    confirmPassword: false,
    agreement: false,
  });

  const { login, register, loginLoading, registerLoading } = useLoginService({
    email,
    password,
  });

  useEffect(() => {
    document.documentElement.classList.add('coze-auth-html');
    document.body.classList.add('coze-auth-body');
    return () => {
      document.documentElement.classList.remove('coze-auth-html');
      document.body.classList.remove('coze-auth-body');
    };
  }, []);

  const copy = AUTH_COPY[locale];
  const emailValid = EMAIL_PATTERN.test(email.trim());
  const passwordValid = password.length >= MIN_PASSWORD_LENGTH;
  const confirmPasswordValid =
    mode === 'login' ||
    (confirmPassword.length >= MIN_PASSWORD_LENGTH &&
      confirmPassword === password);
  const emailError = touched.email && !emailValid;
  const passwordError = touched.password && !passwordValid;
  const confirmPasswordError =
    mode === 'register' && touched.confirmPassword && !confirmPasswordValid;
  const agreementError = touched.agreement && !accepted;
  const loading = mode === 'login' ? loginLoading : registerLoading;

  const submitTestID = useMemo(
    () => (mode === 'login' ? 'login.button.login' : 'login.button.signup'),
    [mode],
  );

  const changeMode = (nextMode: AuthMode) => {
    setMode(nextMode);
    setTouched({
      email: false,
      password: false,
      confirmPassword: false,
      agreement: false,
    });
  };

  const changeLocale = () => {
    const nextLocale: AuthLocale = locale === 'zh-CN' ? 'en' : 'zh-CN';
    setLocale(nextLocale);
    try {
      localStorage.setItem('i18next', nextLocale === 'en' ? 'en' : 'zh-CN');
    } catch (error) {
      void error;
      // The selected language still applies to this page when storage is unavailable.
    }
    I18n.setLang(nextLocale === 'en' ? 'en' : 'zh-CN');
  };

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setTouched({
      email: true,
      password: true,
      confirmPassword: mode === 'register',
      agreement: true,
    });

    if (
      !emailValid ||
      !passwordValid ||
      !confirmPasswordValid ||
      !accepted ||
      loading
    ) {
      return;
    }

    if (mode === 'login') {
      login();
      return;
    }
    register();
  };

  return (
    <main className="coze-auth-page">
      <div className="coze-auth-brand" aria-label="Coze Studio">
        <CozeBrand isOversea={false} />
      </div>
      <button
        type="button"
        className="coze-auth-language"
        aria-label={copy.switchLanguage}
        onClick={changeLocale}
      >
        <IconCozEarth size="small" />
        <span>{copy.language}</span>
      </button>

      <section className="coze-auth-intro" aria-label={copy.brandName}>
        <div className="coze-auth-intro-content">
          <p className="coze-auth-intro-name">{copy.brandName}</p>
          <h1>{copy.brandTitle}</h1>
          <p className="coze-auth-intro-description">{copy.brandDescription}</p>
        </div>
      </section>

      <section className="coze-auth-content">
        <div className="coze-auth-panel">
          <div
            className="coze-auth-tabs"
            role="tablist"
            aria-label={copy.welcome}
          >
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'login'}
              className={mode === 'login' ? 'is-active' : ''}
              data-testid="login.tab.login"
              onClick={() => changeMode('login')}
            >
              {copy.loginTab}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === 'register'}
              className={mode === 'register' ? 'is-active' : ''}
              data-testid="login.tab.register"
              onClick={() => changeMode('register')}
            >
              {copy.registerTab}
            </button>
          </div>

          <form className="coze-auth-form" onSubmit={handleSubmit} noValidate>
            <h2>{copy.welcome}</h2>

            <div className="coze-auth-field">
              <Input
                data-testid="login.input.email"
                aria-label={copy.emailPlaceholder}
                autoComplete="email"
                value={email}
                onChange={setEmail}
                onBlur={() =>
                  setTouched(current => ({ ...current, email: true }))
                }
                placeholder={copy.emailPlaceholder}
                validateStatus={emailError ? 'error' : 'default'}
              />
              {emailError ? <p role="alert">{copy.invalidEmail}</p> : null}
            </div>

            <div className="coze-auth-field">
              <Input
                data-testid="login.input.password"
                aria-label={copy.passwordPlaceholder}
                autoComplete={
                  mode === 'login' ? 'current-password' : 'new-password'
                }
                mode="password"
                value={password}
                onChange={setPassword}
                onBlur={() =>
                  setTouched(current => ({ ...current, password: true }))
                }
                placeholder={copy.passwordPlaceholder}
                validateStatus={passwordError ? 'error' : 'default'}
              />
              {passwordError ? (
                <p role="alert">{copy.invalidPassword}</p>
              ) : null}
            </div>

            {mode === 'register' ? (
              <div className="coze-auth-field">
                <Input
                  data-testid="login.input.confirm-password"
                  aria-label={copy.confirmPasswordPlaceholder}
                  autoComplete="new-password"
                  mode="password"
                  value={confirmPassword}
                  onChange={setConfirmPassword}
                  onBlur={() =>
                    setTouched(current => ({
                      ...current,
                      confirmPassword: true,
                    }))
                  }
                  placeholder={copy.confirmPasswordPlaceholder}
                  validateStatus={confirmPasswordError ? 'error' : 'default'}
                />
                {confirmPasswordError ? (
                  <p role="alert">{copy.passwordMismatch}</p>
                ) : null}
              </div>
            ) : null}

            <Button
              data-testid={submitTestID}
              className="coze-auth-submit"
              htmlType="submit"
              loading={loading}
              color="hgltplus"
            >
              {mode === 'login' ? copy.login : copy.register}
            </Button>

            <label className="coze-auth-agreement">
              <input
                type="checkbox"
                checked={accepted}
                aria-label={copy.agreementRequired}
                onChange={event => {
                  setAccepted(event.target.checked);
                  setTouched(current => ({ ...current, agreement: true }));
                }}
              />
              <span>
                {copy.agreementPrefix}
                <a
                  data-testid="login.link.terms"
                  href="https://github.com/coze-dev/coze-studio?tab=Apache-2.0-1-ov-file"
                  target="_blank"
                  rel="noreferrer"
                >
                  {copy.agreement}
                </a>
              </span>
            </label>
            {agreementError ? (
              <p className="coze-auth-agreement-error" role="alert">
                {copy.agreementRequired}
              </p>
            ) : null}
          </form>

          <footer className="coze-auth-footer">{copy.footer}</footer>
        </div>
      </section>
    </main>
  );
};
