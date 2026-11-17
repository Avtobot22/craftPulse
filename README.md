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
В `src/` используются слои `app`, `pages` и `shared`. Клиент и общие запросы находятся в `shared/api`, сессия - в `shared/auth`; каждый срез страницы и сегмент Shared экспортирует публичный API через `index.ts`.

Типы из `docs/technical-specification/openapi.yaml` хранятся в
`frontend/src/shared/api/generated/schema.d.ts`. После изменения контракта запусти
`npm run generate:api` и включи результат в тот же коммит. Клиент API и разбор ответов
используют сгенерированные типы: несовместимые изменения проверяет TypeScript.
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

### Маршруты и API-клиент

Приложение подключает Gravity UI 8, TanStack Query 5 и React Router 8. UIKit `Provider`
объединяет провайдеры темы и языка; выбраны `light` и `ru`. Экраны пока представлены
заглушками:

| UI | Адрес | Доступ |
| --- | --- | --- |
| UI-01 | `/` | После проверки сессии |
| UI-02 | `/login`, `/login/:orgSlug` | Публичный |
| UI-03 | `/session-expired` | Публичный |
| UI-04 | `/access-denied` | После проверки сессии |
| UI-05 | `/installation` | После проверки сессии |
| UI-06 | `/welcome` | После проверки сессии |
| UI-07 | `/organization/structure` | После проверки сессии |
| UI-08 | `/users`, `/users/:userId` | После проверки сессии |

Защищённые маршруты вызывают `getCurrentUser`. Для работы с ними нужен API на адресе
`API_PROXY_TARGET`; при недоступном API показывается ошибка проверки сессии.
Вход через `/o/{org_slug}/login` и callback адресуются от корня сайта; Vite
проксирует `/o/` на тот же сервер, что и `/api/v1`. `createLoginUrl` проверяет slug
и безопасный относительный `return_to`, сохраняя кодирование фильтров и фрагмент.

Клиент принимает JSON как `unknown`, проверяет поля ветвления и возвращает модели,
производные от сгенерированных типов. Неизвестные строковые значения перечислений
передаются как `unknown`, лишние поля игнорируются. Ошибки различаются по `code`,
`correlation_id` доступен для диагностики, `detail` удаляется при разборе. Ошибки
полей группируются по JSON Pointer через `fieldErrorsByPointer`.

`ApiClient.read` сохраняет прочитанный `ETag` в памяти. `update` требует эту версию
для того же URL и передаёт её в `If-Match`; при отсутствии версии запрос не отправляется.
Начальный `ETag: "0"` передаётся как прочитан; после ошибки изменения требуется новое
чтение. `logout` выполняет отдельный неверсионируемый POST по контракту.

Ответ 401 очищает кэш запросов и мутаций, закрывает локальное состояние сессии и
ведёт на UI-03 с исходным относительным путём. Поздний ответ предыдущей сессии
отклоняется. Выход после успешного ответа API очищает кэш и ведёт на UI-02;
ошибка выхода сохраняет текущую сессию. При возврате на вкладку `getCurrentUser`
запрашивается снова. Сессия и ответы API остаются в памяти; сохранения в
`localStorage`, `sessionStorage` или IndexedDB нет.

Декларации UIKit 8.1 конфликтуют с `exactOptionalPropertyTypes` при проверке самих
библиотек. Приложение использует `skipLibCheck` с сохранёнными строгими флагами кода;
сгенерированный контракт отдельно проверяется командой `npm run typecheck:api`
с `strict` и `skipLibCheck: false`. Эта команда входит в `npm run typecheck`
и общий `npm run check`.

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
