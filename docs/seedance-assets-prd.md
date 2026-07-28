# PRD：new-api 接入 Seedance 2.0 素材库（Assets）接口

| 项目 | 内容 |
|---|---|
| 文档版本 | v2.2（单渠道版；决策已关闭，待 M0 联调验证若干上游细节） |
| 日期 | 2026-07-28 |
| 状态 | 待评审 |
| 上游文档 | seegen.ai for Business（`https://api.seegen.ai/app/guide`） |
| 涉及仓库 | `new-api`（Go 1.26 / Gin / GORM + React 18 / Vite / Semi Design） |

> **v2.0 变更**：明确本期**只支持单个素材渠道**，不做多渠道聚合。相比 v1.0 移除了渠道选择中间件、`asset://` 自动锁渠道中间件、region→模型→渠道映射。多渠道演进路径见 §10。
>
> **v2.1 变更**：§13 三个开放问题全部关闭——上游无配额上限、Excel 不做解析校验、不做存量导入。新增 §3.5（批量上传落库与异步回填）。工作量约 6.5 人日。
>
> **v2.2 变更**：§3.5 补充 `index` 对应关系的说明，并标注两处此前基于推断的假设（Excel 的 index 基准、视频/音频素材的 URL 字段名）。§12 新增 M0 上游联调 checklist，集中收拢全部待实测项。

---

## 1. 背景与目标

### 1.1 背景

new-api 已支持 Seedance 2.0 视频生成任务（`relay/channel/task/doubao`，渠道类型 `ChannelTypeDoubaoVideo=54`），下游客户端通过 `POST /v1/video/generations` 提交任务。

但 Seedance 2.0 的核心能力——**多模态参考生成**（首尾帧、参考图 / 参考视频 / 参考音频）——最佳实践是使用上游素材库中的 `asset://<officialId>` 引用，而非公网 URL：

- `asset://` 素材经过上游预审核，提交任务时不会因审核不通过而失败；
- 不受公网 URL 时效性 / 可达性影响；
- region（cn / intl）与模型自动匹配，国际版与大尺度模型**强制要求**素材位于 `region=intl` 的素材组下。

目前 new-api **没有暴露任何素材管理接口**，用户无法通过 new-api 获得 `asset://` 引用，只能绕过网关直连上游，导致：多模态场景在 new-api 上不可用、素材归属无法审计、无法做用量管控。

### 1.2 目标

1. 在 new-api 中完整代理上游 seegen.ai 的素材管理接口（素材 CRUD + 批量上传 + 素材组），保持与上游**请求 / 响应格式完全一致**，使已按上游文档开发的客户端只需换 `base_url` 即可使用。
2. 在单一上游账号（= new-api 中的一个渠道）下，实现**多用户的素材归属隔离**——上游账号本身没有 new-api 的用户概念，同账号下所有素材对上游是平的。
3. 提供前端素材库管理页面，让用户在 new-api 控制台内完成上传、查看审核状态、复制 `asset://` 引用、删除。

### 1.3 非目标（本期不做）

- **多渠道 / 多上游账号支持**。本期假定全站只配置一个 Seedance 素材渠道。
- 本地文件直传（上游 `POST /v1/assets` 只接收公网 URL；new-api 侧不引入对象存储。仓库根目录的 `upload_assets.py` 是用户自备 OSS 的脚本方案，本期不纳入）。
- 素材内容审核 / 转码 / 缩略图生成。
- 素材上传计费（本期明确**完全免费**，见 §6）。

---

## 2. 上游接口清单（来源：seegen.ai 官方文档）

Base URL：`https://api.seegen.ai`，鉴权：`Authorization: Bearer <API_KEY>`

| # | 方法 | 路径 | 说明 |
|---|---|---|---|
| A1 | POST | `/v1/assets` | 上传单个素材（图片 / 视频 / 音频 URL），进入异步审核 |
| A2 | POST | `/v1/assets/batch` | 批量上传，最多 50 条；支持 JSON 数组或 Excel 文件（multipart） |
| A3 | GET | `/v1/assets/batch/template` | 下载批量上传 Excel 模板（二进制 xlsx） |
| A4 | GET | `/v1/assets/{officialId}` | 查询单个素材审核状态（Processing / Active / Failed） |
| A5 | GET | `/v1/assets` | 素材列表，支持 `groupId` / `status` / `page_num` / `page_size` |
| A6 | DELETE | `/v1/assets/{officialId}` | 软删除素材 |
| A7 | POST | `/v1/assets/groups` | 创建素材组（`name` / `description` / `region`，region 创建后不可改） |
| A8 | GET | `/v1/assets/groups` | 查询素材组列表（含素材数量统计 `_count.assets`） |

