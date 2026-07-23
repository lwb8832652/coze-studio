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

/* eslint-disable @typescript-eslint/naming-convention */

import { Helmet } from 'react-helmet';
import React, { PropsWithChildren } from 'react';

import classNames from 'classnames';

import UIHeader, { UIHeaderProps } from '../ui-header';
import UIFooter, { UIFooterProps } from '../ui-footer';
import UIContent from '../ui-content';

import s from './index.module.less';

export const UIDocumentTitle: React.FC<{ title: string }> = ({ title }) => (
  <Helmet>
    <title>{title}</title>
  </Helmet>
);

export const UILayout: React.FC<
  PropsWithChildren<{
    className?: string;
    title?: string;
  }>
> & {
  Header: React.FC<UIHeaderProps>;
  Content: typeof UIContent;
  Footer: React.FC<UIFooterProps>;
} = ({ className, children, title }) => (
  <div className={classNames(s['ui-layout'], className)}>
    {title ? <UIDocumentTitle title={title} /> : null}
    {children}
  </div>
);

UILayout.Header = UIHeader;
UILayout.Content = UIContent;
UILayout.Footer = UIFooter;

export default UILayout;
