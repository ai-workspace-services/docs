# 运营控制台设计

面向**管理员 / 运营者**的独立控制台：发放权益、调整配额、管理订阅与定价、维护用户分类，且**每一次变更都留下可追溯的记录**。

本文是 [03-operations-console.md](./03-operations-console.md) 的落地版——03 描述能力清单，这份描述具体怎么做：路由、接口、数据模型、审计机制、分阶段顺序。

## 为什么要独立开页，而不是继续堆在 `/panel/management`

现状 `/panel/management` 是**用户中心扩展下的一个路由**，和"账户""订阅""外观设置"平级。单文件 525 行，页面里同时挤着**权限矩阵**和**首页视频配置**——这个组合本身就说明它已经变成杂物间。

| 维度 | 现状 | 运营实际需要 |
| --- | --- | --- |
| 定位 | 用户中心的子页 | 独立控制台 |
| 内容 | 权限矩阵 + 用户列表 + 首页视频 | 权益、订阅、定价、账单、分类、审计 |
| 导航 | 侧边栏单条 | 自己的多级导航 |
| 变更留痕 | **无** | 硬要求 |

## 审计：先做这个，不是最后做

**`audit_logs` 表已经存在于 `sql/schema.sql`，但没有任何 Go 代码写它——是个空壳。**

```sql
CREATE TABLE IF NOT EXISTS public.audit_logs (
  uuid       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  action     TEXT NOT NULL DEFAULT '',
  actor_uuid UUID,
  details    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- 已有索引：created_at DESC、(action, created_at DESC)
```

现成的表结构够用，**不需要迁移**。要补的是写入侧。

### 约定

每个运营写操作落一条，`details` 里必须包含变更前后值与原因：

```jsonc
{
  "action": "billing.quota.adjust",
  "actor_uuid": "<操作人 uuid>",
  "details": {
    "target_uuid": "<被操作账号>",
    "before": { "remaining_included_quota": 0 },
    "after":  { "remaining_included_quota": 21474836480 },
    "reason": "客户投诉补偿，工单 #1234",   // 必填，接口层校验非空
    "request_id": "<trace id>"
  }
}
```

**`reason` 必填**：能改余额和权益的接口，没有理由就不该放行。接口层直接 400，不给"事后补"的空间。

### 动作命名

`<域>.<对象>.<动作>`，便于按前缀检索：

```
billing.plan.upsert        billing.plan.delete
billing.quota.adjust       billing.balance.adjust
billing.entitlement.grant  billing.trial.grant
billing.arrears.clear      billing.subscription.cancel
account.segment.update     account.role.update
```

### 实现要点

- **审计写入与业务写入同事务**：业务成功但审计丢失，等于没有审计。用同一个 `tx`。
- **审计表只增不改**：不提供 UPDATE/DELETE 接口。
- **`audit_logs` 会长得很快**：已有 UAT 因审计日志 IO 激增的事故 runbook（portal#165）。上线前定好保留策略（建议 180 天后归档，或按 `created_at` 分区）。

## 路由结构

新开 `/panel/ops` 作为控制台根，按域分子路由：

| 路由 | 内容 |
| --- | --- |
| `/panel/ops` | 概览：MRR、活跃订阅、欠费名单、用量 TopN |
| `/panel/ops/accounts` | 账号检索（按邮箱/UUID/套餐/分类/状态） |
| `/panel/ops/accounts/:uuid` | **单账号操作台**：权益、订阅、余额、配额、分类、停机状态、该账号的审计流水 |
| `/panel/ops/plans` | 套餐目录 CRUD + **发布到 `/prices`** |
| `/panel/ops/subscriptions` | 全站订阅列表，按状态筛 |
| `/panel/ops/ledger` | 账单明细、导出、对账 |
| `/panel/ops/audit` | 全局审计流水，按 action / actor / 时间检索 |

现有 `/panel/management` **保留不删**，逐步搬空：权限矩阵与黑名单迁到 `/panel/ops/system`，首页视频配置属于内容运营、留在原处或另归。一次性删除会打断正在使用的人。

### 访问控制