关键约束：

- **region 是素材组级属性，不是账号级属性**。同一个上游账号下可以同时存在 `region=cn` 和 `region=intl` 的素材组。因此单渠道方案下，region 完全由用户在创建素材组时决定，**new-api 无需为 region 做任何渠道路由**。
- `region=cn`（默认）用于 `doubao-seedance-2-0-*` 系列；`region=intl` 用于 `dreamina-seedance-2-0-*` 与大尺度模型 `ep-20260414121243-hp7w5` / `ep-20260414121306-pk5j6`。
- 素材上传返回 `officialId`（如 `asset-2026abc123`），路径参数用的是 `officialId` **字符串**，不是数字 `id`。
- 审核为异步流程，`status=Processing` 时需轮询；只有 `Active` 状态的素材才能在生成任务中引用。
- 生成任务中通过 `image_url.url` / `video_url.url` / `audio_url.url` 填写 `asset://<officialId>` 引用。

---

## 3. 下游接口设计（new-api 对客户端暴露）

### 3.1 路径映射

一比一透传，**下游路径与上游路径完全相同**，仅鉴权换成 new-api 的 token：

| new-api | → 上游 | 处理方式 |
|---|---|---|
| `POST /v1/assets` | `POST /v1/assets` | 透传 + 落库 |
| `POST /v1/assets/batch` | `POST /v1/assets/batch` | 透传（JSON 与 multipart 两种 Content-Type）+ 批量落库，见 §3.5 |
| `GET /v1/assets/batch/template` | `GET /v1/assets/batch/template` | 二进制流式透传 |
| `GET /v1/assets/:id` | `GET /v1/assets/{id}` | 归属校验 → 透传 → 回写本地 status |
| `GET /v1/assets` | — | **本地查询**，不透传（见 §3.4） |
| `DELETE /v1/assets/:id` | `DELETE /v1/assets/{id}` | 归属校验 → 透传 → 本地软删除 |
| `POST /v1/assets/groups` | `POST /v1/assets/groups` | 透传 + 落库 |
| `GET /v1/assets/groups` | — | **本地查询**，不透传（见 §3.4） |

鉴权：`Authorization: Bearer sk-xxx`（new-api token），沿用 `middleware.TokenAuth()`。

### 3.2 ⚠️ Gin 路由注册方案（实现风险点）

`GET /v1/assets/:id`、`GET /v1/assets/groups`、`GET /v1/assets/batch/template` 在同一 HTTP method 下同层级混用了通配段与静态段。当前依赖为 **gin v1.9.1**（`go.mod:20`），其路由树对同层级 wildcard 与 static 冲突存在 `panic: conflicts with existing wildcard` 风险。

**推荐方案（安全）**：单一 catch-all 路由 + 控制器内部分发，与仓库现有 `httpRouter.POST("/models/*path", ...)` 模式一致。

```go
// router/assets-router.go（新增）
func SetAssetsRouter(router *gin.Engine) {
    g := router.Group("/v1/assets")
    g.Use(middleware.RouteTag("relay"))
    g.Use(middleware.TokenAuth())
    g.Use(middleware.AssetsRateLimit())      // §6.2
    {
        g.POST("", controller.RelayAssetCreate)
        g.GET("", controller.RelayAssetList)
        // /batch、/batch/template、/groups、/<officialId> 统一由 *action 分发
        g.Any("/*action", controller.RelayAssetDispatch)
    }
}
```

`RelayAssetDispatch` 按 `c.Param("action")` 分发：`/batch`、`/batch/template`、`/groups`，其余视为 `officialId`。

**备选方案**：分别注册各静态路径，并在实现阶段用 `go test` 验证 gin 不 panic。若验证通过则采用，路由更直观。**该验证放在 M0 第一项**，因为它决定控制器的组织形式。

### 3.3 请求 / 响应格式

**完全对齐上游**，不做任何字段改写。示例：

```bash
# 上传单个素材
curl -X POST https://your-new-api.com/v1/assets \
  -H "Authorization: Bearer sk-newapi-xxx" \
  -H "Content-Type: application/json" \
  -d '{
    "groupId": "ag-2026xyz",
    "url": "https://example.com/my-image.jpg",
    "name": "产品主图"
  }'
```

响应（原样透传上游）：

```json
{
  "id": 42,
  "officialId": "asset-2026abc123",
  "name": "产品主图",
  "status": "Processing",
  "region": "cn",
  "imageUrl": "https://example.com/my-image.jpg",
  "createdAt": "2026-03-30T10:00:00.000Z"
}
```

