# Источники документации

Материалы по метрикам и интеграциям зафиксированы 21.10.2026, рыночные материалы - 14.10.2026. Источники обосновывают определения, форматы, возможности систем и сравнение рынка. Доступность конкретных полей зависит от версии, редакции и прав; подключение проверяется на выбранном инстансе.

<a id="ref-001"></a>

## ref-001. SonarQube: определения метрик

Адрес: `https://docs.sonarsource.com/sonarqube-server/user-guide/code-metrics/metrics-definition`.

Разделение overall/new code, complexity, duplication, issues и coverage; значения разных режимов Sonar нельзя бездумно смешивать

<a id="ref-002"></a>

## ref-002. SonarQube: Web API

Адрес: `https://docs.sonarsource.com/sonarqube-server/extension-guide/web-api`.

Measures API, Browse permission, Bearer token, постепенная замена API v1 на v2

<a id="ref-003"></a>

## ref-003. Sonar: quality profiles и gates

Адрес: `https://www.sonarsource.com/blog/clean_coding-quality_profile_quality_gate_guidance/`.

Идея контроля нового кода вместо блокировки всего legacy

<a id="ref-004"></a>

## ref-004. CodeScene: терминология 7.3.6

Адрес: `https://docs.enterprise.codescene.io/versions/7.3.6/terminology/codescene-terminology.html`.

Hotspot означает релевантность изменений и требует контекста качества

<a id="ref-005"></a>

## ref-005. CodeScene: temporal coupling

Адрес: `https://docs.enterprise.codescene.io/versions/3.5.22/guides/technical/temporal-coupling.html`.

Исторически документированный анализ совместных изменений; используем идею, не заявляем актуальный API или тариф по старой версии

<a id="ref-006"></a>

## ref-006. CodeScene: виды Code Health

Адрес: `https://helpcenter.codescene.com/en/help/articles/8038123-what-are-the-different-code-health-scores`.

Различие общего и hotspot-взвешенного представления; закрытая формула не переносится в CraftPulse

<a id="ref-007"></a>

## ref-007. Lizard

Адрес: `https://github.com/terryyin/lizard`.

Структурные показатели функций, NLOC, cyclomatic complexity; не бизнес-корректность кода

<a id="ref-008"></a>

## ref-008. jscpd

Адрес: `https://github.com/kucherenko/jscpd`.

Поиск копий; возможности текущего upstream не автоматически доступны pinned версии 5.1.2 прототипа

<a id="ref-009"></a>

## ref-009. Ruff: linter

Адрес: `https://docs.astral.sh/ruff/linter/`.

Настраиваемые правила, suppression, разделение безопасных/небезопасных исправлений

<a id="ref-010"></a>

## ref-010. ESLint: formatters

Адрес: `https://eslint.org/docs/latest/use/formatters/`.

JSON-диагностика; version-specific адаптеры и плагины

<a id="ref-011"></a>

## ref-011. ShellCheck

Адрес: `https://github.com/koalaman/shellcheck`.

Статическая проверка shell; отдельный движок, не исполнение скрипта

<a id="ref-012"></a>

## ref-012. Codecov: project и patch coverage

Адрес: `https://docs.codecov.com/docs/frequently-asked-questions`.

Покрытие всего scope и изменённых строк - разные знаменатели

<a id="ref-013"></a>

## ref-013. PVS-Studio: форматы результатов

Адрес: `https://pvs-studio.com/en/docs/manual/0038/`.

Экспорт SARIF, GitLab Code Quality и другие форматы через converter; запуск самого анализатора требует своей лицензии

<a id="ref-014"></a>

## ref-014. Opengrep

Адрес: `https://github.com/opengrep/opengrep`.

SAST-движок, LGPL-2.1, JSON/SARIF; возможности конкретных правил проверяются отдельно

<a id="ref-015"></a>

## ref-015. Opengrep rules

Адрес: `https://github.com/opengrep/opengrep-rules`.

Кандидат источника правил; происхождение fork и лицензирование отдельных файлов требуют фиксации на выбранном commit

<a id="ref-016"></a>

## ref-016. Semgrep Rules License

Адрес: `https://semgrep.dev/legal/rules-license/`.

Ограничения распространения/предоставления правил как сервиса; не разрешение на комплектование конкурирующего SaaS

<a id="ref-017"></a>

## ref-017. Semgrep: изменение лицензирования

