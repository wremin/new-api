# usage_ratio 按渠道独立配置 — 设计需求文档

## 1. 背景与现状

### 1.1 当前状态

`usage_ratio` 目前是一个**全局运营设置**，对所有渠道统一生效。

**存储：**
- `model/setting/operation_setting/quota_usage_ratio_setting.go`
- 配置结构 `QuotaUsageRatioSetting{UsageRatio float64}`，默认 1.0
- 注册在全局配置管理器，通过后台运营设置页面修改

**读取：**
- `operation_setting.GetQuotaUsageRatio()` — 读取全局值

**生效位置（共3处）：**

| 位置 | 文件 | 说明 |
|------|------|------|
| 计费入口 | `service/quota.go:L273` — `PostAudioConsumeQuota()` | 计费落地前修改 Usage |
| Gemini 响应 | `relay/channel/gemini/relay-gemini.go:L1060` — `applyUsageRatioToGeminiResponse()` | 修改返回给客户端的 Gemini 原生响应 |
| OpenAI 响应 | `relay/channel/openai/relay-openai.go:L591` — `OpenaiHandlerWithUsage()` | 修改返回给客户端的 OpenAI 兼容响应 |

**核心计算函数：**
- `service/usage_helpr.go` — `ApplyUsageRatioToUsage(usage *dto.Usage, completionRatio float64)`
- 策略：PromptTokens 不变，通过调整 CompletionTokens 体现倍率

```
公式: 新CompletionTokens = [(PromptTokens + CompletionTokens × CompletionRatio) × usageRatio - PromptTokens] / CompletionRatio
```

### 1.2 痛点

| 痛点 | 说明 |
|------|------|
| 一刀切 | 所有渠道使用同一个倍率，无法差异化定价 |
| 商务不灵活 | 无法为不同供应商（渠道商）设置不同的成本系数 |
| 运维风险 | 修改全局 ratio 会影响所有渠道，调一个要全部跟着动 |

### 1.3 目标

支持**按渠道独立设置** `usage_ratio`，优先级：
```
渠道级 > 全局级（默认 1.0）
```

---

## 2. 方案设计

### 2.1 数据模型变更

#### 方案选择：数据库新增列（推荐）

| 方案 | 优点 | 缺点 |
|------|------|------|
| **A) `channels` 表新增 `usage_ratio` 列** | 结构化、可索引、查询快 | 需要 DDL 迁移 |
| B) 存入 `settings` JSON 字段 | 无需 DDL | 非结构化、无法索引、反模式 |
| C) 存入 `ChannelOtherSettings` | 已有 JSON 字段 | 和渠道运营设置混在一起，语义不清 |

**选择方案 A**，新增列：

```sql
-- channels 表新增字段
ALTER TABLE channels ADD COLUMN usage_ratio DOUBLE PRECISION NOT NULL DEFAULT 1.0;
-- MySQL 写法：ALTER TABLE channels ADD COLUMN usage_ratio DOUBLE NOT NULL DEFAULT 1.0;
-- SQLite 写法：ALTER TABLE channels ADD COLUMN usage_ratio REAL NOT NULL DEFAULT 1.0;
```

#### Go Model 变更

```go
// model/channel.go — Channel struct 新增字段
type Channel struct {
    // ... 现有字段 ...
    UsageRatio float64 `json:"usage_ratio" gorm:"default:1.0"`  // ← 新增
}
```

#### 语义约定

| 值 | 含义 |
|----|------|
| `1.0` | 默认值，不调整（走全局 ratio） |
| `> 1.0` | 放大，该渠道消耗按倍数计算 |
| `< 1.0` | 缩小，该渠道消耗按折扣计算 |
| 任何非 1.0 的值 | **覆盖**全局 usage_ratio |

---

### 2.2 计算逻辑变更

#### 当前调用链
```
PostConsumeQuota()
  └── operation_setting.GetQuotaUsageRatio()   ← 读全局
  └── ApplyUsageRatioToUsage(usage, completionRatio)
```

