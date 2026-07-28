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
elif [[ "${GITHUB_REF}" == refs/tags/daily-build-* ]]; then
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
