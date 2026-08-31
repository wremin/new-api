 export function preliminaryOverseasVideoProvenanceVerification(
        ⋮
    128  ): VideoProvenanceVerification {
    129 +  const officialUrlEvidence = officialResultUrlCheck(input);
    130 +  if (officialUrlEvidence) {
    131 +    return {
    132 +      contractVersion: VIDEO_PROVENANCE_CONTRACT_VERSION,
    133 +      status: "completed",
    134 +      verdict: "verified",
    135 +      claimedProvider: claimedProvider(input),
    136 +      claimedModel: claimedModel(input),
    137 +      verifiedProvider: officialUrlEvidence.provider,
    138 +      verifiedModel: officialUrlEvidence.model,
    139 +      sourceFileSha256: null,
    140 +      verificationTaskId: input.upstreamJobId,
    141 +      requestId: null,
    142 +      checkedAt: (input.checkedAt ?? new Date()).toISOString(),
    143 +      summary: "BytePlus 官方结果 URL 已验证线路来源；C2PA、ExifTool、SEI 文件内验证待执行。",
    144 +      checks: [officialUrlEvidence],
    145 +    };
    146 +  }
    147    const evidence = isHttpResultUrl(input.resultUrl)
        ⋮
    302    ]);
    257 -  const checks = [parseC2paCheck(c2pa), parseExifToolCheck(exif), parseSeiCheck(sei)];
    258 -  const c2paEvidence = checks[0];
    303 +  const officialUrlEvidence = officialResultUrlCheck(input);
    304 +  const fileChecks = [parseC2paCheck(c2pa), parseExifToolCheck(exif), parseSeiCheck(sei)];
    305 +  const checks = officialUrlEvidence ? [officialUrlEvidence, ...fileChecks] : fileChecks;
    306 +  const c2paEvidence = fileChecks[0];
    307    const isWan30 = input.assetProvider === "wan30";
    260 -  const expectedVersion = input.assetProvider === "oversea5" || input.assetProvider === "oversea7"
    261 -    ? expectedSeedanceVersion(input.assetProvider)
    262 -    : null;
    308 +  const expectedVersion = isWan30 ? null : expectedSeedanceVersion(input.assetProvider);
    309    const signedTarget = c2paEvidence.outcome === "verified" && (isWan30
        ⋮
    312    const signedVersion = c2paEvidence.model?.match(/2\.[05]/)?.[0] ?? null;
    267 -  const exactVersionMismatch = !isWan30 && signedVersion !== null && signedVersion !== expectedVersion;
    313 +  const exactVersionMismatch = !isWan30 && signedVersion !== null && expectedVersion !== null && signedVersion !== expectedVersion;
    314    const signedModelMismatch = c2paEvidence.outcome === "verified" && Boolean(c2paEvidence.model) && !signedTarget;
    269 -  const unavailable = checks.some((check) => check.outcome === "unsupported" || check.outcome === "error");
    270 -  const heuristicEvidence = checks.slice(1).some((check) => check.outcome === "verified");
    315 +  const unavailable = fileChecks.some((check) => check.outcome === "unsupported" || check.outcome === "error");
    316 +  const heuristicEvidence = fileChecks.slice(1).find((check) => check.outcome === "verified" && (isWan30
    317 +    ? check.model === "wan3.0-video"
    318 +    : check.model?.startsWith("Seedance") && (!check.model.match(/2\.[05]/)?.[0] || check.model.includes(expectedVersion!))));
    319    const verdict = exactVersionMismatch || signedModelMismatch
    320      ? "not_verified"
    273 -    : signedTarget
    321 +    : officialUrlEvidence || signedTarget || heuristicEvidence
    322        ? "verified"
    275 -      : unavailable || heuristicEvidence
    323 +      : unavailable
    324          ? "inconclusive"
        ⋮
    329        ? `C2PA 中的模型 ${c2paEvidence.model} 与该线路声明的 ${claimedModel(input)} 不一致。`
    282 -    : verdict === "verified"
    283 -      ? isWan30
    284 -        ? "C2PA 已验证 WAN3.0 来源。"
    285 -        : `C2PA 已验证 Seedance 来源${signedVersion ? `及 ${signedVersion} 版本` : "，但未编码精确版本"}。`
    286 -      : heuristicEvidence
    287 -        ? "本地工具发现来源线索，但没有可独立验证的目标模型签名。"
    330 +    : officialUrlEvidence
    331 +      ? "BytePlus 官方结果 URL 已验证线路来源；文件内检测结果另列。"
    332 +      : signedTarget
    333 +        ? isWan30
    334 +          ? "C2PA 已验证 WAN3.0 来源。"
    335 +          : `C2PA 已验证 Seedance 来源${signedVersion ? `及 ${signedVersion} 版本` : "，但未编码精确版本"}。`
    336 +        : heuristicEvidence
    337 +          ? `${heuristicEvidence.verifier} 检测到与声明线路一致的来源标记。`
    338          : unavailable
    339            ? "部分来源验证工具不可用或执行失败，当前结论不确定。"
    290 -          : "三项本地检测均已完成，未发现可独立验证的模型来源证据。";
    340 +          : "结果 URL 未命中官方域名，三项文件扫描也未检测到官方来源证据。";
    341 +  const verifiedEvidence = officialUrlEvidence || (signedTarget ? c2paEvidence : heuristicEvidence || null);
    342    return {
        ⋮
    347      claimedModel: claimedModel(input),
    297 -    verifiedProvider: verdict === "verified" ? c2paEvidence.provider : null,
    298 -    verifiedModel: verdict === "verified" ? c2paEvidence.model : null,
    348 +    verifiedProvider: verdict === "verified" ? verifiedEvidence?.provider ?? null : null,
    349 +    verifiedModel: verdict === "verified" ? verifiedEvidence?.model ?? null : null,
    350      sourceFileSha256: null,

• Edited ~/projects/wt/ac-codexlb-provenance/scripts/video-status-poll.ts (+1 -0)
    781                  claimedModel: upstreamVideoJob?.model ?? null,
    782 +                resultUrl: st.videoUrl,
    783                });

