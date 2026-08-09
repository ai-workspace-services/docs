# 发放试用额度与充值入账缺口

补 [06-management-console-integration.md](./06-management-console-integration.md) 的接口清单没覆盖的两件事：**发放试用额度**（运营明确要的功能），以及**充值不入余额**（此前全套文档都没记录过的代码缺口）。

## 一、发放试用额度

06 的接口清单里有「指派套餐」和「调整余额」，但没有「发试用」。这两者不能替代它：试用有**期限**和**到期回落**语义，指派套餐没有。

### 底层能力已经齐了

`api/entitlements.go` 里三步俱全，只是没有对外入口：

```go
applyPlanEntitlements(ctx, userID, plan)          // 写 account_billing_profiles
resetQuotaForPlan(ctx, userID, plan, start, end)  // 写 account_quota_states，可传周期边界
publishBillingEvent(ctx, &BillingEvent{...})      // 发 PGMQ 事件
```

`provisionTrialEntitlements` 正是这三步的现成组合，**但硬编码了 `TRIAL-7D`，且只被 `provisionOnboardingTrial` 在注册/OAuth 首登时调用一次**。运营想给一个已存在的账号发试用，没有任何路径能触发它。

**所以这不是从零实现，是把已有能力接出一个受控入口。**

### 接口

```
POST /api/auth/admin/billing/accounts/:uuid/grant-trial
```

```jsonc
{
  "planId": "TRIAL-7D",      // 必填，须是目录里 kind=trial 的条目
  "days": 7,                 // 可选，默认取 plan.trialDays
  "reason": "客户投诉补偿",   // 必填，写入审计
  "supersedeExisting": true  // 默认 true
}
```

行为：

1. 校验账号存在、`planId` 存在且 `kind == "trial"`
2. `supersedeExisting` 时调 `supersedeActiveTrials`——**必须做**，否则同账号叠加多个 active 试用，`ListSubscriptionsByUser` 的消费方无法判断哪个有效
3. `UpsertSubscription` 写试用订阅
4. `applyPlanEntitlements` + `resetQuotaForPlan(now, now+days)`
5. `publishBillingEvent{Type: "trial_granted"}`
6. 写审计

### 三个容易踩的坑

**ExternalID 不能沿用注册路径的格式。** `provisionOnboardingTrial` 用的是 `fmt.Sprintf("trial-%s", userID)`——同一账号上是固定值。运营第二次发放会**覆盖**第一条记录，审计链直接断掉。管理端发放应使用 `trial-admin-<uuid>-<timestamp>`。

**事件类型要与自动发放区分。** 注册路径发的是 `trial_provisioned`；管理端发放应发 `trial_granted`，否则下游无法分辨"系统自动送的"和"运营手动送的"。

**试用天数必须有上限**（建议 90）。否则一次误操作就能发出近乎永久的全功能账号。超限返回 400，不要静默截断。

### 幂等性

试用发放**不是**幂等操作——同一账号可以合理地先后发两次。因此：

- 不要用固定 `ExternalID` 去重（会静默覆盖历史）
- 用显式 `idempotencyKey` 去重：同 key 重复提交返回首次结果
- 前端提交后立即禁用按钮

## 二、充值不入余额（代码缺口）

### 问题

`api/stripe.go` 处理 `checkout.session.completed` 时，一次性支付（`mode=payment`，即 PAYG 充值）分支只做了 `UpsertSubscription` 写一条订阅记录，**完全没有触碰 `current_balance`**：

```go
// api/stripe.go — 一次性支付分支，实测无任何余额写入
sub := &store.Subscription{ ... }
return h.store.UpsertSubscription(ctx, sub)
```

也就是说：**PAYG 用户充值成功，钱进了 Stripe，账户余额还是 0。**

这是 [01-plan-catalog.md](./01-plan-catalog.md#pay-as-you-go) 里 PAYG 档的核心路径。目录里 `PAYG-TOPUP-{50,100,500}` 三个面额已在 [05](./05-stripe-catalog-automation.md) 的 IaC 目录中定义，但收到钱之后没有入账逻辑接住。

### 要补什么

webhook 收到 `mode=payment` 且 `metadata.kind == "paygo"` 时：

1. 从 session 取实付金额（用 `amount_total`，不要用目录里的面额——以 Stripe 实际收到的为准）
2. `current_balance += amount`
3. **同时写一条 ledger 分录**（`entry_type = "topup"`），保持「余额 = ledger 累计」这个不变式。[03](./03-operations-console.md#余额调整必须走-ledger) 已经为手动调余额定过这条规则，充值同样适用
4. 发 `BillingEvent{Type: "balance_topped_up"}`

### 幂等是硬要求

**Stripe webhook 会重试**（网络超时、5xx、投递失败都会重投）。重复入账等于凭空发钱。

去重键用 `payment_intent`（一次性支付场景下唯一且稳定）：入账前先查 ledger 是否已有该 `payment_intent` 的 `topup` 分录，有则直接返回成功、不重复加钱。

这一点必须在写代码时就做进去，不能事后补——一旦线上发生重复入账，追回比预防难得多。

## 实施建议

| 顺序 | 项 | 理由 |
| --- | --- | --- |
| 1 | 充值入账 + ledger + 幂等 | 涉及真实资金，且 PAYG 档没它就是坏的 |
| 2 | `GET /admin/billing/accounts/:uuid` | 读接口无副作用，让运营先能看（06 已设计） |
| 3 | `POST .../grant-trial` | 运营最高频需求 |
| 4 | `POST .../plan`、`.../balance` | 06 已设计，涉及更多账务正确性考量 |

第 1 项与第 2、3 项无依赖，可并行。