#### 新调用链
```
PostConsumeQuota(info)                          ← info 中携带 channel usage_ratio
  └── getEffectiveUsageRatio(info)              ← 新函数，返回最终 ratio
      ├── if info.ChannelRatio != nil && *info.ChannelRatio != 1.0 → return *info.ChannelRatio
      └── return operation_setting.GetQuotaUsageRatio()             → 兜底全局
  └── ApplyUsageRatioToUsage(usage, completionRatio, usageRatio)   ← 传入显式 ratio
```

#### 核心函数修改

```go
// service/usage_helpr.go — 增加 usageRatio 参数
// 旧: func ApplyUsageRatioToUsage(usage *dto.Usage, completionRatio float64)
// 新: func ApplyUsageRatioToUsage(usage *dto.Usage, completionRatio float64, usageRatio float64)
func ApplyUsageRatioToUsage(usage *dto.Usage, completionRatio float64, usageRatio float64) {
    if usage == nil || usageRatio == 1.0 {
        return
    }
    // ... 计算逻辑不变，但用参数传入的 usageRatio ...
}
```

#### 优先级函数

```go
// service/usage_helpr.go — 新增
func GetEffectiveUsageRatio(channelUsageRatio *float64) float64 {
    if channelUsageRatio != nil && *channelUsageRatio != 1.0 {
        return *channelUsageRatio
    }
    return operation_setting.GetQuotaUsageRatio()
}
```

---

### 2.3 数据流转链路

```
┌──────────────┐    ┌──────────────┐    ┌─────────────────┐    ┌──────────────┐
│ channels 表   │───►│ model.Channel│───►│ relayInfo        │───►│ ApplyUsage   │
│ usage_ratio   │    │ .UsageRatio  │    │ .ChannelRatio    │    │ RatioToUsage │
│ (DB)          │    │ (Go struct)  │    │ (承载到计费层)    │    │ (最终生效)    │
└──────────────┘    └──────────────┘    └─────────────────┘    └──────────────┘
```

#### 3个生效位置的修改：

| 位置 | 如何获取渠道 ratio | 修改点 |
|------|-------------------|--------|
| `PostAudioConsumeQuota` | `relayInfo.PriceData.ChannelRatio` | 从 `relayInfo` 读出传给 `ApplyUsageRatioToUsage` |
| `applyUsageRatioToGeminiResponse` | 已有 `info.UpstreamModelName`，需增加 `info` 访问 | 从 `relayInfo` 传入 `info.PriceData.ChannelRatio` |
| `OpenaiHandlerWithUsage` | 已有 `info.PriceData.CompletionRatio` | 从 `info.PriceData.ChannelRatio` 读取 |

---

### 2.4 RelayInfo 承载

`relayInfo.PriceData` 中新增字段：

```go
// 在 PriceData struct 中新增
type PriceData struct {
    // ... 现有字段 ...
    ChannelUsageRatio *float64  `json:"channel_usage_ratio,omitempty"`  // ← 新增
}
```

赋值时机：渠道分发时（`middleware/distributor.go` 或 `service/channel.go` 的渠道加载逻辑中）：

```go
if channel.UsageRatio != 1.0 {
    ratio := channel.UsageRatio
    relayInfo.PriceData.ChannelUsageRatio = &ratio
}
```

---

### 2.5 API 变更

#### 渠道管理 API

| 方法 | 路径 | 变更 |
|------|------|------|
| `POST` | `/api/channel/` | 请求体增加 `usage_ratio` 字段 |
| `PUT` | `/api/channel/` | 请求体增加 `usage_ratio` 字段 |
| `GET` | `/api/channel/` | 响应体增加 `usage_ratio` 字段 |
| `GET` | `/api/channel/:id` | 响应体增加 `usage_ratio` 字段 |

**不需要新增 API**，在已有渠道 CRUD 中增加字段即可。

---

### 2.6 前端变更

#### 渠道编辑表单

在 `web/src/pages/Channel/` 的渠道编辑弹窗中新增：

```
┌─────────────────────────────────┐
│  用户使用量比例                   │
│  ┌─────────────────┐            │
│  │  1.0            │  ±0.01步长  │
│  └─────────────────┘            │
│  ℹ️ 调整该渠道的令牌消耗计算比例。   │
│    留空或 1.0 使用全局设置。       │
│    大于1表示消耗放大，小于1表示折扣。 │
└─────────────────────────────────┘
```

