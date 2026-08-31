# 万象一刻（Yike）渠道 —— 实现与配置说明

渠道类型 **58**。上游是阿里云 RPC 风格接口，与已接入的豆包/润元**没有一层可以复用**。

---

## 1. 新增与改动的文件

### 新增

| 文件 | 作用 |
|---|---|
| `common/aliyunsign/sign.go` | 阿里云 ACS3-HMAC-SHA256（V3）签名 |
| `common/aliyunsign/sign_test.go` | 签名单测（含"无密钥派生链"的钉死断言） |
| `relay/channel/task/yike/constants.go` | 模型目录、参数边界、计费倍率 |
| `relay/channel/task/yike/types.go` | 请求/响应结构 |
| `relay/channel/task/yike/adaptor.go` | TaskAdaptor 实现 |
| `relay/channel/task/yike/billing.go` | GetYikeJobCredit 差额结算 |
| `relay/channel/task/yike/adaptor_test.go` | 适配器单测 |
| `service/assets_provider_yike.go` | 素材登记 Provider（ImportMedia / GetMedia） |
| `service/assets_provider_yike_test.go` | Provider 单测 |

### 改动（每处都是插入，未改动既有行）

| 文件 | 改动 |
|---|---|
| `constant/channel.go` | `ChannelTypeYike = 58` + BaseURL + 名称 |
| `relay/relay_adaptor.go` | `GetTaskAdaptor` 注册 |
| `controller/channel-test.go` | 加入"不支持渠道测试"列表 |
| `dto/channel_settings.go` | `yike_credit_usd_rate` |
| `service/assets.go` | 加入 `assetsChannelTypes` |
| `service/assets_provider.go` | Provider 注册与 base_url 探测 |
| `web/src/constants/channel.constants.js` | 渠道下拉项 |
| `web/src/components/table/channels/modals/EditChannelModal.jsx` | 密钥格式提示 |

`controller/video_proxy.go` **未改动**：默认分支已经走 `GetUpstreamResultURL()`，
而万象一刻的 OutputUrl 是预签名 OSS 地址，回源不需要额外鉴权头，正是默认分支的行为。

---

## 2. 渠道配置

| 项 | 值 |
|---|---|
| 类型 | Yike (58) |
| 密钥 | `AccessKeyId|AccessKeySecret`（阿里云 RAM 密钥，用 `|` 分隔） |
| Base URL | `https://yike.ap-southeast-1.aliyuncs.com` |

### 关于地域

地域**没有写死**，由 Base URL 决定，默认 `ap-southeast-1`。
换 `cn-shanghai` 只需把 Base URL 改成 `https://yike.cn-shanghai.aliyuncs.com`，
代码无需改动 —— 签名的 host 直接取自 URL。

> 官方 Python 示例里 `cn-shanghai` 是被注释掉的备选项，`ap-southeast-1` 是默认值，
> 因此按后者设默认。两地是否共用同一套 AK/SK 与模型白名单，需要联调确认。

### 模型重定向（必填）

对外模型名与上游模型名不同，**必须**在渠道「模型重定向」里配置，否则上游会报模型不存在：

```json
{
  "seedance-2.0": "Wonder-Standard",
  "seedance-2.0-mini": "Wonder-Pro",
  "seedance-2.5": "Wonder-Ultra",
  "yk-happyhorse-1.0": "happyhorse-1.0",
  "yk-happyhorse-1.1": "happyhorse-1.1",
  "yk-wan-2.7": "wan2.7",
  "yk-wan-3.0": "wan3.0-video"
}
```

这份映射在 `constants.go` 的 `SuggestedModelMapping` 里有一份同步的副本，
并有单测保证它与 `ModelList` 逐项对齐 —— 加模型时漏配会被测试挡下。

**漏配会在出网前被拦下**：如果提交时的模型名仍是对外名（说明重定向没生效），
适配器直接报错并给出该填的目标值，而不是把一个上游根本不存在的名字发过去、
换回一句与真实原因毫无关联的「模型不存在」。

---

## 3. 计费

### 必须先为 7 个对外模型配价

`ModelPriceHelperPerCall` 的行为是**失败关闭**：模型既没有按次价格、也没有倍率时，
请求直接报「模型 xxx 的价格未配置」，不会以 0 计费放行。所以配价是渠道能用的前提，
不是可选项。

我**没有**塞默认价 —— 万象一刻的成本我没有依据，凭空写一个数会被静默当成真实定价，
比报错要糟。报错文案本身已经指明了模型名和该去哪配（系统设置 → 分组与模型定价设置）。

配的是**按次价格**（`ModelPrice`），**不要**配成 token 倍率。

原因是一个已经在 `doubao-seedance-2.0` 上造成超收的坑：`RecalculateTaskQuotaByTokens`
（`service/task_billing.go:296`）在按 token 结算时**同样会乘 `otherMultiplier`**，而上游
返回的 token 数本身就已经随时长和分辨率缩放了（实测 5 秒 720P ≈ 108,900 token，
15 秒 ≈ 324,900）。两者相乘等于把时长算两遍，15 秒的任务会被收 3 倍。

