# 套餐目录设计

四档产品线：`free` → `payg` → `pro` → `custom`，构成从体验到自助付费到订阅再到商务定制的完整升级路径。

## 现有数据模型

`billing_plans`（实测 UAT schema）：

| 列 | 类型 | 用途 |
| --- | --- | --- |
| `plan_id` | text PK | 目录主键，如 `PRO-MONTHLY` |
| `stripe_price_id` | text UNIQUE | 对应 Stripe Price；`custom` 与 `free` 留空 |
| `display_name` | text | 展示名 |
| `kind` | text | `subscription` / `trial` / 新增 `prepaid`、`custom` |
| `included_quota_bytes` | bigint | 周期内含高速流量字节数 |
| `package_name` | text | 写入 `account_billing_profiles.package_name`，计费规则组标识 |
| `price_multipliers` | jsonb | **费率**，本设计用它承载分层单价 |
| `features` | jsonb | **功能开关与非流量额度**，本设计的主要扩展位 |
| `trial_days` | integer | 试用天数 |
| `active` / `sort_order` | | 上架与排序 |

两个 jsonb 列是关键：四档的差异化能力**不需要加列**即可表达。

## 四档定义

### free

体验档。可登录、可试用，但不承诺 SLA、不提供持久化。

```
plan_id              = FREE
kind                 = subscription
stripe_price_id      = (空)
included_quota_bytes = 0          -- 高速流量不按字节给，按时间窗口给
package_name         = free
```

```jsonc
// features
{
  "sla": "none",
  "session_persistence": false,        // 不提供多端会话持久存储
  "demo_cards": {
    "enabled": true,
    "create": true,                    // 仅 Demo 资源卡片
    "runs_per_day": 1,
    "session_minutes": 60
  },
  "fast_lane": {
    "mode": "windowed",                // 时间窗口配额，非字节配额
    "window": "weekly",
    "minutes": 60,
    "fallback": "vps"                  // 窗口耗尽后降级为 VPS 流量，不断线
  },
  "resource_cards": { "create": false },
  "dunning": { "policy": "none" }      // 无付费，无欠费概念
}
```

```jsonc
// price_multipliers
{ "fast_lane_cny_per_gb": 0, "vps_lane_cny_per_gb": 0 }
```

