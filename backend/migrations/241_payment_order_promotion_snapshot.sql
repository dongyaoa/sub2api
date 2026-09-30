-- Immutable recharge promotion amounts, separate from provider configuration.
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS promotion_snapshot JSONB;
CREATE INDEX IF NOT EXISTS paymentorder_recharge_code ON payment_orders (recharge_code);
