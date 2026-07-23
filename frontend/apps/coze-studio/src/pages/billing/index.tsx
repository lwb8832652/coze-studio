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

/* eslint-disable @coze-arch/max-line-per-function, max-lines -- Billing tabs share one cohesive customer account surface. */

import { useNavigate, useParams } from 'react-router-dom';
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';

import {
  checkoutOrder,
  createOrder,
  getAccount,
  getBillingConfig,
  getSubscription,
  listLedger,
  listOrders,
  listPackages,
  listPlans,
  listUsage,
  type Balance,
  type CreditPackage,
  type Ledger,
  type Order,
  type Plan,
  type PublicBillingConfig,
  type Subscription,
  type Usage,
} from './service';
import './style.less';

type Section = 'subscriptions' | 'credits' | 'usage' | 'orders';

const sections: Array<{ key: Section; label: string; hint: string }> = [
  { key: 'subscriptions', label: '我的订阅', hint: '套餐与积分包' },
  { key: 'credits', label: '积分明细', hint: '余额与流水' },
  { key: 'usage', label: '用量统计', hint: '模型与 Token' },
  { key: 'orders', label: '我的订单', hint: '支付与履约' },
];

const credits = (value = 0) =>
  (value / 1_000_000).toLocaleString('zh-CN', { maximumFractionDigits: 6 });
const money = (value = 0, currency = 'CNY') =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency }).format(
    value / 1_000_000,
  );
const date = (value?: string) =>
  value ? new Date(value).toLocaleString('zh-CN') : '-';
const label = (value?: string) =>
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
    succeeded: '成功',
  })[value ?? ''] ??
  value ??
  '-';

