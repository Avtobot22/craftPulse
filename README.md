# craftPulse

Платформа аналитики разработки: связывает задачи, изменения кода, CI и находки анализаторов, показывает ролевые дашборды с раскрытием до исходных событий.

## Состояние проекта

В репозитории находятся продуктовые документы и каркас backend на Go с процессами `api` и `worker`, PostgreSQL 18 и S3-совместимым хранилищем. Frontend пока не реализован. Контракт API хранится в `docs/technical-specification/openapi.yaml`.

## Проверка репозитория

Нужны Git и Python 3.9+ без дополнительных пакетов:

```sh
python3 scripts/check_repository.py
python3 -m unittest discover -s tests -v
git diff --check
```

После клонирования подключи локальную проверку заголовка коммита:

```sh
git config --local core.hooksPath .githooks
```

## Проверка backend и контракта

Нужны Go версии из `backend/go.mod`, Node.js 26.3.0 с npm и Docker с Linux-контейнерами. Для `go test -race` нужен C-компилятор: GCC на Linux или MinGW-w64 на Windows. Добавь в `PATH` каталог `bin` внутри пути, который выводит `go env GOPATH`. Команды ниже рассчитаны на Linux, как раннер CI; на Windows полный набор можно выполнять в Linux-контейнере с Go и Node.js.

В корне репозитория проверь контракт теми же командами, что выполняет `repository-check`:

```sh
npm --prefix scripts/openapi ci --ignore-scripts --no-audit --no-fund
npm --prefix scripts/openapi run verify
npm --prefix scripts/openapi run generators
npm --prefix scripts/openapi run docs
```

Проверки backend выполняются из `backend/`. Линтер закреплён на версии 2.15.0; `govulncheck` устанавливается в актуальной версии:

```sh
cd backend
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
golangci-lint config verify
golangci-lint run --timeout=5m
golangci-lint fmt --diff
go generate ./...
git diff --exit-code
git ls-files --others --exclude-standard
go vet ./...
go test -race ./...
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
docker build -t craftpulse-server:local -f Dockerfile .
```

После генерации `git diff --exit-code` должен завершиться успешно, а список новых файлов должен быть пустым. Перед проверкой зафиксируй ожидаемые изменения: CI сравнивает генерацию с содержимым коммита. Код oapi-codegen и sqlc хранится в Git. Интеграционные тесты сами поднимают PostgreSQL 18 и SeaweedFS через testcontainers-go; работающий Docker обязателен, тесты без него не пропускаются.

Границы слоёв проверяет depguard в `backend/.golangci.yml`. Для сценариев и репозиториев перечислены модули из раздела 10.3 ТЗ: разрешён импорт собственных подпакетов, а связи между модулями передаются через интерфейсы в `internal/app`. Новый модуль нужно добавить в эти правила; для неизвестных модулей импорты других сценариев и реализаций репозиториев запрещены по умолчанию.

Workflow `Repository` запускается на каждом PR и push в `main`. `repository-check` выполняется всегда, `backend` - при изменениях `backend/`, `deploy/postgres/` или `openapi.yaml`, а `demo-dataset` - при изменениях `demo-dataset/`. Обе Go-проверки используют общий workflow `go-check.yml`: линтер, форматирование, vet, тесты с детектором гонок, проверку уязвимостей и сборку. Backend дополнительно проверяет генерацию и собирает Docker-образ без публикации.

Изменение `repository.yml` или `go-check.yml`, как и ручной запуск, включает обе Go-проверки. Итоговый job `ci` требует успешные `detect` и `repository-check`, а пропуск Go-проверки принимает только для незатронутого модуля. Устаревшие прогоны отменяются только на PR; каждый push в `main` проверяется отдельно.

## Манифесты пакетов CI

Схемы `ScanManifestV1` и `JenkinsExecutionManifestV1` находятся в [envelope.yaml](docs/technical-specification/envelope.yaml). После первого `manifest.json` архив с отчётами содержит `scan-manifest.json`, `records.jsonl` и файлы под `files/`; порядок этих последующих записей произвольный. Для Jenkins `execution-manifest.json` необязателен: без него внутренние повторы остаются неизвестными.