**要点**：free 的高速流量是**时间维度**配额，与其余三档的字节维度不同。`included_quota_bytes = 0` 不代表没有高速流量，而是表示"不由字节配额管辖"，由 `features.fast_lane` 管辖。计量侧必须能识别这个区别，详见 [02](./02-metering-and-entitlements.md#时长维度)。

### Pay-As-You-Go

预充值、按量扣费。无月费、无赠送额度，欠费立即停机。

```
plan_id              = PAYG
kind                 = prepaid        -- 新增 kind
stripe_price_id      = (空，充值走一次性支付，见下)
included_quota_bytes = 0
package_name         = payg
```

```jsonc
// features
{
  "sla": "standard",
  "session_persistence": true,
  "resource_cards": { "create": true, "pricing": "list_price" },
  "retention": {
    "compute_days": 7,                 // 欠费停机后资源保留 7 天
    "object_storage_days": 30          // 对象存储最多保留 30 天后释放
  },
  "dunning": {
    "policy": "suspend_on_zero_balance",
    "grace_hours": 0                   // 欠费立刻停机
  }
}
```

```jsonc
// price_multipliers
{ "fast_lane_cny_per_gb": 1.0, "vps_lane_cny_per_gb": 0 }
```

高速流量 1 元/GB，折算 `base_price_per_byte = 1 / 1073741824 ≈ 9.3132257461548e-10`。

#### 充值入口（待确认）

`account_quota_states.current_balance`（double precision）已存在，但**目前没有任何充值路径**。两种方案：

| 方案 | 做法 | 权衡 |
| --- | --- | --- |
| A. 一次性 Checkout | 建若干固定面额 Price（如 ¥50/¥100/¥500），`mode=payment` 下单，webhook `checkout.session.completed` 后 `current_balance += 金额` | 实现最轻，复用现有 checkout 代码；面额固定，不支持任意金额 |
| B. Stripe Customer Balance | 用 Stripe 的客户余额对象作为事实源 | 对账天然一致；但要把余额事实源从本地库迁到 Stripe，改动大，且与现有 `current_balance` 扣减逻辑冲突 |

**建议 A**：本地库继续是余额事实源，Stripe 只负责收钱。与「billing-service 不碰 Stripe、accounts 单向同步」的既有边界一致。

### Pro

订阅档，含赠送高速流量，超出部分按 PAYG 费率计价。

```
plan_id              = PRO-MONTHLY        |  PRO-YEARLY
kind                 = subscription       |  subscription
stripe_price_id      = price_xxx (¥20/月) |  price_yyy (¥200/年)
included_quota_bytes = 21474836480 (20GB) |  21474836480 (20GB，见下)
package_name         = pro                |  pro
```

```jsonc
// features（两者相同）
{
  "sla": "standard",
  "session_persistence": true,
  "resource_cards": {
    "create": true,
    "pricing": "list_price_plus_managed_fee",
    "managed_fee_rate": 0.20            // 明码实价 + 20% 托管服务费
  },
  "fast_lane": { "mode": "quota" },     // 字节配额，非时间窗口
  "overage": { "policy": "charge", "cny_per_gb": 1.0 },
  "dunning": { "policy": "grace_then_suspend", "grace_days": 14 }
}
```

#### Pro 年付的配额周期

原始描述：*年付 200 元/年，每年累计 240GB，超过按 1GB/元 收费（每个自然月循环）*。

这句话有两种读法，工作量差别很大：

| 读法 | 含义 | 实现影响 |
| --- | --- | --- |
| **(a) 月度池** | 每自然月 20GB，共 12 期，年累计 240GB | 与月付共用同一套重置逻辑，只是 Stripe 周期是年 |
| (b) 年度池 | 一次性发放 240GB，用完即按量，全年不重置 | 需要新的"年度池"配额类型 |

`20GB × 12 = 240GB` 且原文明确写「每个自然月循环」，**本设计按 (a) 实现**：`included_quota_bytes = 20GB`，每自然月重置。

⚠️ **这带来一个真实的实现冲突**：现有 `resetQuotaForPlan` 的周期边界来自 Stripe subscription 的 `current_period_start/end`。年付订阅的 Stripe 周期是**一年**，直接用它会导致配额一年才重置一次。详见 [02 配额周期](./02-metering-and-entitlements.md#配额周期与-stripe-周期解耦)。

### Custom（专属定制）

商务签约档，不走自助支付。

```
plan_id              = CUSTOM-<客户标识>
kind                 = custom          -- 新增 kind
stripe_price_id      = (空)
included_quota_bytes = 按合同
package_name         = custom
```

```jsonc
// features
{
  "sla": "contract",
  "session_persistence": true,
  "resource_cards": { "create": true, "pricing": "contract" },
  "dunning": { "policy": "manual" },   // 不自动停机，由运营处理
  "contract": { "ref": "<合同编号>", "owner": "<商务负责人>" }
}
```

**开通方式**：运营通过 admin 套餐 CRUD 建条目，再手动为账号 `applyPlanEntitlements`。需要新增一个"手动为指定账号指派套餐"的管理端接口（现有 admin 接口只能改目录，不能给某个账号指派）。

## 升级路径

```
free ──(充值)──> payg ──(订阅)──> pro ──(商务)──> custom
  │                                  │
  └──────────(直接订阅)──────────────┘
```

| 转换 | 触发 | 权益处理 |
| --- | --- | --- |
| free → payg | 首次充值成功 | 写 `payg` profile；余额入账；解除 free 的时间窗口限制 |
| free/payg → pro | Checkout 成功 | `applyPlanEntitlements(PRO-*)` + `resetQuotaForPlan`；PAYG 余额**保留**，用于超额抵扣 |
| pro → free | 订阅取消/到期 | `downgradeToFreePlan`；余额保留 |
| any → custom | 运营手动 | 手动指派，不经 Stripe |

**关键**：升降级时 `current_balance` 一律保留，不清零。Pro 用户的超额费用优先从余额扣，余额不足才转欠费。

## Stripe 对象映射

| 本地 | Stripe | 说明 |
| --- | --- | --- |
| `PRO-MONTHLY` | Product「Pro」+ Price ¥20/month recurring | |
| `PRO-YEARLY` | 同 Product + Price ¥200/year recurring | 同一 Product 下两个 Price，便于在 Billing Portal 内切换 |
| PAYG 充值 | Product「余额充值」+ 若干 one-time Price | `mode=payment`，非订阅 |
| 超额流量 | **不建 Stripe 计量对象** | 超额从本地余额扣；余额不足转欠费。见 [02](./02-metering-and-entitlements.md#超额计费) |
| `FREE` / `CUSTOM-*` | 无 | 不经 Stripe |

**超额不上报 Stripe usage records** 是一个明确取舍：上报 metered usage 会把"用量事实源"分裂到 Stripe 和本地库两处，与既有「PostgreSQL 是账务事实源」的决策冲突。代价是超额部分不出现在 Stripe 发票上，需要本地账单体现。

## 目录初始化

上述条目通过既有 admin 接口写入，不需要迁移脚本：

```
PUT /api/auth/admin/billing/plans/FREE
PUT /api/auth/admin/billing/plans/PAYG
PUT /api/auth/admin/billing/plans/PRO-MONTHLY
PUT /api/auth/admin/billing/plans/PRO-YEARLY
```

现有 `TRIAL-7D` 保留不动——它是注册后的 7 天全功能试用，与 free 档并存：试用期内享 Pro 级权益，到期 `supersedeActiveTrials` 后落回 free。
