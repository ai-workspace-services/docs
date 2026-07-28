#!/usr/bin/env bash
set -euo pipefail

EVENT_NAME="${EVENT_NAME:-}"
GITHUB_REF="${GITHUB_REF:-}"
INPUT_ENV="${INPUT_ENV:-}"

if [[ "${EVENT_NAME}" == "workflow_dispatch" && -n "${INPUT_ENV}" ]]; then
  # The tag orchestration script selects the environment explicitly.
  push_image="true"
  deployment_environment="${INPUT_ENV}"
elif [[ "${EVENT_NAME}" == "pull_request" ]]; then
  push_image="false"
  deployment_environment="sit"
elif [[ "${GITHUB_REF}" == refs/heads/main ]]; then
  push_image="true"
  deployment_environment="${INPUT_ENV:-uat}"
elif [[ "${GITHUB_REF}" == refs/tags/prod-* ]]; then
  push_image="true"
  deployment_environment="prod"
elif [[ "${GITHUB_REF}" == refs/tags/sit-* ]]; then
  push_image="true"
  deployment_environment="sit"
# Operational tags carry an environment prefix, so uat-daily-build-* must be
# matched unanchored — a bare refs/tags/daily-build-* misses the tags the
# cross-repo daily snapshot actually creates, and they then fall through to
# the prod branch below.
elif [[ "${GITHUB_REF}" == refs/tags/uat-* || "${GITHUB_REF}" == *daily-build-* ]]; then
  push_image="true"
  deployment_environment="uat"
elif [[ "${GITHUB_REF}" == refs/heads/release/* || "${GITHUB_REF}" == refs/tags/* ]]; then
  push_image="true"
  deployment_environment="${INPUT_ENV:-prod}"
else
  # Vault OIDC roles for uat and prod require main or release/* refs.
  # Custom or feature branches must use sit to satisfy Vault claim validation.
  push_image="false"
  deployment_environment="sit"
fi

{
  echo "push_image=${push_image}"
  echo "deployment_environment=${deployment_environment}"
} >> "$GITHUB_OUTPUT"
