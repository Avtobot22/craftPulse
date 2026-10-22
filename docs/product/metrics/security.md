# Метрики безопасности

Блок поддерживает импорт существующих результатов и штатные проверки в выбранном CI/runner. Source of truth для triage, версии правил и область проверки сохраняются. Общие определения заданы в [модели измерения](model.md).

## Какую задачу закрываем

Команда получает базовую проверку безопасности без самостоятельной сборки нескольких сервисов либо подключает уже купленные инструменты. В обоих случаях CraftPulse отвечает: что найдено, в каком артефакте и версии, что новое, что действительно проверено, почему находка приоритетна, кто принимает решение и проверено ли исправление.

Штатный продукт не обещает отсутствие уязвимостей, полного покрытия OWASP/CWE или замену AppSec-специалиста. Предлагается конкретный управляемый набор SAST+SCA+secrets+IaC/container checks. Supply-chain evidence и эксплуатационная экспозиция появляются только при подключении соответствующих источников.

Безопасность выделена в отдельный продуктовый блок по разделению работы, хотя в `categories.md` она была подвидом findings. Общие сущности findings, scans, exceptions и policy сохраняются; отдельная страница безопасности не означает второй несовместимый движок.

## Области проверки и входные данные

| Область | Что проверяем | Основные факты | Что не покрывает |
|---|---|---|---|
| **SAST** | Исходный код по правилам | Rule/CWE, location, data flow при наличии, language, confidence, commit | Полноту runtime-путей, бизнес-логику и production exposure |
| **SCA** | Разрешённые зависимости и их версии | PURL/ecosystem/name/version, dependency path, direct/transitive, advisory aliases, affected/fixed range | Библиотеки без определяемой версии; реальную достижимость без дополнительного анализа |
| **Secrets** | Дерево, изменения и отдельно история Git | Detector/rule, redacted location, fingerprint, первый видимый commit | Факт действительности/отзыва секрета без разрешённой проверки провайдера |
| **Контейнеры** | Конкретный образ по digest | OS/language packages, base image, CVE, фиксированная версия, layer/source metadata при наличии | То, что именно этот образ запущен в production, без deployment inventory |
| **IaC/config** | Dockerfile, Kubernetes/Helm, Terraform и др. в поддержанном формате | Misconfiguration rule, resource, location, severity | Реальное состояние кластера и итоговые значения всех шаблонов без rendering context |
| **SBOM/лицензии** | Состав конкретного артефакта | Components, relationships, hashes, licenses, evidence | Автоматическое юридическое заключение о совместимости всей поставки |
| **Build integrity** | Подпись, attestation и provenance | Artifact digest, issuer/identity, builder, materials, verification result | Сертификацию SLSA или безопасность самого приложения |
| **DAST** | Развёрнутый тестовый target | URL/route, authenticated context, request/evidence, scanner scope | Проверку из Git-клона без запуска приложения |

## Коннекторы к сервисам клиента

### AppSec и vulnerability management

