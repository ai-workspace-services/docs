# UAT 实施、灰度与验收

## 1. 发布前置条件

- UAT 使用同一不可变 snapshot tag，例如 `daily-build-2026.07.31-rN`；Portal、Accounts、Billing、部署编排必须可追溯到该 tag 或 digest。
- `console-uat.onwalk.net`、`accounts-uat.onwalk.net`、`agent-proxy.onwalk.net` 均解析到本次 UAT 资源；不得混用生产域名。
- Accounts、Billing、Exporter 的 Vault role 仅能读取 UAT 需要的凭据；不使用 GitHub Secrets 保存运行时数据库密码或内部 token。
- Accounts 的 accounting schema 已存在；Billing 不负责在真实 UAT 数据库上执行破坏性 bootstrap DDL。
- 至少一个测试用户拥有稳定 UUID、有效套餐/配额，且已被当前 Agent 配置渲染到 Xray。

## 2. 实施顺序

### P0：基线和可观测准备

1. 统一 UAT 镜像 tag，记录镜像 digest、Git commit、部署时间与回滚 tag。
2. 在 agent-proxy 部署 exporter，但暂时不启动 Billing 定时任务。
3. 验证 Exporter 能采集 xHTTP/TCP 两类 Xray 实例，并以 `env=uat`、稳定 `node_id` 输出窗口快照。
4. 启用 Exporter/Billing/Accounts 的 node 级指标与日志采集；确认 Observability 的故障不会使 Exporter 服务退出。

退出标准：连续 15 分钟 snapshot 正常，`snapshot_age_seconds < 120`，没有生产域名或生产 node ID。

### P1：影子采集（不写配额）

1. Billing 读取 `EXPORTER_SOURCES_JSON`，为每个 UAT 节点显式配置 `source_id`、`base_url`、`expected_node_id`、`expected_env=uat`。
2. 执行 collect-and-rate 的 dry-run/shadow 模式，输出 candidate delta、未知 UUID、reset 事件和预计 bucket 数量。
3. 与 Exporter 原始累计值及少量人工测试流量交叉核对。

退出标准：同一窗口重跑结果一致；未知 UUID 为零或有明确排除原因；计数器 reset 不生成负用量。

### P2：受限写入

1. 仅允许测试租户写入 `traffic_stat_checkpoints`、`traffic_minute_buckets`、`billing_source_sync_state`。
2. 先写 minute bucket 和 checkpoint，核对 24 小时；确认无重复 bucket 后再启用 ledger 与 quota state 更新。
3. 每分钟或每 5 分钟运行一次 collect-and-rate；失败时保留 watermarks，不自动跳过窗口。

退出标准：重复执行不改变总用量/账本；重启 Billing 后能从 `last_completed_until` 补齐。

### P3：Accounts 聚合 API

1. 增加用户级 summary、timeseries、billing summary、nodes、audit API。
2. 添加 repository 和集成测试：登录用户只能读取自己的 account UUID；管理员路径单独鉴权和审计。
3. 对 data freshness 定义 `fresh`、`stale`、`unavailable`，避免把采集延迟显示为零流量。

退出标准：测试用户 API 数据与 PostgreSQL 聚合一致；Billing 停止时 API 仍返回最后结算值和 stale 状态。

### P4：Portal 逐卡发布

1. 先发布流量配额卡和数据新鲜度，不移除任何现有页面区块。
2. 再发布节点健康、趋势和账单摘要；每一块均独立 loading/error state。
3. 保留 `/panel/account`、二维码、订阅和 MFA 的 DOM/功能入口；仅调整布局与新增组件。

退出标准：禁用任意一个新 API 时，用户仍可以登录、查看/复制二维码、管理订阅和 MFA。

## 3. UAT 配置示例

以下为结构示例，真实 URL、token 与证书仅从 Vault 注入：

