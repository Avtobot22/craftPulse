# Метрики кода

Этапы блока: M0 - базовый профиль, M1 - расширение, M2 - сложные связи и рекомендации. Общие правила заданы в [модели измерения](model.md).

## Что продаём в этом блоке

Техлид получает конкретный список проблем **нового изменения** и обоснованный список мест, в которые стоит вложить время команды. Разработчик видит небольшую сводку в MR: новые нарушения, изменившиеся сложные функции, новые копии, проверенные и непроверенные области. Руководитель видит изменение технического долга по компонентам, причину изменения и связанные задачи.

Внутри блока различаем четыре вещи: корректность/надёжность по правилам анализатора, поддерживаемость структуры, давление изменений и достаточность наблюдения. Их нельзя сворачивать в один доказанный «уровень здоровья». Сложность не доказывает баг; частые коммиты не доказывают плохую архитектуру; отсутствие изменений тестов не доказывает отсутствие проверки.

Находки безопасности хранятся в общей модели, но получают отдельный тип и входят в [блок 2](security.md). Покрытие относится к данным проверок [блока 3](ci-cd.md), а здесь используется для контекста конкретного файла/MR. Двойного учёта одной находки в числах нет.

## Коннекторы: какие сервисы подключаем и что получаем

### Git и code review

Git-репозиторий и его API - разные источники. Через clone/fetch читаем дерево, diff и историю. API нужен для MR, комментариев, review, head/base SHA и статусов проверок. Подключение read-only Git URL само по себе не даёт доступа к MR.

| Сервис | Данные | Метрики/возможности | Этап и условия |
|---|---|---|---|
| **GitLab self-managed CE/EE**, также GitLab.com | Git, default branch, MR head/base, refs, merge, paths, notes/review; pipeline links | Все C01–C25 при наличии соответствующих отчётов, публикация MR-сводки | **M0**. Раздельные read credentials и bot write permissions. CE не требует Ultimate для получения Git/MR и обычных CI-артефактов |
| **SourceCraft** | Git clone; PR, comments, reviewers через API; существующие адаптеры | Структура, история, первоначальная связь PR с кодом | **M0: сохранить существующую поддержку**, расширенные проверки delta/публикации после contract test; не обещать все GitLab-возможности |
| **GitHub / Enterprise Server** | Git, PR, reviews, checks, workflow links | Та же нормализованная модель и рекомендации | **M1**. GitHub App с минимальными правами; Enterprise Server проверять по его версии |
| **Bitbucket Data Center / Cloud** | Git и PR API соответствующей редакции | Структура/история и PR-представление | **M2**. Это два адаптера с разными контрактами; CI может быть Jenkins |
| **GitVerse** | Git; PR/files/commits/reviews, API version=1 | Российский provider, общий каталог C-метрик | **M1**. Capabilities проверяются отдельно; совместимость с GitHub не предполагается |
| **GitFlic** | Git, MR и обсуждения | Анализ дерева и истории, связка findings с MR | **M1** после GitVerse либо по пилотному клиенту |
| **Любой Git по SSH/HTTPS**: Gitea, Forgejo, Gerrit, внутренний Git | Код, refs, история без гарантии MR API | C01–C10, C18–C23 и импорт отчётов | **M0, generic Git**; публикация результата через ссылку/CI, review-коннектор отдельно |