- 默认值：1.0
- 范围：0.01 ~ 10
- 步长：0.01
- 提示：留空或 1.0 使用全局设置

#### 渠道列表

渠道列表表格增加一列显示 `usage_ratio`（当值 ≠ 1.0 时高亮）。

#### 全局运营设置

保持不变（保留作为兜底）。在 `SettingsCreditLimit.jsx` 的全局 usage_ratio 旁增加说明：
> "该值仅对未单独设置 usage_ratio 的渠道生效。"

---

### 2.7 数据库迁移

```sql
-- PostgreSQL
ALTER TABLE channels ADD COLUMN usage_ratio DOUBLE PRECISION NOT NULL DEFAULT 1.0;

-- MySQL
ALTER TABLE channels ADD COLUMN usage_ratio DOUBLE NOT NULL DEFAULT 1.0;

-- SQLite
ALTER TABLE channels ADD COLUMN usage_ratio REAL NOT NULL DEFAULT 1.0;
```

GORM AutoMigrate 会自动处理，但建议在 `model/main.go` 中增加迁移逻辑以兼容 SQLite（SQLite 不支持 ALTER COLUMN）：

```go
// model/main.go — init 或 migration 段
if !DB.Migrator().HasColumn(&Channel{}, "usage_ratio") {
    DB.Migrator().AddColumn(&Channel{}, "usage_ratio")
}
```

---

## 3. 实施任务清单

| # | 模块 | 任务 | 涉及文件 |
|---|------|------|----------|
| 1 | **Model** | `Channel` struct 增加 `UsageRatio` 字段 | `model/channel.go` |
| 2 | **Model** | 数据库迁移逻辑（兼容三库） | `model/main.go` |
| 3 | **DTO** | `PriceData` 增加 `ChannelUsageRatio` 字段 | `relay/common/relay_info.go` |
| 4 | **Service** | `ApplyUsageRatioToUsage()` 增加 `usageRatio` 参数 | `service/usage_helpr.go` |
| 5 | **Service** | 新增 `GetEffectiveUsageRatio()` 函数 | `service/usage_helpr.go` |
| 6 | **Service** | `PostAudioConsumeQuota()` 改用渠道 ratio | `service/quota.go` |
| 7 | **Service** | 渠道加载时将 `UsageRatio` 写入 `relayInfo` | `service/channel.go` 或中间件 |
| 8 | **Gemini** | `applyUsageRatioToGeminiResponse()` 传入渠道 ratio | `relay/channel/gemini/relay-gemini.go` |
| 9 | **OpenAI** | `OpenaiHandlerWithUsage()` 传入渠道 ratio | `relay/channel/openai/relay-openai.go` |
| 10 | **Controller** | 渠道 CRUD 增加 `usage_ratio` 字段透传 | `controller/channel.go` |
| 11 | **Frontend** | 渠道编辑表单增加 `usage_ratio` 输入 | `web/src/pages/Channel/` |
| 12 | **Frontend** | 全局运营设置增加说明文字 | `web/src/pages/Setting/Operation/SettingsCreditLimit.jsx` |
| 13 | **Compile** | 全量编译验证 | `go build ./...` |
| 14 | **Test** | 多渠道、多 ratio 计费一致性测试 | 手动测试 |

---

## 4. 兼容性保障

1. **存量数据** — 默认值 1.0，已有渠道行为不变
2. **全局设置** — 保留，作为未单独设置渠道的兜底
3. **API 兼容** — 新增字段，旧客户端不传则默认 1.0
4. **计费一致性** — 同一套计算逻辑，只是 ratio 来源不同

---

## 5. 验收标准

1. 渠道 A 设置 `usage_ratio=2.0`，渠道 B 保持默认，两者计费结果独立
2. 全局 usage_ratio 修改只影响未单独设置的渠道
3. 客户端收到的 tokens 响应值 = 计费日志中的 tokens 值
4. 前端渠道编辑页面可正常设置和查看 usage_ratio
5. 三个数据库（SQLite/MySQL/PostgreSQL）迁移无报错
