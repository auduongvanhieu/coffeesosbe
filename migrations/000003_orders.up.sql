-- Orders, payments, customers (loyalty), promotions, staff PIN, store bank details.

-- Staff log into the POS with a short PIN once the device knows its store.
ALTER TABLE users ADD COLUMN pin_hash text;

-- Bank account shown as a VietQR code on the payment screen.
ALTER TABLE stores
    ADD COLUMN bank_bin      text,   -- NAPAS BIN, e.g. 970436 (Vietcombank)
    ADD COLUMN bank_code     text,   -- short code used by img.vietqr.io, e.g. VCB
    ADD COLUMN bank_account  text,
    ADD COLUMN bank_holder   text,
    ADD COLUMN next_order_no integer NOT NULL DEFAULT 1;   -- per-store running number

CREATE TABLE customers (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id    uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    phone       text NOT NULL,
    name        text NOT NULL DEFAULT '',
    points      integer NOT NULL DEFAULT 0 CHECK (points >= 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (brand_id, phone),
    UNIQUE (id, brand_id)
);

CREATE TABLE promotions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id      uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    code          text NOT NULL,
    name          text NOT NULL,
    type          text NOT NULL CHECK (type IN ('percent', 'fixed')),
    value         bigint NOT NULL CHECK (value >= 0),      -- percent (0-100) or VND
    min_subtotal  bigint NOT NULL DEFAULT 0,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (brand_id, code)
);

CREATE TABLE orders (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id        uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    store_id        uuid NOT NULL,
    order_no        integer NOT NULL,
    number          text NOT NULL,                          -- "A-1042" (app) / "P-0012" (pos)
    source          text NOT NULL CHECK (source IN ('pos', 'app')),
    order_type      text NOT NULL CHECK (order_type IN ('dine_in', 'takeaway', 'pickup')),
    table_label     text,
    status          text NOT NULL CHECK (status IN ('open', 'pending', 'preparing', 'ready', 'completed', 'rejected', 'cancelled')),
    payment_status  text NOT NULL DEFAULT 'unpaid' CHECK (payment_status IN ('unpaid', 'paid')),
    payment_method  text CHECK (payment_method IS NULL OR payment_method IN ('cash', 'vietqr', 'momo', 'zalopay')),
    cash_received   bigint,
    change_due      bigint,
    customer_id     uuid,
    customer_name   text,
    customer_phone  text,
    promotion_code  text,
    subtotal        bigint NOT NULL DEFAULT 0,
    discount        bigint NOT NULL DEFAULT 0,
    total           bigint NOT NULL DEFAULT 0,
    points_earned   integer NOT NULL DEFAULT 0,
    note            text,
    created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    paid_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (store_id, brand_id) REFERENCES stores(id, brand_id) ON DELETE CASCADE,
    FOREIGN KEY (customer_id, brand_id) REFERENCES customers(id, brand_id) ON DELETE SET NULL,
    UNIQUE (store_id, number)
);

CREATE INDEX orders_store_created_idx ON orders (store_id, created_at DESC);
CREATE INDEX orders_store_status_idx  ON orders (store_id, status);

CREATE TABLE order_items (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    item_id       uuid REFERENCES menu_items(id) ON DELETE SET NULL,
    name          text NOT NULL,                              -- snapshot at order time
    quantity      integer NOT NULL CHECK (quantity > 0),
    unit_price    bigint NOT NULL CHECK (unit_price >= 0),    -- base price + chosen option deltas
    line_total    bigint NOT NULL CHECK (line_total >= 0),
    -- [{"group":"size","groupName":"Size","code":"l","name":"L","priceDelta":5000}]
    choices       jsonb NOT NULL DEFAULT '[]'::jsonb,
    options_text  text NOT NULL DEFAULT '',                   -- "Size L · Ít đá"
    note          text,
    sort_order    integer NOT NULL DEFAULT 0
);

CREATE INDEX order_items_order_idx ON order_items (order_id, sort_order);
