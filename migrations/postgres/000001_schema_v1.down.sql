BEGIN;

DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS couriers;
DROP TABLE IF EXISTS addresses;

DROP FUNCTION IF EXISTS set_updated_at();

DROP TYPE IF EXISTS status_source_t;
DROP TYPE IF EXISTS order_status_t;
DROP TYPE IF EXISTS service_level_t;
DROP TYPE IF EXISTS vehicle_type_t;

COMMIT;
