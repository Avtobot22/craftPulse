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
объединяет провайдеры темы и языка; выбраны `light` и `ru`. UI-02 и UI-03 реализуют
вход и обработку ошибок; остальные маршруты пока представлены заглушками:

| UI | Адрес | Доступ |
| --- | --- | --- |
| UI-01 | `/` | После проверки сессии |
| UI-02 | `/login`, `/login/:orgSlug`, `/o/:orgSlug` | Публичный |
| UI-03 | `/session-expired` | Публичный |
| UI-04 | `/access-denied` | После проверки сессии |
| UI-05 | `/installation` | После проверки сессии |
| UI-06 | `/welcome` | После проверки сессии |
| UI-07 | `/organization/structure` | После проверки сессии |
| UI-08 | `/users`, `/users/:userId` | После проверки сессии |

Защищённые маршруты вызывают `getCurrentUser`. Для работы с ними нужен API на адресе
`API_PROXY_TARGET`; при недоступном API показывается ошибка проверки сессии.
Кнопка входа переводит браузер на серверный `/o/{org_slug}/login`. Vite проксирует
`/o/*/login` и `/o/*/callback` на тот же сервер, что и `/api/v1`; публичный
`/o/:orgSlug` отдаётся SPA и принимает `login_error` из серверного redirect.
`createLoginUrl` проверяет slug и безопасный относительный `return_to`, сохраняя
кодирование фильтров и фрагмент. Ошибки входа показывают причину и доступное действие
без ответа провайдера и данных предыдущей сессии.

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
запрашивается снова. Сессия и ответы API остаются в памяти. Для повторного входа
`sessionStorage` хранит только проверенный slug организации из последнего
`getCurrentUser`. Первый вход выполняется по адресу организации; её списка на
экране входа нет. После повторного входа данные читаются заново, форма автоматически
не отправляется. Заголовки UI-02 и UI-03 не получают автоматический фокус.
Tab переключает кнопки и ссылки; Enter и Space нажимают кнопки.

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

## Загрузка пакета из CI

`POST /api/v1/ingest/packages` принимает tar.gz потоком с `Content-Type: application/gzip` и Bearer-токеном проекта. Ответ `202` содержит `receipt_id` и `Location`; пока работает задание `ingest.package` в очереди `interactive`, квитанция возвращает `quarantined` и `retry_after_seconds`. Итог и построчные причины доступны через `GET /api/v1/receipts/{id}` и `/records` тем же токеном или вторым токеном этого проекта.

В процессе `api` задай `RUNNER_NETWORK` как список IPv4/IPv6 CIDR через запятую, а `TRUSTED_PROXIES` - адреса или CIDR доверенного nginx. Пустая область runner запрещает загрузку. Заголовки `X-Forwarded-For` и `X-Real-IP` учитываются только от доверенных прокси; запрос вне области получает `403` до чтения тела и создания объекта или квитанции. Пример для одного пограничного nginx:

```nginx
location /api/v1/ingest/packages {
    client_max_body_size 25m;
    proxy_pass http://api:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Real-IP $remote_addr;
}
```

`manifest.source` задаёт провайдера и ключ экземпляра. В `manifest.area` GitLab и GitHub указывают `repository` и его native ID, а Jenkins указывает `ci_project` и полное имя job. Область должна быть связана с проектом токена через `org.project_area`. Приём разрешает CI-записи, находки и файлы отчётов; записи другой области или вида отклоняются с причиной, остальные принимаются. Повтор пакета определяется внутри проекта, в том числе до окончания первого разбора; повторное задание не добавляет записи журнала.

Размер свыше 25 MiB даёт `413`, другой тип тела - `415`, невалидный конверт или CI-манифест - `422`. После отказа карантин удаляется. При недоступном хранилище ответ `503`, квитанция не создаётся. Аудит `uploadPackage` хранит ID токена и область проекта. Метрика `craftpulse_ingest_ci_packages_total{outcome,reason}` считает окончательные исходы и ошибки хранилища; значения токенов в журнал не попадают.

### GitHub Actions и Jenkins