• Ran node --import tsx --test packages/generation/application/overseas-seedance-provenance.test.ts
  └ ✔ a returned WAN3 result URL immediately creates a preliminary model judgment (1.284208ms)
    ✖ an authenticated BytePlus task result immediately verifies the official delivery route (2.213916ms)
    … +110 lines (ctrl + t to view transcript)
        diff: 'simple'
      }

• Edited 2 files (+8 -8)
  └ ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.test.ts (+5 -5)
     48    assert.equal(verification.checks[0]?.outcome, "verified");
     49 -  assert.match(verification.summary, /官方任务查询响应/);
     49 +  assert.match(verification.summary, /结果 URL 命中官方域名/);
     50    assert.match(verification.summary, /文件内验证待执行/);
        ⋮
    141
    142 -test("unsigned metadata cannot verify a Wonder Pro Seedance 2.0 claim", async () => {
    142 +test("matching ExifTool metadata detects a Wonder Pro Seedance 2.0 source", async () => {
    143    const runner: ProvenanceToolRunner = async (command) => {
        ⋮
    151    }, runner);
    152 -  assert.equal(verification.verdict, "inconclusive");
    153 -  assert.equal(verification.verifiedProvider, null);
    154 -  assert.match(verification.summary, /来源线索/);
    152 +  assert.equal(verification.verdict, "verified");
    153 +  assert.equal(verification.verifiedProvider, "ByteDance");
    154 +  assert.match(verification.summary, /检测到与声明线路一致的来源标记/);
    155  });

  └ ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.ts (+3 -3)
    142        checkedAt: (input.checkedAt ?? new Date()).toISOString(),
    143 -      summary: "BytePlus 官方结果 URL 已验证线路来源；C2PA、ExifTool、SEI 文件内验证待执行。",
    143 +      summary: "BytePlus 官方线路来源已验证（结果 URL 命中官方域名）；C2PA、ExifTool、SEI 文件内验证待执行。",
    144        checks: [officialUrlEvidence],
        ⋮
    330      : officialUrlEvidence
    331 -      ? "BytePlus 官方结果 URL 已验证线路来源；文件内检测结果另列。"
    331 +      ? "BytePlus 官方线路来源已验证（结果 URL 命中官方域名）；文件内检测结果另列。"
    332        : signedTarget
        ⋮
    339            ? "部分来源验证工具不可用或执行失败，当前结论不确定。"
    340 -          : "结果 URL 未命中官方域名，三项文件扫描也未检测到官方来源证据。";
    340 +          : "结果 URL 未命中官方域名，三项文件扫描均未检测到来源证据。";
    341    const verifiedEvidence = officialUrlEvidence || (signedTarget ? c2paEvidence : heuristicEvidence || null);

• Ran node --import tsx --test packages/generation/application/overseas-seedance-provenance.test.ts
  └ ✔ a returned WAN3 result URL immediately creates a preliminary model judgment (1.265875ms)
    ✔ an authenticated BytePlus task result immediately verifies the official delivery route (0.689292ms)
    … +17 lines (ctrl + t to view transcript)
    ℹ todo 0
    ℹ duration_ms 207.295709

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，回归用例已经跑通：官方 BytePlus URL 会直接判绿色；非官方 URL 且三项扫描都没有来源证据，会明确判“未检测到来源”；截图中的 2.0 对 null 假冲突也已消除。

  我正在补 T7→T3 的显示契约，确保 T3 不再把“文件内没有 C2PA”错误覆盖掉第一层官方 URL 结论。

• Edited ~/projects/wt/ac-codexlb-provenance/docs/T7_TO_T3_VIDEO_PROVENANCE_MONITOR_HANDOFF_2026-08-24.md (+20 -2)
    138  | `pending/running` | `验证中` | 蓝 | 显示 summary |
    139 -| `completed + verified` | `来源已验证` | 绿 | 显示 verifiedProvider、verifiedModel、issuer、SHA-256 前 12 位 |
    139 +| `completed + verified` | `来源已验证` | 绿 | 第一层官方 URL 或第二层文件扫描命中来源证据；显示 verifiedProvider、verifiedModel、issuer、SHA-256 前 12 位 |
    140  | `completed + not_verified` | `未验证到来源` | 红/橙 | 检查已完成，但没有得到可验证来源凭证 |
        ⋮
    151
    152 -禁止根据 `claim.model`、线路名称、可见 `AI generated` 字样自动显示绿色「来源已验证」。
    152 +禁止仅根据 `claim.model`、线路名称、可见 `AI generated` 字样自动显示绿色「来源已验证」。T3 必须读取
    153 +T7 写入的 `verification.verdict` 与 `verification.summary`，不得在 UI 侧重新推导或覆盖结论。
    154 +
    155 +### 两层判定顺序
    156 +
    157 +1. 第一层检查上游成片 URL。对于 `claimedProvider=byteplus-modelark`，仅当 HTTPS hostname
    158 +   为 `bytepluses.com` 或其严格子域名时，写入 `provider_signature: verified`，结论为
    159 +   `来源已验证`。相似字符串、查询参数、路径中的 `bytepluses.com` 均不算官方域名。
    160 +2. URL 不能确认官方来源时，依次读取 C2PA/c2patool、ExifTool、SEI 三项文件扫描结果。
    161 +3. 任一文件扫描发现与声明线路、版本一致的来源标记时，结论为 `来源已验证`；每项详情仍须注明
    162 +   是否属于密码学证明。
    163 +4. URL 未命中官方域名，且三项扫描均未发现一致来源证据时，结论为 `未验证到来源`，summary 为
    164 +   `结果 URL 未命中官方域名，三项文件扫描均未检测到来源证据。`
    165 +5. 工具缺失、超时或解析失败导致扫描没有完整执行时，结论为 `结果不确定`，不得写成已完成的未发现。
    166 +6. 只有已识别出线路期望版本且 C2PA 明确签出另一个版本时，才允许显示“版本不一致”；禁止把未知版本
    167 +   `null` 拼入结论。BytePlus Mini/Fast/Standard 均映射为 Seedance 2.0，7 阶映射为 Seedance 2.5。
    168
        ⋮
    180
    181 +BytePlus 海外 4.5/5/5.5/7 阶除三项文件扫描外，先执行上述官方 URL 域名判断。官方 URL 只证明
    182 +BytePlus 官方交付线路；C2PA、ExifTool、SEI 仍各自显示，用于说明文件内是否还存在独立来源证据。
    183 +
    184  验证失败不会阻断用户取得成片；任务会写入 `verificationStatus=failed`，由 T3 明确显示为验证失败。

• Ran git diff --check && git diff --stat && git status --short && node --import tsx --test packages/generation/application/provider-video-job-metadata.test.ts packages/generation/application/
  │ overseas-seedance-provenance.test.ts
  └  M docs/T7_TO_T3_VIDEO_PROVENANCE_MONITOR_HANDOFF_2026-08-24.md
     M packages/generation/application/overseas-seedance-provenance.test.ts
    … +28 lines (ctrl + t to view transcript)
    ℹ todo 0
    ℹ duration_ms 224.177625

• Edited ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.test.ts (+18 -0)
    141
    142 +test("lookalike BytePlus hostnames never pass the official URL check", () => {
    143 +  for (const resultUrl of [
    144 +    "https://bytepluses.com.evil.example/video.mp4",
    145 +    "https://fakebytepluses.com/video.mp4",
    146 +    "http://ark-content.tos-ap-southeast-1.bytepluses.com/video.mp4",
    147 +  ]) {
    148 +    const verification = preliminaryOverseasVideoProvenanceVerification({
    149 +      assetProvider: "oversea45",
    150 +      upstreamJobId: "spoofed",
    151 +      claimedProvider: "byteplus-modelark",
    152 +      claimedModel: "dreamina-seedance-2-0-mini-260615",
    153 +      resultUrl,
    154 +    });
    155 +    assert.equal(verification.verdict, "inconclusive");
    156 +    assert.equal(verification.checks.length, 0);
    157 +  }
    158 +});
    159 +
    160  test("matching ExifTool metadata detects a Wonder Pro Seedance 2.0 source", async () => {

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，二级规则已经落到代码并通过 20 项相关测试；我又补了伪域名防护，像 bytepluses.com.evil.com、fakebytepluses.com 或 HTTP 地址都不会误判官方。

  接下来跑完整生产构建和完整性检查。当前改动尚未提交、推送或部署；等验证全部通过后我会给你明确状态。

• Waited for background terminal · node --import tsx --test packages/generation/application/overseas-seedance-provenance.test.ts && npm run build

• Ran node --import tsx --test packages/generation/application/overseas-seedance-provenance.test.ts && npm run build
  └ ✔ a returned WAN3 result URL immediately creates a preliminary model judgment (1.339875ms)
    ✔ an authenticated BytePlus task result immediately verifies the official delivery route (0.850625ms)
    … +150 lines (ctrl + t to view transcript)
      311 |     : c2paEvidence.model?.startsWith("Seedance"));
    Next.js build worker exited with code: 1 and signal: null

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，完整构建抓到一个 TypeScript 窄化问题，不是业务逻辑失败：布尔别名没有让 TS 排除 wan30。我会做一行类型安全修正后重新跑完整构建。

• Edited ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.ts (+1 -1)
    307    const isWan30 = input.assetProvider === "wan30";
    308 -  const expectedVersion = isWan30 ? null : expectedSeedanceVersion(input.assetProvider);
    308 +  const expectedVersion = input.assetProvider === "wan30" ? null : expectedSeedanceVersion(input.assetProvider);
    309    const signedTarget = c2paEvidence.outcome === "verified" && (isWan30

• Waited for background terminal · npm run build

• Ran 2 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，我又发现一个必须一起修的点：只改新任务还不够。现在历史回填只挑“从未检测过”的资产，截图这条已经写过旧结论，因此部署后不会自动纠正。

  我会把判定规则升级为 v2，并让 worker 自动重检所有旧 v1 海外视频；重检时使用保存下来的原厂结果 URL。这样 T3 的旧记录也会逐步得到新结论，不需要人工改数据库。

• Ran 2 commands · ctrl + t to view transcript

• Edited 3 files (+24 -13)
  └ ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.test.ts (+2 -1)
    155      assert.equal(verification.verdict, "inconclusive");
    156 -    assert.equal(verification.checks.length, 0);
    156 +    assert.equal(verification.checks[0]?.method, "provider_signature");
    157 +    assert.equal(verification.checks[0]?.outcome, "not_found");
    158    }

  └ ~/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.ts (+15 -12)
    111  function officialResultUrlCheck(input: VerificationInput): VideoProvenanceCheck | null {
    112 -  if (claimedProvider(input) !== "byteplus-modelark" || !isOfficialBytePlusResultUrl(input.resultUrl)) {
    113 -    return null;
    114 -  }
    112 +  if (claimedProvider(input) !== "byteplus-modelark") return null;
    113 +  const verified = isOfficialBytePlusResultUrl(input.resultUrl);
    114    return {
    115      method: "provider_signature",
    117 -    outcome: "verified",
    116 +    outcome: verified ? "verified" : "not_found",
    117      verifier: "BytePlus official result URL",
    119 -    issuer: "BytePlus",
    120 -    provider: "BytePlus",
    121 -    model: claimedModel(input),
    122 -    note: "成片 URL 命中 BytePlus 官方域名；官方线路来源已验证",
    118 +    issuer: verified ? "BytePlus" : null,
    119 +    provider: verified ? "BytePlus" : null,
    120 +    model: verified ? claimedModel(input) : null,
    121 +    note: verified
    122 +      ? "成片 URL 命中 BytePlus 官方域名；官方线路来源已验证"
    123 +      : "成片 URL 未命中 BytePlus 官方域名",
    124    };
        ⋮
    129  ): VideoProvenanceVerification {
    129 -  const officialUrlEvidence = officialResultUrlCheck(input);
    130 +  const resultUrlCheck = officialResultUrlCheck(input);
    131 +  const officialUrlEvidence = resultUrlCheck?.outcome === "verified" ? resultUrlCheck : null;
    132    if (officialUrlEvidence) {
        ⋮
    164      summary: `初步判断：${claimedModelLabel(input)}。依据${evidence}；C2PA、ExifTool、SEI 文件级验证待执行。`,
    163 -    checks: [],
    165 +    checks: resultUrlCheck ? [resultUrlCheck] : [],
    166    };
        ⋮
    304    ]);
    303 -  const officialUrlEvidence = officialResultUrlCheck(input);
    305 +  const resultUrlCheck = officialResultUrlCheck(input);
    306 +  const officialUrlEvidence = resultUrlCheck?.outcome === "verified" ? resultUrlCheck : null;
    307    const fileChecks = [parseC2paCheck(c2pa), parseExifToolCheck(exif), parseSeiCheck(sei)];
    305 -  const checks = officialUrlEvidence ? [officialUrlEvidence, ...fileChecks] : fileChecks;
    308 +  const checks = resultUrlCheck ? [resultUrlCheck, ...fileChecks] : fileChecks;
    309    const c2paEvidence = fileChecks[0];

  └ ~/projects/wt/ac-codexlb-provenance/scripts/video-status-poll.ts (+7 -0)
    363          { content: { not: { contains: "\"videoProvenanceVerification\"" } } },
    364 +        {
    365 +          AND: [
    366 +            { content: { contains: "\"provider\":\"byteplus-modelark\"" } },
    367 +            { content: { not: { contains: "\"verifier\":\"BytePlus official result URL\"" } } },
    368 +          ],
    369 +        },
    370        ],
        ⋮
    386          claimedModel: upstreamJob?.model ?? null,
    387 +        resultUrl: upstreamJob?.terminalSourceUrl ?? null,
    388        });

