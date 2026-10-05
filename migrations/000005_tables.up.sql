-- Floor plan: the tables a store serves at, so the POS can show which ones
-- are occupied. Orders keep table_label (printed on tickets) and now also
-- point at the table they belong to.

CREATE TABLE store_tables (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id    uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name        text NOT NULL,                 -- "Bàn 05"
    zone        text NOT NULL DEFAULT '',      -- "Trong nhà" / "Ngoài sân"
    seats       smallint NOT NULL DEFAULT 2 CHECK (seats > 0),
    sort_order  integer NOT NULL DEFAULT 0,
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (store_id, name),
    UNIQUE (id, store_id)                      -- lets orders enforce same-store
);

CREATE INDEX store_tables_store_idx ON store_tables (store_id, sort_order);

ALTER TABLE orders
    ADD COLUMN table_id uuid,
    ADD FOREIGN KEY (table_id, store_id) REFERENCES store_tables(id, store_id) ON DELETE SET NULL;

CREATE INDEX orders_table_open_idx ON orders (table_id) WHERE status IN ('open', 'preparing', 'ready');