Получить workflow или стадию Jenkins можно через `getCiConfiguration` с
`ci_system=github_actions` или `ci_system=jenkins`. Задай адрес сайта в
`CRAFTPULSE_URL`, а токен загрузки сохрани как секрет репозитория GitHub или
Secret text в Jenkins с ID `CRAFTPULSE_UPLOAD_TOKEN`. Если ключ экземпляра
адаптера отличается от host и порта CI, задай `CRAFTPULSE_INSTANCE_KEY`.
Workflow GitHub использует `github.token` с правом `actions: read` для
поиска числового ID job в текущей попытке run. Для своего workflow с
переименованным job передай его display name в `CRAFTPULSE_ACTIONS_JOB_NAME`
и токен Actions в `CRAFTPULSE_ACTIONS_TOKEN`.

В Jenkins стадия требует узел с Docker Pipeline и настроенным SCM. В своём
pipeline вызывай `craftpulse-ci jenkins record` после каждого выполнения шага
с логическим именем стадии и шага, номером повтора, исходом и временем начала
в RFC3339. Для matrix передавай оси через `--axes os=linux,arch=arm64`.
Укажи общий для сборки каталог `--attempts-dir` всем ветвям и команде
`jenkins finish`. После завершения ветвей архивируй полученный
`execution-manifest.json` и передай его в `upload --execution-manifest`.
Чтобы связать отчёты текущего сканирования с конкретным повтором, передай
ключ из вывода `jenkins record` в `upload --jenkins-attempt-id`.

Контейнерная проверка обеих сред на Linux с локальным тестовым HTTP-сервером:

```sh
docker build --target ci-runtime -f backend/ci.Dockerfile -t craftpulse-ci:local backend
cd backend
CRAFTPULSE_TEST_IMAGE=craftpulse-ci:local go test ./cmd/craftpulse-ci -run '^TestUpload_AdditionalCIProviders$'
```

## Кандидаты связей задачи и MR

После нормализации задач и MR модуль `normalize` ищет ключи вида `PA-7` в source branch и title. Поиск задач ограничен областями трекера проектов, которым назначен репозиторий через `org.project_area`. Методы `branch_key` и `title_key` записывают кандидатов с полем, найденным фрагментом и `ambiguous` в evidence. Совпадение ключа в нескольких экземплярах даёт отдельного кандидата на каждую задачу; автоматического подтверждения нет.

Задача, пришедшая позже MR, участвует в том же поиске. Исчезнувшее основание закрывается через `superseded_seq`; прежние срезы знания сохраняются. Отклонённый кандидат остаётся отклонённым, подтверждение пользователя сохраняется при изменении текста MR. Связь `mr_run` с методом `source_explicit` создаётся только для pipeline, чей `mr_id` нормализация проверила по экземпляру, репозиторию и известному head SHA MR. Если исправленная версия запуска больше не подтверждает связь, она закрывается с сохранением прежнего среза.

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

## Профиль секретов Gitleaks

`gitleaks-default` использует встроенный Gitleaks 8.30.1. Его версия закреплена в
`backend/go.mod`; дополнительные правила и минимальная длина удаляемого фрагмента
хранятся в `backend/pkg/secrets/profiles/gitleaks-default/`. `rules_hash` покрывает
версию стандартных правил Gitleaks, локальные правила и политику очистки.
Правило `craftpulse-demo-secret` распознаёт нерабочий учебный маркер demo-dataset.

Образ строится поверх базового CLI отдельной целью того же Dockerfile:

```sh
docker build --platform linux/amd64 --target gitleaks-default -f backend/ci.Dockerfile -t craftpulse-secrets:local backend
python3 scripts/check_secrets_profile.py --image craftpulse-secrets:local
```

Выпуск включает `craftpulse-ci-gitleaks-default`; версия инструмента и хэш правил
попадают в release manifest. Проверка образа без сети создаёт настоящие Git-репозитории
для head, diff, чистого файла, shallow clone и удалённого binary-секрета, импортирует JUnit с копией маркера,
распаковывает результат и проверяет отсутствие всех его фрагментов от восьми байт.

