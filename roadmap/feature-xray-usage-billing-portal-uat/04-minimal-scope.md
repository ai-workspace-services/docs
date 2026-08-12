# 最小实现方案(2026-08-01 定稿)

> 本文是 [03-audit-2026-08-01.md](./03-audit-2026-08-01.md) 审计后,按**实际需求收敛**的可执行方案。
> **实际需求**:每月订阅配额总量 + 按量使用统计,供账单与计费/服务对接。
> **明确约束**:最小改动、不过度设计、先不区分高速/普通流量、设计稿仅作效果参考。
> **部署事实**:当前 = 单 xray 实例 **多 inbound**;未来 = 多 xray 实例多 inbound;**统计口径按 邮箱/UUID 聚合总量**。

---

## 一、结论:大部分能力已存在,只需 2 个必做改动

| 需求 | 现状 | 需要做什么 |
|---|---|---|
| 配额总量 | `account_billing_profiles.included_quota_bytes` ✅ | 无 |
| 剩余配额 | `account_quota_states.remaining_included_quota` ✅(Billing 实时扣减) | 无 |
| 用量明细 | `traffic_minute_buckets` ✅ | 无 |
| 账本 | `billing_ledger` ✅ | 无 |
| 订阅联动重置 | P1 entitlement sync 在 `invoice.paid` 重置配额 ✅ | 无 |
| 查询接口 | `/api/account/usage/summary` ✅ | 补字段(纯增) |
| Portal 消费 | `fetchAccountUsage.ts` 已读这些字段 ✅ | 加一张卡 |
| **多 inbound 下计量正确** | ❌ **当前有数据损坏缺陷** | **必做 #1** |
| **本期边界(每月)** | ❌ 无周期字段 | **必做 #2** |

---

## 二、必做 #1:修复多 inbound 计量缺陷(P0,先于一切)

### 问题(当前正在发生)

- Exporter 聚合键 = `(uuid, inbound_tag)`(`xray-exporter/internal/service/service.go:252`)→ **单实例多 inbound 时,一次 snapshot 中同一用户有多条 Sample,各自持有独立累计值**
- Billing checkpoint 键 = `(node_id, account_uuid)`(`billing-service/sql/billing-service-schema.sql:17`),**不含 inbound_tag** → 这些样本共用一行 checkpoint,互相覆盖

```
Sample(inbound=A, up=100) → checkpoint 空 → delta=100 → 写 bucket → checkpoint=100
Sample(inbound=B, up=50 ) → checkpoint=100 → delta=-50 → 误判"计数器重置" → 丢弃该样本
下一轮 (A, up=120) → checkpoint=50 → delta=70(真实应为 20)→ 超计 50
下一轮 (B, up=60 ) → checkpoint=120 → delta=-60 → 又一次假重置 → 再丢弃
```

**后果**:交替出现丢数据与超额计费,`reset_epoch` 无限增长。计费金额与配额扣减都不可信。

### 最小修法:**按 UUID 聚合后再算 delta**(零 schema 改动)

与"按邮箱/UUID 聚合统计总量"的口径完全一致。

**改动点**:`billing-service/internal/service/service.go` 的 `processSnapshot`(第 213-225 行),在遍历前先按 UUID 合并:

```go
// 单实例多 inbound 时,Exporter 会为同一 UUID 输出多条样本(每 inbound 一条)。
// 计费口径按用户聚合总量,因此在计算 delta 前先按 UUID 合并各 inbound 的累计值,
// 否则它们会共用同一行 checkpoint 互相覆盖,产生假重置与超计。
aggregated := map[string]model.Sample{}
order := make([]string, 0, len(snapshot.Samples))
for _, s := range snapshot.Samples {
    uuid := strings.TrimSpace(s.UUID)
    if uuid == "" { continue }
    cur, seen := aggregated[uuid]
    if !seen {
        cur = model.Sample{UUID: uuid, Email: s.Email}
        order = append(order, uuid)
    }
    cur.UplinkBytesTotal   += s.UplinkBytesTotal
    cur.DownlinkBytesTotal += s.DownlinkBytesTotal
    aggregated[uuid] = cur
}
// 之后按 order 遍历 aggregated 调用 processSample
```

配套:`processSample` 里 `LineCode` 由 `sample.InboundTag` 改为固定值(建议 `""`,表示"账户聚合口径")。

**为什么这是最优解**
- ✅ 零 schema 改动、零迁移
- ✅ 与需求口径一致(按 UUID 统计总量,先不区分线路)
- ✅ 未来多 xray 实例天然兼容:每实例一个 exporter → 独立 `node_id` → checkpoint 键 `(node_id, uuid)` 仍唯一,聚合在各自 snapshot 内进行
- ✅ 一个 inbound 消失(xray 重启丢 tag)时总和下降 → 走既有 reset 逻辑,不回冲账本(保守,方向是少计)

**放弃的能力**:按线路(inbound)拆分用量。当前明确不需要;**未来若要区分高速/普通流量,再把 checkpoint 主键扩为 `(node_id, account_uuid, line_code)`** 并保留本次聚合逻辑作为可选口径。

### 回归测试(必须补)

