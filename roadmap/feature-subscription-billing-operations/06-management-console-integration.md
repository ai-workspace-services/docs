# 管理台整合：`/panel/management`

把 [03-operations-console.md](./03-operations-console.md) 里规划的运营能力，落到真实存在的
[`/panel/management`](https://console-uat.onwalk.net/panel/management) 页面上，而不是另起一套界面。

## 现状（实测代码，2026-08-08）

页面源码：`portal/src/modules/extensions/builtin/user-center/routes/management.tsx`（525 行）。

结构是**纵向堆叠的 Card 区块**，不是 tab 切换：

```
OverviewCards          — 用户量/活跃度概览卡片
TrendChart              — 按日/周趋势图
PermissionMatrixEditor  — 角色 × 权限矩阵
HomepageVideoSettingsPanel — 首页视频配置
UserGroupManagement      — 用户表 + 批量邀请/导入 + 黑名单
EmailBlacklist（弹层）
```

`UserGroupManagement` 是承接本次改动的核心组件：

| 现有列 | 内容 |
| --- | --- |
| 邮箱 / 用户名 | 只读 |
| 角色 | `<select>`，`admin`/`operator`/`user`，改动即调用 `onRoleChange` |
| 用户组 | `user.groups?.join("、")`，只读展示，编辑走"创建自定义用户"表单 |
| 状态 | 活跃/已暂停徽章 |
| 操作 | 暂停 / 恢复 / 删除 / 重置 UUID |

关键结论：**`ManagedUser` 类型里没有任何计费字段**——不知道套餐、余额、欠费状态、订阅到期时间。管理台今天对"这个用户在用哪个套餐、欠不欠费"完全没有可见性，只能去数据库里查。这正是要补的东西。

## 整合方案

不新建页面、不新建 tab——沿用"新增一个 Card 区块 + 给现有用户表加列"的既有模式，改动面小、用户不需要学新的导航。

### 1. 新增区块：`BillingOverviewCards`

风格模仿现有 `OverviewCards`，插在它后面。展示：

- 各套餐账号数（free / payg / pro-monthly / pro-yearly / custom）
- 当前欠费账号数、距停机 < 24h 的账号数
- 本月 MRR（简单加总，不用做精确财务口径）

数据源：新增 `GET /api/auth/admin/billing/overview`（accounts 新增，聚合查询 `account_billing_profiles` + `account_quota_states`）。

### 2. 扩展 `UserGroupManagement` 表格

`ManagedUser` 类型新增（对应 [01-plan-catalog.md](./01-plan-catalog.md) 与 [02-metering-and-entitlements.md](./02-metering-and-entitlements.md) 的模型）：

```ts
export type ManagedUser = {
  // ...现有字段不变
  billing?: {
    planId?: string;          // FREE / PAYG / PRO-MONTHLY / PRO-YEARLY / CUSTOM-*
    packageName?: string;
    currentBalance?: number;
    remainingIncludedQuota?: number;
    arrears?: boolean;
    suspendState?: string;    // active / suspended
    segments?: string[];      // 见下文「账号分类标签」，与 billing plan 是两回事
  };
};
```

表格新增两列，插在「用户组」和「状态」之间：

| 新列 | 展示 | 交互 |
| --- | --- | --- |
| 套餐 | `billing.planId` 徽章，`custom` 用紫色区分 | 点击展开「指派套餐」下拉——和「角色」列同一套 `<select>` 交互模式，选项来自 `GET /api/billing/plans` |
| 余额/欠费 | 余额数字；欠费时红色徽章 + 距停机剩余时间 | 点击打开「调整余额」小弹窗（金额 + 必填原因，见下） |

操作列新增：

```
[暂停] [恢复] [删除] [重置UUID] [指派套餐] [调整余额] [账单明细]
```

「账单明细」跳转到该用户的 ledger 只读视图（复用 Portal `SubscriptionPanel` 的详情 tab 渲染逻辑，管理台传 `accountUuid` 参数改为查任意账号而非当前登录用户）。

### 3. 后端新增接口

延续现有 admin 接口的鉴权模式（`requireAdminPermission`，复用 settings 权限）：

```
GET  /api/auth/admin/billing/overview                    — 上面的概览卡片数据
POST /api/auth/admin/billing/accounts/:uuid/plan          — 指派套餐（新建，03 里点名缺失的那个）
POST /api/auth/admin/billing/accounts/:uuid/balance       — 调整余额（新建，必须写 ledger，见 03 的审计要求）
GET  /api/auth/admin/billing/accounts/:uuid                — 单账号完整权益详情（表格行展开用）
```

`调整余额` 的权限位单独判断（`permissionAdminBilling` 而非复用 `permissionAdminSettings`）——03 已经指出这一点：能改价目表和能给账号打钱不是同一个信任级别。

### 4. 视觉一致性

新增元素严格复用现有 class 约定（本页面用的是 `text-gray-*`/`purple-*` 这套配色，与 Portal 账户面板的 `var(--color-*)` CSS 变量体系是两套皮肤——**不要**把 Portal 那套变量抄进管理台，管理台目前是硬编码 Tailwind 颜色，跟着现状走，不引入第三套风格）。

## 账号分类标签（新概念，独立于套餐）

**"注册用户 / 订阅用户 / 运营用户 / 内测用户"这类人工分类，不应该复用现有的 `groups` 字段。**

`groups` 目前的真实语义是**节点路由可达性**（`EligibleNodeGroups`，决定这个账号能连哪些 Xray 节点/Agent），在 `internal/agentserver/registry.go`、`config/config.go` 里贯穿使用。把业务分类标签也塞进同一个数组，会重演今天已经踩过一次的坑（同一字段在不同代码路径里被赋予不同含义，最终在某处解释错——这正是 [accounts#49](https://github.com/ai-workspace-services/accounts/pull/49) 修的那类 bug 的根因，只是换了个字段）。

### 设计

新增独立字段，`users` 表加一列：

```sql
ALTER TABLE users ADD COLUMN segments text[] NOT NULL DEFAULT '{}';
```

预置枚举（人工可多选，不互斥）：

| 值 | 含义 | 典型来源 |
| --- | --- | --- |
| `registered` | 已注册，无其他特征 | 注册流程默认打上，几乎所有账号都有 |
| `subscribed` | 当前有生效的付费订阅 | **建议由系统自动派生**（`account_billing_profiles.package_name != 'free'` 且 `subscribe_state=active`），不需要人工维护，人工只能看不能改 |
| `ops` | 内部运营/客服账号 | 人工打标，用于从计费报表、用量排行里排除，避免内部测试流量污染经营指标 |
| `beta` | 内测用户 | 人工打标，用于灰度新功能、单独通知渠道 |
| `legacy` | 存量用户迁移标记 | 见下文迁移方案，批量回填用 |

`subscribed` 强烈建议**不做成人工可编辑**——人工能改的标签和系统真实状态一旦分叉（比如运营手滑把一个真实付费用户标成非 subscribed），后续所有依赖这个标签的报表和过滤都会悄悄出错，而且没有任何报错，纯粹是数据对不上。人工只维护 `ops`/`beta`/`legacy` 这类系统无法自动判断的标签，`subscribed` 走计算属性。

### 管理台交互

用户表新增「分类」列，展示为多个小标签（chip），点击进入编辑态时是复选框组，样式参考现有「分组列表」输入框（`parseGroupList` 那套 textarea + 逗号/换行分隔），但存成独立字段，不合并进 `groups`。

### 用途

- 运营报表（[03-operations-console.md](./03-operations-console.md) P2 的收入/活跃看板）按 `ops`/`beta` 过滤，避免内部账号污染经营数据
- 批量操作（比如群发迁移通知）按标签筛选目标人群
- `custom` 套餐账号天然就是 `subscribed`，不需要额外标记

## 与存量用户迁移的关系

见 [07-existing-user-migration.md](./07-existing-user-migration.md)——`legacy` 标签就是给这次迁移用的，回填时统一打上，方便后续追踪"哪些账号是历史遗留、按什么规则处理的"，而不是回填完就再也分不清谁是原生走完整注册流程的、谁是迁移进来的。
