# 用户侧：`/panel` 用户中心的订阅与注册

运营侧（`/panel/ops`）已经落地：账号处置、套餐目录、审计台三块都在 `main` 上，见 [10](./10-ops-console-design.md) / [11](./11-ops-console-handoff.md)。
本篇补的是**另一侧**——真实用户从注册到付费的这条路径。

结论先说：**这条路目前是断的，而且断在最贵的一环**。`/prices` 上「按量付费 ¥1/GB · 登录后充值」这张卡片，点进去落到 `/panel/subscription`，用户在那里看到的是 **USD 19 的 XConnect 流量包**和 **USD 49/月的 XConnect Pro**——两个价格、两种货币、两套 planId，没有一个能真正下单。

---

## 一、现状实测（2026-08-13，console-uat.onwalk.net）

### 1.1 线上目录只有两条，且都不可购买

```bash
curl -s https://console-uat.onwalk.net/api/billing/plans
```

```json
{"plans":[
  {"planId":"TRIAL-7D","displayName":"7-Day Trial","kind":"trial","includedQuotaBytes":10737418240,"trialDays":7,"active":true,"sortOrder":0},
  {"planId":"FREE","displayName":"Free","kind":"subscription","includedQuotaBytes":0,"trialDays":0,"active":true,"sortOrder":10}
]}
```

`PAYG`、`PRO-MONTHLY`、`PRO-YEARLY` **在 UAT 目录里根本不存在**，`stripePriceId` 也全部为空。
这与 [README 现状盘点](./README.md) 里记的一致，八天过去没有变化——**任务 #10「Seed billing_plans catalog with four tiers」在代码上完成了，但没有落到 UAT 数据库**。

### 1.2 `/prices` 与 `/panel/subscription` 是两套价格

| | 数据来源 | XConnect PAYG | XConnect Pro |
| --- | --- | --- | --- |
| `/prices` | `src/app/prices/page.tsx`，硬编码中英文案 + 目录只读 `stripePriceId`/`active` | ¥1/GB（按钮「登录后充值」→ `/panel/subscription`） | ¥20/月、¥200/年（目录缺失 → 显示「即将上线」） |
| `/panel/subscription` | `BillingOptionsPanel` 读 `src/modules/products/registry.ts` 的**静态** `PRODUCT_LIST` | `XCONNECT-PAYGO`，**USD 19**，一次性 | `XCONNECT-SUBSCRIPTION`，**USD 49/月** |

`XCONNECT-PAYGO` / `XCONNECT-SUBSCRIPTION` 这两个 planId 在 `billing_plans` 里从未存在过。
即使目录被正确 seed，`BillingOptionsPanel` 也**读不到**——它压根不查目录。

