# 素材库 API 文档

素材库用于管理图片、视频、音频素材。素材经上游审核通过后，可在视频生成任务中以
`asset://<officialId>` 的形式引用，替代公网 URL。

相比直接传公网 URL，素材引用的优势：预先完成审核（提交生成任务时不会再因审核失败）、
不受 URL 时效与可达性影响、多模态场景下更稳定。

---

## 基础信息

| 项 | 值 |
|---|---|
| Base URL | `https://www.metamind.yun` |
| 鉴权 | `Authorization: Bearer <你的令牌>` |
| 请求格式 | `application/json` |

所有接口路径以 `/v1/assets` 开头。

### 错误格式

```json
{
  "error": {
    "message": "asset not found: asset-abc123",
    "type": "invalid_request_error",
    "code": "asset_not_found"
  }
}
```

按 `error.code` 判断，不要匹配 `message` 文本。完整错误码见文末。

### 数据隔离

素材按令牌所属用户隔离。你只能查询、引用、删除自己上传的素材，
用别人的 `officialId` 会得到 `404 asset_not_found`。

---

## 0. 查询能力（建议先调这个）

不同上游支持的能力不同。集成前先查一次，据此决定走哪条路径。

```
GET /v1/assets/capabilities
```

```bash
curl https://www.metamind.yun/v1/assets/capabilities \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "provider": "metamind",
  "batchCreate": true,
  "excelTemplate": false,
  "regions": false,
  "groupTypes": ["AIGC", "LivenessFace"],
  "renameAsset": true,
  "deleteGroup": true,
  "batchMaxItems": 50
}
```

| 字段 | 含义 |
|---|---|
| `regions` | 为 `true` 时素材组用 **region**（`cn` / `intl`）分类 |
| `groupTypes` | 非空时素材组用 **groupType** 分类，取值即此数组 |
| `batchCreate` | 是否可用批量上传 |
| `excelTemplate` | 是否支持 Excel 模板下载与表格批量上传 |
| `batchMaxItems` | 单次批量上传的条数上限 |

> `regions` 与 `groupTypes` **互斥**：一个上游只会有其中一种分类方式。
> 传了当前上游不支持的那个字段，会返回 `asset_unsupported_by_provider`，不会被静默忽略。

---

## 1. 创建素材组

素材必须归属于某个素材组，先建组再传素材。

```
POST /v1/assets/groups
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 素材组名称 |
| `description` | string | 否 | 描述 |
| `region` | string | 否 | 仅当 `capabilities.regions=true` 时可传：`cn` / `intl` |
| `groupType` | string | 否 | 仅当 `capabilities.groupTypes` 非空时可传，取值来自该数组 |

**分类字段创建后不可修改**，且决定了这个组里的素材能配哪些模型，建组前想清楚。

```bash
# 上游用 groupType 分类时
curl -X POST https://www.metamind.yun/v1/assets/groups \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"产品素材","description":"宣传图","groupType":"AIGC"}'

# 上游用 region 分类时
curl -X POST https://www.metamind.yun/v1/assets/groups \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"产品素材","region":"cn"}'
```

```json
{
  "id": 7,
  "officialId": "group-abc123",
  "name": "产品素材",
  "description": "宣传图",
  "groupType": "AIGC",
  "provider": "metamind",
  "_count": { "assets": 0 },
  "createdAt": 1785311958
}
```

记下 `officialId`，后续上传素材要用。

---

## 2. 查询素材组列表

```
GET /v1/assets/groups
```

```bash
curl https://www.metamind.yun/v1/assets/groups \
  -H "Authorization: Bearer $API_KEY"
