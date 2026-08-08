# 计量维度、配额周期与欠费策略

## 从一维到三维

当前计量链路只有**流量字节**一个维度。四档套餐的设计引入了另外两个：

| 维度 | 用在哪 | 计量源 | 现状 |
| --- | --- | --- | --- |
| **流量字节** | PAYG 扣费、Pro 配额与超额 | Xray → xray-exporter → Vector → billing-service | ✅ 已通 |
| **时长** | free 的「每周 1 小时高速」、Demo 卡片「每次 1 小时」 | 无 | ❌ 需新建 |
| **资源实例** | 资源服务卡片创建类、对象存储 | 无 | ❌ 需新建 |

三者互不替代，且落地难度递增。**建议分阶段**：流量分层 → 时长 → 资源实例。

## 流量分层

四档中有三档要区分**高速流量**与 **VPS 流量**（free 降级、PAYG 只对高速收费、Pro 只对高速计配额）。

⚠️ **这与既有决策冲突**：此前明确决定「先不区分高速流量和普通 VPS 流量」，当前 exporter → billing 链路按单一流量类型聚合。

### 判定依据

Xray 的 stats 计数器天然带 `inbound_tag`，exporter 已按 `(uuid, inbound_tag)` 产出 Sample——**这个维度已经存在于数据里，只是在 billing 侧被聚合掉了**（`aggregateSamplesByUUID` 把同一 UUID 的多个 inbound 合并）。

因此分层的成本比预期低：

1. 在 exporter 或 billing 侧引入 `inbound_tag → lane` 映射（`fast` / `vps`），映射表随节点配置下发
2. `traffic_minute_buckets` 增加 `lane` 列，聚合键从 `(node_id, account_uuid)` 扩为 `(node_id, account_uuid, lane)`
3. `aggregateSamplesByUUID` 改为按 `(uuid, lane)` 聚合，保留 lane 维度
4. 评率时只对 `lane = fast` 计费/扣配额

**待确认**：以 `inbound_tag` 作为判定依据是否成立，以及具体的 tag 命名约定。

### 对既有链路的影响

`traffic_minute_buckets` 加列是**向后兼容的**：存量行 `lane` 置为 `fast`（此前所有流量都按高速计），新行带真实 lane。checkpoint 表同理需要扩键，这一步要小心——[之前的 UUID 聚合 bug](../feature-xray-usage-billing-portal-uat/) 正是 checkpoint 维度与 Sample 维度不一致导致的。

## 时长维度

free 档的「高速流量每周 1 小时，其余时间降级 VPS」是**时间窗口配额**，不是字节配额。

### 口径问题（待确认）

「1 小时」有两种口径：

| 口径 | 含义 | 计量成本 |
| --- | --- | --- |
| **连接时长** | 从建立高速连接到断开的墙钟时间 | 需要会话级事件（连接建立/断开），Xray 侧需新增上报 |
| **有流量时长** | 有高速流量产生的分钟数 | **可直接从现有分钟桶推导**：该 UUID 在 `lane=fast` 上有非零字节的分钟数 |

**强烈建议后者**：分钟桶已经在写，「有流量的分钟数」是免费副产品，不需要任何新的计量源，且更贴近"实际使用"的直觉（挂着不用不该扣配额）。

### 数据模型

`account_quota_states` 现有 `remaining_included_quota`（bigint，字节）不足以表达时间配额。两种做法：

| 做法 | 说明 |
| --- | --- |
| A. 复用字节列 + features 解释 | `remaining_included_quota` 存"剩余秒数"，由 `features.fast_lane.mode` 决定单位语义 |
| **B. 新增列** | `remaining_fast_seconds bigint`，语义清晰 |

**建议 B**。A 的隐式语义切换正是这条链路上反复出问题的模式（同一字段在不同上下文含义不同），不值得为省一列付这个代价。

### 降级而非断线

窗口耗尽后**降级为 VPS 流量，不断线**。这意味着 `throttle_state` 需要一个新取值：

```
normal → fast_lane_exhausted → (下周期重置) → normal
```

Agent 侧需要能响应这个状态，把该用户的路由从高速出口切到 VPS 出口。这条控制链路（accounts → agent 策略下发）已经存在（`/api/account/policy` 的 `preferredStrategy` / `eligibleNodeGroups`），复用即可。

## 资源实例维度

资源服务卡片（创建类）与对象存储的计量**完全没有链路**，是本规划中最大的新建部分。

需要定义：

1. **计量源**：谁上报？资源编排侧（创建/销毁事件）还是周期性快照？
2. **计价模型**：
   - free：Demo 卡片，每天 1 次、每次 1 小时 → 次数 + 时长
   - PAYG：明码实价，按量
   - Pro：明码实价 + 20% 托管费
