# 进度与交接

最后更新：2026-08-13

这份文档是给接手的 Code Agent 看的：当前做到哪、下一步做什么、哪些坑已经踩过不要重踩、什么在阻塞。**其余文档描述"应该是什么"，这份描述"现在是什么"。**

## 一句话状态

UAT 已重建并在线；运营台 `/panel/ops` 已上线可用；**用户侧订阅链路的代码今天补齐（4 个 PR 待合并）**；但商业化仍**未激活**——UAT 上 Stripe 密钥仍为空，套餐目录仍只有 `TRIAL-7D` / `FREE` 两条。

**离"能收到第一笔钱"只差配置，不差代码。** 全部步骤见 [12 号文档的 UAT runbook](./12-user-center-subscription-and-signup.md#五把-xconnect-订阅跑通uat-runbook)。

## UAT 实测（2026-08-13）

环境已重建，与本文档上一版（08-09 记录"环境已销毁"）不同：

```
console-uat.onwalk.net              → HTTP 200
accounts-uat.onwalk.net/api/...     → 200（根路径 404 是正常的，没有 / 路由）
DNS console-uat.onwalk.net          → 66.42.45.216
```

### 目录仍是两条

```bash
curl -s https://accounts-uat.onwalk.net/api/billing/plans
# TRIAL-7D  includedQuotaBytes=10737418240  stripePriceId 无
# FREE      includedQuotaBytes=0            stripePriceId 无
```

`PRO-MONTHLY` / `PRO-YEARLY` / `PAYG-TOPUP-*` **一条都没有**。与 08-09 的记录一致，四天无变化。

> `TRIAL-7D` 是 **10GiB**，但已拍板改为 **5GiB**（见 [12 §三](./12-user-center-subscription-and-signup.md)）。种子逻辑是 insert-only，改代码不会动到已有的行，**必须经运营台改**。

### Stripe 仍未配置（今天新测出的硬阻断）

```bash
curl -X POST https://accounts-uat.onwalk.net/api/billing/stripe/webhook \
  -H 'Content-Type: application/json' -d '{}'
# HTTP 503 {"error":"stripe_not_configured","message":"stripe is not configured"}
```

`stripeWebhook` 的第一行就是 `h.stripe.enabled()` 判断，503 = `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` 为空。

**这是整条链路上唯一的硬阻断。** 在它解决之前，前端无论怎么改，点击购买都只会拿到 503。

> 这个探针是安全的：两个分支都在任何写库动作之前返回，一个未签名的空 body 不会产生任何副作用。以后验证 Stripe 是否生效，用它即可——从 **503 变成 401 `invalid_signature`** 就说明密钥已经装上了。

### 账号与审计

```
GET /api/admin/users/metrics  → totalUsers 2, activeUsers 2, subscribedUsers 0, newUsersLast24h 2
GET /api/admin/audit?limit=5  → {"entries":[]}
```

- **PROD→UAT 用户同步没有重跑**：只有 2 个账号且都是近 24 小时新建的。上一版记录的 16 个存量账号不在这个环境里，[06 存量用户迁移](./07-existing-user-migration.md) 的验证做不了
- **审计表是空的**：`audit_logs` 的读写链路是通的（接口返回 200 而不是报错），只是还没有任何一次经过管理端接口的变更。这也说明现有的两条目录记录是**种子写入的，不是运营台写的**

### 已登录账号的真实状态

```
套餐 default   配额 0 B / 0 B   余额 0.00   订阅记录：暂无
```

`account_billing_profiles` 与 `subscriptions` 对这个账号都是空的——和 08-05、08-09 记录的现象一样，**没有任何改善**。

## PR 状态

### 今天新开、**待合并**

| PR | 内容 |
| --- | --- |
| [accounts#75](https://github.com/ai-workspace-services/accounts/pull/75) | 目录挂牌价字段 `price_amount`/`price_currency`/`price_unit`，并入审计快照；同步脚本新增 `--write-catalog` 直接写回 `billing_plans` |
| [accounts#77](https://github.com/ai-workspace-services/accounts/pull/77) | OAuth 已验证邮箱直接发试用；`TRIAL-7D` 种子 10GiB→5GiB |
| [portal#218](https://github.com/ai-workspace-services/portal/pull/218) | 用户中心改读目录（删掉 6 张硬编码 USD 卡片）；`/prices` Free 卡片文案改为真实条款 |
| [content-service#26](https://github.com/ai-workspace-services/content-service/pull/26) | 12 号文档（用户侧规划 + UAT runbook）、05 文档更新、本文档更新 |

**这 4 个 PR 需要你来合并**（合并动作被权限分类器挡住，我做不了）。

### 此前已合并

| PR | 内容 |
| --- | --- |
| [accounts#55](https://github.com/ai-workspace-services/accounts/pull/55) | Stripe 目录 IaC 化（声明式 YAML + 幂等同步脚本） |
| [accounts#58](https://github.com/ai-workspace-services/accounts/pull/58) | 审计写入 + P0 运营接口（`grant-trial` / `assign-plan` / `adjust-quota` / `adjust-balance`） |
| [accounts#59](https://github.com/ai-workspace-services/accounts/pull/59) | **充值入余额**——[09](./09-trial-grants-and-topup-gap.md) 记的代码缺口已修复 |
| [accounts#62](https://github.com/ai-workspace-services/accounts/pull/62) | `admin.billing.money.write` 权限分档（动钱与发权益分离） |
| [portal#166](https://github.com/ai-workspace-services/portal/pull/166) | `/prices` 四档卡片 + 实时读目录判断可购买性 |
| [portal#167](https://github.com/ai-workspace-services/portal/pull/167) | 用户组角色继承 |
| portal `/panel/ops` | 运营台三块（账号处置 / 套餐目录 / 审计台）已在 main 上并在 UAT 可用 |

## 阻塞中

| 阻塞项 | 影响 | 谁能解 |
| --- | --- | --- |
| **UAT 上 Stripe 密钥为空** | 唯一硬阻断。所有下单路径返回 503 | 需要有 Stripe 后台 + Vault 权限的人。今天实测确认仍未解决 |
| **套餐目录缺三档付费** | 即使 Stripe 装上，也没有东西可卖 | 跑 `stripe-sync-catalog.sh --write-catalog`（accounts#75 合并后可用） |
| **PROD→UAT 用户未同步** | 存量用户迁移方案无法验证 | 重跑 `data-migration.yaml`，`migration_scope=accounts` |

### 已解除的阻塞

- ~~UAT 环境已销毁~~ → 已重建并在线
- ~~充值不入余额（代码缺口）~~ → accounts#59 已修复，用确定性 ledger 主键抗 webhook 重投
- ~~运营管理接口缺失~~ → accounts#58 + `/panel/ops` 已上线

⚠️ **Stripe 密钥为空是"优雅降级"而非故障**：accounts 的 stripe 客户端在密钥为空时软性 disabled（`api/stripe.go` 的 `enabled()`），不阻塞部署。密钥到位后自动生效，无需改代码或重新合并任何 PR。

## 已知缺口（不阻塞收钱，但要记着）

| 缺口 | 现象 | 说明 |
| --- | --- | --- |
| `GET /admin/billing/overview` / `/admin/billing/ledger` **在 accounts 上根本不存在** | 运营台 MRR / 欠费 / 待处理 / 经营趋势 / 用量 TopN 五个卡片显示「待同步」；portal 代理拿到非 JSON 返回 **502 `invalid_response`** | portal 侧代理路由已写好并在调用，只是上游没有这两个路由。**已决定先保持不变** |
| 试用状态对用户不可见 | 用户看不到自己在试用期、剩几天、到期会怎样 | [12 §二 G5](./12-user-center-subscription-and-signup.md) |
| 无升降档 / 月付↔年付切换 | 只有 `/subscriptions/cancel`，改档只能退订重买 | [12 §二 G7](./12-user-center-subscription-and-signup.md) |
| 欠费页无自助恢复入口 | 只显示状态，不给「立即充值恢复」动作 | [12 §二 G8](./12-user-center-subscription-and-signup.md) |
| MFA 门槛无引导 | 付费前强制绑 MFA（`api/stripe.go` checkout 与 portal 都挡），提示只有一句话没有引导链路 | [12 §二 G9](./12-user-center-subscription-and-signup.md) |
| `users` 表无 `segments` 列 | 账号分类标签尚未落库 | [06](./06-management-console-integration.md) |

## 下一步

**按 [12 号文档的 UAT runbook](./12-user-center-subscription-and-signup.md#五把-xconnect-订阅跑通uat-runbook) 逐步执行**，那里有完整的顺序、判据和依赖关系。摘要：

1. 拿 Stripe test key → 2. 跑同步脚本建 Price/Webhook → 3. 密钥进 Vault 并重启 → 4. 合并部署今天这 4 个 PR → 5. `--write-catalog` 写回目录 → 6. 运营台把 `TRIAL-7D` 改 5GiB → 7. 测试卡实测

第 2 步不必等第 3 步：同步脚本只用 `STRIPE_SECRET_KEY` 直连 Stripe API，不经过 accounts。

**第 7 步有一条判据必须实测、不能只看代码**：在 Stripe 后台手动重投同一个 `checkout.session.completed` 事件，**余额不能变**。这是整条链路上唯一直接防重复扣款的断言。

跑通 XConnect 之后，其余产品线按同一模式复制：目录加条目 → 同步脚本加 price → 前端加一段展示文案。**不要再引入第二套硬编码商品表**（见下方"踩过的坑"最后一条）。

## 踩过的坑（不要重踩）

| 坑 | 症状 | 根因与修复 |
| --- | --- | --- |
| **`account_user` 无任何表权限** | 部署全绿但用量恒为 0，billing ingest 报 `SQLSTATE 42501` | schema 以 `psql -U postgres` 灌入，所有表属主是 postgres。修复：[playbooks#255](https://github.com/ai-workspace-infra/playbooks/pull/255) 转移属主 + accounts 改用 `account_user` 连库 |
| **序列属主不能单独改** | 属主转移脚本报 `cannot change owner of sequence` | serial/identity 序列跟随所属表。修复：[playbooks#256](https://github.com/ai-workspace-infra/playbooks/pull/256) 先改表带走序列，再收独立序列 |
| **JSON 字段名大小写漂移** | Portal 整页白屏 `Cannot read properties of undefined (toLocaleString)` | 4 个响应体 struct 无 json tag，Go 默认输出大驼峰，前端读小驼峰。**ledger 为空时零症状，有数据才炸**。修复：[accounts#49](https://github.com/ai-workspace-services/accounts/pull/49) 补 tag + 契约测试 |
| **`v*` tag 触发生产部署** | 打快照 tag 意外把 toolkit 拉进 prod apply | 修复：[toolkit#260](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/260) 快照 tag 闸门 + [#261](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/261) `v*` push 改 plan-only + [#262](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/262) master pipeline 同类修复 |
| **内存 store 比 postgres 宽松** | 重复 ledger 主键在测试里被静默覆盖，在生产会被 PK 拒绝——**双重入账的回归测不出来** | 修复：加 `ErrDuplicateLedgerEntry` 哨兵，让 memory store 与真实约束行为一致（accounts#59）。**这类"测试环境比生产宽松"是本项目反复出现的失败模式** |
| **前端另起一套商品表** | 用户中心推销 6 张 USD 卡片，含 2 个定价页根本不卖的产品，planId 全都不在 `billing_plans` 里，**登录用户一张也买不了** | 根因是 `/prices` 已按"目录是唯一事实源"改造过，用户中心没有，读的是 `src/modules/products/registry.ts`。修复：portal#218 让两个页面读同一份目录 |

**共同教训**：这条链路上反复出现的失败模式是**静默失败**——部署全绿、日志无异常，但数据没落地、字段读错、或者页面在卖不存在的商品。任何新增的关键步骤都应该有"真的会失败"的显式校验，而不是只看 exit code。

## 重要的设计约束（改代码前必读）

1. **`groups` 字段不能承载业务分类**。它的真实语义是节点路由可达性（`EligibleNodeGroups`），贯穿 `internal/agentserver/registry.go`。账号分类要用独立的 `segments` 字段——把两个含义塞进一个字段正是 accounts#49 那类 bug 的根因
2. **billing-service 永不直连 Stripe**。Stripe 只与 accounts 对话，这是既有架构边界
3. **PostgreSQL 是账务事实源**，超额不上报 Stripe metered usage（[01](./01-plan-catalog.md#stripe-对象映射)）
4. **Stripe 价格不可变**。改价必须换新 `lookup_key`，不能原地改（[05](./05-stripe-catalog-automation.md)）
5. **手动调余额必须写 ledger**，不能直接 UPDATE `current_balance`（[03](./03-operations-console.md#余额调整必须走-ledger)）
6. **充值入账必须按 `payment_intent` 幂等**。Stripe webhook 会重试，重复入账等于凭空发钱（[09](./09-trial-grants-and-topup-gap.md#幂等是硬要求)）
7. **`billing_plans` 是唯一事实源，前端不得另建商品表**。展示金额取目录的 `price_amount`/`price_currency`/`price_unit`；可售判定取 `active && stripe_price_id`（正是 `validCheckoutPrice` 校验的那一对）。文案可以留在前端，**价格和商品清单不行**
8. **改目录数据走管理端接口，不要直连数据库**。直接 `INSERT`/`UPDATE` 会绕过 `audit_logs`，运营台的变更历史里会出现一段无法解释的空白——而那正是 [10](./10-ops-console-design.md) 要解决的问题
9. **`requireAdminPermission` 对 admin 和 root 无条件放行**。权限字符串实际只约束 `operator` 角色，`admin.billing.money.write` 这条分档也不例外——不要把它误读成比实际更强的保证

## 环境速查

| 项 | 值 |
| --- | --- |
| UAT 域名 | `console-uat.onwalk.net` / `accounts-uat.onwalk.net` / `agent-proxy.onwalk.net` |
| UAT IP（2026-08-13） | `66.42.45.216`。**重建后会变，从 CMDB 或部署日志取，不要沿用本文档里的值** |
| UAT 数据库 | `docker exec -i web-saas-postgresql psql -U postgres -d account` |
| PROD 域名 | `console.svc.plus` / `accounts.svc.plus` |
| Vault Stripe 密钥路径 | `kv/billing-service`，键 `SANDBOX_STRIPE_*` / `PROD_STRIPE_*` |
| 部署工作流 | `platform-ops.yaml` |
| PROD→UAT 数据同步 | `data-migration.yaml`，`migration_scope=accounts` |
| Stripe 是否已配置 | `curl -X POST https://accounts-uat.onwalk.net/api/billing/stripe/webhook -d '{}'`：503 = 未配置，401 = 已配置 |

> 本套文档现在住在 **`ai-workspace-services/content-service`** 仓库的 `roadmap/` 下。早期提交信息里引用的 `ai-workspace-services/docs` PR 链接（docs#16/#17/#19 等）指向的是仓库改名前的历史，可能已失效。

### 重建 UAT

```bash
# 1. 先出一个当天的快照 tag（deploy_tag 必须是不可变 tag，main/latest 会被拒绝）
gh workflow run daily-main-snapshot.yaml --repo ai-workspace-infra/platform-ops-toolkit \
  -f snapshot_tag=uat-daily-build-YYYY.MM.DD-r1 \
  -f snapshot_source_ref=main -f deploy_env=uat

# 2. 用该 tag 重建（run_infrastructure=false + run_full_stack=true 是既有惯例，
#    full_stack 会自行打开基础设施与应用部署两个开关）
gh workflow run platform-ops.yaml --repo ai-workspace-infra/platform-ops-toolkit \
  -f runner_type=ubuntu-latest \
  -f deploy_tag=uat-daily-build-YYYY.MM.DD-r1 \
  -f offline_mode=off \
  -f source_host=install.svc.plus -f source_domain_base=svc.plus \
  -f target_domain_base=onwalk.net \
  -f run_full_stack=true -f run_infrastructure=false -f run_application_deploy=false \
  -f "target_domains=web-saas + agent-proxy" \
  -f cloud_provider=vultr-vps -f instance_plan=2C4G -f action=deploy
```

重建后 `run_full_stack=true` 会自动跑 `Reconcile UAT DNS Records`（把域名指到新 IP）和 post-deploy 的 `data-migration`（从 PROD 同步账号）。两者都跑完再验证，否则会看到域名指向旧 IP、库里没有用户这类假象。

> 2026-08-13 实测：环境在线但只有 2 个账号，说明 `data-migration` 这一步**没有生效或没有跑**。验证存量用户迁移前需要先补跑。

**销毁 UAT**（同一个工作流，`action=destroy`）：

```bash
gh workflow run platform-ops.yaml --repo ai-workspace-infra/platform-ops-toolkit \
  -f runner_type=ubuntu-latest -f action=destroy \
  -f deploy_tag=<任一有效 tag> \
  -f source_host=install.svc.plus -f source_domain_base=svc.plus \
  -f target_domain_base=onwalk.net \
  -f "target_domains=web-saas + agent-proxy" \
  -f cloud_provider=vultr-vps -f instance_plan=2C4G \
  -f run_full_stack=false -f offline_mode=off
```

`deploy_tag` 在 destroy 时不影响结果，但工作流强制要求非空。
