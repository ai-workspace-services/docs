# 进度与交接

最后更新：2026-08-08

这份文档是给接手的 Code Agent 看的：当前做到哪、下一步做什么、哪些坑已经踩过不要重踩、什么在阻塞。**其余文档（01~07）描述"应该是什么"，这份描述"现在是什么"。**

## 一句话状态

计量链路已通并有真实数据；商业化链路的代码接线已完成但**未激活**（Stripe 密钥仍为空）；生产 16 个真实用户已同步到 UAT，**全部没有计费权益**——这是当前最明确的下一步工作。

## UAT 实测数据（2026-08-08，`console-uat.onwalk.net` / `45.77.128.182`）

```
users                     16     ← 已从 PROD 同步（含 admin/sandbox/review 等内部账号）
identities                 4
subscriptions              0     ← 无任何订阅记录
billing_plans              2     ← 仅 TRIAL-7D / FREE，且 stripe_price_id 均为空
account_billing_profiles   0     ← ★ 16 个用户无一有计费权益
account_quota_states       1     ← 仅 admin，且是早期测试残留
traffic_minute_buckets   138     ← 真实流量数据，计量链路确实在工作
billing_ledger           138
```

`users` 表有 `groups` 列，**没有** `segments` 列——[06](./06-management-console-integration.md#账号分类标签新概念独立于套餐) 设计的账号分类标签尚未落库。

### 无计费权益的账号名单（16/16）

```
guopan1227@gmail.com        2026-02-02
1195129840@qq.com           2026-03-01
624220860@qq.com            2026-03-02
haitaopanhq@gmail.com       2026-03-02
review@svc.plus             2026-03-16
sukhadukkha@163.com         2026-04-24
challen66@163.com           2026-06-08
qinlee2020@hotmail.com      2026-06-20
13191446515@163.com         2026-07-04
manbuzhe2009@qq.com         2026-07-13
sandbox@svc.plus            2026-07-14
1032581954@qq.com           2026-07-28
sbo60618@gmail.com          2026-07-29
1073303225@qq.com           2026-08-04
xiaoqiang360361@gmail.com   2026-08-07
admin@svc.plus              2026-08-08
```

内部账号（`admin@` / `sandbox@` / `review@` / `haitaopanhq@`）在回填时应打 `ops` 标签，与真实用户区分，避免污染后续经营报表。

## 已完成

| 项 | PR / 产出 | 状态 |
| --- | --- | --- |
| Stripe 密钥接线（Vault→CI→secrets.env） | [playbooks#260](https://github.com/ai-workspace-infra/playbooks/pull/260)、[toolkit#277](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/277) | ✅ 已合并并部署 |
| Stripe 目录 IaC 化 | [accounts#55](https://github.com/ai-workspace-services/accounts/pull/55) | 🟡 待合并 |
| 规划文档 01~07 | [docs#16](https://github.com/ai-workspace-services/docs/pull/16) | 🟡 待合并 |
| PROD→UAT 用户数据同步 | [run 31255959162](https://github.com/ai-workspace-infra/platform-ops-toolkit/actions/runs/31255959162) | ✅ 15 users + 4 identities + 496 sessions，收敛校验通过 |

## 阻塞中

| 阻塞项 | 影响 | 谁能解 |
| --- | --- | --- |
| **Vault 策略未授权** | `github-actions-platform-ops-toolkit-uat` 角色可能没有 `kv/data/billing-service` 读权限。已部署但 `docker exec web-saas-accounts printenv STRIPE_SECRET_KEY` 仍为空 | 需要有 Vault 管理权限的人确认/授予 |
| **Stripe 目录未创建** | 需要 `STRIPE_SECRET_KEY` 跑 `accounts/scripts/stripe-sync-catalog.sh` | 需要有 Stripe 后台权限的人 |
| **PR 合并** | accounts#55、docs#16 | 本会话中所有 PR 合并均被权限分类器拦截，需人工合并 |
| **充值不入余额（代码缺口）** | `checkout.session.completed` 的一次性支付分支只写订阅记录、**不动 `current_balance`**。PAYG 用户充值成功后余额仍是 0——该档核心路径是坏的 | 需要写代码，见 [09](./09-trial-grants-and-topup-gap.md#二充值不入余额代码缺口)。不阻塞 Pro 订阅，只阻塞 PAYG |

⚠️ **Stripe 密钥为空是"优雅降级"而非故障**：accounts 的 stripe 客户端在密钥为空时软性 disabled（`api/stripe.go` 的 `enabled()`），不阻塞部署。密钥到位后自动生效，无需改代码或重新合并任何 PR。

## 下一步（按依赖顺序）

1. **回填计费权益** —— 16/16 账号缺 profile，这是 [S1.5](./04-delivery-phases.md#s15--存量生产用户迁移s2-前置不可跳过) 的核心。**不依赖 Stripe**，现在就能做
   - 需先在 `billing_plans` 写入 `LEGACY-GRANDFATHERED` 等目录条目
   - 需给 `users` 加 `segments` 列（[06](./06-management-console-integration.md)）
   - 回填脚本必须幂等 + 支持 `--dry-run` + 事务内完成三件事（profile / quota_states / segments）
2. **Stripe 目录同步** —— 阻塞于上面的 Vault/Stripe 权限
3. **S1 权益闭环验证** —— 阻塞于 2

## 踩过的坑（不要重踩）

这些都是本会话中真实发生并已修复的问题，接手时如果看到类似症状，先看这里：

| 坑 | 症状 | 根因与修复 |
| --- | --- | --- |
| **`account_user` 无任何表权限** | 部署全绿但用量恒为 0，billing ingest 报 `SQLSTATE 42501` | schema 以 `psql -U postgres` 灌入，所有表属主是 postgres。修复：[playbooks#255](https://github.com/ai-workspace-infra/playbooks/pull/255) 转移属主 + accounts 改用 `account_user` 连库 |
| **序列属主不能单独改** | 属主转移脚本报 `cannot change owner of sequence` | serial/identity 序列跟随所属表。修复：[playbooks#256](https://github.com/ai-workspace-infra/playbooks/pull/256) 先改表带走序列，再收独立序列 |
| **JSON 字段名大小写漂移** | Portal 整页白屏 `Cannot read properties of undefined (toLocaleString)` | 4 个响应体 struct 无 json tag，Go 默认输出大驼峰，前端读小驼峰。**ledger 为空时零症状，有数据才炸**。修复：[accounts#49](https://github.com/ai-workspace-services/accounts/pull/49) 补 tag + 契约测试 |
| **`v*` tag 触发生产部署** | 打快照 tag 意外把 toolkit 拉进 prod apply | 修复：[toolkit#260](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/260) 快照 tag 闸门 + [toolkit#261](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/261) `v*` push 改 plan-only + [toolkit#262](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/262) master pipeline 同类修复 |

**共同教训**：这条链路上反复出现的失败模式是**静默失败**——部署全绿、日志无异常，但数据没落地或字段读错。任何新增的关键步骤都应该有"真的会失败"的显式校验，而不是只看 exit code。

## 重要的设计约束（改代码前必读）

1. **`groups` 字段不能承载业务分类**。它的真实语义是节点路由可达性（`EligibleNodeGroups`），贯穿 `internal/agentserver/registry.go`。账号分类要用独立的 `segments` 字段——把两个含义塞进一个字段正是上面 accounts#49 那类 bug 的根因
2. **billing-service 永不直连 Stripe**。Stripe 只与 accounts 对话，这是既有架构边界
3. **PostgreSQL 是账务事实源**，超额不上报 Stripe metered usage（[01](./01-plan-catalog.md#stripe-对象映射)）
4. **Stripe 价格不可变**。改价必须换新 `lookup_key`，不能原地改（[05](./05-stripe-catalog-automation.md)）
5. **手动调余额必须写 ledger**，不能直接 UPDATE `current_balance`（[03](./03-operations-console.md#余额调整必须走-ledger)）
6. **充值入账必须按 `payment_intent` 幂等**。Stripe webhook 会重试，重复入账等于凭空发钱（[09](./09-trial-grants-and-topup-gap.md#幂等是硬要求)）

## 环境速查

| 项 | 值 |
| --- | --- |
| UAT web-saas 主机 | `45.77.128.182`（`console-uat.onwalk.net`） |
| UAT agent-proxy 主机 | `66.42.45.216`（`agent-proxy.onwalk.net`） |
| UAT 数据库 | `docker exec -i web-saas-postgresql psql -U postgres -d account` |
| Vault Stripe 密钥路径 | `kv/billing-service`，键 `SANDBOX_STRIPE_*` / `PROD_STRIPE_*` |
| 部署工作流 | `platform-ops.yaml`，`run_full_stack=true`、`target_domains=web-saas + agent-proxy` |
| PROD→UAT 数据同步 | `data-migration.yaml`，`migration_scope=accounts`、`accounts_dry_run=false` |

**部署注意**：`deploy_tag` 必须是不可变镜像 tag（如 `daily-build-2026.08.07`），`main`/`latest` 会被拒绝。