В job профиля запускай `craftpulse-ci secrets`, затем при необходимости `import`
и `upload`. Образ задаёт `CRAFTPULSE_SECRETS=1`. Перед каждой командой он повторно
сканирует репозиторий `CI_PROJECT_DIR` (по умолчанию текущий каталог), поскольку
найденные значения и таблица очистки существуют только в памяти процесса.
`secrets-info` печатает версию и `rules_hash` без обращения к репозиторию или API.
При нескольких строках профиля задай `CRAFTPULSE_COMPONENT_ID`.

Head читается из Git-дерева `CI_COMMIT_SHA`, независимо от изменений рабочей копии.
Diff проверяет добавленные строки каждого доступного коммита в
`CI_MERGE_REQUEST_DIFF_BASE_SHA..CI_COMMIT_SHA`; для branch pipeline база берётся из
`CI_COMMIT_BEFORE_SHA`. Удалённый в следующем коммите секрет остаётся в diff-находках.
В `scan-manifest.json` head и diff представлены отдельными запусками с диапазоном
`history`. При отсутствующей базе проверяется доступная история до head, `from_sha`
равен самому старому проверенному коммиту. Любая shallow-копия или неизвестная база
даёт `history.truncated=true` и `completeness=partial`; загрузка истории не выполняется.
Отсутствующий head, submodule, превышение лимита 32 MiB на один ответ Git,
256 MiB на прочитанные блобы или 15 минут останавливают команду без отправки пакета.

Сырые находки и содержимое Git-блобов не записываются на диск. В JSON-отчёте остаются
только `RuleID`, `File`, `StartLine`, `Commit` и `Fingerprint`. Отпечаток вычисляется
из правила, пути и SHA-256 значения, затем хэшируется ещё раз; строки и коммит
в него не входят, поэтому соседняя правка не меняет идентичность находки.

Перед сохранением импортированного отчёта и упаковкой удаляются полные значения
и **все восьмибайтовые окна** каждого найденного значения. Более короткое значение
удаляется целиком. Это распространяется на остальные отчёты, manifest и provenance;
JSON/XML разбираются перед очисткой, включая экранированные строки и атрибуты;
namespace-префиксы XML сохраняются.
В пакете используется короткая замена `[!]`. После сборки проверяется каждый
распакованный файл и его путь; оставшийся фрагмент запрещает отправку. Исходные
отчёты сторонних инструментов остаются ответственностью job, их очищенные копии
хранятся в состоянии CLI. Произвольные stdout/stderr инструментов в secrets-образе
подавляются, CLI печатает очищенные итоги. Вывод до запуска `craftpulse-ci` этим
механизмом не защищён. Изменение SHA, диапазона или профиля требует нового каталога состояния.
Повторная загрузка из базового образа состояния, содержащего
запуски `gitleaks-default`, запрещена без свежего сканирования.

Для локальной проверки без отправки используй
`craftpulse-ci upload --dry-run --save package.tar.gz`. Профиль проекта должен быть
заранее получен командой `profile` или сохранён в состоянии job. Это тот же пакет
и те же проверки, что при обычной загрузке; HTTP upload и ожидание receipt пропускаются.

## Профили кода Go, Python и JS/TS

Профиль кода проверяет компоненты одного языка тремя инструментами: линтер языка даёт
lint-находки в SARIF 2.1.0, Lizard 1.26.0 - функции с CC и NLOC в CSV, jscpd 5.4.2 - пары копий
в JSON. Профили лежат в `backend/internal/ci/profiles/<профиль>/`:

| Профиль | Язык | Линтер и конфигурация |
|---|---|---|
| `go-default` | `go` | golangci-lint 2.15.0, `golangci.yml`: errcheck, govet, ineffassign, staticcheck, unused |
| `python-default` | `python` | Ruff 0.17.4, `ruff.toml`: F401, F632, F811, F821, F841, B006, B011, B015, B023 |
| `js-default` | `javascript`, включая TypeScript | ESLint 10.14.0 с typescript-eslint 8.72.2 и TypeScript 6.0.3, `eslint.config.mjs` |

