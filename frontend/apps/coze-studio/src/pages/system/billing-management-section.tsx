// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @typescript-eslint/naming-convention, @typescript-eslint/no-explicit-any, @coze-arch/max-line-per-function, max-lines -- Admin billing tables preserve wire fields and Semi render callback contracts in one surface. */

import {
  type FormEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';

import { Modal } from '@coze-arch/coze-design';

import {
  adjustBillingCredits,
  createBillingModelPrice,
  createBillingPackage,
  createBillingPlan,
  getBillingConfig,
  getBillingOverview,
  listBillingAccounts,
  listBillingLedger,
  listBillingModelPrices,
  listBillingOrders,
  listBillingPackages,
  listBillingPlans,
  listBillingUsageMonitoring,
  runBillingMaintenance,
  saveBillingConfig,
  type BillingAccount,
  type BillingConfig,
  type BillingLedger,
  type BillingMaintenanceResult,
} from './service';
import './billing-management-section.less';

export type BillingSectionKey =
  | 'billing-config'
  | 'billing-plans'
  | 'billing-packages'
  | 'billing-pricing'
  | 'billing-monitoring'
  | 'billing-accounts'
  | 'billing-ledger'
  | 'billing-orders';

type DialogState =
  | { kind: 'plan' }
  | { kind: 'package' }
  | { kind: 'price' }
  | { kind: 'adjust'; account: BillingAccount }
  | null;

interface TableRow {
  key: string;
  cells: ReactNode[];
}

const formatCredits = (value?: number) =>
  ((value ?? 0) / 1_000_000).toLocaleString('zh-CN', {
    maximumFractionDigits: 6,
  });

const formatMoney = (value?: number, currency = 'CNY') =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(
    (value ?? 0) / 1_000_000,
  );

const formatDate = (value?: string) =>
  value ? new Date(value).toLocaleString('zh-CN') : '-';

const statusLabel = (value?: string) =>
  ({
    active: '生效中',
    cancelled: '已取消',
    closed: '已关闭',
    expired: '已到期',
    failed: '失败',
    fulfilled: '已履约',
    paid: '已支付',
    past_due: '已逾期',
    pending: '处理中',
    pending_payment: '待支付',
    published: '已发布',
    succeeded: '成功',
  })[value ?? ''] ??
  value ??
  '-';

export const decimalToMicros = (value: string): number => {
  const normalized = value.trim();
  if (!/^-?\d+(\.\d{0,6})?$/.test(normalized)) {
    throw new Error('请输入最多 6 位小数的有效数值');
  }
  const negative = normalized.startsWith('-');
  const unsigned = negative ? normalized.slice(1) : normalized;
  const [whole, fraction = ''] = unsigned.split('.');
  const micros = Number(whole) * 1_000_000 + Number(fraction.padEnd(6, '0'));
  if (!Number.isSafeInteger(micros)) {
    throw new Error('数值超出安全范围');
  }
  return negative ? -micros : micros;
};

const billingRowKey = (
  namespace: string,
  item: { id?: unknown; ID?: unknown },
  index: number,
) => {
  const identifier = item.id ?? item.ID;
  return identifier === undefined || identifier === null || identifier === ''
    ? `${namespace}:row:${index}`
    : `${namespace}:${String(identifier)}`;
};

export const createBillingLedgerRows = (items: BillingLedger[]): TableRow[] =>
  items.map((item, index) => ({
    key: billingRowKey('billing-ledger', item, index),
    cells: [
      item.BusinessNo,
      item.account_id,
      item.EntryType,
      item.Direction,
      formatCredits(item.AmountMicros),
      formatCredits(item.AvailableAfterMicros),
      formatDate(item.CreatedAt),
    ],
  }));

export const BillingManagementSection = ({
  section,
}: {
  section: BillingSectionKey;
}) => {
  const [data, setData] = useState<unknown>(null);
  const [overview, setOverview] = useState<Record<string, number> | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [saving, setSaving] = useState(false);
  const [dialog, setDialog] = useState<DialogState>(null);
  const [maintenance, setMaintenance] =
    useState<BillingMaintenanceResult | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const loaders: Record<BillingSectionKey, () => Promise<unknown>> = {
        'billing-accounts': listBillingAccounts,
        'billing-config': getBillingConfig,
        'billing-ledger': listBillingLedger,
        'billing-monitoring': listBillingUsageMonitoring,
        'billing-orders': listBillingOrders,
        'billing-packages': listBillingPackages,
        'billing-plans': listBillingPlans,
        'billing-pricing': listBillingModelPrices,
      };
      const [overviewData, sectionData] = await Promise.all([
        getBillingOverview(),
        loaders[section](),
      ]);
      setOverview(overviewData as unknown as Record<string, number>);
      setData(sectionData);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '加载失败，请重试');
    } finally {
      setLoading(false);
    }
  }, [section]);

  useEffect(() => {
    void load();
  }, [load]);

  const perform = async (action: () => Promise<unknown>, success: string) => {
    setSaving(true);
    setError('');
    setNotice('');
    try {
      await action();
      setDialog(null);
      setNotice(success);
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '操作失败，请重试');
    } finally {
      setSaving(false);
    }
  };

  const metrics = useMemo(
    () => [
      ['账单账户', overview?.Accounts ?? 0],
      ['订阅套餐', overview?.Plans ?? 0],
      ['积分包', overview?.Packages ?? 0],
      ['待支付订单', overview?.PendingOrders ?? 0],
      ['用量记录', overview?.UsageRecords ?? 0],
    ],
    [overview],
  );

  if (loading) {
    return (
      <div className="billing-state" role="status">
        <span className="billing-spinner" />
        正在加载订阅与积分数据...
      </div>
    );
  }

  if (error && !data) {
    return (
      <div className="billing-state is-error">
        <strong>加载失败</strong>
        <span>{error}</span>
        <button onClick={() => void load()}>重新加载</button>
      </div>
    );
  }

  const accounts = (data ?? []) as BillingAccount[];
  const sectionPanel = (() => {
    if (section === 'billing-config') {
      return (
        <div className="billing-admin-stack">
          <ConfigEditor
            value={data as BillingConfig}
            saving={saving}
            onSave={value =>
              perform(() => saveBillingConfig(value), '基础配置已保存')
            }
          />
          <MaintenancePanel
            result={maintenance}
            running={saving}
            onRun={async () => {
              setSaving(true);
              setError('');
              try {
                const result = await runBillingMaintenance();
                setMaintenance(result);
                setNotice('维护与对账已完成');
              } catch (cause) {
                setError(
                  cause instanceof Error ? cause.message : '维护任务执行失败',
                );
              } finally {
                setSaving(false);
              }
            }}
          />
        </div>
      );
    }
    if (section === 'billing-plans') {
      return (
        <DataPanel
          title="套餐列表"
          description="套餐采用版本化快照，历史订单不会被后续价格调整覆盖。"
          action="新增套餐"
          onAction={() => setDialog({ kind: 'plan' })}
          columns={['套餐', '周期', '价格', '周期积分', '状态', '版本']}
          rows={((data ?? []) as any[]).map(item => ({
            key: String(item.id),
            cells: [
              <NameCell
                key="name"
                name={item.name}
                description={item.description || item.key}
              />,
              item.cycle === 'yearly'
                ? '年度'
                : item.cycle === 'monthly'
                  ? '月度'
                  : '-',
              formatMoney(item.price_micros, item.currency),
              formatCredits(item.credit_grant_micros),
              <StatusPill key="status" value={item.status} />,
              `v${item.current_version}`,
            ],
          }))}
        />
      );
    }
    if (section === 'billing-packages') {
      return (
        <DataPanel
          title="积分包列表"
          description="一次购买按批次入账，优先消耗最早到期积分。"
          action="新增积分包"
          onAction={() => setDialog({ kind: 'package' })}
          columns={['积分包', '积分', '售价', '有效期', '状态']}
          rows={((data ?? []) as any[]).map(item => ({
            key: String(item.ID ?? item.id),
            cells: [
              <NameCell
                key="name"
                name={item.Name}
                description={item.Description || item.Key}
              />,
              formatCredits(item.CreditMicros),
              formatMoney(item.PriceMicros, item.Currency),
              item.ValidityDays ? `${item.ValidityDays} 天` : '永久',
              <StatusPill key="status" value={item.Status} />,
            ],
          }))}
        />
      );
    }
    if (section === 'billing-pricing') {
      return (
        <DataPanel
          title="模型定价版本"
          description="价格按每百万 Token 配置，新版本不会改写历史用量记录。"
          action="新增定价"
          onAction={() => setDialog({ kind: 'price' })}
          columns={[
            '模型',
            '版本',
            '输入',
            '输出',
            '缓存写入',
            '缓存命中',
            '生效时间',
          ]}
          rows={((data ?? []) as any[]).map((item, index) => ({
            key: billingRowKey('billing-price', item, index),
            cells: [
              <NameCell
                key="model"
                name={item.ModelID}
                description={item.Provider}
              />,
              `v${item.Version}`,
              formatCredits(item.InputPrice),
              formatCredits(item.OutputPrice),
              formatCredits(item.CacheWritePrice),
              formatCredits(item.CacheHitPrice),
              formatDate(item.EffectiveAt),
            ],
          }))}
        />
      );
    }
    if (section === 'billing-monitoring') {
      return (
        <DataPanel
          title="模型用量监控"
          description="按供应商和模型聚合真实结算记录。"
          columns={[
            '模型',
            '调用次数',
            '输入 Token',
            '输出 Token',
            '缓存命中',
            '积分消耗',
            '最近调用',
          ]}
          rows={((data ?? []) as any[]).map(item => ({
            key: `${item.provider}:${item.model_id}`,
            cells: [
              <NameCell
                key="model"
                name={item.model_id}
                description={item.provider}
              />,
              Number(item.calls).toLocaleString(),
              Number(item.input_tokens).toLocaleString(),
              Number(item.output_tokens).toLocaleString(),
              Number(item.cache_hit_tokens).toLocaleString(),
              formatCredits(item.charge_micros),
              formatDate(item.last_used_at),
            ],
          }))}
        />
      );
    }
    if (section === 'billing-accounts') {
      return (
        <DataPanel
          title="用户积分账户"
          description="所有人工调整均通过不可变流水完成并写入审计。"
          columns={['主体', '可用积分', '预占积分', '更新时间', '操作']}
          rows={accounts.map(item => ({
            key: item.id,
            cells: [
              <NameCell
                key="subject"
                name={`${item.subject_type} · ${item.subject_id}`}
                description={`账户 ${item.id}`}
              />,
              formatCredits(item.available_micros),
              formatCredits(item.reserved_micros),
              formatDate(item.updated_at),
              <button
                className="billing-link-button"
                key="adjust"
                onClick={() => setDialog({ kind: 'adjust', account: item })}
              >
                调整积分
              </button>,
            ],
          }))}
        />
      );
    }
    if (section === 'billing-ledger') {
      return (
        <DataPanel
          title="积分流水"
          description="收入、预占、结算和释放均保留业务号与账后余额。"
          columns={[
            '业务号',
            '账户',
            '类型',
            '方向',
            '积分',
            '账后余额',
            '发生时间',
          ]}
          rows={createBillingLedgerRows((data ?? []) as BillingLedger[])}
        />
      );
    }
    return (
      <DataPanel
        title="订单列表"
        description="支付、关闭和履约状态均由服务端状态机推进。"
        columns={[
          '订单号',
          '用户',
          '类型',
          '金额',
          '订单状态',
          '支付',
          '履约',
          '下单时间',
        ]}
        rows={((data ?? []) as any[]).map((item, index) => ({
          key: billingRowKey('billing-order', item, index),
          cells: [
            item.OrderNo,
            item.UserID,
            item.OrderType,
            formatMoney(item.TotalMicros, item.Currency),
            <StatusPill key="order" value={item.Status} />,
            <StatusPill key="payment" value={item.PaymentStatus} />,
            <StatusPill key="fulfillment" value={item.FulfillmentStatus} />,
            formatDate(item.CreatedAt),
          ],
        }))}
      />
    );
  })();

  return (
    <section className="billing-admin">
      <div className="billing-metrics">
        {metrics.map(([label, value]) => (
          <article key={String(label)}>
            <span>{label}</span>
            <strong>{Number(value).toLocaleString()}</strong>
          </article>
        ))}
      </div>
      {error ? <div className="billing-inline-error">{error}</div> : null}
      {notice ? <div className="billing-inline-success">{notice}</div> : null}
      {sectionPanel}
      <Modal
        className="billing-admin-modal"
        maskClosable={!saving}
        title={dialogTitle(dialog)}
        visible={Boolean(dialog)}
        footer={null}
        onCancel={() => !saving && setDialog(null)}
      >
        {dialog?.kind === 'plan' ? (
          <PlanForm
            saving={saving}
            onCancel={() => setDialog(null)}
            onSubmit={value =>
              perform(() => createBillingPlan(value), '订阅套餐已创建')
            }
          />
        ) : null}
        {dialog?.kind === 'package' ? (
          <PackageForm
            saving={saving}
            onCancel={() => setDialog(null)}
            onSubmit={value =>
              perform(() => createBillingPackage(value), '积分包已创建')
            }
          />
        ) : null}
        {dialog?.kind === 'price' ? (
          <PriceForm
            saving={saving}
            onCancel={() => setDialog(null)}
            onSubmit={value =>
              perform(() => createBillingModelPrice(value), '模型定价已创建')
            }
          />
        ) : null}
        {dialog?.kind === 'adjust' ? (
          <AdjustmentForm
            account={dialog.account}
            saving={saving}
            onCancel={() => setDialog(null)}
            onSubmit={value =>
              perform(() => adjustBillingCredits(value), '积分调整已完成')
            }
          />
        ) : null}
      </Modal>
    </section>
  );
};