错误格式沿用 new-api 现有的 OpenAI 风格错误体（参考 `controller/video_proxy.go` 中的 `videoProxyError`）：

```json
{ "error": { "message": "...", "type": "invalid_request_error", "code": "asset_not_found" } }
```

### 3.4 列表类接口为什么不透传

`GET /v1/assets` 与 `GET /v1/assets/groups` 从 new-api 本地表查询并按上游响应结构返回，**不透传上游**。

单渠道下这条理由反而更硬：全站用户共用一个上游账号，上游的列表接口会返回**该账号下所有 new-api 用户的素材**。直接透传等于把用户 A 的素材列表和 officialId 暴露给用户 B。本地表按 `user_id` 过滤是做数据隔离的唯一办法。

`page_num` / `page_size` / `groupId` / `status` 筛选参数语义与上游一致。可选 `?verbose=true` 时额外返回 `channel_id` 等内部字段，仅管理员可用。

### 3.5 批量上传的落库策略

上游 `POST /v1/assets/batch` 的响应**只按 index 返回 `officialId`**，不回显 `name` / `groupId` / `url`：

```json
{
  "batchId": "a1b2c3d4e5f6a7b8",
  "total": 2,
  "results": [
    { "index": 0, "status": "ok", "officialId": "asset-2026abc" },
    { "index": 1, "status": "error", "error": "invalid url" }
  ]
}
```

**`index` 即对应关系**——这一点对客户端和对 new-api 服务端的含义不同，需要区分：

- **对客户端**：`results[].index` 是提交数组的下标（上游示例中传 2 条返回 `index: 0` / `1`）。客户端手上有原始请求数组，按下标即可还原「哪个 officialId 对应哪个 url」。Excel 路径下 `index` 对应表格数据行序号，用户按行号对回。**下游不缺这个映射，无需 new-api 额外提供。**
- **对 new-api 服务端**：JSON 路径可以按 index 与请求体对齐；Excel 路径因不解析表格而无法对齐，但**回填并不依赖 index**——上游 `GET /v1/assets/{officialId}` 的响应本身就含 `imageUrl` / `name` / `region`（见 §2 A1 的响应示例），逐条查询即可拿到完整信息。

因此两条路径的落库方式不同：

| 路径 | 落库字段来源 | 缺失字段处理 |
|---|---|---|
| **JSON 数组**（`application/json`） | 按 `results[].index` 与请求体数组下标对齐，`name` / `groupId` / `url` 直接从请求体取 | 无缺失，一次写全 |
| **Excel 文件**（`multipart/form-data`） | **已确认不做解析**，new-api 只能从响应拿到 `officialId` + `batchId` | 先写入 `official_id` / `user_id` / `token_id` / `batch_id` / `status=Processing`，其余字段留空；再由**状态同步逻辑异步回填** |

**异步回填**：复用素材状态同步链路——前端轮询或用户主动刷新时，对 `name` 为空的记录调用上游 `GET /v1/assets/{officialId}`，把 `name` / `groupId` / `region` / 素材 URL / `status` 一次性补齐。上游文档明确该接口在 `status=Processing` 时会自动同步最新状态，正好与审核轮询是同一个动作，不额外增加请求。

> ⚠️ **两个待联调验证的假设**（M0，见 §12）：
>
> 1. **Excel 路径的 `index` 基准未知**：上游文档只给了 JSON 数组的例子。Excel 的 `index` 是从表头后的第一行数据算 0 还是 1、是否把空行计入，均未说明。这只影响用户按行号自查错误条目的体验（new-api 不依赖它），但需要在文档中向用户写清楚，故必须实测确认。
> 2. **视频 / 音频素材的 URL 字段名未知**：上游 A1 的响应示例是图片，字段为 `imageUrl`。视频 / 音频素材是复用 `imageUrl`，还是 `videoUrl` / `audioUrl`，或统一为 `url`，文档未给出。回填逻辑必须覆盖到，否则视频素材会回填出空 URL。**实现时按「依次尝试 `url` / `imageUrl` / `videoUrl` / `audioUrl`，取第一个非空」的兼容写法处理**，同时实测确认真实字段名。

**取舍说明**：不解析 Excel 意味着 new-api 无法在提交前校验表格里的 `groupId` 是否属于当前用户。这**不构成数据泄露**——`groupId` 只能写不能读，把素材传进他人的素材组后，该素材的归属记录在 new-api 本地仍然挂在上传者名下，他人也无法通过 `GET /v1/assets` 看到它。最坏情况是上游返回 `error` 或素材"传丢了"（进了别人的组、上传者自己在列表里能看到但语义混乱）。考虑到 Excel 批量上传是低频运维操作、且 `groupId` 需要用户手工从界面复制，判定为可接受风险，本期不做解析。