Основания контрактов российских провайдеров: [SourceCraft API](../../research/sources.md#ref-051), [GitVerse API](../../research/sources.md#ref-054), [GitFlic API](../../research/sources.md#ref-057). Полный список внешних источников - [источники методики](../../research/sources.md).

### Системы качества и анализаторы

| Подключение | Как забираем | Какие данные сохраняем | Этап / ограничение |
|---|---|---|---|
| **SonarQube Server / Community Build**, Cloud отдельной конфигурацией | **Web API**, webhook об окончании analysis как триггер чтения | Project/branch/analysis ID, revision, measures, issues/rules/status, quality gate и условия, coverage/duplication/complexity | **M0 - первый direct quality connector**. Список endpoints и доступность branch/PR зависят от версии/редакции. SARIF не предполагается |
| **ESLint + typescript-eslint**, Ruff, golangci-lint, ShellCheck | Родной JSON, GitLab Code Quality; SARIF, если реально выпускается конкретной версией | Rule, location, message, native severity, suppression/exit status, execution metadata | **M0**. Стандартный runner не равен API-коннектору к сервису |
| **PVS-Studio** | SARIF после plog-converter либо GitLab Code Quality | Предупреждения, diagnostic ID, CWE при наличии, paths, native severity | **M0 через SARIF**, direct service integration позже. Коммерческий движок клиента |
| **Qodana** | SARIF из CI | Rule, location, findings и baseline-состояния при наличии | **M0 через общий SARIF**; результат конкретного producer нужно проверить fixture-отчётом |
| **PMD/CPD, Checkstyle, SpotBugs, clang-tidy, detekt, Clippy, Roslyn, SwiftLint** | SARIF, Checkstyle XML или собственный adapter | Findings и/или duplication/structure в поддерживаемом формате | **M1 по языкам**. Нельзя объявлять весь список подключённым только потому, что есть XML parser |
| **CodeScoring TQI** | Прямой API/экспорт после проверки конкретного модуля | Структурные/исторические данные TQI, если доступны | **M2**. SCA API CodeScoring не доказывает наличие тех же полей в TQI |
| **CodeScene** | Клиентский экспорт/API по контракту установки | Hotspot/code health/coupling и provenance поставщика | **M2**, не строить первый выпуск на недокументированном публичном API; наш hotspot считается самостоятельно |

У Sonar Web API есть постепенный переход v1→v2; контракт проверяем на установленном сервере. Метрики cognitive complexity и debt из Sonar храним с источником. Они не тождественны Lizard cyclomatic complexity и нашему backlog. См. [Web API](../../research/sources.md#ref-002), [definitions](../../research/sources.md#ref-001), [PVS formats](../../research/sources.md#ref-013).

### Приоритет между внешним и штатным анализом

Для каждого `(component, language, analysis_kind, rule_profile)` выбираем authoritative source. Сначала используем законченный внешний отчёт **на нужном SHA и scope**. Штатный анализ добавляем для непокрытых языков/проверок. Включение Sonar не отключает автоматически secret scan или анализ Git-истории. Два результата одного правила не суммируем без дедупликации. Несовместимые инструменты показываем раздельно; «coverage Sonar + coverage локального XML / 2» не считается.

## Штатная реализация по умолчанию

### Что включается при подключении

1. Находим default branch и компоненты. По manifests предлагаем границы; монорепозиторий допускает ручное подтверждение путей. Имена директорий не считаются доказательством границы сервиса.
2. Строим inventory: production, tests, generated, vendor, examples, configs, unknown. Исключения объясняются в UI, есть число исключённых файлов/строк.
3. Выполняем structural/duplication-анализ для поддержанных языков; импортируем существующий lint либо включаем штатный профиль.
4. Сохраняем baseline default branch и версии инструментов. Затем на MR анализируем **head и сопоставимый base**, а не произвольный последний отчёт main.
5. Считаем факты истории за 90 дней, при доступности - расширенный тренд до 365 дней. Shallow clone отражается в полноте истории.
6. Публикуем один обновляемый комментарий: actionable-изменения и ссылки на полный отчёт; подробный backlog остаётся на сайте.

### Комплект инструментов

| Проверка | Штатный выбор | Что собирает | Статус/граница |
|---|---|---|---|
| Функции и сложность | **Lizard** | Function path/name/range, NLOC, CC, parameters при поддержке адаптера | Уже используется в прототипе для CC/NLOC. В M0 сохранять детальные function facts, не только агрегат |
| Копии | **jscpd** | Диапазоны clone-групп, язык, tokens/lines, объединение пересечений | Уже есть; pinned 5.1.2. Новые возможности upstream не переносить без обновления и фикстур |
| Python | **Ruff**, curated correctness profile | Диагностика F, E9 и согласованный набор B; native code/severity, suppression | M0. Набор B - версия профиля CraftPulse, подбирается на fixtures; не объявлять каждое правило security-уязвимостью |
| JS/TS | **ESLint recommended + typescript-eslint** | Syntax/correctness, unused/unreachable и проверенные TS-правила | M0. Type-aware rules требуют type information и dependency setup, поэтому отдельный opt-in профиль |
| Go | **golangci-lint** с контролируемым набором govet/staticcheck/errcheck/ineffassign и совместимых правил | Package/file/rule diagnostics | M0. Компиляция/загрузка типов и module cache в контуре клиента; ошибка загрузки пакета - неполная проверка |
| Shell | **ShellCheck** | Типовые ошибки quoting, expansion, portability | M0 при наличии shell. Репозиторные скрипты не выполняются |
| HTML/CSS/SCSS | HTMLHint / Stylelint | Синтаксис и согласованный минимальный профиль | Есть в прототипе; **M1 продуктовый профиль**, низкий приоритет в MR. Библиотеки клиента не грузим в server runner автоматически |
| Долговые маркеры | Собственный comment-aware extractor | TODO/FIXME/HACK с location и возрастом | M0 справочно, вне gate. Комментарий о долге не означает нарушение качества сам по себе |
| История | Собственный Git extractor | File touches, churn, renames, contributors, co-change | M0 базовые факты; coupling/ownership в M1 |
| Java/Kotlin/C#/C++/Rust/Swift/PHP | Импорт существующего producer; позже профили PMD/detekt/Roslyn/clang-tidy/Clippy/SwiftLint | Языковые находки | M1/M2. Структурная поддержка Lizard не равна полному качественному профилю языка |

Обоснование выбора: минимизировать новую обвязку и использовать уже включённые в worker движки; [Lizard](../../research/sources.md#ref-007), [jscpd](../../research/sources.md#ref-008), [Ruff](../../research/sources.md#ref-009), [ESLint JSON](../../research/sources.md#ref-010). Продукт не фиксирует «latest»: при выпуске утверждается immutable tool manifest с version, digest, config/rules hash, лицензиями и тестовым набором.

### Конкретные стартовые настройки

| Настройка | По умолчанию M0 | Почему / изменение |
|---|---|---|
| Production scope | Исключить generated/vendor/build/cache/binary, тесты считать отдельно | Vendor не должен улучшать/ухудшать поддерживаемость собственного кода |
| Security scope | Передаётся в блок безопасности отдельно | Lockfiles и configs нужны SCA/IaC/secrets, даже если исключены из code quality |
| Complexity hint | CC функции >10, высокий структурный сигнал >20 | Проектные ориентиры для сортировки, не универсальный запрет; показываем язык и распределение |
| Длина функции | NLOC >50 как сигнал | Порог пилотный; длинный декларативный код может быть нормой |
| Копии | Минимум 10 строк и 100 tokens, пересечения объединить | Совместимо с исходным прототипом; показываем parser/config версию |
| Стиль | Не повышать приоритет только за форматирование, не запускать автофиксы | Полезность для ревью важнее количества warnings |
| Gate | Advisory; новых high correctness-находок >0 - предупреждение | Жёсткая блокировка - отдельная настройка команды |
| Delta | Merge-base SHA и head SHA с одинаковым профилем | Старый latest-main не гарантирует сопоставимость |
| Периоды | MR; snapshot main ежедневно; тренды 30/90 дней | Никакой необходимости пересканировать тот же SHA каждую минуту |
| Budget | Стартовый лимит: 5 мин на лёгкий MR-профиль, отдельная очередь full scan | Это ограничение ресурсов, не обещание latency; превышение → partial, а не «успех» |

Чужой ESLint config может исполнять JS. Поэтому в собственном серверном runner используем наш зафиксированный конфиг и плагины; конфиг клиента исполняется его CI/агентом в его доверенной среде. Это конкретная причина, почему нельзя просто «найти eslint.config.js и запустить у нас».

## Каталог метрик

### Структура, объём и анализируемость

В формулах доля умножается на 100%. Нулевой знаменатель означает `NOT_APPLICABLE` или `NOT_EVALUATED` в зависимости от наличия объекта/источника. Ниже все C-метрики относятся к фиксированному component/SHA; период для истории явно указан.

| ID | Метрика и определение | Источник / единица | Этап и использование |
|---|---|---|---|
| C01 | Production NLOC: сумма непустых строк вне комментариев по утверждённому scope | Ш inventory/parser; строки | M0. Знаменатель и объём проекта, не эффективность команды |
| C02 | Поддержка языков: analyzed eligible files/eligible files отдельно по language и tool | Ш/О run manifest; % и n | M0. Нельзя выдавать «поддержан Java», когда сделан только clone search |
| C03 | Успех анализа: complete requested scopes/all requested scopes | Ш/О execution; % scopes | M0. Parser error/timeout отдельно от findings |
| C04 | CC функции: значение analyzer; p50/p90/max по функциям | Ш Lizard/К Sonar; CC | M0. Сравнение внутри языка/инструмента; среднее не скрывает хвост |
| C05 | Функции выше CC-порога: count(CC>configured threshold)/analyzed functions | Ш function facts; % и count | M0. Порог отображается рядом |
| C06 | Длина функции: NLOC, p50/p90/max и count NLOC>threshold | Ш Lizard; строки | M0. Сигнал для review, не оценка бизнеса |
| C07 | Параметры функции: count parameters, tail выше порога | Ш расширенный adapter; параметры | M1. Нет поля у producer → не считать |
| C08 | Дублированные строки: union clone ranges / eligible analyzed physical lines того же scope | Ш jscpd/О; % и строки | M0. Не делить физические ranges на несовместимый NLOC denominator; при ином формате хранить native ratio отдельно |
| C09 | Clone groups и их распределение между компонентами | Ш/О clone facts; группы | M0 локальные, M1 cross-component. Одна группа не считается отдельной парой на каждый файл |
| C10 | Cognitive complexity: native analyzer value/distribution | К Sonar/О; native units | M1. Lizard CC не подставляется вместо cognitive complexity |

### Находки и изменение кода

| ID | Метрика и определение | Источник / единица | Этап и использование |
|---|---|---|---|
| C11 | Открытые actionable findings: unique active, не false-positive/accepted, по class/rule/native severity | О/К/Ш findings; count | M0. Security отделена типом, источник и mapping видны |
| C12 | Новые findings MR: unmatched head findings после identity mapping к base | С baseline+head; count | M0. Без comparable base - «впервые наблюдены», не «внесены автором MR» |
| C13 | Устранённые findings: присутствуют в base, отсутствуют в complete сопоставимом head scope | С; count | M0. Не закрываем при неуспешном скане или исключении файла |
| C14 | Findings density: C11 ×1000/analyzed production NLOC | Ш/О/К; findings/KLOC | M1. Только один producer/profile/scope; низкая плотность не доказывает качество |
| C15 | Размер MR: added/deleted lines и changed files раздельно | К/Git diff; строки/файлы | M0. Generated/vendor отдельно; не превращать в норму производительности |
| C16 | Изменение сложности: head CC−base CC для сопоставленной функции; новые функции отдельно | С structure+diff; CC | M0. Не сравнивать суммы при переносе/разбиении без mapping confidence |
| C17 | Новые копии в MR: clone groups с новым/изменённым участником, не известные base | С clone+diff; группы/строки | M0. Одно удаление/rename не должно превращать старую копию в новую |
| C18 | Давление изменений: touches и added+deleted lines за 30/90 дней | Ш Git; touches/строки | M0. First-parent для истории main; MR history отдельным видом, squash/merge не удваивать |
| C19 | Change concentration: churn top 10% файлов / весь eligible churn окна | Ш Git; % | M1. Ранжирование релевантности, не диагноз архитектуры |
| C20 | Hotspot shortlist: ранжирование сочетания complexity и touches; детали формулы ниже | С; rank и исходные факты | M1, в M0 можно показать существующий экспериментальный риск с маркировкой |
| C21 | Temporal coupling A↔B: совместные eligible commits / commits, менявшие хотя бы A или B | Ш Git; Jaccard % и support | M1. Исключить mass-format/merge/vendor commits, минимум support 5, окно 90 дней |
| C22 | Межкомпонентные совместные изменения: co-change groups с ≥2 подтверждёнными components | Ш/С; группы и доля изменений | M2. Нужна утверждённая карта компонентов, не только директории |
| C23 | Концентрация знаний: максимальная доля нормализованного author contribution по файлу | Ш Git/blame; % и coverage | M1. Это ownership proxy, не настоящий bus factor; bots/aliases обрабатываются отдельно |

### Техдолг, контекст проверки и эксплуатация блока

| ID | Метрика и определение | Источник / единица | Этап и использование |
|---|---|---|---|
| C24 | Возраст открытой находки: now−first observed; p50/p90 и возраст oldest | История findings; дни | M0. Первое наблюдение CraftPulse не равно дате появления дефекта |
| C25 | Net backlog: new−resolved за окно, accepted/FP/moved вынесены | История lifecycle; count | M0. Смена ruleset отдельно от изменения кода |
| C26 | Принятые исключения: active accepted/false-positive, expiry и rationale | О/К/Ш triage; count | M0. «Принят риск» не равно «исправлено» |
| C27 | TODO/FIXME/HACK: count по типу, возраст и ссылка на задачу при наличии | Ш comments+Git; count/дни | M0 справочно. Строка внутри literal не считается комментарием |
| C28 | Patch line coverage: covered changed executable lines / changed executable lines с coverage mapping | С coverage+diff; % и lines | M0 при входном покрытии. JUnit не источник coverage; данные собирает блок 3 |
| C29 | Изменённые функции без доступного покрытия: functions without matched coverage / changed functions | С; % и n | M1. «Нет данных» отличается от «все строки не покрыты» |
| C30 | Review latency и first review: первый human review−ready for review | К MR/reviews; часы p50/p85 | M1 библиотека, не основной дашборд. Bots/draft/self-comments не первый ответ |
| C31 | Findings со связанной задачей: active findings с явной issue relation / active findings | С интерфейс с трекером; % и n | M1. Поиск ключа в тексте - guessed link, не подтверждённая связь |
| C32 | Проверяемость рекомендаций: recommendations с исходными фактами и воспроизводимым rule / все emitted | Ш; % | M0 технический критерий. LLM-текст не считается основанием |
| C33 | Действия по предупреждениям: fixed / task prepared / accepted / ignored по cohort MR | С lifecycle; % и count | M1 оценка пилота, не показатель личной дисциплины |
| C34 | Оценка effort внешнего инструмента: сумма native remediation minutes в сопоставимом scope | К Sonar/О; условные минуты | M1. Не выдавать за трудозатраты команды и не складывать с реальным time tracking |
| C35 | Freshness: now−time последнего complete snapshot для каждого component/profile | К/О/Ш; часы | M0. Старый полный отчёт не выдаётся за актуальную проверку head |
| C36 | Latency/overhead анализатора: finish−start, queue отдельно, resource limits | Ш execution; сек/p95 | M0. Нужна для контроля стоимости штатного набора |

## Как ранжировать и объяснять

### Hotspots

Заимствуем у CodeScene идею «сложное и часто меняемое заслуживает внимания», а не закрытый балл. В первом продуктном варианте: внутри component+language считаем percentile rank touches и percentile rank CC-tail по файлам; сортируем по произведению этих двух ranks. Показываем сами touches, CC, NLOC, window, n и источник. Файл с нулём touches не получает активный hotspot только из-за ранга одинаковых нулей. При отсутствии достаточной истории сортируем только по структуре с отметкой неполноты.

Эта формула - **наша эвристика для проверки**, не вероятность бага. Прототип уже содержит другую формулу `100 × structural_risk × (0.40 frequency + 0.25 recency + 0.35 relative_churn)`; её не смешиваем с новым ранжированием без новой metric version и отметки на графике. Ранг 1 означает первый в списке, не «опасность 100%». [CodeScene terminology](../../research/sources.md#ref-004).

### Сопоставление baseline/head

Храним base/head SHA, merge-base, content hash, tool/rules/config/scope versions и completeness каждого запуска. Перенос файла отслеживаем через Git rename + location/context, перенос функции - через устойчивую сигнатуру и структуру при наличии parser. Rule ID+line не годится как единственный fingerprint: добавленная сверху строка не создаёт новую проблему.

Если идентичность не доказана, даём `possible_match` и не закрываем старую находку автоматически. Отдельные категории: introduced, resolved, unchanged, moved, newly_observed, methodology_changed, source_missing. Базовый подход к стабильным идентификаторам подтверждает и [DefectDojo reimport](../../research/sources.md#ref-032), но нормализация CraftPulse требует собственных fixtures.

### Понятный MR-результат

Пример сводки на учебных данных:

> В изменении 7 файлов: 2 новых correctness-находки, 1 функция выросла с CC 8 до 17, добавлена копия 18 строк. Patch coverage 74% на 46 executable lines. Проверено 93% eligible Python-кода; Go-пакет не загрузился. Режим предупреждения. Открыть finding → строку → правило; подготовить задачу на долг.

Максимум 5 наиболее важных новых сигналов в комментарии, остальные по ссылке. Обновляем комментарий по стабильному marker, связываем с конкретным head SHA. Суммарный legacy backlog в MR не выгружаем. При новом commit старый результат виден как устаревший до окончания новой проверки.

## Пользовательские сценарии

| Фича | Как складывается из данных | Почему полезно / граница | Очередь |
|---|---|---|---|
| **Объяснённое ухудшение MR** | Base/head findings + function delta + scope/version diff | Вместо «score упал» видно, какая функция/правило изменились; methodological jump не приписываем разработчику | M0, сильнее всего связано с Фархудиновым |
| **Legacy можно улучшать постепенно** | Baseline debt + new-code policy + expiry исключений | Не блокировать каждый релиз за весь старый долг; контроль новых ухудшений. Идея есть у Sonar, ценность CraftPulse - единообразие источников | M0 |
| **Карта первых мест для разбора проекта** | Complexity × history + ownership proxy + known findings | Ответ Micha «куда смотреть» с конкретными файлами и вопросами; не автоматический архитектурный диагноз | M1 |
| **План техдолга по компонентам** | Actionable backlog + churn + tasks + estimated external effort | Сначала 3–5 групп проблем с подтверждаемой ценой/частотой касания, а не 500 отдельных warnings | M1, проверка на пилоте |
| **Проверка результата рефакторинга** | Function mapping + complexity/duplication + CI и coverage | Показать «снизилась сложность; тесты не ухудшились в наблюдённых запусках»; не доказывать семантическую эквивалентность | M1 |
| **Неожиданная связность** | Co-change + component map + MR | Подсказка ревьюеру о компоненте, который исторически меняется вместе, но в MR не затронут | M2, минимум support и обратная связь пользователя |

Ни одна отдельная идея не заявляется уникальной на рынке. Заимствуем механику new-code gates, history-aware prioritization и patch context, но связываем её с российским стеком, общим evidence и штатной поставкой.

## Первый выпуск и критерии готовности

**M0:** generic Git и GitLab, сохранение SourceCraft, Sonar measures/issues, SARIF 2.1.0 + GitLab Code Quality + native JSON поддержанных линтеров, базовые Go/Python/JS/TS profiles, CC/NLOC/clones, baseline/head, lifecycle, advisory MR comment, states/coverage. Без construction-kit для произвольных plugin chains и без собственного language-semantic engine.

**M1:** больше языков и провайдеров, temporal coupling, component ownership, создаваемые по запросу задачи, устойчивый refactoring mapping. **M2:** cross-repo coupling и рекомендации по инвестициям с проверкой на реальных затратах/результатах.

Приёмка M0: fixture-репозитории на трёх языках и монорепозиторий; добавление строки/rename не создаёт ложный new finding; partial scan не закрывает старый finding; смена правила маркируется; внешний отчёт и штатный скан не удваивают counts; неисполняемый/вредный repo config не запускается на сервере; missing coverage не становится 0%; результаты привязаны к head SHA и доступны пользователю только в его scope. Пилотный критерий полезности - доля предупреждений с подтверждённым действием и оценка техлида, а не прирост нашего собственного балла.
