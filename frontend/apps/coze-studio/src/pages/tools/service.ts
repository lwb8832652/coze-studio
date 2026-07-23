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

import {
  DeleteMCPToolServer,
  DiscoverMCPToolServer,
  ExportMCPToolServer,
  GetMCPToolServer,
  InstallMCPOfficialCatalog,
  ListMCPOfficialCatalog,
  ListMCPToolAuditEvents,
  ListMCPToolRegistryEntries,
  ListMCPToolServers,
  TestMCPToolCall,
  UpsertMCPToolServer,
} from '@coze-studio/api-schema/workbench-tool';

export const listMCPToolServers = ListMCPToolServers;
export const listMCPOfficialCatalog = ListMCPOfficialCatalog;
export const installMCPOfficialCatalog = InstallMCPOfficialCatalog;
export const listMCPToolRegistryEntries = ListMCPToolRegistryEntries;
export const upsertMCPToolServer = UpsertMCPToolServer;
export const getMCPToolServer = GetMCPToolServer;
export const deleteMCPToolServer = DeleteMCPToolServer;
export const testMCPToolCall = TestMCPToolCall;
export const discoverMCPToolServer = DiscoverMCPToolServer;
export const exportMCPToolServer = ExportMCPToolServer;
export const listMCPToolAuditEvents = ListMCPToolAuditEvents;