Набор ESLint для `.js`, `.jsx`, `.mjs` и `.cjs`: no-undef, no-unused-vars, no-unreachable,
no-dupe-keys, no-self-assign, use-isnan, valid-typeof, no-cond-assign. Для `.ts`, `.tsx`, `.mts`
и `.cts` те же правила ядра без no-undef и no-unused-vars, а также `@typescript-eslint/`
no-unused-vars, no-duplicate-enum-values, no-misused-new, no-non-null-asserted-optional-chain
и no-unsafe-declaration-merging. Все правила работают без информации о типах, поэтому tsconfig
и зависимости проекта не нужны. Версии, лимит времени 5 минут и пороги C05 и C06 (CC выше 10,
NLOC выше 50) записаны в `profile.json` профиля. `jscpd.json` задаёт минимальную копию
10 строк и 100 токенов и форматы языка. `rules_hash` - SHA-256 `profile.json`, конфигурации
линтера и `jscpd.json` подряд, `config_hash` запуска - SHA-256 конфигурации его инструмента.
У запуска ESLint в `parser_version` записаны версии typescript-eslint и TypeScript.

Образ строится целью с именем профиля поверх базового CLI:

```sh
docker build --platform linux/amd64 --target python-default -f backend/ci.Dockerfile -t craftpulse-python-default:local backend
python3 scripts/check_code_profile.py --profile python-default --image craftpulse-python-default:local
```

golangci-lint собирается из исходников закреплённой версии. Lizard и Ruff ставятся по хэшам из
requirements профиля, бинарник jscpd скачивается по SHA-256. ESLint и плагины ставятся `npm ci`
по `package-lock.json` профиля без install-скриптов, Node.js берётся из Alpine. Выпуск сверяет
версии `profile.json` с Dockerfile, requirements и lock-файлом. Проверка образа идёт без сети:
версии инструментов, rule fixtures, запуск job на репозитории с каталогами зависимостей и
сборки, сгенерированным файлом, копией кода и файлом с синтаксической ошибкой, запуск с лимитом
1 мс. Для Python и JS/TS проверка ещё собирает из
`backend/internal/ci/testdata/code-projects/<язык>/` репозиторий из двух коммитов, base и head,
прогоняет профиль на каждом и сверяет SHA в scan manifest и находки с `expected.json`.

Job профиля выполняет `craftpulse-ci code`, затем `craftpulse-ci upload`. Команда берёт
строки `code` профиля образа из профиля проекта и на каждый компонент записывает три запуска
с общим лимитом времени, например `python-default-ruff`, `python-default-lizard` и
`python-default-jscpd`. Репозиторий берётся из `CI_PROJECT_DIR`, профиль - из
`CRAFTPULSE_CODE_PROFILE` образа; `--timeout` заменяет лимит профиля, `--component`
выбирает один компонент. Инструмент, не уложившийся в лимит, даёт `timeout` и `partial`,
следующие инструменты после истечения лимита не запускаются. Все пути в отчётах
указаны от корня репозитория.

Линтеры получают только конфигурацию профиля. Ruff не читает `pyproject.toml` и `ruff.toml`
репозитория, не пишет кэш в checkout и ничего не исправляет. ESLint не ищет
`eslint.config.*` репозитория и загружает только плагины образа, поэтому код и плагины клиента
не исполняются. Отметки подавления в коде (`# noqa`, `eslint-disable`, `//nolint`) действуют.
Файл, который линтер не разобрал, перечисляется в `skipped_targets` запуска линтера с
причиной `unsupported`, его находки не передаются, а запуск получает `partial`. Lizard и
jscpd разбирают такой файл как обычно. Из вывода Ruff и ESLint в SARIF переносятся правило,
уровень, сообщение и координаты; исправления, подсказки и исходный текст файла не попадают в отчёт.

Область анализа задаётся языком. Каталоги на `.` не анализируются и не перечисляются.
Остальные исключения попадают в `excluded_targets` с причиной: исключение задаёт границу
области и не делает запуск неполным. Тестовый код анализируется и перечислен в `test_targets`,
чтобы показатели production-области считались без него.

