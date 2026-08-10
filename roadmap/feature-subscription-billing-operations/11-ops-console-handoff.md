# Handoff Spec: 运营控制台 `/panel/ops`

开发交接规格。设计意图与分期见 [10-ops-console-design.md](./10-ops-console-design.md)，这份是可直接照着写代码的实现细节。

技术栈：Next.js App Router + React + Tailwind + SWR + Zustand（`useUserStore`）。

## 接手基线（2026-08-10）

本计划以 UAT 为唯一验收环境：<https://console-uat.onwalk.net/>。所有涉及余额、充值、价格发布的验证都先使用 Stripe 测试模式和专用测试账号；未通过 UAT 验收前，不把运营入口指向生产。

当前 UAT 主机：`root@167.179.64.91`。主机地址仅用于本次环境排查和验收；若后续流水线重建环境，以部署输出和 DNS gate 结果为准，不要默认沿用该 IP。

UAT 部署证据：[`platform-ops-toolkit` run #31367412474](https://github.com/ai-workspace-infra/platform-ops-toolkit/actions/runs/31367412474)（`platform-ops.yaml`，`main@e1511df`，状态 `Success`）。该流水线包含 UAT DNS gate、应用/基础设施部署以及 PROD → UAT 账号数据迁移；后续验收记录应继续挂在同一条部署证据链上。

当前判断：

- Portal 已落地 `/panel/ops` Build 2：运营工作台、账号处置台和计费运营总览可以通过独立运营路由访问；侧栏修复已随 [portal PR #172](https://github.com/ai-workspace-services/portal/pull/172) 合入，套餐目录与审计页提交为 `f10360c`，对应待验收的 [portal PR #176](https://github.com/ai-workspace-services/portal/pull/176)。Accounts 套餐写入原因强制校验对应 [accounts PR #66](https://github.com/ai-workspace-services/accounts/pull/66)。
- 当前实现的访问边界是 `root`（归一化为 `admin`）、`admin`、`operator`；普通用户由页面 guard 和 BFF 双重拦截。MFA 未完成时，运营入口按预期跳转到 `/panel/account?setupMfa=1`。
- 账号详情所需的后端 P0 数据契约已经具备：账号全景、指派套餐、调整配额、调整余额、发放试用、清欠费、审计流水；前端不再自行拼接数据库字段。账号列表 BFF 当前复用受保护的 `/api/auth/users`，避免调用尚未提供的聚合路径。
- PAYG 充值入账是独立的发布闸门。候选实现即使已有单元测试，也必须在 UAT 验证「一次支付 → 余额 + ledger → 重复 webhook 不重复入账」后，才算完成。
- `/prices` 已经读取实时套餐目录，因此套餐上架、下架和 Stripe 价格关联属于真实运营变更，必须走更高等级的权限和审计。

今天的 UAT 只读核对结果：

- `admin@svc.plus` 登录态可以进入 `/panel/ops`、`/panel/ops/accounts`、`/panel/ops/billing/ledger`；UAT 展示 16 个账号、16 个活跃订阅。
- MRR、欠费、账单例外、资金流和审批队列在数据未闭环时均显示「待同步」或明确空态，没有用 `0` 或推算值冒充真实金额。
- 账号处置台的「调整余额」弹窗要求填写操作原因；空原因点击确认会被拦截，未发起写请求。
- UAT 当前部署版本仍把「资源运维」放在用户中心侧栏顶部，尚未包含 PR #172 的底部收纳修复；这不是本地代码验证失败，而是 UAT 尚未部署该 PR 的证据。合入并部署后必须重新验收侧栏顺序。
- UAT 本次未切换普通用户账号做真实登录验证；普通用户阻断已由 Portal 权限单测覆盖，部署后仍需用专用普通用户账号补做一次 UAT 访问验收。

本地与线上环境必须严格分离：

- `Dev` 只连接本地 Portal、Accounts、Billing、Docs 和本地 Postgres；禁止为了验证页面临时把本地 Portal 指向 UAT Accounts。
- `SIT`、`UAT`、`PROD` 使用各自的运行时配置、数据库和账号服务；不得复用 session、cookie、测试账号或数据库连接串。
- 本地 `make dev` 负责回收 stale Portal/Accounts 进程与 `.next/dev/lock`，并复用隔离的本地测试容器；端口冲突先检查本地进程，不要修改线上配置绕过。

本文件是前端交接计划；后端接口以 Accounts 的实际路由和响应体为准。若接口响应与本文示例不一致，先统一契约和 BFF，再写页面，禁止在组件内兼容多套字段名。

## Overview

面向 **admin / operator** 的独立运营控制台。当前 `/panel/management` 把权限矩阵和首页视频配置放在同一页，已经成为杂物间——本控制台把**运营职责**独立出来，并把那两块按归属拆走。当前 Build 2 已可供 UAT 只读验证；`/panel/management` 的完整拆分仍未完成。

进入路径：侧边栏 `admin` 分组（[portal#167](https://github.com/ai-workspace-services/portal/pull/167) 已取消 `hidden`，admin/operator 及由组继承者可见）。

## 路由

| 路由 | 页面 | 迁移来源 |
| --- | --- | --- |
| `/panel/ops` | 概览：MRR、活跃订阅、欠费名单、用量 TopN、用户趋势 | 新建（复用 `OverviewCards` / `TrendChart`） |
| `/panel/ops/accounts` | 账号检索 → 列表 | 新建 |
| `/panel/ops/accounts/:uuid` | 目标单账号操作台：指派套餐、发试用、调余额、调配额、清欠费 | 新建；当前 Build 2 通过 `/panel/ops/accounts?uuid=:uuid` 打开 |
| `/panel/ops/billing/plans` | 套餐目录 CRUD + 发布 `/prices` | 新建（后端接口已存在） |
| `/panel/ops/billing/ledger` | 账单明细、导出、对账 | 新建 |
| `/panel/ops/system` | 权限矩阵、黑名单 | **从 `/panel/management` 迁出** |
| `/panel/ops/audit` | 审计流水检索 | 新建 |

`/panel/management` **保留**，改为仅承载「首页视频配置」（内容运营，不属于本控制台），并在页首放一条指向 `/panel/ops` 的迁移提示。不要一次性删除——会打断正在使用的人。

### 路由注册

沿用扩展机制，在 `src/modules/extensions/builtin/user-center/index.ts` 的 `routes` 数组中登记：

```ts
{
  id: "ops",
  path: "/panel/ops",
  label: "Operations",
  description: "运营与计费管理",
  icon: Settings,          // lucide-react
  loader: () => import("./routes/ops"),
  match: "startsWith",     // 子路由由页面内部导航承担
  guard: {
    requireLogin: true,
    roles: ["admin", "operator"],
    permissions: ["admin.settings.read", "admin.users.list.read"],
  },
  redirect: { unauthenticated: "/login", forbidden: "/panel" },
  sidebar: { section: "admin", order: 10 },
}
```

`match: "startsWith"` 让所有 `/panel/ops/*` 都命中同一守卫，子页不必各自声明。

## Design Tokens

**全部来自 `src/app/globals.css` 的 CSS 自定义属性，不要写死色值。** 暗色主题通过 `<html class="dark">` 切换（`ThemeProvider.tsx`），使用 token 才能自动适配。

| Token | 值（亮色） | 用途 |
| --- | --- | --- |
| `--color-surface` | `#ffffff` | 卡片底 |
| `--color-surface-muted` | `#f2f5f8` | 表头、次级底色 |
| `--color-surface-hover` | `#edf2f7` | 行 hover |
| `--color-surface-border` | `rgba(166,180,200,0.18)` | 卡片/表格描边 |
| `--color-heading` | `#1c1b1f` | 标题、指标数值 |
| `--color-text` | `#1c1b1f` | 正文 |
| `--color-text-muted` | `#667085` | 次要说明 |
| `--color-text-subtle` | `#98a1b2` | 占位、辅助文字 |
| `--color-primary` | `#0058bd` | 主按钮、链接、选中态 |
| `--color-primary-hover` | `#0b4f9a` | 主按钮 hover |
| `--color-primary-muted` | `#e8f0fb` | 选中背景、信息底 |
| `--color-success` / `-muted` / `-foreground` | `#16a34a` / `#dcfce7` / `#166534` | 正常、已付 |
| `--color-warning` / `-muted` / `-foreground` | `#8f4a00` / `#fff3cd` / `#664d03` | 欠费、限速 |
| `--color-danger` / `-muted` / `-foreground` | `#c3655c` / `#fee2e2` / `#7f1d1d` | 停机、危险操作 |
| `--radius-lg` / `--radius-xl` / `--radius-pill` | `.375rem` / `.5rem` / `999px` | 输入框 / 卡片 / 徽章 |
| `--shadow-sm` / `--shadow-md` | — | 卡片 / 弹窗 |
| `--type-heading-1-size` | `1.5rem` | 页标题 |
| `--type-heading-2-size` | `1.25rem` | 区块标题 |
| `--type-body-size` | `0.8125rem` | 正文（注意：全站基准偏小） |
| `--type-meta-size` | `0.75rem` | 表格辅助列、时间戳 |

Tailwind 中以 `bg-[color:var(--color-surface)]`、`text-[var(--color-heading)]` 形式引用，与现有页面写法一致。

## 复用组件

**不要重写这些，它们已存在且被测试覆盖。**

| 组件 | 路径（`src/modules/extensions/builtin/user-center/`） | 复用位置 |
| --- | --- | --- |
| `Card` | `components/Card.tsx` | 所有区块容器 |
| `AccountSection` | `components/AccountSection.tsx` | 带 eyebrow/title/description 的分区 |
| `OverviewCards` | `management/components/OverviewCards.tsx` | **概览页用户分类指标** |
| `TrendChart` | `management/components/TrendChart.tsx` | **概览页用户趋势** |
| `PermissionMatrixEditor` | `management/components/PermissionMatrixEditor.tsx` | 迁到 `/panel/ops/system` |
| `EmailBlacklist` | `management/components/EmailBlacklist.tsx` | 迁到 `/panel/ops/system` |
| `UserGroupManagement` | `management/components/UserGroupManagement.tsx` | 账号列表基础（需扩展，见下） |

### `OverviewCards` 已覆盖用户分类

现有 `MetricsOverview` 契约恰好对应需求里的用户分类：

```ts
type MetricsOverview = {
  totalUsers: number       // 注册用户
  subscribedUsers: number  // 订阅用户
  activeUsers: number      // 活跃用户
  newUsersLast24h: number  // 近 24 小时新增
}
```

**运营用户 / 内测用户**需要扩展该类型 + 后端 `GET /admin/users/metrics` 一并返回：

```ts
operatorUsers: number   // segments 含 ops
betaUsers: number       // segments 含 beta
```

分类约定：`registered` 是注册基线；`subscribed` 由生效付费订阅派生、不可人工编辑；`ops` 表示运营/客服内部账号；`beta` 表示内测账号；`legacy` 表示存量迁移账号。页面上「运营用户」显示 `ops`，不要新增裸 `operator` 标签——它与运营角色容易混淆，也可能触发权限继承。

### `TrendChart` 数据契约

```ts
type MetricsPoint  = { period: string; total: number; active: number; subscribed: number }
type MetricsSeries = { daily: MetricsPoint[]; weekly: MetricsPoint[] }
```

组件内置 `daily | weekly` 粒度切换（pill 按钮组）。概览页直接传 `series` 即可，不要另造图表。

## 布局

### 页面骨架（所有 `/panel/ops/*` 共用）

```
┌─ Breadcrumbs ────────────────────────────────┐
├─ 页标题 + 描述 ──────────────────────────────┤
├─ 二级导航（横向 pill，当前页高亮）───────────┤
├─ 页面内容 ───────────────────────────────────┤
└──────────────────────────────────────────────┘
```

二级导航复用 `TrendChart` 内已有的 pill 组样式：容器 `inline-flex items-center gap-2 rounded-full border border-[color:var(--color-surface-border)] bg-[color:var(--color-surface)] p-1 text-xs shadow-sm`，选中项 `bg-[var(--color-primary)] text-white shadow`，未选中 `text-[var(--color-text-subtle)] hover:bg-[var(--color-surface-muted)]`。

区块间距 `space-y-6`，与现有 management 页一致。

### 概览页栅格

```
指标卡     grid gap-3 md:grid-cols-2 xl:grid-cols-3   （6 个分类指标）
趋势图     单列全宽
欠费名单 + 用量 TopN    grid gap-4 lg:grid-cols-2
```

### 单账号操作台

左右分栏，**危险操作独立成区且视觉降级**——避免误点：

```
lg:grid-cols-3
├─ 左 2 栏：身份、权益（套餐/配额/余额）、订阅列表、审计流水
└─ 右 1 栏：操作面板（指派套餐、发试用、调余额、调配额、清欠费）
```

## Components 规格

| 组件 | 变体 | 关键 props | 说明 |
| --- | --- | --- | --- |
| `OpsNav` | — | `current: string` | 二级导航，新建 |
| `MetricTile` | default / warning / danger | `label, value, helper, tone` | 概览指标，可从 `OverviewCards` 抽出 |
| `AccountSearchTable` | — | `rows, isLoading, onSelect` | 支持按邮箱/UUID/套餐/分类/状态筛 |
| `StateBadge` | normal / arrears / throttled / suspended | `state` | 见下配色 |
| `OpsActionForm` | — | `title, fields, onSubmit, requireReason` | **所有写操作的统一表单**，强制 reason |
| `AuditTrail` | compact / full | `entries, isLoading` | 审计流水 |
| `PlanEditor` | — | `plan, onSave` | 价格字段**只读** |

### `StateBadge` 配色

```
正常 normal      bg var(--color-success-muted)  text var(--color-success-foreground)
欠费 arrears     bg var(--color-warning-muted)  text var(--color-warning-foreground)
限速 throttled   bg var(--color-warning-muted)  text var(--color-warning-foreground)
停机 suspended   bg var(--color-danger-muted)   text var(--color-danger-foreground)
```

样式 `inline-flex items-center rounded-full px-2 py-1 text-xs font-medium`，与 `UserGroupManagement` 现有状态徽章一致。

### `OpsActionForm` — 最关键的组件

所有会写数据的运营动作都走它，保证行为一致且**不可能绕过 reason**：

- `reason` 为**必填** textarea，空值时提交按钮 `disabled`
- 提交前弹二次确认，确认框内**展示变更前后值**
- 提交中：按钮 `disabled` + spinner，文案「提交中…」
- 成功：绿色内联提示 + 自动 `mutate()` 刷新所在区块
- 失败：红色内联提示，保留用户输入**不清空**（重填成本高）

> 前端的 `disabled` 只是体验保障，**后端必须独立校验 `reason` 非空并返回 400**。不能依赖前端。

## States and Interactions

| 元素 | 状态 | 行为 |
| --- | --- | --- |
| 主按钮 | default | `bg-[var(--color-primary)] text-white` |
| 主按钮 | hover | `bg-[var(--color-primary-hover)]` |
| 主按钮 | disabled | `opacity-60 cursor-not-allowed` |
| 主按钮 | loading | spinner + disabled，文案切「提交中…」 |
| 危险按钮（停机/删除） | default | `border-[color:var(--color-danger-border)] text-[color:var(--color-danger-foreground)]`，**不用实心红底**——降低误触视觉引力 |
| 表格行 | hover | `bg-[var(--color-surface-hover)]`；整行可点进详情 |
| 二级导航 pill | active | `bg-[var(--color-primary)] text-white shadow` |
| 搜索框 | typing | 300ms debounce 后触发查询 |
| 表单 | error | 输入框 `border-[color:var(--color-danger)]`，错误文案在下方 |

### 数据刷新

用 SWR，key 与页面数据源一一对应。写操作成功后 `mutate()` 对应 key，**不要整页 reload**。

## 前端 BFF 与接口映射

浏览器只调用 Portal 的 `/api/admin/*` BFF，不直接请求 Accounts 服务，也不把 session token 放入客户端状态。每个 BFF route 都要复用 `getAccountSession` / `evaluateAccountAdminAccess`，并把上游状态码和错误体原样透传。

| 页面能力 | Portal BFF | Accounts 接口 | 最低权限 |
| --- | --- | --- | --- |
| 账号检索 | `GET /api/admin/billing/accounts` | 当前复用 `GET /api/auth/users`；聚合账号账务列表待补 | `admin.users.list.read` |
| 账号详情 | `GET /api/admin/billing/accounts/:uuid` | `GET /api/auth/admin/billing/accounts/:accountUUID` | `admin.users.list.read` |
| 指派套餐 | `POST /api/admin/billing/accounts/:uuid/plan` | `POST /api/auth/admin/billing/accounts/:accountUUID/plan` | `admin.settings.write` |
| 调整配额 | `POST /api/admin/billing/accounts/:uuid/quota` | `POST /api/auth/admin/billing/accounts/:accountUUID/quota` | `admin.settings.write` |
| 发放试用 | `POST /api/admin/billing/accounts/:uuid/grant-trial` | `POST /api/auth/admin/billing/accounts/:accountUUID/grant-trial` | `admin.settings.write` |
| 清欠费 | `POST /api/admin/billing/accounts/:uuid/clear-arrears` | `POST /api/auth/admin/billing/accounts/:accountUUID/clear-arrears` | `admin.settings.write` |
| 调整余额 | `POST /api/admin/billing/accounts/:uuid/balance` | `POST /api/auth/admin/billing/accounts/:accountUUID/balance` | `admin.billing.money.write` |
| 套餐目录读取 | `GET /api/admin/billing/plans` | `GET /api/auth/admin/billing/plans` | `admin.settings.read` |
| 套餐发布/删除 | `PUT/DELETE /api/admin/billing/plans/:planId` | `PUT/DELETE /api/auth/admin/billing/plans/:planId` | `admin.billing.money.write` |
| 审计流水 | `GET /api/admin/audit` | `GET /api/auth/admin/audit` | `admin.settings.read` |
| 用户趋势 | 现有 `GET /api/admin/users/metrics` | `GET /api/auth/admin/users/metrics` | `admin.users.metrics.read` |
| 经营概览 | `GET /api/admin/billing/overview` | 聚合 read model 待补 | `admin.users.metrics.read` |
| 全局账单 | `GET /api/admin/billing/ledger` | 全局 ledger read model 待补 | `admin.billing.ledger.read` |

以下接口不应由前端假造或用全量用户数据临时聚合，需作为后续后端契约补齐：聚合账号账务列表、`GET /admin/billing/overview` MRR/欠费/TopN、全局 ledger（分页/导出/对账）和全站订阅列表。第一批前端只依赖受保护的账号目录、单账号契约和已有用户 metrics；缺少聚合接口的卡片显示加载态或明确空态，不显示不可信的 0。

### 当前实现状态（2026-08-10）

| 能力 | 状态 | 交接说明 |
| --- | --- | --- |
| 运营工作台 `/panel/ops` | 已实现，待 UAT 部署最新 PR | 页面壳、权限门禁、运营快捷入口已具备；趋势和用量 TopN 仍是待接入空态。 |
| 账号处置台 `/panel/ops/accounts` | 已实现，UAT 可读 | 账号检索、账号详情、套餐/试用/配额/余额/清欠费动作已接入；列表 BFF 复用受保护账号目录。单账号 canonical path 仍需从 query 参数升级为 `/:uuid`。 |
| 计费运营总览 `/panel/ops/billing/ledger` | 已实现，数据待闭环 | BFF 与页面已接入；UAT 当前显示待同步，不能据此宣称 MRR、ledger 或对账数据已完成。 |
| 高风险原因与审计 | 后端已保护 | Accounts 写接口强制 `reason` 并记录 before/after；Portal BFF 额外拒绝空原因和超过 500 字的原因。 |
| 资源运维侧栏调整 | 已实现于 PR #172，UAT 未部署 | 本地代码将 `infra` 分组放入侧栏底部；UAT 当前仍显示在顶部。 |
| 套餐 CRUD、审计读取、权限矩阵迁移 | 套餐/审计已完成首版，权限矩阵未完成 | `/panel/ops/billing/plans` 与 `/panel/ops/audit` 已接真实页面/BFF，待 PR #176 部署后 UAT 验收；`/panel/ops/system` 仍需迁移权限矩阵和黑名单。 |

不要把“页面可打开”写成“账务闭环完成”。当前交接验收只能确认页面、权限、空态和写操作保护；真实金额、PAYG 入账和全局对账仍以 P0 验收记录为准。

## 权限与变更边界

权限矩阵要在页面上体现为“能做什么”，而不是把所有按钮渲染出来再让接口返回 403：

| 能力 | admin | operator 默认 | 说明 |
| --- | --- | --- | --- |
| 查看账号/订阅/ledger/审计 | ✅ | ✅ | 只读查询可审计 |
| 指派套餐、调整配额、发试用、清欠费 | ✅ | 按 `admin.settings.write` | 每次必须有 reason |
| 调整余额 | ✅ | ❌ | 仅 `admin.billing.money.write`，默认不授予 operator |
| 发布/删除套餐 | ✅ | ❌ | `/prices` 实时生效，和调余额同级保护 |
| 权限矩阵、黑名单 | ✅ | 按现有权限 | 迁到 `/panel/ops/system` |

余额、套餐和试用表单都必须展示操作人、目标账号、变更前后值和 reason。前端隐藏无权能力，后端仍必须独立校验；不能把 `admin.settings.write` 当成资金权限。

## Responsive

| 断点 | 变化 |
| --- | --- |
| Desktop (≥1280px) | 指标 3 列；单账号操作台 2:1 分栏 |
| Tablet (768–1279px) | 指标 2 列；操作台改上下堆叠，操作面板置顶（运营常用） |
| Mobile (<768px) | 指标 1 列；表格**横向滚动**（`overflow-x-auto`，沿用现有表格做法），不做卡片化改造——运营台以桌面为主，移动端保证可读可查即可 |

二级导航在窄屏横向滚动，不换行、不折叠成下拉。

## Edge Cases

| 场景 | 处理 |
| --- | --- |
| **空状态 - 无搜索结果** | 「未找到匹配的账号」+ 清除筛选按钮 |
| **空状态 - 无审计记录** | 「暂无变更记录」，虚线边框容器（沿用 `SubscriptionPanel` 空态样式） |
| **空状态 - 无欠费** | 「当前无欠费账号」+ ✅ 图标，这是好消息，不要显示成错误 |
| **加载中** | 表格用骨架行（`animate-pulse`，`UserGroupManagement` 已有实现）；指标卡用骨架块。**不要用整页 spinner** |
| **长文本** | 邮箱/UUID 单行截断 `truncate` + `title` 属性显示全文；原因字段最多 2 行后 `line-clamp-2` |
| **超长列表** | 账号/ledger/审计一律**分页**，默认 50 条/页。审计表增长极快（UAT 曾因审计日志 IO 激增触发事故），绝不可全量加载 |
| **数值为 0** | 显示 `0`，不显示 `—`。`—` 只用于「数据缺失」，两者语义不同 |
| **配额/流量单位** | 一律走 `formatBytes()`（`@lib/format`），不要显示裸字节数 |
| **金额** | 保留 2 位小数并带币种；余额为负时用 `--color-danger` |
| **权限不足** | 页面级由 guard 拦截跳 `/panel`；组件级（如 operator 看不到调余额）**隐藏而非禁用**，避免暴露不可用能力 |
| **并发修改** | 写接口返回 409 时提示「数据已被他人修改，请刷新后重试」，并自动 `mutate()` |

## PAYG 充值发布闸门（P0，优先于控制台 UI）

充值是实际资金路径，不能因为控制台页面先上线就把它降为普通缺陷。`checkout.session.completed` 的一次性支付分支只有同时满足以下条件，才允许标记完成：

1. 仅处理 `mode=payment`、`metadata.kind=paygo` 且 Stripe 报告已支付的 session；金额取 `amount_total`，按 Stripe 最小货币单位换算，不从前端或套餐目录重新计算。
2. 以 `payment_intent` 作为稳定幂等键，重复 webhook 只能返回成功，不能新增余额或 ledger 分录；不同 payment intent 必须分别入账。
3. 余额变更与 `entry_type=topup` ledger 分录必须在同一个数据库事务中完成，并在并发充值下保持 `current_balance = ledger 累计`。仅“先插 ledger、再单独更新余额”而没有事务/行锁，不算资金安全实现。
4. 入账成功发布 `balance_topped_up`；未支付、金额为 0、缺少用户或非 PAYG session 不得入账，并留下可诊断日志。

UAT 验收用例：

| 用例 | 预期 |
| --- | --- |
| 一次支付首次 webhook | 余额增加实付金额，新增一条 `topup` ledger，余额与 ledger 对账一致 |
| 同一事件重放 3 次 | 余额和 ledger 行数都不再增加 |
| 同账号第二笔支付 | 两笔金额累加，两个 payment intent、两条 ledger |
| 未支付/失败 session | 余额、ledger 均不变 |
| 两个并发 payment intent | 不丢钱、不覆盖余额，最终余额等于两笔 ledger 累计 |

这组用例必须在 UAT 通过后，才继续把 PAYG 充值入口标成可运营；控制台的余额卡片也必须能显示 topup 分录。

## Motion

| 元素 | 触发 | 动画 | 时长 | 缓动 |
| --- | --- | --- | --- | --- |
| 按钮 / 徽章 | hover | 背景色渐变 | 160ms | `ease-in-out`（全局 `globals.css` 已定义） |
| 二级导航 pill | 切换 | 背景色渐变 | 160ms | `ease-in-out` |
| 确认弹窗 | 打开 | fade + scale 0.98→1 | 150ms | `ease-out` |
| 内联提示 | 出现 | fade in | 120ms | `ease-out` |
| 骨架屏 | 加载中 | `animate-pulse` | Tailwind 默认 | — |

遵循 `prefers-reduced-motion`：该媒体查询下仅保留 fade，去掉位移与缩放。

## Accessibility

- **焦点顺序**：面包屑 → 页标题 → 二级导航 → 筛选 → 表格 → 操作面板。表格内按行、行内按列。
- **二级导航**：`role="tablist"`，各项 `role="tab"` + `aria-selected`；或用链接语义 + `aria-current="page"`（推荐后者，因为它们是真实路由）。
- **表格**：`<caption>` 说明用途（可 `sr-only`）；表头 `<th scope="col">`；排序列 `aria-sort`。
- **状态徽章**：颜色不是唯一信息载体——徽章内必须有文字（「欠费」「停机」），色盲用户可读。
- **危险操作**：确认弹窗 `role="alertdialog"` + `aria-describedby` 指向变更摘要；打开时焦点移入弹窗，关闭后归还触发元素；`Esc` 关闭。
- **表单**：每个输入有关联 `<label>`；错误用 `aria-invalid` + `aria-describedby` 指向错误文案。
- **异步结果**：成功/失败提示置于 `aria-live="polite"` 区域，让屏幕阅读器播报。
- **加载态**：骨架容器 `aria-busy="true"`（`UserGroupManagement` 已有此用法）。
- **键盘**：表格行可点进详情时必须可聚焦（`tabIndex={0}` + `Enter` 触发），不要只绑 `onClick` 到 `<tr>`。

## 接手后的实施顺序与 Definition of Done

### P0-A：先封住真实资金风险

- [ ] 合入并部署 PAYG top-up 修复；确认 webhook 分支、`payment_intent` 幂等、ledger 和余额事务语义。（代码与 Accounts 单测已有，UAT 未验收）
- [ ] 在 UAT 完成上面的 5 组充值用例，并保留事件 ID、payment intent、ledger ID 和对账结果。
- [x] 充值未通过前，控制台不得提供“充值成功”的运营文案；余额展示可读，但必须标出数据状态。

### P0-B：前端基础设施与 `/panel/ops` 壳

- [x] 注册 `ops` route，使用 `startsWith`、admin/operator guard、`/panel` forbidden fallback；补 sidebar admin 分组入口。
- [x] 建立运营页面共用的面包屑、二级导航、`jsonFetcher` 和 SWR 刷新模式。（当前仍集中在 ops 页面组件，后续可再抽公共组件）
- [ ] 先补 BFF route，再接页面；为每个 BFF 增加未登录、无权限、上游 400/409/5xx 的测试。（主要 BFF 已有，聚合 read model 和路由级测试未齐）
- [ ] `/panel/ops` 首屏先复用 `OverviewCards`、`TrendChart`，展示用户趋势和分类；MRR/欠费/TopN 等没有可信聚合接口时不临时猜算。（当前待同步空态正确，但趋势/TopN 尚未接入）

### P0-C：账号工作流（第一批可运营版本）

- [ ] `/panel/ops/accounts`：邮箱/UUID 搜索、套餐/分类/状态筛选、分页、空态、行键盘可访问。（当前有搜索/筛选/空态，仍只显示前 10 条且行键盘访问未补）
- [x] `/panel/ops/accounts/:uuid`：身份、套餐、订阅、余额、配额、停机状态、最近 ledger、账号审计。
- [ ] 接入指派套餐、发试用、调配额、调余额、清欠费；所有写操作统一经过 `OpsActionForm`，reason、确认、loading、成功/失败、409 刷新齐全。（当前已有动作弹窗与 reason，二次确认/409 专项处理待补）
- [ ] operator 默认看不到调余额；无 `admin.billing.money.write` 的用户也看不到套餐发布/删除。（后端权限已保护，UI 组件级隐藏尚未完全补齐）
- [ ] 以测试账号完整跑通“查账号 → 发试用/指派套餐 → 调配额 → 清欠费 → 查看审计”的闭环。

### P1-A：拆分 `/panel/management`

- [ ] 新建 `/panel/ops/system`，迁移 `PermissionMatrixEditor`、`EmailBlacklist`，保留权限和黑名单现有行为。
- [ ] `/panel/management` 只保留 `HomepageVideoSettingsPanel` 和迁移提示；旧 URL、权限和已有用户操作不被一次删除打断。
- [ ] 分类 UI 使用 `segments`：展示 `registered/subscribed/ops/beta/legacy`，不把业务分类写回 `groups`。

### P1-B：套餐、价格与账单

- [x] `/panel/ops/billing/plans` 接入套餐 CRUD；`active=false` 下架，Stripe price ID/金额字段只读并展示改价边界；写入要求原因和二次确认。
- [ ] “发布到 `/prices`”必须显示影响范围、当前 active 状态、操作者和审计结果；发布失败不能给成功提示。
- [ ] `/panel/ops/billing/ledger` 先做分页明细和单账号筛选，再做 CSV 导出与对账；默认 50 条，禁止全量加载。
- [x] `/panel/ops/audit` 接 action prefix、actor、target、时间筛选；详情显示 before/after/reason 快照。分页/导出和 request context 仍需后续补齐。

### P1-C：经营概览补齐

- [ ] 后端提供可信的 overview read model：MRR、活跃订阅、欠费名单、用量 TopN、用户趋势和更新时间。
- [ ] 明确 MRR 口径（active paid subscriptions、币种、退款/欠费处理、`ops`/`beta` 是否排除），在 UI 的 helper 文案和接口契约中固定。
- [ ] 对账通过后，概览卡片才从骨架/空态切换为正式数据；任何数据缺失显示“暂无数据/不可用原因”，不显示伪造的 0。

### 最终 UAT 验收

- [ ] admin/operator 访问边界正确，组继承仍可用，普通用户不能看到或进入 `/panel/ops/*`。
- [ ] 所有资金、价格、权益和系统变更都能按 target、actor、action 找到审计记录。
- [ ] 充值重放、并发修改、重复提交、上游 409、权限不足、空列表和移动端横向滚动均通过验收。
- [ ] `npm test` / `npm run lint` / `npm run build` 通过；Accounts 侧对应 Go 测试和数据库验收通过；最后在 UAT 手工走一遍三条运营主线：开通 custom、补偿余额、解释停机。
