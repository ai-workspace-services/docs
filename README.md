# docs.svc.plus

Go service for Cloud-Neutral documentation delivery and `docs-agent` actions.

## Environment

- `KNOWLEDGE_REPO_PATH`: Absolute path to the local git repository (e.g., `/Users/xxx/knowledge`). If the directory is not a git repo but `KNOWLEDGE_REPO_URL` is set, the service will attempt to initialize and sync it from the upstream.
- `KNOWLEDGE_REPO_URL`: (Optional) Upstream git repository URL to watch and sync from (e.g., `https://github.com/haitaopanhq/knowledge.git`).
- `KNOWLEDGE_REPO_REF`: Branch, tag, or commit to synchronize (default: `main`).
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

The service initializes the Git working tree before its first index build when a
repository URL is configured. Each reload builds a new in-memory snapshot and
atomically swaps it only after parsing and rendering succeed. The snapshot exposes
stable source-file hashes and a content hash through `/healthz`, which makes
content releases observable without adding a database. A failed Git sync or index
build leaves the previous known-good snapshot serving.

## CI/CD 与 GHCR 鉴权

本仓库的 CI 只需要把当前仓库构建的镜像推送到 GHCR，不需要读取云账号、Terraform State、主机私钥或其他运行时凭据。因此 `.github/workflows/ci-pipeline.yml` 使用 GitHub Actions 为当前 run 自动签发的短期 `GITHUB_TOKEN` 登录 GHCR，并通过 workflow 的 `packages: write` 权限完成推送。

这样 content-service 的镜像构建不依赖 Vault role 的实时 provisioning，也不会把 GHCR 长期 Token 写入仓库或传递给构建脚本。Vault 仍由 platform-ops-toolkit 管理需要 Vault 的基础设施和其他服务权限；content-service 的运行时部署凭据由 GitOps/Vault 按部署环境注入。

如果未来构建需要访问超出当前仓库 GHCR 权限范围的私有资源，应新增最小权限的专用凭据和对应 Vault role，不应恢复共享的长期 GHCR Token。