1. 同一 UUID、同节点、**3 个 inbound_tag** 的 snapshot → 断言只写 1 条 bucket、`total = 三者之和`、`reset_epoch` 不增长
2. 连续 3 轮上述 snapshot → 断言累计用量 == 各 inbound 增量之和(**当前实现会失败,可用作缺陷复现**)
3. 其中一个 inbound 消失 → 断言走 reset 分支、不产生负用量、账本不回冲
4. 重复投递同一窗口 → `WrittenMinutes=0 / ReplayedMinutes>0`,配额不二次扣减

---

## 三、必做 #2:补计费周期边界(支撑"每月")

### 问题

`account_quota_states` 无任何周期字段(只有 `effective_at`/`updated_at`/`last_rated_bucket_at`),因此答不出「**本月**已用」与「重置日期」。

### 最小修法

**① DB:加 2 列**(幂等,可直接进 `billing-service-schema.sql` 与迁移脚本)

```sql
ALTER TABLE public.account_quota_states
  ADD COLUMN IF NOT EXISTS period_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS period_end   TIMESTAMPTZ;
```

**② accounts:重置配额时写入周期**

`accounts/api/entitlements.go` 的 `resetQuotaForPlan`:写 `remaining_included_quota` 的同时写入本期区间。
- 有 Stripe 订阅:取 subscription 的 `current_period_start` / `current_period_end`
- 无订阅(FREE/TRIAL):自然月兜底(当月 1 日 00:00 UTC → 次月 1 日),或 trial 起始 +7d

**③ 注意字段所有权**:`period_*` 由 **Accounts 写**(权益侧),Billing 只读。与 §5.2 既有约定一致(Accounts 重置权益,Billing 推进消耗)。

---

## 四、可选(不阻塞,按需再做)

**A. `usage/summary` 补字段** —— **只加不改**,不得重命名现有字段

现有字段 `totalBytes` / `remainingIncludedQuota` / `billingProfile` **必须保留**(Portal `fetchAccountUsage.ts:9,14` 正在消费,改名即破坏现网)。新增:

```
includedQuotaBytes   // = profile.included_quota_bytes
usedBytes            // = included - remaining   ← O(1),无需扫表
usagePercent
periodStart / periodEnd
```

> 关键:**"本期已用"用 `included − remaining` 直接得到,不需要聚合查询** —— 这也是本方案不需要 hourly/daily 滚存表的原因。

**B. Portal 配额卡** —— 在既有 `SubscriptionPanel` / `UserOverview` 内新增一张卡,读上述字段。新字段缺失时显示 `—`,不得影响二维码、订阅、账户安全区块。

**C. 分钟桶保留策略** —— 加一条定时清理(建议保留 14~30 天),防长期膨胀。一条 cron 即可,不需要分区表。

---

## 五、明确不做(避免过度设计)

| 项 | 理由 |
|---|---|
| ❌ **拆分 Billing 专属 DB** | 共库最优(论证见审计 C0)。`BILLING_DATABASE_URL` 保持复用 `account`;若要权限隔离,建 `billing_user` 按表 GRANT 即可,**纯配置零代码**。拆库会打断 5 个 FK、entitlement 写路径、认证中间件与 Xray 下发读路径,并使审计 §10 故障隔离矩阵失效 |
| ❌ 双配额(高速/普通流量) | 明确先不区分。未来要做时:checkpoint 加 `line_code` + 配额维度表 |
| ❌ hourly/daily 滚存表 | 本期已用是 O(1) 计算,不需要 |
| ❌ `/api/account/timeseries` `/nodes` `/audit` | 设计稿仅效果参考,非当前需求 |
| ❌ Observability 接入 | 旁路,不阻塞计费,后置 |
| ❌ UAT 不可变 tag 改造 | 与 `IMAGE-TAG-CONTRACT.md`「UAT 恒用 latest」冲突,需平台层裁决,不阻塞本功能 |

---

## 六、实施顺序与工作量

| 步骤 | 内容 | 预估 | 阻塞关系 |
|---|---|---|---|
| 1 | **必做 #1** 按 UUID 聚合 + 4 个回归测试 | 0.5~1 天 | **最先做**,独立缺陷修复 |
| 2 | **必做 #2** 加 2 列 + entitlement sync 写周期 | 0.5 天 | 独立 |
| 3 | 可选 A:`usage/summary` 补字段 | 0.5 天 | 依赖 2 |
| 4 | 可选 B:Portal 配额卡 | 0.5 天 | 依赖 3 |
| 5 | 可选 C:分钟桶保留 cron | 0.2 天 | 独立 |

**合计约 2~3 天**,全部为加法改动,无破坏性变更,不触碰 Xray 配置生成、Agent 心跳、登录、MFA、二维码、订阅流程。

---

## 七、验收要点

1. **计量正确性**(核心):构造多 inbound 流量 → Exporter snapshot 中同 UUID 多条样本 → `collect-and-rate` 后 `traffic_minute_buckets` 中该用户当分钟**总量 = 各 inbound 增量之和**,`reset_epoch` 不变
2. 重复执行 `collect-and-rate` → 不产生重复账本、配额不二次扣减
3. `invoice.paid` 后 → 配额重置、`period_start/end` 前移到新周期
4. `usage/summary` 仍返回 `totalBytes` 与 `remainingIncludedQuota`(防回归)
5. Billing 停机 → Xray 配置同步、Agent 心跳、用户登录、二维码渲染**均不受影响**
6. UAT 不连生产 PG、不用生产 Stripe 凭据
