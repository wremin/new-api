# 万象一刻（Yike / Wonder）渠道适配设计文档

- 文档版本：v0.1（设计评审稿）
- 参考资料：`bin/wonder_seedance_SDk/`（yike.md、sample.py、query_job.py、README.md）
- 仓库基线：`metamind @ 05bb43c1`

---

## 1. 上游是什么

阿里云「万象一刻」（产品代号 Yike），后端是部署在阿里云上的 Seedance：

| 对外线路 | 上游 model | 实际内核 |
|---|---|---|
| Wonder-Standard | `Wonder-Standard` | Seedance 2.0 |
| Wonder-Pro | `Wonder-Pro` | Seedance 2.0 Mini |
| Wonder-Ultra | `Wonder-Ultra` | Seedance 2.5 |

同一账号下还有 `happyhorse-1.0` / `happyhorse-1.1` / `wan2.7` / `wan3.0-video` 四条非 Seedance 线路，走同一套接口，只靠 `model` 字段区分。

**基础配置**

```
Endpoint      yike.ap-southeast-1.aliyuncs.com   （新加坡；另有 yike.cn-shanghai.aliyuncs.com）
Region        ap-southeast-1
API Version   2026-07-07
鉴权          阿里云 RAM AccessKey ID / Secret 签名
协议风格      阿里云 RPC —— Action 放在参数里，不是 REST 路径
```

参考资料里特别强调了一句：**「不应自行拼接 `/SubmitVideoGenerationJob` 之类的 REST 路径」**。

---

## 2. 与已接入的润元/豆包有什么本质不同

这是本次工作量的来源。表面上都是「Seedance 异步视频生成」，但**每一层都不一样**：

| 维度 | 润元 / 豆包（已接入） | 万象一刻（本次） |
|---|---|---|
| 协议风格 | REST，路径区分操作 | **RPC，Action 区分操作** |
| 鉴权 | `Authorization: Bearer {key}` | **阿里云 RAM AK/SK 签名** |
| 提交路径 | `POST /v1/video/tasks` | `POST /`，`Action=SubmitVideoGenerationJob` |
| 任务 ID | `task_id` | `JobId` |
| 提示词位置 | `content[]` 数组 | **`Input` 字段内的 JSON 字符串** |
| 参考素材 | `asset://` 引用，写在 content 里 | **`Input.Medias[].MediaId`** |
| 素材登记 | `CreateAsset`（火山 AK/SK 签名） | **`ImportMedia` + `GetMedia` 轮询** |
| 时长参数 | `duration: 5`（整数） | `duration: "10"`（**字符串**） |
| 分辨率 | `resolution: "720p"`（小写） | `resolution: "720P"`（**大写 P**） |
| 宽高比字段 | `ratio` | **`aspectRatio`** |
| 音频开关 | `generate_audio: false` | **`jobParameters` 里的 `{"EnableAudio":true}`** |
| 水印 | `watermark: false` | **不支持**，上游无此参数 |
| 状态取值 | queued/running/succeeded/failed | **Created/Queuing/Executing/Finished/Failed** |
| 结果地址 | `content.video_url` | **`Output` 字符串里的 `Medias[].OutputUrl`** |

**没有一层能直接复用豆包适配器。** 这不是"再加一条 base_url 分支"能解决的，必须独立成新渠道类型 + 新任务适配器。

---

## 3. 请求与响应

### 3.1 提交任务 `SubmitVideoGenerationJob`

```json
{
  "model": "Wonder-Pro",
  "jobType": "reference_to_video",
  "scene": "general",
  "input": "{\"Prompt\":\"视频提示词\",\"Medias\":[{\"Type\":\"image\",\"MediaId\":\"media-xxx\"}]}",
  "jobParameters": "{\"EnableAudio\":true}",
  "resolution": "1080P",
  "aspectRatio": "16:9",
  "duration": "10",
  "n": 1,
  "clientToken": "<幂等键>"
}
```

> ⚠️ **`input` 和 `jobParameters` 是序列化后的 JSON 字符串，不是嵌套对象。**
> 这是最容易写错的一处 —— 直接传对象上游会拒。

**`jobType` 由输入素材决定**：

| 输入 | jobType |
|---|---|
| 无参考素材 | `text_to_video` |
| 恰好 1 张图 | `image_to_video` |
| 恰好 2 张图（首尾帧） | `first_last_frame` |
| 多图 / 视频 / 音频 | `reference_to_video` |