const BillingCenterPage = () => {
  const { section } = useParams();
  const navigate = useNavigate();
  const active = (
    sections.some(item => item.key === section) ? section : 'subscriptions'
  ) as Section;
  const [config, setConfig] = useState<PublicBillingConfig | null>(null);
  const [balance, setBalance] = useState<Balance | null>(null);
  const [subscription, setSubscription] = useState<Subscription | null>(null);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [packages, setPackages] = useState<CreditPackage[]>([]);
  const [ledger, setLedger] = useState<Ledger[]>([]);
  const [usage, setUsage] = useState<Usage[]>([]);
  const [orders, setOrders] = useState<Order[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [ordering, setOrdering] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [
        billingConfig,
        account,
        currentSubscription,
        planItems,
        packageItems,
        ledgerItems,
        usageItems,
        orderItems,
      ] = await Promise.all([
        getBillingConfig(),
        getAccount(),
        getSubscription(),
        listPlans(),
        listPackages(),
        listLedger(),
        listUsage(),
        listOrders(),
      ]);
      setConfig(billingConfig);
      setBalance(account);
      setSubscription(currentSubscription);
      setPlans(planItems);
      setPackages(packageItems);
      setLedger(ledgerItems);
      setUsage(usageItems);
      setOrders(orderItems);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const continueCheckout = async (
    order: Pick<Order, 'OrderNo' | 'TotalMicros'>,
  ) => {
    setOrdering(order.OrderNo);
    setError('');
    setNotice('');
    try {
      const checkout = await checkoutOrder(order.OrderNo);
      if (checkout.checkout_url) {
        window.location.assign(checkout.checkout_url);
        return;
      }
      await load();
      setNotice(
        order.TotalMicros === 0 ? '免费订单已领取并到账' : '订单已完成',
      );
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : '支付服务暂时不可用，请稍后重试',
      );
    } finally {
      setOrdering('');
    }
  };

  const buy = async (
    type: 'subscription' | 'credit_package',
    id: string,
    priceMicros: number,
  ) => {
    setOrdering(id);
    setError('');
    setNotice('');
    try {
      const order = await createOrder(type, id);
      navigate('/billing/orders');
      await continueCheckout({
        OrderNo: order.OrderNo,
        TotalMicros: priceMicros,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '创建订单失败');
    } finally {
      setOrdering('');
    }
  };

  const summary = useMemo(
    () => ({
      calls: usage.length,
      charge: usage.reduce((sum, row) => sum + row.ChargeMicros, 0),
      input: usage.reduce((sum, row) => sum + row.InputTokens, 0),
    }),
    [usage],
  );

  return (
    <main className="customer-billing">
      <aside>
        <button className="billing-back" onClick={() => navigate(-1)}>
          返回工作台
        </button>
        <div className="billing-brand">
          <span>NEWX ACCOUNT</span>
          <h1>积分与订阅</h1>
          <p>清楚掌握套餐、余额和每一次模型调用。</p>
        </div>
        <nav aria-label="积分与订阅导航">
          {sections.map(item => (
            <button
              key={item.key}
              data-active={active === item.key}
              onClick={() => navigate(`/billing/${item.key}`)}
            >
              <strong>{item.label}</strong>
              <span>{item.hint}</span>
            </button>
          ))}
        </nav>
        <div className="billing-balance-mini">
          <span>可用{config?.credit_name ?? '积分'}</span>
          <strong>{credits(balance?.AvailableMicros)}</strong>
          <small>执行中预占 {credits(balance?.ReservedMicros)}</small>
        </div>
      </aside>
      <section className="billing-main">
        <header>
          <div>
            <span>ACCOUNT CENTER</span>
            <h2>{sections.find(item => item.key === active)?.label}</h2>
          </div>
          <button onClick={() => void load()}>刷新数据</button>
        </header>
        {error ? (
          <div className="billing-alert">
            <span>{error}</span>
            <button onClick={() => void load()}>重试</button>
          </div>
        ) : null}
        {notice ? <div className="billing-notice">{notice}</div> : null}
        {loading ? (
          <div className="billing-loading">
            <span className="billing-customer-spinner" />
            正在同步账务数据...
          </div>
        ) : (
          <>
            {active === 'subscriptions' ? (
              <Subscriptions
                config={config}
                current={subscription}
                plans={plans}
                packages={packages}
                ordering={ordering}
                buy={buy}
              />
            ) : null}
            {active === 'credits' ? (
              <Credits balance={balance} rows={ledger} />
            ) : null}
            {active === 'usage' ? (
              <UsagePanel rows={usage} summary={summary} />
            ) : null}
            {active === 'orders' ? (
              <Orders
                config={config}
                rows={orders}
                ordering={ordering}
                onCheckout={continueCheckout}
              />
            ) : null}
          </>
        )}
      </section>
    </main>
  );
};

const Subscriptions = ({
  config,
  current,
  plans,
  packages,
  ordering,
  buy,
}: {
  config: PublicBillingConfig | null;
  current: Subscription | null;
  plans: Plan[];
  packages: CreditPackage[];
  ordering: string;
  buy: (
    type: 'subscription' | 'credit_package',
    id: string,
    priceMicros: number,
  ) => void;
}) => (
  <div className="billing-stack">
    <article className="current-plan">
      <div>
        <span>当前套餐</span>
        <h3>{current?.PlanName ?? '免费版'}</h3>
        <p>
          {current
            ? `有效期至 ${date(current.CurrentPeriodEnd)}`
            : '选择套餐后即可获得本周期积分。'}
        </p>
      </div>
      <em>{current ? label(current.Status) : '可升级'}</em>
    </article>
    <div className="billing-heading">
      <h3>订阅套餐</h3>
      <p>价格和权益在下单时保存快照，后续调整不会影响历史订单。</p>
    </div>
    <div className="plan-grid">
      {plans.map(plan => {
        const enabled =
          plan.PriceMicros === 0 || Boolean(config?.payment_enabled);
        return (
          <article key={plan.id}>
            <span>{plan.Cycle === 'yearly' ? '年度方案' : '月度方案'}</span>
            <h3>{plan.Name}</h3>
            <p>{plan.Description || '稳定的周期额度，适合持续使用。'}</p>
            <strong>
              {money(plan.PriceMicros, plan.Currency)}
              <small> / {plan.Cycle === 'yearly' ? '年' : '月'}</small>
            </strong>
            <ul>
              <li>每周期 {credits(plan.CreditGrantMicros)} 积分</li>
              <li>价格与权益版本可追溯</li>
              <li>到期后可手动续订</li>
            </ul>
            <button
              disabled={!enabled || ordering === plan.id}
              onClick={() => buy('subscription', plan.id, plan.PriceMicros)}
            >
              {ordering === plan.id
                ? '处理中...'
                : enabled
                  ? '选择套餐'
                  : '支付暂未开放'}
            </button>
          </article>
        );
      })}
      {!plans.length ? <Empty text="暂无可订阅套餐" /> : null}
    </div>
    <div className="billing-heading">
      <h3>积分包</h3>
      <p>一次购买，按有效期使用，优先消耗最早到期积分。</p>
    </div>
    <div className="package-grid">
      {packages.map(item => {
        const enabled =
          item.PriceMicros === 0 || Boolean(config?.payment_enabled);
        return (
          <article key={item.id}>
            <div>
              <h3>{item.Name}</h3>
              <p>
                {item.Description ||
                  (item.ValidityDays
                    ? `${item.ValidityDays} 天有效`
                    : '永久有效')}
              </p>
            </div>
            <strong>
              {credits(item.CreditMicros)}
              <small> 积分</small>
            </strong>
            <span>{money(item.PriceMicros, item.Currency)}</span>
            <button
              disabled={!enabled || ordering === item.id}
              onClick={() => buy('credit_package', item.id, item.PriceMicros)}
            >
              {ordering === item.id
                ? '处理中...'
                : enabled
                  ? '购买'
                  : '支付暂未开放'}
            </button>
          </article>
        );
      })}
      {!packages.length ? <Empty text="暂无可购买积分包" /> : null}
    </div>
  </div>
);

const Credits = ({
  balance,
  rows,
}: {
  balance: Balance | null;
  rows: Ledger[];
}) => (
  <div className="billing-stack">
    <div className="credit-summary">
      <article>
        <span>可用积分</span>
        <strong>{credits(balance?.AvailableMicros)}</strong>
      </article>
      <article>
        <span>执行中预占</span>
        <strong>{credits(balance?.ReservedMicros)}</strong>
      </article>
      <article>
        <span>账户版本</span>
        <strong>v{balance?.Version ?? 1}</strong>
      </article>
    </div>
    <Table
      columns={['时间', '类型', '方向', '积分', '业务号', '账后余额']}
      rows={rows.map(row => ({
        key: row.id,
        cells: [
          date(row.CreatedAt),
          row.EntryType,
          row.Direction,
          credits(row.AmountMicros),
          row.BusinessNo,
          credits(row.AvailableAfterMicros),
        ],
      }))}
    />
  </div>
);

const UsagePanel = ({
  rows,
  summary,
}: {
  rows: Usage[];
  summary: { calls: number; input: number; charge: number };
}) => (
  <div className="billing-stack">
    <div className="credit-summary">
      <article>
        <span>调用次数</span>
        <strong>{summary.calls.toLocaleString()}</strong>
      </article>
      <article>
        <span>输入 Token</span>
        <strong>{summary.input.toLocaleString()}</strong>
      </article>
      <article>
        <span>消耗积分</span>
        <strong>{credits(summary.charge)}</strong>
      </article>
    </div>
    <Table
      columns={['时间', '模型', '输入', '输出', '缓存命中', '积分']}
      rows={rows.map(row => ({
        key: row.id,
        cells: [
          date(row.CreatedAt),
          <span className="billing-model-cell" key="model">
            <strong>{row.ModelID}</strong>
            <small>{row.Provider}</small>
          </span>,
          row.InputTokens.toLocaleString(),
          row.OutputTokens.toLocaleString(),
          row.CacheHitTokens.toLocaleString(),
          credits(row.ChargeMicros),
        ],
      }))}
    />
  </div>
);

const Orders = ({
  config,
  rows,
  ordering,
  onCheckout,
}: {
  config: PublicBillingConfig | null;
  rows: Order[];
  ordering: string;
  onCheckout: (order: Pick<Order, 'OrderNo' | 'TotalMicros'>) => Promise<void>;
}) => (
  <Table
    columns={[
      '下单时间',
      '订单号',
      '类型',
      '金额',
      '订单',
      '支付',
      '履约',
      '操作',
    ]}
    rows={rows.map(row => {
      const canCheckout =
        row.PaymentStatus === 'pending' &&
        (row.TotalMicros === 0 || Boolean(config?.payment_enabled));
      return {
        key: row.id,
        cells: [
          date(row.CreatedAt),
          row.OrderNo,
          row.OrderType,
          money(row.TotalMicros, row.Currency),
          <Status key="order" value={row.Status} />,
          <Status key="payment" value={row.PaymentStatus} />,
          <Status key="fulfillment" value={row.FulfillmentStatus} />,
          row.PaymentStatus === 'pending' ? (
            <button
              className="billing-table-action"
              disabled={!canCheckout || ordering === row.OrderNo}
              onClick={() => void onCheckout(row)}
              key="action"
            >
              {ordering === row.OrderNo
                ? '处理中...'
                : canCheckout
                  ? row.TotalMicros === 0
                    ? '完成领取'
                    : '继续支付'
                  : '支付暂未开放'}
            </button>
          ) : (
            <span key="done">-</span>
          ),
        ],
      };
    })}
  />
);

const Status = ({ value }: { value: string }) => (
  <span className="customer-status" data-status={value}>
    {label(value)}
  </span>
);
const Table = ({
  columns,
  rows,
}: {
  columns: string[];
  rows: Array<{ key: string; cells: ReactNode[] }>;
}) =>
  rows.length ? (
    <div className="customer-table">
      <table>
        <thead>
          <tr>
            {columns.map(item => (
              <th key={item}>{item}</th>
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
    <Empty text="暂无相关记录" />
  );
const Empty = ({ text }: { text: string }) => (
  <div className="customer-empty">
    <strong>{text}</strong>
    <span>产生数据后会自动显示在这里。</span>
  </div>
);

export default BillingCenterPage;
