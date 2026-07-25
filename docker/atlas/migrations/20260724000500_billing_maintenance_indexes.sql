-- Active subscription maintenance scans filter by status and period boundary,
-- then advance deterministically by period and id.
ALTER TABLE `user_subscriptions`
  ADD INDEX `idx_user_subscription_maintenance` (`status`, `current_period_end`, `id`);

-- The existing status/expiry index does not cover payment and fulfillment
-- predicates used by the timeout worker.
ALTER TABLE `billing_orders`
  ADD INDEX `idx_billing_order_timeout_maintenance`
    (`status`, `payment_status`, `fulfillment_status`, `expires_at`, `id`);
