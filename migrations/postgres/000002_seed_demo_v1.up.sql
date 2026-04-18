BEGIN;

INSERT INTO addresses (id, city, street, house, apartment, lat, lng)
VALUES
    ('a1111111-1111-1111-1111-111111111111', 'Saint Petersburg', 'Nevsky Prospekt', '10', NULL, 59.934300, 30.335100),
    ('a2222222-2222-2222-2222-222222222222', 'Saint Petersburg', 'Ligovsky Prospekt', '50', '12', 59.923800, 30.360900),
    ('a3333333-3333-3333-3333-333333333333', 'Saint Petersburg', 'Sadovaya Street', '44', NULL, 59.928500, 30.320800),
    ('a4444444-4444-4444-4444-444444444444', 'Saint Petersburg', 'Kamennoostrovsky Prospekt', '27', NULL, 59.966300, 30.311900),
    ('a5555555-5555-5555-5555-555555555555', 'Saint Petersburg', 'Liteyny Prospekt', '15', '34', 59.941700, 30.346800),
    ('a6666666-6666-6666-6666-666666666666', 'Saint Petersburg', 'Moskovsky Prospekt', '72', NULL, 59.890700, 30.316900)
ON CONFLICT (id) DO NOTHING;

INSERT INTO couriers (id, name, phone, vehicle_type, is_active)
VALUES
    ('c1111111-1111-1111-1111-111111111111', 'Ivan Petrov', '+79990000001', 'car', TRUE),
    ('c2222222-2222-2222-2222-222222222222', 'Anna Smirnova', '+79990000002', 'bike', TRUE),
    ('c3333333-3333-3333-3333-333333333333', 'Sergey Morozov', '+79990000003', 'van', TRUE),
    ('c4444444-4444-4444-4444-444444444444', 'Oleg Kuznetsov', '+79990000004', 'foot', FALSE)
ON CONFLICT (id) DO NOTHING;

INSERT INTO orders (
    id,
    customer_id,
    pickup_address_id,
    dropoff_address_id,
    weight_kg,
    distance_km,
    service_level,
    status,
    idempotency_key,
    last_status_at,
    created_at,
    updated_at
)
VALUES
    (
        'e1111111-1111-1111-1111-111111111111',
        'd1111111-1111-1111-1111-111111111111',
        'a1111111-1111-1111-1111-111111111111',
        'a2222222-2222-2222-2222-222222222222',
        2.50,
        6.80,
        'standard',
        'created',
        'demo-order-0001',
        NOW() - INTERVAL '20 minutes',
        NOW() - INTERVAL '20 minutes',
        NOW() - INTERVAL '20 minutes'
    ),
    (
        'e2222222-2222-2222-2222-222222222222',
        'd2222222-2222-2222-2222-222222222222',
        'a2222222-2222-2222-2222-222222222222',
        'a3333333-3333-3333-3333-333333333333',
        1.20,
        4.10,
        'express',
        'assigned',
        'demo-order-0002',
        NOW() - INTERVAL '2 hours',
        NOW() - INTERVAL '3 hours',
        NOW() - INTERVAL '2 hours'
    ),
    (
        'e3333333-3333-3333-3333-333333333333',
        'd3333333-3333-3333-3333-333333333333',
        'a3333333-3333-3333-3333-333333333333',
        'a4444444-4444-4444-4444-444444444444',
        7.00,
        12.30,
        'standard',
        'in_transit',
        'demo-order-0003',
        NOW() - INTERVAL '45 minutes',
        NOW() - INTERVAL '6 hours',
        NOW() - INTERVAL '45 minutes'
    ),
    (
        'e4444444-4444-4444-4444-444444444444',
        'd4444444-4444-4444-4444-444444444444',
        'a4444444-4444-4444-4444-444444444444',
        'a5555555-5555-5555-5555-555555555555',
        3.90,
        9.60,
        'express',
        'delivered',
        'demo-order-0004',
        NOW() - INTERVAL '18 hours',
        NOW() - INTERVAL '2 days',
        NOW() - INTERVAL '18 hours'
    ),
    (
        'e5555555-5555-5555-5555-555555555555',
        'd5555555-5555-5555-5555-555555555555',
        'a5555555-5555-5555-5555-555555555555',
        'a6666666-6666-6666-6666-666666666666',
        0.70,
        3.20,
        'standard',
        'cancelled',
        'demo-order-0005',
        NOW() - INTERVAL '1 day',
        NOW() - INTERVAL '1 day 3 hours',
        NOW() - INTERVAL '1 day'
    )
ON CONFLICT (id) DO NOTHING;