前端上传流程会把当前选中的素材组 `officialId` 显示在模板下载按钮旁，降低填错概率。

---

## 4. 渠道选择与素材归属（单渠道方案）

### 4.1 渠道选择：配置指定，不走 Distribute

素材接口请求体中没有 `model` 字段，而 `middleware.Distribute()`（`middleware/distributor.go:75`）在 `shouldSelectChannel=true` 时强制要求 `modelRequest.Model != ""`，否则直接 400。

**单渠道下无需引入任何选渠道逻辑**，也不复用 `Distribute()`（避免向请求体注入伪 `model` 字段污染上游请求）。改为一个 service 层辅助函数：

```go
// service/assets.go
// GetAssetsChannel 返回本期唯一的素材渠道。
func GetAssetsChannel() (*model.Channel, error) {
    // 1. 优先取系统设置 assets_channel_id
    if id := operation_setting.GetAssetsChannelId(); id > 0 {
        ch, err := model.CacheGetChannel(id)
        ...
        return ch, nil
    }
    // 2. 未配置时，自动取唯一一个启用的 Seedance 渠道
    //    （Type ∈ {ChannelTypeDoubaoVideo(54), ChannelTypeVolcEngine(45)}）
    //    若命中 0 个 → assets_channel_not_configured
    //    若命中 >1 个 → assets_channel_ambiguous，提示管理员显式配置 assets_channel_id
}
```

配置项 `assets_channel_id` 落在 `setting/operation_setting/`，后台「运营设置」中提供输入框。

校验时机：**启动时不校验**（渠道可能后建），首次调用素材接口时校验并给出明确错误码，避免影响服务启动。

### 4.2 素材归属：本地表隔离

上游账号没有 new-api 的用户概念，因此归属关系必须由 new-api 自己维护：

- **写入类**（`POST /v1/assets`、`/batch`、`/groups`）：上游成功返回后，把 `officialId → (user_id, token_id, group_official_id, region, status, ...)` 写入本地表。
- **读取 / 删除类**（`GET|DELETE /v1/assets/:id`）：先用 `official_id + user_id` 查本地表；**查不到直接返回 `404 asset_not_found`，不透传到上游**。这一条是防止用户遍历他人 officialId 的关键。
- **列表类**：见 §3.4，纯本地查询。

### 4.3 `asset://` 引用的处理

单渠道下，生成任务和素材必然落在同一个上游账号，**不需要锁渠道**（v1.0 的 `AssetChannelPin` 中间件本期取消）。

但仍建议保留一个**轻量校验中间件** `middleware.AssetRefCheck()`，挂在视频生成路由上（`router/video-router.go`，位于 `SeedanceRequestConvert()` 与 `TokenAuth()` 之后）：

1. 用 `common.UnmarshalBodyReusable` 读请求体，对整个请求体字符串做正则 `asset://([A-Za-z0-9\-_]+)` 提取（覆盖 `content[].image_url.url` / `video_url.url` / `audio_url.url` 以及 `metadata` 内同名字段）。
2. **无匹配 → 直接 `c.Next()`**，行为与改动前完全一致，对现有纯 URL 用法零影响。
3. 有匹配 → 批量查本地表（`WHERE official_id IN (...) AND user_id = ?`）并校验：

| 校验 | 失败响应 |
|---|---|
| 素材存在且属于当前用户 | `404 asset_not_found`（指出具体哪个 id） |
| `status = Active` | `400 asset_not_active` |
| 素材 region 与请求 `model` 所属区域一致 | `400 asset_region_mismatch` |

价值：把「引用了别人的素材」「引用了还没审核完的素材」「cn 素材配 intl 模型」这三类必然失败的请求在网关侧拦掉，用户拿到的是明确错误而不是上游的一句 `invalid asset`。这个中间件约 60 行，不涉及渠道逻辑，**建议本期做**。

区域映射表落到 `relay/channel/task/doubao/constants.go`，新增 `GetModelRegion(model string) string`：

| 模型 ID | region |
|---|---|
| `doubao-seedance-2-0-260128` / `-mini-260615` / `-fast-260128` | cn |
| `dreamina-seedance-2-0-260128` / `-mini-260615` / `-fast-260128` | intl |
| `ep-20260414121243-hp7w5` / `ep-20260414121306-pk5j6` | intl |

> **顺带修复**：现有 `doubao/constants.go` 的 `ModelList` 缺少 `doubao-seedance-2-0-mini-260615`、全部 `dreamina-*` 与两个大尺度 `ep-*` 模型。这些模型不在列表里则渠道无法承载对应模型的生成请求，`region=intl` 的素材上传后也没有模型可用。建议本期一并补齐。

