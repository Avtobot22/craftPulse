# craftPulse

Платформа аналитики разработки: связывает задачи, изменения кода, CI и находки анализаторов, показывает ролевые дашборды с раскрытием до исходных событий.

## Состояние проекта

В репозитории находятся продуктовые документы и каркас backend на Go с процессами `api` и `worker`, PostgreSQL 18 и S3-совместимым хранилищем. Frontend содержит начальную SPA на React и TypeScript без продуктовых экранов. Контракт API хранится в `docs/technical-specification/openapi.yaml`.

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

## Frontend

Нужны Node.js 24.18.0+ и npm 11.16.0. В CI и Docker используется Node.js 26.3.0.
Из корня репозитория:

```sh
cd frontend
npm ci --no-audit --no-fund
npm run generate:api
npm run check
npm run dev
```

Vite выводит адрес приложения, по умолчанию `http://localhost:5173`. Запросы `/api/v1`
проксируются на `http://localhost:8080`; другой адрес задаётся через `API_PROXY_TARGET`.
Для просмотра статической сборки выполни `npm run preview` после `npm run build`.

`npm run check` сверяет типы с OpenAPI, проверяет форматирование, ESLint, границы FSD,
тест Vitest и строгие типы TypeScript, затем собирает `dist/`. Отдельные команды:
`npm run typecheck`, `npm run lint`, `npm test`, `npm run build`.
ESLint использует type-aware правила typescript-eslint 8.72; Steiger запрещает импорт
из вышележащего слоя, между срезами одного слоя и в обход публичного API среза.
В `src/` созданы только `app`, `pages/home` и `shared/api`.

Типы из `docs/technical-specification/openapi.yaml` хранятся в
`frontend/src/shared/api/generated/schema.d.ts`. После изменения контракта запусти
`npm run generate:api` и включи результат в тот же коммит. Начальный компонент
использует `Problem.title` как строку: несовместимое изменение поля ломает TypeScript.
У генератора `openapi-typescript 7.13.0` отдельный локальный npm-пакет
`tools/api-types-generator` с поддерживаемым им TypeScript 5.9.3. Приложение и
сгенерированные типы проверяются TypeScript 6.0; общий `package-lock.json` закрепляет
обе версии без отключения проверки peer-зависимостей.

Job `frontend` в workflow `Repository` запускается при изменении `frontend/`,
`openapi.yaml`, самого `repository.yml` и при ручном запуске. Он сверяет генерацию
через `git diff --exit-code`, выполняет проверки и собирает статический артефакт
и Docker-образ без публикации. Итоговый `ci` отклоняет ошибку или отмену frontend,
а пропуск допускает только при незатронутом frontend вне ручного запуска.
PR только с изменением backend пропускает frontend. Dependabot проверяет npm-пакеты
`frontend/` раз в неделю.

Сборка и запуск образа из корня репозитория:

```sh
docker build -t craftpulse-frontend:local frontend
docker run --rm -p 127.0.0.1:8081:80 craftpulse-frontend:local
```

Открой `http://localhost:8081`. Финальный слой использует nginx стабильной ветки
1.30.6 и получает только `dist/`, без исходников и `node_modules`. Конфигурация nginx
стенда подключается при развёртывании; в образе остаётся стандартная конфигурация
для раздачи статики, без TLS и маршрутизации API стенда.

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