Адрес: `https://semgrep.dev/blog/2024/important-updates-to-semgrep-oss`.

Явное различие лицензий движка и правил

<a id="ref-018"></a>

## ref-018. Trivy: vulnerabilities

Адрес: `https://trivy.dev/docs/latest/scanner/vulnerability/`.

Пакетные уязвимости, источники и severity, особенности определения установленных версий

<a id="ref-019"></a>

## ref-019. Trivy: SBOM

Адрес: `https://trivy.dev/docs/latest/supply-chain/sbom/`.

Генерация/сканирование SBOM; CycloneDX/SPDX не означают полноту инвентаризации автоматически

<a id="ref-020"></a>

## ref-020. Gitleaks CLI

Адрес: `https://github.com/gitleaks/gitleaks`.

Сканирование файлов/Git, JSON/SARIF, redaction; нельзя полагаться только на маскирование stdout

<a id="ref-021"></a>

## ref-021. OSV-Scanner

Адрес: `https://google.github.io/osv-scanner/`.

Альтернатива/дополнение SCA, поддерживаемые manifests, отдельные расширенные возможности

<a id="ref-022"></a>

## ref-022. EPSS: данные

Адрес: `https://www.first.org/epss/data.html`.

Обогащение CVE данными вероятности эксплуатации; дата score обязательна

<a id="ref-023"></a>

## ref-023. API

Адрес: `https://api.first.org/epss/`.

Обогащение CVE данными вероятности эксплуатации; дата score обязательна

<a id="ref-024"></a>

## ref-024. CISA KEV

Адрес: `https://www.cisa.gov/known-exploited-vulnerabilities-catalog`.

Каталог известных эксплуатируемых уязвимостей. Основная страница при прямом открытии вернула 403; описание найдено в официальной индексации. Live-feed не загружался

<a id="ref-025"></a>

## ref-025. официальный JSON

Адрес: `https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json`.

<a id="ref-026"></a>

## ref-026. SARIF 2.1.0, OASIS

Адрес: `https://docs.oasis-open.org/sarif/sarif/v2.1.0/os/sarif-v2.1.0-os.html`.

Общий формат статических находок; хранение rule, location, fingerprint, invocation. Не универсальный формат данных тестов/SBOM

<a id="ref-027"></a>

## ref-027. CycloneDX: relationships и VEX

Адрес: `https://cyclonedx.org/guides/sbom/relationships/`.

Контекст применимости уязвимости и зависимости; VEX - отдельное свидетельство

<a id="ref-028"></a>

## ref-028. CodeScoring: export

Адрес: `https://docs.codescoring.ru/en/user-guide/sca/export-results`.

CycloneDX/SPDX и отдельные расширения свойств; доступные версии проверять при импорте

<a id="ref-029"></a>

## ref-029. CodeScoring: scan bom

Адрес: `https://docs.codescoring.ru/en/user-guide/agent/scan-bom`.

Импорт SBOM/VEX, различие findings, scan error и empty result в exit codes

<a id="ref-030"></a>

## ref-030. Solar appScreener: руководство 3.14.2

Адрес: `https://rt-solar.ru/upload/documents/appscreener/solar-appscreener-v-3-14-2-user-guide.pdf`.

SARIF и JSON через API в указанной версии; не универсальная гарантия всех редакций

<a id="ref-031"></a>

## ref-031. CodeQL CLI

Адрес: `https://docs.github.com/en/code-security/concepts/code-scanning/codeql/codeql-cli`.

Использование и лицензирование для закрытых репозиториев; не включать бесплатно в произвольный B2B комплект

<a id="ref-032"></a>

## ref-032. DefectDojo: reimport

Адрес: `https://docs.defectdojo.com/import_data/import_intro/reimport/`.

Стабильная идентичность находок, риск ложного закрытия/переоткрытия при изменении строк

<a id="ref-033"></a>

## ref-033. deduplication

Адрес: `https://docs.defectdojo.com/triage_findings/finding_deduplication/about_deduplication/`.

Стабильная идентичность находок, риск ложного закрытия/переоткрытия при изменении строк

<a id="ref-034"></a>

## ref-034. Dependency-Track: suppression

Адрес: `https://docs.dependencytrack.org/triage/suppression/`.

Контекстный триаж и агрегирование зависимостей; не подтверждает единую метрику риска для всех классов

<a id="ref-035"></a>

## ref-035. terminology

Адрес: `https://docs.dependencytrack.org/terminology/`.

