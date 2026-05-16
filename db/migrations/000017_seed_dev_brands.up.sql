-- Idempotent: insert demo brand-owner users if missing.
-- Password hash below corresponds to "DevBrand@1234" using bcrypt cost=12.
-- Regenerate via the auth module's hash package if you want different creds.
INSERT INTO users (id, email, password_hash, role, status, name, email_verified_at)
VALUES
    ('11111111-1111-1111-1111-111111111111',
     'owner1@local.test',
     '$2a$12$KIXxPfnK7UvK7vTFvO5/lOQqB.6t9aS8L0iHnxOEKi4n3a6P3Hk9q',
     'brand', 'active', 'Local-X Owner', NOW()),
    ('22222222-2222-2222-2222-222222222222',
     'owner2@local.test',
     '$2a$12$KIXxPfnK7UvK7vTFvO5/lOQqB.6t9aS8L0iHnxOEKi4n3a6P3Hk9q',
     'brand', 'active', 'BadVibes Owner', NOW())
ON CONFLICT (id) DO NOTHING;

INSERT INTO brands (id, slug, name, owner_user_id, story, status)
VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'local-x', 'Local-X',
     '11111111-1111-1111-1111-111111111111',
     'Local-X kể câu chuyện streetwear Việt Nam đương đại.',
     'active'),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     'badvibes', 'BadVibes',
     '22222222-2222-2222-2222-222222222222',
     'Phong cách Y2K hoài niệm cho Gen Z Việt.',
     'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO brand_addresses (brand_id, label, address_line, ward, district, city, is_primary, is_public)
VALUES
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'Flagship Hà Nội', '12 Phố Huế', 'Ngô Thì Nhậm', 'Hai Bà Trưng', 'Hà Nội', TRUE, TRUE),
    ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
     'Showroom HCM', '45 Lý Tự Trọng', 'Bến Nghé', 'Quận 1', 'Hồ Chí Minh', FALSE, TRUE),
    ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
     'BadVibes HQ', '8 Nguyễn Huệ', 'Bến Nghé', 'Quận 1', 'Hồ Chí Minh', TRUE, TRUE)
ON CONFLICT DO NOTHING;
