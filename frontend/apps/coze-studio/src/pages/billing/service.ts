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

/* eslint-disable @typescript-eslint/naming-convention -- Billing DTO fields intentionally preserve server wire names. */

export interface Envelope<T> {
  code: number;
  msg: string;
  data: T;
}
export interface PublicBillingConfig {
  credit_name: string;
  display_scale: number;
  payment_enabled: boolean;
  default_currency: string;
}
export interface Balance {
  AccountID: string;
  AvailableMicros: number;
  ReservedMicros: number;
  Version: number;
}
export interface Subscription {
  id: string;
  plan_id: string;
  PlanName: string;
  Status: string;
  CurrentPeriodStart: string;
  CurrentPeriodEnd: string;
  AutoRenew: boolean;
}
export interface Plan {
  id: string;
  Name: string;
  Description: string;
  Cycle: string;
  Currency: string;
  PriceMicros: number;
  CreditGrantMicros: number;
  FeaturesJSON: string;
}
export interface CreditPackage {
  id: string;
  Name: string;
  Description: string;
  Currency: string;
  PriceMicros: number;
  CreditMicros: number;
  ValidityDays: number;
}
export interface Ledger {
  id: string;
  Direction: string;
  EntryType: string;
  AmountMicros: number;
  AvailableAfterMicros: number;
  BusinessNo: string;
  CreatedAt: string;
}
export interface Usage {
  id: string;
  task_id: string;
  RunID: string;
  Provider: string;
  ModelID: string;
  InputTokens: number;
  OutputTokens: number;
  CacheWriteTokens: number;
  CacheHitTokens: number;
  ChargeMicros: number;
  CreatedAt: string;
}
export interface Order {
  id: string;
  OrderNo: string;
  OrderType: string;
  Status: string;
  PaymentStatus: string;
  FulfillmentStatus: string;
  Currency: string;
  TotalMicros: number;
  CreatedAt: string;
}
export interface Checkout {
  gateway: string;
  provider_transaction_id: string;
  checkout_url: string;
}

const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`/api/billing${path}`, {
    credentials: 'include',
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  });
  const payload = (await response.json().catch(() => ({}))) as Partial<
    Envelope<T>
  >;
  if (!response.ok || payload.code !== 0) {
    throw new Error(payload.msg || `请求失败 (${response.status})`);
  }
  return payload.data as T;
};
export const getAccount = () => request<Balance>('/account');
export const getBillingConfig = () => request<PublicBillingConfig>('/config');
export const getSubscription = () =>
  request<Subscription | null>('/subscription');
export const listPlans = () => request<Plan[]>('/plans');
export const listPackages = () => request<CreditPackage[]>('/credit-packages');
export const listLedger = () => request<Ledger[]>('/ledger');
export const listUsage = () => request<Usage[]>('/usage');
export const listOrders = () => request<Order[]>('/orders');
export const createOrder = (
  orderType: 'subscription' | 'credit_package',
  targetID: string,
) =>
  request<Order>('/orders', {
    method: 'POST',
    body: JSON.stringify({
      order_type: orderType,
      target_id: targetID,
      order_no: `ord_${crypto.randomUUID().replaceAll('-', '')}`,
    }),
  });
export const checkoutOrder = (orderNo: string) =>
  request<Checkout>(`/orders/${encodeURIComponent(orderNo)}/checkout`, {
    method: 'POST',
    body: '{}',
  });