3. **生命周期**：PAYG 欠费停机后计算资源保留 7 天、对象存储保留 30 天后释放——需要一个到期释放的定时任务，且释放是**不可逆**操作，必须有充分的通知与审计。

**建议**：这部分单独拆一个规划文档，不与流量计费混在同一阶段交付。流量侧的商业化闭环（S0~S2）不依赖它。

## 配额周期与 Stripe 周期解耦

现有 `resetQuotaForPlan` 的周期边界来自 Stripe subscription 的 `current_period_start/end`，并有 `naturalMonthPeriod` 作为无订阅时的兜底。

**Pro 年付会踩坑**：Stripe 年付订阅的 `current_period` 是一年，直接沿用会导致配额一年才重置一次，而设计要求每自然月重置 20GB。

### 方案

把「计费周期」与「配额周期」显式分开：

| 概念 | 来源 | 用途 |
| --- | --- | --- |
| 计费周期 | Stripe `current_period_start/end` | 何时扣款、发票 |
| **配额周期** | `features.quota_cycle` | 何时重置 `remaining_included_quota` |

```jsonc
// PRO-YEARLY features 增加
{ "quota_cycle": "natural_month" }
```

`resetQuotaForPlan` 改为：`quota_cycle = natural_month` 时用 `naturalMonthPeriod`（已存在的函数）计算边界，忽略 Stripe 周期；否则沿用 Stripe 周期。月付两者恰好一致，行为不变。

同时需要一个**周期滚动任务**：年付订阅在 Stripe 侧一年只发一次 `invoice.paid`，中间 11 个月没有任何 webhook 驱动重置。需要 billing-service 增加一个定时器，扫描 `period_end < now()` 且订阅仍有效的账号，执行下一期重置。

## 超额计费

Pro 超出 20GB 后按 1 元/GB 计价。

### 扣费顺序

```
高速流量产生 → 先扣 remaining_included_quota
             → 配额耗尽后，按 1元/GB 从 current_balance 扣
             → 余额不足 → arrears = true, arrears_since = now()
```

这个顺序让 Pro 与 PAYG 共用同一套扣费代码：PAYG 只是 `included_quota_bytes = 0` 的特例，一上来就走余额扣减。

### 不上报 Stripe

超额**不**通过 Stripe metered usage 计费，理由见 [01](./01-plan-catalog.md#stripe-对象映射)。代价：超额金额不出现在 Stripe 发票上，需要在本地账单（Portal「详情」tab 的 ledger）体现，运营对账时以本地库为准。

## 欠费与停机策略

现有实现：`billing-service` 的 `SuspendSyncer` 按**全局统一阈值**把 `arrears_since` 超期的账号升级为 `suspend_state = suspended`。阈值是构造参数 `NewSuspendSyncer(repo, threshold, interval, log)`，测试里是 14 天。

**四档需要不同策略**：

| 档位 | 策略 | 阈值 |
| --- | --- | --- |
| free | 无欠费概念 | — |
| PAYG | 余额归零**立即**停机 | 0 |
| Pro | 宽限后停机 | 14 天 |
| Custom | 不自动停机，运营手动处理 | ∞ |

### 改造方向

`SuspendSyncer` 从"全局单一阈值"改为"按账号所属套餐取阈值"：

1. `features.dunning.policy` / `grace_days` 已在套餐目录中定义
2. SuspendSyncer 查询时 join `account_billing_profiles.package_name` → `billing_plans.features`
3. 阈值为 0 → 立即停机；为 `manual` → 跳过

保留全局阈值作为套餐未定义时的兜底，避免目录缺失导致"永不停机"这种静默失败。

### 停机后的资源保留

PAYG 停机后计算资源保留 7 天、对象存储 30 天。这需要：

- `suspend_state` 之外记录 `suspended_at`
- 到期释放的定时任务（与资源实例维度一并实现）
- 释放前的通知（邮件）

**释放是不可逆的**，必须有审计记录与至少一次提前通知。

## 存量账号补齐

实测 UAT 上 `account_billing_profiles` **没有任何行**——包括 `admin@svc.plus`。原因：`provisionOnboardingTrial` 只在注册 / OAuth 首次登录时触发，bootstrap 出来的账号从未走过那条路径。

Portal 显示「套餐 default、配额 0 B」就是没有 profile 行时前端的兜底文案。

需要一个**回填路径**：

| 方案 | 说明 |
| --- | --- |
| A. 启动时回填 | accounts 启动时扫描无 profile 的账号，按 free 档补齐 |
| B. 管理端接口 | 运营手动为指定账号指派套餐（`custom` 档也需要这个接口） |
| **C. 读时兜底** | 查询权益时若无 profile，按 free 档即时创建 |

**建议 B + C**：C 保证任何账号都有合理默认值、不再出现「配额 0 B」的困惑展示；B 满足运营指派 custom 档与人工补偿的需求。A 的一次性扫描在新账号从其他路径进入时仍会漏。