• Ran node --import tsx --test packages/contracts/video-provenance-verification.test.ts packages/generation/application/provider-video-job-metadata.test.ts packages/generation/application/
  │ overseas-seedance-provenance.test.ts && git diff --check && node scripts/check-autodev-integrity.mjs
  └ ✔ video provenance public contract keeps stable lifecycle and verdict values (1.188875ms)
    ✔ a returned WAN3 result URL immediately creates a preliminary model judgment (1.605625ms)
    … +104 lines (ctrl + t to view transcript)

    ✓ 全部完整性契约通过。

• Waited for background terminal · npm run build

• Ran 2 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，结论不是单纯“没告诉 T3”：旧交接文档确实让 T3只认文件级证据，所以 T3按旧规则显示了；同时 T7 还存在 2.0 与 null 比较的错误。现在两处都已修正，历史记录也纳入自动重检。

  按你前面“提交和部署”的授权，我现在做精确提交、非强推，并用标准脚本部署 EC2 与 HYPER；不会碰 T3 UI 文件。

• Ran 3 commands · ctrl + t to view transcript

• Waited for background terminal · node scripts/check-autodev-integrity.mjs && npm run check:satellite-drift && node --import tsx --test packages/contracts/video-provenance-verification.test.ts packages/
generation/application/provider-video-job-metadata.test.ts packages/generation/application/overseas-seedance-provenance.test.ts && npm run build

