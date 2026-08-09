# 订阅、支付与运营账单

状态：`proposal`
范围：UAT 先行，生产按同一契约迁移
原则：只新增；不改动已打通的 Xray → Exporter → Billing → PostgreSQL → Accounts → Portal 计量链路语义。

## 目标

在已经跑通的**流量计量**之上，补齐**商业化闭环**：四档产品(free / Pay-As-You-Go / Pro / Custom)的权益定义、Stripe 支付与订阅生命周期、预充值余额、欠费停机策略，以及运营侧的账单与管理能力。

计量链路本身不动。本规划新增的是"钱"这一侧：谁买了什么、买了之后享有什么配额、用超了怎么算、没付钱怎么处理、运营怎么看和怎么干预。

## 文档

| 文档 | 内容 |
| --- | --- |
| [01-plan-catalog.md](./01-plan-catalog.md) | 四档套餐的完整定义、数据模型映射、Stripe 对象映射 |
| [02-metering-and-entitlements.md](./02-metering-and-entitlements.md) | 三个计费维度、配额周期、欠费与停机策略 |
| [03-operations-console.md](./03-operations-console.md) | 运营管理台与账单能力 |
| [05-stripe-catalog-automation.md](./05-stripe-catalog-automation.md) | Product/Price/Webhook 的 IaC 化：声明式配置 + 幂等同步脚本，换 Stripe 账号只需换密钥重跑 |
| [06-management-console-integration.md](./06-management-console-integration.md) | 落到真实 `/panel/management` 页面：扩展现有用户表、新增账号分类标签（独立于路由用的 `groups`） |
| [07-existing-user-migration.md](./07-existing-user-migration.md) | 生产环境存量用户的迁移方案：过渡套餐、配额估算、回填脚本、时序要求 |
| **[08-status-and-handoff.md](./08-status-and-handoff.md)** | **当前进度、UAT 实测数据、阻塞项、已踩过的坑——接手前先读这篇** |
| [09-trial-grants-and-topup-gap.md](./09-trial-grants-and-topup-gap.md) | 发放试用额度接口；以及**充值成功但余额不入账**这个代码缺口 |
| [04-delivery-phases.md](./04-delivery-phases.md) | 分阶段实施与验收 |

## 现状盘点（2026-08-05 实测）

代码侧**基本已建成**，缺的主要是配置、目录数据与运营闭环，不是从零开发。

| 层 | 已建成（main） | UAT 实况 |
| --- | --- | --- |
| Stripe 客户端 | checkout session / billing portal / webhook 验签 / 订阅查询取消 / 客户查找 | ❌ `STRIPE_SECRET_KEY`、`STRIPE_WEBHOOK_SECRET`、`STRIPE_ALLOWED_PRICE_IDS` 全空 → client disabled，Stripe 功能整体不可用 |
| accounts 路由 | `POST /stripe/checkout`、`/stripe/portal`、`POST /api/billing/stripe/webhook`、`GET /api/billing/plans`、admin 套餐 CRUD、admin 清欠费 | 路由在，但因上一行不可用 |
| 权益同步 | `applyPlanEntitlements` / `resetQuotaForPlan` / `markAccountArrears` / `downgradeToFreePlan` / `supersedeActiveTrials` / 新用户 `TRIAL-7D` 自动发放 | ❌ `account_billing_profiles` 无任何行；`subscriptions` 0 行 |
| 套餐目录 | 表结构 + 管理端 CRUD 齐全 | ⚠️ 仅 `TRIAL-7D`、`FREE`，`stripe_price_id` 均为空；无付费套餐 |
| portal 前端 | checkout/portal 代理路由、`SubscriptionPanel`、`BillingOptionsPanel`、`CheckoutStatusBanner`、`PricingTeaser` | 页面在，点「管理 Stripe 账单」会失败 |
| 流量计量 | 全链路已通 | ✅ 已验证（7.47 MB，含小时/24h/月三档聚合） |
| billing-service | 按设计**完全不碰 Stripe**，只做计量与欠费升级 | ✅ 正常 |

Portal 上「套餐 default、配额 0 B、暂无订阅记录」即以上三处 ❌ 的直接结果。

## 关键决策

1. **Stripe 只与 accounts 对话**。billing-service 永不直连 Stripe，继续只负责计量、评率与配额扣减。这条边界沿用既有设计，本规划不改。
2. **套餐目录是唯一权益事实源**。`billing_plans` 决定配额与功能开关；`STRIPE_ALLOWED_PRICE_IDS` 仅作目录为空时的引导期兜底。
3. **权益写入走 webhook 驱动的同步**，不在前端下单成功时抢先写库——避免"付款未落地但权益已发"。
4. **利用现有 `features` / `price_multipliers` 两个 jsonb 列承载新维度**，四档套餐的差异化能力优先用它们表达，避免为每个新特性加列。
5. **计量维度从一维扩到三维**（流量 / 时长 / 资源实例），但**分阶段落地**：先流量分层，再时长，最后资源卡片。

## 必须先拍板的问题

以下问题的答案会显著改变工作量，实施前需要确认（详见 01/02 文档中的对应小节）：

1. **Pro 年付的配额周期**（[01](./01-plan-catalog.md#pro-年付的配额周期)）——「每年累计 240GB」与「每个自然月循环」两种读法工作量差别很大。文档按「每自然月 20GB、共 12 期」实现，需确认。
2. **高速流量与 VPS 流量的判定依据**（[02](./02-metering-and-entitlements.md#流量分层)）——此前明确决定「先不区分」，本设计要求区分。需要确认以 Xray `inbound_tag` 作为判定依据。
3. **Free 档「每周 1 小时高速」的计量口径**（[02](./02-metering-and-entitlements.md#时长维度)）——是连接时长还是有流量的时长？前者需要新的会话计量源。
4. **预充值的充值入口**（[01](./01-plan-catalog.md#pay-as-you-go)）——Stripe 一次性支付充值到 `current_balance`，还是走 Stripe Customer Balance？
5. **资源服务卡片的计量源**（[02](./02-metering-and-entitlements.md#资源实例维度)）——目前完全没有这条链路，需要确认由谁上报。
6. **存量生产用户的迁移规则**（[07](./07-existing-user-migration.md)）——生产已有几十个真实用户从未走过计费流程，文档按「过渡套餐 + 按历史用量估算配额」设计，需要确认这批账号的准确名单与是否需要主动通知。这一步是 S2（[04](./04-delivery-phases.md)）的前置条件，不是可选项。

## 与既有规划的关系

本目录承接 [feature-xray-usage-billing-portal-uat](../feature-xray-usage-billing-portal-uat/)：那份规划解决"用量怎么算出来、怎么展示"，本份解决"用量怎么变成钱、钱怎么变成权益"。两者共用同一套 PostgreSQL 事实表与 Accounts 聚合层。