| Язык | Исключается | Тестовый код |
|---|---|---|
| Go | `vendor`, `build`, файлы с отметкой `// Code generated ... DO NOT EDIT.`; каталоги на `_` не анализируются | `_test.go`, каталоги `testdata` |
| Python | виртуальные окружения (каталог с `pyvenv.cfg`), `site-packages`, `__pypackages__`, `node_modules`, `build`, `dist`, сгенерированные файлы; `__pycache__` не анализируется | `test_*.py`, `*_test.py`, `conftest.py`, каталоги `tests` и `test` |
| JS/TS | `node_modules`, `bower_components`, `build`, `dist`, `coverage`, `*.min.js`, сгенерированные файлы | `*.test.*`, `*.spec.*`, каталоги `__tests__`, `__mocks__`, `tests` и `test` |

Сгенерированный файл Python и JS/TS отмечен в начальных комментариях словами `DO NOT EDIT`
или `@generated`. golangci-lint запускается в каждом Go-модуле компонента, а для компонента
внутри модуля - в ближайшем модуле выше с шаблоном пакетов компонента. Ruff и ESLint получают
список файлов области от корня репозитория. Отчёт jscpd сохраняет файлы, строки и токены копий
без фрагмента кода; копии ищутся внутри одного формата, `.js` и `.ts` между собой не сравниваются.

Анализу Go не нужна сеть, если зависимости модуля есть в checkout или кэше модулей. Иначе
golangci-lint не загрузит пакеты, и запуск получит `failed`. `GOTOOLCHAIN=local`:
модуль, которому нужна версия Go новее 1.27.4, тоже даёт `failed`. Ruff и ESLint
зависимости проекта не загружают.

Rule fixtures профиля лежат в `fixtures/` его каталога: на каждое правило линтера проблемный и
безопасный пример, функция выше порога сложности, длинная функция, копия, область анализа и
для Python и JS/TS файл с синтаксической ошибкой. В `expected.txt` кроме находок
`<rule> <path>:<line>` перечислены функции `function <path>:<line> <name> ccn=<n> nloc=<n>`,
копии `clone <path>:<start>-<end> <path>:<start>-<end>`, тестовые файлы `test <path>`,
исключения `excluded <path> <reason>` и пропущенные файлы `skipped <path> <reason>`.
Записанный отчёт сравнивается со списком целиком:

```sh
craftpulse-ci fixtures /opt/craftpulse/profiles/python-default/fixtures -- craftpulse-ci code --fixture
```

## Профиль SAST Opengrep

`opengrep-default` проверяет Go, Python и JS/TS Opengrep 1.33.1 по набору из девяти
правил в `backend/internal/ci/profiles/opengrep-default/rules/`. На каждый язык три правила:

- Go: SQL из `fmt.Sprintf` или конкатенации, команда через `sh -c`, `InsecureSkipVerify`;
- Python: `subprocess` с `shell=True`, `yaml.load` без безопасного загрузчика, `eval`;
- JS/TS: `eval` и `new Function`, `exec`/`execSync` из `child_process`, присваивание в `innerHTML`.

Правила написаны в проекте. В `sources.json` для каждого правила записаны файл, источник, версия
источника и лицензия. Профиль не загружается, если у правила нет записи, запись ссылается на
отсутствующее правило или лицензия не из списка распространяемых: `LicenseRef-craftPulse`, MIT,
Apache-2.0, BSD-2-Clause, BSD-3-Clause. Правила semgrep-rules и opengrep-rules
не используются: их лицензия не SPDX и ограничивает распространение. `rules_hash` - SHA-256
`profile.json`, `sources.json` и файлов правил по имени. Выпуск переносит перечень источников
в поле `rule_sources` release manifest.

```sh
docker build --platform linux/amd64 --target opengrep-default -f backend/ci.Dockerfile -t craftpulse-sast:local backend
python3 scripts/check_sast_profile.py --image craftpulse-sast:local
```