```

返回数组，每项结构同上，`_count.assets` 是该组下的素材数量。

---

## 3. 上传单个素材

```
POST /v1/assets
```

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `groupId` | string | 是 | 素材组的 `officialId` |
| `url` | string | 是 | 素材的**公网可访问** URL，仅支持 http / https |
| `name` | string | 否 | 素材名称，不传时按 URL 文件名生成 |
| `assetType` | string | 否 | `Image` / `Video` / `Audio`，不传时按 URL 后缀推断 |

```bash
curl -X POST https://www.metamind.yun/v1/assets \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "groupId": "group-abc123",
    "url": "https://example.com/product.jpg",
    "name": "产品主图"
  }'
```

```json
{
  "id": 42,
  "officialId": "asset-abc123",
  "groupId": "group-abc123",
  "name": "产品主图",
  "status": "Processing",
  "assetType": "Image",
  "url": "https://example.com/product.jpg",
  "assetRef": "asset://asset-abc123",
  "provider": "metamind",
  "createdAt": 1785311958
}
```

`assetRef` 是便利字段，可以直接填进生成任务，不用自己拼字符串。

**刚上传的素材是 `Processing` 状态，还不能用**，需要轮询到 `Active`。

---

## 4. 批量上传

```
POST /v1/assets/batch
```

请求体是一个**数组**，每项结构同单个上传。条数上限见 `capabilities.batchMaxItems`。

```bash
curl -X POST https://www.metamind.yun/v1/assets/batch \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '[
    {"groupId":"group-abc123","url":"https://example.com/a.jpg","name":"图A"},
    {"groupId":"group-abc123","url":"https://example.com/b.jpg"}
  ]'
```

```json
{
  "batchId": "a1b2c3d4",
  "total": 2,
  "results": [
    { "index": 0, "status": "ok", "officialId": "asset-aaa" },
    { "index": 1, "status": "error", "error": "invalid url" }
  ]
}
```

`index` 对应你提交数组的下标，据此把 `officialId` 映射回自己的原始条目。
**部分失败不影响其余条目**，逐条判断 `status`。

`batchId` 可能为空——上游没有原生批量接口时由网关循环单条实现，
对你来说响应形态一致。

---

## 5. 查询单个素材

```
GET /v1/assets/{officialId}
```

```bash
curl https://www.metamind.yun/v1/assets/asset-abc123 \
  -H "Authorization: Bearer $API_KEY"
```

**这个接口会顺带向上游同步最新状态**，所以轮询审核结果用它。

### 状态说明

| status | 含义 |
|---|---|
| `Processing` | 审核中，还不能引用 |
| `Active` | 审核通过，可以引用 |
| `Failed` | 审核未通过，`failReason` 里有原因 |

审核通常在几十秒内完成，建议每 5 秒轮询一次。

---

## 6. 查询素材列表

```
GET /v1/assets
```

| 查询参数 | 说明 |
|---|---|
| `groupId` | 按素材组筛选 |
| `status` | `Processing` / `Active` / `Failed` |
| `assetType` | `Image` / `Video` / `Audio` |
| `keyword` | 按名称或 officialId 模糊匹配 |
| `page_num` | 页码，默认 1 |
| `page_size` | 每页数量，默认 20，最大 100 |
| `refresh` | 传 `true` 时先向上游同步待处理素材的状态 |

```bash
curl "https://www.metamind.yun/v1/assets?status=Active&page_size=50" \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "items": [ { "officialId": "asset-abc123", "status": "Active", "assetRef": "asset://asset-abc123" } ],
  "total": 1,
  "page_num": 1,
  "page_size": 50
}
```

> **性能提示**：默认只查本地记录，响应很快。带 `refresh=true` 会逐条回源同步，
> 明显更慢，只在需要刷新审核状态时用。轮询单个素材请用第 5 节的接口。

---

## 7. 删除素材

```
DELETE /v1/assets/{officialId}
```

```bash
curl -X DELETE https://www.metamind.yun/v1/assets/asset-abc123 \
  -H "Authorization: Bearer $API_KEY"
