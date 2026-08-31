# 润元（Runyuan）图生视频 API 文档

本文档描述如何通过 NewAPI 调用润元（Runyuan）视频生成能力，包括素材组管理、素材上传、视频生成、任务查询和视频下载。

## 基础信息

- **Base URL**: `https://www.metamind.yun`（生产环境）
- **鉴权方式**: `Authorization: Bearer sk-...`
- **Content-Type**: `application/json`

> **响应形态有三种，不要用一套解包逻辑套所有接口**（下表为 2026-08-22 实测）：
>
> | 接口 | 形态 |
> |---|---|
> | 素材接口（第 1、2 节） | **裸返回** —— 直接是目标对象或数组 |
> | 提交视频任务 `POST /v1/video/generations`（3.1） | **裸返回** —— OpenAI Video 对象 |
> | 查询任务 `GET /v1/video/generations/{task_id}`（4.1） | `{"code":"success","data":{...}}` **包装** |
> | 查询任务 `GET /v1/videos/{task_id}`（4.2） | **裸返回** —— OpenAI Video 对象 |
>
> 失败时统一是 `{"error":{"code","message","type"}}`，以 HTTP 状态码区分成败。

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

**响应示例**（裸数组，按创建时间倒序）：

```json
[
  {
    "id": 3,
    "officialId": "group-20260819151939-3696",
    "name": "测试",
    "groupType": "AIGC",
    "provider": "runyuan",
    "_count": { "assets": 0 },
    "createdAt": 1787123980
  },
  {
    "id": 1,
    "officialId": "group-20260818152945-6253",
    "name": "default-assets",
    "groupType": "AIGC",
    "provider": "runyuan",
    "_count": { "assets": 0 },
    "createdAt": 1787038186
  }
]
```

> `_count.assets` 统计的是**本地记录中 groupId 指向该组的素材数**。实测该值恒为 0
> 且与素材列表对不上，原因见第 8 节。

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

**响应示例**（裸对象）：

```json
{
  "id": 3,
  "officialId": "group-20260819151939-3696",
  "name": "my-assets",
  "groupType": "AIGC",
  "provider": "runyuan",
  "_count": { "assets": 0 },
  "createdAt": 1787123980
}
```

> `groupType` 目前只支持 `AIGC`（见 `GET /v1/assets/capabilities` 返回的 `groupTypes`）。
> `LivenessFace` 类型的素材组无法通过本接口创建，只能由真人认证流程产出。

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

**响应示例**（裸对象）：

```json
{
  "id": 19,
  "officialId": "asset-20260822112830-5jgkm",
  "groupId": "group-20260819151939-3696",
  "name": "api-test-112829",
  "status": "Processing",
  "assetType": "Image",
  "url": "https://example.com/public-image.jpg",
  "assetRef": "asset://asset-20260822112830-5jgkm",
  "provider": "runyuan",
  "createdAt": 1787369311,
  "updatedAt": 1787369311
}
```

素材是异步处理的，刚注册时 `status` 为 `Processing`，需轮询 2.2 等待转 `Active`。

### 2.2 查询素材状态

```http
GET /v1/assets/{officialId}?refresh=true
```

带 `refresh=true` 时会回源上游同步最新状态，不带则只读本地记录。

**响应示例**（裸对象）：

```json
{
  "id": 16,
  "officialId": "asset-20260822101241-l244w",
  "groupId": "group-20260819112518-5p6j9",
  "name": "reference_image",
  "status": "Active",
  "assetType": "Image",
  "url": "https://ark-media-asset.tos-cn-beijing.volces.com/...(带签名参数)",
  "assetRef": "asset://asset-20260822101241-l244w",
  "provider": "runyuan",
  "createdAt": 1787364762,
  "updatedAt": 1787364768
}
```

**状态说明**：

| 状态 | 说明 |
|---|---|
| `Processing` | 处理中，刚注册时的初始状态 |
| `Active` | 可用，可引用到视频任务 |
| `Failed` | 处理失败，`failReason` 里有原因 |

> 素材转 `Active` 后，`url` 会被上游替换成它自己对象存储上的**带签名地址**（`X-Tos-Expires`
> 实测为 41400 秒 ≈ 11.5 小时），不再是你注册时传入的原始 URL。
>
> **注意**：`refresh=true` 还会用上游返回的 `groupId` 覆盖本地记录，而这个值与你注册时
> 指定的组不一致，见第 8 节。