> 这是 [README 关键决策 2](./README.md#关键决策)「套餐目录是唯一权益事实源」在前端的一处直接违反。`/prices` 已经按这条决策改造过（只信目录的 `active`/`stripePriceId`），用户中心没有。

**登录实测（admin 账号，2026-08-13）比代码推断的更糟**：`/panel/subscription` 实际渲染的不是 2 张卡，是 **6 张**——`PRODUCT_LIST` 里三个产品各自的 paygo + saas：

| 卡片 | 标价 | 状态 |
| --- | --- | --- |
| XConnect 流量包 / XConnect Pro | USD 19.00 / USD 49.00 per month | 该套餐需要先配置 Stripe price_id |
| ScopeHub 数据查询包 / ScopeHub SaaS | USD 15.00 / USD 39.00 per month | 同上 |
| CloudFlow 任务包 / CloudFlow SaaS | USD 12.00 / USD 59.00 per month | 同上 |

**六张卡全部标 USD、全部不可购买、全部不在 `billing_plans` 目录里**，而 `/prices` 上对外报的是人民币。
其中 ScopeHub 与 CloudFlow 两个产品在 `/prices` 上根本没有对应卡片——用户中心在推销定价页上不存在的商品。

### 1.2b 登录后的真实账户状态

同一次实测，`SubscriptionPanel` 对已登录账号显示：

```
权威用量 0 B          月度配额 0.0%（已用 0 B / 0 B）    本期重置 —
余额 / 配额 0.00      剩余配额 0 B                       套餐 default，规则 —
状态 正常 / normal / active                              订阅记录：暂无订阅记录
```

`套餐 default` 说明 `account_billing_profiles` 里没有这个账号的行，`暂无订阅记录` 说明连 `TRIAL-7D` 都没有——与 [README 现状盘点](./README.md#现状盘点2026-08-05-实测) 记的两个 ❌ 完全吻合，**八天无变化**。

同时 `/panel/ops` 运营台已上线并能读到数据（`活跃订阅 2`、`2 个账号可检索`），但 `MRR` / `欠费金额` / `待处理事项` / `经营趋势` / `用量 TopN` 五个卡片均显示「待同步」——**聚合接口尚未接入**，这是运营侧遗留的独立缺口（不在本篇范围，记录在此以免被当成本次改动引入）。

### 1.3 目录里没有「价格」这个字段

`internal/store/store.go` 的 `BillingPlan`：

```go
type BillingPlan struct {
	PlanID, StripePriceID, DisplayName, Kind, PackageName string
	IncludedQuotaBytes int64
	PriceMultipliers   map[string]float64
	Features           map[string]any
	TrialDays          int
	Active             bool
	SortOrder          int
}
```

**没有 `price` / `currency` / `unit`**。金额的唯一事实源是 Stripe Price 对象，而前端展示的金额是三处各自硬编码的文案：`prices/page.tsx`、`products/registry.ts`、Stripe 后台。

运营在 `/panel/ops/billing/plans` 里「调整价格」，改的是 `stripePriceId` 的指向，**页面上那个 ¥20 不会跟着变**。这个落差必须在动 UI 之前拍板（见 §三-D1）。

### 1.4 充值（top-up）完全没有入口

后端已经通了：`api/billing_topup.go` 的 `creditTopUpBalance` 在 `checkout.session.completed` 的一次性支付分支里把金额记入 `billing_ledger` 并累加 `current_balance`，用确定性 UUID 作主键抗 webhook 重投（见 [09](./09-trial-grants-and-topup-gap.md)）。

前端**一个充值按钮都没有**。`/prices` 的「登录后充值」是一句空承诺。

### 1.5 试用状态对用户不可见

注册即发 `TRIAL-7D`（10 GB / 7 天）。但在 `src/modules/extensions/builtin/user-center/` 下 grep `试用`，只命中 `ops/` 目录下的运营组件——**用户自己看不到自己在试用期、还剩几天、到期会怎样**。

`SubscriptionPanel` 已经渲染了配额、余额、欠费标志和最近 20 条流水（读 `/api/account/billing/summary`），读的这一侧是好的；缺的是**试用/到期/续费这一层语义**。

### 1.6 OAuth 注册拿不到试用

`api/api.go:724` 的注释写得很清楚，代码也确实如此：

- 密码注册 → `api.go:707` 调 `provisionOnboardingTrial`
- OAuth（GitHub / Google）注册 → `EmailVerified=false`，**不发试用**；只有走完邮箱验证（`api.go:809`）才发

首页登录框上就摆着 GitHub / Google 两个按钮。用这两个入口进来的用户，落到 `/panel` 时**没有 billing profile、配额为 0**，而 `ServiceReadinessCard` 只把「验证邮箱」列为一个待办清单项，没有说明它和「能不能用」直接挂钩。

### 1.7 付款前强制 MFA

`api/stripe.go:420` / `:459`：

```go
if !user.MFAEnabled {
    respondError(c, http.StatusForbidden, "mfa_required", ...)
}
```

checkout 和 billing portal 都挡。安全上没问题，但它是**注册 → 付费漏斗上的一道硬门槛**，而现在的提示只有一句「绑定 MFA 后可支付」，没有引导链路。

---

## 二、缺口清单（按「离钱的距离」排序）

| # | 缺口 | 影响 | 位置 |
| --- | --- | --- | --- |
| G1 | 用户中心不读目录，6 张静态卡片全是 USD、全不可购买，含 2 个定价页没有的产品 | 用户看到与 `/prices` 不一致的报价与商品；下单必然失败 | `account/BillingOptionsPanel.tsx` |
| G2 | UAT 目录缺 `PAYG`/`PRO-MONTHLY`/`PRO-YEARLY` 与 `stripePriceId` | 三档付费全部不可购买 | UAT 数据 + Stripe |
| G3 | 无充值入口 | PAYG 档在控制台内**无法完成核心动作** | 用户中心新增 |
| G4 | 目录无价格字段 | 「调整价格」是假的；三处金额各自漂移 | `store.BillingPlan` + 前端 |
| G5 | 试用状态不可见、到期无预告 | 用户在第 8 天突然断流且不知为何 | `SubscriptionPanel` |
| G6 | OAuth 注册无权益 | 一整类注册入口的新用户开箱即坏 | `api/api.go` 注册链路 |
| G7 | 无升降档 / 月付↔年付切换 | 只有 `/subscriptions/cancel`，改档只能退订重买 | accounts + 用户中心 |
| G8 | 欠费/停机页面无自助恢复 | 只显示状态，不给「立即充值恢复」动作 | `SubscriptionPanel` |
| G9 | MFA 门槛无引导 | 付费转化在最后一步静默流失 | `PaymentMfaNotice` |

G1–G3 是**「用户拿钱买不到东西」**这一类，其余是体验与一致性。

---

## 三、决策（2026-08-13 已拍板）

> 五个决策均已确认，下方保留推导过程。**结论集中在这里**：
>
> | # | 结论 |
> | --- | --- |
> | D1 | 给 `billing_plans` 加 `price_amount`/`price_currency`/`price_unit`，挂牌价进目录、进审计 ✅ 已实现（accounts#75） |
> | D2 | 充值维持 Stripe 一次性 Checkout ✅ 与既有 `creditTopUpBalance` 一致，无需改动 |
> | D3 | **Free/试用档 = 7 天内 5GB 高速流量；用完不订阅即断流**，并发邮件引导订阅 |
> | D4 | OAuth 邮箱**已验证**即可直接发试用；provider 未返回已验证的，仍需走验证 |
> | D5 | 收窄到目录里存在的商品，ScopeHub / CloudFlow 移除 ✅ 已实现（portal#218） |
>
> D3 有两处待改：`TRIAL-7D.included_quota_bytes` 目前是 **10GB**，须改为 **5GB**（5368709120）；`/prices` 的 Free 卡片文案「每周 1 小时高速流量，用完降级 VPS」与「7 天 5GB、之后断流」不符，须一并改写——注意结论是**断流**而不是降级到 VPS。

## 附：决策推导过程

### D1 · 价格的事实源放哪里？（阻塞 G1/G4，其余都等它）

三个选项：

| 方案 | 做法 | 代价 |
| --- | --- | --- |
| **A（推荐）** | 给 `billing_plans` 加 `price_amount`(int, 最小货币单位) / `price_currency` / `price_unit`(`month`/`year`/`GB`/`once`)，目录成为展示价的唯一来源；Stripe Price 仍是**实际扣款**来源，两者不一致时前端以目录为准展示、以 Stripe 为准收费，并在 ops 台标红提示偏差 | 一次 migration + payload 扩字段 + `/prices` 与用户中心同时改读 |
| B | 不动数据模型，前端金额继续走 i18n 文案，目录只提供 `active`/`stripePriceId` | 零成本，但「调整价格」永远是假功能，且文案与 Stripe 长期漂移 |
| C | 前端实时查 Stripe Price | accounts 要新增代理接口 + 缓存；Stripe 不可用时定价页整页降级 |

**建议 A**。理由：运营明确要「调整价格 + 变更有记录」（[10](./10-ops-console-design.md)），B 做不到审计一个不存在的字段，C 把公开定价页的可用性绑到 Stripe 上。
A 的价格字段天然进 `audit_logs` 的 before/after 快照，和已经跑通的 `AuditActionPlanUpsert` 是一套。

### D2 · 充值走 Stripe 一次性支付还是 Customer Balance？

[README 待拍板 #4](./README.md#必须先拍板的问题) 到今天仍未决。

**建议：Stripe 一次性 Checkout（`mode=payment`）**，因为 `creditTopUpBalance` 已经按这条路实现并测过幂等，改走 Customer Balance 等于推翻已完成的工作。
Customer Balance 的好处（退款、余额与 Stripe 对账天然一致）在当前规模下不值这个改造。

配套要定的是**面额**：建议 ¥50 / ¥100 / ¥300 三档固定面额 + 自定义金额（最低 ¥10、最高 ¥5000）。固定面额可以直接建三个 Stripe Price，自定义金额需要 `price_data` 动态创建——**先只上固定面额**，自定义金额放 P1。

### D3 · 试用到期后回落到哪一档？

代码里有 `downgradeToFreePlan`，`/prices` 的「Free 体验版」文案却写的是「每周 1 小时高速流量，用完降级 VPS」，而 `FREE` 这条目录记录的 `includedQuotaBytes` 是 **0**。

三者对不上。需要确认：**FREE 档到底给不给流量**。这同时是 [README 待拍板 #3](./README.md#必须先拍板的问题)（Free 档时长口径）的前置。
在它定下来之前，试用到期提示只能说「将回落到 Free 档」，不能承诺具体额度。

### D4 · OAuth 用户的权益门槛

- **选项 1**：OAuth 注册即视为已验证邮箱（信任 GitHub/Google 的邮箱），直接发试用
- **选项 2**：维持现状，但在 `/panel` 顶部加一条**阻断式**提示「验证邮箱后开通 7 天全功能试用」，并把 `ServiceReadinessCard` 的邮箱项提到第一位、说明它决定能否用

**建议选项 1**，且只对 provider 返回 `email_verified=true` 的账号生效（GitHub 的 primary+verified email、Google 的 `email_verified` claim）。
选项 2 保留了一个「注册完什么都用不了」的状态，是当前 UAT 上真实存在的坏体验。

---

### D5 · ScopeHub / CloudFlow 现在卖不卖？

用户中心在推销这两个产品（4 张卡、USD 12–59），`/prices` 上没有它们，`billing_plans` 里也没有。
只有两条自洽的路：

- **收窄（推荐）**：P0 阶段 `BillingOptionsPanel` 只渲染目录里存在的条目，这两个产品自然消失，等它们真的进目录再出现
- 补齐：给两个产品补目录记录、定价文案和 Stripe Price——但这是三个产品线的商业化，不是本轮范围

**建议收窄**。改读目录（P0-2）本身就会产生这个效果，不需要额外删代码；`products/registry.ts` 的 `billing` 字段在切换完成后成为死代码，一并清理。

---

## 四、分期计划

### P0 · 打通「看得到就买得到」（阻塞发布）

1. **落 UAT 目录数据**：`PAYG` / `PRO-MONTHLY` / `PRO-YEARLY` 三条记录 + 对应 Stripe Price ID
   - 走 `/panel/ops/billing/plans` 的管理端 CRUD 落数据，**不要直连数据库**——这样变更自动进审计
   - 验收：`curl /api/billing/plans` 返回 5 条，三条付费档 `stripePriceId` 非空
2. **`BillingOptionsPanel` 改读目录**（G1）
   - 删掉对 `@modules/products/registry` 的 `billing` 依赖，改 `fetch("/api/billing/plans")`
   - 只渲染 `active && stripePriceId` 的条目；其余显示「即将上线」，与 `/prices` 同一套判定
   - 按 D5，ScopeHub / CloudFlow 的 4 张卡随之消失；确认后清理 `products/*.ts` 里的 `billing` 死代码
   - 验收：同一账号在 `/prices` 和 `/panel/subscription` 看到的 planId、金额、货币、可购买状态**逐条一致**，且不出现定价页没有的商品
3. **充值入口**（G3）
   - 新增 `TopUpPanel`：三档固定面额 + 当前余额 + 「充值后立即到账，可在下方流水核对」
   - 复用现有 `startStripeCheckout({ mode: "payment" })`，成功回跳复用 `CheckoutStatusBanner`
   - 验收：UAT 用测试卡完成一次 ¥50 充值 → `current_balance` +50、`billing_ledger` 多一条 `topup`；**手动重投同一个 webhook 事件，余额不变**（这条是 `creditTopUpBalance` 幂等的回归验证，必须实测）

P0 完成的判据是一句话：**一个新注册用户能在控制台里花掉真钱并拿到对应权益。**

### P1 · 生命周期可见（G5/G8/G9）

4. `SubscriptionPanel` 增加**当前档位卡片**：档位名、周期起止、试用剩余天数、到期后行为（依赖 D3）
5. 试用剩余 ≤ 2 天时在 `/panel` 顶部出提示条，给「立即订阅」与「充值」两个出口
6. 欠费/停机状态下把提示升级为阻断条，主按钮直接进充值流程
7. MFA 门槛引导：把「绑定 MFA 后可支付」换成可点击的引导，直接拉起 `MfaSetupPanel`，完成后回到原来的下单动作

### P2 · 档位变更与注册链路（G6/G7）

8. OAuth 注册按 D4 修复（**这条如果选了方案 1，实现成本很低，可以提前到 P0 一起做**）
9. 升降档：accounts 增加 `POST /subscriptions/change-plan`，走 Stripe subscription update（proration 策略需单独确认）；用户中心提供月付↔年付切换
10. 价格字段落库（D1 方案 A），`/prices` 与用户中心同时切到目录取价，ops 台的「调整价格」变成真功能

---

## 五、把 XConnect 订阅跑通（UAT runbook）

代码侧已经就位（accounts#75 / #77，portal#218）。**真正的阻塞不是代码，是 UAT 上没有 Stripe 凭据**：

```bash
curl -X POST https://accounts-uat.onwalk.net/api/billing/stripe/webhook \
  -H 'Content-Type: application/json' -d '{}'
# HTTP 503 {"error":"stripe_not_configured","message":"stripe is not configured"}
```

`stripeWebhook` 的第一行就是 `h.stripe.enabled()` 判断，503 说明 `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` 仍然是空的——与 [README 现状盘点](./README.md) 在 2026-08-05 记的完全一致，**至 2026-08-13 未变**。在这一步解决之前，下单按钮无论怎么改都只会拿到 503。

按顺序执行：

| # | 步骤 | 谁做 | 判据 |
| --- | --- | --- | --- |
| 1 | 在 Stripe 拿一个 test mode secret key（`sk_test_...`） | 人工，Stripe 后台 | — |
| 2 | 跑目录同步，创建 Product / Price / Webhook Endpoint | `scripts/stripe-sync-catalog.sh --env uat --domain-base onwalk.net` | 打印出 5 个 `price_...` |
| 3 | 把 `sk_test_...` 与第 2 步一次性打印的 `whsec_...` 写进 Vault，重新渲染 `secrets.env` 并重启 accounts | Ansible / Doco-CD | 上面那条 curl 从 503 变成 401 `invalid_signature` |
| 4 | 合并并部署 accounts#75、#77、portal#218 | — | `/api/billing/plans` 的 payload 出现 `priceAmount` 字段 |
| 5 | 带 `--write-catalog` 重跑同步，把 price id 与挂牌价写回 `billing_plans` | 需要 `ACCOUNTS_ADMIN_TOKEN` | 目录从 2 条变 8 条，三档付费 `stripePriceId` 非空 |
| 6 | 把 UAT 上 `TRIAL-7D` 的配额从 10GiB 改成 5GiB | 运营台 `/panel/ops/billing/plans`（**不要直连数据库**，走接口才进审计） | 目录 `includedQuotaBytes` = 5368709120 |
| 7 | 用测试卡走一遍订阅与充值 | 人工 | 见下 |

第 2 步不必等第 3 步——同步脚本只用 `STRIPE_SECRET_KEY` 直连 Stripe API，不经过 accounts。

### 第 7 步的验收判据

1. `/prices` 与 `/panel/subscription` 上同一个 planId 的**金额、货币、可购买状态逐条一致**
2. 订阅 `PRO-MONTHLY` 成功后：`subscriptions` 多一行 `active`，`account_billing_profiles` 的 `package_name` 变 `pro`，配额按目录重置
3. 充值 `PAYG-TOPUP-50` 成功后：`current_balance` +50，`billing_ledger` 多一条 `topup`
4. **在 Stripe 后台手动重投同一个 `checkout.session.completed` 事件，余额不变**——这条是 `creditTopUpBalance` 幂等性的回归验证，是整条链路上唯一直接关系到重复扣款的断言，**必须实测，不能只看代码**

### 关于 `STRIPE_ALLOWED_PRICE_IDS`

不需要配。`validCheckoutPrice` 一旦发现目录里存在任何带 `stripe_price_id` 的 active 套餐，就**只认目录**，env 允许列表仅在目录完全为空时作为引导期兜底。第 5 步做完之后它就永久失效了。

---

## 五、实施注意事项

**不要在用户中心复制一份定价文案。** 现在 `/prices` 和 `products/registry.ts` 已经是两份，再加一份就是三份。P0-2 的正确做法是让两个页面**共用同一个目录读取 hook**（把 `prices/page.tsx` 里的 `useBillingCatalog` 提到 `src/modules/billing/` 下共享）。

**充值金额单位。** `creditTopUpBalance` 里 `amount := float64(session.AmountTotal) / 100.0`，即 `current_balance` 的单位是「元」而非「分」。前端展示与后端 ledger 必须统一按元，不要在前端再除一次 100。

**`billing_ledger` 的 PK 幂等不能被绕过。** 任何新增的「补记余额」路径（比如运营手工补一笔充值）都必须复用 `topUpLedgerID(paymentRef)` 的推导方式或走已有的 `adminAdjustBalance`，不要另起一条无幂等保护的写入。

**改 `BillingOptionsPanel` 会动到测试。** `account/__tests__/SubscriptionPanel.test.tsx` 里有 3 个**改动前就已存在**的失败用例（在未修改的 `main` 上同样失败）。修复它们不是本次工作的范围，但也不要把新失败混进这 3 个里——动手前先跑一遍基线并记下来。

**目录数据落 UAT 时走管理端接口。** 直接 `INSERT` 会绕过 `audit_logs`，之后运营台的变更历史里会有一段空白，而这正是 [10](./10-ops-console-design.md) 要解决的问题。

---

## 六、交接说明

本篇是**规划**，不含实现。接手时的推荐顺序：

1. 先拿 D1–D4 四个决策的答复——**D1 和 D3 不定，P1 的 UI 文案就没法写**
2. 从 P0-1（落目录数据）开始，它不改代码，但能立刻让 `/prices` 的三张付费卡片从「即将上线」变成可点击，是验证整条链路最便宜的一步
3. P0-2 和 P0-3 可以并行：前者是把展示切到目录，后者是新增充值组件，两者只在共享的目录读取 hook 上有交集

相关文档：[01 套餐定义](./01-plan-catalog.md) · [09 试用与充值缺口](./09-trial-grants-and-topup-gap.md) · [10 运营台设计](./10-ops-console-design.md) · [11 运营台前端交接](./11-ops-console-handoff.md)
