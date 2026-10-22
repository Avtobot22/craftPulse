# Коннекторы

Коннектор получает исходные факты в доступной области, сохраняет их историю и сообщает полноту. Штатный движок craftPulse применяет подтверждённый mapping, связывает сущности и считает метрики. Наличие API у сервиса не означает готовый адаптер, а чтение текущего состояния не означает поддержку исторических расчётов.

Этапы и ограничения взяты из [кода](metrics/code.md), [безопасности](metrics/security.md), [CI/CD](metrics/ci-cd.md), [активности](metrics/activity.md), [документации](metrics/documentation.md) и [задач](metrics/tasks.md). Ниже описан состав подключения для продуктовых сценариев.

## Минимальный сквозной пилот M0

| Подключение | Что читаем | Какой результат открывает | Граница |
|---|---|---|---|
| Jira Server/Data Center | Issues, справочники статусов и исходов, schema полей, история переходов/полей, links в проверенной версии | T01-T05, T07-T15, T18, T23, T28, T38-T42 в пределах данных | Cloud - другой адаптер. `expand=changelog` проверяется на полноту; исходы и поля сопоставляются по instance-scoped IDs |
| GitLab self-managed CE/EE | Git, metadata/default branch, MR, head/base SHA, human notes/review и связи | A01-A07/A10/A18/A20/A22 и контекст C-метрик при необходимых отчётах | Clone сам по себе не читает MR; note не всегда review. Публикация требует отдельного write-доступа |
| GitLab CI | Pipelines, jobs со всеми attempts, статусы/allow_failure, timestamps, artifacts и reports | I01-I13, тесты и coverage при отчётах, сводка MR | Пагинация всех jobs, parent/child и matrix не удваиваются; полный expected graph не угадывается |
| Универсальный импорт | Задачи и события истории, явные relations, findings, JUnit, coverage и manifests | Поддержанные метрики без нового native API-коннектора | CSV текущих задач даёт snapshot; прошлый WIP из него не восстанавливается |
| Git-документация, опция | Выбранные Markdown roots, owners, локальные links/anchors, OpenAPI | Тонкий D-профиль M0 | Scope и аудитория задаются явно; произвольный проектный build не запускается ради чтения текста |

Существующие Git/PR, CI и AppSec-сборщики SourceCraft сохраняются в техническом M0. Расширение их возможностей проходит отдельную проверку API и версии. Сохранённая поддержка прототипа не приравнивается к полной поддержке новых flow-метрик SourceCraft Issues.