响应：`{"RequestId": "...", "JobId": "ag_3e761e9d1140c42a1b7..."}`

### 3.2 查询任务 `GetVideoGenerationJob`

```
job.status ∈ Created | Queuing | Executing | Finished | Failed
job.output        —— JSON 字符串，需二次解析
job.error_message —— 失败原因
```

`output` 解析后：

```json
{ "Medias": [ { "OutputUrl": "https://.../xxx.mp4" } ] }
```

> `OutputUrl` 是**临时地址**（参考资料里的流程明确写了「下载临时 OutputUrl → 转存」）。
> 有效期未知，需实测确认 —— 这直接决定视频代理层要不要做「过期重取」。

### 3.3 参数边界

| 模型 | 时长 | 分辨率 | 参考素材上限 |
|---|---|---|---|
| Wonder-Standard | 4–15 秒 | 720P / 1080P | 最多 15 个 |
| Wonder-Pro | 4–15 秒 | 720P / 1080P | 最多 15 个 |
| Wonder-Ultra | 4–30 秒 | 720P / 1080P | 图片 30、视频 10、音频 10，总计 50 |

画幅：`16:9` / `9:16` / `1:1` / `4:3` / `3:4`

---

## 4. ★ 核心难点：阿里云 RAM 签名

仓库现状：`relay/channel/ali` 走的是 DashScope 的 Bearer Token，**不是** RPC 签名。全仓没有任何阿里云 RAM 签名实现，必须新写。

API Version `2026-07-07` 属于阿里云新版接口，对应 **ACS3-HMAC-SHA256（V3）签名**：

```
CanonicalRequest = HTTPMethod \n CanonicalURI \n CanonicalQueryString \n
                   CanonicalHeaders \n SignedHeaders \n HashedRequestPayload
StringToSign     = "ACS3-HMAC-SHA256" \n HEX(SHA256(CanonicalRequest))
Signature        = HEX(HMAC-SHA256(SecretKey, StringToSign))
Authorization    = ACS3-HMAC-SHA256 Credential=<AK>,SignedHeaders=<...>,Signature=<sig>
```

必需请求头：`x-acs-action`、`x-acs-version`、`x-acs-date`、`x-acs-signature-nonce`、`x-acs-content-sha256`、`host`。

> **与火山签名的关键差异**：火山有四级密钥派生链（kDate→kRegion→kService→kSigning），
> **阿里云 V3 没有** —— 直接用 SecretKey 对 StringToSign 做一次 HMAC。
> 照着火山那套写必然失败。

### 签名正确性怎么保证

火山那次我能拿仓库里已在生产跑通的 `jimeng.Sign` 做逐字节交叉验证。**这次没有可比对的实现**，而签名错一个字节就是 403，本地无法自证。

建议用「黄金样本」法：

1. 在本机跑一次官方 Python SDK（`bin/wonder_seedance_SDk/alibabacloud_sample/sample.py`），
   用 mitmproxy 或 `HTTP_PROXY` 抓下它实际发出的完整请求头；
2. 把那次请求的 AK、时间戳、nonce、body 固定住，作为 Go 单测的输入；
3. 断言 Go 实现算出的 `Authorization` 与 SDK 一致。

这需要一组可用的 RAM AK/SK 和网络 —— **我这边两个沙箱都出不了网，只能由你执行**。
没有这一步，签名只能等联调时靠上游的 403 反推，成本高很多。

---

## 5. 素材登记链路

Wonder 系列**不能直接把外部 URL 放进生成请求**，必须先登记：

```
ImportMedia  { importSource:"url", inputURL:"https://...", mediaType:"image",
               title:"...", registerConfig:"{\"NeedThirdPartyAsset\":true,\"NeedSnapshot\":true}" }
     ↓
GetMedia     { mediaId:"media-xxx", authTimeout:3600 }
     ↓  轮询直到 ThirdPartyAssetStatus == Success
SubmitVideoGenerationJob   input.Medias[] = [{ Type:"image", MediaId:"media-xxx" }]
```

素材类型：`image` / `video` / `audio`。

**与现有素材库抽象的关系**：仓库已有 `service.AssetsProvider` 接口（seegen / stelloria / runyuan 三家）。
万象一刻的素材语义能对上大部分方法（Create/Get/Delete），但有两处不匹配：

