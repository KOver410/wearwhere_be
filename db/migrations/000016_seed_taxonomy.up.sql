INSERT INTO categories (slug, name, display_order) VALUES
    ('t-shirt',     'T-Shirt',     10),
    ('shirt',       'Shirt',       20),
    ('dress',       'Dress',       30),
    ('pants',       'Pants',       40),
    ('shorts',      'Shorts',      50),
    ('jacket',      'Jacket',      60),
    ('skirt',       'Skirt',       70),
    ('hoodie',      'Hoodie',      80),
    ('shoes',       'Shoes',       90),
    ('accessory',   'Accessory',  100)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO style_tags (slug, name) VALUES
    ('streetwear', 'Streetwear'),
    ('minimalist', 'Minimalist'),
    ('y2k',        'Y2K'),
    ('vintage',    'Vintage'),
    ('casual',     'Casual'),
    ('formal',     'Formal'),
    ('sporty',     'Sporty'),
    ('vietnamese', 'Vietnamese Heritage'),
    ('preppy',     'Preppy'),
    ('grunge',     'Grunge')
ON CONFLICT (slug) DO NOTHING;