<a id="ref-036"></a>

## ref-036. SLSA build track 1.2

Адрес: `https://slsa.dev/spec/v1.2/build-track-basics`.

Provenance и требования к доверию сборки; наличие файла не равно соответствию уровню

<a id="ref-037"></a>

## ref-037. Cosign: verification

Адрес: `https://docs.sigstore.dev/cosign/verifying/verify/`.

Проверка подписи с учётом identity/issuer; хранение «есть подпись» недостаточно

<a id="ref-038"></a>

## ref-038. GitLab: Jobs API

Адрес: `https://docs.gitlab.com/api/jobs/`.

`include_retried=true`, job timestamps/status/allow_failure; адаптер должен учитывать версию API

<a id="ref-039"></a>

## ref-039. GitLab: Pipelines API

Адрес: `https://docs.gitlab.com/api/pipelines/`.

Pipeline metadata, duration и test reports при наличии

<a id="ref-040"></a>

## ref-040. GitLab: Deployments API

Адрес: `https://docs.gitlab.com/api/deployments/`.

Deployment как отдельный объект, environment и SHA; сборка не равна деплою

<a id="ref-041"></a>

## ref-041. GitLab: Unit test reports

Адрес: `https://docs.gitlab.com/ci/testing/unit_test_reports/`.

JUnit из pipeline; наличие отчёта не делает проверку автоматически блокирующей

<a id="ref-042"></a>

## ref-042. GitLab: Code Quality

Адрес: `https://docs.gitlab.com/ci/testing/code_quality/`.

Free-импорт, MR reports, обязательные поля Code Quality; CodeClimate scan template deprecated

<a id="ref-043"></a>

## ref-043. GitLab: CI Lint API

Адрес: `https://docs.gitlab.com/api/lint/`.

Валидация в контексте проекта; применение параметров зависит от версии

<a id="ref-044"></a>

## ref-044. GitHub: workflow runs

Адрес: `https://docs.github.com/en/rest/actions/workflow-runs`.

Запуски/попытки/jobs, права чтения; GitHub App предпочтительнее общего пользовательского PAT

<a id="ref-045"></a>

## ref-045. workflow jobs

Адрес: `https://docs.github.com/en/rest/actions/workflow-jobs`.

Запуски/попытки/jobs, права чтения; GitHub App предпочтительнее общего пользовательского PAT

<a id="ref-046"></a>

## ref-046. GitHub: code scanning API

Адрес: `https://docs.github.com/en/rest/code-scanning/code-scanning`.

Чтение alerts и analyses; данные зависят от включённых security-возможностей и прав

<a id="ref-047"></a>

## ref-047. Jenkins Remote Access API

Адрес: `https://www.jenkins.io/doc/book/using/remote-access-api/`.

JSON/XML API; stage graph, test plugin и retry semantics не гарантируются core API

<a id="ref-048"></a>

## ref-048. TeamCity Build

Адрес: `https://www.jetbrains.com/help/teamcity/rest/build.html`.

Builds, очередь, revisions и доступные связи; JSON/XML, version-specific контракт

<a id="ref-049"></a>

## ref-049. REST overview

Адрес: `https://www.jetbrains.com/help/teamcity/rest/teamcity-rest-api-documentation.html`.

Builds, очередь, revisions и доступные связи; JSON/XML, version-specific контракт

<a id="ref-050"></a>

## ref-050. SourceCraft API overview

Адрес: `https://vibe.sourcecraft.dev/portal/docs/en/api-ref/`.

На дату проверки CI/CD опубликован отдельно от Withdrawn; даты run/workflow/task/cube, статусы и артефакты

<a id="ref-051"></a>

## ref-051. ListRunsByID

Адрес: `https://sourcecraft.dev/portal/docs/en/api-ref/CICD/ListRunsByID`.

<a id="ref-052"></a>

## ref-052. SourceCraft: CI tutorial

Адрес: `https://yandex.cloud/en/docs/tutorials/serverless/ci-cd-sourcecraft-github-actions`.

Действующий CI и интеграция внешних SARIF-анализаторов; официальный пример `.sourcecraft/ci.yaml`

<a id="ref-053"></a>

## ref-053. июньский дайджест 2026

Адрес: `https://yandex.cloud/ru/blog/digest-june-2026`.

Действующий CI и интеграция внешних SARIF-анализаторов; официальный пример `.sourcecraft/ci.yaml`

<a id="ref-054"></a>