万象一刻走按次价格路径，`EstimateBilling` 的乘数只在提交时应用一次，且上游不返回
token（差额由 credit 结算），因此不受这个坑影响。但如果有人把这 7 个模型配成倍率而不是
按次价格，就会踩进同一个双重计算。

### 预扣

后台把模型价格设为 **720P / 5 秒** 的基准价，系统自动乘：

- `seconds` —— 每满 5 秒一个计费单位，不足向上取整（10 秒 = 2，11 秒 = 3）
- `size` —— 720P = 1.0，1080P = 1.5

### 差额结算（`yike_credit_usd_rate`）

任务成功后会调 `GetYikeJobCredit` 拿上游实际消耗，按此换算率结算差额。

**默认不启用**（值为 0 时保持预扣）。原因：上游资料只说明了这个 Action 的用途，
没有给响应样例，credit 的计价单位无从推断。在拿到真实响应之前按它结算，
等于拿客户余额赌一个未经验证的假设。

联调拿到样例后：
1. 确认 credit 的实际单位与响应字段名；
2. 在渠道「其他设置」里填 `yike_credit_usd_rate`；
3. 核对 `billing.go` 的 `creditKeys` 是否命中真实字段名（目前是按候选名递归搜第一个正数）。

查询失败或解析不出时一律**保持预扣**，绝不当成 0 消耗 ——
否则上游每一次抖动都会变成一次全额退款。

---

## 4. 素材登记

Wonder 系列**不接受外部 URL**，参考素材必须先登记：

```
ImportMedia → GetMedia 轮询 ThirdPartyAssetStatus=Success → 提交任务传 MediaId
```

生成请求里的素材引用支持两种写法，都会被归一化：`media-xxx`、`asset://media-xxx`。
直链（`http://` / `https://`）会被**静默剔除**，并让 jobType 相应退化 ——
透传给上游只会换来一个与素材毫无关联的错误信息。

### 两处与其他上游不同的地方

1. **可用性看 `ThirdPartyAssetStatus`，不是 `Status`。**
   `Status` 只说明上游自己入库成功，不代表 Wonder 能引用它。拿错字段会让素材
   过早变成 Active，任务带着不可用素材提交，白扣一次额度。字段缺失时保守判为处理中。

2. **上游没有素材组，也没有删除接口。**
   - 组：用合成的固定组 ID `yike-default` 占位，让上层的 groupId 非空契约仍然成立。
   - 删除：只做本地清理并返回成功。调用方的语义是"从本站移除"，这个目标已达成；
     返回 501 会让用户反复删除反复失败，比上游残留一条孤儿记录糟糕得多。

### 素材渠道自动探测

`assetsChannelTypes` 现在包含 Yike。**如果同时启用了豆包视频渠道和 Yike 渠道，
自动探测会判为 ambiguous**，必须在系统设置里显式指定 `assets_setting.channel_id`。

---

## 5. 格式陷阱

上游对这几处零容忍，写错直接拒，且错误信息通常不指向真正的字段：

| 项 | 正确写法 | 常见错误 |
|---|---|---|
| 时长 | `"Duration": "10"` | 整数 `10` |
| 分辨率 | `"720P"` | 小写 `"720p"` |
| 宽高比 | `AspectRatio` | `Ratio` |
| Input | 序列化后的 **JSON 字符串** | 嵌套对象 |
| JobParameters | 序列化后的 **JSON 字符串** | 嵌套对象 |
| 水印 | **不支持该参数** | 传 `watermark` |
| jobType | 由素材构成推导 | 由调用方指定 |

适配器会把 `720p` / `1280x720` 都归一化到 `720P`；认不出来的写法返回空串走上游默认值，
而不是原样透传一个上游必然不认的字符串。

---

## 6. 尚未验证的部分

### 签名无法本地验证 ⚠️

润元那次可以拿生产已验证的 `jimeng.Sign` 做逐字节交叉校验，**万象一刻没有这样的参照**。
单测能保证的只是"实现与算法描述一致"（含那条钉死无密钥派生链的断言），
**不能**保证与阿里云服务端一致。

建议的验证方式（需要你的网络与一组 RAM AK/SK）：
1. 用官方 Python SDK 发一次真实请求，抓下 `Authorization` 头；
2. 固定 AK、时间戳、nonce、请求体；
3. 断言 Go 实现产出同一个签名。

在这之前，签名正确性属于**未验证**状态。

### 待联调核对的字段名

- `ImportMedia` 的参数名：SDK 证明 RPC 层是 PascalCase（`job_type` → `JobType`），
  但参考文档里写的是 `importSource` / `inputURL` 的 lowerCamel 形态。目前按 PascalCase 实现。
- `GetYikeJobCredit` 的响应结构（见第 3 节）。
- `GetVideoGenerationJob` 响应的包装层字段名 —— 已做包装/扁平两种形态兜底。
