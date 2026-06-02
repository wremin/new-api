# Bug Audit Report

Scope: full project (Go backend + React frontend) on branch `metamind`.
Depth: all real bugs (high + medium + minor logic), excluding pure style/nits.

Totals: **~46 distinct real bugs** — 11 high, ~15 medium, ~20 low — plus two
codebase-wide convention-violation categories (Rule 1 / Rule 6) listed at the end.

Items marked **[FIXED]** were addressed in follow-up commits. The WeChat Pay
(`controller/topup_wechat.go`, `model/topup.go` WeChat path) and enterprise
(`controller/enterprise.go`, `model/enterprise.go`) issues fixed in earlier
passes are not relisted here.

---

## High severity

### Money / quota

1. **[FIXED]** **Stripe over-credit in TOKENS display mode** — `controller/topup_stripe.go` `GetChargedAmount` + `model/topup.go` `Recharge`.
   Stored `Money` was not divided by `QuotaPerUnit` in token display mode, but credited quota is `Money × QuotaPerUnit`. → user received ~500,000× the quota paid for. Fix: `GetChargedAmount` now divides by `QuotaPerUnit` in TOKENS mode, mirroring `getStripePayMoney`.

2. **[DEFERRED]** **Stripe webhook never verifies the paid amount** — `controller/topup_stripe.go` `fulfillOrder` (~256-286).
   `amount_total`/`currency` are only logged, never compared. NOT fixed: the actual charge is determined by the provider-side `StripePriceId` × quantity, so there is no locally-stored expected charge to compare against. A correct fix requires storing the expected `amount_total`+currency at session creation, then matching in the webhook. Needs a schema/flow change + testing; do not ship a guess that could reject valid payments.

3. **[DEFERRED]** **Creem webhook never verifies the paid amount** — `controller/topup_creem.go` `handleCheckoutCompleted` (~287-365).
   Same situation as Stripe: Creem prices live in the provider-side product config; `order.AmountPaid` unit/currency isn't reliably comparable to a locally-stored value. Needs the same expected-amount-at-creation approach + testing before it's safe.

4. **[FIXED]** **`Recharge` credits quota with float math** — `model/topup.go` `Recharge`.
   `quota = topUp.Money * common.QuotaPerUnit` (float64) → now computed with `decimal` + `IntPart()`, consistent with the other recharge paths.

### Auth

5. **[FIXED]** **2FA login bypasses the banned-user check** — `controller/twofa.go` `Verify2FALogin`.
   Added a `user.Status != common.UserStatusEnabled` check after fetching the user; banned users are now rejected before `setupLogin`.

### Database / cross-DB (project must support SQLite + MySQL ≥5.7.8 + PostgreSQL ≥9.6)

6. **`SELECT ... FOR UPDATE` on SQLite** — `model/subscription.go` (523,618,725,770,993,1066,1110,1177), `model/redemption.go:130`, `model/user.go:357`, `model/topup.go`, `model/enterprise.go`.
   SQLite rejects `FOR UPDATE` syntax. ⚠️ This pattern is used pervasively (incl. existing payment code); either SQLite payments are already broken or GORM strips it. **Verify on a real SQLite build before mass-changing.** Suggested fix: `clause.Locking{Strength:"UPDATE"}` guarded by `!common.UsingSQLite`.

7. **[FIXED]** **`SumUsedToken` uses `ifnull(...)`** — `model/log.go:440`.
   `ifnull` is MySQL/SQLite-only. Replaced with `coalesce`, which is supported by all three databases.

8. **[FIXED]** **`channel_cache.go` nil-pointer panic + stray debug** — `model/channel_cache.go` `CacheUpdateChannel`.
   Removed the three `println` debug statements, including the one dereferencing a possibly-nil `*Channel`.

### Frontend

9. **[FIXED]** **WeChat QR poll TDZ** — `web/src/components/topup/index.jsx`.
   Reordered so `modalInstance` is created before the poll callback/`setInterval`; added a 10-minute safety timeout that also stops the poll.

10. **Playground SSE never closed on unmount** — `web/src/hooks/playground/usePlaygroundState.js:219-225`.
    Cleanup clears the save-config timeout but never closes `sseSourceRef`; navigating away mid-stream leaks the connection and writes state after unmount. (Not yet fixed — needs care around the SSE abort API.)