Job профиля выполняет `craftpulse-ci sast`, затем `craftpulse-ci upload`. Команда запускает
Opengrep на каждом компоненте строк `sast` профиля `opengrep-default` из корня `CI_PROJECT_DIR` и
записывает один запуск на компонент с SARIF 2.1.0. Лимит профиля 5 минут на все компоненты, лимит
одного файла 30 с. В отчёте остаются ID правил без префикса пути, CWE в `tags`, `security-severity`
и уровень правила. Строки кода (`snippet`) удаляются.

Встроенный список исключений Opengrep отключён: он молча пропускает и `tests/`. Вместо него
каталоги `vendor`, `node_modules`, `build`, `dist` и каталоги на `.` исключаются явно, первые
четыре попадают в `excluded_targets`. Файл, который Opengrep разобрал не полностью или не уложился
в лимит, перечисляется в `skipped_targets` с причиной `unsupported` или `timeout`. Запуск тогда
получает `partial` и сохраняет отчёт. Ограничения размера файла нет, чтобы файлы не пропускались
без отметки. Бинарник Opengrep распаковывается во временный каталог и запускается оттуда,
поэтому `/tmp` job не должен быть смонтирован с `noexec`.

Rule fixtures лежат в `fixtures/<язык>/` профиля: для каждого правила проблемный и безопасный
пример в одном файле.

```sh
craftpulse-ci fixtures /opt/craftpulse/profiles/opengrep-default/fixtures -- craftpulse-ci sast --fixture
```

## Профиль SCA и IaC Trivy

`trivy-default` проверяет зависимости и конфигурацию Trivy 0.76.2 без сети. Job профиля выполняет
`craftpulse-ci trivy`, затем `craftpulse-ci upload`. Команда берёт строки `sca` и `iac` профиля
`trivy-default` и на каждую записывает запуск с нативным JSON Trivy (`trivy_json`, версия схемы 2):
`trivy-default-sca` и `trivy-default-iac`. Лимит профиля 5 минут на все строки.

SCA читает lockfile Go (`go.mod`), Python (`requirements.txt`, `poetry.lock`, `Pipfile.lock`, `uv.lock`)
и JS/TS (`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`). Отчёт сохраняет пакет, установленную и
исправленную версии, путь lockfile, advisory и его aliases (`VendorIDs`) и полный список пакетов.
Запуск SCA пишет в scan manifest `db_version` - версию схемы базы и SHA-256 её содержимого, например
`2@sha256:5389...`, и `db_published_at` - время публикации снимка из `metadata.json` Trivy. Без базы
или с несогласованной базой запуск SCA получает `failed` без отчёта, а не пустой результат.

Новые зависимости изменения (S16) отмечаются на runner. Base берётся из
`CI_MERGE_REQUEST_DIFF_BASE_SHA` или `CI_COMMIT_BEFORE_SHA`: шаг достаёт lockfile отчёта из base-коммита
через `git show` и сканирует их той же базой. Пакет и его уязвимость получают `"CraftPulseDelta":
"added"`, если в lockfile base такого пакета не было, и `"changed"`, если была другая версия. Поле
`CraftPulse` в корне отчёта называет `BaseSHA` и состояние сравнения: `computed`, `no_base` без
base-коммита или `unavailable`, если base нет в checkout. В последнем случае отчёт SCA остаётся полным,
отметок нет.

IaC проверяет Dockerfile и манифесты Kubernetes встроенными проверками Trivy этой версии. Виды
конфигураций записаны в `iac_types` профиля. Terraform не включён: его проверки в Trivy написаны для
AWS, Azure и GCP. Helm без рендеринга шаблонов не проверяется. `trivyignore` профиля исключает
KSV-0125: эта проверка доверяет только реестрам публичных облаков и отмечает каждый образ из
внутреннего реестра. Строки конфигурации (`CauseMetadata.Code`) удаляются из отчёта. Каждая находка
получает `CauseMetadata.Resource`: Trivy называет ресурс Terraform сам, для Kubernetes записывается
`<kind>/<name>` объекта, для Dockerfile - `stage <имя или образ>`, для находки о файле целиком - `file`.

