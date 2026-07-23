/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, expect, it } from 'vitest';

import {
  deleteMCPToolServer,
  discoverMCPToolServer,
  exportMCPToolServer,
  getMCPToolServer,
  installMCPOfficialCatalog,
  listMCPToolAuditEvents,
  listMCPOfficialCatalog,
  listMCPToolRegistryEntries,
  listMCPToolServers,
  testMCPToolCall,
  upsertMCPToolServer,
} from '../service';

describe('tools service', () => {
  it('exports the complete MCP management API surface', () => {
    [
      listMCPToolServers,
      listMCPOfficialCatalog,
      installMCPOfficialCatalog,
      listMCPToolRegistryEntries,
      upsertMCPToolServer,
      getMCPToolServer,
      deleteMCPToolServer,
      testMCPToolCall,
      discoverMCPToolServer,
      exportMCPToolServer,
      listMCPToolAuditEvents,
    ].forEach(client => expect(typeof client).toBe('function'));
  });
});
