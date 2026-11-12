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