const dialogTitle = (dialog: DialogState) =>
  dialog?.kind === 'plan'
    ? '新增订阅套餐'
    : dialog?.kind === 'package'
      ? '新增积分包'
      : dialog?.kind === 'price'
        ? '新增模型定价'
        : dialog?.kind === 'adjust'
          ? '调整用户积分'
          : '';

const ConfigEditor = ({
  value,
  saving,
  onSave,
}: {
  value: BillingConfig;
  saving: boolean;
  onSave: (value: BillingConfig) => Promise<void>;
}) => {
  const [draft, setDraft] = useState(value);
  return (
    <article className="billing-panel">
      <header>
        <div>
          <h2>基础配置</h2>
          <p>运行时结算和支付默认关闭，生产依赖就绪后再启用。</p>
        </div>
        <button disabled={saving} onClick={() => void onSave(draft)}>
          {saving ? '保存中...' : '保存配置'}
        </button>
      </header>
      <div className="billing-form-grid">
        <Field label="积分名称">
          <input
            value={draft.credit_name}
            onChange={event =>
              setDraft({ ...draft, credit_name: event.target.value })
            }
          />
        </Field>
        <Field label="显示精度">
          <input
            type="number"
            min={0}
            max={6}
            value={draft.display_scale}
            onChange={event =>
              setDraft({ ...draft, display_scale: Number(event.target.value) })
            }
          />
        </Field>
        <Field label="默认币种">
          <input
            maxLength={8}
            value={draft.default_currency}
            onChange={event =>
              setDraft({
                ...draft,
                default_currency: event.target.value.toUpperCase(),
              })
            }
          />
        </Field>
        <label className="billing-switch-row">
          <input
            type="checkbox"
            checked={draft.settlement_enabled}
            onChange={event =>
              setDraft({ ...draft, settlement_enabled: event.target.checked })
            }
          />
          <span>
            <strong>运行时结算</strong>
            <small>调用前预占，调用后按真实 Token 结算。</small>
          </span>
        </label>
        <label className="billing-switch-row">
          <input
            type="checkbox"
            checked={draft.payment_enabled}
            onChange={event =>
              setDraft({ ...draft, payment_enabled: event.target.checked })
            }
          />
          <span>
            <strong>在线支付</strong>
            <small>仅在支付网关及回调配置完成后开启。</small>
          </span>
        </label>
      </div>
    </article>
  );
};