Для начального flow-отчёта не требуется внедрять все сканеры и все технические M0-профили. Их подключение образует отдельный законченный сценарий MR и анализа. Порядок - в [видении](vision.md#ход-разработки).

## Git и взаимодействия

| Источник | Этап | Что добавляет | Существенное условие |
|---|---|---|---|
| Generic Git SSH/HTTPS | M0 | Дерево, refs, diff и история для структуры и касаний | MR/review API и публикация отдельно; можно использовать CI-ссылку на результат |
| SourceCraft | Сохранение существующего M0 | Git, PR и факты имеющихся адаптеров | Delta, публикация и расширенная CI-история включаются после contract test |
| GitHub / Enterprise Server | M1 | PR, reviews, checks и workflow links в общей модели | Проверяются версия GHES и доступ App; права на private security data отдельны |
| GitVerse | M1 | PR, файлы, коммиты, review и CI по подтверждённому контракту | Сходство API с GitHub не означает совместимость семантики |
| GitFlic | M1 после GitVerse либо по пилоту | Git, MR, обсуждения и CI | Test/report capabilities проверяются отдельно |
| Bitbucket DC / Cloud | M2 | Дерево и PR соответствующей редакции | DC и Cloud - разные контракты; CI может находиться в Jenkins |

Для очереди ревью важны ready/draft, actor-kind, назначенный reviewer и тип human-response. Если источник даёт только комментарии, показатель называется первым внешним ответом. Авторы и боты не превращаются в reviewer по факту комментария.

## Трекеры задач

| Источник | Этап | Что требуется для метрик | Ограничение |
|---|---|---|---|
| Redmine | Первый следующий адаптер M1 | Открытые и закрытые issues, journals, relations, custom fields и версии | Default list только открытых недостаточен для backfill; sprint plugin и estimates mapping отдельны |
| Яндекс Трекер | M1 | Changelog с пагинацией, очереди, типы/статусы, поля/оценки и links | История и workflow не копируются из Jira; sprint capabilities проверяются |
| Jira Cloud | M1, отдельный адаптер | Issues/changelog и Agile metadata на своей версии API | DC endpoints и модель пагинации не переносятся автоматически |
| GitLab Issues / GitHub Issues | M1 для базового flow | Состояния, timeline, поля и связи | Label не задаёт универсальный workflow; GitHub PR не удваивает issue-throughput |
| SourceCraft Issues | M1, часть snapshot-фактов есть | Проверенные status/timestamps/labels/comments; история добавляется отдельно | Текущий status не даёт cycle time и старый WIP |
| YouTrack / Kaiten | M2 | Подтверждённые activities/перемещения, поля и история | Схемы workflow, categories и пагинация проверяются по версии |
| Другие трекеры | По отдельному discovery | Сначала generic import с нужными событиями | Адаптер включается после проверки необходимых capabilities |

Для каждой системы отдельно отмечаются `current_state`, `status_history`, `resolution_history`, `field_history`, `sprint_membership_history`, `sprint_metadata`, `issue_links`, `permissions` и `deletions`. Метрики включаются по этим возможностям.

Один проект может объединять несколько трекеров. Source IDs сохраняются; mirrors подтверждаются явно с источником истины по полям. Конфликты статуса и размера показываются пользователю. Первая версия читает данные; автоматическая двусторонняя синхронизация в неё не входит.

## Код и безопасность: сервисы и отчёты

| Источник | Подключение и этап | Что сохраняем | Граница |
|---|---|---|---|
| SonarQube Server / Community Build | Web API, первый direct quality connector технического M0; Cloud отдельной конфигурацией | Analysis/revision, measures, issues/rules, native severity, gates, scope | Версия/edition определяют branch и PR capabilities; SARIF не предполагается |
| Линтеры | Поддержанные родные JSON, GitLab Code Quality, Checkstyle XML; общий SARIF при реальном экспорте | Правило, location, message, severity, suppression, execution metadata | Парсер формата не доказывает поддержку всех правил и языков producer |
| PVS-Studio / Qodana | M0 через поддержанный SARIF, у PVS через converter | Rule IDs, paths, severity и имеющееся baseline-evidence | Движок клиента и его лицензия не становятся нашей штатной поставкой |
| SourceCraft AppSec | Существующий отдельный API-адаптер M0 | Scan ID/SHA/status, engine, группы findings и доступные поля | Enabled engine не доказывает полный file/package coverage |
| GitLab security reports | M0 ingest JSON artifacts | Findings, scanner/rules, identifiers, scope, commit и scan status | Security JSON - отдельная schema, не SARIF; платный vulnerability API для этого импорта не обязателен |
| Solar appScreener | M0 SARIF, M1 API по пилоту | Findings и статусы в доступной редакции | Набор exports/endpoints проверяется на конкретном инстансе |
| CodeScoring SCA | M1: SBOM/Johnny output, API по контракту | Компоненты, resolved versions, vulnerabilities/policies при наличии | SBOM без vulnerabilities - inventory. SCA API не доказывает наличие TQI-метрик |
| CodeScoring TQI / CodeScene | M2 API или export после проверки | Детальные native данные и их происхождение | Первый выпуск не строится на предположении о недокументированном API |
| Внутренний анализатор | M0 generic finding envelope или поддержанный отчёт | Evidence, rule, scope, version, execution, triage | Пустой отчёт без complete manifest не означает отсутствие проблем |

Внешние исключения сохраняют источник истины. craftPulse не переписывает triage во внешней системе как побочный эффект импорта. Severity отображается вместе с исходной шкалой и версией преобразования.

## CI, тесты и доставка

| Данные | Путь получения | Какие правила чтения |
|---|---|---|
| Попытки CI | GitLab M0; GitHub Actions/Jenkins и другие CI по M1-пилоту | Сохраняются run/check/attempt IDs, stage/matrix/config, native statuses и timestamps |
| JUnit | Artifact upload или API в имеющейся области доступа | Cases и executions раздельны; известные configs, suite-version и attempts нужны для сравнения |
| Покрытие | Cobertura, LCOV, JaCoCo в поддержанном профиле | Lines/branches и source mapping сохраняются; JUnit не заменяет coverage |
| Артефакты | Manifest build с source SHA и immutable digest | Имя файла и tag не гарантируют идентичность сборки |
| Deployments | Отдельный CD-контракт M1 | Service, environment, digest/source, outcome и время; production определяется явно |
| Incidents/recovery | Подтверждённые связи для расширенных метрик M2 | Pre-prod failed job не является production change failure |

Загрузка отчёта из CI работает и без native CI API-коннектора. Она даёт ровно те факты, которые переданы, с их полнотой. История runner queue, parent/child и внутренних retries не появляется автоматически из одного summary-артефакта.

## Документация

| Источник | Этап | Что открывает | Ограничение |
|---|---|---|---|
| Выбранные Git roots | M0 | Inventory, owner, локальные ссылки, API-spec validation и task/doc relations | На старте Markdown; остальные parser profiles подключаются отдельно |
| Confluence DC / Cloud | M1, два адаптера | Pages, body, versions, labels, структура и доступные ограничения | Разные API и pagination. Один restrictions response не доказывает effective ACL всех пользователей |
| Яндекс Wiki | M1 кандидат по пилоту | Страницы и дерево в разрешённой области | API может быть выключен; UI-history не гарантирует API-архив версий |
| Backstage / TechDocs | M1 импорт каталога, M2 native adapter | Service refs, owner/lifecycle и doc references | Каталог не заменяет исходный ACL и не доказывает качество текста |
| Файлы / S3 export | M1 | Выбранные документы с manifest, hash, owner и извлекаемым текстом | Без metadata не появляется история review/авторства |
| Notion, Google Drive/Docs, SharePoint/OneDrive | M2 | Inventory, текст и изменения в пределах подтверждённых возможностей | Текущие metadata и change feed не равны полной истории редактирования |

Корпус имеет согласованную аудиторию. Сборщик не получает права показывать всем заголовки и выдержки только потому, что его service account видит страницу. Внешние links проверяются из выбранной сетевой области, с отдельной классификацией доступа и ошибок.

## Штатные профили и приоритет источников

| Область | Штатная поставка | Как совмещается с внешним |
|---|---|---|
| Код | Lizard, jscpd, Ruff, ESLint/typescript-eslint, golangci-lint, ShellCheck в поддержанном профиле | Внешний законченный отчёт выбранного kind/profile/SHA закрывает только соответствующую область |
| Безопасность | Opengrep с проверенным starter rule pack, Trivy SCA/IaC/image по профилю, Gitleaks CLI; image scan в CI клиента | Отдельные kinds не отключаются включением одного внешнего анализатора |
| Документация | Markdown parsing, links/anchors, выбранные markdownlint/Vale, lychee, Spectral/oasdiff по этапу | Предпочтителен готовый build/test report клиента; произвольный wiki/build не исполняется в backend |
| Активность и задачи | Сбор, история, mapping, relations, метрики и выдача craftPulse | Трекер и Git остаются исходными системами; штатный движок их не переписывает |

В поставке фиксируются version/digest, config/rules hash и разрешённые лицензии компонентов. Внешний или штатный источник выбирается по component, language, kind и profile. Несовместимые результаты сохраняются раздельно; два отчёта одной проверки не суммируются без дедупликации.

## Что необходимо передать при импорте

Минимальный контракт включает координаты источника и объекта, tenant/project, время события и наблюдения, source SHA/digest или версию документа, тип отчёта и версию schema, producer/profile/rules, область и исключения, признак завершённости и evidence. Для истории задач передаются переходы и изменение полей, для attempts - их идентичность и исходы.

Для проверки самого подключения сохраняются точный scope, границы backfill, cursor/checkpoint, известные page totals, ошибки, последняя успешная сверка и доступные возможности. Подробный контракт определяется каталогом соответствующего блока; один JSON-формат без этих фактов не заменяет его.

## Ошибки и повторная сверка

| Ситуация | Поведение |
|---|---|
| Нет необходимых прав | Показать недоступную capability и причину; не выдавать ноль вместо неизвестного результата |
| Rate limit или временная ошибка | Сохранить checkpoint и видимый lag; после восстановления продолжить и сверить пропуски |
| Урезанная история/страница | Отметить completeness и допустимую границу результата; не считать старый план полным |
| Неверная schema или несопоставимый отчёт | Сохранить причину отклонения; прежний отчёт не объявлять актуальным для нового head |
| Повторный webhook/импорт | Обновить известное событие по identity без двойного результата |
| Источник удалён или доступ отозван | Прекратить чтение и показ в отозванной области, обработать состояние по policy удаления/retention |

Подключение принято для сценария, когда выборка и история сверяются с источником на тот же as-of, повторная доставка идемпотентна, нужная метрика раскрывается до исходного evidence, а неполнота видна пользователю. Список проверок и правила расчёта - в [metrics.md](metrics.md); сценарии использования - в [features.md](../requirements/features.md).
