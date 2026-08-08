# 存量用户迁移

已有几十个真实用户在用（生产 svc.plus），从来没有经过计费/套餐流程——他们注册时这套体系还不存在。计费上线不能让这批人"突然被收费"或"突然被限速/停机"，否则第一批真实用户的体验就是一次事故。

本文档只覆盖生产环境的迁移设计；**执行前需要你的明确批准**——按会话里的约定，生产环境的变更我不会未经确认直接执行。

## 现状（同样的问题在 UAT 已经实测过一次）

[02-metering-and-entitlements.md](./02-metering-and-entitlements.md#存量账号补齐) 已经记录过：UAT 上 `account_billing_profiles` 一行都没有，包括 bootstrap 出来的 `admin@svc.plus`。生产的存量用户是同一个问题的真实版本——`provisionOnboardingTrial` 只在注册/OAuth 首次登录时触发，所有在这套逻辑上线前就存在的账号，从来没有被这个函数碰过。

不迁移的后果不是"缺一些数据"，是**功能性故障**：一旦欠费停机策略（[02](./02-metering-and-entitlements.md#欠费与停机策略)）上线，这些没有 `account_quota_states` 行的账号在权益查询时会被判定为异常态，具体后果取决于查询代码对"无 profile"的处理方式——必须在开启任何强制策略之前解决，不能等出问题再补。

## 原则

1. **先分类，再分配**：不是所有存量用户都该套同一个默认方案。至少要分两类：
   - 纯免费体验用户 → 落 `FREE`，符合直觉，无争议
   - 已经在稳定使用、流量不小的用户 → 直接套 `FREE` 可能意味着立刻被限速（[01](./01-plan-catalog.md#free) 里 free 档"每周 1 小时高速"），对这批人是实质性的服务降级，需要一个更宽松的**过渡套餐**
2. **迁移过程本身不触发任何计费或停机**。回填是纯粹的数据操作，欠费判定的时钟（`arrears_since`）在回填当下不启动
3. **迁移可复现、可审计**：回填脚本本身就是一份记录——谁、什么时候、以什么规则，被批量分类成了什么
4. **打 `legacy` 标签**（见 [06](./06-management-console-integration.md#账号分类标签新概念独立于套餐)），迁移完成后这批账号在管理台里始终可识别、可单独筛选、可单独调整策略

## 方案：过渡套餐 `LEGACY-GRANDFATHERED`

不直接把存量用户分进 free/payg/pro 任何一档，新开一档专用于迁移：

```
plan_id              = LEGACY-GRANDFATHERED
kind                 = custom
stripe_price_id      = (空，不接 Stripe)
included_quota_bytes = 按迁移当下的实际使用水平设定（见下）
package_name         = legacy
```

```jsonc
// features
{
  "sla": "standard",
  "session_persistence": true,
  "fast_lane": { "mode": "quota" },
  "overage": { "policy": "none" },        // 超出配额不计费、不降级——观察期内不能让人有实质损失
  "dunning": { "policy": "manual" },      // 不自动停机，出问题运营手动处理
  "grandfathered": {
    "migrated_at": "<回填执行时间戳>",
    "reason": "pre-billing existing user",
    "review_after": "<建议 90 天后复核>"
  }
}
```

**为什么不是直接给 `FREE`**：free 档的时长限制（每周 1 小时高速）是为**新用户**设计的体验档位，直接套用在已经稳定使用的存量用户身上等于一次没有预警的服务降级。`LEGACY-GRANDFATHERED` 本质是"维持现状，但纳入计费体系的可观测范围"，不是"给优惠"。

**为什么不是直接给 `PRO`**：Pro 是付费订阅，存量用户从未同意付费，不能未经同意把人放进一个会在将来触发扣费的套餐。

**为什么 `overage.policy = none`**：观察期内目标是"先摸清这批人的真实用量"，不是"先收一轮钱"。等 [07 复核](#复核与退出) 之后再决定分流到哪个正式套餐。

## 配额怎么定

不能拍一个统一数字——几十个用户里，用量分布大概率很不均匀。回填脚本按每个账号过去 30 天的真实用量（`traffic_minute_buckets` 已经在记录，[feature-xray-usage-billing-portal-uat](../feature-xray-usage-billing-portal-uat/) 那条链路上线后的数据可以直接查）设置 `included_quota_bytes`，给一个宽松系数（建议 2×，避免用量周环比波动被误判为"超额"）：

```sql
-- 示例：按账号过去 30 天实际用量的 2 倍设定迁移配额
SELECT account_uuid, SUM(total_bytes) * 2 AS suggested_quota
FROM traffic_minute_buckets
WHERE bucket_start >= now() - interval '30 days'
GROUP BY account_uuid;
```

没有历史用量数据的账号（比如计量链路上线前就存在、从未被记录过）给一个保守的固定值，并优先人工核实。

## 执行步骤

### 1. 摸底（只读，随时可以做，不影响生产）

```sql
SELECT u.uuid, u.email, u.created_at,
       p.package_name IS NOT NULL AS has_billing_profile
FROM users u
LEFT JOIN account_billing_profiles p ON p.account_uuid = u.uuid
WHERE p.account_uuid IS NULL;
```

产出：确切的存量用户数量与名单，替换掉"几十个"这个估计值。这一步现在就能做，不依赖后面任何步骤。

### 2. 用量画像

对上一步的名单跑 [配额怎么定](#配额怎么定) 的查询，人工看一遍分布——是否存在个别账号用量远超其他人（可能是异常/滥用，需要单独处理而不是套用批量规则）。

### 3. 回填脚本（新增，`accounts/scripts/backfill-legacy-billing.sh` 或 Go migration）

幂等设计，与今天写的 `stripe-sync-catalog.sh` 同一个原则——可以安全重跑：

- 只处理 `account_billing_profiles` 不存在的账号（重跑时已迁移的自动跳过）
- 先 `--dry-run` 打印将要写入的 `(uuid, email, suggested_quota)` 表，人工过一遍再执行
- 执行时：写 `account_billing_profiles`（`package_name=legacy`）+ `account_quota_states`（配额按上面算好的值，`arrears=false`）+ `users.segments` 加 `legacy` 标签，三件事在一个数据库事务里做完，不允许"分类打了但配额没写"这种半成品状态

### 4. 验证

回填后：管理台（[06](./06-management-console-integration.md)）里筛 `legacy` 标签，抽查几个账号的套餐/配额/分类是否符合预期；Portal 账户面板确认这批用户看到的是合理数字，不是 `0 B`/`default` 这种此前 UAT 上出现过的空态。

### 5. 通知（可选，取决于业务决定）

是否需要告知这批用户"你现在被纳入了正式的计费体系，当前维持原有使用方式不受影响"，是产品/运营决定，不是本文档要下的结论。如果需要发通知，`legacy` 标签正好是精准的目标人群筛选依据。

## 复核与退出

`LEGACY-GRANDFATHERED` 不是永久状态。建议 90 天后（`grandfathered.review_after`）复核每个账号的真实用量，人工决定：

- 用量稳定且不大 → 迁移到 `FREE` 或引导订阅 `PRO`
- 用量持续增长、明显是重度用户 → 引导升级到 `PRO`，作为最早一批转化对象
- 长期不活跃 → 保留观察或按运营策略处理

复核本身也应该走管理台的「指派套餐」操作（[06](./06-management-console-integration.md#2-扩展-usergroupmanagement-表格)），产生正常的套餐变更记录，而不是另开一套特殊流程。

## 时序要求

这个迁移必须在以下两件事**之前**完成：

1. [02-metering-and-entitlements.md](./02-metering-and-entitlements.md#欠费与停机策略) 的分档欠费策略上线（否则存量用户会被现有 UAT 上观察到的"无 profile"异常状态影响，或被全局兜底阈值误伤）
2. 任何面向存量用户的主动通知/引导升级动作

也就是说，回填必须排在 [04-delivery-phases.md](./04-delivery-phases.md) 的 **S2 之前**，作为 S2 的前置步骤，而不是并行或之后。