## ref-054. GitVerse Public API

Адрес: `https://gitverse.ru/docs/developers/public-api/`.

Токен, Accept version=1, группы API. Нельзя объявлять полную совместимость с GitHub по сходству структуры

<a id="ref-055"></a>

## ref-055. versioning

Адрес: `https://gitverse.ru/docs/developers/public-api/using-public-api/api-versioning`.

<a id="ref-056"></a>

## ref-056. OpenAPI repo

Адрес: `https://gitverse.ru/gitverse/rest-api-description`.

<a id="ref-057"></a>

## ref-057. GitFlic CI API

Адрес: `https://docs.gitflic.ru/latest/en/api/cicd/`.

Pipeline, jobs, artifacts; источники test/coverage берём через отчёты, не выдуманный test endpoint

<a id="ref-058"></a>

## ref-058. Argo CD notification triggers

Адрес: `https://argo-cd.readthedocs.io/en/stable/operator-manual/notifications/triggers/`.

События sync/health, повторное уведомление требует дедупликации; sync не равен бизнес-здоровью

<a id="ref-059"></a>

## ref-059. OpenTelemetry CI/CD

Адрес: `https://opentelemetry.io/docs/specs/semconv/cicd/`.

Открытая модель телеметрии; отдельные элементы находятся в Release Candidate, фиксировать schema version

<a id="ref-060"></a>

## ref-060. entities

Адрес: `https://opentelemetry.io/docs/specs/semconv/registry/entities/cicd/`.

<a id="ref-061"></a>

## ref-061. DORA: delivery performance metrics

Адрес: `https://dora.dev/guides/dora-metrics/`.

Пять текущих метрик, включая deployment rework; delivery не смешивать с CI reliability

<a id="ref-062"></a>

## ref-062. история метрик

Адрес: `https://dora.dev/insights/tags/metrics/`.

Пять текущих метрик, включая deployment rework; delivery не смешивать с CI reliability

<a id="ref-063"></a>

## ref-063. Datadog: flaky tests

Адрес: `https://docs.datadoghq.com/tests/flaky_tests/`.

Контекст одного commit/configuration, стоимость нестабильности и lifecycle

<a id="ref-064"></a>

## ref-064. management

Адрес: `https://docs.datadoghq.com/tests/flaky_management/`.

Контекст одного commit/configuration, стоимость нестабильности и lifecycle

<a id="ref-065"></a>

## ref-065. Datadog: test impact analysis

Адрес: `https://docs.datadoghq.com/tests/test_impact_analysis/how_it_works/`.

Для выбора тестов нужна per-test связь с кодом; JUnit один её не даёт

<a id="ref-066"></a>

## ref-066. JUnit limits

Адрес: `https://docs.datadoghq.com/tests/guides/setup_new_flaky_pr_gate/`.

Для выбора тестов нужна per-test связь с кодом; JUnit один её не даёт

<a id="ref-067"></a>

## ref-067. actionlint

Адрес: `https://github.com/rhysd/actionlint`.

Статический разбор GitHub Actions и некоторых опасных выражений; не доказательство полноты security review

<a id="ref-068"></a>

## ref-068. license

Адрес: `https://github.com/rhysd/actionlint/blob/main/LICENSE.txt`.

<a id="ref-069"></a>

## ref-069. GitHub: secure pull_request_target

Адрес: `https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target`.

Разделение доверенного workflow и недоверенного кода; privileged runner нельзя использовать для исполнения PR-кода

<a id="ref-070"></a>

## ref-070. Apache DevLake: sources

Адрес: `https://devlake.apache.org/docs/Overview/SupportedDataSources/`.

Бесплатная реальная альтернатива агрегатору; не заявлять, что объединение Git/Jira/CI само по себе уникально

<a id="ref-071"></a>

## ref-071. GitLab: artifacts reports

Адрес: `https://docs.gitlab.com/ci/yaml/artifacts_reports/`.

Ordinary artifacts, native report processing и security widgets - разные возможности; upload не равен ingestion, отдельные security types требуют Ultimate

<a id="ref-072"></a>

## ref-072. SPACE, Microsoft Research

Адрес: `https://www.microsoft.com/en-us/research/publication/the-space-of-developer-productivity-theres-more-to-it-than-you-think/`.

Продуктивность многомерна, активность - только одна ось; A-каталог и отсутствие person-score | Веса нашего dashboard или спрос респондентов

<a id="ref-073"></a>

