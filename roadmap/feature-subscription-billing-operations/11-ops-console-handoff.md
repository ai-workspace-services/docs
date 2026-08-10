# Handoff Spec: 运营控制台 `/panel/ops`

开发交接规格。设计意图与分期见 [10-ops-console-design.md](./10-ops-console-design.md)，这份是可直接照着写代码的实现细节。

技术栈：Next.js App Router + React + Tailwind + SWR + Zustand（`useUserStore`）。

## Overview

面向 **admin / operator** 的独立运营控制台。当前 `/panel/management` 把权限矩阵和首页视频配置放在同一页，已经成为杂物间——本控制台把**运营职责**独立出来，并把那两块按归属拆走。

进入路径：侧边栏 `admin` 分组（[portal#167](https://github.com/ai-workspace-services/portal/pull/167) 已取消 `hidden`，admin/operator 及由组继承者可见）。

## 路由

| 路由 | 页面 | 迁移来源 |
| --- | --- | --- |
| `/panel/ops` | 概览：MRR、活跃订阅、欠费名单、用量 TopN、用户趋势 | 新建（复用 `OverviewCards` / `TrendChart`） |
| `/panel/ops/accounts` | 账号检索 → 列表 | 新建 |
| `/panel/ops/accounts/:uuid` | 单账号操作台：指派套餐、发试用、调余额、调配额、清欠费 | 新建 |
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
operatorUsers: number   // groups 含 segment:operator
betaUsers: number       // groups 含 segment:beta
```

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

## 实现顺序

前端依赖后端接口，而**账号级接口目前一个都不存在**（现有仅套餐 CRUD + 清欠费）。因此：

1. **后端 P0 接口 + 审计写入**（[10](./10-ops-console-design.md) 阶段 A/B）——没有它前端做完也没数据
2. `/panel/ops` 骨架 + 概览页（复用 `OverviewCards`/`TrendChart`，**可先行**，metrics 接口已存在）
3. 账号检索 + 单账号操作台
4. `/panel/ops/billing/plans`（后端已就绪）
5. ledger / 审计页
6. 从 `/panel/management` 迁出权限矩阵与黑名单

第 2 步不阻塞于第 1 步，可并行启动。