• Ran 2 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，修复已提交并推送到主干：10d15c76a。本地构建、22 项来源契约测试、卫星漂移和自动开发完整性闸全部通过。

  现在进入标准部署阶段；部署完成后我会等历史回填跑一轮，再核对截图这类记录是否从旧矛盾结论更新。

• Ran sed -n '1,220p' ~/.ssh/config 2>/dev/null || true && rg -n "deploy\.sh|creator\.ashot\.live|ubuntu@" ~/.zsh_history 2>/dev/null | tail -30
  └ # Added by OrbStack: 'orb' SSH host for Linux machines
    # This only works if it's at the top of ssh_config (before any Host blocks).
    … +39 lines (ctrl + t to view transcript)
    634:  ssh -N -p 2222 -i ~/ASHOT@2026.pem -L 127.0.0.1:9000:127.0.0.1:9000 ubuntu@3.82.32.89\
    998:    -J ubuntu@3.82.32.89:2222 ubuntu@172.31.26.215\

• Waited for background terminal · ssh -p 2222 -i /Users/justinopenclaw/ASHOT@2026.pem -o StrictHostKeyChecking=accept-new ubuntu@3.82.32.89 'cd /var/www/creator.ashot.live/ashot-creator-mvp-v0.3.1 && bash ops/
deploy.sh'

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，EC2 已拉到 10d15c76a，数据库契约与部署前检查全部通过，正在完成生产构建。历史重检会由已部署的轮询 worker 每分钟限量处理，旧记录不会永久停留在错误结论。

• Waited for background terminal · ssh -p 2222 -i /Users/justinopenclaw/ASHOT@2026.pem -o StrictHostKeyChecking=accept-new ubuntu@3.82.32.89 'cd /var/www/creator.ashot.live/ashot-creator-mvp-v0.3.1 && bash ops/
deploy.sh'

• Ran 5 commands · ctrl + t to view transcript

• Waited for background terminal · ssh -i /Users/justinopenclaw/ASHOT@2026.pem -J ubuntu@3.82.32.89:2222 -o StrictHostKeyChecking=accept-new ubuntu@172.31.26.215 'cd /home/ubuntu/ashot && bash deploy-hyper.sh'