## ref-073. Kanban Guide, May 2025

Адрес: `https://kanbanguides.org/the-kanban-guide/2025.5/`.

Workflow, WIP, throughput, age, cycle time, SLE; T-каталог и review | Production origin по reopen, универсальный target или статистическую гарантию малого n

<a id="ref-074"></a>

## ref-074. Diátaxis

Адрес: `https://diataxis.fr/`.

Разные пользовательские потребности и формы документации; doc profiles | Обязательность четырёх документов для каждого repo

<a id="ref-075"></a>

## ref-075. Backstage technical overview

Адрес: `https://backstage.io/docs/overview/technical-overview/`.

Service catalog, ownership, APIs и TechDocs как связанные компоненты | Автоматическое знание архитектуры или effective ACL внешнего wiki

<a id="ref-076"></a>

## ref-076. TechDocs setup

Адрес: `https://backstage.io/docs/features/techdocs/getting-started/`.

Docs-as-code, builder/generator/publisher, рекомендация CI generation | Enterprise docs всегда должны быть только в Git

<a id="ref-077"></a>

## ref-077. DevLake domain schema

Адрес: `https://devlake.apache.org/docs/DataModels/DevLakeDomainLayerSchema/`.

Нормализованный domain layer из tool-specific данных | Уникальность нашего графа или лёгкость реализации

<a id="ref-078"></a>

## ref-078. GitLab value stream analytics

Адрес: `https://docs.gitlab.com/user/group/value_stream_analytics/`.

VSA capabilities, tier-dependent aggregation/default stages | Что на GitLab Free нет вообще никакой аналитики

<a id="ref-079"></a>

## ref-079. GitLab Merge Requests API

Адрес: `https://docs.gitlab.com/api/merge_requests/`.

MR lifecycle, поля и фильтры; A05–A10/A18 | Не все review/approval capabilities одинаковы по версиям

<a id="ref-080"></a>

## ref-080. GitLab resource state events

Адрес: `https://docs.gitlab.com/api/resource_state_events/`.

State transition events отдельных ресурсов | Не вся история любых custom fields

<a id="ref-081"></a>

## ref-081. GitLab Events API

Адрес: `https://docs.gitlab.com/api/events/`.

Feed событий и ссылки на retention limits | Не бессрочный полный архив всей инженерной работы

<a id="ref-082"></a>

## ref-082. GitHub issue timeline

Адрес: `https://docs.github.com/en/rest/issues/timeline`.

Timeline; PR связан с issue API model | Не отдельная дополнительная задача для throughput

<a id="ref-083"></a>

## ref-083. GitHub pull request reviews

Адрес: `https://docs.github.com/en/rest/pulls/reviews`.

Reviews со state, chronological list, read permission | Обычный comment не автоматически review decision

<a id="ref-084"></a>

## ref-084. Jira DC REST 9.17

Адрес: `https://docs.atlassian.com/software/jira/docs/api/REST/9.17.0/`.

DC issues/fields/changelog-related API contract изученной версии | Не Cloud и не заявление о latest DC

<a id="ref-085"></a>

## ref-085. Jira Cloud REST v3

Адрес: `https://developer.atlassian.com/cloud/jira/platform/rest/v3/`.

Permissions/pagination/versioned Cloud model | Total/страницы и доступ не предполагаются постоянными

<a id="ref-086"></a>

## ref-086. Jira Cloud Issues/changelogs

Адрес: `https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/`.

Paginated issue changelog methods, issue field operations | История не гарантированно полна в одном `expand`

<a id="ref-087"></a>

## ref-087. Jira Software REST

Адрес: `https://developer.atlassian.com/cloud/jira/software/rest/`.

Agile board/sprint resources | Текущие члены sprint не начальный план

<a id="ref-088"></a>

## ref-088. Jira sprint endpoints

Адрес: `https://developer.atlassian.com/cloud/jira/software/rest/api-group-sprint`.

Sprint methods; deprecated старый issues endpoint и enhanced token-based путь | Новый endpoint не восстанавливает сам initial membership

<a id="ref-089"></a>

## ref-089. Jira reparenting changelog migration

Адрес: `https://developer.atlassian.com/cloud/jira/platform/deprecation-notice-issue-reparenting-changelogs/`.

Изменение Epic Link/Parent semantics в сторону IssueParentAssociation | Не новый event-time; дата обновления страницы не дата начала миграции

<a id="ref-090"></a>

