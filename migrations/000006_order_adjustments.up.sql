-- Audit trail for corrected bills. Staff sometimes ring up the wrong item or
-- quantity; the order is edited in place, but every edit is recorded here so
-- the shift totals can always be explained.
CREATE TABLE order_adjustments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    reason      text NOT NULL,
    old_total   bigint NOT NULL,
    new_total   bigint NOT NULL,
    difference  bigint NOT NULL,            -- new - old: > 0 collect more, < 0 refund
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX order_adjustments_order_idx ON order_adjustments (order_id, created_at);