- 它**没有素材组概念** —— `CreateGroup` / `GroupTypes` 无对应物；
- 状态字段叫 `ThirdPartyAssetStatus`，取值 `Success`，与现有的 `Processing/Active/Failed` 需要映射。

可以实现成第四个 Provider，`Capabilities()` 里把 `Regions`、`DeleteGroup` 等置 false。

---

## 6. 计费

上游提供了两个额度接口，是**豆包/润元都没有的**：

| Action | 用途 |
|---|---|
| `GetYikeAccountCredit` | 查账户会员与剩余额度 |
| `GetYikeJobCredit` | 查**单个任务**的实际计费 |

`GetYikeJobCredit` 很有价值 —— 它让 `AdjustBillingOnComplete` 能拿到上游的真实扣费，
而不是像现在这样靠 `usage.total_tokens` 反推。建议在任务完成时调一次，用真实值做差额结算。

代价是每个任务多一次上游调用。可以做成渠道级开关。

---

## 7. 改动清单（预估）

| 文件 | 改动 |
|---|---|
| `constant/channel.go` | 新增 `ChannelTypeYike = 58`（当前最大 57）+ 名称 + 默认 base url |
| `common/aliyunsign/sign.go` | **新增** ACS3-HMAC-SHA256 签名 |
| `common/aliyunsign/sign_test.go` | **新增** 黄金样本单测 |
| `relay/channel/task/yike/adaptor.go` | **新增** 任务适配器 |
| `relay/channel/task/yike/types.go` | **新增** 请求/响应结构 |
| `relay/channel/task/yike/constants.go` | **新增** 模型列表、参数边界、倍率 |
| `relay/relay_adaptor.go` | `GetTaskAdaptor` 注册 |
| `controller/video_proxy.go` | 新增 case（`OutputUrl` 若有有效期需重取） |
| `controller/channel-test.go` | 加入 `unsupportedTestChannelTypes` |
| `service/assets_provider_yike.go` | **新增** 素材 Provider（若纳入本期） |
| `web/src/constants/channel.constants.js` | 前端渠道项 |
| `web/.../EditChannelModal.jsx` | AK/SK 与 Region 配置项 |

**密钥存放**：沿用润元那次的结论 —— 渠道 Key 存 `AK|SK`（仓库既有的火山系约定），
Region / Endpoint 放渠道「其他设置」。这次只有一套凭证，比润元简单。

---

## 8. 待确认决策

1. **模型范围** —— 只做 Wonder-Standard / Pro / Ultra 三条，还是把 happyhorse / wan 系列也纳入？
   （协议完全相同，只是 `model` 字符串不同，多做几乎无额外成本）
2. **素材登记（ImportMedia / GetMedia）是否纳入本期？**
   不做的话只能跑纯文生视频 —— 而 Wonder 系列的主要卖点正是参考素材。
3. **区域** —— 固定 `ap-southeast-1`（新加坡），还是做成渠道可配（另有 `cn-shanghai`）？
   涉及数据出境合规，建议你确认后再定。
4. **`GetYikeJobCredit` 是否接入**做精确计费？每任务多一次上游调用。
5. **对外模型名** —— 直接暴露 `Wonder-Pro`，还是配模型重定向换成自己的命名？
   （上次润元那边这个问题悬而未决，两个渠道最好一致）

---

## 9. 风险

| 风险 | 说明 | 应对 |
|---|---|---|
| **签名无法本地自证** | 没有可交叉验证的实现，错了只能等 403 | 用官方 SDK 抓黄金样本（第 4 节） |
| `OutputUrl` 有效期未知 | 影响视频代理是否需要"过期重取" | 联调时实测：拿到地址后隔 1 小时再访问 |
| 参数格式陷阱密集 | 字符串时长、大写 P、序列化 JSON 字符串、`aspectRatio` 拼写 | 每条都写进单测钉死 |
| 参考资料是对话记录 | `yike.md` 是与另一 AI 的聊天导出，非官方文档，且前半段混入了无关的 TypeScript 代码 | 关键参数以官方 SDK（`alibabacloud_yike20260707==2.3.2`）的模型定义为准 |
| 无水印控制 | 上游不接受 `watermark` 参数 | 文档中明确告知调用方 |

---

*基于 `bin/wonder_seedance_SDk/` 的参考资料与仓库当前代码撰写，供评审。*
