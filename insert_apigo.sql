INSERT INTO channels (type, key, base_url, models, name, status, "group", test_model, auto_ban, created_time) 
VALUES (
    1, 
    '8e2177cb3d564f1ea08e909fb5f4a2a6', 
    'https://api-vip.apigo.ai', 
    'nano2,nano pro,gpt-image-2', 
    'apigo', 
    1, 
    'default', 
    'nano2', 
    1, 
    EXTRACT(EPOCH FROM NOW())::bigint
) 
RETURNING id;
