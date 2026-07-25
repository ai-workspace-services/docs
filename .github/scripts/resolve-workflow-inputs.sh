#!/usr/bin/env bash
set -euo pipefail

EVENT_NAME="${EVENT_NAME:-}"
GITHUB_REF="${GITHUB_REF:-}"
INPUT_ENV="${INPUT_ENV:-}"

case "${EVENT_NAME}" in
  pull_request)
    push_image="false"
    deployment_environment="sit"
    ;;
  push)
    push_image="true"
    if [[ "${GITHUB_REF}" == refs/heads/main || "${GITHUB_REF}" == refs/heads/release/* ]]; then
      deployment_environment="uat"
    elif [[ "${GITHUB_REF}" == refs/tags/* ]]; then
      deployment_environment="prod"
    else
      deployment_environment="uat"
    fi
    ;;
  workflow_dispatch)
    push_image="true"
    deployment_environment="${INPUT_ENV:-uat}"
    ;;
  *)
    push_image="false"
    deployment_environment="sit"
    ;;
esac

{
  echo "push_image=${push_image}"
  echo "deployment_environment=${deployment_environment}"
} >> "$GITHUB_OUTPUT"
