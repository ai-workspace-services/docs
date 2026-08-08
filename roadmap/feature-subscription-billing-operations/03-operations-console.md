# 运营管理台与账单

## 现状

管理端目前只有三个能力：

| 接口 | 能力 |
| --- | --- |
| `GET/PUT/DELETE /api/auth/admin/billing/plans[/:planId]` | 套餐目录 CRUD |
| `POST /api/auth/admin/billing/accounts/:accountUUID/clear-arrears` | 清欠费 + 解除停机 |

也就是说：**能改价目表、能救一个欠费账号，此外没有任何运营视图**。没有订阅列表、没有账单查询、没有用量排行、没有收入看板、没有人工调额与补偿。

要支撑四档产品线的日常运营，这一层需要补齐。

## 能力清单

按优先级分三组。

### P0 — 没有就没法运营

| 能力 | 接口 | 说明 |
| --- | --- | --- |
| 账号权益查询 | `GET /admin/billing/accounts/:uuid` | 当前套餐、配额余量、余额、欠费与停机状态、订阅列表，一次返回 |
| 账号检索 | `GET /admin/billing/accounts?q=&plan=&state=` | 按邮箱/UUID 搜，按套餐、欠费、停机状态筛 |
| **手动指派套餐** | `POST /admin/billing/accounts/:uuid/plan` | 开通 `custom` 档、人工纠错的必需能力，当前完全缺失 |
| **手动调整余额** | `POST /admin/billing/accounts/:uuid/balance` | 补偿、退款入账、赠送。必须写 ledger 且带操作人与原因 |
| 订阅列表 | `GET /admin/billing/subscriptions?status=` | 全站订阅，按状态筛 |

### P1 — 运营效率与对账

| 能力 | 说明 |
| --- | --- |
| 账单明细导出 | 按账号 / 时间范围导出 ledger，CSV |
| 对账报表 | 本地 ledger 与 Stripe 发票的差异清单——因为超额不上报 Stripe，两边天然不等，需要能解释差额 |
| 欠费名单 | 当前欠费账号，按欠费时长排序，含距停机剩余时间 |
| 用量 TopN | 按周期的高速流量消耗排行，识别异常账号 |
| 操作审计 | 所有管理端写操作的日志：谁、何时、对谁、改了什么、原因 |

### P2 — 经营看板

| 指标 | 说明 |
| --- | --- |
| MRR / ARR | 按套餐拆分 |
| 活跃订阅数 | 新增 / 流失 / 净增 |
| 各档账号分布 | free / payg / pro / custom 占比与迁移漏斗 |
| 收入构成 | 订阅费 vs 超额费 vs 充值 |
| 配额利用率 | Pro 用户的平均配额使用率——过低说明定价偏高，过高说明该涨档 |

## 设计要点

### 审计是硬要求，不是加分项

管理端能改余额、改套餐、解除停机——这些直接对应真金白银。**每一个写操作都必须落审计**：操作人、时间、目标账号、变更前后值、原因（必填）。

审计表建议独立于 ledger：ledger 是账务事实，审计是操作记录，两者生命周期与查询模式都不同。

### 余额调整必须走 ledger

手动调余额不能直接 `UPDATE account_quota_states SET current_balance = ...`。必须写一条 ledger 分录（`entry_type = manual_adjustment`），让余额始终等于 ledger 累计——否则对账时无法解释差额，这正是账务系统最容易烂掉的地方。

### 与 Stripe 的职责边界

| 事项 | 在哪做 |
| --- | --- |
| 改支付方式、看 Stripe 发票、取消订阅 | **Stripe Billing Portal**（用户自助，已接） |
| 退款 | **Stripe Dashboard**（运营），本地通过 webhook 感知 |
| 权益调整、余额补偿、停机解除 | **本地管理台** |

不要在本地管理台重造 Stripe 已经做好的东西（发票、支付方式、退款流程）。本地只做 Stripe 不知道的那部分：权益与用量。

退款场景需要注意：Stripe 退款会发 `charge.refunded`，但**权益不会自动回收**。是否回收、回收多少，是业务决策，建议先只记录并告警，由运营人工处理。

### 权限模型

管理端接口复用现有 settings 权限（`permissionAdminSettings` 一类）。但**改余额**这类操作建议单独一个权限位——能改价目表和能给账号打钱不是同一个信任级别。

## Portal 侧

用户可见的账单能力目前是「订阅与计费」面板的概览/详情两个 tab。四档上线后需要补：

| 页面 | 内容 |
| --- | --- |
| 定价页 | 四档对比，从 `GET /api/billing/plans` 渲染，不硬编码 |
| 升级/降级流程 | free → payg 充值引导；→ pro 走 Checkout；custom 引导联系商务 |
| 余额与充值 | PAYG/Pro 用户的余额展示与充值入口 |
| 用量明细 | 已有 ledger 列表，需按 lane 区分高速/VPS |
| 欠费提醒 | 欠费状态的显著提示与距停机倒计时 |

现有 `PricingTeaser`、`BillingOptionsPanel`、`CheckoutStatusBanner` 组件已存在，多数是扩展而非新建。
