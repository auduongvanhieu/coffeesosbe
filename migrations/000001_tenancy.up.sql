-- Tenancy foundation: brands -> stores, roles, users.
-- Every business table in CoffeeSOS carries brand_id (and store_id where it
-- makes sense) from day one.

CREATE TABLE brands (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    slogan      text,
    logo_url    text,
    theme       jsonb NOT NULL DEFAULT '{}'::jsonb,   -- {primary, primaryLight, success, ...}
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE stores (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id    uuid NOT NULL REFERENCES brands(id) ON DELETE CASCADE,
    code        text NOT NULL,
    name        text NOT NULL,
    address     text,
    phone       text,
    timezone    text NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (brand_id, code),
    UNIQUE (id, brand_id)            -- lets child tables enforce same-brand via composite FK
);

CREATE INDEX stores_brand_idx ON stores (brand_id);

-- Role hierarchy. Higher level = more power. Middleware compares levels.
CREATE TABLE roles (
    code   text PRIMARY KEY,
    name   text NOT NULL,
    level  smallint NOT NULL UNIQUE
);

INSERT INTO roles (code, name, level) VALUES
    ('staff',          'Nhân viên',            10),
    ('store_manager',  'Quản lý cửa hàng',     20),
    ('brand_owner',    'Chủ thương hiệu',      30),
    ('platform_admin', 'Quản trị nền tảng',   100);

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_id       uuid REFERENCES brands(id) ON DELETE CASCADE,   -- NULL only for platform_admin
    store_id       uuid,                                           -- NULL for brand-level users
    role_code      text NOT NULL REFERENCES roles(code),
    email          text,
    phone          text,
    full_name      text NOT NULL,
    password_hash  text,
    is_active      boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    -- a store user must belong to a store of the same brand
    FOREIGN KEY (store_id, brand_id) REFERENCES stores(id, brand_id) ON DELETE SET NULL,
    CHECK (role_code = 'platform_admin' OR brand_id IS NOT NULL)
);

CREATE UNIQUE INDEX users_email_uq ON users (lower(email)) WHERE email IS NOT NULL;
CREATE INDEX users_brand_idx ON users (brand_id);
CREATE INDEX users_store_idx ON users (store_id);