| Сервис | Путь подключения | Что собираем | Приоритет и ограничения |
|---|---|---|---|
| **SourceCraft Security / AppSec** | Существующий отдельный AppSec API адаптер с PAT владельца | Scan ID/SHA/status, engine, группы находок, severity, first found, CVSS при наличии | **M0: сохранить и улучшить**. В текущем адаптере нет полноценной доказанной file/package coverage; не выдавать enabled engine за complete scan |
| **GitLab SAST / dependency / secret / container reports** | JSON security artifacts из CI, не обязательный платный vulnerability dashboard | Findings, scanner/rules, identifiers, locations/packages, scan status и commit | **M0 ingest**. JSON-схемы security reports отличаются от SARIF; версия parser обязательна. Direct vulnerability API - позже с проверкой редакции и прав |
| **GitHub Code Security / code scanning**, также результаты CodeQL клиента | Code scanning API или SARIF из CI | Alerts/analysis/rule/state/location, dismissal metadata при наличии | **M1 direct**, **M0 SARIF**. Для private repositories нужны соответствующие возможности/права, а не один read Git-token |
| **SonarQube** | Тот же connector, что в блоке кода | Security issues, hotspots/review state, rules и analysis revision | **M0**. Security hotspot требует review и не равен доказанной vulnerability. Community/Server editions не объявляются одинаковыми |
| **CodeScoring SCA** | CycloneDX/SPDX export, Johnny output; API по контракту tenant | Компоненты, версии, зависимости, vulnerabilities/policies и reachability properties, если реально переданы | **M1**. SBOM без vulnerabilities импортируется как inventory, не как «0 уязвимостей». Не обещаем SARIF для всех данных |
| **Solar appScreener** | SARIF/JSON export через API клиента | SAST/SCA findings в имеющейся версии, статусы при их наличии | **M0 SARIF**, **M1 API по пилоту**. Официальное руководство подтверждает экспорт, но не любой endpoint/редакцию |
| **PVS-Studio** | SARIF converter | Правила, CWE, source location и severity | **M0 SARIF**; correctness и security классифицировать по rule metadata, не по имени сканера |
| **Semgrep AppSec** | SARIF/JSON из pipeline; direct API после проверки | Findings, dataflow/reachability/triage, доступные клиенту | **M0 отчёт / M2 API**. Не перепаковывать платные правила и engine capabilities |
| **Snyk** | Клиентский JSON/SARIF export в поддержанном продукте, API scoped project | SAST/SCA/container/IaC findings с origins | **M1 адаптер отчёта, M2 API**. Форматы различаются между продуктами; считать candidate до fixture/live acceptance |
| **PT Application Inspector** | API/экспорт конкретного инстанса | Security findings и lifecycle | **M2 discovery**. Контракт полей проверяется при подключении; интеграция не входит в M0 |
| **DefectDojo** | API import источника triage/findings | Finding identity, duplicates, accepted/FP, dates, tests/engagements | **M1**. Если это source of truth для triage, CraftPulse не перезаписывает статусы сам |
| **OWASP Dependency-Track** | CycloneDX и API клиента | Projects/versions, components, vulnerabilities, policy и suppression | **M1**. Нужны versioned project coordinates и связь с artifact digest |
| **Внутренние анализаторы** | SARIF 2.1.0, security JSON adapter или нормализованный finding envelope | Findings и scan manifest | **M0 generic**. Без сведений о завершении scan отсутствие findings не означает чистый scope |

