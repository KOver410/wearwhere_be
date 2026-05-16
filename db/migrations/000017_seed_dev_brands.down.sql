DELETE FROM brand_addresses
 WHERE brand_id IN ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
                    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb');
DELETE FROM brands
 WHERE id IN ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
              'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb');
DELETE FROM users
 WHERE id IN ('11111111-1111-1111-1111-111111111111',
              '22222222-2222-2222-2222-222222222222');