• Ran ssh -i /Users/justinopenclaw/ASHOT@2026.pem -J ubuntu@3.82.32.89:2222 -o StrictHostKeyChecking=accept-new ubuntu@172.31.26.215 'cd /home/ubuntu/ashot && bash deploy-hyper.sh'
  └ ==> waiting up to 10 minutes for the HYPER deployment lock
    ==> HYPER deploy @ 2026-08-30 05:56:44  dir=/home/ubuntu/ashot scope=full
    … +27 lines (ctrl + t to view transcript)
    ==> HYPER deploy done.  CLIProxy(8317-9000) 是独立二进制,本脚本不动。
        cron 改动:在 HYPER 'crontab -e';它们走 tsx,git 同步后下次执行即新码。

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，EC2、Creator、Admin 和 HYPER 均已部署到 10d15c76a，健康检查全部通过。现在只剩最后一项线上验证：观察 HYPER 的来源回填任务是否已把旧 BytePlus 记录按新二级规则重新检测。

• Ran 3 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，线上历史回填已经实际运行，不是只部署代码。HYPER 日志确认：

  - oversea45 旧记录已重检为 verified
  - oversea5 旧记录已重检为 verified
  - oversea55 原先错误的 not_verified 已重检为 verified
  - 新结果不再出现 Seedance 2.0 与 null 不一致

  我再做最终版本与工作树核对后交付。