### 2.3 列出素材

```http
GET /v1/assets
```

**查询参数**：

| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `groupId` | string | — | 按素材组 officialId 过滤 |
| `status` | string | — | 按状态过滤，如 `Active` / `Processing` |
| `assetType` | string | — | 按类型过滤，如 `Image` |
| `keyword` | string | — | 素材名称模糊匹配 |
| `page_num` | int | 1 | 页码 |
| `page_size` | int | 20 | 每页数量 |
| `refresh` | bool | false | 传 `true` 时回源同步处理中的素材状态 |

> **`groupId` 过滤当前不可靠** —— 素材经过一次 `refresh` 后本地 groupId 会被上游改写，
> 用注册时的组 ID 过滤会查不到，见第 8 节。

**响应示例**：

```json
{
  "items": [
    {
      "id": 16,
      "officialId": "asset-20260822101241-l244w",
      "groupId": "group-20260819112518-5p6j9",
      "name": "reference_image",
      "status": "Active",
      "assetType": "Image",
      "url": "https://ark-media-asset.tos-cn-beijing.volces.com/...(带签名参数)",
      "assetRef": "asset://asset-20260822101241-l244w",
      "provider": "runyuan",
      "createdAt": 1787364762,
      "updatedAt": 1787364768
    }
  ],
  "total": 16,
  "page_num": 1,
  "page_size": 1
}
```

**两点行为需要注意**：

1. **只查本地记录，不透传上游。** 只返回当前令牌所属用户的素材 —— 全站共用一个上游账号，透传上游列表会把其他用户的素材一起暴露出去。
2. **默认不回源。** 回源是同步 HTTP 调用，会拖慢列表响应，因此只有带 `refresh=true` 时才会挑最多 20 条仍处于 `Processing` 的素材去上游同步状态。前端轮询与用户主动刷新时才需要带这个参数。

### 2.4 删除素材

```http
DELETE /v1/assets/{officialId}
```

**响应示例**：

```json
{
  "officialId": "asset-20260819xxxxxx-xxxx",
  "deleted": true
}
```

**执行顺序**：先校验归属 → 删除上游素材 → 软删除本地记录。

**几种边界情况**：

| 情况 | 行为 |
|---|---|
| 素材不属于当前用户 | 返回 404，不会误删他人素材 |
| 上游已不存在（404） | 视为删除成功，本地照常清理 |
| 素材注册于上游切换之前 | 只清理本地记录，不去动新上游，响应多带 `upstreamSkipped: true` |

上游切换后的响应：

```json
{
  "officialId": "asset-20260819xxxxxx-xxxx",
  "deleted": true,
  "upstreamSkipped": true
}
```

素材不存在时：

```json
{
  "error": {
    "code": "asset_not_found",
    "message": "asset not found: asset-20260819xxxxxx-xxxx",
    "type": "invalid_request_error"
  }
}
```

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

**响应示例**（裸对象，OpenAI Video 形态）：

```json
{
  "id": "task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR",
  "task_id": "task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR",
  "object": "video",
  "model": "doubao-seedance-2.0",
  "status": "queued",
  "progress": 0,
  "created_at": 1787370064
}
```

> 这里的 `task_id` 是网关生成的公开 ID，不是上游的真实任务 ID。

## 4. 任务查询

### 4.1 查询任务状态

```http
GET /v1/video/generations/{task_id}
```