Проверенные основания: [SourceCraft API](../../research/sources.md#ref-050), [GitHub scanning API](../../research/sources.md#ref-046), [CodeScoring export](../../research/sources.md#ref-028), [Solar руководство](../../research/sources.md#ref-030), [DefectDojo](../../research/sources.md#ref-032). Для candidate-коннекторов отсутствие проверки отмечено явно; полный marketing feature set поставщика не приписывается CraftPulse.

**GitLab CE и форматы безопасности:** приём CraftPulse security JSON/SARIF не требует покупки GitLab Ultimate. Файлы можно передать обычными `artifacts:paths` и нашим collector/upload. Возможность GitLab самостоятельно обработать `reports:sast`, `dependency_scanning` и показать security widgets - отдельная тарифная возможность. Нельзя обещать клиенту CE нативный Ultimate dashboard только за счёт загрузки файла. Кроме того, GitLab различает загрузку артефакта и ingestion findings: неуспешная producing job может оставить файл, но не дать findings в нативном интерфейсе. Поэтому фиксируем отдельно scanner completion, findings и policy verdict. См. [официальные artifacts reports](../../research/sources.md#ref-071).

### Реестры артефактов и внешние базы

| Источник | Подключение и данные | Что становится доступно | Этап |
|---|---|---|---|
| **OCI Registry**: Harbor, GitLab Registry, GHCR, Docker-compatible private registry, Yandex Container Registry | Read-only pull конкретного digest внутри контура; credentials и CA поставщика | Container inventory/scan, digest linkage, позже SBOM и signature attachment | **M1**. M0 - image scan в CI клиента и импорт отчёта. Наличие OCI не обещает одинаковых tag/metadata/security API |
| **Trivy vulnerability DB / OSV-compatible данные** | Локальное зеркало и pinned DB snapshot | SCA matching без отправки списка private packages наружу | **M0** для Trivy; OSV как отдельный M1 enrichment, с явной дедупликацией aliases |
| **FIRST EPSS** | Кэш официального feed/API по CVE и дате | Дополнительный threat signal | **M1**. Это оценка вероятности эксплуатации CVE, не риск конкретного сервиса |
| **CISA KEV** | Кэш JSON/feed с датой | Факт включения CVE в каталог известных эксплуатируемых | **M1**. Отсутствие CVE в KEV не доказывает отсутствия эксплуатации |
| **Deployment inventory / CMDB** | Контракт из блока CI/CD и команды клиента | Где реально развёрнут vulnerable digest, exposure и criticality | **M1/M2**. Нельзя угадывать internet exposure по имени сервиса |
| **VEX клиента/поставщика** | CycloneDX VEX или поддержанный отдельный формат | Affected/not affected/fixed с контекстом и автором | **M1**. Проверяем продукт/version/digest и срок действия; это не глобальное удаление CVE |

## Штатный комплект CraftPulse

### Штатный стек по умолчанию

| Назначение | Инструмент | Режим M0 | Какую работу делает CraftPulse |
|---|---|---|---|
| SAST | **Opengrep**, LGPL-2.1 engine | Поддержанные Go/Python/JS/TS rules; full baseline и delta MR | Собственный небольшой проверенный rule pack с доказанным происхождением; severity/confidence, test cases, packaging и baseline |
| SCA | **Trivy**, Apache-2.0 | Filesystem/lockfile scan, версии и advisories; отделить runtime/dev dependencies где данные есть | Mirror DB, metadata, component identity, new-dependency delta и повторная оценка inventory |
| Secrets | **Gitleaks CLI**, MIT | Head/MR diff и отдельный initial history scan | Redaction до передачи, secret-safe fingerprint, exclusions/triage, прозрачный depth истории |
| IaC/config | **Trivy misconfiguration** | Только поддержанные обнаруженные конфиги; включается явно профилем, не «случайно по defaults Trivy» | Версионированный набор проверок, resource mapping и ограничения templating |
| Контейнеры | **Trivy image scan** | В CI клиента при переданном immutable digest; не server build | Связь source→build→digest, import и предотвращение сканирования другого тега |
| SBOM | **Trivy CycloneDX export**, далее импорт SPDX | Inventory version/digest и компоненты | Повторная оценка по новой DB без пересборки там, где SBOM достаточно полон |
| Supply chain | Cosign verification, provenance parsing | Не обязательный M0, **M1** | Trust policy с issuer/identity; отчёт о проверке, а не о наличии JSON |

[Opengrep](../../research/sources.md#ref-014), [Trivy](../../research/sources.md#ref-018), [Gitleaks](../../research/sources.md#ref-020) подтверждают базовые возможности движков. **OSV-Scanner** - M1 вариант, если его поддержка экосистемы/возможности дают пользу; не запускаем два SCA по умолчанию и не суммируем CVE. DAST (например, подключаемый ZAP) - отдельный opt-in M2 профиль для test environment, с явно заданными target и доступом. Он не включён в обещание «подключил репозиторий - всё просканировано».

### Правила, версии и лицензии

Ключевой риск комплекта - не сам executable, а **rule pack**. Semgrep Registry нельзя целиком включить в наш competing service под предлогом LGPL-движка: [лицензия правил](../../research/sources.md#ref-016) ограничивает такое использование. Opengrep rules repository - кандидат, происхождение/лицензия **каждого включаемого правила на выбранном commit** проверяется; название fork не даёт автоматического разрешения на всё содержимое.

Для M0 используем небольшой **CraftPulse starter pack**: явные dangerous API/конструкции с низкой неоднозначностью и поддержкой трёх языков. Возможные категории: прямое выполнение недоверенных shell/SQL-конструкций, опасная десериализация, отключённая TLS verification, известные небезопасные crypto/API usages. Конкретный rule включается только после позитивных и негативных fixtures, проверки dataflow-возможностей движка и разрешённых исходников. Формулировка «SQL injection найдена» допустима только при достаточном evidence; совпадение строки с шаблоном само по себе - подозрение.

Не обещаем тысячи правил, межфайловую достижимость и enterprise taint analysis без отдельной реализации. Список rules и поддержанных frameworks публикуется как часть manifest. Если разрешённый starter pack не готов, SAST отмечен unavailable, а не «проверен»; при этом SCA/secrets/IaC работают самостоятельно.

**CodeQL** оставляем инструментом клиента с его лицензией. **Gitleaks CLI** и коммерческая GitHub Action-обвязка - не одна лицензия; в своём CI-комплекте используем CLI и собственный launcher. Раздельно фиксируются engine license, rules, vulnerability DB, license database и dependencies. Перечень источников лицензий - [источники методики](../../research/sources.md).

### Политика первого подключения

1. Выбираем import-existing или managed-baseline; гибрид разрешён по scope. Existing security-policy остаётся у клиента, автоматически не меняется.
2. Security inventory включает tests/examples/configs/lockfiles там, где это важно для утечек и поставки. Исключения quality scope не переносятся в security.
3. Initial full scan main и inventory; history secrets выполняется отдельным заданием с лимитом, показанным пользователю. Старые находки составляют baseline, не приписываются первому MR.
4. MR scan сопоставляется с тем же профилем base. Dependency scan повторяется при изменении relevant manifest/lock или DB, результаты кешируются по content+DB+rules.
5. Ежедневно предлагаем refresh vulnerability DB и повторную оценку актуального inventory. В offline-контуре источник обновлений задаёт клиент; время DB видно.
6. На старте **advisory**. Категории потенциальных блокировок предложены, но включаются владельцем policy: новый подтверждённый секрет, actionable high/critical SAST, запрещённая зависимость и т.д.
7. Ограниченный результат обозначается partial/unknown. Неуспешный scan не снимает последнюю находку и не даёт новый «чистый» статус.

## Каталог метрик

### Наблюдение и покрытие

| ID | Метрика / точное определение | Источники / единица | Этап, смысл и предел |
|---|---|---|---|
| S01 | Матрица проверок: последний complete run по component×SHA/digest×scan kind×profile | Ш/О/К manifest; состояния и время | M0. Отдельно SAST/SCA/secrets/IaC/image; ни одно «3/3» не доказывает покрытие всех файлов |
| S02 | SAST file coverage: successfully analyzed eligible supported files / eligible files соответствующего языка | Ш/О invocation/scope; % и files | M0, если producer знает denominator; иначе неизвестно |
| S03 | Dependency resolution: packages с точной resolved version / обнаруженные dependency entries | Ш SCA/О SBOM; % и n | M0. Диапазон версии в manifest не точная версия |
| S04 | История secrets: фактически проверенный commit/time range и truncated flag | Ш Git scanner; commits/даты | M0. Shallow scan не полный historical scan |
| S05 | Свежесть DB: now−published_at snapshot используемой vulnerability DB | Ш mirror; часы/дни | M0. DB stale не означает автоматического отсутствия доверия ко всем прошлым finds |
| S06 | Доля актуально просканированных artifacts: current production digests с complete scan / все известные production digests | С CD+scan; % и n | M1. Без runtime/deployment inventory нет знаменателя |
| S07 | Успешность security jobs: completed successful executions / requested executions; scan errors отдельно | К/Ш; % и n | M0. Exit code findings отличается от operational error |
| S08 | Scan freshness per scope: now−last complete scan | Ш/О/К; часы | M0. Для повторной SCA-оценки ещё фиксируется DB snapshot |

### Находки SAST, SCA, secrets и configs

Сначала считаем **occurrence** `(component, branch/artifact, canonical finding identity)`; затем отдельно число уникальных advisories/правил. В организации одна CVE в десяти сервисах - одна CVE, но десять затронутых сервисов. Образы с одинаковым digest можно хранить как один артефакт с несколькими deployment-связями.

| ID | Метрика / точное определение | Источник / единица | Этап и ограничение |
|---|---|---|---|
| S09 | Открытые actionable security occurrences по kind/native severity | Findings с lifecycle; count | M0. Accepted/FP не исчезают, показываются отдельным слоем |
| S10 | Новые code/dependency findings MR относительно comparable base | С base/head; count | M0. Новый advisory для старого кода - отдельная причина, не внесённая MR проблема |
| S11 | Устранённые находки: больше не воспроизводятся в complete сопоставимом scan | Scan+lifecycle; count | M0. Missing report/removed scanner не является fix |
| S12 | SAST findings по CWE/rule с native confidence | Ш/О/К; count | M0. SARIF level error/warning не напрямую CVSS/high/critical |
| S13 | Affected package instances: уникальные package version+dependency context с ≥1 affected advisory | Ш SCA/О; packages | M0. Отдельно runtime/dev и direct/transitive, если источник знает |
| S14 | Уникальные advisories: объединение CVE/GHSA/OSV aliases по подтверждённым связям | Enrichment; count | M0 базовый canonical ID, M1 полный alias graph; совпадение текста не основание объединения |
| S15 | Fixable findings: affected instances с определённой исправленной версией / affected instances с известной fix metadata | SCA DB; % и n | M0. Отдельно доля unknown и no-fix; fixed version не гарантирует совместимость upgrade |
| S16 | Проблемы новых зависимостей: introduced affected package instances и policy violations | С lock diff+SCA; count | M0. Различать dependency update и DB update |
| S17 | Уязвимости образа по immutable digest: unique package+advisory occurrences | Image scan; count | M0 при импорте/CI image scan. Не сравнивать плавающие tags |
| S18 | Небезопасные конфигурации: active rule+resource occurrences по supported IaC scope | Trivy/О; count | M0. Учитывать version/rendering context |
| S19 | Потенциальные секреты: unique redacted detector occurrences в head и истории раздельно | Gitleaks/О; count | M0. Не называем действующими credentials без проверки |
| S20 | Подтверждённые/отозванные секреты: explicit provider/operator verification states | Triage / опциональный verifier; count | M1. Удаление строки не доказывает отзыв; сеть для проверки по умолчанию выключена |

### Контекст приоритета и состав поставки

| ID | Метрика / определение | Источник / единица | Этап и ограничение |
|---|---|---|---|
| S21 | KEV-affected occurrences: active SCA instances с CVE в feed | CISA+SCA; count сервисов/occurrences | M1. Дата feed и coverage aliases обязательны |
| S22 | EPSS distribution и count выше threshold в текущем snapshot | FIRST+SCA; probability/% и count | M1. Не умножать на CVSS и называть итог вероятностью взлома |
| S23 | Reachability states: reachable / not_reachable / unknown, по доказательству producer | CodeScoring/другой source; count и доля известных | M1. Unknown не сортируется как безопасное; версия анализа важна |
| S24 | Exposure states: public/internal/unknown для affected deployments | CMDB/runtime mapping; count | M2. Репозиторий public/private не exposure приложения |
| S25 | Affected services: components/services с ≥1 actionable finding в известном deployed digest | С CD inventory; count и покрытие | M1/M2. Security backlog репозитория без CD показывается отдельно |
| S26 | SBOM coverage: known released digests с валидным привязанным SBOM / known released digests | С artifacts+SBOM; % и n | M1. Валидный schema SBOM не доказывает полноту перечня |
| S27 | Unknown licenses: components без распознанного license evidence / components inventory | SBOM/SCA; % и n | M1. Отсутствие поля не означает запрет лицензии |
| S28 | License policy violations: component occurrences, не удовлетворяющие policy клиента | Ш policy+license evidence; count | M1. Результат конкретной policy, не юридическое заключение |
| S29 | Signature verification coverage: released digests с успешно проверенной подписью / digests, для которых policy требует подпись | Cosign+trust policy; % и n | M1. Identity/issuer проверяются, просто attachment не зачёт |
| S30 | Provenance verification coverage: artifacts с подтверждёнными builder/materials/digest claims / требуемые artifacts | Attestation+policy; % и n | M2. Нельзя автоматически присваивать SLSA level по одному файлу |

### Триаж, долг и результат исправления

| ID | Метрика / определение | Источник / единица | Этап и ограничение |
|---|---|---|---|
| S31 | Возраст открытых actionable findings: now−first observed по severity/kind, p50/p90 | Lifecycle; дни | M0. First observed vs imported first seen различаются |
| S32 | Время исправления: verified fix time−first observed, по закрытому cohort | Lifecycle; дни p50/p85 и n | M1. Рядом возраст незакрытого cohort, иначе survivorship bias |
| S33 | Время триажа: first human decision−first observed | Triage audit; часы/дни | M1. Автоматический статус import не human decision |
| S34 | Просрочка SLA: active actionable старше policy target / active actionable с применимым SLA | Policy+lifecycle; % и n | M1. Время отклонений/pauses и severity change по versioned policy |
| S35 | Active exceptions: FP/accepted/not affected, без причины, без expiry и истёкшие раздельно | Triage/VEX; count | M0. Исключение строго scoped, без глобального подавления CWE |
| S36 | Reopened findings: previously verified fixed, again observed with same identity | Lifecycle; count | M1. Новая версия rule не обязана означать возврат дефекта |
| S37 | Triage disposition: FP/accepted/confirmed среди обработанных findings | Triage; % и n | M1. Это распределение решений, не истинная precision сканера; нужен независимый sample review |
| S38 | Новые находки из новых данных: DB/rules/scope change при неизменном artifact | С snapshot diff; count | M0 причина изменений. Не называть ухудшением кода |
| S39 | Неполнота приоритизации: findings без CVSS/EPSS/reachability/exposure по полям | Evidence; % missing | M1. Не заменять неизвестное нулём в risk formula |
| S40 | Security gate bypass: release decisions при failing required checks, с автором/основанием | С policy+CD; count и % подходящих release decisions | M1. Accepted waiver и несанкционированный bypass различаются |

## Нормализация, триаж и приоритет

### Общая модель находки

Храним `native_id`, `canonical_id`, scanner+version, rule ID, native severity, normalized severity+mapping version, confidence, kind, SHA/digest, package coordinates, location, first observed/source first seen, evidence references, lifecycle и source triage. В source occurrence сохраняется оригинальный статус; локальное решение CraftPulse существует отдельным слоем.

**SARIF** описывает статические результаты, а не весь security inventory. SBOM, VEX и scan manifest обрабатываются отдельными сущностями. SARIF warning может быть стилем или серьёзной проблемой - перевод в severity определяется правилом и metadata. [OASIS SARIF](../../research/sources.md#ref-026).

Дедупликация сначала внутри одного producer по устойчивому fingerprint. Между сканерами нужна проверенная rule/CWE+location/package mapping; при сомнении делаем related findings, а не необратимое слияние. Findings в нескольких branches не закрываются глобально, если исправлен только один branch.

### Порядок приоритета без магического security score

M0: группы **немедленно разобрать → запланировать исправление → наблюдать/триаж**. Внутри видны native severity, confidence, fix availability и возраст. Потенциальный секрет получает отдельный путь «проверить и отозвать при подтверждении», а не просто «удалить строку».

В M1 добавляем объяснимый лексикографический порядок: подтверждённая эксплуатация/KEV и известная экспозиция; затем applicable high/critical с fix и reachability; затем остальные actionable; unknown context остаётся явным неизвестным. Каждая причина видна отдельным фактором. Это policy продукта, не математически откалиброванная вероятность ущерба.

Прототипная `SECURITY-REMEDIATION-PRIORITY-1` использует веса, но текущий AppSec adapter не получает EPSS, KEV, reachability, exposure и fixability. Нельзя показывать его как полноценную threat-aware prioritization. Для нового продукта или показываем доступные факторы без score, или отдельно валидируем новую версию метода.

### Исключения

Решения: confirmed, false_positive, accepted_risk, not_affected (VEX), fixed_pending_verification, verified_fixed. У каждого решения actor, причина, scope, время, при необходимости reviewer и expiry. Source FP не превращается в новое локальное FP без сохранения ссылки. «Риск оправдан» и «ошибка детектора» - разные причины.

Удаление файла, смена parser или отказ в доступе не closes finding. Для секрета дополнительно показываем remediation steps: rotate/revoke, убрать из дерева, оценить историю и потребителей; автоматическое переписывание Git-истории не выполняется CraftPulse по умолчанию.

## Безопасная работа штатного комплекта

Рекомендуемый основной режим - **сканирование внутри CI/агента клиента**: исходники, registry credentials и raw secrets остаются в его контуре; CraftPulse получает минимальный redacted report. Центральный orchestration server не строит чужой Dockerfile и не запускает repo tests для security scan.

В runner: read-only checkout, отдельный temp/output, non-root, без Docker socket, лимиты CPU/RAM/disk/time, запрет egress кроме контролируемого mirror при обновлении. Config/plugin code не исполняется в серверной зоне доверия. Pull недоверенного образа для анализа не означает запуск контейнера с его entrypoint.

Маскируем и stdout, и report fields. У Gitleaks флаг redaction документирован прежде всего для logs/stdout; **дополнительная санация JSON/SARIF перед upload обязательна**. Удаляем raw secret/snippet/commandline/env, кроме явно разрешённого безопасного контекста. Для идентичности secret используем tenant-scoped HMAC, не публичный SHA сырого короткого пароля. Доступ к security evidence ограничивается project scope и ролью; ссылки на sources не должны обходить это ограничение.

Предлагаемый pilot default: metadata/history хранить 12 месяцев, сырые очищенные отчёты 30 дней с настройкой клиента; raw secrets не сохранять. Это проектные параметры, требующие оценки storage и требований клиента. Offline DB older than 7 days - предупреждение пилотного профиля, а не универсальное правило непригодности скана.

## Пользовательские сценарии

| Фича | Состав данных и результат | Ценность / честная граница | Этап |
|---|---|---|---|
| **Защита с первого дня** | Управляемые SCA+secrets+SAST starter+IaC, inventory и MR delta | Команда без AppSec получает законченный путь; качество определяется проверенным pack, не числом движков | M0 |
| **Новая CVE без нового коммита** | Сохранённый SBOM/digest + обновлённая DB | Сообщить, какие известные версии теперь affected, без повторного clone/build; требуется достаточный inventory | M1 |
| **Исправление с проверкой** | Finding→подготовленная задача→fix MR→complete rescan→verified fixed | Видно, что проблема исчезла из сопоставимого scope; запись статуса в трекере сама не доказательство fix | M1 |
| **Один upgrade закрывает несколько проблем** | Dependency graph + advisories + fixed ranges + доступные CI результаты | Сгруппировать remediation по обновлению компонента; совместимость/безопасность upgrade не гарантируется | M1/M2 |
| **Паспорт безопасности релиза** | Artifact digest + scan coverage + exceptions + signature/provenance + CD | Можно проверить, что именно выпущено и какие проверки реально были, а не приложить скан другой версии | M1 |
| **История управляемого исключения** | External triage + local policy + expiry + current finding | Не повторять ложные тревоги и не терять принятые риски | M0 базово, M1 federation |
| **Риск production, а не всего backlog** | CVE + package+digest + deployment+exposure | Выделить реально затронутые сервисы; без deployment map остаётся repository risk | M2 |

Подобные механики уже есть в AppSec-платформах и [Dependency-Track](../../research/sources.md#ref-035). Предполагаемое отличие CraftPulse - их доступная совместная реализация поверх разнородного стека и связка с изменением/доставкой, а не «мы первые придумали SBOM».

## Этапы реализации и приёмка

**M0:** managed toolkit внутри CI клиента, versioned starter rules, reports+scan manifest, GitLab security JSON/SARIF/native отчёты выбранных producer, SourceCraft import, Sonar security facts, redaction, baseline/lifecycle, advisory policy. Поддержанные языки SAST и экосистемы SCA перечисляются точно в manifest; всё остальное import-first.

**M1:** registry inventory, EPSS/KEV, CodeScoring/DefectDojo/Dependency-Track по первым клиентам, SBOM reevaluation, SLA/exceptions, паспорт релиза. **M2:** runtime exposure, точечные language-specific reachability и DAST, автоматизированные предложения исправлений после проверки качества.

Приёмка: специально уязвимый fixture и похожий безопасный fixture для каждого starter rule; ни raw token, ни его часть не попадают в reports/logs; разный registry tag с одинаковым digest не создаёт дубликаты; смена CVE alias не удваивает проблему; stale DB и truncated history видны; no-findings при failed scan не даёт clean; VEX другого digest не подавляет finding; accepted risk не становится fixed; источник без package version даёт unresolved inventory; все MR сигналы воспроизводимы по сохранённым версиям.

Продуктовый pilot success: время первого подключения, подтверждённая полезность findings и время от сигнала до решения. Количество CVE и снижение собственного условного score не являются достаточным доказательством пользы.
