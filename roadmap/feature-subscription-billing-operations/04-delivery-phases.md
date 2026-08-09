# 分阶段实施与验收

排序原则：**先让钱能收进来**（配置为主、几乎不写代码），再补权益正确性，最后做运营与新维度。每个阶段独立可验收、可回滚。

## S0 · 点亮支付主链路

**几乎不写业务代码，主要是配置。**

| 步骤 | 内容 |
| --- | --- |
| 1 | ✅ 已完成：Vault `kv/billing-service` 的 sandbox 密钥经 `web_saas_host_config` 注入 `secrets.env`（[playbooks#260](https://github.com/ai-workspace-infra/playbooks/pull/260) + [platform-ops-toolkit#277](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/277)）。放进了 optional-secrets 脚本、对 403/404 都容忍，不会因为 Vault 策略未授权而砸整个部署 |
| 1a | 待确认：`github-actions-platform-ops-toolkit-uat` 角色是否已有 `kv/data/billing-service` 读权限 |
| 2 | ✅ 已完成（IaC 化）：`accounts/scripts/stripe-sync-catalog.sh` + `stripe-catalog.yaml` 幂等创建 Product「Pro」+ 两个 Price（¥20/月、¥200/年）+ 充值一次性 Price，见 [05-stripe-catalog-automation.md](./05-stripe-catalog-automation.md)。不再手工点 Dashboard |
| 3 | 同一脚本创建/同步 Webhook endpoint，指向 `https://accounts-uat.onwalk.net/api/billing/stripe/webhook`，订阅 6 类事件（见下） |
| 4 | 用脚本输出的 `plan_id → stripe_price_id` 对照表，经 admin 接口写入 `FREE` / `PAYG-TOPUP-*` / `PRO-MONTHLY` / `PRO-YEARLY` 目录 |

**需订阅的 webhook 事件**：
`checkout.session.completed`、`customer.subscription.created`、`customer.subscription.updated`、`customer.subscription.deleted`、`invoice.paid`、`invoice.payment_failed`

**验收**：`GET /api/billing/plans` 返回四档；`docker exec ... printenv STRIPE_SECRET_KEY` 非空；Portal「管理 Stripe 账单」能跳转到 Billing Portal。

## S1 · 权益闭环验证

用 Stripe 测试卡把生命周期跑一遍，验证既有 `entitlements.go` 的同步逻辑在真实 webhook 下成立。

| 场景 | 触发 | 期望 |
| --- | --- | --- |
| 首次订阅 | 测试卡下单 PRO-MONTHLY | `subscriptions` 有行；profile `package_name=pro`；`remaining_included_quota = 20GB`；Portal 配额非 0 |
| 续期 | Stripe CLI 触发 `invoice.paid` | 配额重置为 20GB，`period_start/end` 前推一个月 |
| 支付失败 | 触发 `invoice.payment_failed` | `arrears = true`，`arrears_since` 落值 |
| 重复失败 | 再次触发 | `arrears_since` **不前推**（同一欠费周期） |
| 恢复 | 触发 `invoice.paid` | `arrears` 清除，配额重置 |
| 退订 | 取消订阅 | `downgradeToFreePlan`，落回 free |

**验收**：以上六条全部在 UAT 实测通过，且 Portal 的展示与库中状态一致。

## S1.5 · 存量生产用户迁移（S2 前置，不可跳过）

生产已有几十个真实用户从未走过计费流程。任何分档欠费/停机策略（S2 的一部分）一旦上线，这批账号必须已经有合法的 `account_billing_profiles`/`account_quota_states` 行，否则要么被现有 UAT 上观察到的"无 profile"异常状态影响，要么被兜底阈值误伤——对一批已经在正常使用服务的真实用户造成事故。

详见 [07-existing-user-migration.md](./07-existing-user-migration.md)：摸底 → 用量画像 → 幂等回填脚本（过渡套餐 `LEGACY-GRANDFATHERED`，按历史用量估算配额，`legacy` 标签打标）→ 验证。

**这一步只作用于生产 svc.plus，且需要你明确批准后才执行**——与本规划其余在 UAT 上完成的步骤不同。

**验收**：摸底 SQL 产出确切名单（替换掉"几十个"这个估计值）；回填后管理台按 `legacy` 标签筛选，抽查套餐/配额/分类符合预期；Portal 面板不再出现空态。

## S2 · 权益正确性补齐

到这一步才需要真正写代码。

| 项 | 内容 | 依据 |
| --- | --- | --- |
| 存量账号回填 | 读时兜底（无 profile 按 free 创建）+ 管理端手动指派接口 | [02](./02-metering-and-entitlements.md#存量账号补齐) |
| 配额周期解耦 | `features.quota_cycle`；年付按自然月重置；billing-service 增加周期滚动定时器 | [02](./02-metering-and-entitlements.md#配额周期与-stripe-周期解耦) |
| 分档欠费策略 | SuspendSyncer 按套餐取阈值；PAYG 0 宽限、Pro 14 天、custom 手动 | [02](./02-metering-and-entitlements.md#欠费与停机策略) |
| 预充值 | 充值 Checkout + `checkout.session.completed` 入账 `current_balance`，写 ledger | [01](./01-plan-catalog.md#充值入口待确认) |
| 超额扣费 | 配额耗尽后按 1元/GB 扣余额；余额不足转欠费 | [02](./02-metering-and-entitlements.md#超额计费) |

**验收**：PAYG 账号余额扣到 0 立即停机；Pro 账号超额从余额扣、余额不足才欠费；年付账号跨自然月配额正确重置。

## S3 · 流量分层

把高速与 VPS 流量分开计量——这是 free 降级与"只对高速收费"的前提。

| 步骤 | 内容 |
| --- | --- |
| 1 | 确认 `inbound_tag → lane` 的判定依据与命名约定 |
| 2 | `traffic_minute_buckets` 增加 `lane` 列，存量行回填为 `fast` |
| 3 | checkpoint 键从 `(node_id, account_uuid)` 扩为含 lane；**这一步风险最高**，需参照此前 UUID 聚合 bug 的教训 |
| 4 | 评率只对 `lane = fast` 扣配额/计费 |
| 5 | Portal ledger 与用量明细按 lane 区分展示 |

**验收**：同一账号同时产生高速与 VPS 流量时，只有高速部分扣配额；两种流量在 Portal 上分别可见。

## S4 · 时长维度与 free 降级

| 步骤 | 内容 |
| --- | --- |
| 1 | 确认「1 小时」口径（建议：有高速流量的分钟数，可从分钟桶直接推导） |
| 2 | `account_quota_states` 增加 `remaining_fast_seconds` |
| 3 | `throttle_state` 增加 `fast_lane_exhausted` |
| 4 | agent 策略下发响应该状态，把用户路由切到 VPS 出口 |
| 5 | 每周重置任务 |

**验收**：free 账号用满 1 小时高速后自动降级为 VPS 且**不断线**；下周期自动恢复。

## S5 · 运营管理台

按 [03](./03-operations-console.md) 的 P0 → P1 → P2 推进。P0 是运营能开工的下限（账号权益查询/检索、手动指派套餐、手动调余额、订阅列表），建议与 S2 并行——S2 做的很多能力正好需要 P0 的接口来验证。

**验收**：运营能独立完成「开通一个 custom 客户」「给一个投诉用户补偿余额」「查清某账号为何被停机」三件事，全程不需要工程介入、且全部留下审计记录。

## S6 · 资源实例维度

资源服务卡片与对象存储的计量、计价、生命周期释放。**建议单独立项**，不与流量商业化混在一起交付——它需要全新的计量源、涉及不可逆的资源释放，风险与工作量都与前面几个阶段不是一个量级。

前置：计量源确定（谁上报创建/销毁事件）、计价模型确定、释放前通知与审计机制确定。

## 依赖关系

```
S0 ──> S1 ──> S1.5(生产, 需批准) ──> S2 ──┬──> S3 ──> S4
                                        └──> S5(P0 可与 S2 并行)

S6 独立立项
```

S3/S4 不阻塞 S2：Pro 与 PAYG 的商业化闭环在"不分层"的前提下已经成立（现状即所有流量按高速计），分层是让 free 档的降级设计成立的前提。

## 风险

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| checkpoint 扩键（S3） | 计量错乱、重复计费或漏计 | 参照 UUID 聚合 bug 的教训；先在 UAT 用合成流量验证幂等 |
| 年付配额周期 | 一年只重置一次，用户实际少拿 11 个月配额 | S2 必须在首个年付订阅产生**之前**完成 |
| 超额不上报 Stripe | 本地账单与 Stripe 发票天然不等 | 对账报表必须能解释差额；上线前与财务对齐口径 |
| 资源释放不可逆（S6） | 误删客户数据 | 提前通知 + 审计 + 释放前二次确认 |
| Stripe sandbox → 生产切换 | 密钥/价格 ID 错配导致收错钱 | 生产切换单独走一次完整的 S0+S1 验收 |
| 存量用户回填配额过紧（S1.5） | 已在正常使用的真实用户被误判超额/降速，造成事故 | 按历史用量 2× 系数设定过渡配额；`overage.policy=none`，观察期内不因超额产生任何实质影响 |
| 存量用户回填配额过松/漏迁移 | 部分账号在 S2 分档欠费策略上线后被兜底阈值误伤 | S1.5 必须在 S2 之前完成且验收通过；回填脚本幂等，可反复核对"是否所有存量账号都已有 profile" |
