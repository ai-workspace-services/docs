# 进度与交接

最后更新：2026-08-09

这份文档是给接手的 Code Agent 看的：当前做到哪、下一步做什么、哪些坑已经踩过不要重踩、什么在阻塞。**其余文档描述"应该是什么"，这份描述"现在是什么"。**

## 一句话状态

所有规划与代码 PR 均已合并；**UAT 环境已于 2026-08-09 01:58 销毁**（`action=destroy`，`Destroy complete! Resources: 3 destroyed.`），当前无任何可验证环境；商业化链路仍**未激活**（Stripe 密钥为空，Vault 授权未开）。

## ⚠️ UAT 环境当前不存在（2026-08-09）

```
console-uat.onwalk.net        → HTTP 000（不可达）
accounts-uat.onwalk.net       → HTTP 000（不可达）
45.77.128.182 / 66.42.45.216  → SSH 超时（主机已销毁）
DNS console-uat.onwalk.net    → 167.179.64.91（陈旧记录，指向已不存在的主机）
```

销毁记录：[run 31289337026](https://github.com/ai-workspace-infra/platform-ops-toolkit/actions/runs/31289337026)，`INPUT_ACTION=destroy`，所有部署 job 均 skipped。

**接手第一件事：重建 UAT**，否则下面任何验证都做不了。重建命令见文末「环境速查」，注意 `deploy_tag` 要用当前有效的快照 tag。

> 本文档下方「UAT 实测数据」一节是 **2026-08-08 销毁前**的快照，保留作为参考基线——重建后数据会归零（用户需重新从 PROD 同步），不要当作现状。

## PR 状态（截至 2026-08-09，全部已合并）

| PR | 内容 | 合并时间 |
| --- | --- | --- |
| [accounts#55](https://github.com/ai-workspace-services/accounts/pull/55) | Stripe 目录 IaC 化（声明式 YAML + 幂等同步脚本） | 08-08 11:19 |
| [accounts#56](https://github.com/ai-workspace-services/accounts/pull/56) | 分段标签后端 `PUT /admin/users/:id/groups` | 08-08 22:42 |
| [portal#162](https://github.com/ai-workspace-services/portal/pull/162) | 分段标签管理台 UI | 08-08 22:42 |
| [portal#166](https://github.com/ai-workspace-services/portal/pull/166) | `/prices` 四档卡片 + 实时读目录判断可购买性 | 08-09 05:25 |
| [docs#16](https://github.com/ai-workspace-services/docs/pull/16) / [#17](https://github.com/ai-workspace-services/docs/pull/17) | 全套规划文档 | 08-08 / 08-09 |

**代码侧没有待合并项。** 剩下的工作全部是配置授权 + 新功能开发。

## PROD 目录现状（2026-08-09 实测）

```
GET https://accounts.svc.plus/api/billing/plans
→ TRIAL-7D  (stripe_price_id: 无)
→ FREE      (stripe_price_id: 无)
```

四档设计里的 `PRO-MONTHLY` / `PRO-YEARLY` / `PAYG-TOPUP-*` **一条都没写入**。因此 `/prices` 页面上两张 Pro 卡片会正确显示为「即将上线」且按钮禁用——这是预期行为，不是 bug（portal#166 特意做了这个保护，避免把用户送进必然失败的结算）。

## UAT 实测数据（2026-08-08 销毁前快照，仅供参考）

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
| Stripe 密钥接线（Vault→CI→secrets.env） | [playbooks#260](https://github.com/ai-workspace-infra/playbooks/pull/260)、[toolkit#277](https://github.com/ai-workspace-infra/platform-ops-toolkit/pull/277) | ✅ 已合并并部署（等 Vault 授权才生效） |
| Stripe 目录 IaC 化 | [accounts#55](https://github.com/ai-workspace-services/accounts/pull/55) | ✅ 已合并 |
| 分段标签后端 + 管理台 UI | [accounts#56](https://github.com/ai-workspace-services/accounts/pull/56)、[portal#162](https://github.com/ai-workspace-services/portal/pull/162) | ✅ 已合并（销毁前在 UAT 验证过路由生效） |
| `/prices` 四档定价页 | [portal#166](https://github.com/ai-workspace-services/portal/pull/166) | ✅ 已合并 |
| 规划文档全套 | [docs#16](https://github.com/ai-workspace-services/docs/pull/16)、[docs#17](https://github.com/ai-workspace-services/docs/pull/17) | ✅ 已合并 |
| PROD→UAT 用户数据同步 | [run 31255959162](https://github.com/ai-workspace-infra/platform-ops-toolkit/actions/runs/31255959162) | ✅ 曾验证通过（16 users，收敛校验 `UAT already contains every snapshot row`）；**环境销毁后需重跑** |

## 阻塞中

| 阻塞项 | 影响 | 谁能解 |
| --- | --- | --- |
| **UAT 环境已销毁** | 无任何可验证环境；所有验收、回填、闭环测试都做不了 | 重建即可，见文末「环境速查」 |
| **Vault 策略未授权** | `github-actions-platform-ops-toolkit-uat` 角色没有 `kv/data/billing-service` 读权限。实测部署日志：`kv/data/billing-service returned HTTP 403; Stripe keys left empty` | 需要有 Vault 管理权限的人授予 |
| **Stripe 目录未创建** | 需要 `STRIPE_SECRET_KEY` 跑 `accounts/scripts/stripe-sync-catalog.sh` | 需要有 Stripe 后台权限的人 |
| **充值不入余额（代码缺口）** | `checkout.session.completed` 的一次性支付分支只写订阅记录、**不动 `current_balance`**。PAYG 用户充值成功后余额仍是 0——该档核心路径是坏的 | 需要写代码，见 [09](./09-trial-grants-and-topup-gap.md#二充值不入余额代码缺口)。不阻塞 Pro 订阅，只阻塞 PAYG |

⚠️ **Stripe 密钥为空是"优雅降级"而非故障**：accounts 的 stripe 客户端在密钥为空时软性 disabled（`api/stripe.go` 的 `enabled()`），不阻塞部署。密钥到位后自动生效，无需改代码或重新合并任何 PR。

## 下一步（按依赖顺序）

0. **重建 UAT** —— 其余所有事项的前置。重建后需重新从 PROD 同步用户（`data-migration.yaml`），环境是全新的

1. **补 Vault 策略** —— 唯一的硬阻断，**不需要写代码**。补完后下次任意一次部署自动生效
2. **跑 Stripe 目录同步** —— `accounts/scripts/stripe-sync-catalog.sh`，产出 Product/Price/Webhook，拿到 `plan_id → stripe_price_id` 对照表
3. **写入四档目录** —— 经 admin API 把对照表写进 `billing_plans`。做完这步 `/prices` 上 Pro 卡片自动从「即将上线」变为可下单，**Pro 订阅即可真实收钱**
4. **回填计费权益** —— 存量账号缺 profile，[S1.5](./04-delivery-phases.md#s15--存量生产用户迁移s2-前置不可跳过) 的核心。**不依赖 Stripe**，与 1~3 可并行
   - 需先在 `billing_plans` 写入 `LEGACY-GRANDFATHERED` 等目录条目
   - 需给 `users` 加 `segments` 列（[06](./06-management-console-integration.md)）
   - 回填脚本必须幂等 + 支持 `--dry-run` + 事务内完成三件事（profile / quota_states / segments）
5. **充值入账 + ledger + 幂等** —— PAYG 档必需，涉及真实资金，见 [09](./09-trial-grants-and-topup-gap.md)
6. **运营管理接口** —— `GET /admin/billing/accounts/:uuid`（读）→ `grant-trial`（发试用）→ `assign-plan` / `adjust-balance`

**1~3 是"让钱能进来"的最短路径**，做完就能收 Pro 订阅费。4 与它们无依赖可并行。5、6 是 PAYG 与运营能力，可以之后再做。

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
| UAT 主机 | **当前不存在**（已销毁）。重建后 IP 是全新的，从 CMDB 或部署日志取，不要沿用任何历史 IP |
| UAT 域名 | `console-uat.onwalk.net` / `accounts-uat.onwalk.net` / `agent-proxy.onwalk.net` |
| UAT 数据库 | `docker exec -i web-saas-postgresql psql -U postgres -d account` |
| PROD 域名 | `console.svc.plus` / `accounts.svc.plus` |
| Vault Stripe 密钥路径 | `kv/billing-service`，键 `SANDBOX_STRIPE_*` / `PROD_STRIPE_*` |
| 部署工作流 | `platform-ops.yaml` |
| PROD→UAT 数据同步 | `data-migration.yaml`，`migration_scope=accounts` |

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
