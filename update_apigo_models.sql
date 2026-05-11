-- 更新 apigo 渠道模型列表为 apigo 官方模型名
UPDATE channels SET models = 'gemini-3.1-flash-image-preview,gemini-3-pro-image-preview,gpt-image-2' WHERE id = 4;

-- 更新 abilities 映射
INSERT INTO abilities ("group", model, channel_id, enabled, priority, weight)
VALUES 
    ('default', 'gemini-3.1-flash-image-preview', 4, true, 0, 0),
    ('default', 'gemini-3-pro-image-preview', 4, true, 0, 0)
ON CONFLICT ("group", model, channel_id) DO UPDATE SET enabled = true;