const MaintenancePanel = ({
  result,
  running,
  onRun,
}: {
  result: BillingMaintenanceResult | null;
  running: boolean;
  onRun: () => Promise<void>;
}) => (
  <article className="billing-panel billing-maintenance-panel">
    <header>
      <div>
        <h2>维护与对账</h2>
        <p>释放超时预占、关闭过期订单、结束到期订阅并核对账户余额。</p>
      </div>
      <button disabled={running} onClick={() => void onRun()}>
        {running ? '执行中...' : '立即执行'}
      </button>
    </header>
    {result ? (
      <div className="billing-maintenance-result">
        <span>
          释放预占 <strong>{result.released_reservations}</strong>
        </span>
        <span>
          到期订阅 <strong>{result.expired_subscriptions}</strong>
        </span>
        <span>
          关闭订单 <strong>{result.closed_orders}</strong>
        </span>
        <span>
          账户核对 <strong>{result.reconciliation?.checked_count ?? 0}</strong>
        </span>
        <span data-alert={Boolean(result.reconciliation?.mismatch_count)}>
          差异 <strong>{result.reconciliation?.mismatch_count ?? 0}</strong>
        </span>
      </div>
    ) : (
      <div className="billing-maintenance-empty">
        系统定时任务默认关闭；启用前可在这里手动执行并确认结果。
      </div>
    )}
  </article>
);