## ref-090. Яндекс Трекер changelog

Адрес: `https://yandex.ru/support/tracker/ru/api/issues/get-changelog`.

История задачи, `Link` pagination с cursor/id | Не переносится Jira statusCategory/field naming

<a id="ref-091"></a>

## ref-091. Redmine REST Issues

Адрес: `https://www.redmine.org/projects/redmine/wiki/Rest_Issues`.

Default open only, `status_id=*`, detail journals, relations | Не доказательство встроенного sprint engine ядра

<a id="ref-092"></a>

## ref-092. YouTrack issue history

Адрес: `https://www.jetbrains.com/help/youtrack/devportal/api-usecase-issue-history.html`.

Activities endpoint, categories/fields, history use case | Coverage зависит от выбранных категорий и pagination

<a id="ref-093"></a>

## ref-093. Kaiten API

Адрес: `https://developers.kaiten.ru/`.

Карточки/доски и доступный REST contract; пометки deprecation | Старые sprint endpoints не основание обещать текущую sprint integration

<a id="ref-094"></a>

## ref-094. Linear GraphQL getting started

Адрес: `https://linear.app/developers/graphql`.

OAuth/API key, schema introspection, workflow states, archived resources, partial errors | Не proof полной истории state/membership без проверки schema

<a id="ref-095"></a>

## ref-095. Linear pagination

Адрес: `https://linear.app/developers/pagination`.

Relay cursor model и pageInfo | Первая страница не весь workspace

<a id="ref-096"></a>

## ref-096. Confluence Cloud REST v2

Адрес: `https://developer.atlassian.com/cloud/confluence/rest/v2/`.

Cursor pagination, page resources | Не DC endpoints

<a id="ref-097"></a>

## ref-097. Confluence Cloud versions

Адрес: `https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-version/`.

Page/content versions по доступным правам | Версия страницы не подтверждённый review

<a id="ref-098"></a>

## ref-098. Confluence content restrictions

Адрес: `https://developer.atlassian.com/cloud/confluence/rest/v1/api-group-content-restrictions/`.

Explicit restrictions APIs и оговорки доступа | Проверка одной restriction не гарантирует effective read access с учётом space/product/inheritance

<a id="ref-099"></a>

## ref-099. Confluence DC REST

Адрес: `https://developer.atlassian.com/server/confluence/rest/latest/`.

DC content/version/history API family | Совместимость каждой версии подтверждается отдельно

<a id="ref-100"></a>

## ref-100. Яндекс Wiki pages API

Адрес: `https://yandex.ru/support/wiki/ru/api-ref/pages/`.

Страницы, slug/id, body и структура по методам | UI history не гарантирует API version archive

<a id="ref-101"></a>

## ref-101. Яндекс Wiki API access

Адрес: `https://yandex.ru/support/wiki/ru/api-ref/access`.

Включение API администратором, пользовательский доступ, OAuth/org context | Integration не получает автоматически все страницы организации

<a id="ref-102"></a>

## ref-102. Яндекс Wiki examples

Адрес: `https://yandex.ru/support/wiki/ru/api-ref/examples`.

Base URL и примеры чтения pages | Пишущие методы из примеров не выполнялись

<a id="ref-103"></a>

## ref-103. Notion block children

Адрес: `https://developers.notion.com/reference/get-block-children`.

Body как blocks, pagination и recursive has_children, Notion-Version | Page metadata не весь документ; unsupported blocks не пустой текст

<a id="ref-104"></a>

## ref-104. Drive files resource

Адрес: `https://developers.google.com/workspace/drive/api/reference/rest/v3/files`.

Metadata/MIME/version и file references | Не весь Workspace document body в metadata

<a id="ref-105"></a>

## ref-105. Drive retrieve changes

Адрес: `https://developers.google.com/workspace/drive/api/guides/manage-changes`.

Start page token, changes.list, current file state | Не полный исторический журнал каждого редактирования

<a id="ref-106"></a>

## ref-106. Drive revisions

Адрес: `https://developers.google.com/workspace/drive/api/guides/manage-revisions`.

Revisions capabilities и ограничения | Не гарантированный immutable audit log

<a id="ref-107"></a>

## ref-107. Microsoft Graph driveItem delta

Адрес: `https://learn.microsoft.com/en-us/graph/api/driveitem-delta?view=graph-rest-1.0`.

nextLink/deltaLink, deletions, permissions | Не readership и не semantic review

