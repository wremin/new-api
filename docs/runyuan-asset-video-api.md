# 润元（Runyuan）图生视频 API 文档

本文档描述如何通过 NewAPI 调用润元（Runyuan）视频生成能力，包括素材组管理、素材上传、视频生成、任务查询和视频下载。

## 基础信息

- **Base URL**: `https://www.metamind.yun`（生产环境）
- **鉴权方式**: `Authorization: Bearer sk-...`
- **Content-Type**: `application/json`

## 认证

所有接口（除健康检查外）都需要在请求头中携带令牌：

```http
Authorization: Bearer sk-你的令牌
```

## 1. 素材组管理

### 1.1 列出素材组

```http
GET /v1/assets/groups
```

**请求头**：

```http
Authorization: Bearer sk-...
```

**响应示例**：

```json
{
  "code": "success",
  "message": "",
  "data": [
    {
      "officialId": "group-20260817174848-1289",
      "name": "default-assets",
      "groupType": "AIGC",
      "createdAt": 1787056995
    }
  ]
}
```

### 1.2 创建素材组

```http
POST /v1/assets/groups
```

**请求体**：

```json
{
  "name": "my-assets",
  "groupType": "AIGC"
}
```

**响应示例**：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "officialId": "group-20260819xxxxxx-xxxx",
    "name": "my-assets",
    "groupType": "AIGC"
  }
}
```

## 2. 素材注册

### 2.1 注册素材

```http
POST /v1/assets
```

**请求体**：

```json
{
  "groupId": "group-20260817174848-1289",
  "url": "https://example.com/public-image.jpg",
  "name": "reference_image",
  "assetType": "Image"
}
```

**字段说明**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `groupId` | string | 是 | 素材组 officialId |
| `url` | string | 是 | 素材图片公网 URL，必须可直接访问 |
| `name` | string | 是 | 素材名称 |
| `assetType` | string | 是 | 固定 `Image` |

**响应示例**：

```json
{
  "code": "success",
  "data": {
    "officialId": "asset-20260819xxxxxx-xxxx",
    "name": "reference_image",
    "status": "Pending"
  }
}
```

### 2.2 查询素材状态

```http
GET /v1/assets/{officialId}?refresh=true
```

**响应示例**：

```json
{
  "code": "success",
  "data": {
    "officialId": "asset-20260819xxxxxx-xxxx",
    "status": "Active",
    "assetRef": "asset://asset-20260819xxxxxx-xxxx"
  }
}
```

**状态说明**：

| 状态 | 说明 |
|---|---|
| `Pending` | 等待处理 |
| `Processing` | 处理中 |
| `Active` | 可用，可引用到视频任务 |
| `Failed` | 处理失败 |

## 3. 视频生成

### 3.1 提交图生视频任务

```http
POST /v1/video/generations
```

**请求体**：

```json
{
  "model": "doubao-seedance-2.0",
  "prompt": "人物自然地看向镜头并微微点头，保持面部特征一致，写实电影感，自然光。",
  "content": [
    {
      "type": "text",
      "text": "人物自然地看向镜头并微微点头，保持面部特征一致，写实电影感，自然光。"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "asset://asset-20260819xxxxxx-xxxx"
      },
      "role": "reference_image"
    }
  ],
  "ratio": "16:9",
  "duration": 5,
  "resolution": "720p",
  "generate_audio": false,
  "watermark": false
}
```

**字段说明**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `model` | string | 是 | 模型名，如 `doubao-seedance-2.0` |
| `prompt` | string | 是 | 视频生成提示词 |
| `content` | array | 是 | 包含 text 和 image_url 的多模态内容 |
| `ratio` | string | 否 | 宽高比，如 `16:9`、`9:16`、`1:1` |
| `duration` | int | 否 | 视频时长（秒） |
| `resolution` | string | 否 | 分辨率，如 `720p`、`1080p` |
| `generate_audio` | bool | 否 | 是否生成音频 |
| `watermark` | bool | 否 | 是否添加水印 |

**响应示例**：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_xxxxxxxxxxxxxx",
    "status": "queued"
  }
}
```

## 4. 任务查询

### 4.1 查询任务状态

```http
GET /v1/video/generations/{task_id}
```

**响应示例**：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_xxxxxxxxxxxxxx",
    "status": "SUCCESS",
    "progress": "100%",
    "result_url": "https://www.metamind.yun/v1/videos/task_xxxxxxxxxxxxxx/content"
  }
}
```

**状态说明**：

| 状态 | 说明 |
|---|---|
| `QUEUED` | 排队中 |
| `IN_PROGRESS` | 生成中 |
| `SUCCESS` | 生成成功 |
| `FAILED` | 生成失败 |

## 5. 视频下载

### 5.1 通过代理接口下载

```http
GET /v1/videos/{task_id}/content
```

**请求头**：

```http
Authorization: Bearer sk-...
```

该接口会返回视频二进制流，可直接下载保存为 `.mp4`。

## 6. 常见错误

### 6.1 图片包含真人

```json
{
  "error": {
    "code": "InputImageSensitiveContentDetected.PrivacyInformation",
    "message": "The request failed because the input image 'content[1]' may contain real person."
  }
}
```

**原因**：润元检测到输入图片可能包含真实人物，触发隐私安全策略。

**解决**：换用非真人图片，如动漫、卡通、风景等。

### 6.2 图片 URL 不可访问

```json
{
  "error": {
    "message": "Failed to download media from the provided URL. Please check if the link is accessible."
  }
}
```

**原因**：润元服务器无法下载提供的图片 URL。

**解决**：确保图片 URL 公网可访问，无签名过期、IP 限制等问题。

### 6.3 任务解析失败

```json
{
  "error": {
    "message": "unmarshal task result failed, unknown format"
  }
}
```

**原因**：上游响应格式与预期不符（如 `tools` 字段类型不一致）。

**解决**：升级 new-api 到包含润元 `tools: {}` 兼容修复的版本。

## 7. Python 脚本示例

```bash
python test/asset2video.py \
  --api-key "sk-..." \
  --base-url "https://www.metamind.yun" \
  --group-id "group-20260817174848-1289" \
  --image-url "https://example.com/public-image.jpg" \
  --output output.mp4
```