const DataPanel = ({
  title,
  description,
  action,
  onAction,
  columns,
  rows,
}: {
  title: string;
  description: string;
  action?: string;
  onAction?: () => void;
  columns: string[];
  rows: TableRow[];
}) => (
  <article className="billing-panel">
    <header>
      <div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      {action ? <button onClick={onAction}>+ {action}</button> : null}
    </header>
    {rows.length ? (
      <div className="billing-table-wrap">
        <table>
          <thead>
            <tr>
              {columns.map(column => (
                <th key={column}>{column}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map(row => (
              <tr key={row.key}>
                {row.cells.map((cell, index) => (
                  <td key={`${row.key}:${columns[index]}`}>{cell}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    ) : (
      <div className="billing-empty">
        <strong>暂无数据</strong>
        <span>完成首条配置或交易后会显示在这里。</span>
      </div>
    )}
  </article>
);

const NameCell = ({
  name,
  description,
}: {
  name: string;
  description: string;
}) => (
  <span className="billing-name-cell">
    <strong>{name}</strong>
    <small>{description}</small>
  </span>
);

const StatusPill = ({ value }: { value?: string }) => (
  <span className="billing-status-pill" data-status={value}>
    {statusLabel(value)}
  </span>
);

const Field = ({
  label,
  hint,
  children,
  wide,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  wide?: boolean;
}) => (
  <label className="billing-field" data-wide={wide}>
    <span>{label}</span>
    {children}
    {hint ? <small>{hint}</small> : null}
  </label>
);

const FormActions = ({
  saving,
  onCancel,
}: {
  saving: boolean;
  onCancel: () => void;
}) => (
  <div className="billing-modal-actions">
    <button
      type="button"
      className="is-secondary"
      disabled={saving}
      onClick={onCancel}
    >
      取消
    </button>
    <button type="submit" disabled={saving}>
      {saving ? '保存中...' : '确认保存'}
    </button>
  </div>
);

const PlanForm = ({
  saving,
  onCancel,
  onSubmit,
}: {
  saving: boolean;
  onCancel: () => void;
  onSubmit: (value: Record<string, unknown>) => Promise<void>;
}) => {
  const [value, setValue] = useState({
    key: '',
    name: '',
    description: '',
    cycle: 'monthly',
    price: '0',
    credits: '0',
    currency: 'CNY',
    features: '',
    sortOrder: '0',
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    void onSubmit({
      Key: value.key.trim(),
      Name: value.name.trim(),
      Description: value.description.trim(),
      Cycle: value.cycle,
      PriceMicros: decimalToMicros(value.price),
      CreditGrantMicros: decimalToMicros(value.credits),
      Currency: value.currency.trim().toUpperCase(),
      FeaturesJSON: JSON.stringify(
        value.features
          .split('\n')
          .map(item => item.trim())
          .filter(Boolean),
      ),
      SortOrder: Number(value.sortOrder),
    });
  };
  return (
    <form className="billing-modal-form" onSubmit={submit}>
      <Field label="套餐标识">
        <input
          required
          maxLength={64}
          placeholder="pro-monthly"
          value={value.key}
          onChange={event => setValue({ ...value, key: event.target.value })}
        />
      </Field>
      <Field label="套餐名称">
        <input
          required
          maxLength={80}
          placeholder="专业版"
          value={value.name}
          onChange={event => setValue({ ...value, name: event.target.value })}
        />
      </Field>
      <Field label="计费周期">
        <select
          value={value.cycle}
          onChange={event => setValue({ ...value, cycle: event.target.value })}
        >
          <option value="monthly">月度</option>
          <option value="yearly">年度</option>
        </select>
      </Field>
      <Field label="售价" hint="按默认币种计价，最多 6 位小数。">
        <input
          required
          inputMode="decimal"
          value={value.price}
          onChange={event => setValue({ ...value, price: event.target.value })}
        />
      </Field>
      <Field label="周期积分">
        <input
          required
          inputMode="decimal"
          value={value.credits}
          onChange={event =>
            setValue({ ...value, credits: event.target.value })
          }
        />
      </Field>
      <Field label="币种">
        <input
          required
          maxLength={8}
          value={value.currency}
          onChange={event =>
            setValue({ ...value, currency: event.target.value })
          }
        />
      </Field>
      <Field label="排序">
        <input
          type="number"
          value={value.sortOrder}
          onChange={event =>
            setValue({ ...value, sortOrder: event.target.value })
          }
        />
      </Field>
      <Field label="套餐说明" wide>
        <textarea
          maxLength={512}
          rows={3}
          value={value.description}
          onChange={event =>
            setValue({ ...value, description: event.target.value })
          }
        />
      </Field>
      <Field label="权益列表" hint="每行一项权益，将作为订单快照保存。" wide>
        <textarea
          rows={4}
          placeholder={'更高并发额度\n优先支持'}
          value={value.features}
          onChange={event =>
            setValue({ ...value, features: event.target.value })
          }
        />
      </Field>
      <FormActions saving={saving} onCancel={onCancel} />
    </form>
  );
};

const PackageForm = ({
  saving,
  onCancel,
  onSubmit,
}: {
  saving: boolean;
  onCancel: () => void;
  onSubmit: (value: Record<string, unknown>) => Promise<void>;
}) => {
  const [value, setValue] = useState({
    key: '',
    name: '',
    description: '',
    price: '0',
    credits: '100',
    currency: 'CNY',
    validityDays: '365',
    purchaseLimit: '0',
    sortOrder: '0',
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    void onSubmit({
      Key: value.key.trim(),
      Name: value.name.trim(),
      Description: value.description.trim(),
      PriceMicros: decimalToMicros(value.price),
      CreditMicros: decimalToMicros(value.credits),
      Currency: value.currency.trim().toUpperCase(),
      ValidityDays: Number(value.validityDays),
      PurchaseLimit: Number(value.purchaseLimit),
      SortOrder: Number(value.sortOrder),
    });
  };
  return (
    <form className="billing-modal-form" onSubmit={submit}>
      <Field label="积分包标识">
        <input
          required
          maxLength={64}
          placeholder="credits-100"
          value={value.key}
          onChange={event => setValue({ ...value, key: event.target.value })}
        />
      </Field>
      <Field label="积分包名称">
        <input
          required
          maxLength={80}
          placeholder="100 积分包"
          value={value.name}
          onChange={event => setValue({ ...value, name: event.target.value })}
        />
      </Field>
      <Field label="售价">
        <input
          required
          inputMode="decimal"
          value={value.price}
          onChange={event => setValue({ ...value, price: event.target.value })}
        />
      </Field>
      <Field label="积分数量">
        <input
          required
          inputMode="decimal"
          value={value.credits}
          onChange={event =>
            setValue({ ...value, credits: event.target.value })
          }
        />
      </Field>
      <Field label="有效天数" hint="0 表示永久有效。">
        <input
          required
          type="number"
          min={0}
          value={value.validityDays}
          onChange={event =>
            setValue({ ...value, validityDays: event.target.value })
          }
        />
      </Field>
      <Field label="购买上限" hint="0 表示不限制。">
        <input
          required
          type="number"
          min={0}
          value={value.purchaseLimit}
          onChange={event =>
            setValue({ ...value, purchaseLimit: event.target.value })
          }
        />
      </Field>
      <Field label="币种">
        <input
          required
          maxLength={8}
          value={value.currency}
          onChange={event =>
            setValue({ ...value, currency: event.target.value })
          }
        />
      </Field>
      <Field label="排序">
        <input
          type="number"
          value={value.sortOrder}
          onChange={event =>
            setValue({ ...value, sortOrder: event.target.value })
          }
        />
      </Field>
      <Field label="说明" wide>
        <textarea
          maxLength={512}
          rows={3}
          value={value.description}
          onChange={event =>
            setValue({ ...value, description: event.target.value })
          }
        />
      </Field>
      <FormActions saving={saving} onCancel={onCancel} />
    </form>
  );
};

const PriceForm = ({
  saving,
  onCancel,
  onSubmit,
}: {
  saving: boolean;
  onCancel: () => void;
  onSubmit: (value: Record<string, unknown>) => Promise<void>;
}) => {
  const [value, setValue] = useState({
    provider: '',
    modelID: '',
    input: '0',
    output: '0',
    cacheWrite: '0',
    cacheHit: '0',
    effectiveAt: '',
  });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    void onSubmit({
      Provider: value.provider.trim(),
      ModelID: value.modelID.trim(),
      Currency: 'CREDIT',
      InputPrice: decimalToMicros(value.input),
      OutputPrice: decimalToMicros(value.output),
      CacheWritePrice: decimalToMicros(value.cacheWrite),
      CacheHitPrice: decimalToMicros(value.cacheHit),
      EffectiveAt: value.effectiveAt
        ? new Date(value.effectiveAt).toISOString()
        : new Date().toISOString(),
    });
  };
  return (
    <form className="billing-modal-form" onSubmit={submit}>
      <Field label="供应商">
        <input
          required
          maxLength={64}
          placeholder="deepseek"
          value={value.provider}
          onChange={event =>
            setValue({ ...value, provider: event.target.value })
          }
        />
      </Field>
      <Field label="模型标识">
        <input
          required
          maxLength={128}
          placeholder="deepseek-v4-pro"
          value={value.modelID}
          onChange={event =>
            setValue({ ...value, modelID: event.target.value })
          }
        />
      </Field>
      <Field label="输入价格" hint="每百万 Token 消耗积分。">
        <input
          required
          inputMode="decimal"
          value={value.input}
          onChange={event => setValue({ ...value, input: event.target.value })}
        />
      </Field>
      <Field label="输出价格">
        <input
          required
          inputMode="decimal"
          value={value.output}
          onChange={event => setValue({ ...value, output: event.target.value })}
        />
      </Field>
      <Field label="缓存写入价格">
        <input
          required
          inputMode="decimal"
          value={value.cacheWrite}
          onChange={event =>
            setValue({ ...value, cacheWrite: event.target.value })
          }
        />
      </Field>
      <Field label="缓存命中价格">
        <input
          required
          inputMode="decimal"
          value={value.cacheHit}
          onChange={event =>
            setValue({ ...value, cacheHit: event.target.value })
          }
        />
      </Field>
      <Field label="生效时间" hint="留空立即生效。" wide>
        <input
          type="datetime-local"
          value={value.effectiveAt}
          onChange={event =>
            setValue({ ...value, effectiveAt: event.target.value })
          }
        />
      </Field>
      <FormActions saving={saving} onCancel={onCancel} />
    </form>
  );
};

const AdjustmentForm = ({
  account,
  saving,
  onCancel,
  onSubmit,
}: {
  account: BillingAccount;
  saving: boolean;
  onCancel: () => void;
  onSubmit: (value: {
    subject_type: string;
    subject_id: string;
    amount_micros: number;
    business_no: string;
    reason: string;
  }) => Promise<void>;
}) => {
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const amountMicros = decimalToMicros(amount);
    if (!amountMicros) {
      return;
    }
    void onSubmit({
      subject_type: account.subject_type,
      subject_id: account.subject_id,
      amount_micros: amountMicros,
      business_no: `adjust_${Date.now()}_${crypto.randomUUID().slice(0, 8)}`,
      reason: reason.trim(),
    });
  };
  return (
    <form className="billing-modal-form" onSubmit={submit}>
      <div className="billing-adjust-account">
        <span>调整账户</span>
        <strong>
          {account.subject_type} · {account.subject_id}
        </strong>
        <small>
          当前可用 {formatCredits(account.available_micros)}，预占{' '}
          {formatCredits(account.reserved_micros)}
        </small>
      </div>
      <Field
        label="调整积分"
        hint="正数增加，负数扣减；扣减不能超过可用余额。"
        wide
      >
        <input
          required
          inputMode="decimal"
          placeholder="例如 100 或 -25"
          value={amount}
          onChange={event => setAmount(event.target.value)}
        />
      </Field>
      <Field label="调整原因" hint="原因将写入不可变流水和审计记录。" wide>
        <textarea
          required
          maxLength={500}
          rows={4}
          value={reason}
          onChange={event => setReason(event.target.value)}
        />
      </Field>
      <FormActions saving={saving} onCancel={onCancel} />
    </form>
  );
};
