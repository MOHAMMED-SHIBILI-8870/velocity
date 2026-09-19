CREATE TABLE IF NOT EXISTS wallet_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id BIGINT NOT NULL
        REFERENCES users(id)
        ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL, -- 'DEPOSIT', 'WITHDRAWAL', 'ORDER_LOCK', 'ORDER_FILL', 'ORDER_CANCEL_UNLOCK', 'MARKETPLACE_BUY'
    asset VARCHAR(16) NOT NULL DEFAULT 'INR',
    amount BIGINT NOT NULL,    -- in smallest units: paise for INR (10000 = Rs 100.00)
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING', -- 'PENDING', 'COMPLETED', 'FAILED', 'REVERSED', 'CANCELLED'
    gateway VARCHAR(32) NOT NULL DEFAULT 'RAZORPAY',
    gateway_order_id TEXT,
    gateway_payment_id TEXT,
    payout_id TEXT,
    trade_id BIGINT REFERENCES trades(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_trade ON wallet_transactions(trade_id);

CREATE INDEX IF NOT EXISTS idx_wallet_transactions_user_id ON wallet_transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transactions_created_at ON wallet_transactions(created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transactions_payment_id 
    ON wallet_transactions(gateway_payment_id) 
    WHERE gateway_payment_id IS NOT NULL AND gateway_payment_id != '';