11. *(reserved — see #8 split; counted as one high group.)*

---

## Medium severity

- **[FIXED]** **Alipay callback never verifies the paid amount** — `controller/topup_alipay.go` `AlipayNotify`. Added a decimal comparison of `total_amount` (yuan) against `topUp.Money` before crediting; mismatch returns `fail`. (Also removed the unreachable `TRADE_FINISHED` branch in the else-if.)
- **[FIXED]** **`GetUser` admin endpoint leaks other users' `access_token`** — `controller/user.go`. The response now nils out `AccessToken`. (`setting` left intact to avoid breaking admin UI; revisit if it carries secrets.)
- **CORS `AllowAllOrigins` + `AllowCredentials`** — `middleware/cors.go:9-15`. Any origin with credentials, and no CSRF tokens on state-changing `selfRoute` endpoints.
- **SSRF DNS-rebinding / TOCTOU** — `common/ssrf_protection.go:208-286` `ValidateURL`. Host is resolved+checked here but re-resolved on the actual fetch. *(uncertain: depends on whether callers pin the resolved IP.)*
- **Redemption redeem doesn't sync Redis quota cache** — `model/redemption.go:115-156`. Cached quota stays stale until TTL.
- **Creem signature comparison is encoding-fragile** — `controller/topup_creem.go:38-50`. Compares raw header bytes to hex digest with no normalization; `CreemTestMode` fully bypasses verification.
- **Rule 6 violations on active paths** (explicit client `0`/`false` dropped on re-marshal upstream):
  - `relay/channel/baidu/dto.go:18` `TopP float64,omitempty`
  - `relay/channel/baidu/dto.go:19` `PenaltyScore float64,omitempty`
  - `relay/channel/zhipu/dto.go:17` `TopP float64,omitempty`
  - `relay/channel/aws/dto.go:23` `TopP float64,omitempty` (AwsClaudeRequest)
  - `relay/channel/aws/dto.go:24` `TopK int,omitempty` (AwsClaudeRequest)
  - `dto/embedding.go:12,14,18,19` `Seed/TopK/NumPredict/NumCtx int,omitempty`
- **Frontend `getTopupInfo` stale closure** — `web/src/components/topup/index.jsx:597`. Reads `topupInfo.amount_options.length` from a stale closure.
- **Frontend `onlineTopUp` async amount branch** — `web/src/components/topup/index.jsx:209-225`. Relies on async state that won't have updated; misleading/dead branch.
- **`DocumentRenderer.sanitizeHtml` does not sanitize** — `web/src/components/common/DocumentRenderer/index.jsx:50-62,197-208`. Misleading name; re-injects raw HTML via `dangerouslySetInnerHTML`. Admin-set content today.

---

## Low severity

- Preset `AmountDiscount` looked up by raw `int(amount)` — never matches in TOKENS mode (`controller/topup.go:191`, `topup_stripe.go:411`, `topup_waffo.go:87`, alipay via getPayMoney).
- Dead branch `TRADE_FINISHED` already matched earlier — `controller/topup_alipay.go:218`.
- Inconsistent `<=0.01` vs `<0.01` boundary — `controller/topup.go:438` vs `:232`.
- Float truncation in task-quota settlement — `service/text_quota.go` / `service/task_billing.go:297`.
- Possible negative prompt-token math for OpenRouter Claude cache tokens — `service/text_quota.go:81-130` *(uncertain)*.
- Creem failure returns 500 indistinguishably from a real mismatch — `controller/topup_creem.go:340`.
- `GenerateAccessToken` collision-check mutates the `user` struct — `controller/user.go:289-322` (brittle, not exploitable).
- Telegram login has no `auth_date` freshness check (replayable) — `controller/telegram.go:101-125`.
- IP-keyed rate limiting evadable via `X-Forwarded-For` if proxy trust is permissive — `middleware/rate-limit.go`.
- SSE framing edge: bare `[DONE]` line mishandled — `relay/helper/stream_scanner.go:244-250`.
- `GetAllLogs` `model_name like ?` without escaping `%`/`_` — `model/log.go:255` (admin only).
- Cache race: async `updateUserCache` can overwrite a concurrent `CacheIncrUserQuota` — `model/user_cache.go:79-112` *(bounded by TTL)*.
- `RedisIncr` silently no-ops when key has no TTL — `common/redis.go:242-272` *(by design, easy to misuse)*.
- Frontend: missing `try/catch` around several `await API...` (unhandled rejections, stuck loading buttons) — `topup/index.jsx:624-652,433-441,704-728`, `auth/PasswordResetForm.jsx:91-104`.
- Frontend: countdown intervals recreated every tick (drift) — `auth/RegisterForm.jsx:158-169`, `PasswordResetForm.jsx:65-76`, `settings/PersonalSetting.jsx:135-146`.
- Frontend: multiple `dangerouslySetInnerHTML` on admin/server content (XSS only if those fields ever take untrusted input) — Announcements/Faq/Notice/Footer/Home/About panels, `helpers/utils.jsx:32`.
- Frontend: direct mutation of API response array in `useDashboardData.js:182-185`.
- Frontend: generic-payment form built from `params` without type check — `topup/index.jsx:316-334`.

---

## Codebase-wide convention categories (not counted in the totals above)

### Rule 1 — direct `encoding/json` instead of `common.*` wrappers
~100+ call sites across 30+ files. Functionally work; violate project policy. Representative areas:
- Payments: `controller/topup_creem.go`, `common/topup-ratio.go`
- Auth/OAuth: `controller/user.go`, `controller/wechat.go`, `oauth/*.go`, `service/passkey/session.go`
- Models: `model/channel.go:228`, `model/user.go:83,92,153`, `model/pricing.go:211,255`, `model/passkey.go:49,68`, `model/prefill_group.go:47`
- Relay: ~90 sites across `relay/channel/*` (openai, gemini, baidu, coze, cohere, ollama, cloudflare, dify, zhipu, xunfei, aws, vertex, tencent, minimax, volcengine, palm, jimeng, mokaai, ali), `relay/helper/*`, `dto/openai_request.go:450,457`

### Rule 6 — additional non-pointer optional scalars (legacy/guarded adapters)
`relay/channel/ali/dto.go:29-31,209`, `relay/channel/palm/dto.go:23-24`, `relay/channel/xunfei/dto.go:18-19`, `relay/channel/cloudflare/dto.go:8`, `relay/channel/aws/dto.go:21,75-78`.

---

## Feature: provisioning an enterprise account (gap closed)

**Background.** An "enterprise account" is just a user whose role is
`RoleEnterpriseAdmin = 5` (`common/constants.go`). That role unlocks the
"企业" sidebar menu (`isEnterpriseAdmin()`, role === 5) and the
`/api/enterprise/*` endpoints (sub-account CRUD, quota allocation, usage stats),
which are gated by `EnterpriseAuth()` (role ≥ 5).

**The gap (before this change).** There was no UI or admin action that set a
user to role 5:
- `ManageUser`'s `promote`/`demote` only toggle between common (1) and admin (10).
- The Edit User modal exposes only group/quota, no role selector.
- `renderRole` knew only 1/10/100, so role 5 rendered as "未知身份".

**Fix (method C) — how to provision now.**
1. Admin dashboard → Users list → target user's "更多" (More) menu →
   **设为企业管理员**.
2. This calls `manageUser(id, 'promote_enterprise')` → `POST /api/user/manage`.
3. The new `promote_enterprise` action in `ManageUser`
   (`controller/user.go`) sets the role to `RoleEnterpriseAdmin`. Guards: caller
   must be ≥ admin; rejects users who are already enterprise admins or who are
   already a higher admin (demote them to common first).
4. The user re-logs in; the 企业 menu appears and the enterprise APIs work.

To revert, use the existing **降级 (demote)** action (5 → 1).

**Files changed:** `controller/user.go` (new action),
`web/src/components/table/users/UsersColumnDefs.jsx` (role-5 label + menu item),
`web/src/components/table/users/UsersTable.jsx` (thread `manageUser` into columns).

**Operational note.** Allocating quota to sub-accounts deducts from the
enterprise admin's own balance (`AllocateQuota`), so after promoting, the
enterprise admin must be given quota (admin "add quota" or a normal top-up)
before they can distribute any.

**Frontend i18n:** the new strings `企业管理员` / `设为企业管理员` use the
project's `t()` source-string convention (display as-is in zh). Run
`bun run i18n:sync` to add other-language translations.