<a id="ref-108"></a>

## ref-108. markdownlint

Адрес: `https://github.com/DavidAnson/markdownlint`.

Markdown/CommonMark lint rules | Lint-clean не равно полезные/истинные docs

<a id="ref-109"></a>

## ref-109. Vale

Адрес: `https://vale.sh/`.

Configurable prose linting, terminology | Универсальный показатель качества русского и английского текста

<a id="ref-110"></a>

## ref-110. Vale packages

Адрес: `https://vale.sh/explorer`.

Подключаемые styles и словари | Полную пригодность каждого community rule без локальной проверки

<a id="ref-111"></a>

## ref-111. lychee CLI

Адрес: `https://lychee.cli.rs/guides/cli/`.

Link check inputs/options и structured integration | Любая сеть и любой код из repo безопасны для запуска

<a id="ref-112"></a>

## ref-112. lychee internals

Адрес: `https://lychee.cli.rs/internals/how-it-works/`.

Markdown/HTML extraction и pipeline проверки | 403 означает broken document

<a id="ref-113"></a>

## ref-113. Spectral primary repository

Адрес: `https://github.com/stoplightio/spectral`.

Rule-based JSON/YAML/API linting | Наличие description подтверждает корректность API

<a id="ref-114"></a>

## ref-114. oasdiff primary repository

Адрес: `https://github.com/oasdiff/oasdiff`.

OpenAPI diff и breaking-change candidates | Spec diff сам доказывает сбой production клиента

<a id="ref-115"></a>

## ref-115. pricing

Адрес: `https://codescene.com/pricing`.

<a id="ref-116"></a>

## ref-116. software-delivery

Адрес: `https://codescene.com/product/software-delivery`.

<a id="ref-117"></a>

## ref-117. FAQ

Адрес: `https://codescene.com/resources/faq`.

<a id="ref-118"></a>

## ref-118. docs PR

Адрес: `https://codescene.io/docs/guides/pr-integration/integrate-into-ci-cd.html`.

<a id="ref-119"></a>

## ref-119. docs PM 3.6.5

Адрес: `https://docs.enterprise.codescene.io/versions/3.6.5/integrations/integrate-project-management-data.html`.

<a id="ref-120"></a>

## ref-120. REST API

Адрес: `https://codescene.io/docs/integrations/rest-api.html`.

<a id="ref-121"></a>

## ref-121. codescoring.ru

Адрес: `https://codescoring.ru/`.

<a id="ref-122"></a>

## ref-122. docs functionality

Адрес: `https://docs.codescoring.ru/functionality`.

<a id="ref-123"></a>

## ref-123. TQI

Адрес: `https://docs.codescoring.ru/en/user-guide/tqi`.

<a id="ref-124"></a>

## ref-124. TQI metrics

Адрес: `https://docs.codescoring.ru/en/user-guide/tqi/metrics`.

<a id="ref-125"></a>

## ref-125. codescoring.ru/tqi

Адрес: `https://codescoring.ru/tqi`.

<a id="ref-126"></a>

## ref-126. anti-malware

Адрес: `https://www.anti-malware.ru/news/2022-03-25-111332/38409`.

<a id="ref-127"></a>

## ref-127. plans

Адрес: `https://www.sonarsource.com/plans-and-pricing/`.

<a id="ref-128"></a>

## ref-128. server plans

Адрес: `https://www.sonarsource.com/plans-and-pricing/sonarqube/`.

<a id="ref-129"></a>

## ref-129. editions

Адрес: `https://docs.sonarsource.com/sonarqube-server/discovering/sonarqube-server-editions`.

<a id="ref-130"></a>

## ref-130. blog

Адрес: `https://www.sonarsource.com/blog/sonarqube-compare-editions/`.

<a id="ref-131"></a>

## ref-131. about

Адрес: `https://www.jetbrains.com/help/qodana/about-qodana.html`.

<a id="ref-132"></a>

## ref-132. editions

Адрес: `https://www.jetbrains.com/help/qodana/pricing.html`.

<a id="ref-133"></a>

## ref-133. CI

Адрес: `https://www.jetbrains.com/help/qodana/ci.html`.

<a id="ref-134"></a>

## ref-134. GitLab

Адрес: `https://www.jetbrains.com/help/qodana/gitlab.html`.

<a id="ref-135"></a>

## ref-135. insights

Адрес: `https://www.jetbrains.com/help/qodana/insights.html`.