沿用 [portal#167](https://github.com/ai-workspace-services/portal/pull/167) 的组继承：

```ts
guard: {
  requireLogin: true,
  roles: ["admin", "operator"],
  permissions: ["admin.settings.read", ...],
}
```

`resolveEffectiveRole` 已支持从 `root`/`admin`/`operator` 组继承角色，所以把人加进"运营者"组即可授权，不必改他的 `role` 字段。

**但要注意分级**：`operator` 能看能改配额，但**改余额、改价格**这类涉及真金白银的操作建议单独一个权限位（如 `admin.billing.money.write`），只给 admin。能改价目表和能给账号打钱不是同一个信任级别。

## 需要新增的后端接口

现有 admin 接口只有套餐 CRUD + 清欠费，**账号级权益操作一个都没有**。这是前端做完也没数据可用的原因，所以后端要先行。

### P0 — 没有就没法运营

| 接口 | 用途 | 审计 action |
| --- | --- | --- |
| `GET /admin/billing/accounts` | 账号检索（`?q=&plan=&segment=&state=`） | — |
| `GET /admin/billing/accounts/:uuid` | 单账号全景：profile + quota + subscriptions + 最近 ledger | — |
| `POST /admin/billing/accounts/:uuid/plan` | **指派套餐**（开通 custom、人工纠错） | `billing.entitlement.grant` |
| `POST /admin/billing/accounts/:uuid/quota` | **调整配额** | `billing.quota.adjust` |
| `POST /admin/billing/accounts/:uuid/balance` | **调整余额**（补偿/退款入账/赠送） | `billing.balance.adjust` |
| `POST /admin/billing/accounts/:uuid/grant-trial` | **发放体验试用额度** | `billing.trial.grant` |
| `GET /admin/audit` | 审计流水检索 | — |

### P1

| 接口 | 用途 |
| --- | --- |
| `GET /admin/billing/subscriptions` | 全站订阅列表 |
| `POST /admin/billing/subscriptions/:id/cancel` | 运营代取消 |
| `GET /admin/billing/ledger` | 账单明细 + CSV 导出 |
| `GET /admin/billing/overview` | MRR / 活跃订阅 / 欠费 / TopN |

### 复用既有函数，不要重写

`api/entitlements.go` 里这些是现成的，新接口直接调：

| 函数 | 作用 |
| --- | --- |
| `applyPlanEntitlements(ctx, userID, plan)` | 按套餐写 billing profile |
| `resetQuotaForPlan(ctx, userID, plan, start, end)` | 重置配额并设周期 |
| `provisionTrialEntitlements(ctx, userID)` | **发试用的现成模板**——目前硬编码 `TRIAL-7D` 且只在注册时触发，改造成接受任意 plan + 时长即可 |
| `supersedeActiveTrials(ctx, userID)` | 付费接管后作废试用 |
| `publishBillingEvent(ctx, event)` | 发计费事件 |

**发放试用额度**这条基本是把 `provisionTrialEntitlements` 参数化 + 加审计，不是从零写。

## 发布更新 `/prices`

`/prices` 页面已经**实时读取 `GET /api/billing/plans`**（[portal#166](https://github.com/ai-workspace-services/portal/pull/166)），所以"发布定价"= 改 `billing_plans` 目录，前台自动生效，**不需要发版**。

`/panel/ops/plans` 的编辑表单落到已有字段上：

| 字段 | 用途 |
| --- | --- |
| `display_name` / `sort_order` | 展示名与排序 |
| `included_quota_bytes` | 含量配额 |
| `price_multipliers` (jsonb) | 单价费率 |
| `features` (jsonb) | 功能开关与非流量额度 |
| `stripe_price_id` | 关联 Stripe 价格 |
| **`active`** | **上架 / 下架开关** |

### 两条硬约束

1. **Stripe 价格创建后金额不可变**。改价必须在 Stripe 侧建新 Price（换 `lookup_key`，见 [05](./05-stripe-catalog-automation.md)），再把新 `stripe_price_id` 写回目录。**编辑表单里的价格字段应该是只读的**，旁边给一条"如何改价"的指引——否则运营会以为改了数字就生效，实际 Stripe 那边纹丝不动，这是最容易造成错收费的地方。
2. **下架用 `active=false`，不要删目录条目**。存量订阅仍然引用它做续费与对账；删掉会让历史账单失去关联。

## 用户分类管理

四类账号（注册 / 订阅 / 运营 / 内测）用 `groups` 承载——[accounts#56](https://github.com/ai-workspace-services/accounts/pull/56) 已提供 `PUT /admin/users/:id/groups`，[portal#162](https://github.com/ai-workspace-services/portal/pull/162) 已提供编辑 UI。

⚠️ **`groups` 是双语义字段，这是个坑**：它同时承载**节点路由可达性**（`EligibleNodeGroups`，贯穿 `internal/agentserver/registry.go`）和**权限继承**（`GROUP_ROLE_MAP` 认 `root`/`admin`/`operator`）。所以业务分类标签必须加前缀隔离：

```
segment:registered    segment:subscribed
segment:operator      segment:beta
```

**不要用裸名 `operator` 当业务标签**——它会被 `resolveEffectiveRole` 当成权限组，直接把人提权成运营者。这类"一个字段两种含义"正是此前 accounts#49 白屏事故的同类根因。

长期建议给 `users` 加独立的 `segments` 列（见 [06](./06-management-console-integration.md)），彻底分开。

## 分阶段

| 阶段 | 内容 | 依赖 |
| --- | --- | --- |
| **A. 审计地基** | `audit_logs` 写入函数 + 同事务约定 + `reason` 必填校验；给**既有**的清欠费/套餐 CRUD 补上审计 | 无，现在就能做 |
| **B. P0 接口** | 账号检索、单账号全景、指派套餐、调配额、调余额、发试用 | A |
| **C. 控制台骨架** | `/panel/ops` 路由 + 概览 + 账号检索 + 单账号操作台 | B |
| **D. 定价管理** | `/panel/ops/plans`，含上下架与"价格只读 + 改价指引" | B |
| **E. 订阅与账单** | 订阅列表、ledger、导出、对账 | B |
| **F. 经营看板** | MRR / 漏斗 / 配额利用率 | E |

**A 必须最先做**，理由：B 阶段的接口一旦上线就能改钱，那时再补审计，中间这段变更就永远查不到了。

## 与既有工作的关系

- [03-operations-console.md](./03-operations-console.md)：能力清单（P0/P1/P2），本文是其落地设计
- [09-trial-grants-and-topup-gap.md](./09-trial-grants-and-topup-gap.md)：发放试用的接口设计 + **充值不入余额**的代码缺口（PAYG 档核心路径仍是坏的）
- [06-management-console-integration.md](./06-management-console-integration.md)：`segments` 独立字段方案
