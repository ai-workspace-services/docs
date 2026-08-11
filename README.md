# docs.svc.plus

Go service for Cloud-Neutral documentation delivery and `docs-agent` actions.

## Environment

- `KNOWLEDGE_REPO_PATH`: Absolute path to the local git repository (e.g., `/Users/xxx/knowledge`). If the directory is not a git repo but `KNOWLEDGE_REPO_URL` is set, the service will attempt to initialize and sync it from the upstream.
- `KNOWLEDGE_REPO_URL`: (Optional) Upstream git repository URL to watch and sync from (e.g., `https://github.com/haitaopanhq/knowledge.git`).
- `DOCS_SERVICE_PORT`: HTTP listen port
- `INTERNAL_SERVICE_TOKEN`: shared service-to-service auth token
- `DOCS_RELOAD_INTERVAL`: background reload interval, for example `5m`

## Endpoints

- `GET /docs`
- `GET /docs/{collection}`
- `GET /docs/{collection}/{slugPath}`
- `GET /healthz`
- `GET /api/v1/docs/home`
- `GET /api/v1/docs/collections`
- `GET /api/v1/docs/search?query={query}&lang={zh|en}`
- `GET /api/v1/docs/pages/{collection}/{slugPath}`
- `GET /api/v1/blogs`
- `GET /api/v1/blogs/{slugPath}`
- `GET /api/v1/home/latest-blogs`
- `POST /api/v1/admin/reload`
- `POST /api/v1/agent/invoke`

All `/api/v1/*` endpoints require `X-Service-Token`.

## Documentation experience

The service keeps each document available in several complementary forms:

- rendered HTML for the reader;
- original Markdown for copying, AI-assisted reading, and portable reuse;
- plaintext for local full-text search;
- heading metadata for an on-page table of contents;
- repository source paths and GitHub edit links when `KNOWLEDGE_REPO_URL` points to GitHub.

Search is local to the synchronized knowledge repository. It indexes titles,
descriptions, tags, and document bodies without sending documentation content to
a third-party search provider. The portal loads search as an optional enhancement;
the document HTML and Markdown remain usable when JavaScript search is unavailable.

## CI/CD 与 Vault 鉴权 (Vault OIDC Role)

本仓库的持续集成流水线 (`.github/workflows/ci-pipeline.yml`) 使用 GitHub Actions OIDC 机制与 HashiCorp Vault (`vault.svc.plus`) 进行无密钥身份认证。

为了遵循最小权限原则（Least Privilege）和环境隔离，本仓库的 CI 拥有独立的 Vault Policy 和 Role，具体安全约束如下：

1. **凭据访问范围（路径隔离）**
   - CI 流水线仅拥有 `kv/data/CICD` 的**只读**权限。
   - 该路径仅包含基础的公共服务凭据（例如 GHCR_USERNAME 和 GHCR_TOKEN），用于构建完成后推送镜像。
   - CI 无法读取任何环境特有的底层云资源凭据、Terraform State 或主机 SSH 部署私钥。

2. **身份铸造限制（绑定收紧）**
   - 本服务在 Vault 中对应 3 个独立环境的 Role（`sit`、`uat`、`prod`）。
   - **`job_workflow_ref` 白名单钉死**：Vault 强制校验调用方的流水线文件。只有本仓库白名单内的流水线（即 `ci-pipeline.yml`）发起的请求才能成功换取 Token。
   - 仓库内任何人**新增**或**重命名**未经授权的 workflow 文件，皆无法绕过限制获取凭据。

> **⚠️ 排障指南 (403 Forbidden)**
> 如果 CI 流水线在 `Fetch Vault Secrets` 步骤报 `403` 权限拒绝，请确认：
> 1. 请求的凭据路径是否超出了 `kv/data/CICD` 层的范围。
> 2. 流水线文件名称或仓库名称是否发生了变更。
> 
> 如果确需修改流水线名称，必须由管理员在 `platform-ops-toolkit` 仓的 `docs/tasks/vault_service_repo_roles.sh` 中更新白名单，并重新对 Vault 服务端执行该脚本。
