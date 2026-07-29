# Seedance 2.0 素材库 — 部署说明

| 项目 | 内容 |
|---|---|
| 版本 | v1.0 |
| 日期 | 2026-07-28 |
| 对应 PRD | `docs/seedance-assets-prd.md`（v2.2 单渠道版） |
| 影响面 | 新增 `/v1/assets*` 接口、两张数据表、一个控制台页面；对现有生成任务链路为**加法变更** |

---

## 0. 升级前必读

**本次改动没有经过编译验证。** 开发环境无法访问 Go 模块代理，代码只做了 `gofmt` 解析与逐符号人工比对。**必须先在本地跑通构建与测试再部署**：

```bash
cd /path/to/new-api
go build ./...
go test ./router/ ./middleware/
```

`go test ./router/` 是 PRD 中 M0 的第一项验证——确认 gin 路由树不会因 `/v1/assets` 下的通配段而 panic。**这条测试不通过就不要部署**，服务会在启动时直接崩溃。

### 兼容性与风险等级

| 项目 | 结论 |
|---|---|
| 现有生成任务（不含 `asset://`） | **零影响**。`AssetRefCheck` 中间件在请求体中匹配不到 `asset://` 时立即放行 |
| 现有 API 契约 | 无变更，只新增路径 |
| 数据库 | 只新增两张表，不修改任何现有表结构 |
| 回滚 | 见 §7，回滚安全（新表可保留） |
| 计费 | 素材接口不计费、不写 `logs` 表，不影响任何对账逻辑 |

---

## 1. 变更清单

### 新增文件（16）

```
constant/seedance.go                          模型 → 区域（cn/intl）映射
model/asset.go                                Asset / AssetGroup 模型与 DAO
dto/asset.go                                  请求 / 响应 DTO
service/assets.go                             渠道解析、上游透传客户端、状态同步
controller/assets.go                          8 个接口的控制器
middleware/asset_ref_check.go                 asset:// 引用校验
middleware/assets_rate_limit.go               素材接口限流
middleware/asset_ref_check_test.go            单测
router/assets-router.go                       路由注册
router/assets_router_test.go                  路由 panic 验证（M0）
setting/operation_setting/assets_setting.go   配置项
web/src/services/assets.js                    前端 API 封装
web/src/hooks/assets/                         前端数据 hooks（2 个文件）
web/src/components/table/assets/              前端组件（11 个文件）
web/src/pages/Assets/index.jsx                页面入口
docs/seedance-assets-prd.md                   PRD
```

### 修改文件（15）

```
model/main.go                                 AutoMigrate 增加 Asset / AssetGroup（两处列表）
router/main.go                                注册 SetAssetsRouter
router/video-router.go                        视频生成链路插入 AssetRefCheck
relay/channel/task/doubao/constants.go        补齐 mini / dreamina / ep-* 模型；新增 GetModelRegion
web/src/App.jsx                               /console/assets 路由
web/src/components/layout/SiderBar.jsx        侧边栏「素材库」
web/src/hooks/common/useSidebar.js            DEFAULT_ADMIN_CONFIG.console.assets
web/src/helpers/render.jsx                    getLucideIcon 增加 assets 图标
web/src/i18n/locales/*.json                   7 个语言各新增 55 个 key
```

---

## 2. 构建

### 方式一：Docker（推荐，与仓库现有流程一致）

`Dockerfile` 未改动，前端与后端都在镜像内构建，无需本地装 Go / Bun：

```bash
cd /opt/new-api          # 或你的部署目录
git pull
docker compose build
docker compose up -d
docker compose logs -f new-api
```

镜像内已配置 `GOPROXY=https://goproxy.cn,direct`，构建阶段会自行拉取依赖。

### 方式二：本地二进制

前端必须先构建，Go 才能把 `web/dist` 嵌进二进制：

```bash
cd web
bun install
DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION=$(cat ../VERSION) bun run build
cd ..
go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api
```

前端首次构建前建议跑一次格式化，本次前端代码未经 eslint 校验：

```bash
cd web && bun run lint:fix && bun run i18n:lint
```

---

## 3. 数据库迁移

**自动执行，无需手工 SQL。** 服务启动时 `model/main.go` 的 `AutoMigrate` 会创建两张新表：