Trivy получает `--config` и `--ignorefile` профиля, поэтому `trivy.yaml` и `.trivyignore`
репозитория не действуют. Обновление базы и проверок отключено. Каталоги `vendor`, `node_modules`,
`build`, `dist` и каталоги на `.` исключаются и, кроме последних, попадают в `excluded_targets`.
`rules_hash` - SHA-256 `profile.json`, `trivy.yaml` и `trivyignore`, `config_hash` - SHA-256 `profile.json`.

```sh
docker build --platform linux/amd64 --target trivy-default -f backend/ci.Dockerfile -t craftpulse-trivy:local backend
python3 scripts/check_trivy_profile.py --image craftpulse-trivy:local
```

Бинарники Trivy для amd64 и arm64 скачиваются по SHA-256. Стадия `trivy-db` загружает снимок базы
командой `craftpulse-ci trivy-db` из `ARG TRIVY_DB_REPOSITORY` -
`ghcr.io/aquasecurity/trivy-db`, закреплённый digest. База без сжатия занимает около 1,7 ГБ, поэтому
образ весит около 2 ГБ. Каталог базы `/opt/craftpulse/trivy` принадлежит пользователю job.
Базу можно заменить из внутреннего источника:

```sh
craftpulse-ci trivy-db --repository registry.example.ru/mirror/trivy-db:2
```

Источник задаётся `--repository` или `CRAFTPULSE_TRIVY_DB_REPOSITORY`, каталог -
`--cache-dir` или `CRAFTPULSE_TRIVY_CACHE`. Новая база скачивается во временный каталог рядом с
текущей, проверяется и только потом заменяет её. Неудачная загрузка оставляет прежнюю базу и её дату и
завершает команду с ошибкой.

Rule fixtures лежат в `fixtures/` профиля: lockfile с ожидаемым advisory для Go, Python и JS/TS,
lockfile без уязвимостей, проблемная и безопасная конфигурация Dockerfile и Kubernetes. Профиль без
SARIF записывает факты в файл `CRAFTPULSE_FACTS_REPORT`, и `expected.txt` сравнивается с ними целиком:
`vulnerability <advisory> <пакет>@<версия> <lockfile>` и
`misconfiguration <проверка> <путь>:<строка> <ресурс>`.

```sh
craftpulse-ci fixtures /opt/craftpulse/profiles/trivy-default/fixtures -- craftpulse-ci trivy --fixture
```

## Профиль Git-документации

`git-markdown-openapi` проверяет корни документации проекта. Job профиля выполняет
`craftpulse-ci docs`, затем `craftpulse-ci upload`. Корни берутся из `documentation_roots` профиля
проекта: на каждый корень вида `docs` или `openapi`, для которого у профиля есть строка матрицы,
пишется запуск `<вид>-<root_id>` с двумя отчётами - SARIF 2.1.0 с находками и `docs_json` с
инвентарём. Компонент запуска - путь корня. Лимит профиля 5 минут на все корни.

Вид `docs` разбирает Markdown (`.md`, `.markdown`) библиотекой goldmark 1.8.7 внутри
`craftpulse-ci`, запуск называет её в `parser_version`. Локальная ссылка или картинка, файл
которой не существует с точно таким именем, даёт находку `missing-file` на строке ссылки. Ссылка,
отличающаяся от файла только регистром, тоже битая. Находку `missing-file` даёт и путь за пределы
репозитория. Якорь, которого нет среди заголовков Markdown-цели, даёт `missing-anchor`. Якоря
строятся по правилу GitHub и GitLab (`anchor_rule: github`): строчные буквы, знаки препинания
удаляются, пробелы становятся дефисами, повтор заголовка получает `-1`, `-2`. Путь от `/`
считается от корня репозитория. Внешние ссылки не проверяются и только считаются.

