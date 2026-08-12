# UAT Xray 用量、计费与 Portal 展示

状态：`proposal`  
范围：UAT 先行，生产按同一契约迁移  
原则：只新增；保持现有 Accounts、Xray、Agent、Portal 登录与配置下发链路兼容。

## 目标

将 Xray 的用户级流量通过 `xray-exporter` 标准化采集，由 `billing-service` 进行幂等计量与配额扣减，落入 Accounts 所在 PostgreSQL；Accounts 作为唯一的用户级查询聚合层，Portal 在既有 `/panel/account` 中展示订阅、流量、节点与账单摘要。

Observability 接收同一份运行指标用于看板、告警与排障，但不是计费事实来源，也不能阻塞 Billing 写库。

## 文档

| 文档 | 内容 |
| --- | --- |
| [01-architecture-and-contracts.md](./01-architecture-and-contracts.md) | 架构、边界、数据/API/安全契约与 UI 映射 |
| [02-uat-rollout-and-acceptance.md](./02-uat-rollout-and-acceptance.md) | UAT 部署顺序、灰度、回滚和验收 |

## 关键决策

1. `xray-exporter` 向 Billing 与 Observability **扇出**，不经由 Observability 转发计费数据。
2. Billing 是流量计量、评率和配额扣减的唯一写入者；PostgreSQL 是用户账务事实源。
3. Accounts 是 Portal 的唯一后端聚合入口；Portal 不直连 Billing、Exporter、Observability 或 PostgreSQL。
4. 现有 Xray 配置生成、Agent 心跳、VLESS/WireGuard 二维码、登录与订阅接口均不改语义。
5. UAT 使用不可变跨仓 snapshot tag；`latest` 不能作为可验收发布版本。
