# Метрики CI/CD

Блок хранит попытки проверок, отчёты, контекст сборки и подтверждённые события доставки. Бизнес-готовность задачи определяется её обязательствами и проверяется отдельно от успешного pipeline.

## Что должен дать блок

Первый ответ техлиду: **можно ли доверять результату конкретного pipeline**. Второй: где тратится время на обратную связь и повторные попытки. Третий, после подключения CD: какой артефакт действительно дошёл до production, какие проверки были выполнены и что происходило после доставки.

Это три независимые панели: надёжность CI, длительность/стоимость и доставка. Успешная сборка не доказывает успешный deployment. Зелёный pipeline не доказывает, что все проверки прошли с первого раза. Прохождение тестов не доказывает достаточного покрытия. Метрики подтверждённого результата и полнота источника всегда отображаются рядом.

## Коннекторы к CI и CD

### Приоритетная матрица

| Сервис | Что читаем | Что получаем для метрик | Этап и существенная граница |
|---|---|---|---|
| **GitLab CI self-managed CE/EE**, GitLab.com | Pipelines; jobs **с include_retried=true**; trigger/child relations в доступном versioned API; artifacts; MR→pipeline | Статусы/attempts/allow_failure, duration/queued_duration, runner labels, tests/coverage/report manifests | **M0**. Пагинация всех jobs, parent/child pipeline не удваивать; security features и права на artifacts проверяются отдельно |
| **SourceCraft CI** | Runs→workflows→tasks→cubes, даты, статусы, artifacts по опубликованному API | Длительность и результат на правильном уровне; расширенная детализация и PR links, если отданы | **M0 сохранить имеющееся; расширить отдельным contract test**. Документированные поля не гарантируют полноту retry history |
| **Jenkins** | Core JSON API builds/result/timestamp/duration/queue; artifacts; test reports через установленный plugin | Базовая история, длительность, tests; step graph при подходящем plugin/collector | **M1**. Внутренний `retry{}` может быть невидим как отдельная попытка; `UNSTABLE` не success |
| **GitHub Actions / GHES** | Workflow runs, run_attempt, jobs для **каждой попытки**, artifacts, checks/PR mapping | Attempts, status/conclusion, timings, matrix, test/coverage импорт | **M1**. Runner queue и pending/manual waits не выводить из одного timestamp без определения |
| **GitVerse CI** | Workflows/runs/jobs/artifacts API при проверенном контракте | Аналог нормализованного CI model | **M1**. GitHub-подобный API не означает identical retry semantics |
| **GitFlic CI** | Pipelines/jobs/status/artifacts | Основные CI-метрики, тесты через артефакты | **M1**. Отдельный JUnit summary API здесь не подтверждён |
| **TeamCity** | Builds, queued/start/finish dates, revisions, test occurrences/artifacts и agent facts, если разрешены | Очередь, builds, tests и build chain relationships | **M2 / по первому клиенту**. Результат build configuration отличается от общего pipeline |
| **Azure Pipelines**, **Bitbucket Pipelines**, CircleCI, Buildkite | Версионные API/collector | Runs/jobs/tests/attempts в пределах capability | **M2**. Дополнительные источники; прямые коннекторы требуют проверки capabilities |
| **Любой CI с shell/container step** | Наш export/upload CLI и versioned envelope | Reports и execution metadata, известные локальному шагу | **M0 generic ingest**. Этот шаг не видит автоматически очередь, пропущенные jobs или все старые retries |
| **GitLab environments/deployments** | Deployment/environment/SHA/status/dates | Базовая delivery timeline | **M1**, event schema M0. `environment` обязателен для надёжного отнесения к prod |
| **Argo CD** | Notifications/API о sync/health/operation и revision | GitOps delivery attempt и observed outcome | **M1/M2**. Sync commit может относиться к manifests repo, а не исходному коду сервиса |
| **Ручной/внутренний деплой** | Подписанное deployment event: service, env, artifact digest, source/build refs, status/time | Release provenance и DORA при достаточном наборе событий | **M1**, не угадываем по job с именем deploy |

