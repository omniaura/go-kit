#!/usr/bin/env bash

set -euo pipefail

modules=()
while IFS= read -r module; do
  modules+=("${module}")
done < <(
  awk '
    $1 == "use" && $2 == "(" {
      in_use = 1
      next
    }
    $1 == "use" && $2 != "(" {
      path = $2
      sub(/^\.\//, "", path)
      print path
      next
    }
    in_use && $1 == ")" {
      in_use = 0
      next
    }
    in_use && NF > 0 {
      path = $1
      sub(/^\.\//, "", path)
      print path
    }
  ' go.work
)

if [ "${#modules[@]}" -eq 0 ]; then
  echo "no workspace modules found in go.work" >&2
  exit 1
fi

for module in "${modules[@]}"; do
  echo "==> go test ./${module}/..."
  (
    cd "${module}"
    go test ./...
  )
done
