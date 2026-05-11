INSERT INTO abilities ("group", model, channel_id, enabled, priority, weight)
VALUES 
    ('default', 'nano2', 4, true, 0, 0),
    ('default', 'nano pro', 4, true, 0, 0),
    ('default', 'gpt-image-2', 4, true, 0, 0)
ON CONFLICT ("group", model, channel_id) DO UPDATE SET enabled = true;