<a id="ref-136"></a>

## ref-136. blog 2026-03

Адрес: `https://blog.jetbrains.com/qodana/2026/03/sonarqube-vs-qodana/`.

<a id="ref-137"></a>

## ref-137. licenses

Адрес: `https://www.jetbrains.com/help/qodana/contributors.html`.

<a id="ref-138"></a>

## ref-138. JetBrains blog

Адрес: `https://blog.jetbrains.com/blog/2022/03/11/jetbrains-statement-on-ukraine/`.

<a id="ref-139"></a>

## ref-139. update

Адрес: `https://blog.jetbrains.com/blog/2022/12/06/update-on-jetbrains-statement-on-ukraine/`.

<a id="ref-140"></a>

## ref-140. plans

Адрес: `https://snyk.io/plans/`.

<a id="ref-141"></a>

## ref-141. SCM

Адрес: `https://docs.snyk.io/developer-tools/scm-integrations/organization-level-integrations`.

<a id="ref-142"></a>

## ref-142. Jira

Адрес: `https://docs.snyk.io/integrate-with-snyk/jira-and-slack-integrations/jira-integration`.

<a id="ref-143"></a>

## ref-143. API

Адрес: `https://docs.snyk.io/snyk-api/snyk-api`.

<a id="ref-144"></a>

## ref-144. CLI

Адрес: `https://docs.snyk.io/developer-tools/snyk-cli/commands/code-test`.

<a id="ref-145"></a>

## ref-145. Snyk blog

Адрес: `https://snyk.io/blog/snyk-ceases-business-russia-belarus/`.

<a id="ref-146"></a>

## ref-146. pricing

Адрес: `https://semgrep.dev/pricing`.

<a id="ref-147"></a>

## ref-147. connect SCM

Адрес: `https://semgrep.dev/docs/deployment/connect-scm`.

<a id="ref-148"></a>

## ref-148. dashboard

Адрес: `https://semgrep.dev/docs/semgrep-appsec-platform/dashboard`.

<a id="ref-149"></a>

## ref-149. API

Адрес: `https://docs.semgrep.dev/api-reference/v1/findingsservice/list-code-supply-chain-or-ai-powered-detection-findings`.

<a id="ref-150"></a>

## ref-150. SARIF

Адрес: `https://docs.semgrep.dev/semgrep-appsec-platform/json-and-sarif`.

<a id="ref-151"></a>

## ref-151. pricing

Адрес: `https://www.coderabbit.ai/pricing`.

<a id="ref-152"></a>

## ref-152. platforms

Адрес: `https://docs.coderabbit.ai/platforms/overview`.

<a id="ref-153"></a>

## ref-153. dashboard

Адрес: `https://docs.coderabbit.ai/guides/dashboard`.

<a id="ref-154"></a>

## ref-154. ToS

Адрес: `https://www.coderabbit.ai/terms-of-service`.

<a id="ref-155"></a>

## ref-155. ispras svace

Адрес: `https://www.ispras.ru/technologies/svace/`.

<a id="ref-156"></a>

## ref-156. rt-solar

Адрес: `https://rt-solar.ru/products/solar_appscreener/`.

<a id="ref-157"></a>

## ref-157. брошюра

Адрес: `https://rt-solar.ru/upload/materials/appscreener/SolarappScreener.pdf`.

<a id="ref-158"></a>

## ref-158. pvs fstec

Адрес: `https://pvs-studio.ru/ru/pvs-studio/fstec/`.

<a id="ref-159"></a>

## ref-159. лицензии

Адрес: `https://pvs-studio.ru/ru/order/license/`.

<a id="ref-160"></a>

## ref-160. Atlas docs

Адрес: `https://pvs-studio.com/en/docs/manual/7197/`.

<a id="ref-161"></a>

## ref-161. покупка

Адрес: `https://pvs-studio.ru/ru/order/`.

<a id="ref-162"></a>

## ref-162. ptsecurity.com/products/ai

Адрес: `https://ptsecurity.com/products/ai/`.

<a id="ref-163"></a>

## ref-163. описание продукта, PDF

Адрес: `https://storage.yandexcloud.net/static.ptsecurity.com/uploads/files/pt_ai_description_124e2a93a5.pdf`.

<a id="ref-164"></a>

## ref-164. datasheet

Адрес: `https://static.ptsecurity.com/uploads/files/PT_Application_Inspector_datasheet_8280d8ef08.pdf`.