| 表 | 用途 | 关键索引 |
|---|---|---|
| `assets` | 素材归属记录 | `official_id` 唯一索引；`user_id` / `channel_id` / `status` / `group_official_id` / `batch_id` 普通索引 |
| `asset_groups` | 素材组归属记录 | `official_id` 唯一索引；`user_id` / `channel_id` / `region` 普通索引 |

注意事项：

- 迁移**只在主节点执行**（`model/main.go` 里 `if !common.IsMasterNode { return nil }`）。多节点部署时先升级主节点，确认表已建好，再滚动升级从节点。
- 三种数据库（SQLite / MySQL ≥ 5.7.8 / PostgreSQL ≥ 9.6）均通过 GORM 抽象建表，无库特有语法，不需要分库处理。
- 只有 `CREATE TABLE`，没有 `ALTER`，对现有数据零风险。

验证表已创建：

```sql
-- PostgreSQL
\d assets
\d asset_groups

-- MySQL
SHOW CREATE TABLE assets;

-- SQLite
.schema assets
```

---

## 4. 配置素材渠道

素材接口需要一个指向 seegen.ai 的渠道。**本期为单渠道模式**：全站素材共用一个上游账号。

### 4.1 创建/确认渠道

在控制台「渠道管理」新建或复用一个渠道：

| 字段 | 值 |
|---|---|
| 类型 | `DoubaoVideo`（类型 ID `54`）或 `VolcEngine`（`45`） |
| 代理地址 / Base URL | `https://api.seegen.ai` |
| 密钥 | 在 seegen.ai「API Keys」页面创建的 Key（`Bearer` 用） |
| 模型 | `doubao-seedance-2-0-260128`、`doubao-seedance-2-0-mini-260615`、`doubao-seedance-2-0-fast-260128`、`dreamina-seedance-2-0-260128`、`dreamina-seedance-2-0-mini-260615`、`dreamina-seedance-2-0-fast-260128`、`ep-20260414121243-hp7w5`、`ep-20260414121306-pk5j6` |
| 状态 | 启用 |

> 模型列表只影响**生成任务**的渠道路由；素材接口本身不按模型选渠道。但 `region=intl` 的素材上传后需要有 intl 模型可用，所以建议一次配全。

### 4.2 指定素材渠道

`service.GetAssetsChannel()` 的解析顺序：

1. 配置项 `assets_setting.channel_id` > 0 → 直接用该渠道；
2. 否则自动探测**唯一一个**启用的 Seedance 渠道（类型 54 或 45）：
   - 命中 0 个 → `assets_channel_not_configured`
   - 命中 1 个 → 使用它
   - 命中 ≥2 个 → `assets_channel_ambiguous`，拒绝服务

**如果你的部署里已经存在其他 VolcEngine(45) 或 DoubaoVideo(54) 渠道（很常见），自动探测一定会报 ambiguous，必须显式指定。**

配置项目前**没有前端表单**（`setting/operation_setting` 的配置需要在设置页写死字段才会出现）。通过 root 账号调用通用配置接口写入：

```bash
# 替换 <SITE>、<ROOT_SESSION_COOKIE>、<CHANNEL_ID>
curl -X PUT https://<SITE>/api/option/ \
  -H "Content-Type: application/json" \
  -H "Cookie: <ROOT_SESSION_COOKIE>" \
  -d '{"key":"assets_setting.channel_id","value":"7"}'
```

该接口走 `middleware.RootAuth()`，只有 root 用户可调用。写入后会同时更新数据库 `options` 表与内存配置，**无需重启**；从节点通过 `SyncOptions` 周期性同步。

也可以直接写库（需重启或等待同步周期）：

```sql
INSERT INTO options (key, value) VALUES ('assets_setting.channel_id', '7')
ON CONFLICT (key) DO UPDATE SET value = '7';   -- PostgreSQL
```

> 在浏览器已登录 root 的情况下，最省事的方式是打开控制台任意页面，在开发者工具 Console 里执行：
> ```js
> await fetch('/api/option/', {method:'PUT', headers:{'Content-Type':'application/json'},
>   body: JSON.stringify({key:'assets_setting.channel_id', value:'7'})}).then(r=>r.json())
> ```

### 4.3 全部配置项

