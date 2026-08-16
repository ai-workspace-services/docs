# Cloud Run deployment

The existing VPS Docker/Compose deployment remains unchanged and continues to
use `DOCS_SERVICE_PORT=8084`. Cloud Run uses the dedicated
`Dockerfile.cloudrun` (which includes Git for repository bootstrap), injects
`PORT=8080`, and the service uses
that value when the VPS-specific variable is absent.

Build and deploy the preview service:

```bash
make cloudrun-build CLOUD_RUN_ENV=preview
make cloudrun-deploy CLOUD_RUN_ENV=preview
```

Use `CLOUD_RUN_ENV=prod` for production. Override `GCP_PROJECT`, `GCP_REGION`,
`CLOUD_RUN_SERVICE`, or `CLOUD_RUN_IMAGE` when the project or Artifact Registry
path differs.

Required Secret Manager secret:

- `internal-service-token`: the service-to-service bearer token

The knowledge repository is cloned into Cloud Run's ephemeral writable
filesystem on startup and refreshed according to `DOCS_RELOAD_INTERVAL`. Keep
the repository public, or extend the service's Git authentication contract
before pointing it at a private repository.
