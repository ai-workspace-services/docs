# Stripe 目录自动化（Product / Price / Webhook）

不在 Stripe Dashboard 里手工点。Product、Price、Webhook Endpoint 全部走 Stripe REST API，由一份声明式配置文件驱动，脚本幂等同步。

**动机**：手工点 Dashboard 的操作没有版本记录、无法复现、切换 Stripe 账号（例如从当前的中国区 sandbox 切到 Stripe US 生产账号）要重新点一遍且容易漏项。走 API 之后，"配置一套 Stripe 账号"退化成"设置一个环境变量、跑一个脚本"。

## 文件

| 文件 | 作用 |
| --- | --- |
| [`accounts/scripts/stripe-catalog.yaml`](../../../accounts/scripts/stripe-catalog.yaml) | 唯一事实源：应该有哪些 Product / Price / Webhook。不含任何密钥，可提交、可 code review |
| [`accounts/scripts/stripe-sync-catalog.sh`](../../../accounts/scripts/stripe-sync-catalog.sh) | 读取上面的文件，对 Stripe API 做幂等同步 |

与 [01-plan-catalog.md](./01-plan-catalog.md) 的四档设计一一对应：`PRO-MONTHLY`/`PRO-YEARLY` 两个订阅价格，`PAYG-TOPUP-{50,100,500}` 三个充值面额。`FREE`/`CUSTOM-*` 不接 Stripe，目录里没有它们。

## 幂等策略

Stripe 的对象各有不同约束，脚本按对象类型分别处理：

| 对象 | 存在性判断 | 已存在时的行为 |
| --- | --- | --- |
| Product | 自定义 `id`（= 目录里的 `key`，如 `pro`），跨账号稳定不变 | 更新 `name`/`description`（这两个字段允许改） |
| Price | `lookup_key`（如 `pro-monthly`） | **只读不改**——Stripe 的价格金额创建后不可变。若目录里的金额和线上不一致，脚本只警告，绝不改价；要变价必须换新 `lookup_key`（如 `pro-monthly-v2`），旧价格保留给存量订阅续费用 |
| Webhook Endpoint | 按 `url` 匹配 | 同步 `enabled_events`；签名密钥只在**创建那一刻**由 Stripe 返回一次，已存在时脚本明确提示"不会重新打印"，不会假装给你一个假密钥 |

## 用法

```bash
cd accounts
STRIPE_SECRET_KEY=sk_test_xxx \
  scripts/stripe-sync-catalog.sh --env uat --domain-base onwalk.net
```

先加 `--dry-run` 看会做什么，不产生任何副作用（只读请求仍会发生，用来判断存在性）：

```bash
STRIPE_SECRET_KEY=sk_test_xxx \
  scripts/stripe-sync-catalog.sh --env uat --domain-base onwalk.net --dry-run
```

Webhook URL 由 `--env`/`--domain-base` 拼出：`https://accounts-{env}.{domain-base}/api/billing/stripe/webhook`；`--env prod` 时不带环境前缀（`https://accounts.{domain-base}/...`），与既有域名约定一致。

### 输出

脚本最后打印一张 `plan_id → stripe_price_id` 对照表：

```
== billing_plans.stripe_price_id to write ==
  PRO-MONTHLY      -> price_1AbC...
  PRO-YEARLY       -> price_1DeF...
  PAYG-TOPUP-50    -> price_1GhI...
  PAYG-TOPUP-100   -> price_1JkL...
  PAYG-TOPUP-500   -> price_1MnO...
```

把这些值通过 admin 接口写回 `billing_plans`：

```bash
curl -X PUT "https://accounts-uat.onwalk.net/api/auth/admin/billing/plans/PRO-MONTHLY" \
  -H "Content-Type: application/json" --cookie "<admin session>" \
  -d '{"planId": "PRO-MONTHLY", "stripePriceId": "price_1AbC...", "displayName": "Pro 月付", "kind": "subscription", "includedQuotaBytes": 21474836480, "packageName": "pro", "active": true}'
```

（这一步目前需要已登录的 admin session；03-operations-console.md 里规划的管理端能力补齐后可以脚本化。）

若创建了新 Webhook Endpoint，脚本会打印一次性显示的签名密钥——手动写入 Vault：

```bash
vault kv patch kv/billing-service SANDBOX_STRIPE_WEBHOOK_SECRET='whsec_...'
```

## 切换 Stripe 账号（例如切到 Stripe US）

1. 在新账号下建一个新的 restricted/secret key
2. 重跑同一个脚本，只换 `STRIPE_SECRET_KEY`：
   ```bash
   STRIPE_SECRET_KEY=sk_live_us_xxx \
     scripts/stripe-sync-catalog.sh --env prod --domain-base svc.plus
   ```
3. 新账号下会创建出结构完全相同、`id`/`lookup_key` 完全相同的 Product/Price/Webhook（`lookup_key` 与 `id` 是脚本写死在目录文件里的，不是 Stripe 自动生成的，所以两个账号里的标识符一致）
4. 把新账号返回的 `price_id`（**值不同**，但 `plan_id` 到目录的映射关系不变）和新的 webhook secret 更新到对应环境的 `billing_plans` 与 Vault

`lookup_key`/自定义 `id` 跨账号一致这一点是关键：应用代码、`billing_plans.plan_id`、这份目录文件都不需要因为换账号而改一个字符，只有 Vault 里的密钥和 admin 接口里的 `stripe_price_id` 两处需要更新。

## 已知限制

- **PAYG 充值面额固定**（¥50/¥100/¥500）。支持任意金额需要改用 Stripe Checkout 的 `custom` 金额模式，不是这个脚本要解决的问题（脚本管的是目录，不是单次下单流程）
- **改价必须手工决定新 `lookup_key`**，脚本不会自动生成版本号——这是有意的：价格变更是业务决策，不该被自动化悄悄做掉
- **`billing_plans` 回写目前仍是手动一步**，见上文；等 03-operations-console.md 的管理端接口补齐后可以让脚本直接调用