Инвентарь `docs_json` перечисляет каждый Markdown и OpenAPI-документ корня записью `DocumentV1`:
путь, `version_ref` - SHA коммита job, ID проекта, формат, заголовок (первый `#` или `info.title`)
и SHA-256 содержимого. Владелец берётся из front matter `owner:` (`owner_front_matter_key`
профиля), у документа без него `owner` пуст. В инвентаре также записаны счётчики проверенных,
битых и внешних ссылок, в scan manifest - число проверенных и пропущенных документов. Каталоги
на `.` не входят в корень, файл больше 5 МиБ пропускается с причиной `limit`. OpenAPI-документ
распознаётся по ключу `openapi:` или `swagger:` в YAML или JSON.

Вид `openapi` проверяет спецификации корня Spectral 6.17.1 с `spectral.yaml` профиля
(`extends: ["spectral:oas"]`). Результат с кодом из `spec_errors` (`parser`,
`unrecognized-format`, `oas2-schema`, `oas3-schema`) делает спецификацию невалидной:
`spec_valid: false` в `docs_json`. Остальные результаты - нарушения правил валидной спецификации.
Все результаты попадают в SARIF с кодом Spectral. `rules_hash` - SHA-256 `profile.json` и
`spectral.yaml`, `config_hash` - SHA-256 `profile.json` для `docs` и `spectral.yaml` для
`openapi`.

```sh
docker build --platform linux/amd64 --target git-markdown-openapi -f backend/ci.Dockerfile -t craftpulse-docs:local backend
python3 scripts/check_docs_profile.py --image craftpulse-docs:local
```

Spectral ставится `npm ci` по `package-lock.json` профиля без install-скриптов, Node.js берётся из
Alpine. Выпуск сверяет версию Spectral с lock-файлом, а goldmark - с `backend/go.mod`. Проверка
образа генерирует набор показа в контейнере Go, клонирует `demo/orders-service` и прогоняет
профиль на его корне `docs` из 25 документов. Инвентарь сверяется с файлами, их SHA-256 и
владельцами, находки - с двумя битыми ссылками и невалидной спецификацией набора. Затем проверка
исправляет якорь в копии и убеждается, что его находка исчезла.

Rule fixtures лежат в `fixtures/` профиля. В `expected.txt` кроме находок `<rule> <path>:<line>`
перечислены факты инвентаря: `document <path> <формат> owner=<владелец или ->` и
`spec <path> valid|invalid`.

```sh
craftpulse-ci fixtures /opt/craftpulse/profiles/git-markdown-openapi/fixtures -- craftpulse-ci docs --fixture
```

## Очистка секретов при приёме

Сервер повторно проверяет пакет правилами Gitleaks 8.30.1 из
`backend/pkg/secrets/profiles/gitleaks-default/`. Этот же пакет используют CLI,
`api` и `worker`. Учебный marker распознаётся и без имени переменной: его значение
вычисляется в памяти по той же формуле, что в demo-dataset.

Каналы `ci`, `import` и `sync` проходят через общий `ingest`: сначала проверяются
лимиты и структура исходного пакета, затем детектор просматривает текст и
декодированные строки JSON/XML. После обнаружения значений удаляются полные значения
и все их восьмибайтовые фрагменты во всём пакете, включая другие отчёты, `evidence`,
описания, манифесты и метаданные отклонённых записей. Имя файла, переданное клиентом,
не позволяет отключить обнаружение через allowlist Gitleaks. Правило и безопасная
location находки сохраняются. Ошибка безопасной очистки отклоняет пакет.

Постоянный объект записывается после очистки; ссылки в записях, scan manifest и
execution manifest содержат новые размеры и SHA-256 файлов. `ingest.receipt.package_hash`
вычисляется из очищенного пакета по прежнему правилу `envelope.Clean`: время создания
манифеста не входит в идентичность содержимого. Повторная доставка даёт тот же хэш.
Исходный объект остаётся только в карантине и удаляется после коммита журнала.

В `ingest.receipt` сохраняются `redaction_count` и `redaction_rules_hash`.
Счётчик `craftpulse_ingest_secret_replacements_total{channel}` показывает число замен
в попытках записи пакета в журнал; повторное получение уже принятого пакета не
увеличивает его. Метрика не содержит значений, путей или идентификаторов секретов.
При повторе транзакции счётчик попыток может увеличиться повторно; точное число для
пакета хранится в квитанции.