Готовые примеры: [GitLab builtin](backend/pkg/envelope/testdata/ci/gitlab-builtin/manifest.json), [внешний SARIF](backend/pkg/envelope/testdata/ci/external-sarif/manifest.json), [таймаут с частичным результатом](backend/pkg/envelope/testdata/ci/timeout/manifest.json), [Jenkins fail→pass и matrix](backend/pkg/envelope/testdata/ci/jenkins-retry/execution-manifest.json). Из корня репозитория:

```sh
tar -czf /tmp/ci-example.tar.gz \
  -C backend/pkg/envelope/testdata/ci/jenkins-retry \
  manifest.json scan-manifest.json execution-manifest.json records.jsonl files
cd backend
go run ./cmd/envelope check /tmp/ci-example.tar.gz
```

Команда возвращает `0`, если пакет принят, `1` при отклонении всего пакета и `2`, если отклонены отдельные записи. Отчёт без scan manifest даёт `missing_scan_manifest`, неизвестная версия любого CI-манифеста - `unsupported_manifest_version`; нарушение схемы или provenance - `invalid_scan_manifest` либо `invalid_execution_manifest`. При отклонении `Clean` ничего не пишет. Очистка сохраняет оба манифеста и файлы, на которые они ссылаются; `ReadCleaned` передаёт их нормализатору.

Scan manifest описывает каждый запуск инструмента: producer и версию, профиль и image digest, режим, вид отчёта, компонент и путь, язык, rules/config/scope hashes, базу и время её публикации, parser version, SHA/digest, координаты CI, времена, exit class, полноту, counts, пропущенные цели с кодом причины, диапазон истории и список файлов с форматом и версией. `null` означает неизвестность или неприменимость. `failed`, `timeout`, `unknown`, пропуски и усечение истории исключают `complete`. Значения секретов, фрагменты находок и свободные evidence-объекты в схеме отсутствуют.

Формат Jenkins совпадает с набором показа. Ключ попытки `JenkinsAttemptKey` - SHA-256 JSON-массива `[job, build_number, stage, name, configuration_hash, attempt]`; организация и instance задаются внешней identity. `dataset_attempt_id`, агент и времена в ключ не входят. Fingerprint конфигурации - SHA-256 JSON объекта строковых axes, с ключами в лексикографическом порядке и стандартным JSON-экранированием Go. Файл отчёта сохраняет класс `file`: namespace - идентичность попытки (без неё SHA/digest), native ID - `report_kind:path`, content hash включает hash файла.

Поддержана версия `1`. Новые поля внутри неё добавляются необязательными, существующие поля и их смысл сохраняются; несовместимое изменение требует новой версии. Пакеты задач и остальных фактов сохраняют прежний формат. Производителей пакетов с отчётами нужно обновить вместе с валидатором: теперь они обязаны передавать scan manifest. Очистка не добавляет происхождение от имени производителя. Дублируемые поля provenance в `report_file` должны совпадать со scan manifest, если переданы; времена сравниваются как моменты времени. Ссылки execution manifest дополнительно сверяются с форматом отчёта и координатами Jenkins: job, build, ключом попытки и её номером.

## Документация и работа команды

- [Документация продукта](docs/README.md)
- [Видение продукта](docs/product/vision.md)
- [Метрики](docs/product/metrics.md)
- [Набор данных показа и независимая сверка](demo-dataset/README.md)
- [Работа над задачами и PR](CONTRIBUTING.md)
- [Доска, настройки и автоматизация GitHub](docs/github-workflow.md)
- [Подготовка автоматизации зависимостей с Copilot](docs/copilot-automation.md)
- [Секреты и уязвимости](SECURITY.md)
- [Доска craftPulse](https://github.com/users/Avtobot22/projects/4)