| Key | 默认值 | 说明 |
|---|---|---|
| `assets_setting.channel_id` | `0` | 素材渠道 ID，0 = 自动探测唯一 Seedance 渠道 |
| `assets_setting.rate_limit_count` | `60` | 每用户每分钟素材接口调用上限，0 = 不限 |
| `assets_setting.batch_max_items` | `50` | 单次批量上传条数上限（代码内强制 ≤ 50，与上游一致） |
| `assets_setting.user_max_total` | `0` | 每用户素材总数上限，0 = 不限 |
| `assets_setting.upload_max_file_mb` | `10` | Excel 批量上传文件大小上限（MB） |

上游已确认单账号素材总数与存储容量**无配额上限**，因此 `user_max_total` 默认放开，只作为异常刷量时的应急开关。滥用防线主要靠 `rate_limit_count`。

限流是内存计数器（配置了 Redis 时走 Redis），多节点部署下未配 Redis 时限流是**每节点独立**的，实际上限 ≈ 配置值 × 节点数。

---

## 5. 冒烟验证

准备一个普通用户的 token（`sk-...`）和站点地址：

```bash
export SITE=https://your-new-api.com
export TOKEN=sk-xxxxxxxx
```

### 5.1 渠道配置是否生效

```bash
curl -s "$SITE/v1/assets/groups" -H "Authorization: Bearer $TOKEN"
```

- 返回 `[]` 或素材组数组 → 渠道解析正常；
- 返回 `assets_channel_not_configured` / `assets_channel_ambiguous` → 回到 §4.2。

### 5.2 创建素材组

```bash
curl -s -X POST "$SITE/v1/assets/groups" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"冒烟测试","description":"deploy smoke","region":"cn"}'
```

记下返回的 `officialId`（形如 `group-2026xxx`）。

### 5.3 上传素材并等待审核

```bash
export GROUP=group-2026xxx
curl -s -X POST "$SITE/v1/assets" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"groupId\":\"$GROUP\",\"url\":\"https://example.com/test.jpg\",\"name\":\"smoke\"}"

# 轮询状态直到 Active
export ASSET=asset-2026xxx
curl -s "$SITE/v1/assets/$ASSET" -H "Authorization: Bearer $TOKEN"
```

### 5.4 归属隔离（必测）

用**另一个用户**的 token 查同一个 officialId，必须返回 404 且**不应打到上游**：

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  "$SITE/v1/assets/$ASSET" -H "Authorization: Bearer $OTHER_USER_TOKEN"
# 期望：404
```

同时确认 `GET /v1/assets` 在另一个用户下看不到这条记录。这是本期最重要的安全断言——全站共用一个上游账号，隔离完全靠本地表。

### 5.5 asset:// 引用与前置校验

```bash
# 正常引用（素材已 Active，region=cn 配 cn 模型）
curl -s -X POST "$SITE/v1/video/generations" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"model\":\"doubao-seedance-2-0-260128\",\"prompt\":\"test\",
       \"content\":[{\"type\":\"image_url\",\"image_url\":{\"url\":\"asset://$ASSET\"},\"role\":\"first_frame\"}]}"

# 区域不匹配 —— 期望 400 asset_region_mismatch，且不打到上游
curl -s -X POST "$SITE/v1/video/generations" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d "{\"model\":\"dreamina-seedance-2-0-260128\",\"prompt\":\"test\",
       \"content\":[{\"type\":\"image_url\",\"image_url\":{\"url\":\"asset://$ASSET\"}}]}"

# 不存在的素材 —— 期望 404 asset_not_found
curl -s -X POST "$SITE/v1/video/generations" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedance-2-0-260128","prompt":"t",
       "content":[{"type":"image_url","image_url":{"url":"asset://asset-not-exist"}}]}'
```

### 5.6 回归：不含 asset:// 的生成请求

**这条必须做。** 用一个纯公网 URL 的生成请求，确认行为与升级前完全一致：

```bash
curl -s -X POST "$SITE/v1/video/generations" \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"model":"doubao-seedance-2-0-260128","prompt":"a cat",
       "content":[{"type":"image_url","image_url":{"url":"https://example.com/a.jpg"}}]}'
