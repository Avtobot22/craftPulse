#!/bin/sh
# Генерирует и собирает контракт, не добавляя сервер или frontend в проект.
set -eu
PROJECT_ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
CHECK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/craftpulse-openapi-generators.XXXXXX")
trap 'rm -rf "$CHECK_DIR"' EXIT HUP INT TERM

go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 \
  --config "$PROJECT_ROOT/scripts/openapi/go-codegen.yaml" \
  --o "$CHECK_DIR/contract.gen.go" \
  "$PROJECT_ROOT/docs/technical-specification/openapi.yaml"
(
  cd "$CHECK_DIR"
  go mod init craftpulse-contract-check
  go get github.com/gin-gonic/gin@v1.12.0 github.com/oapi-codegen/runtime@v1.8.0
  go mod tidy
  go build ./...
)
"$PROJECT_ROOT/scripts/openapi/node_modules/.bin/openapi-typescript" \
  "$PROJECT_ROOT/docs/technical-specification/openapi.yaml" -o "$CHECK_DIR/openapi.d.ts"
# Генератор использует peer TypeScript 5.x; результат отдельно проверяется компилятором 6.
npm exec --yes --package typescript@6.0.3 -- \
  tsc --strict --noEmit --skipLibCheck false --ignoreConfig "$CHECK_DIR/openapi.d.ts"
printf '%s\n' 'Go/Gin generation and TypeScript 6 strict compilation passed.'
