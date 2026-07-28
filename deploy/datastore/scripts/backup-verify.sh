#!/usr/bin/env bash
#
# backup-verify.sh — assert the WAL->S3 backup is present and restorable.
#
# `bottomless-cli verify` downloads the latest generation and checks integrity.
# Run it from cron/monitoring. A backup you never restore-test is a rumor.
# Exit 0 == verified, non-zero == the backup is not restorable: page someone.
#
# Reads the same S3_* env as docker-compose (source your .env first), plus:
#   SQLD_NAMESPACE  default namespace (default: "ns-:default")
#   SQLD_IMAGE      image providing bottomless-cli
#
set -euo pipefail

SQLD_IMAGE="${SQLD_IMAGE:-ghcr.io/tursodatabase/libsql-server:latest}"
SQLD_NAMESPACE="${SQLD_NAMESPACE:-ns-:default}"
: "${S3_BUCKET:?set S3_BUCKET}" "${S3_ENDPOINT:?set S3_ENDPOINT}"
: "${S3_ACCESS_KEY_ID:?set S3_ACCESS_KEY_ID}" "${S3_SECRET_ACCESS_KEY:?set S3_SECRET_ACCESS_KEY}"
S3_REGION="${S3_REGION:-us-east-1}"

# bottomless-cli insists on LIBSQL_BOTTOMLESS_AWS_DEFAULT_REGION specifically
# (not the generic AWS_DEFAULT_REGION).
docker run --rm \
  ${DOCKER_NETWORK:+--network "$DOCKER_NETWORK"} \
  -e LIBSQL_BOTTOMLESS_AWS_ACCESS_KEY_ID="$S3_ACCESS_KEY_ID" \
  -e LIBSQL_BOTTOMLESS_AWS_SECRET_ACCESS_KEY="$S3_SECRET_ACCESS_KEY" \
  -e LIBSQL_BOTTOMLESS_AWS_DEFAULT_REGION="$S3_REGION" \
  --entrypoint bottomless-cli "$SQLD_IMAGE" \
  -e "$S3_ENDPOINT" -b "$S3_BUCKET" -n "$SQLD_NAMESPACE" verify