• Ran 4 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，不是 T3 读取不到，而是 T7 之前写入了错误结论，并且旧交接规则只强调文件扫描。现在已经修复并部署。

  1. Task completed

  - 第一层：结果 URL 命中 BytePlus 官方 HTTPS 域名，直接判定“官方线路来源已验证”。
  - 第二层：非官方 URL 再检查 C2PA、ExifTool、SEI。
  - URL 和三项扫描均无证据时，判定“未检测到来源”。
  - 修复 Seedance 2.0 与 null 不一致 的错误。
  - 历史 BytePlus 记录会自动重新检测。

  2. Changed files

  - /Users/justinopenclaw/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.ts
  - /Users/justinopenclaw/projects/wt/ac-codexlb-provenance/packages/generation/application/overseas-seedance-provenance.test.ts
  - /Users/justinopenclaw/projects/wt/ac-codexlb-provenance/scripts/video-status-poll.ts
  - /Users/justinopenclaw/projects/wt/ac-codexlb-provenance/docs/T7_TO_T3_VIDEO_PROVENANCE_MONITOR_HANDOFF_2026-08-24.md

  3. Commands run

  - 22 项来源验证与元数据契约测试
  - 自动开发完整性检查
  - 卫星漂移检查
  - npm run build
  - Git 精确提交、rebase、非强制推送
  - EC2 ops/deploy.sh
  - HYPER deploy-hyper.sh
  - 生产健康及历史回填日志核验

  4. Build result

  本地及生产构建通过，仅有仓库原有 lint 警告。

  5. E2E result

  未运行完整 Playwright E2E；相关契约测试 22/22 通过。

  6. Product flow status

  - 提交：10d15c76a
  - GitHub、EC2、Creator、Admin、HYPER 均为该版本。
  - Creator/Admin 健康检查均 ok=true。
  - HYPER 已实际把多个 4.5/5/5.5 阶历史记录重新判定为 verified。
  - 工作树干净，HEAD 与 origin/main 一致。

  7. Known limitations

  T3 当前会正确显示绿色“来源已验证”和新 summary，但详情弹层仍叫“查看三项校验”，尚未把 provider_signature 单独列为第一行。T3 可按交接文档改成“查看来源校验”，然后展示“官方 URL + 三项文件扫描”。

  官方域名目前严格限定为 HTTPS bytepluses.com 及其子域，与 BytePlus ModelArk 官方端点和素材域格式一致；相似伪域名不会通过。BytePlus ModelArk API 文档 (https://docs.byteplus.com/api/docs/ModelArk/1520757)

  8. Recommended next task

  让 T3读取更新后的交接文档，把 provider_signature 显示为“官方 URL 验证”第一行；T7数据已经准备好。

─ Worked for 24m 20s ─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────


› 继续推进


• 爸爸，我继续做最后闭环：先精确核对截图中的任务 cgt-20260830132649-x7cnr 是否已经被历史回填改正，再把 T3 仍需展示的 provider_signature 字段整理成可直接执行的交接项。T3 UI 文件属于 T3，我不会跨工作树代改。

• Ran ssh -i /Users/justinopenclaw/ASHOT@2026.pem -J ubuntu@3.82.32.89:2222 ubuntu@172.31.26.215 "cd /home/ubuntu/ashot && timeout 20 bash -lc 'set -a; source .env; set +a; node -e \"const {PrismaClient}
  │ =require(\\\"@prisma/client\\\");const p=new PrismaClient();p.workspaceAsset.findFirst({where:{content:{contains: \\\"cgt-20260830132649-x7cnr\\\"}},select:
  │ {id:true,provider:true,content:true}}).then(a=>{if(!a){console.log(\\\"not-found\\\");return;}const c=JSON.parse(a.content||\\\"{}\\\");const v=c.videoProvenanceVerification;const
  │ … +3 lines
  └ bash: -c: line 1: syntax error near unexpected token `('
    bash: -c: line 1: `set -a; source .env; set +a; node -e "const {PrismaClient}=require(\"@prisma/client\");const p=new PrismaClient();p.workspaceAsset.findFirst({where:{content:{contains:
    \"cgt-20260830132649-x7cnr\"}},select:{id:true,provider:true,content:true}}).then(a=>{if(!a){console.log(\"not-found\");return;}const c=JSON.parse(a.content||\"{}\");const
    v=c.videoProvenanceVerification;const u=c.upstreamVideoJob?.terminalSourceUrl;console.log(JSON.stringify({id:a.id,provider:a.provider,resultHost:u?new
    URL(u).hostname:null,verdict:v?.verdict,summary:v?.summary,checks:v?.checks?.map(x=>({method:x.method,outcome:x.outcome,provider:x.provider,model:x.model}))},null,2));}).finally(()=>p.\"\\"())"'

⚠ Exceeded skills context budget. All skill descriptions were removed and 24 additional skills were not included in the model-visible skills list.

• Context compacted

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，我继续按 systematic-debugging 做精确闭环：先核对截图任务 cgt-20260830132649-x7cnr 在线上的最新验证记录，再确认 T7 DTO 与 T3 展示之间还差哪一项。当前只做只读检查，不跨域修改 T3 界面。

• Ran sed -n '1,240p' /Users/justinopenclaw/.agents/skills/systematic-debugging/SKILL.md && sed -n '1,180p' PROJECT_STATE.md && sed -n '1,160p' ROADMAP.md && sed -n '1,160p' TASK_QUEUE.md && git status --short
  │ --branch
  └ ## wt/codexlb-prompt-switch...origin/main [behind 170]
     M .env.example
    … +4 lines (ctrl + t to view transcript)
     M src/lib/rshot/companion-media.ts
    ?? docs/ASHOT_AI_VIDEO_PROFESSIONAL_FINISHING_AND_PROVENANCE_SOP.md


› 你提交和部署了吗


• 爸爸，已经提交、推送并部署过。我现在再做一次只读复核，确认 GitHub、EC2 和 HYPER 三处仍是同一提交，避免只凭上一轮日志回答你。

• Ran git status --short --branch && git rev-parse HEAD && git ls-remote origin refs/heads/main | awk '{print $1}' && ssh -o BatchMode=yes -o ConnectTimeout=10 -i /Users/justinopenclaw/ASHOT@2026.pem
  │ ubuntu@3.82.32.89 'cd /var/www/ashot-creator-mvp && git rev-parse HEAD && curl -fsS http://127.0.0.1:3000/api/health' && ssh -o BatchMode=yes -o ConnectTimeout=10 -i /Users/justinopenclaw/ASHOT@2026.pem
  │ -J ubuntu@3.82.32.89:2222 ubuntu@172.31.26.215 'cd /home/ubuntu/ashot && git rev-parse HEAD && curl -fsS http://127.0.0.1:3000/api/health'
  └ ## wt/codexlb-provenance...origin/main
    10d15c76a6134d636488005f9393b4469cb8ec8b
    10d15c76a6134d636488005f9393b4469cb8ec8b
    Connection closed by 3.82.32.89 port 22

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 本地与 GitHub 已确认一致，都是 10d15c76a。刚才 EC2 的默认 22 端口被关闭，我改用生产机实际 SSH 端口 2222 继续核对线上版本与健康状态；这不是部署失败。

• Ran ssh -o BatchMode=yes -o ConnectTimeout=10 -p 2222 -i /Users/justinopenclaw/ASHOT@2026.pem ubuntu@3.82.32.89 'cd /var/www/ashot-creator-mvp && git rev-parse HEAD && curl -fsS
  │ http://127.0.0.1:3000/api/health' && ssh -o BatchMode=yes -o ConnectTimeout=10 -i /Users/justinopenclaw/ASHOT@2026.pem -J ubuntu@3.82.32.89:2222 ubuntu@172.31.26.215 'cd /home/ubuntu/ashot && git
  │ rev-parse HEAD && curl -fsS http://127.0.0.1:3000/api/health'
  └ bash: line 1: cd: /var/www/ashot-creator-mvp: No such file or directory

• Ran 2 commands · ctrl + t to view transcript
• Ran ssh -o BatchMode=yes -o ConnectTimeout=10 -p 2222 -i /Users/justinopenclaw/ASHOT@2026.pem ubuntu@3.82.32.89 'cd /var/www/creator.ashot.live/ashot-creator-mvp-v0.3.1 && git rev-parse HEAD && curl -fsS
  │ https://creator.ashot.live/api/health' && ssh -o BatchMode=yes -o ConnectTimeout=10 -i /Users/justinopenclaw/ASHOT@2026.pem -J ubuntu@3.82.32.89:2222 ubuntu@172.31.26.215 'cd /home/ubuntu/ashot && git
  │ rev-parse HEAD && curl -fsS http://127.0.0.1:3000/api/health'
  └ 10d15c76a6134d636488005f9393b4469cb8ec8b
    {"ok":true,"service":"ashot-creator","version":"unknown","sha":"10d15c76a","builtAt":"2026-08-30T05:49:51Z","env":"production","appEnv":"production","time":"2026-08-30T06:05:22.564Z","checks":{"database":
    … +5 lines (ctrl + t to view transcript)
    {"status":"ok"},"post_production_worker":{"status":"ok"},"app_env":{"status":"ok"}}}10d15c76a6134d636488005f9393b4469cb8ec8b
    curl: (7) Failed to connect to 127.0.0.1 port 3000 after 0 ms: Couldn't connect to server

• Ran 3 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，已完成提交、推送和部署。

  1. Task completed
     两层来源验证逻辑已上线：官方 URL 优先判定；否则执行 C2PA、ExifTool、SEI 三项扫描。

  2. Changed files
     来源验证模块、轮询脚本、测试及 T3 交接文档。

  3. Commands run
     定向测试、构建、完整性检查、Git 推送、EC2/HYPER 部署及健康检查。

  4. Build result
     构建通过。

  5. E2E result
     22/22 相关测试通过；未运行完整 Playwright E2E。

  6. Product flow status
     提交：10d15c76a
     GitHub main、EC2、HYPER 均为该提交。Creator 健康检查 ok=true；HYPER Control API healthy + ready。

  7. Known limitations
     T3 顶层已经能读取验证结论，但详情目前仍只固定展示 C2PA、ExifTool、SEI，尚未单独展示第一层 provider_signature / 官方 URL。这属于 T3 UI 文件所有权范围。

  8. Recommended next task
     让 T3 按交接文档把“查看三项校验”改为“查看来源校验”，并在三项扫描之前展示“官方 URL 验证”。相关 T7 工作树当前干净。

─ Worked for 3m 21s ──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────


› 我们之前接入万象一刻的时候，似乎需要很多种KEY和资料，才能接入的对吗


• 爸爸，我核对一下仓库里万象一刻（Yike）的实际鉴权与配置项，区分“调用接口必需的凭证”和“商务开通/回调等辅助资料”，避免把历史测试配置混在一起。

• Ran 2 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，对，但需要区分：万象一刻本身的核心鉴权并不复杂，真正必需的是：

  - 阿里云 RAM AccessKey ID
  - RAM AccessKey Secret
  - RAM 账号权限，历史上使用 AliyunYikeFullAccess
  - 万象一刻主账号会员资格，至少 Standard
  - 新加坡区域：ap-southeast-1
  - 接口地址：yike.ap-southeast-1.aliyuncs.com

  之所以当时感觉需要很多 Key，是因为几条线路曾被拆开配置：

  - 2.5/3/3.5/4 阶使用 YIKE_ACCESS_KEY_ID/SECRET
  - 4.5/5/7 阶 Wonder 系列曾使用独立的 WONDER_YIKE_ACCESS_KEY_ID/SECRET
  - WAN3.0 还短暂走过 Model Studio，因此另外需要过：
      - Workspace API Key
      - Workspace ID
      - Workspace 专属 Host

  另外还有一些不是 Key、但开通时必须具备的资料：

  - 模型权限和会员套餐
  - 账户余额/积分
  - 模型灰度白名单
  - 素材 ImportMedia 登记权限
  - 创建、查询、计费等接口权限
  - HYPER 上的线路启用开关

  所以准确结论是：万象一刻纯接口调用主要是一组 RAM AK/SK；此前资料很多，是因为同时存在多条模型线路、独立凭证域、Model Studio 迁移路线以及素材登记、会员和灰度验收，并不代表每次调用都要携带很多种 Key。

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────


› 对接的 API 和URL 又是哪些呢


• 爸爸，我按当前服务端适配器整理一份准确清单，把万象一刻 RPC、素材登记、任务查询和历史 Model Studio 地址分开，避免把两套协议混用。

• Ran 2 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，万象一刻对接主要分为“上游 Yike 接口”和“ASHOT 对外接口”两层。

  ### 一、万象一刻上游接口

  统一地址：

  https://yike.ap-southeast-1.aliyuncs.com

  基础配置：

  Region: ap-southeast-1
  API Version: 2026-07-07
  鉴权: 阿里云 RAM AK/SK 签名

  这是阿里云 RPC 风格接口，通过 SDK 的 Action 调用，不应自行拼接 /SubmitVideoGenerationJob 之类的 REST 路径。

  主要 Action：

   用途                      Action
  ━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━
   创建视频任务              SubmitVideoGenerationJob
  ────────────────────────  ──────────────────────────
   查询任务状态与结果        GetVideoGenerationJob
  ────────────────────────  ──────────────────────────
   查询账户会员与额度        GetYikeAccountCredit
  ────────────────────────  ──────────────────────────
   查询单个任务计费          GetYikeJobCredit
  ────────────────────────  ──────────────────────────
   登记参考图片/视频/音频    ImportMedia
  ────────────────────────  ──────────────────────────
   查询素材登记状态          GetMedia

  其中 Wonder-Standard、Wonder-Pro、Wonder-Ultra 使用参考素材时，链路是：

  ImportMedia
  → GetMedia 轮询 ThirdPartyAssetStatus
  → SubmitVideoGenerationJob
  → GetVideoGenerationJob
  → GetYikeJobCredit
  → 下载临时 OutputUrl
  → 转存 ASHOT S3

  ### 二、上游模型名

  happyhorse-1.0
  wan2.7
  happyhorse-1.1
  wan3.0-video
  Wonder-Standard
  Wonder-Pro
  Wonder-Ultra

  创建任务时，通过 Model 参数区分，不是每个模型使用不同 URL。

  ### 三、ASHOT 原生对外 API

  创建：

  POST https://creator.ashot.live/api/creator/v1/video-generations
  Authorization: Bearer ask_live_xxx

  查询：

  GET https://creator.ashot.live/api/creator/v1/video-generations/{generationId}
  Authorization: Bearer ask_live_xxx

  机器可读模型和参数文档：

  GET https://creator.ashot.live/api/creator/v1/video-generations/schema

  ### 四、ASHOT 兼容格式

  OpenAI 风格：

  POST /v1/videos
  GET  /v1/videos/{videoId}
  GET  /v1/videos/{videoId}/content

  BytePlus/ModelArk 风格：

  POST /api/v3/contents/generations/tasks
  GET  /api/v3/contents/generations/tasks/{taskId}

  这几套 ASHOT 接口最终进入同一条任务、计费、队列、轮询和成片转存链路。

  ### 五、历史 Model Studio 地址

  WAN3.0 曾短暂使用过另一套 Model Studio Workspace 接口：

  POST https://{workspaceId}.ap-southeast-1.maas.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis
  GET  https://{workspaceId}.ap-southeast-1.maas.aliyuncs.com/api/v1/tasks/{task_id}

  这套需要 Workspace ID 和 Workspace API Key，不能与万象一刻 RAM AK/SK 混用。现在讨论万象一刻接入时，权威上游地址是：

  https://yike.ap-southeast-1.aliyuncs.com

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────


› 我们发过去的参数，也是这样的吗### 二、上游模型名

    happyhorse-1.0
    wan2.7
    happyhorse-1.1
    wan3.0-video
    Wonder-Standard
    Wonder-Pro
    Wonder-Ultra


• 爸爸，我直接核对当前 origin/main 的实际路由映射和提交请求，确认这些字符串是不是原样发给上游，而不是只看旧文档。

• Ran 3 commands · ctrl + t to view transcript

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

• 爸爸，不完全是。刚核对了当前 origin/main，实际发送分成两套。

  ### 万象一刻 Yike

  以下 4 个模型名会作为 SubmitVideoGenerationJob 请求的 model 字段原样发送：

   ASHOT 阶位    线路            实际发送的 model
  ━━━━━━━━━━━━  ━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━
   2.5阶         happyhorse      happyhorse-1.0
  ────────────  ──────────────  ──────────────────
   3阶           wan27           wan2.7
  ────────────  ──────────────  ──────────────────
   3.5阶         happyhorse11    happyhorse-1.1
  ────────────  ──────────────  ──────────────────
   4阶           wan30           wan3.0-video

  请求结构大致是：

  {
    "model": "wan3.0-video",
    "jobType": "reference_to_video",
    "scene": "general",
    "input": "{\"Prompt\":\"...\",\"Medias\":[...]}",
    "jobParameters": "{\"EnableAudio\":true}",
    "resolution": "720P",
    "aspectRatio": "16:9",
    "duration": "10",
    "n": 1
  }

  ### 海外 Seedance 当前走 BytePlus

  4.5/5/5.5/7 阶现在不是向万象一刻发送 Wonder-*，而是向 BytePlus ModelArk 发送以下模型名：

   ASHOT 阶位          实际发送的 model
  ━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   4.5阶 Mini          dreamina-seedance-2-0-mini-260615
  ──────────────────  ───────────────────────────────────
   5阶 Fast            dreamina-seedance-2-0-fast-260128
  ──────────────────  ───────────────────────────────────
   5.5阶 Standard      dreamina-seedance-2-0-260128
  ──────────────────  ───────────────────────────────────
   7阶 Seedance 2.5    dreamina-seedance-2-5-260628

  地址是：

  POST https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks
  GET  https://ark.ap-southeast.bytepluses.com/api/v3/contents/generations/tasks/{taskId}

  请求体类似：

  {
    "model": "dreamina-seedance-2-0-260128",
    "content": [
      {
        "type": "text",
        "text": "..."
      }
    ],
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 10,
    "generate_audio": true,
    "watermark": false
  }

  Wonder-Standard、Wonder-Pro、Wonder-Ultra 仍保留在旧万象一刻适配器中，用于历史任务和兼容逻辑，但当前海外 4.5/5/5.5/7 新任务不应该再把这些名称发送给上游。

  所以当前准确划分是：

  万象一刻：
  happyhorse-1.0
  wan2.7
  happyhorse-1.1
  wan3.0-video

  BytePlus：
  dreamina-seedance-2-0-mini-260615
  dreamina-seedance-2-0-fast-260128
  dreamina-seedance-2-0-260128
  dreamina-seedance-2-5-260628

──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────


› 万象一刻里的呢，这三个旧的线路呢，Wonder-Standard、Wonder-Pro、Wonder-Ultra，是什么参数传递的


• 爸爸，万象一刻旧的三条 Wonder 线路使用同一个 Yike RPC 接口，主要通过 model 字段区分。

  ### 模型参数

  {
    "model": "Wonder-Standard"
  }

  {
    "model": "Wonder-Pro"
  }

  {
    "model": "Wonder-Ultra"
  }

  大小写和连字符必须完全一致。

  ### 完整任务参数

  发送给 SubmitVideoGenerationJob 的结构是：

  {
    "model": "Wonder-Pro",
    "jobType": "reference_to_video",
    "scene": "general",
    "input": "{\"Prompt\":\"视频提示词\",\"Medias\":[{\"Type\":\"image\",\"MediaId\":\"media-xxx\"},{\"Type\":\"video\",\"MediaId\":\"media-yyy\"}]}",
    "jobParameters": "{\"EnableAudio\":true}",
    "resolution": "1080P",
    "aspectRatio": "16:9",
    "duration": "10",
    "n": 1,
    "clientToken": "ashot-task-id"
  }

  注意 input 和 jobParameters 都是序列化后的 JSON 字符串，不是直接嵌套对象。

  ### jobType 选择规则

   输入素材                  jobType
  ━━━━━━━━━━━━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━
   没有参考素材              text_to_video
  ────────────────────────  ────────────────────
   只有1张参考图             image_to_video
  ────────────────────────  ────────────────────
   首尾帧、恰好2张图片       first_last_frame
  ────────────────────────  ────────────────────
   多图片、视频或音频参考    reference_to_video

  ### 参考素材必须先登记

  Wonder 系列不能直接把外部 URL 放进生成请求。每个素材首先调用 ImportMedia：

  {
    "importSource": "url",
    "inputURL": "https://example.com/reference.jpg",
    "mediaType": "image",
    "title": "ASHOT Wonder Pro image reference",
    "registerConfig": "{\"NeedThirdPartyAsset\":true,\"NeedSnapshot\":true}"
  }

  然后通过 GetMedia 查询：

  {
    "mediaId": "media-xxx",
    "authTimeout": 3600
  }

  必须等到：

  ThirdPartyAssetStatus = Success

  生成请求才传入登记后的 MediaId：

  {
    "Type": "image",
    "MediaId": "media-xxx"
  }

  素材类型分别是：

  image
  video
  audio

  ### 三个模型的参数边界

   模型                 时长    分辨率         参考素材
  ━━━━━━━━━━━━━━━━━  ━━━━━━━━  ━━━━━━━━━━━━━  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   Wonder-Standard    4–15秒    720P、1080P    最多15个
  ─────────────────  ────────  ─────────────  ────────────────────────────────────
   Wonder-Pro         4–15秒    720P、1080P    最多15个
  ─────────────────  ────────  ─────────────  ────────────────────────────────────
   Wonder-Ultra       4–30秒    720P、1080P    图片最多30、视频10、音频10，总计50

  画幅支持：

  16:9
  9:16
  1:1
  4:3
  3:4

  ### 音频与水印

  音频开关放在：

  {
    "jobParameters": "{\"EnableAudio\":true}"
  }

  现有旧 Wonder 适配器没有向万象一刻传递独立的 watermark 参数。也就是说，它不像 BytePlus 请求那样发送：

  {
    "watermark": false
  }

  ### 接口与鉴权

  统一上游地址：

  https://yike.ap-southeast-1.aliyuncs.com

  使用的 Action：

  ImportMedia
  GetMedia
  SubmitVideoGenerationJob
  GetVideoGenerationJob
  GetYikeAccountCredit
  GetYikeJobCredit

  鉴权配置：

  WONDER_YIKE_ACCESS_KEY_ID
  WONDER_YIKE_ACCESS_KEY_SECRET
  WONDER_YIKE_ENDPOINT