```

### 5.7 计费未受影响

上述素材接口调用完成后，检查 `logs` 表**没有**新增记录、用户额度**未**变化：

```sql
SELECT count(*) FROM logs WHERE created_at > <上面操作的起始时间戳>;
```

### 5.8 前端页面

登录控制台 → 侧边栏应出现「素材库」→ 打开 `/console/assets`：

- 素材列表能加载、筛选、分页；
- 「素材组」Tab 能创建（region 单选带不可修改警告）；
- 上传弹窗能提交单条与多条 URL；
- 下载模板按钮能拿到 .xlsx；
- 存在 `Processing` 记录时列表每 15s 自动刷新。

---

## 6. 部署后需要留意的运行特征

1. **列表接口默认不回源。** `GET /v1/assets` 只查本地表；只有带 `?refresh=true` 时才向上游同步审核状态（前端轮询会自动带上）。纯 API 用户想拿最新状态应调 `GET /v1/assets/{officialId}`，该接口每次都同步。

2. **Excel 批量上传的记录会先缺字段。** 服务端**不解析** Excel，上游批量响应只按 index 回显 `officialId`，因此 `name` / `groupId` / `region` / `url` 落库时为空，由后续同步逐条回填。前端会显示「同步中…」。这是设计如此，不是 bug。

3. **单点依赖。** 唯一的素材渠道被删除、禁用或上游账号欠费时，素材功能整体不可用，且已发出的 `asset://` 全部失效。删除该渠道前先确认 `SELECT count(*) FROM assets WHERE channel_id = ?`。

4. **绕过 new-api 直接在 seegen.ai 控制台创建的素材不会出现在列表里**，本期不做导入同步。请在团队内约定：素材一律通过 new-api 创建。

5. **限流生效于每个用户每分钟 60 次**（默认），素材接口不计费但会占用上游审核队列。

---

## 7. 回滚

回滚是安全的，两张新表可以保留（不影响旧版本运行）：

```bash
# Docker
cd /opt/new-api
git checkout <上一个 tag 或 commit>
docker compose build && docker compose up -d

# 二进制
git checkout <上一个 commit>
# 重新执行 §2 的前端 + 后端构建，替换二进制后重启
```

回滚后：

- `/v1/assets*` 返回 404，前端「素材库」入口消失；
- 生成任务链路恢复为不带 `AssetRefCheck`，含 `asset://` 的请求会直接透传给上游（上游会自行判定，行为等同于本次改动前）；
- `assets` / `asset_groups` 两张表保留，数据不丢，再次升级即可继续使用；
- `options` 表里的 `assets_setting.*` 记录保留，旧版本会忽略未注册的配置。

若确实要清库（**不可逆**）：

```sql
DROP TABLE assets;
DROP TABLE asset_groups;
DELETE FROM options WHERE key LIKE 'assets_setting.%';
```

---

## 8. 已知缺口 / 后续待办

| # | 事项 | 影响 | 建议 |
|---|---|---|---|
| 1 | `assets_setting.*` 没有前端设置表单 | 只能用 API / SQL 配置 | 前端未配置渠道时的空态里有一个指向 `/console/setting` 的链接，但那里目前**没有对应字段**。下个迭代补一个运营设置分区 |
| 2 | 视频 / 音频素材的上游 URL 字段名未实测确认 | 回填可能拿到空 URL | 代码已按 `url → imageUrl → videoUrl → audioUrl → fileUrl` 依次兜底。上线后传一个视频素材，检查 `assets.source_url` 是否非空 |
| 3 | Excel 的 `results[].index` 基准未实测确认 | 影响用户按行号自查失败条目 | 上线后用一个含错误行的模板实测，把结论写进用户文档 |
| 4 | 前端未经 eslint / vite build 校验 | 可能有格式漂移或运行时报错 | 部署前跑 `bun run lint:fix` 与一次完整 `bun run build` |
| 5 | 多渠道支持 | 本期不支持 | 演进路径见 PRD §10，三点加法变更，无需数据迁移 |

---

## 9. 一页速查

```bash
# 1. 本地验证（必做）
go build ./... && go test ./router/ ./middleware/

# 2. 构建部署
docker compose build && docker compose up -d

# 3. 确认表已建
#    assets / asset_groups

# 4. 指定素材渠道（root 身份）
curl -X PUT $SITE/api/option/ -H "Content-Type: application/json" \
  -H "Cookie: <root session>" \
  -d '{"key":"assets_setting.channel_id","value":"<渠道ID>"}'

# 5. 冒烟
curl $SITE/v1/assets/groups -H "Authorization: Bearer $TOKEN"

# 6. 回归（最重要）
#    不含 asset:// 的生成请求，行为必须与升级前一致
```