---

## 5. 数据模型

### 5.1 `assets` 表（新增 `model/asset.go`）

```go
type Asset struct {
    Id              int64  `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
    OfficialId      string `json:"official_id" gorm:"type:varchar(191);uniqueIndex"`
    GroupOfficialId string `json:"group_official_id" gorm:"type:varchar(191);index"`
    UserId          int    `json:"user_id" gorm:"index"`
    ChannelId       int    `json:"channel_id" gorm:"index"` // 单渠道下恒为同一值，保留以便后续多渠道演进
    TokenId         int    `json:"token_id"`
    Region          string `json:"region" gorm:"type:varchar(10);index"`
    Name            string `json:"name" gorm:"type:varchar(191)"`
    AssetType       string `json:"asset_type" gorm:"type:varchar(20);index"` // Image/Video/Audio
    SourceUrl       string `json:"source_url" gorm:"type:text"`
    Status          string `json:"status" gorm:"type:varchar(20);index"`     // Processing/Active/Failed
    UpstreamId      int64  `json:"upstream_id"`                              // 上游数字 id
    UpstreamRaw     json.RawMessage `json:"-" gorm:"type:json"`              // 上游原始响应
    BatchId         string `json:"batch_id" gorm:"type:varchar(64);index"`
    CreatedAt       int64  `json:"created_at" gorm:"index"`
    UpdatedAt       int64  `json:"updated_at"`
    DeletedAt       int64  `json:"deleted_at" gorm:"index"`                  // 0 = 未删除
}
```

单渠道下 `official_id` 全局唯一，直接用 `uniqueIndex`。`channel_id` 字段保留但恒定，后续多渠道时改为复合唯一索引 `(channel_id, official_id)` 即可，属加法变更。

### 5.2 `asset_groups` 表

```go
type AssetGroup struct {
    Id          int64  `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
    OfficialId  string `json:"official_id" gorm:"type:varchar(191);uniqueIndex"`
    UserId      int    `json:"user_id" gorm:"index"`
    ChannelId   int    `json:"channel_id" gorm:"index"`
    Name        string `json:"name" gorm:"type:varchar(191)"`
    Description string `json:"description" gorm:"type:text"`
    Region      string `json:"region" gorm:"type:varchar(10);index"`
    AssetCount  int    `json:"asset_count" gorm:"-"` // 查询时聚合，不落库
    CreatedAt   int64  `json:"created_at" gorm:"index"`
    UpdatedAt   int64  `json:"updated_at"`
    DeletedAt   int64  `json:"deleted_at" gorm:"index"`
}
```

### 5.3 数据库兼容性约束（CLAUDE.md Rule 2）

- 表结构必须同时兼容 SQLite / MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6；
- JSON 字段用 `type:json`（GORM 在 SQLite 上退化为 TEXT），**禁止**使用 PG 专属 `JSONB` 操作符；
- 软删除用 `deleted_at int64`（0 表示未删除），不用 `gorm.DeletedAt`，与仓库现有风格一致；
- 迁移在 `model/main.go` 的 `AutoMigrate` 列表（`model/main.go:258`）追加两个新模型，无需 `ALTER COLUMN`。

### 5.4 JSON 规范（CLAUDE.md Rule 1）

所有序列化 / 反序列化必须走 `common.Marshal` / `common.Unmarshal` / `common.UnmarshalBodyReusable`，禁止直接调用 `encoding/json` 的 marshal 系列函数。

---

## 6. 计费、鉴权与限流

### 6.1 计费：完全免费

素材接口**不计费、不写 `logs` 表**（上游侧素材上传本身不收费，仅生成任务收费）。

因此这批接口：

- 不进入 `service.PreConsumeBilling` / `service.PostConsumeQuota` 计费链路；
- 不生成 `model.Log` 记录；
- 但**保留**用户状态校验：用户被禁用或额度耗尽时拒绝上传（返回 `403 insufficient_quota`），防止已欠费用户继续占用上游素材配额。

### 6.2 限流

新增 `middleware.AssetsRateLimit()`，配置项挂在 `setting/operation_setting/`：

| 配置项 | 默认值 | 说明 |
|---|---|---|
| `assets_channel_id` | 0 | 素材渠道 ID，0 = 自动探测唯一 Seedance 渠道 |
| `assets_rate_limit_count` | 60 | 每用户每分钟素材接口调用次数 |
| `assets_batch_max_items` | 50 | 单次批量上传上限（与上游一致） |
| `assets_user_max_total` | 0 | 每用户素材总数上限（0 = 不限） |
| `assets_upload_max_file_mb` | 10 | Excel 批量上传文件大小上限 |

> 已与上游确认：**单账号的素材总数与存储容量没有配额上限**。因此 `assets_user_max_total` 默认设为 0（不限），仅作为出现异常刷量时的应急开关保留。滥用防线主要落在 `assets_rate_limit_count`（频次）与 `assets_batch_max_items`（单批条数）上——上游没有硬上限不等于可以无限制刷，异步审核队列与本地表膨胀仍是实际成本。

### 6.3 鉴权与数据隔离

- 所有接口走 `middleware.TokenAuth()`；
- 所有本地查询强制带 `user_id` 条件；
- 管理员（`common.RoleAdminUser`）通过 `/api/assets/*` 后台接口可查看全部素材（供前端管理页与排障使用），`/v1/assets*` 始终按 user_id 隔离。

### 6.4 SSRF 防护

`POST /v1/assets` 的 `url` 由用户提供并由**上游**去拉取，new-api 不主动请求该 URL，因此不直接构成 SSRF。仍需基础校验：仅允许 `http` / `https` scheme，拒绝 `file://` / `data://` 等，复用 `common/ssrf_protection.go` 中的既有工具函数。

---

## 7. 前端：素材库管理页面

### 7.1 页面结构

新增 `web/src/pages/Assets/`，在 `web/src/App.jsx` 注册路由 `/assets`，侧边栏（`web/src/components/layout/SiderBar.jsx`）放在「任务」附近。

**Tab 1：素材列表**

| 列 | 说明 |
|---|---|
| 预览 | 图片缩略图；视频 / 音频显示类型图标 |
| 名称 | `name` |
| 类型 | Image / Video / Audio 标签 |
| 素材组 | 素材组名称（点击可筛选） |
| 区域 | cn / intl 标签 |
| 状态 | Processing（蓝，带轮询动效）/ Active（绿）/ Failed（红，hover 显示原因） |
| 引用 | `asset://asset-2026abc`，一键复制 |
| 创建时间 | 相对时间 |
| 操作 | 刷新状态 / 删除 |

功能点：

- 顶部筛选：素材组、状态、类型、关键词；
- **上传弹窗**：素材组（下拉，必选）+ 素材 URL（必填）+ 名称（选填）；支持多行 URL 一次提交（走 `/v1/assets/batch`）；
- **Excel 批量上传**：下载模板按钮 → 上传 xlsx → 展示逐条结果（`results[].status` = ok / error）。模板下载按钮旁展示当前选中素材组的 `officialId` 并支持一键复制，降低用户在表格里填错 `groupId` 的概率（§3.5）；成功记录入列表时 `name` / 素材组列先显示为"同步中"，由回填逻辑补齐；
- **状态轮询**：列表中存在 `Processing` 记录时每 15s 自动刷新，全部结束后停止；
- 删除二次确认。

**Tab 2：素材组管理**

- 列表：名称、描述、region、素材数量、创建时间；
- 创建弹窗：名称 + 描述 + region 单选，**明确提示：创建后不可修改；国际版 / 大尺度模型必须使用 intl**。

**未配置渠道时的空态**：`assets_channel_not_configured` / `assets_channel_ambiguous` 时，页面展示引导文案（管理员可见「去配置」链接指向运营设置），而不是一个报错 toast。

### 7.2 与 Playground 的联动（可选增强）

在视频生成 Playground 的参考素材输入框旁增加「从素材库选择」按钮，选中后自动填入 `asset://` 引用。排期紧张可延后至下一迭代。

### 7.3 前端规范

- 使用 Semi Design 组件与既有 `web/src/components/table/` 抽象保持一致；
- 所有文案走 i18n，`bun run i18n:extract` 后补齐 `web/src/i18n/locales/` 全部语言；
- 包管理与脚本统一用 `bun`（CLAUDE.md Rule 3）。

---

## 8. 改动清单

### 8.1 新增文件

| 文件 | 内容 |
|---|---|
| `router/assets-router.go` | 素材路由注册（§3.2） |
| `controller/assets.go` | `RelayAssetCreate` / `RelayAssetList` / `RelayAssetDispatch` 等控制器 |
| `middleware/asset_ref_check.go` | `AssetRefCheck()`：生成任务中 `asset://` 的归属 / 状态 / region 校验 |
| `middleware/assets_rate_limit.go` | `AssetsRateLimit()` |
| `model/asset.go` | `Asset` / `AssetGroup` 模型与 DAO |
| `service/assets.go` | `GetAssetsChannel()`、上游透传客户端、状态同步、归属校验 |
| `dto/asset.go` | 请求 / 响应 DTO（可选标量字段用指针类型，遵循 CLAUDE.md Rule 6） |
| `web/src/pages/Assets/*` | 前端页面 |
| `docs/seedance-assets-api.md` | 面向用户的接口文档 |

> 相比 v1.0，取消了 `middleware/assets_distribute.go`（渠道选择）与 `setting/model_setting/seedance.go`（region→模型映射）。

### 8.2 修改文件

| 文件 | 改动 |
|---|---|
| `main.go` | 注册 `router.SetAssetsRouter` |
| `model/main.go` | `AutoMigrate` 追加 `Asset` / `AssetGroup` |
| `router/video-router.go` | 视频生成路由链插入 `middleware.AssetRefCheck()` |
| `relay/channel/task/doubao/constants.go` | 补齐 mini / dreamina / ep-* 模型；新增 `GetModelRegion` |
| `setting/operation_setting/` | 新增 §6.2 的 5 个配置项 |
| `web/src/App.jsx` | 注册 `/assets` 路由 |
| `web/src/components/layout/SiderBar.jsx` | 侧边栏菜单项 |
| `web/src/i18n/locales/*` | 新增文案 |

---

## 9. 验收标准

### 9.1 功能

- [ ] 用 new-api token 调用全部素材接口，请求 / 响应结构与上游文档逐字段一致；
- [ ] 创建 `region=cn` 与 `region=intl` 两个素材组，分别上传素材，`officialId` 正确落库；
- [ ] 素材 `status` 从 `Processing` 变为 `Active` 后，本地表状态同步更新；
- [ ] **用户 A 无法通过 `GET /v1/assets` 看到用户 B 的素材**；
- [ ] **用户 A 用用户 B 的 officialId 调 `GET|DELETE /v1/assets/{id}` 返回 404，且抓包确认请求未打到上游**；
- [ ] 生成任务引用未审核通过的素材 → `asset_not_active`；引用他人素材 → `asset_not_found`；`region=cn` 素材配 `dreamina-*` 模型 → `asset_region_mismatch`；三者均不打到上游；
- [ ] **不含 `asset://` 的生成请求行为与改动前完全一致（回归）**；
- [ ] 批量上传 50 条 JSON 与 Excel 两种方式均成功，部分失败时返回逐条结果；
- [ ] **Excel 批量上传的记录先以空 `name` 落库，一次状态刷新后 `name` / `groupId` / `region` 被正确回填**（§3.5）；
- [ ] 未配置素材渠道时返回 `assets_channel_not_configured`，前端展示引导空态；配置了 2 个及以上 Seedance 渠道且未显式指定时返回 `assets_channel_ambiguous`；
- [ ] 前端完成上传、筛选、轮询、复制引用、删除、素材组创建全流程。

### 9.2 非功能

- [ ] `go test ./...` 全绿；新增 `model/asset_test.go`、`middleware/asset_ref_check_test.go`、路由注册单测（验证 §3.2 不 panic）；
- [ ] SQLite / MySQL / PostgreSQL 三库分别启动并跑通迁移与全部接口；
- [ ] 素材接口不产生 `logs` 记录、不扣减额度（对账验证）；
- [ ] 所有 JSON 操作走 `common.*` 包装（`grep -rn "json.Marshal\|json.Unmarshal" controller/assets.go service/assets.go` 应无匹配）；
- [ ] 前端 `bun run lint` 与 `bun run i18n:lint` 通过。

---

## 10. 多渠道演进路径（本期不做，仅记录）

本期方案已为多渠道预留了扩展点，后续开启时的增量改动：

1. `assets` / `asset_groups` 的 `channel_id` 字段已存在 → 唯一索引由 `official_id` 改为 `(channel_id, official_id)`；
2. `service.GetAssetsChannel()` 的「唯一渠道」假设改为按 region 选渠道（复用 `service.CacheGetRandomSatisfiedChannel` + 虚拟模型名）；
3. `middleware.AssetRefCheck()` 扩展为 `AssetChannelPin()`：校验通过后额外设置 `common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(channelId))`，`Distribute()`（`middleware/distributor.go:33`）会读取该 key 走指定渠道分支，与 `middleware/auth.go:437` 中 token key 携带 `-channel_id` 后缀的既有机制同一条通路，无需改动 `Distribute()`；同时新增 `asset_channel_conflict` 错误码（一个请求引用了跨渠道的素材）。

以上三点均为加法变更，不需要重构本期代码或做数据迁移。

---

## 11. 风险与对策

| # | 风险 | 影响 | 对策 |
|---|---|---|---|
| R1 | gin v1.9.1 同层级 static / wildcard 路由冲突 panic | 服务启动失败 | §3.2 catch-all 方案；M0 第一项写单测验证 |
| R2 | **单点依赖**：唯一素材渠道被删除 / 禁用 / 上游账号欠费 | 素材功能整体不可用，且已有 `asset://` 全部失效 | 删除或禁用该渠道时在后台强提示关联素材数量；素材接口在渠道不可用时返回明确错误码而非静默失败；文档中说明该渠道为单点 |
| R3 | 用户绕过 new-api，直接在上游控制台创建素材组 / 素材 | 这些记录不会出现在 new-api 列表中，用户困惑 | **不做导入同步功能**（已确认线上无存量素材）。改为运营约定 + 文档说明：素材必须通过 new-api 创建。前端空态与文档中明确提示这一点 |
| R3b | Excel 批量上传的 `groupId` 未做校验，可能填入他人素材组 | 素材进入他人组，语义混乱；**不构成数据泄露** | 已确认本期不解析 Excel（§3.5）。前端展示当前素材组 officialId 供复制以降低出错率 |
| R4 | 免费策略被滥用，单个用户高频刷量 | 上游审核队列排队、本地表膨胀；因上游无配额上限，**不会导致其他用户完全不可用** | §6.2 以频次限流（`assets_rate_limit_count`）+ 单批条数为主要防线；`assets_user_max_total` 默认不限，作为应急开关保留。风险等级由「高」下调为「中」 |
| R5 | `asset://` 正则扫描漏掉嵌套结构 | 校验被绕过，退化为上游报错 | 对整个请求体字符串做正则扫描（而非按已知字段路径遍历），宁可多匹配不可漏匹配 |
| R6 | 上游素材接口变更 | 透传失败 | 响应用 `json.RawMessage` 原样透传，仅提取落库必需的少量字段，降低耦合 |

---

## 12. 里程碑

| 阶段 | 内容 | 预估 |
|---|---|---|
| M0 | 路由方案验证（R1）+ 上游联调，**联调 checklist 见下** | 0.5 天 |
| M1 | 数据模型 + 迁移 + DAO | 1 天 |
| M2 | 接口透传 + `GetAssetsChannel` + 归属隔离 + 批量落库与异步回填（§3.5） | 1.5 天 |
| M3 | `AssetRefCheck` + region 映射 + doubao ModelList 补齐 | 0.5 天 |
| M4 | 前端素材库页面 + i18n | 2 天 |
| M5 | 三库兼容测试 + 回归 + 文档 | 1 天 |
| | **合计** | **约 6.5 人日** |

砍掉导入同步功能省下的时间，抵消了 §3.5 异步回填新增的工作量，总量不变。

### M0 上游联调 checklist

以下均为当前文档中**基于推断而非上游明文**的假设，动手写实现前必须实测确认，避免返工：

- [ ] gin v1.9.1 下 §3.2 的路由注册方案不 panic（决定控制器组织形式）；
- [ ] `GET /v1/assets/{officialId}` 对**视频 / 音频**素材返回的 URL 字段名（`url` / `imageUrl` / `videoUrl` / `audioUrl`？）——直接决定 §3.5 回填逻辑；
- [ ] Excel 批量上传时 `results[].index` 的基准（是否含表头、0-based 还是 1-based、空行是否计入）；
- [ ] Excel 模板的实际列结构（是否含 `groupId` 列、列名与顺序）——虽不解析，但前端要向用户说明怎么填；
- [ ] 批量接口部分失败时，`results[].error` 的错误文案是否可直接透传给终端用户（有无内部信息泄露）；
- [ ] 素材软删除后，上游 `GET /v1/assets/{officialId}` 的返回行为（404？还是返回带删除标记的记录）——影响回填逻辑对已删除素材的容错。

---

## 13. 已确认事项（原待确认）

本 PRD 的开放问题已全部关闭，可直接进入开发：

| # | 问题 | 结论 | 影响章节 |
|---|---|---|---|
| 1 | Excel 模板里的 `groupId` 是否需要 new-api 侧解析校验？ | **不需要**，直接透传不解析。连带影响：Excel 路径落库时 `name` / `groupId` 等字段留空，由状态同步逻辑异步回填 | §3.5、§7.1、R3b |
| 2 | 上游对单账号的素材总数 / 存储容量是否有配额上限？ | **无上限**。`assets_user_max_total` 默认设为 0（不限），仅作应急开关；R4 风险等级由高下调为中 | §6.2、R4 |
| 3 | 是否需要把上游控制台已有的素材一次性导入 new-api？ | **不需要**，线上无存量素材。同步 / 导入功能整体砍掉，改为运营约定「素材必须通过 new-api 创建」 | R3、§7.1 |
