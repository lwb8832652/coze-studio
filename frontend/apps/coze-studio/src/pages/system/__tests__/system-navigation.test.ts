// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';

import { SYSTEM_NAV_GROUPS, SYSTEM_SECTIONS } from '../content';

describe('system navigation groups', () => {
  it('keeps model pricing and monitoring under model management', () => {
    const modelGroup = SYSTEM_NAV_GROUPS.find(group => group.key === 'models');
    const billingGroup = SYSTEM_NAV_GROUPS.find(
      group => group.key === 'billing',
    );

    expect(modelGroup?.label).toBe('模型管理');
    expect(modelGroup?.items).toEqual([
      'models',
      'billing-pricing',
      'billing-monitoring',
    ]);
    expect(billingGroup?.label).toBe('订阅与积分');
    expect(billingGroup?.items).toEqual([
      'billing-config',
      'billing-plans',
      'billing-packages',
      'billing-accounts',
      'billing-ledger',
      'billing-orders',
    ]);
  });

  it('includes every system section exactly once', () => {
    const groupedKeys = SYSTEM_NAV_GROUPS.flatMap(group => group.items);

    expect(groupedKeys).toHaveLength(new Set(groupedKeys).size);
    expect(groupedKeys).toEqual(SYSTEM_SECTIONS.map(section => section.key));
  });
});