```json
[
  {
    "source_id": "uat-agent-proxy-xhttp",
    "base_url": "https://xray-exporter.agent-proxy.internal:8443",
    "expected_node_id": "agent-proxy.onwalk.net",
    "expected_env": "uat",
    "timeout": "10s"
  }
]
```

旧 `EXPORTER_BASE_URL` 可以保留作为过渡兼容，但新 UAT 配置必须以 `EXPORTER_SOURCES_JSON` 为主路径。多节点上线时新增 source，不能用同一 global URL 覆盖既有来源。

## 4. 验收用例

| 编号 | 场景 | 通过标准 |
| --- | --- | --- |
| AC-01 | Xray → Exporter | 测试用户制造确定流量后，snapshot 累计上下行增加，`node_id/env` 正确 |
| AC-02 | Exporter → Billing | Billing 从 window API 读取分页数据；错误来源不推进该 source watermark |
| AC-03 | Billing 幂等性 | 对同一 `since/until` 重跑至少两次，minute bucket、ledger、剩余配额不重复变化 |
| AC-04 | 重启回补 | 停止 Billing 10 分钟后恢复，能补齐缺口且无双扣 |
| AC-05 | 计数器 reset | 重启 Xray/重置计数后无负流量、无历史账本冲销 |
| AC-06 | Observability 隔离 | 停止指标上报或模拟 VictoriaMetrics 不可用，Billing 仍可成功落库 |
| AC-07 | Billing 隔离 | 停止 Billing，Portal 仍能显示旧结算数据和 stale，不影响二维码/登录/Xray |
| AC-08 | Accounts API 授权 | 用户 A 无法以参数、cookie 或 path 读取用户 B 的用量/账单 |
| AC-09 | Portal 降级 | 任一新 API 5xx 时仅对应卡片降级，旧订阅、MFA、二维码功能正常 |
| AC-10 | UAT 边界 | 所有 endpoint、node ID、DB/Vault role 和 Stripe 模式均为 UAT；无生产域名混入 |

## 5. 回滚策略

| 变更层 | 回滚方式 | 数据处理 |
| --- | --- | --- |
| Portal UI | 回退 Portal snapshot；旧 `/panel/account` 保持可用 | 无数据迁移 |
| Accounts 只读 API | Feature flag/路由降级为隐藏卡片 | 不删事实表 |
| Billing 调度 | 停止 scheduler，不删除 checkpoint/watermark | 修复后从 watermarks 回补 |
| Billing 写入 | 仅对已确认错误的 UAT 测试数据执行有审计的定向修正 | 禁止全表清空或直接修改生产数据 |
| Exporter | 停止 source/回退 exporter 二进制 | snapshot store 保留到回补完成 |

回滚的第一原则是停止新的写入、保留可审计事实和水位；不得通过删除 PostgreSQL 用量表来“修复”问题。

## 6. 交付物与责任划分

| 交付物 | 责任组件 |
| --- | --- |
| Window snapshot、每 source token/mTLS、运行指标 | xray-exporter / agent-proxy |
| source 配置、collect scheduler、幂等写入、状态与运维接口 | billing-service |
| 用户级聚合 repository、鉴权 API、节点/Audit 聚合 | Accounts |
| 卡片、趋势图、降级状态、现有页面兼容测试 | Portal |
| 指标摄取、告警规则、Grafana 运维看板 | Observability |
| UAT tag 固定、Vault OIDC、部署和跨域探针 | GitOps / Playbooks / Platform Ops |

## 7. 上线判定

以下条件全部满足才能从 UAT 推进：

1. AC-01 至 AC-10 全部通过，且结果与 snapshot tag、镜像 digest 关联归档。
2. 连续 24 小时 Billing watermarks 无无故跳跃，重复任务零重复账单。
3. Observability 断链演练证明不影响 Billing 事实写入。
4. Portal 降级演练证明不影响现有订阅、二维码、MFA、登录和 Agent 配置主链路。
5. UAT 测试账本和配额结果由产品/财务负责人确认，之后才开启生产变更评审。
