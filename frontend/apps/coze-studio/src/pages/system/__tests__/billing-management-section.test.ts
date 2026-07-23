// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';

import {
  createBillingLedgerRows,
  decimalToMicros,
} from '../billing-management-section';

describe('decimalToMicros', () => {
  it('converts whole and fractional credits without floating point rounding', () => {
    expect(decimalToMicros('12')).toBe(12_000_000);
    expect(decimalToMicros('0.000001')).toBe(1);
    expect(decimalToMicros('12.345678')).toBe(12_345_678);
    expect(decimalToMicros('-2.5')).toBe(-2_500_000);
  });

  it('rejects invalid precision and unsafe values', () => {
    expect(() => decimalToMicros('1.0000001')).toThrow();
    expect(() => decimalToMicros('not-a-number')).toThrow();
    expect(() => decimalToMicros('999999999999999999')).toThrow();
  });
});

describe('createBillingLedgerRows', () => {
  it('uses the API contract fields and produces stable unique row keys', () => {
    const rows = createBillingLedgerRows([
      {
        id: 'ledger-1',
        account_id: 'account-1',
        Direction: 'credit',
        EntryType: 'grant',
        AmountMicros: 1_000_000,
        AvailableAfterMicros: 2_000_000,
        ReservedAfterMicros: 0,
        BusinessNo: 'grant-1',
        CreatedAt: '2026-07-23T02:00:00Z',
      },
      {
        id: '',
        account_id: 'account-1',
        Direction: 'debit',
        EntryType: 'consume',
        AmountMicros: 2_000_000,
        AvailableAfterMicros: 0,
        ReservedAfterMicros: 0,
        BusinessNo: 'consume-1',
        CreatedAt: '2026-07-23T02:01:00Z',
      },
    ]);

    expect(rows.map(row => row.key)).toEqual([
      'billing-ledger:ledger-1',
      'billing-ledger:row:1',
    ]);
    expect(rows[0].cells[1]).toBe('account-1');
  });
});
