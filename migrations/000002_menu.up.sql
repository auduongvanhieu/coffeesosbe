-- Menu belongs to a brand. Stores inherit it and may override price/availability.

CREATE TABLE menu_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id    uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    name        text NOT NULL,
    sort_order  integer NOT NULL DEFAULT 0,
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, brand_id)
);

CREATE INDEX menu_categories_brand_idx ON menu_categories (brand_id, sort_order);

CREATE TABLE menu_items (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id      uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    category_id   uuid NOT NULL,
    name          text NOT NULL,
    description   text,
    image_url     text,
    base_price    bigint NOT NULL CHECK (base_price >= 0),      -- VND, no decimals
    -- option groups: size / ice / sugar / topping ...
    -- [{"code":"size","name":"Size","type":"single","required":true,
    --   "choices":[{"code":"m","name":"M","price_delta":0},{"code":"l","name":"L","price_delta":5000}]}]
    options       jsonb NOT NULL DEFAULT '[]'::jsonb,
    is_available  boolean NOT NULL DEFAULT true,
    sort_order    integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    -- category must belong to the same brand as the item
    FOREIGN KEY (category_id, brand_id) REFERENCES menu_categories(id, brand_id) ON DELETE RESTRICT
);

CREATE INDEX menu_items_brand_cat_idx ON menu_items (brand_id, category_id, sort_order);

-- Per-store overrides (price, sold out). Absence of a row = inherit brand defaults.
CREATE TABLE store_menu_items (
    store_id        uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    item_id         uuid NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
    price_override  bigint CHECK (price_override IS NULL OR price_override >= 0),
    is_available    boolean NOT NULL DEFAULT true,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, item_id)
);