Первичные контракты: [GitLab Jobs](../../research/sources.md#ref-038), [Pipelines](../../research/sources.md#ref-039), [Deployments](../../research/sources.md#ref-040), [SourceCraft CI](../../research/sources.md#ref-051), [GitHub runs](../../research/sources.md#ref-044), [Jenkins](../../research/sources.md#ref-047), [TeamCity](../../research/sources.md#ref-048), [Argo triggers](../../research/sources.md#ref-058).

### Что знаем без настройки клиента

После API-подключения обычно доступны status/duration базовых сущностей. Для actual runner queue нужны соответствующие timestamps/fields. Для flaky-test analysis нужен **test case identity и несколько сопоставимых executions**, а не только job result. Для patch coverage нужен построчный coverage report. Для costs нужны runtime class и цена/биллинг. Для DORA нужны deployment и failure/recovery факты. Для test impact analysis нужна per-test coverage связь - JUnit её не содержит.

UI подключения показывает capability grid: «CI metadata получены; retry history получена; JUnit отсутствует; coverage отсутствует; prod mapping не задан». Не рисуем пустые чарты с нулями, когда данных нет.

### Форматы отчётов

| Формат / producer | Что импортируем | Существенная семантика |
|---|---|---|
| **JUnit XML**: pytest, JUnit/TestNG adapters, Jest/Vitest reporters, Go test→JUnit, Google Test и др. | Suite/case, classname/name, outcome, duration, error/failure/skipped, file при наличии | Диалекты различаются; требуется fixture на producer. Параметры теста и matrix входят в identity. XML summary не равен всем attempts |
| **Cobertura XML** | Line hit/branch facts, files, source roots | Нормализация путей и SHA; ветви только где producer их передал |
| **LCOV** | Line/function/branch records | Наличие ветвлений не универсально; merge только одного source revision и одинаковых units |
| **JaCoCo XML** | JVM line/branch/instruction/class facts | Не превращаем instruction coverage в line coverage |
| **Go coverprofile** | Block ranges и counts | Native block coverage хранить отдельно; построчное отображение - явно описанный adapter, не точная branch coverage |
| **SARIF / GitLab Code Quality / security JSON** | Findings и completion metadata | Находки передаются в блоки кода/безопасности; test pass rate из них не считается |
| **Native test telemetry / OTel-compatible events** | Execution/attempt/env metadata и spans | M1; versioned schema. Текущая OTel CI/CD семантика частично RC, не безусловно stable contract |

Основания: [GitLab JUnit](../../research/sources.md#ref-041), [OpenTelemetry CI/CD](../../research/sources.md#ref-059), [Codecov patch/project](../../research/sources.md#ref-012). Форматный импорт уменьшает число сервисных коннекторов, но не избавляет от проверки конкретных диалектов.

## Штатная реализация по умолчанию

В CI/CD штатный CraftPulse - **сборщик, анализатор конфигурации и движок фактов**, а не новый CI-сервер. Он не переносит pipelines клиента на свои runners и не придумывает команды сборки/тестов. Штатные quality/security tools из блоков 1–2 могут быть добавлены отдельными jobs.

### Что поставляем

1. **GitLab collector**: API+webhooks, курсоры, все страницы, повторная сверка после завершения pipeline. Ищет retry attempts, а не только последнее состояние.
2. **Сборщик отчётов CI** - CLI/контейнер подключения. Принимает explicit reports globs и CI variables, формирует manifest, очищает чувствительные поля и отправляет данные идемпотентно. Конкретная исполняемая команда задаётся поставкой интеграции.
3. **Normalizers** для JUnit и coverage, report/source identity, статус missing/partial/complete. Отчёт от другого SHA нельзя прикрепить к текущему head молча.
4. **MR publisher** на сервере или в доверенном integration context. Один обновляемый комментарий плюс status/check; token записи в MR не передаётся недоверенному коду PR.
5. **Policy engine**: advisory по умолчанию, результаты и transport errors различаются; blocking profile подключается отдельно. CraftPulse не меняет уже существующие gates клиента.
6. **Config checks**: для GitLab - lint/разбор в контексте версии instance; для GitHub - actionlint при подключении провайдера, позже дополнительные rules. Собственный schema/parser для SourceCraft проверяется по актуальной версии, а не по GitHub YAML.

У CLI должны быть разные результаты **успешно выполнено без находок**, **успешно выполнено с находками**, **scan/import error** и отдельный policy verdict. Они не сводятся к одному «exit code !=0 = сломанный анализ». При интеграции с GitLab учитываем его native security-report ingestion: загрузка файла не равна появлению findings в security UI, а security widgets могут требовать Ultimate. Для CE остаётся обычный artifact/upload в CraftPulse. [GitLab artifacts reports](../../research/sources.md#ref-071).

### Defaults

| Параметр | Настройка M0 |
|---|---|
| История при подключении | До 90 дней с лимитами rate/storage; показывать фактическую нижнюю границу и truncation |
| Cohorts | Отдельно main/push, MR, scheduled/nightly, release и manual; default branch из metadata provider, не «самая частая ветка» |
| Окна | 7/30/90 дней, календарные UTC границы; отображение в timezone организации |
| Перцентили | p50/p85/p95 duration; nearest-rank. При n<20 пометка малой выборки, не «надёжный p95» |
| Итог pipeline | Сохранять native status и нормализованный status, terminal/nonterminal, final attempt; manual/canceled/skipped отдельно |
| Перезапуски | Все доступные attempts; не запускать их самостоятельно и не назначать автоматический quarantine |
| Зелёный с оговорками | failed→passed, allow_failure failures, missing required reports - отдельные бейджи |
| Gate | Advisory; missing data не pass. В advisory pipeline не блокируется CraftPulse, но отчёт говорит incomplete |
| Пороги duration | В первый день без универсального «дольше 30 мин плохо»; после сопоставимой истории показывать baseline и задаваемый budget |
| Публикация | После каждого завершённого attempt обновить сводку, при новом head отметить старый результат stale |
| Reconciliation | Webhook триггерит import; периодическая перепроверка последних изменённых runs закрывает пропуски |

**Почему не достаточно финального upload step:** pipeline может оборваться до него, финализатор может не иметь reports предыдущих stages, cancelled job может не исполнить post-action. Поэтому API-import и завершение событий/артефактов дополняют друг друга. «Данных не получили» нельзя показывать как «проверок не было».

### Пример подключения в первом выпуске

Пользователь указывает GitLab instance и projects, получает список доступных capabilities и готовый snippet. Snippet ссылается на закреплённый digest collector, перечисляет пути к уже создаваемым отчётам и задаёт upload credential только для этого project. Новый quality/security scan подключается отдельным explicit profile.

Пример декларативного состава подключения; CI-snippet формируется под stages, доступ и артефакты проекта:

```yaml
craftpulse:
  schema: 1
  mode: advisory
  component: payments-api
  reports:
    junit: [artifacts/tests/*.xml]
    coverage: [artifacts/coverage/cobertura.xml]
    sarif: [artifacts/security/*.sarif]
  context:
    source_sha: supplied-by-provider
    pipeline_id: supplied-by-provider
    job_id: supplied-by-provider
    attempt: supplied-by-provider-or-unknown
  require_reports: [junit]
```

Реальная CI YAML зависит от stages, needs, retention и permissions клиента. Нельзя дать универсальный рабочий `needs: []` финализатор и обещать, что он скачает все артефакты. Поле `require_reports` означает договорённость данного профиля, не автоматическое требование для любого нового проекта.

## Семантика попыток и достоверность результата

Сущности: pipeline definition → run → logical job → attempt → test execution. Имена jobs не уникальны: добавляем pipeline/run, matrix, shard, workflow, config hash и environment fingerprint. Иначе `test:python3.11` и `test:python3.12` превращаются в ложный flaky signal.

**Наблюдаемая смена результата**: в цепочке попыток одной логической проверки на одном SHA есть fail и последующий pass. **Подтверждённая нестабильность test case**: один тест при сопоставимой конфигурации/входах имеет и pass, и fail. Один SHA недостаточен, если внешние artifacts, зависимости, seed или среда поменялись. У job мог упасть runner или registry, а не тест. Сохраняем failure reason и уровень уверенности.

Новые pipeline на том же SHA не объединяются автоматически: manual variables/nightly suites могут быть разными. При недостаточном контексте показываем candidate retry cluster и отдельную полноту. Не обвиняем сотрудника в «накрутке зелёного»: намерение из телеметрии не наблюдается.

Методическое основание для поведения теста на том же commit есть у [Datadog](../../research/sources.md#ref-063). Для CraftPulse дополняем его обязательным context fingerprint и различием test/infrastructure failure.

## Каталог метрик

### Надёжность CI

Формулы долей ниже ×100%. Срез по pipeline definition/component/branch class/source/config version; completion cohort определяется `finished_at` в окне, pipeline без terminal outcome в знаменатель завершённых не входит.

| ID | Метрика / формула | Источник / единица | Этап и решение |
|---|---|---|---|
| I01 | Финальная успешность: terminal success / (terminal success+failed+timeout+unstable по mapping) | К runs/jobs; % и n | M0. Canceled/skipped/manual separate; native status рядом |
| I02 | First-pass success: logical checks, прошедшие на первой попытке / checks с известным first outcome | К attempts; % и n | M0. Final outcome и полнота first attempts отдельно |
| I03 | Проверки со сменой fail→pass: comparable logical checks с такой цепочкой / comparable checks с complete attempts | К/С; % и n | M0. Называть observed retry flip, не confirmed flaky test |
| I04 | Retry overhead: число attempts сверх первой и их суммарное execution time | К attempts; count/job-min | M0. Неполная история даёт lower bound, не точный total |
| I05 | Success с разрешёнными падениями: successful pipeline runs с ≥1 финально failed allow_failure job / successful complete runs | К GitLab/jobs; % и n | M0. Ранее упавший, затем успешный attempt идёт в I03; не маскировать одним счётчиком |
| I06 | Распределение исходов: success/failure/timeout/canceled/skipped/manual/unstable/unknown | К; count/% каждого | M0. Нельзя удалить 80% cancellations и назвать оставшиеся 100% success здоровьем всего CI |
| I07 | Причины failure: infra/code/test/scan/policy/unknown по evidence provider | К+О; counts | M0 native reason, M1 классификация. Unknown не приписывать flaky tests |
| I08 | Missing required checks: обязательные logical checks без outcome / expected checks | Policy+run graph; count и % | M0 если expected checks задан. Без expected graph нельзя вычислить полный знаменатель |
| I09 | Missing/invalid reports: expected report scopes без валидного complete report | О manifest+policy; count/% | M0. Пустой валидный отчёт отличается от отсутствующего файла |
| I10 | Completeness источника: complete imported runs/known eligible runs, pagination/truncation и missing attempts | К import ledger; % и состояния | M0. Если total unknown - показываем count imported и bound вместо выдуманного % |

### Скорость, очереди и ресурсы

| ID | Метрика / формула | Источник / единица | Этап и решение |
|---|---|---|---|
| I11 | Wall duration run: finished−started; end-to-end latency: finished−created отдельно | К dates; сек p50/p85/p95 | M0. Разные definitions у providers храним явно |
| I12 | Runner queue: started−queued timestamp logical attempt при наличии | К; сек и n | M0 GitLab native queue; отсутствие queued timestamp не заменяется произвольным created_at |
| I13 | Execution consumption: sum attempt execution duration | К jobs; job-min | M0. Это расход параллельных jobs, не wall time и не CPU usage |
| I14 | Critical path: самый длинный зависимый путь workflow с учётом временной модели | К DAG+dates/О spans; сек | M1. Неполный graph → unknown; сумма всех jobs не critical path |
| I15 | Time to first actionable failure: first required failing result−pipeline created | К graph+outcomes; сек | M1. Observed failure feedback, не время прочтения человеком |
| I16 | Time to green: first required all-success state−logical run start, включая observed retries | К attempts+graph; сек и censored count | M1. Незавершённые серии не выбрасываются молча; manual pauses отдельно |
| I17 | Nightly duration/outcomes по suite/config cohort | К scheduled+О; сек/статусы | M0. Не смешивать семичасовой e2e с MR lint в общей средней |
| I18 | Расчётная стоимость: sum runtime units × ставка class, фиксированные fees отдельно | Billing/rates+I13; валюта | M2. При отсутствии ставки только job-min. Счёт поставщика и оценку различать |

### Данные тестов и покрытия

| ID | Метрика / формула | Источник / единица | Этап и ограничение |
|---|---|---|---|
| I19 | Outcomes tests: pass/fail/error/skip, distinct case и executions отдельно | О JUnit/native; count | M0. Один case в 8 matrix configs - 8 configurations, не 8 новых тестов |
| I20 | Test instability: distinct cases с mixed outcomes в comparable commit+env / cases с достаточными сопоставимыми executions | О attempts+identity; % и n | M0 basic candidate, M1 enriched confidence. Summary JUnit без attempts даёт только observed executions |
| I21 | Test fail rate: fail/error executions / non-skipped executions данного stable case/config | О; % и n | M0. Рядом retry bias и count independent runs |
| I22 | Test duration tail: p50/p95/max по стабильному case/suite/config | О; сек | M0. Малое n явно; суммарное время тестов не wall time suite |
| I23 | Stable-suite progress: outcomes на пересечении известных test IDs двух сравнимых suite versions; added/removed отдельно | О+С; count/% | M1. Не выдавать рост pass count после удаления сложных тестов за улучшение |
| I24 | Quarantined/skipped debt: active excluded cases, age и expected suite share | О+policy/runner metadata; count/дни/% | M1. Из `<skipped>` нельзя надёжно вывести причину quarantine |
| I25 | Line coverage: covered executable lines / executable lines в report scope | О Cobertura/LCOV/JaCoCo; % и lines | M0. Общая формула не стирает producer semantics |
| I26 | Branch coverage: covered branches / reported total branches | О; % и branches | M0 если источник даёт ветви. Line coverage не branch coverage |
| I27 | Patch coverage: покрытые изменённые executable lines / изменённые executable lines с mapping | О+Git diff; % и lines | M0 при данных. Та же сущность, что C28: это один metric ID-alias, не две независимые оценки |
| I28 | Coverage data reach: файлы/строки с coverage mapping относительно eligible scope | О+inventory; % и n | M0. Неподдержанный язык не получает coverage 0 |
| I29 | Ресурс перезапусков из-за test failures: observed rerun time с подтверждённой test failure причиной | К+О; job-min | M1. Не всё время упавшей job - доказанная стоимость flaky tests |
| I30 | Test-to-code map availability: cases с инструментированными touched files / known cases | Native instrumentation; % и n | M2. Нужен для test selection; JUnit/общий line coverage недостаточны |

### Конфигурация и политика CI

| ID | Метрика / формула | Источник / единица | Этап и ограничение |
|---|---|---|---|
| I31 | Валидность конфигурации: errors/warnings server lint/parser на текущем config revision | Ш/API lint; count/status | M0 GitLab; M1 GitHub. Includes/variables требуют server context |
| I32 | Потенциально опасные CI patterns: findings по curated rules, например privilege+untrusted code | Ш actionlint/правила; count | M1. Не каждый pull_request_target опасен; проверяется комбинация действий |
| I33 | Неизменяемость внешних steps/images: pinned immutable references / eligible external references | Ш config parser; % и n | M1. Docker tag и action tag не digest/SHA; local refs отдельно |
| I34 | Соответствие required policy: runs с полным evidence обязательных checks / runs с применимой policy | Policy+manifest; % и n | M0. Missing evidence не success; waived отдельно |
| I35 | Overhead CraftPulse: duration/run consumption своих jobs отдельно от исходного pipeline | К+Ш; сек/job-min/p95 | M0. Нельзя продавать ускорение CI, не считая свой добавленный расход |
| I36 | Artifact linkage: complete builds с digest+source SHA / известные release builds | О artifact manifest; % и n | M1. Название artifact.zip не immutable identity |

### Доставка и расширение DORA

Метрики I37–I43 относятся к **service+production environment**, а не репозиторию вообще. У multi-repo сервиса нужен единый release identity; multi-region rollout одного релиза не считается N независимыми выпусками. Повторный technical deploy и unplanned rework имеют отдельные признаки.

| ID | Метрика / формула | Источник / единица | Этап и ограничение |
|---|---|---|---|
| I37 | Deployment frequency: count successful production deployment events / длительность окна | CD events; deploy/day и total | M1. Release tags и CI builds не заменяют deploy |
| I38 | Change lead time: first successful prod deployment containing change−commit time, по unique changes | Git+build+CD; часы p50/p85 | M1. Commit mapping/clock/squash правила видны; undeployed changes показываются отдельно |
| I39 | Change fail rate: prod deployments, вызвавшие failure с необходимым вмешательством / eligible prod deployments | CD+incident/failure relation; % и n | M2. Failed pre-prod pipeline не production change failure; relation подтверждается человеком/источником |
| I40 | Failed deployment recovery time: recovery time−failure onset для deployment-caused failures | CD+incident/recovery; часы p50/p85 | M2. Не общий MTTR всех инцидентов; timestamps и связь обязательны |
| I41 | Deployment rework rate: unplanned prod deployments вследствие production incident / all eligible prod deployments | CD+incident+reason; % и n | M2. Новое актуальное пятое измерение DORA; hotfix label сам по себе кандидат, не доказательство |
| I42 | Rollback rate: releases с подтверждённым rollback / releases с известным rollout outcome | CD; % и n | M1. Не синоним I39: failure может устраняться без rollback |
| I43 | Verified delivery evidence: prod releases с digest/source/checks linkage / known releases | С code+security+CI+CD; % и n | M1. Сводка evidence, не новый health score |
| I44 | Проверки до production: released digests с required scan на этом digest/SHA до deploy / applicable releases | С scans+CD+policy; % и n | M1. Новый scan после релиза не делает прошлый release gate пройденным |
| I45 | Waivers/bypasses при релизе: решения с исключением и без авторизованного исключения раздельно | Policy+audit+CD; count | M1. Это те же факты, что S40; не суммировать как второй риск |
| I46 | CD observation completeness: known deployments с outcome, change mapping, incident mapping - три независимых доли | Import ledger; % и n | M1. Отсутствие incident connector не означает CFR=0 |

Текущий DORA guide описывает пять метрик: [DORA delivery performance](../../research/sources.md#ref-061). Здесь определения операционализированы для CraftPulse; выбранный cohort/окно и единицы фиксируются в metric version. **Task cycle time из трекера не равен change lead time до production**. DORA-панель подключается по capability и необходимости клиента, не вытесняет первичный запрос на честный CI.

## Что заимствуем у сильных внешних решений

| Подход | Где он уже есть | Что берём в CraftPulse | Что не обещаем |
|---|---|---|---|
| Flaky lifecycle и влияние на CI | Datadog Test Optimization | Стабильная test identity, first/last observed, impact и подтверждение исправления | Не утверждаем, что flaky detection уникален; не обещаем все возможности SDK через один JUnit |
| Patch coverage | Codecov | Покрытие изменённых executable lines рядом с MR | Не заменяем patch coverage общей средней проекта |
| Готовая Git/Jira/CI аналитика | Apache DevLake | Нормализованный catalogue и reusable calculations | Не заявляем отсутствие бесплатных агрегаторов |
| Code Quality report в MR | GitLab | Совместимость формата и использование штатного pipeline | Сам MR-комментарий не дифференциатор |
| CI entities/spans | OpenTelemetry | Понятные run/task/worker IDs и возможность общего ingest | Не считаем каждую RC-конвенцию стабильной навсегда |
| Verified artifact provenance | SLSA/Sigstore | Проверяемое происхождение конкретного digest и trust policy | Наличие attestation не сертификат безопасности |

Ссылки: [Datadog flaky management](../../research/sources.md#ref-064), [test impact limitations](../../research/sources.md#ref-066), [GitLab Code Quality](../../research/sources.md#ref-042), [DevLake sources](../../research/sources.md#ref-070). GitLab предупреждает о deprecated CodeClimate scan template; используем прямой запуск инструментов с импортом report, а не зависимость от этого старого шаблона.

## Пользовательские сценарии

### «Зелёный, но с оговорками» - M0

Карточка показывает четыре независимых факта: final outcome, first-pass outcome, allowed failures, completeness required evidence. Пример искусственный: pipeline success, test job fail→fail→pass на том же SHA, один allowed security job failed, JUnit отсутствует. Вывод: есть observed нестабильность проверки и неполный test evidence; какой именно тест виноват, неизвестно.

Ценность - закрывает конкретный случай Хаверко без обещания магически найти баг статическим анализом. Проверка на пилоте: подтвердить цепочку attempts по исходному API и получить решение техлида, не открывая логи каждой job.

### Карта потерянного времени - M1

Соединяем queue time, wall time, DAG, retries и suite duration. Выдаём конкретные наблюдения: «очередь runner X даёт 40% observed end-to-end latency» либо «job Y часто перезапускается». Проценты рассчитываются только на определённом cohort; parallel queues и jobs не суммируются как последовательное ожидание.

Рекомендация увеличить parallelism не обещает линейного ускорения: нужны concurrency limits, bottlenecks и пилотное измерение. Экономию человеко-часов без наблюдения работы людей не показываем.

### Устойчивая картина ночных прогонов - M0/M1

Для большого e2e suite: stable IDs, исходы, новые/удалённые/skipped tests, нестабильность и duration tail, отдельно версия suite и environment. Отвечаем на запрос Дениса энеса о прогрессе тестов, не подменяя его ростом числа кейсов после изменения matrix. Связь результатов тестов с готовностью фичи определяется критериями DoD и назначением suite.

### Пропавшая проверка - M0

Если ранее обязательный test/security job перестал запускаться из-за rules/paths или report не доехал, показываем изменение evidence. Expected graph/required policy нужны явно: отсутствие строки JUnit не доказывает злонамеренное отключение тестов.

### Паспорт доставки - M1

Из MR → source SHA → build → artifact digest → security/code checks → deployment → known incidents собирается проверяемая временная линия. Отдельно видно, что результаты сканирования относились к тому же digest и существовали до release decision. Эту же сущность используют безопасность и блок качества выпуска.

### Выбор тестов под изменение - M2, эксперимент

Можно рекомендовать impacted tests только после построения per-test code map/надёжного dependency graph. Сначала shadow mode: рекомендации сравниваются с полным прогоном, замеряются missed failures. Автоматическое skipping не M0-default; безопасная политика fallback - полный прогон. Простое совпадение имени теста и файла недостаточно.

## План выпуска и приёмка

**M0:** GitLab collector, generic upload, поддержанные JUnit/coverage parsers, complete attempt history, outcomes/cohorts, observed retry flips, allow_failure, missing reports, MR advisory и native SourceCraft preservation. **M1:** GitHub/Jenkins/российские CI по пилотам, очередь/DAG, устойчивое test identity, artifact/deploy events, основные delivery показатели при полных данных. **M2:** incident-aware пять DORA метрик полностью, billing и shadow test selection.

Приёмочный набор: pipeline с 3 attempts и последним pass; matrix с одинаковым job name; failed allowed job; canceled финальный шаг; pipeline без JUnit; пустой валидный JUnit; job infra failure без test failures; child pipeline; artifacts с другого SHA; expired artifact; неполная пагинация; workflow/nightly разной длины; одинаковый digest в разных tags; одно multi-region rollout; новая CVE после релиза; failed deploy и отдельный production incident. Для каждого ожидаются правильные числитель/знаменатель, data state и MR текст.

Операционные критерии: идемпотентный повтор webhook/report, восстановление пропущенных событий reconciliation, rollback версии parser без потери raw sanitized facts, отсутствие записи токенов/секретов, отдельный queue/execution overhead CraftPulse. Полезность проверяется действиями техлида по скрытым failures и временем разбора pipeline, а не обещанием «мы всегда повышаем DORA».