INSERT INTO assignments (id, order_id, courier_id, assigned_at, assigned_by, comment)
VALUES
    (
        'b1111111-1111-1111-1111-111111111111',
        'e2222222-2222-2222-2222-222222222222',
        'c2222222-2222-2222-2222-222222222222',
        NOW() - INTERVAL '2 hours',
        'dispatcher-demo',
        'Fast assignment for express order'
    ),
    (
        'b2222222-2222-2222-2222-222222222222',
        'e3333333-3333-3333-3333-333333333333',
        'c1111111-1111-1111-1111-111111111111',
        NOW() - INTERVAL '5 hours',
        'dispatcher-demo',
        'Courier picked large package'
    ),
    (
        'b3333333-3333-3333-3333-333333333333',
        'e4444444-4444-4444-4444-444444444444',
        'c3333333-3333-3333-3333-333333333333',
        NOW() - INTERVAL '1 day 18 hours',
        'dispatcher-demo',
        'Assigned to van courier'
    )
ON CONFLICT (order_id) DO NOTHING;

INSERT INTO order_status_history (
    id,
    order_id,
    status,
    source,
    comment,
    metadata,
    created_at
)
VALUES
    (
        'f0000001-1111-1111-1111-111111111111',
        'e1111111-1111-1111-1111-111111111111',
        'created',
        'system',
        'Order created from demo seed',
        '{"channel":"demo_seed"}'::jsonb,
        NOW() - INTERVAL '20 minutes'
    ),
    (
        'f0000002-2222-2222-2222-222222222222',
        'e2222222-2222-2222-2222-222222222222',
        'created',
        'system',
        'Order created from demo seed',
        '{"channel":"demo_seed"}'::jsonb,
        NOW() - INTERVAL '3 hours'
    ),
    (
        'f0000003-2222-2222-2222-222222222222',
        'e2222222-2222-2222-2222-222222222222',
        'assigned',
        'manager',
        'Assigned to courier Anna Smirnova',
        '{"courier_id":"c2222222-2222-2222-2222-222222222222"}'::jsonb,
        NOW() - INTERVAL '2 hours'
    ),
    (
        'f0000004-3333-3333-3333-333333333333',
        'e3333333-3333-3333-3333-333333333333',
        'created',
        'system',
        'Order created from demo seed',
        '{"channel":"demo_seed"}'::jsonb,
        NOW() - INTERVAL '6 hours'
    ),
    (
        'f0000005-3333-3333-3333-333333333333',
        'e3333333-3333-3333-3333-333333333333',
        'assigned',
        'manager',
        'Assigned to courier Ivan Petrov',
        '{"courier_id":"c1111111-1111-1111-1111-111111111111"}'::jsonb,
        NOW() - INTERVAL '5 hours'
    ),
    (
        'f0000006-3333-3333-3333-333333333333',
        'e3333333-3333-3333-3333-333333333333',
        'in_transit',
        'courier',
        'Courier is on route',
        '{"eta_minutes":35}'::jsonb,
        NOW() - INTERVAL '45 minutes'
    ),
    (
        'f0000007-4444-4444-4444-444444444444',
        'e4444444-4444-4444-4444-444444444444',
        'created',
        'system',
        'Order created from demo seed',
        '{"channel":"demo_seed"}'::jsonb,
        NOW() - INTERVAL '2 days'
    ),
    (
        'f0000008-4444-4444-4444-444444444444',
        'e4444444-4444-4444-4444-444444444444',
        'assigned',
        'manager',
        'Assigned to courier Sergey Morozov',
        '{"courier_id":"c3333333-3333-3333-3333-333333333333"}'::jsonb,
        NOW() - INTERVAL '1 day 18 hours'
    ),
    (
        'f0000009-4444-4444-4444-444444444444',
        'e4444444-4444-4444-4444-444444444444',
        'in_transit',
        'courier',
        'Parcel moved to destination district',
        '{"warehouse":"north_hub"}'::jsonb,
        NOW() - INTERVAL '1 day 6 hours'
    ),
    (
        'f0000010-4444-4444-4444-444444444444',
        'e4444444-4444-4444-4444-444444444444',
        'delivered',
        'courier',
        'Delivered to recipient',
        '{"recipient":"customer"}'::jsonb,
        NOW() - INTERVAL '18 hours'
    ),
    (
        'f0000011-5555-5555-5555-555555555555',
        'e5555555-5555-5555-5555-555555555555',
        'created',
        'system',
        'Order created from demo seed',
        '{"channel":"demo_seed"}'::jsonb,
        NOW() - INTERVAL '1 day 3 hours'
    ),
    (
        'f0000012-5555-5555-5555-555555555555',
        'e5555555-5555-5555-5555-555555555555',
        'cancelled',
        'manager',
        'Cancelled by customer request',
        '{"reason":"customer_changed_mind"}'::jsonb,
        NOW() - INTERVAL '1 day'
    )
ON CONFLICT (id) DO NOTHING;

COMMIT;