**响应示例**（包装；`data` 是完整的任务记录，字段远多于下面截取的部分）：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "id": 14,
    "task_id": "task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR",
    "status": "SUCCESS",
    "progress": "100%",
    "result_url": "https://www.metamind.yun/v1/videos/task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR/content",
    "fail_reason": "",
    "action": "generate",
    "submit_time": 1787370064,
    "start_time": 1787370099,
    "finish_time": 1787370244,
    "created_at": 1787370064,
    "updated_at": 1787370244,

    "platform": "54",
    "user_id": 1,
    "group": "default",
    "channel_id": 1,
    "quota": 787671,
    "properties": { "upstream_model_name": "doubao-seedance-2.0", "origin_model_name": "doubao-seedance-2.0" },
    "data": { "id": "sd_1787370062953_2339", "status": "succeeded", "usage": { "total_tokens": 108900 }, "…": "上游原始响应" }
  }
}
```

> ⚠️ **本接口会把内部字段一并返回**：`platform`（渠道类型）、`channel_id`、`user_id`、
> `quota`（计费额度）、`group`，以及 `data` 里上游的原始响应（含上游真实任务 ID）。
> 它更适合作为控制台的内部接口。**对外的客户端建议改用 4.2 的 OpenAI 兼容端点。**

**状态说明**（实测取值）：

| 状态 | 说明 |
|---|---|
| `NOT_START` | 已入库、尚未开始 |
| `QUEUED` | 排队中 |
| `IN_PROGRESS` | 生成中 |
| `SUCCESS` | 生成成功 |
| `FAILED` | 生成失败 |

### 4.2 查询任务状态（OpenAI 兼容，推荐）

```http
GET /v1/videos/{task_id}
```

**响应示例**（裸对象，不含任何内部字段）：

```json
{
  "id": "task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR",
  "task_id": "task_H2q2LJR9a5PosLOn5YJpNdwJsjleCMZR",
  "object": "video",
  "status": "completed",
  "progress": 100,
  "model": "doubao-seedance-2.0",
  "metadata": { "url": "https://www.metamind.yun/v1/videos/task_.../content" }
}
```

## 5. 视频下载

### 5.1 通过代理接口下载

```http
GET /v1/videos/{task_id}/content
```

**请求头**：

```http
Authorization: Bearer sk-...
```

该接口返回视频二进制流，可直接保存为 `.mp4`。**必须带 `Authorization`**，
不带会返回 401 —— 2026-08-22 实测：带 token 拿到 2,578,198 字节的 mp4，不带 token 返回 401。

> ⚠️ **部署必看**：`result_url` 是用系统设置里的「服务器地址」拼出来的。若该项没配，
> 返回给客户端的会是 `http://localhost:3000/v1/videos/.../content` —— 客户端根本访问不到。
> 请确认后台「系统设置 → 服务器地址」填的是站点的公网地址。

**为什么要走代理而不是直接给上游地址？**

实测上游地址（形如 `https://runy.yitd.cn/uploads/2026/08/22/xxx.mp4`）**并不需要鉴权** ——
网关回源时一个鉴权头都没带就拉到了。所以代理不是为了鉴权，而是为了：

1. 不把上游域名暴露给调用方；
2. 上游有 WAF，客户端直连可能被拦（实测本机 curl 该地址返回 405，与浏览器访问上游站点
   被防火墙阻断时的返回一致）；
3. 上游地址若将来加上有效期，代理层可以透明地重新取。

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

## 8. 已知问题：素材组归属与上游脱节

> 2026-08-22 实测复现，尚未修复。用 `bin/test_asset_api.py` 可复现。

**现象**：注册素材时指定的组，和上游实际归属的组不是同一个。

```
POST /v1/assets   groupId=group-20260819151939-3696   → 响应 groupId 一致
GET  /v1/assets/{id}?refresh=true                     → groupId=group-20260819151940-fl8bh
```

两个 ID 的时间戳只差 1 秒，但后缀完全不同 —— 说明上游并没有把素材放进指定的组。

**连带影响**：

1. **按 `groupId` 过滤查不到素材**。`refresh` 时 `applyUpstreamFields` 会用上游返回的
   groupId 覆盖本地 `GroupOfficialId`，素材被"搬"到一个本地没有记录的组里。
2. **素材组的 `_count.assets` 恒为 0**。实测账号下有 16 条素材，但三个素材组的计数全是 0 —— 
   这些素材的 groupId 指向 `group-20260819112518-5p6j9` 这类本地不存在的组。

**排查方向**：直接调上游 `GetAsset` 看原始响应里的 `GroupId`，确认是上游忽略了
`CreateAsset` 传入的 `GroupId`（文档 §2.4.1 说"未传时自动使用默认 AIGC 素材组"，
但我们是传了的），还是上游对素材组做了某种归一化。

在查清之前，**不要依赖 groupId 做素材归类**。
