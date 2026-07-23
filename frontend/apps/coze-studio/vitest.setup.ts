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

vi.stubGlobal('IS_BOE', false);
vi.stubGlobal('IS_BOT_OP', false);
vi.stubGlobal('IS_CN_REGION', true);
vi.stubGlobal('IS_DEV_MODE', false);
vi.stubGlobal('IS_OPEN_SOURCE', true);
vi.stubGlobal('IS_OVERSEA', false);
vi.stubGlobal('IS_OVERSEA_RELEASE', false);
vi.stubGlobal('IS_PPE', false);
vi.stubGlobal('IS_PROD', false);
vi.stubGlobal('IS_PROD_MODE', false);
vi.stubGlobal('IS_RELEASE_VERSION', false);
vi.stubGlobal('IS_VA_REGION', false);
vi.stubGlobal('REGION', 'cn');
