BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'service_level_t') THEN
        CREATE TYPE service_level_t AS ENUM ('standard', 'express');
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_status_t') THEN
        CREATE TYPE order_status_t AS ENUM (
            'created',
            'assigned',
            'in_transit',
            'delivered',
            'cancelled'
        );
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'status_source_t') THEN
        CREATE TYPE status_source_t AS ENUM ('system', 'courier', 'manager', 'carrier_sync');
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vehicle_type_t') THEN
        CREATE TYPE vehicle_type_t AS ENUM ('foot', 'bike', 'car', 'van', 'truck');
    END IF;
END
$$;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    city TEXT NOT NULL,
    street TEXT NOT NULL,
    house TEXT NOT NULL,
    apartment TEXT,
    lat NUMERIC(9,6) NOT NULL,
    lng NUMERIC(9,6) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_addresses_city_not_blank CHECK (btrim(city) <> ''),
    CONSTRAINT chk_addresses_street_not_blank CHECK (btrim(street) <> ''),
    CONSTRAINT chk_addresses_house_not_blank CHECK (btrim(house) <> ''),
    CONSTRAINT chk_addresses_lat_range CHECK (lat >= -90 AND lat <= 90),
    CONSTRAINT chk_addresses_lng_range CHECK (lng >= -180 AND lng <= 180)
);

CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL,
    pickup_address_id UUID NOT NULL REFERENCES addresses(id) ON DELETE RESTRICT,
    dropoff_address_id UUID NOT NULL REFERENCES addresses(id) ON DELETE RESTRICT,
    weight_kg NUMERIC(8,2) NOT NULL,
    distance_km NUMERIC(8,2) NOT NULL,
    service_level service_level_t NOT NULL DEFAULT 'standard',
    status order_status_t NOT NULL DEFAULT 'created',
    idempotency_key TEXT NOT NULL,
    last_status_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_orders_idempotency_key UNIQUE (idempotency_key),
    CONSTRAINT chk_orders_idempotency_key_not_blank CHECK (btrim(idempotency_key) <> ''),
    CONSTRAINT chk_orders_weight_positive CHECK (weight_kg > 0),
    CONSTRAINT chk_orders_distance_positive CHECK (distance_km >= 0),
    CONSTRAINT chk_orders_pickup_dropoff_diff CHECK (pickup_address_id <> dropoff_address_id),
    CONSTRAINT chk_orders_last_status_at_not_before_created CHECK (last_status_at >= created_at)
);

CREATE TABLE IF NOT EXISTS couriers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    phone TEXT,
    vehicle_type vehicle_type_t NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_couriers_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT chk_couriers_phone_not_blank CHECK (phone IS NULL OR btrim(phone) <> ''),
    CONSTRAINT uq_couriers_phone UNIQUE (phone)
);

CREATE TABLE IF NOT EXISTS assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    courier_id UUID NOT NULL REFERENCES couriers(id) ON DELETE RESTRICT,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_by TEXT,
    comment TEXT,
    CONSTRAINT chk_assignments_assigned_by_not_blank CHECK (assigned_by IS NULL OR btrim(assigned_by) <> ''),
    CONSTRAINT uq_assignments_order_id UNIQUE (order_id)
);

CREATE TABLE IF NOT EXISTS order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    status order_status_t NOT NULL,
    source status_source_t NOT NULL,
    comment TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_order_status_history_comment_not_blank CHECK (comment IS NULL OR btrim(comment) <> '')
);

CREATE INDEX IF NOT EXISTS idx_orders_status_created_at
    ON orders(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_customer_created_at
    ON orders(customer_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_orders_pickup_address_id
    ON orders(pickup_address_id);

CREATE INDEX IF NOT EXISTS idx_orders_dropoff_address_id
    ON orders(dropoff_address_id);

CREATE INDEX IF NOT EXISTS idx_orders_open_last_status_at
    ON orders(last_status_at DESC)
    WHERE status IN ('created', 'assigned', 'in_transit');

CREATE INDEX IF NOT EXISTS idx_order_status_history_order_id_created_at
    ON order_status_history(order_id, created_at);

CREATE INDEX IF NOT EXISTS idx_order_status_history_status_created_at
    ON order_status_history(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_assignments_courier_assigned_at
    ON assignments(courier_id, assigned_at DESC);

CREATE INDEX IF NOT EXISTS idx_couriers_active
    ON couriers(is_active)
    WHERE is_active = TRUE;

DROP TRIGGER IF EXISTS trg_addresses_set_updated_at ON addresses;
CREATE TRIGGER trg_addresses_set_updated_at
BEFORE UPDATE ON addresses
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_orders_set_updated_at ON orders;
CREATE TRIGGER trg_orders_set_updated_at
BEFORE UPDATE ON orders
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_couriers_set_updated_at ON couriers;
CREATE TRIGGER trg_couriers_set_updated_at
BEFORE UPDATE ON couriers
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

COMMIT;
