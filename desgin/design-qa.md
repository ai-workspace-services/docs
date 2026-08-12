# 运营管理面板 Design QA

## 结论

**PASS（本地实现与权限边界）**

本次 Build 2 已完成三张设计稿对应的 Portal 页面与 BFF 路由接入：

- `/panel/ops`：运营工作台，对应图片 1；
- `/panel/ops/accounts`：账号处置台，对应图片 2；
- `/panel/ops/billing/ledger`：计费运营总览，对应图片 3。

## 检查项

| 检查项 | 结果 | 说明 |
| --- | --- | --- |
| 设计稿已保存 | PASS | `01-operations-workbench.png`、`02-account-triage.png`、`03-billing-operations-overview.png` |
| TypeScript | PASS | `npm run typecheck` |
| ESLint | PASS | 0 error；10 条为仓库既有 warning |
| Production build | PASS | `npm run build`，新增 billing API 路由被识别 |
| 未登录访问 | PASS | `/panel/ops` 与 `/panel/ops/billing/ledger` 重定向 `/login` |
| 未登录 API | PASS | `/api/admin/billing/ledger` 返回 401 |
| 普通用户权限 | PASS（代码路径） | 页面守卫与 BFF 均要求 admin / operator 运营角色；普通用户不能以权限字符串单独绕过 |
| 真实金额安全 | PASS | 无前端硬编码经营金额；数据缺失显示“待同步” |
| UAT 视觉回归 | 待部署 | 当前 UAT 对新 `/panel/ops` 返回 404，需要流水线部署后做登录态截图回归 |

## 视觉实现检查

- 采用图片 2 的账号处置台作为 Build 2 主路径：搜索、关注账号列表、右侧账号预览、套餐 / 试用 / 配额 / 余额 / 清欠费操作和审计入口均已实现。
- 图片 1 的工作台优先展示经营指标、趋势、快捷处置、审计和用量 TopN；聚合接口未返回时显示明确空状态。
- 图片 3 的计费总览优先展示收款、待入账、欠费、审批、账单例外、资金流和对账可信度；PAYG 入账链路告警固定保留。
- 操作弹层要求填写原因；资金操作额外由 `admin.billing.money.write` 控制。

## 部署后回归清单

1. 以 root、管理员、运营者分别登录 UAT，确认三个入口可见且页面可进入。
2. 以普通用户登录 UAT，确认入口不显示，直接访问三个路由和四个 BFF 均被拒绝。
3. 用真实账号完成一次套餐、试用、配额和清欠费变更，核对审计前后值。
4. 在 PAYG 测试账号上覆盖支付成功、重复回调、账本入账、余额增加和对账完成。
5. 记录 1440px 桌面截图，与三张设计稿做最终视觉回归。

