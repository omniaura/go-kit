#!/usr/bin/env bash

set -euo pipefail

modules=(
  cmd/monotag
  convert
  convert/sqlconv/pgconv
  errs
  mapcache
  net
  set
  slicer
  tasker
)

for module in "${modules[@]}"; do
  echo "==> go test ./${module}/..."
  (
    cd "${module}"
    go test ./...
  )
done
