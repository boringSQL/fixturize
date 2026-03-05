-- =============================================================================
-- Medium Seed Data (~130K rows)
-- Scaled from lighting/seed.sql for instant-start Docker demo
-- =============================================================================

\timing on

BEGIN;

-- =============================================================================
-- 1. Lookup arrays for realistic data
-- =============================================================================

CREATE TEMP TABLE _first_names (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _first_names (idx, name) VALUES
    (1,'James'),(2,'Mary'),(3,'Robert'),(4,'Patricia'),(5,'John'),
    (6,'Jennifer'),(7,'Michael'),(8,'Linda'),(9,'David'),(10,'Elizabeth'),
    (11,'William'),(12,'Barbara'),(13,'Richard'),(14,'Susan'),(15,'Joseph'),
    (16,'Jessica'),(17,'Thomas'),(18,'Sarah'),(19,'Christopher'),(20,'Karen'),
    (21,'Charles'),(22,'Lisa'),(23,'Daniel'),(24,'Nancy'),(25,'Matthew'),
    (26,'Betty'),(27,'Anthony'),(28,'Margaret'),(29,'Mark'),(30,'Sandra'),
    (31,'Steven'),(32,'Ashley'),(33,'Paul'),(34,'Dorothy'),(35,'Andrew'),
    (36,'Kimberly'),(37,'Joshua'),(38,'Emily'),(39,'Kenneth'),(40,'Donna');

CREATE TEMP TABLE _last_names (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _last_names (idx, name) VALUES
    (1,'Smith'),(2,'Johnson'),(3,'Williams'),(4,'Brown'),(5,'Jones'),
    (6,'Garcia'),(7,'Miller'),(8,'Davis'),(9,'Rodriguez'),(10,'Martinez'),
    (11,'Hernandez'),(12,'Lopez'),(13,'Gonzalez'),(14,'Wilson'),(15,'Anderson'),
    (16,'Thomas'),(17,'Taylor'),(18,'Moore'),(19,'Jackson'),(20,'Martin'),
    (21,'Lee'),(22,'Perez'),(23,'Thompson'),(24,'White'),(25,'Harris'),
    (26,'Sanchez'),(27,'Clark'),(28,'Ramirez'),(29,'Lewis'),(30,'Robinson'),
    (31,'Walker'),(32,'Young'),(33,'Allen'),(34,'King'),(35,'Wright'),
    (36,'Scott'),(37,'Torres'),(38,'Nguyen'),(39,'Hill'),(40,'Flores');

CREATE TEMP TABLE _streets (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _streets (idx, name) VALUES
    (1,'123 Main St'),(2,'456 Oak Ave'),(3,'789 Pine Rd'),(4,'321 Elm Blvd'),
    (5,'654 Maple Dr'),(6,'987 Cedar Ln'),(7,'147 Birch Way'),(8,'258 Walnut Ct'),
    (9,'369 Spruce Pl'),(10,'741 Willow Ter'),(11,'852 Ash St'),(12,'963 Cherry Ave'),
    (13,'111 Poplar Rd'),(14,'222 Hickory Blvd'),(15,'333 Sycamore Dr'),
    (16,'444 Magnolia Ln'),(17,'555 Cypress Way'),(18,'666 Juniper Ct'),
    (19,'777 Sequoia Pl'),(20,'888 Redwood Ter'),(21,'100 Broadway'),
    (22,'200 Park Ave'),(23,'300 Fifth Ave'),(24,'400 Market St'),
    (25,'500 State St'),(26,'600 Church Rd'),(27,'700 Washington Blvd'),
    (28,'800 Franklin Dr'),(29,'900 Lincoln Ln'),(30,'1000 Jefferson Way'),
    (31,'1100 Adams Ct'),(32,'1200 Madison Pl'),(33,'1300 Monroe Ter'),
    (34,'1400 Jackson Ave'),(35,'1500 Harrison Rd'),(36,'1600 Tyler Blvd'),
    (37,'1700 Polk Dr'),(38,'1800 Taylor Ln'),(39,'1900 Pierce Way'),
    (40,'2000 Grant St');

CREATE TEMP TABLE _cities (idx INT PRIMARY KEY, name TEXT NOT NULL, state TEXT NOT NULL, zip TEXT NOT NULL);
INSERT INTO _cities (idx, name, state, zip) VALUES
    (1,'New York','NY','10001'),(2,'Los Angeles','CA','90001'),(3,'Chicago','IL','60601'),
    (4,'Houston','TX','77001'),(5,'Phoenix','AZ','85001'),(6,'Philadelphia','PA','19101'),
    (7,'San Antonio','TX','78201'),(8,'San Diego','CA','92101'),(9,'Dallas','TX','75201'),
    (10,'San Jose','CA','95101'),(11,'Austin','TX','73301'),(12,'Jacksonville','FL','32099'),
    (13,'Fort Worth','TX','76101'),(14,'Columbus','OH','43085'),(15,'Charlotte','NC','28201'),
    (16,'Indianapolis','IN','46201'),(17,'San Francisco','CA','94101'),(18,'Seattle','WA','98101'),
    (19,'Denver','CO','80201'),(20,'Nashville','TN','37201'),(21,'Portland','OR','97201'),
    (22,'Las Vegas','NV','89101'),(23,'Memphis','TN','38101'),(24,'Louisville','KY','40201'),
    (25,'Baltimore','MD','21201'),(26,'Milwaukee','WI','53201'),(27,'Albuquerque','NM','87101'),
    (28,'Tucson','AZ','85701'),(29,'Fresno','CA','93701'),(30,'Sacramento','CA','95814'),
    (31,'Mesa','AZ','85201'),(32,'Kansas City','MO','64101'),(33,'Atlanta','GA','30301'),
    (34,'Omaha','NE','68101'),(35,'Colorado Springs','CO','80901'),(36,'Raleigh','NC','27601'),
    (37,'Virginia Beach','VA','23450'),(38,'Miami','FL','33101'),(39,'Minneapolis','MN','55401'),
    (40,'Tampa','FL','33601');

CREATE TEMP TABLE _carriers (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _carriers (idx, name) VALUES
    (1,'UPS'),(2,'FedEx'),(3,'USPS'),(4,'DHL'),(5,'Amazon Logistics');

CREATE TEMP TABLE _product_adjectives (idx INT PRIMARY KEY, word TEXT NOT NULL);
INSERT INTO _product_adjectives (idx, word) VALUES
    (1,'Premium'),(2,'Ultra'),(3,'Classic'),(4,'Pro'),(5,'Essential'),
    (6,'Deluxe'),(7,'Advanced'),(8,'Elite'),(9,'Smart'),(10,'Eco'),
    (11,'Turbo'),(12,'Flex'),(13,'Compact'),(14,'Mega'),(15,'Supreme'),
    (16,'Royal'),(17,'Prime'),(18,'Select'),(19,'Luxury'),(20,'Value');

CREATE TEMP TABLE _product_nouns (idx INT PRIMARY KEY, word TEXT NOT NULL);
INSERT INTO _product_nouns (idx, word) VALUES
    (1,'Widget'),(2,'Gadget'),(3,'Sensor'),(4,'Module'),(5,'Device'),
    (6,'Controller'),(7,'Adapter'),(8,'Connector'),(9,'Hub'),(10,'Station'),
    (11,'Monitor'),(12,'Speaker'),(13,'Camera'),(14,'Keyboard'),(15,'Mouse'),
    (16,'Charger'),(17,'Cable'),(18,'Stand'),(19,'Mount'),(20,'Light');

CREATE TEMP TABLE _colors (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _colors (idx, name) VALUES
    (1,'Black'),(2,'White'),(3,'Silver'),(4,'Blue'),(5,'Red'),
    (6,'Green'),(7,'Gold'),(8,'Rose Gold'),(9,'Space Gray'),(10,'Navy');

CREATE TEMP TABLE _sizes (idx INT PRIMARY KEY, name TEXT NOT NULL);
INSERT INTO _sizes (idx, name) VALUES
    (1,'XS'),(2,'S'),(3,'M'),(4,'L'),(5,'XL');

-- =============================================================================
-- 2. Seed organizations (10 rows)
-- =============================================================================
\echo Seeding organizations...

INSERT INTO organizations (id, name, slug, domain, metadata)
SELECT
    g,
    'Org ' || g,
    'org-' || g,
    'org' || g || '.example.com',
    jsonb_build_object(
        'plan', CASE WHEN g <= 2 THEN 'enterprise' WHEN g <= 5 THEN 'business' ELSE 'starter' END,
        'max_users', CASE WHEN g <= 2 THEN 100000 WHEN g <= 5 THEN 10000 ELSE 1000 END
    )
FROM generate_series(1, 10) g;
SELECT setval('organizations_id_seq', 10);

-- =============================================================================
-- 3. Seed org_settings (10 rows, 1:1)
-- =============================================================================
\echo Seeding org_settings...

INSERT INTO org_settings (organization_id, billing_email, billing_address, tax_id, payment_config, feature_flags)
SELECT
    o.id,
    'billing@org' || o.id || '.example.com',
    jsonb_build_object(
        'street', s.name,
        'city', c.name,
        'state', c.state,
        'zip', c.zip,
        'country', 'US'
    ),
    'US-TAX-' || lpad(o.id::text, 6, '0'),
    '{"gateway": "stripe", "auto_capture": true}'::jsonb,
    '{"dark_mode": true, "beta_features": false}'::jsonb
FROM organizations o
JOIN _streets s ON s.idx = ((o.id - 1) % 40) + 1
JOIN _cities c ON c.idx = ((o.id - 1) % 40) + 1;

-- =============================================================================
-- 4. Seed categories (50 rows, self-referencing, 3 levels)
-- =============================================================================
\echo Seeding categories...

-- Level 0: 10 root categories (1 per org)
INSERT INTO categories (organization_id, parent_id, name, slug, depth, path)
SELECT o.id, NULL, 'Root Category ' || o.id, 'root-' || o.id, 0, 'root-' || o.id
FROM organizations o;

-- Level 1: 2 per root = 20
INSERT INTO categories (organization_id, parent_id, name, slug, depth, path)
SELECT
    c.organization_id,
    c.id,
    c.name || ' > Sub ' || g,
    c.slug || '-sub-' || g,
    1,
    c.path || '/sub-' || g
FROM categories c, generate_series(1, 2) g
WHERE c.depth = 0;

-- Level 2: 1 per level-1 = 20 (total 50)
INSERT INTO categories (organization_id, parent_id, name, slug, depth, path)
SELECT
    c.organization_id,
    c.id,
    c.name || ' > Leaf ' || 1,
    c.slug || '-leaf-1',
    2,
    c.path || '/leaf-1'
FROM categories c
WHERE c.depth = 1;

-- =============================================================================
-- 5. Seed users (5,000 rows, UUID PK)
-- =============================================================================
\echo Seeding users (5K rows)...

INSERT INTO users (id, organization_id, role, email, password_hash, first_name, last_name, phone, date_of_birth, avatar_url, preferences, last_login_ip)
SELECT
    gen_random_uuid(),
    ((g - 1) % 10) + 1,
    (ARRAY['buyer','buyer','buyer','seller','admin','support']::user_role[])[(g % 6) + 1],
    'user' || g || '@example.com',
    '$2b$12$' || md5(g::text || 'salt'),
    fn.name,
    ln.name,
    '+1-' || lpad(((g * 7 + 100) % 900 + 100)::text, 3, '0') || '-' ||
        lpad(((g * 13 + 200) % 900 + 100)::text, 3, '0') || '-' ||
        lpad(((g * 17 + 300) % 9000 + 1000)::text, 4, '0'),
    '1970-01-01'::date + (g % 15000),
    'https://cdn.example.com/avatars/' || (g % 500) || '.jpg',
    jsonb_build_object(
        'theme', CASE WHEN g % 3 = 0 THEN 'dark' ELSE 'light' END,
        'locale', CASE WHEN g % 5 = 0 THEN 'es' WHEN g % 7 = 0 THEN 'fr' ELSE 'en' END,
        'notifications', g % 2 = 0
    ),
    ('10.' || (g % 256) || '.' || ((g / 256) % 256) || '.' || ((g / 65536) % 256))::inet
FROM generate_series(1, 5000) g
JOIN _first_names fn ON fn.idx = (g % 40) + 1
JOIN _last_names ln ON ln.idx = ((g / 40) % 40) + 1;

-- =============================================================================
-- 6. Build _user_lookup temp table
-- =============================================================================
\echo Building user lookup table...

CREATE TEMP TABLE _user_lookup AS
SELECT id, organization_id, row_number() OVER (ORDER BY created_at, id) AS idx
FROM users;

CREATE INDEX idx_user_lookup_idx ON _user_lookup(idx);
CREATE INDEX idx_user_lookup_org ON _user_lookup(organization_id);

CREATE TEMP TABLE _seller_users AS
SELECT id AS user_id, organization_id, row_number() OVER () AS idx
FROM _user_lookup
WHERE idx % 20 = 1;

CREATE INDEX idx_seller_users_idx ON _seller_users(idx);

-- =============================================================================
-- 7. Seed user_addresses (10,000 rows)
-- =============================================================================
\echo Seeding user_addresses (10K rows)...

INSERT INTO user_addresses (user_id, label, first_name, last_name, street, city, state, zip, country, phone, is_default)
SELECT
    ul.id,
    CASE WHEN g = 1 THEN 'home' ELSE 'work' END,
    fn.name,
    ln.name,
    s.name,
    c.name,
    c.state,
    c.zip,
    'US',
    '+1-555-' || lpad(((ul.idx * 3 + g * 7) % 9000 + 1000)::text, 4, '0'),
    g = 1
FROM _user_lookup ul
CROSS JOIN generate_series(1, 2) g
JOIN _first_names fn ON fn.idx = ((ul.idx + g) % 40) + 1
JOIN _last_names ln ON ln.idx = ((ul.idx * 3 + g) % 40) + 1
JOIN _streets s ON s.idx = ((ul.idx + g * 7) % 40) + 1
JOIN _cities c ON c.idx = ((ul.idx + g * 13) % 40) + 1;

-- =============================================================================
-- 8. Seed seller_profiles (250 rows)
-- =============================================================================
\echo Seeding seller_profiles (250 rows)...

INSERT INTO seller_profiles (user_id, store_name, store_description, bank_account_number, bank_routing_number, tax_id, commission_rate, payout_config, verified_at)
SELECT
    su.user_id,
    pa.word || ' ' || pn.word || ' Store',
    'Quality products from ' || pa.word || ' ' || pn.word,
    lpad((su.idx * 31337 % 999999999)::text, 12, '0'),
    lpad((su.idx * 7919 % 999999)::text, 9, '0'),
    'SELLER-TAX-' || lpad(su.idx::text, 6, '0'),
    0.05 + (su.idx % 10) * 0.01,
    jsonb_build_object('payout_day', (su.idx % 28) + 1, 'min_payout', 50),
    CASE WHEN su.idx % 5 != 0 THEN now() - (su.idx % 365 || ' days')::interval ELSE NULL END
FROM _seller_users su
JOIN _product_adjectives pa ON pa.idx = (su.idx % 20) + 1
JOIN _product_nouns pn ON pn.idx = ((su.idx / 20) % 20) + 1;

-- =============================================================================
-- 9. Seed departments (100 rows)
-- =============================================================================
\echo Seeding departments...

INSERT INTO departments (id, organization_id, name, code, budget, metadata)
SELECT
    g,
    ((g - 1) % 10) + 1,
    (ARRAY['Engineering','Sales','Marketing','Support','Operations','Finance','HR','Legal','Product','Design'])[((g - 1) % 10) + 1] || ' ' || ((g - 1) / 10 + 1),
    'DEPT-' || lpad(g::text, 4, '0'),
    50000 + (g * 1337 % 450000),
    jsonb_build_object('floor', (g % 10) + 1, 'building', 'HQ-' || ((g % 3) + 1))
FROM generate_series(1, 100) g;
SELECT setval('departments_id_seq', 100);

-- =============================================================================
-- 10. Seed employees (500 rows)
-- =============================================================================
\echo Seeding employees (500 rows)...

INSERT INTO employees (user_id, department_id, employee_number, title, salary, hire_date, metadata)
SELECT
    ul.id,
    ((ul.idx - 1) % 100) + 1,
    'EMP-' || lpad(ul.idx::text, 6, '0'),
    (ARRAY['Engineer','Manager','Analyst','Specialist','Coordinator','Director','Lead','Associate','Consultant','Architect'])[(ul.idx % 10) + 1],
    40000 + (ul.idx * 97 % 160000),
    '2020-01-01'::date + (ul.idx % 1500)::int,
    jsonb_build_object('badge_id', 'B' || lpad(ul.idx::text, 5, '0'))
FROM _user_lookup ul
WHERE ul.idx <= 500;

-- Set managers
WITH numbered AS (
    SELECT id, row_number() OVER (ORDER BY id) AS rn FROM employees
),
managers AS (
    SELECT id, rn FROM numbered WHERE rn <= 50
)
UPDATE employees e
SET manager_id = m.id
FROM numbered n
JOIN managers m ON m.rn = ((n.rn - 1) % 50) + 1
WHERE e.id = n.id AND n.rn > 50;

-- Circular FK: set lead_employee_id on departments
UPDATE departments d
SET lead_employee_id = e.id
FROM employees e
WHERE e.department_id = d.id
AND e.id <= 100;

-- =============================================================================
-- 11. Seed warehouses (50 rows)
-- =============================================================================
\echo Seeding warehouses...

INSERT INTO warehouses (id, organization_id, name, code, street, city, state, zip, country, capacity, metadata)
SELECT
    g,
    ((g - 1) % 10) + 1,
    'Warehouse ' || c.name || '-' || g,
    'WH-' || lpad(g::text, 4, '0'),
    s.name,
    c.name,
    c.state,
    c.zip,
    'US',
    10000 + (g * 137 % 90000),
    jsonb_build_object('type', CASE WHEN g % 3 = 0 THEN 'cold_storage' WHEN g % 3 = 1 THEN 'standard' ELSE 'fulfillment' END)
FROM generate_series(1, 50) g
JOIN _streets s ON s.idx = (g % 40) + 1
JOIN _cities c ON c.idx = ((g / 3) % 40) + 1;
SELECT setval('warehouses_id_seq', 50);

-- =============================================================================
-- 12. Seed products (2,000 rows)
-- =============================================================================
\echo Seeding products (2K rows)...

CREATE TEMP TABLE _cat_lookup AS
SELECT id, organization_id, row_number() OVER (ORDER BY id) AS idx
FROM categories;
CREATE INDEX idx_cat_lookup_org ON _cat_lookup(organization_id);
CREATE INDEX idx_cat_lookup_idx ON _cat_lookup(idx);

CREATE TEMP TABLE _seller_lookup AS
SELECT sp.id AS seller_id, su.organization_id, row_number() OVER (ORDER BY sp.id) AS idx
FROM seller_profiles sp
JOIN _seller_users su ON su.user_id = sp.user_id;
CREATE INDEX idx_seller_lookup_idx ON _seller_lookup(idx);

INSERT INTO products (organization_id, category_id, seller_id, sku, name, description, base_price, tags, is_active)
SELECT
    sl.organization_id,
    cl.id,
    sl.seller_id,
    'SKU-' || lpad(g::text, 8, '0'),
    pa.word || ' ' || pn.word || ' ' || g,
    'High quality ' || lower(pa.word) || ' ' || lower(pn.word) || ' for everyday use.',
    5.99 + (g * 17 % 49401)::numeric / 100,
    jsonb_build_array(lower(pa.word), lower(pn.word), CASE WHEN g % 2 = 0 THEN 'bestseller' ELSE 'new' END),
    g % 20 != 0
FROM generate_series(1, 2000) g
CROSS JOIN (SELECT count(*)::int AS cnt FROM _cat_lookup) cat_cnt
JOIN _seller_lookup sl ON sl.idx = ((g - 1) % 250) + 1
JOIN _cat_lookup cl ON cl.idx = ((g - 1) % cat_cnt.cnt) + 1
JOIN _product_adjectives pa ON pa.idx = (g % 20) + 1
JOIN _product_nouns pn ON pn.idx = ((g / 20) % 20) + 1;

-- =============================================================================
-- 13. Seed product_variants (5,000 rows, ~2.5 per product)
-- =============================================================================
\echo Seeding product_variants (5K rows)...

CREATE TEMP TABLE _prod_lookup AS
SELECT id, base_price, row_number() OVER (ORDER BY id) AS idx
FROM products;
CREATE INDEX idx_prod_lookup_idx ON _prod_lookup(idx);

INSERT INTO product_variants (product_id, sku, name, price, weight_kg, attributes, stock_count, is_active)
SELECT
    pl.id,
    'VAR-' || lpad(g::text, 8, '0'),
    co.name || ' / ' || sz.name,
    pl.base_price + (g % 20) - 10,
    0.1 + (g % 500)::numeric / 100,
    jsonb_build_object('color', co.name, 'size', sz.name, 'material', CASE WHEN g % 3 = 0 THEN 'plastic' WHEN g % 3 = 1 THEN 'metal' ELSE 'fabric' END),
    (g * 37 % 500),
    g % 30 != 0
FROM generate_series(1, 5000) g
JOIN _prod_lookup pl ON pl.idx = ((g - 1) * 2 / 5) + 1
JOIN _colors co ON co.idx = (g % 10) + 1
JOIN _sizes sz ON sz.idx = (g % 5) + 1;

-- =============================================================================
-- 14. Seed warehouse_inventory (5,000 rows)
-- =============================================================================
\echo Seeding warehouse_inventory (5K rows)...

CREATE TEMP TABLE _variant_lookup AS
SELECT id, row_number() OVER (ORDER BY id) AS idx
FROM product_variants;
CREATE INDEX idx_variant_lookup_idx ON _variant_lookup(idx);

INSERT INTO warehouse_inventory (warehouse_id, variant_id, quantity, reserved, reorder_point, metadata)
SELECT
    ((g - 1) % 50) + 1,
    vl.id,
    (g * 31 % 1000),
    (g * 7 % 100),
    (g * 3 % 50) + 5,
    jsonb_build_object('bin', 'A' || ((g % 26) + 1) || '-' || ((g / 26) % 100))
FROM generate_series(1, 5000) g
JOIN _variant_lookup vl ON vl.idx = ((g - 1) % 5000) + 1
ON CONFLICT (warehouse_id, variant_id) DO NOTHING;

CREATE TEMP TABLE _wi_lookup AS
SELECT warehouse_id, variant_id, row_number() OVER (ORDER BY warehouse_id, variant_id) AS idx
FROM warehouse_inventory;
CREATE INDEX idx_wi_lookup_idx ON _wi_lookup(idx);

-- =============================================================================
-- 15. Seed inventory_movements (10,000 rows, composite FK)
-- =============================================================================
\echo Seeding inventory_movements (10K rows)...

INSERT INTO inventory_movements (warehouse_id, variant_id, quantity_delta, reason, reference_id, metadata, created_at)
SELECT
    wl.warehouse_id,
    wl.variant_id,
    CASE WHEN g % 3 = 0 THEN -(g % 10 + 1) ELSE (g % 50 + 1) END,
    (ARRAY['receipt','sale','adjustment','return','transfer'])[(g % 5) + 1],
    'REF-' || lpad(g::text, 10, '0'),
    jsonb_build_object('operator', 'system', 'batch', g / 1000),
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval + (g % 86400 || ' seconds')::interval
FROM generate_series(1, 10000) g
CROSS JOIN (SELECT count(*)::int AS cnt FROM _wi_lookup) wi_cnt
JOIN _wi_lookup wl ON wl.idx = ((g - 1) % wi_cnt.cnt) + 1;

-- =============================================================================
-- 16. Seed orders (15,000 rows)
-- =============================================================================
\echo Seeding orders (15K rows)...

INSERT INTO orders (organization_id, user_id, status, shipping_first_name, shipping_last_name,
    shipping_street, shipping_city, shipping_state, shipping_zip, shipping_country, shipping_phone,
    notes, total_amount, created_at, updated_at)
SELECT
    ul.organization_id,
    ul.id,
    (ARRAY['pending','confirmed','processing','shipped','delivered','delivered','delivered']::order_status[])[(g % 7) + 1],
    fn.name,
    ln.name,
    s.name,
    c.name,
    c.state,
    c.zip,
    'US',
    '+1-555-' || lpad((g % 9000 + 1000)::text, 4, '0'),
    CASE WHEN g % 10 = 0 THEN jsonb_build_object('gift', true, 'message', 'Happy birthday!') ELSE '{}'::jsonb END,
    19.99 + (g * 41 % 48001)::numeric / 100,
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval + (g % 86400 || ' seconds')::interval,
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval + ((g % 86400) + 3600 || ' seconds')::interval
FROM generate_series(1, 15000) g
JOIN _user_lookup ul ON ul.idx = ((g - 1) % 5000) + 1
JOIN _first_names fn ON fn.idx = (g % 40) + 1
JOIN _last_names ln ON ln.idx = ((g / 40) % 40) + 1
JOIN _streets s ON s.idx = ((g * 3) % 40) + 1
JOIN _cities c ON c.idx = ((g * 7) % 40) + 1;

CREATE TEMP TABLE _order_lookup AS
SELECT id, user_id, organization_id, created_at, row_number() OVER (ORDER BY id) AS idx
FROM orders;
CREATE INDEX idx_order_lookup_idx ON _order_lookup(idx);

-- =============================================================================
-- 17. Seed order_items (30,000 rows, ~2 per order)
-- =============================================================================
\echo Seeding order_items (30K rows)...

INSERT INTO order_items (order_id, variant_id, quantity, unit_price, metadata, created_at)
SELECT
    ol.id,
    vl.id,
    (g % 5) + 1,
    5.99 + ((g * 23) % 49401)::numeric / 100,
    CASE WHEN g % 20 = 0 THEN jsonb_build_object('gift_wrap', true) ELSE '{}'::jsonb END,
    ol.created_at
FROM generate_series(1, 30000) g
JOIN _order_lookup ol ON ol.idx = ((g - 1) / 2 + 1)::int
JOIN _variant_lookup vl ON vl.idx = ((g * 7) % 5000) + 1;

-- =============================================================================
-- 18. Seed payments (20,000 rows, partitioned, identity col)
-- =============================================================================
\echo Seeding payments (20K rows)...

INSERT INTO payments (order_id, method, status, amount, currency, credit_card_last4, credit_card_brand, gateway_ref, metadata, created_at)
SELECT
    ol.id,
    (ARRAY['credit_card','debit_card','bank_transfer','paypal','crypto']::payment_method[])[(g % 5) + 1],
    (ARRAY['captured','captured','captured','captured','pending','authorized','failed','refunded']::payment_status[])[(g % 8) + 1],
    19.99 + ((g * 37) % 48001)::numeric / 100,
    'USD',
    lpad((g % 10000)::text, 4, '0'),
    (ARRAY['Visa','Mastercard','Amex','Discover'])[((g / 10000) % 4) + 1],
    'gw_' || md5(g::text),
    jsonb_build_object('attempt', (g % 3) + 1, 'risk_score', (g % 100)::numeric / 100),
    '2024-01-01'::timestamptz + (g % 900 || ' days')::interval + (g % 86400 || ' seconds')::interval
FROM generate_series(1, 20000) g
JOIN _order_lookup ol ON ol.idx = ((g - 1) % 15000) + 1;

-- =============================================================================
-- 19. Seed shipments (5,000 rows)
-- =============================================================================
\echo Seeding shipments (5K rows)...

INSERT INTO shipments (order_id, carrier, tracking_number, recipient_name, recipient_phone,
    recipient_street, recipient_city, recipient_state, recipient_zip, recipient_country,
    weight_kg, metadata, shipped_at, delivered_at, created_at)
SELECT
    ol.id,
    ca.name,
    upper(md5(g::text || 'track')),
    fn.name || ' ' || ln.name,
    '+1-555-' || lpad((g % 9000 + 1000)::text, 4, '0'),
    s.name,
    c.name,
    c.state,
    c.zip,
    'US',
    0.5 + (g % 300)::numeric / 10,
    jsonb_build_object('insurance', g % 5 = 0, 'signature_required', g % 3 = 0),
    ol.created_at + '1 day'::interval,
    CASE WHEN g % 4 != 0 THEN ol.created_at + '5 days'::interval ELSE NULL END,
    ol.created_at
FROM generate_series(1, 5000) g
JOIN _order_lookup ol ON ol.idx = ((g - 1) % 15000) + 1
JOIN _carriers ca ON ca.idx = (g % 5) + 1
JOIN _first_names fn ON fn.idx = ((g * 3) % 40) + 1
JOIN _last_names ln ON ln.idx = ((g * 7) % 40) + 1
JOIN _streets s ON s.idx = ((g * 11) % 40) + 1
JOIN _cities c ON c.idx = ((g * 13) % 40) + 1;

-- =============================================================================
-- 20. Seed reviews (10,000 rows, multi-FK)
-- =============================================================================
\echo Seeding reviews (10K rows)...

INSERT INTO reviews (user_id, product_id, order_id, rating, title, body, media, is_verified, created_at, updated_at)
SELECT
    ul.id,
    pl.id,
    ol.id,
    (g % 5) + 1,
    CASE (g % 5) + 1
        WHEN 5 THEN 'Absolutely love it!'
        WHEN 4 THEN 'Really great product'
        WHEN 3 THEN 'Decent, meets expectations'
        WHEN 2 THEN 'Could be better'
        WHEN 1 THEN 'Very disappointed'
    END,
    'Review body for product. ' || CASE
        WHEN g % 3 = 0 THEN 'Fast shipping, well packaged.'
        WHEN g % 3 = 1 THEN 'Good value for the price.'
        ELSE 'Would recommend to friends.'
    END,
    CASE WHEN g % 10 = 0 THEN jsonb_build_array(jsonb_build_object('url', 'https://cdn.example.com/reviews/' || g || '.jpg', 'type', 'image')) ELSE '[]'::jsonb END,
    g % 3 != 0,
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval,
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval
FROM generate_series(1, 10000) g
JOIN _user_lookup ul ON ul.idx = ((g - 1) % 5000) + 1
JOIN _prod_lookup pl ON pl.idx = ((g * 3) % 2000) + 1
JOIN _order_lookup ol ON ol.idx = ((g - 1) % 15000) + 1;

-- =============================================================================
-- 21. Seed support_tickets (2,000 rows, 2 FKs to users)
-- =============================================================================
\echo Seeding support_tickets (2K rows)...

CREATE TEMP TABLE _support_staff AS
SELECT ul.id AS user_id, row_number() OVER () AS idx
FROM _user_lookup ul
JOIN users u ON u.id = ul.id
WHERE u.role IN ('support', 'admin')
LIMIT 500;
CREATE INDEX idx_support_staff_idx ON _support_staff(idx);

INSERT INTO support_tickets (organization_id, requester_id, assignee_id, order_id, status, priority, subject, metadata, created_at, updated_at)
SELECT
    ul.organization_id,
    ul.id,
    ss.user_id,
    ol.id,
    (ARRAY['open','in_progress','waiting_customer','resolved','closed']::ticket_status[])[(g % 5) + 1],
    (g % 4) + 1,
    (ARRAY['Order not received','Wrong item shipped','Refund request','Account issue','Payment failed',
           'Product defective','Shipping delay','Billing question','Return request','Cancel order'])[((g - 1) % 10) + 1],
    jsonb_build_object('channel', CASE WHEN g % 3 = 0 THEN 'email' WHEN g % 3 = 1 THEN 'chat' ELSE 'phone' END),
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval,
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval + '2 hours'::interval
FROM generate_series(1, 2000) g
JOIN _user_lookup ul ON ul.idx = ((g * 7) % 5000) + 1
CROSS JOIN (SELECT count(*)::int AS cnt FROM _support_staff) ss_cnt
JOIN _support_staff ss ON ss.idx = ((g - 1) % ss_cnt.cnt) + 1
JOIN _order_lookup ol ON ol.idx = ((g * 3) % 15000) + 1;

CREATE TEMP TABLE _ticket_lookup AS
SELECT id, requester_id, row_number() OVER (ORDER BY id) AS idx
FROM support_tickets;
CREATE INDEX idx_ticket_lookup_idx ON _ticket_lookup(idx);

-- =============================================================================
-- 22. Seed ticket_messages (5,000 rows)
-- =============================================================================
\echo Seeding ticket_messages (5K rows)...

INSERT INTO ticket_messages (ticket_id, sender_id, body, attachments, is_internal, metadata, created_at)
SELECT
    tl.id,
    CASE WHEN g % 2 = 0 THEN tl.requester_id ELSE ss.user_id END,
    CASE (g % 4)
        WHEN 0 THEN 'Thank you for contacting us. We are looking into your issue.'
        WHEN 1 THEN 'Could you please provide more details about the problem?'
        WHEN 2 THEN 'I have attached a screenshot showing the error.'
        ELSE 'The issue has been escalated to our team. We will update you shortly.'
    END,
    CASE WHEN g % 5 = 0 THEN
        jsonb_build_array(jsonb_build_object(
            'url', 'https://cdn.example.com/attachments/' || g || '.png',
            'filename', 'screenshot_' || g || '.png',
            'size', (g % 5000) + 1000
        ))
    ELSE '[]'::jsonb END,
    g % 7 = 0,
    jsonb_build_object('read', g % 2 = 0),
    '2024-01-01'::timestamptz + (g % 730 || ' days')::interval + ((g % 4) || ' hours')::interval
FROM generate_series(1, 5000) g
JOIN _ticket_lookup tl ON tl.idx = ((g - 1) / 2 + 1)::int
CROSS JOIN (SELECT count(*)::int AS cnt FROM _support_staff) ss_cnt2
JOIN _support_staff ss ON ss.idx = ((g * 3) % ss_cnt2.cnt) + 1;

-- =============================================================================
-- 23. Seed audit_logs (10,000 rows, partitioned)
-- =============================================================================
\echo Seeding audit_logs (10K rows)...

INSERT INTO audit_logs (organization_id, user_id, action, table_name, record_id, old_values, new_values, ip_address, user_agent, metadata, created_at)
SELECT
    ul.organization_id,
    ul.id,
    (ARRAY['INSERT','UPDATE','DELETE','SELECT','LOGIN','LOGOUT'])[(g % 6) + 1],
    (ARRAY['users','orders','products','payments','reviews','shipments','tickets','addresses','inventory','settings'])[((g / 6) % 10) + 1],
    (g % 10000000)::text,
    CASE WHEN g % 6 IN (1, 2) THEN jsonb_build_object('status', 'old_value') ELSE NULL END,
    CASE WHEN g % 6 IN (0, 1) THEN jsonb_build_object('status', 'new_value') ELSE NULL END,
    ('10.' || (g % 256) || '.' || ((g / 256) % 256) || '.' || ((g / 65536) % 256))::inet,
    'Mozilla/5.0 (compatible; bot/' || (g % 100) || ')',
    jsonb_build_object('request_id', md5(g::text), 'duration_ms', g % 500),
    '2024-01-01'::timestamptz + (g % 900 || ' days')::interval + (g % 86400 || ' seconds')::interval
FROM generate_series(1, 10000) g
JOIN _user_lookup ul ON ul.idx = ((g - 1) % 5000) + 1;

COMMIT;

-- =============================================================================
-- 24. ANALYZE all tables
-- =============================================================================
\echo Running ANALYZE...

ANALYZE organizations;
ANALYZE org_settings;
ANALYZE categories;
ANALYZE users;
ANALYZE user_addresses;
ANALYZE seller_profiles;
ANALYZE departments;
ANALYZE employees;
ANALYZE warehouses;
ANALYZE products;
ANALYZE product_variants;
ANALYZE warehouse_inventory;
ANALYZE inventory_movements;
ANALYZE orders;
ANALYZE order_items;
ANALYZE payments;
ANALYZE shipments;
ANALYZE reviews;
ANALYZE support_tickets;
ANALYZE ticket_messages;
ANALYZE audit_logs;

\echo Seeding complete!