```

```json
{ "officialId": "asset-abc123", "deleted": true }
```

已删除的素材不能再用于新任务，但不影响已提交的任务。

---

## 8. 在生成任务中引用素材

素材变成 `Active` 之后，把 `assetRef` 填进视频生成请求：

```bash
curl -X POST https://www.metamind.yun/v1/video/generations \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "moma-seedance-2.0",
    "prompt": "图片中的人物缓缓转头微笑，背景樱花飘落",
    "image": "asset://asset-abc123",
    "duration": 5,
    "ratio": "16:9",
    "resolution": "720p"
  }'
```

网关会在转发前校验素材引用，以下情况**不会打到上游**，直接返回明确错误：

| 情况 | 错误码 |
|---|---|
| 素材不存在或不属于你 | `asset_not_found` |
| 素材还没审核通过 | `asset_not_active` |
| 素材与模型的区域不匹配（仅有区域概念的上游） | `asset_region_mismatch` |
| 素材是切换上游之前创建的，已失效 | `asset_provider_mismatch` |

不含 `asset://` 的请求不受任何影响，行为与之前完全一致。

---

## 完整流程示例

```bash
API_KEY=sk-your-token
BASE=https://www.metamind.yun/v1

# 1. 看当前上游支持什么
curl -s $BASE/assets/capabilities -H "Authorization: Bearer $API_KEY"

# 2. 建组（按上一步的结果决定用 groupType 还是 region）
GROUP=$(curl -s -X POST $BASE/assets/groups \
  -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d '{"name":"演示","groupType":"AIGC"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['officialId'])")

# 3. 传素材
ASSET=$(curl -s -X POST $BASE/assets \
  -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d "{\"groupId\":\"$GROUP\",\"url\":\"https://example.com/a.jpg\"}" \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['officialId'])")

# 4. 轮询到 Active
until [ "$(curl -s $BASE/assets/$ASSET -H "Authorization: Bearer $API_KEY" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["status"])')" = "Active" ]; do
  echo "审核中…"; sleep 5
done

# 5. 用起来
curl -s -X POST $BASE/video/generations \
  -H "Authorization: Bearer $API_KEY" -H "Content-Type: application/json" \
  -d "{\"model\":\"moma-seedance-2.0\",\"prompt\":\"人物转头微笑\",\"image\":\"asset://$ASSET\"}"
```

现成的脚本见 `docs/examples/upload_assets.py`，它会自动探测能力、建组、上传、
轮询到通过并打印可用的引用。

---

## 错误码一览

| code | HTTP | 含义与处理 |
|---|---|---|
| `asset_invalid_request` | 400 | 参数有误，看 `message` |
| `asset_not_found` | 404 | 素材不存在或不属于你 |
| `asset_not_active` | 400 | 素材未通过审核，等到 `Active` 再引用 |
| `asset_region_mismatch` | 400 | 素材区域与模型不匹配 |
| `asset_provider_mismatch` | 409 | 素材在上游切换前创建，已失效，只能删除 |
| `asset_unsupported_by_provider` | 501 | 当前上游不支持该能力，先查 capabilities |
| `asset_quota_exceeded` | 403 | 素材数量超出管理员设定的上限 |
| `insufficient_quota` | 403 | 账户额度耗尽 |
| `assets_rate_limit_exceeded` | 429 | 触发限流，稍后重试 |
| `assets_channel_not_configured` | 503 | 管理员尚未配置素材渠道 |
| `assets_channel_ambiguous` | 503 | 存在多个可用渠道，需管理员显式指定 |
| `asset_upstream_error` | 502 | 上游返回错误，`message` 里有详情 |

---

## 注意事项

- 素材 URL 必须**公网可访问**，仅支持 `http` / `https`。
- 素材上传**不计费**，但受频次限流约束（默认每用户每分钟 60 次）。
- 素材数量可能有上限，超出返回 `asset_quota_exceeded`，具体值问管理员。
- 生成结果的视频链接通常 24 小时内有效，请及时下载保存。
- 素材列表、素材组列表由网关本地维护，绕过网关直接在上游控制台创建的素材不会出现在这里。
