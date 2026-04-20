# Seedance 视频生成 API 文档

## 概述

Seedance 是由火山引擎提供的高质量 AI 视频生成模型，支持多种输入方式：
- **文本生成视频** (Text-to-Video)
- **图像生成视频** (Image-to-Video)
- **视频生成视频** (Video-to-Video)
- **图像+视频混合生成** (Mixed Input) ✨

## 基础信息

- **API 端点**: `https://ark.cn-beijing.volces.com`
- **认证方式**: Bearer Token
- **请求格式**: JSON
- **文档参考**: 
  - [Seedance 2.0 API 参考](https://www.volcengine.com/docs/82379/1520757?lang=zh)
  - [查询视频生成任务](https://www.volcengine.com/docs/82379/1521309?lang=zh)

---

## API 接口

### 1. 统一视频生成 API (推荐)

#### 1.1 原生格式

**端点**: `POST /v1/video/generations`

**请求示例 - 图像+视频混合输入**:

```bash
curl -X POST "https://your-api.com/v1/video/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "将静态场景与动态效果融合，创造流畅的视频过渡",
    "images": [
      "https://example.com/reference-image.jpg",
      "https://example.com/style-reference.jpg"
    ],
    "videos": [
      "https://example.com/motion-reference.mp4"
    ],
    "seconds": 5,
    "metadata": {
      "resolution": "720p",
      "ratio": "16:9",
      "seed": 42,
      "watermark": false
    }
  }'
```

**请求示例 - 仅图像输入**:

```bash
curl -X POST "https://your-api.com/v1/video/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "让这张图片动起来，展示海浪拍打岸边的场景",
    "images": [
      "https://example.com/ocean-scene.jpg"
    ],
    "seconds": 5,
    "resolution": "1080p"
  }'
```

**请求示例 - 仅视频输入**:

```bash
curl -X POST "https://your-api.com/v1/video/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "将视频转换为动漫风格",
    "videos": [
      "https://example.com/input-video.mp4"
    ],
    "seconds": 5
  }'
```

**请求示例 - 纯文本生成**:

```bash
curl -X POST "https://your-api.com/v1/video/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "一只金色的狗在海滩上奔跑，夕阳西下，海浪轻拍岸边",
    "seconds": 5,
    "resolution": "720p",
    "ratio": "16:9"
  }'
```

#### 1.2 OpenAI 兼容格式

**端点**: `POST /v1/videos`

```bash
curl -X POST "https://your-api.com/v1/videos" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "图像与视频融合生成",
    "images": ["https://example.com/image.jpg"],
    "videos": ["https://example.com/video.mp4"],
    "seconds": 5
  }'
```

---

### 2. Seedance 专属 API

**端点**: `POST /seedance/v1/videos/generations`

```bash
curl -X POST "https://your-api.com/seedance/v1/videos/generations" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -d '{
    "model": "seedance-2.0",
    "prompt": "专业的图像视频混合生成示例",
    "images": [
      "https://example.com/character-design.jpg",
      "https://example.com/background.jpg"
    ],
    "videos": [
      "https://example.com/motion-template.mp4"
    ],
    "seconds": 10,
    "metadata": {
      "resolution": "1080p",
      "ratio": "16:9",
      "seed": 12345,
      "camera_fixed": true,
      "watermark": false
    }
  }'
```

---

## 请求参数

### 核心参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `model` | string | ✅ | 模型名称，如 `seedance-2.0` |
| `prompt` | string | ✅ | 文本提示，描述期望的视频效果 |
| `images` | string[] | ❌ | 图像 URL 数组，用于 Image-to-Video |
| `videos` | string[] | ❌ | 视频 URL 数组，用于 Video-to-Video |
| `seconds` | string | ❌ | 视频时长（秒），默认 5 |
| `duration` | int | ❌ | 视频时长（秒），与 seconds 等价 |

### 高级参数 (通过 metadata 传递)

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `resolution` | string | `720p` | 分辨率：`720p`, `1080p` |
| `ratio` | string | `16:9` | 画面比例：`16:9`, `9:16`, `1:1`, `4:3` |
| `seed` | int | 随机 | 随机种子，用于复现结果 |
| `watermark` | bool | `false` | 是否添加水印 |
| `camera_fixed` | bool | `false` | 是否固定相机视角 |
| `service_tier` | string | `standard` | 服务层级：`standard`, `priority` |
| `return_last_frame` | bool | `false` | 是否返回最后一帧 |
| `generate_audio` | bool | `false` | 是否生成配套音频 |

### 图像要求

- **格式**: JPEG, PNG, WebP
- **大小**: 最大 10MB
- **分辨率**: 建议 512x512 以上
- **URL 要求**: 必须可公开访问

### 视频要求

- **格式**: MP4, MOV, AVI
- **大小**: 最大 50MB
- **时长**: 建议 3-30 秒
- **分辨率**: 建议 720p 以上
- **URL 要求**: 必须可公开访问

---

## 响应格式

### 提交成功响应

```json
{
  "code": "success",
  "data": {
    "task_id": "task_abc123def456",
    "status": "submitted",
    "model": "seedance-2.0",
    "created_at": 1234567890
  }
}
```

### 任务状态查询

**端点**: `GET /v1/video/generations/{task_id}`

```bash
curl -X GET "https://your-api.com/v1/video/generations/task_abc123def456" \
  -H "Authorization: Bearer YOUR_API_TOKEN"
```

**处理中响应**:

```json
{
  "code": "success",
  "data": {
    "task_id": "task_abc123def456",
    "status": "processing",
    "progress": "50%",
    "model": "seedance-2.0",
    "created_at": 1234567890,
    "updated_at": 1234567920
  }
}
```

**完成响应**:

```json
{
  "code": "success",
  "data": {
    "task_id": "task_abc123def456",
    "status": "success",
    "progress": "100%",
    "url": "https://example.com/generated-video.mp4",
    "model": "seedance-2.0",
    "created_at": 1234567890,
    "completed_at": 1234567950,
    "metadata": {
      "resolution": "1080p",
      "ratio": "16:9",
      "duration": 5,
      "seed": 42
    }
  }
}
```

### OpenAI 兼容格式响应

**端点**: `GET /v1/videos/{task_id}`

```json
{
  "id": "task_abc123def456",
  "task_id": "task_abc123def456",
  "status": "success",
  "progress": "100%",
  "model": "seedance-2.0",
  "created_at": 1234567890,
  "completed_at": 1234567950,
  "metadata": {
    "url": "https://example.com/generated-video.mp4"
  }
}
```

---

## 获取生成的视频

### 方式一：通过代理 URL (推荐)

**端点**: `GET /v1/videos/{task_id}/content`

```bash
curl -X GET "https://your-api.com/v1/videos/task_abc123def456/content" \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -o generated-video.mp4
```

**优势**:
- ✅ 自动处理 URL 签名和认证
- ✅ 支持 SSRF 保护
- ✅ 内置缓存机制 (24小时)
- ✅ 统一的访问接口

### 方式二：直接使用返回的 URL

```bash
curl -o generated-video.mp4 "https://example.com/generated-video.mp4"
```

---

## 使用场景示例

### 场景 1: 角色动画生成

使用角色设计图 + 动作参考视频生成角色动画：

```json
{
  "model": "seedance-2.0",
  "prompt": "让角色按照参考视频中的动作进行舞蹈，保持角色设计的一致性",
  "images": [
    "https://example.com/character-design.png"
  ],
  "videos": [
    "https://example.com/dance-motion.mp4"
  ],
  "seconds": 8,
  "metadata": {
    "resolution": "1080p",
    "ratio": "9:16",
    "camera_fixed": true
  }
}
```

### 场景 2: 场景风格迁移

使用场景图片 + 风格视频生成新视频：

```json
{
  "model": "seedance-2.0",
  "prompt": "将静态场景按照参考视频的艺术风格动态化",
  "images": [
    "https://example.com/static-scene.jpg"
  ],
  "videos": [
    "https://example.com/art-style-video.mp4"
  ],
  "seconds": 6,
  "metadata": {
    "resolution": "720p",
    "seed": 42
  }
}
```

### 场景 3: 多图多视频融合

结合多个参考素材生成综合视频：

```json
{
  "model": "seedance-2.0",
  "prompt": "融合所有参考素材，创造一段流畅的动态场景",
  "images": [
    "https://example.com/character.jpg",
    "https://example.com/background.jpg",
    "https://example.com/props.jpg"
  ],
  "videos": [
    "https://example.com/motion-reference1.mp4",
    "https://example.com/motion-reference2.mp4"
  ],
  "seconds": 10,
  "metadata": {
    "resolution": "1080p",
    "ratio": "16:9",
    "service_tier": "priority"
  }
}
```

### 场景 4: 产品广告视频

使用产品图 + 动态模板生成广告视频：

```json
{
  "model": "seedance-2.0",
  "prompt": "专业的产品展示视频，突出产品特点和质感",
  "images": [
    "https://example.com/product-photo.jpg"
  ],
  "videos": [
    "https://example.com/ad-template.mp4"
  ],
  "seconds": 5,
  "metadata": {
    "resolution": "1080p",
    "ratio": "9:16",
    "watermark": false
  }
}
```

---

## 状态说明

| 状态 | 说明 | 进度 |
|------|------|------|
| `submitted` | 任务已提交 | 10% |
| `queued` | 任务排队中 | 20% |
| `processing` | 正在生成 | 30%-90% |
| `success` | 生成成功 | 100% |
| `failed` | 生成失败 | 100% |

---

## 错误处理

### 错误响应格式

```json
{
  "error": {
    "message": "错误描述信息",
    "type": "错误类型",
    "code": "错误代码"
  }
}
```

### 常见错误

| HTTP 状态码 | 错误类型 | 说明 | 解决方案 |
|------------|---------|------|---------|
| 400 | `invalid_request_error` | 请求参数错误 | 检查必填参数和格式 |
| 400 | `invalid_request_error` | 任务未完成 | 等待任务完成后再获取 |
| 401 | `authentication_error` | 认证失败 | 检查 API Token |
| 403 | `server_error` | 请求被安全策略阻止 | 检查 URL 是否可访问 |
| 404 | `invalid_request_error` | 任务不存在 | 检查 task_id |
| 429 | `rate_limit_error` | 请求频率超限 | 降低请求频率 |
| 500 | `server_error` | 服务器内部错误 | 稍后重试 |

---

## 最佳实践

### 1. 图像和视频准备

- ✅ 使用高质量的参考素材
- ✅ 确保 URL 可公开访问（或使用预签名 URL）
- ✅ 图像分辨率建议 1024x1024 以上
- ✅ 视频时长建议 3-10 秒
- ✅ 保持输入素材的风格一致性

### 2. Prompt 编写技巧

- ✅ 清晰描述期望的视频效果
- ✅ 说明图像和视频素材的作用
- ✅ 指定运动方式和风格
- ✅ 包含场景、动作、氛围描述

**示例**:
```
"角色从静止状态开始，按照参考视频中的舞蹈动作进行表演，
保持角色设计的一致性，背景使用提供的场景图，
整体风格温暖明亮，相机保持中景视角"
```

### 3. 参数优化

- 使用 `seed` 参数复现满意的结果
- 测试不同的 `resolution` 和 `ratio` 组合
- 重要任务使用 `service_tier: "priority"`
- 调试阶段使用较低分辨率节省成本

### 4. 错误处理

```python
import requests
import time

def generate_video(prompt, images, videos, max_retries=3):
    # 提交任务
    response = requests.post(
        "https://your-api.com/v1/video/generations",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={
            "model": "seedance-2.0",
            "prompt": prompt,
            "images": images,
            "videos": videos,
            "seconds": 5
        }
    )
    
    task_id = response.json()["data"]["task_id"]
    
    # 轮询任务状态
    for _ in range(max_retries):
        time.sleep(10)
        status_response = requests.get(
            f"https://your-api.com/v1/video/generations/{task_id}",
            headers={"Authorization": f"Bearer {TOKEN}"}
        )
        
        status = status_response.json()["data"]["status"]
        if status == "success":
            # 下载视频
            video_response = requests.get(
                f"https://your-api.com/v1/videos/{task_id}/content",
                headers={"Authorization": f"Bearer {TOKEN}"}
            )
            with open("output.mp4", "wb") as f:
                f.write(video_response.content)
            return "output.mp4"
        elif status == "failed":
            raise Exception("Video generation failed")
    
    raise TimeoutError("Task timeout")
```

---

## 计费说明

- 按生成视频的分辨率和时长计费
- 使用图像/视频输入可能有额外的处理费用
- 失败的任务不收费
- 查看详细定价请参考官方文档

---

## 支持渠道

| 渠道类型 | 渠道 ID | 说明 |
|---------|---------|------|
| DoubaoVideo | 54 | 豆包视频 |
| VolcEngine | 45 | 火山引擎 |
| Seedance | 58 | Seedance 专属 |

---

## 相关文档

- [火山方舟 - 获取 API Key](https://www.volcengine.com/docs/82379/1541594?lang=zh)
- [Seedance 2.0 API 参考](https://www.volcengine.com/docs/82379/1520757?lang=zh)
- [查询视频生成任务](https://www.volcengine.com/docs/82379/1521309?lang=zh)

---

## 更新日志

### 2026-04-17
- ✨ 新增视频 URL 上传支持 (`videos` 字段)
- ✨ 新增图像+视频混合输入支持
- 📝 完善 API 文档和示例

---

## 技术支持

如有问题，请参考：
- 项目文档: [New API GitHub](https://github.com/QuantumNous/new-api)
- 火山引擎文档: [火山方舟](https://www.volcengine.com/docs/82379)
