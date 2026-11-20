# Поставка OCI

Workflow `OCI release` публикует сборку точного SHA после успешного прогона `Repository`, вызванного push в `main` репозитория `Avtobot22/craftPulse`. Он запускается событием `workflow_run`, проверяет исходный репозиторий, ветку, событие и исход проверок и делает checkout `head_sha`. PR и ручной прогон `Repository` ничего не публикуют. Параллельные выпуски одного SHA сериализованы; более новый коммит не отменяет предыдущий выпуск.

## Состав и права

Реестр поставки - GitHub Container Registry, пространство `ghcr.io/avtobot22`. Job `publish` получает `packages: write` через встроенный `GITHUB_TOKEN`; остальные workflows не получают это право или отдельный секрет реестра. Checkout не сохраняет credentials, токен не передаётся процессам сборки. Вход в реестр происходит через стандартный ввод Docker и удаляется в конце job. Actions закреплены полными SHA, бинарники Trivy, Syft и ORAS - версиями и SHA-256 архивов в `.github/release-tools.json`.

Сборка выпускает Linux/amd64 образы `craftpulse-server`, `craftpulse-frontend`, `craftpulse-ci` и `craftpulse-adapter-<имя>`. Сервер содержит `api` и `worker`. Frontend содержит статические файлы и nginx. `craftpulse-ci` - базовый образ передачи отчётов из CI клиента: бинарник `craftpulse-ci` на Alpine с CA и shell, потому что GitLab выполняет в образе job скрипт. Образ `craftpulse-ci-gitleaks-default` строится поверх него целью `gitleaks-default` того же Dockerfile: добавляет Git и включает встроенный Gitleaks 8.30.1. Образ `craftpulse-ci-go-default` - профиль кода Go, цель `go-default`: Go 1.27.4, golangci-lint 2.15.0, Lizard 1.25.0 и jscpd 5.4.2. Процесс работает от `nonroot` с UID/GID 65532, команда входа не задана. Список адаптеров вычисляется по `backend/cmd/adapter-*/main.go`; сейчас выпускается `adapter-git`. `adapter-test` исключён: его команда предназначена только для записанных тестовых ответов и локального стенда. Новый адаптер с теми же зависимостями runtime включается автоматически; для новых системных зависимостей нужно дополнить `backend/adapter.Dockerfile`. Runtime адаптера содержит Git, SSH-клиент и CA; процесс работает от системной учётной записи `nonroot` с UID/GID 65532. Переменная `GIT_CACHE_DIR=/var/cache/craftpulse-git` совпадает с конфигурацией по умолчанию; каталог принадлежит этому пользователю. Адаптер принимает TLS на порту 8443.

Каждый образ имеет тег из полного SHA коммита и OCI-метку revision; тег `latest` не используется. Базовые образы закреплены digest. При сборке frontend и адаптеров команда `apk upgrade --no-cache` обновляет предустановленные системные пакеты из своей ветки Alpine. Это включает исправления безопасности, выпущенные после создания базового образа. Установленные APK-пакеты фиксируются содержимым выпущенного образа и SBOM; их версии разрешаются при первой сборке SHA.

До первой публикации workflow строит все образы и сканирует каждый Trivy. Любая уязвимость класса `HIGH` или `CRITICAL`, включая ещё не исправленные, останавливает выпуск. Ошибка сканера также останавливает выпуск. JSON-отчёты Trivy и Syft сохраняются в артефакте Actions `oci-release-<SHA>-<attempt>` даже после отказа. Все образы должны пройти проверку до первого push. После push образ скачивается по digest реестра, его ID сравнивается с ID сканированного образа. Сбой сети во время публикации может оставить часть образов без общего манифеста; такой набор ещё не считается выпуском.

Проверка `scripts/check_release_server.py` поднимает временный PostgreSQL 18, применяет миграции из образа, запускает API и сравнивает `/version` с SHA сборки и версией схемы. Она выполняется в PR после сборки серверного образа и в выпуске после публикации по digest из манифеста. Результат выпуска записывается в `server-version.json` рядом с манифестом и отчётами; ошибка останавливает workflow. Временные контейнеры и файлы тестовых секретов удаляются после проверки.

В `Repository` job `release-check` проверяет discovery, версии схем и лицензии npm, собирает обнаруженные адаптеры и образ `craftpulse-ci` и проверяет runtime без credentials реестра. В контейнере адаптера от UID/GID 65532 проверяются системная учётная запись, создание каталогов Git-кэша по умолчанию и разбор конфигурации SSH, в контейнере `craftpulse-ci` - UID/GID и запуск `craftpulse-ci version`; сеть отключена. Та же проверка выполняется перед публикацией. Результат `release-check` включён в обязательный гейт `ci`. Изменения backend и файлов выпуска включают эту проверку; тесты скрипта выпуска входят в `repository-check`.

## Манифест

`release-manifest.json` хранится как OCI-артефакт `ghcr.io/avtobot22/craftpulse-release:<SHA>` типа `application/vnd.craftpulse.release.v1+json`. Дополнительная копия находится в артефакте Actions с отчётами, который хранится 90 дней. Манифест в GHCR не зависит от срока хранения Actions. Как и образы, новые пакеты GHCR по умолчанию приватные; доступ предоставляется средствами Packages. Автоматизация не меняет их видимость.

Поля манифеста версии `1`:

| Поле | Содержимое |
| --- | --- |
| `commit_sha` | Полный SHA исходного коммита |
| `images` | Имя образа → полный адрес `ghcr.io/avtobot22/<имя>@sha256:<digest>` |
| `core_schema_version` | Максимальный номер миграции `backend/migrations/core` |
| `adapter_schema_versions` | Имя выпускаемого адаптера → максимальный номер общей схемы адаптеров |
| `adapter_contract_version` | `CurrentContractVersion` из SDK; `null`, если адаптеров нет |
| `ci_tool_versions`, `rule_hashes` | Версии Gitleaks, golangci-lint, Lizard и jscpd. SHA-256 версии стандартных правил, локальных правил и политики очистки `gitleaks-default`; для `go-default` - SHA-256 файлов `profile.json`, `golangci.yml` и `jscpd.json` подряд |
| `licenses.images` | Компоненты runtime и встроенных Go-бинарников каждого образа: имя, версия, тип, purl, список лицензий из Syft |
| `licenses.frontend_npm` | Компоненты npm из lock-файла frontend, включая build/dev зависимости, с версиями, лицензиями и признаком `dev` |

Syft ищет лицензии Go-модулей в upstream Go proxy. Неопределённая лицензия обозначается `NOASSERTION`, а не выдуманным значением; это поле требует проверки при оценке лицензионных обязательств. Исходные SBOM доступны в том же прогоне. Discovery включает `gitleaks-default`; версия в его policy проверяется против `go.mod`. `release-check` и выпуск запускают `check_secrets_profile.py` в собранном образе без сети: head, diff, clean, shallow и очистка JUnit/пакета/вывода. Discovery включает `go-default`, если есть `backend/internal/ci/profiles/go-default/profile.json`; версии из него сверяются с `ARG GOLANGCI_LINT_VERSION`, адресами архивов jscpd в `backend/ci.Dockerfile` и `lizard-requirements.txt`. `check_go_profile.py` проверяет образ без сети: версии инструментов, rule fixtures, запуск job с provenance, исключениями и тестовым кодом, отчёт копий без кода и лимит 1 с.

## Получение и повторный запуск

Оператор с правом чтения пакетов входит в GHCR через `docker login ghcr.io --password-stdin`. ORAS использует тот же Docker config. SHA берётся из успешного `Repository` или выпуска:

```sh
oras pull --output ./release ghcr.io/avtobot22/craftpulse-release:<SHA>
python3 -m json.tool ./release/release-manifest.json
docker pull ghcr.io/avtobot22/craftpulse-server@sha256:<digest-из-манифеста>
```

Один и тот же digest продвигается на стенд, в облако и в контур клиента. При выкладке конфигурация и секреты задаются отдельно; пересборка по средам не требуется.

Повтор выполняется через Actions → OCI release → выбранный прогон → Re-run all jobs. Уже существующие SHA-теги читаются из реестра и скачиваются по digest. Их содержимое повторно сканируется, но не пересобирается и не отправляется заново. Существующий общий манифест проверяется по SHA, составу, версиям и digest и копируется побайтно вместе с исходным списком лицензий. При частично завершённом выпуске строятся только отсутствующие образы. Если тег появился во время сборки, изменился или реестр недоступен, workflow останавливается; неопределённость не считается отсутствием тега. Новая база Trivy может заблокировать повторный запуск, сохранив опубликованный выпуск без изменений.

## Проверка первого выпуска

После merge проверьте успешный `Repository` для итогового SHA, затем `OCI release` для того же `head_sha`. В Packages должны появиться все обнаруженные образы с SHA-тегами и общий манифест. Ошибка `403` при push означает, что текущая политика аккаунта или пакета не дала встроенному токену право публикации; workflow не подменяет его персональным токеном и не меняет настройки аккаунта.

1. Скачайте манифест через ORAS и сохраните его SHA-256. Получите каждый образ по записанному digest; сравните его с digest SHA-тега через `oras resolve ghcr.io/avtobot22/<имя>:<SHA>`.
2. Проверьте успешный шаг проверки сервера и файл `server-version.json` в артефакте Actions. Поле `commit_sha` ответа `/version` должно совпасть с SHA, `schema_version` - с `core_schema_version` манифеста. Так проверяется значение, встроенное в бинарник через `COMMIT_SHA`, а не только метка контейнера.
3. Для повторной локальной проверки выполните `python3 scripts/check_release_server.py --manifest ./release/release-manifest.json --output ./release/server-version.json` из корня репозитория. Нужен Docker; скрипт сам создаёт временную БД и применяет миграции. Рабочий IdP, object store и production-база не требуются.
4. Повторите прогон и скачайте манифест в новый каталог. SHA-256 всего файла и все digest должны совпасть. Проверьте сохранённые отчёты Trivy для каждого образа и наличие компонентов Go/npm в `licenses`.
5. В конфигурации PR workflow убедитесь, что `packages: write` отсутствует; фактическую возможность публикации встроенным токеном подтверждает успешный push выпуска.

Использованные интерфейсы: [событие workflow_run](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#workflow_run), [GHCR и встроенный токен](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry), [Trivy image](https://trivy.dev/docs/latest/references/configuration/cli/trivy_image/), [Syft Go license lookup](https://github.com/anchore/syft/blob/v1.57.2/syft/pkg/cataloger/golang/config.go), [ORAS push](https://oras.land/docs/commands/oras_push/), [ORAS pull](https://oras.land/docs/commands/oras_pull/). Параметры и версии архивов проверены 17 ноября 2026 года.
