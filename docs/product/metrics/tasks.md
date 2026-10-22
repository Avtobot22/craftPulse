# Метрики задач и потока

## Продуктовое решение

Штатная реализация собирает историю из трекеров, восстанавливает состояние на дату, считает согласованные показатели потока и связывает их с MR, CI, документацией и выпуском. Пользователь получает готовую панель и прозрачные определения. Jira, Redmine или Яндекс Трекер остаются местом ведения задач.

Главные вопросы: сколько взяли и завершили по размерности; что стареет в работе; где заблокировано; какую часть исходного плана выполнили; сколько вернулось на доработку; как бизнес-эпик связан с инженерным результатом. Нельзя назвать закрытую задачу выпуском в production без отдельного доказательства.

## Коннекторы

| Сервис | Этап и состояние | Что получаем | Метрики | Обязательные ограничения |
|---|---|---|---|---|
| Jira Server/Data Center | M0, новый продуктовый адаптер; скрипт исследования не адаптер | Issues, status/resolution dictionaries, paginated history по проверенной версии, field schema, links, project/board/sprint API при доступности | Почти все T при настроенной семантике; release/DoD только с внешними данными | Не путать Server/DC с Cloud API. История из `expand=changelog` проверяется на truncation. Права и customfield ids scoped к instance. [Изученная REST 9.17](../../research/sources.md#ref-084) |
| Redmine | M1, следующий приоритет для Micha | Issues со `status_id=*`, detail `include=journals`, relations, custom fields, versions | T01–T18, T23–T24/T28–T34/T38–T42 по данным | Default list только открытые задачи; полный backfill нужен явно. Sprint plugin и estimates mapping отдельны. [REST](../../research/sources.md#ref-091) |
| Яндекс Трекер | M1 | Issues, очереди, статусы/типы, changelog и links; поля/оценки по схеме клиента | Поток, blockers, связность, полевая аналитика | Changelog пагинация через Link; категории и custom workflow нельзя копировать из Jira. Sprint/history capability подтверждать. [История](../../research/sources.md#ref-090) |
| Jira Cloud | M1, отдельный адаптер | v3 issues + changelogs; Agile board/sprint resources | T-метрики по permissions и доступной истории | Cursor/token endpoints меняются; pinned connector contract. Старый sprint-issues endpoint deprecated. [Issues](../../research/sources.md#ref-086), [sprint API](../../research/sources.md#ref-088) |
| GitLab Issues | M1 | Issues, labels, milestones, links, resource state events, notes; EE capabilities отдельно | Базовый flow и связи с MR | Label ≠ универсальный workflow. Sprint и weight могут зависеть от edition; milestone не автоматически спринт. [State events](../../research/sources.md#ref-080) |
| GitHub Issues / Projects | M1 Issues; M2 Projects | Timeline, issue state, links; project fields/history только в проверенном adapter scope | Базовый flow; T19–T22 только при исторических данных плана | PR также issue: раздельные сущности, общие API не означают двойной throughput. [Timeline](../../research/sources.md#ref-082) |
| SourceCraft Issues | M1, часть фактов уже собирается | Используем текущие status/timestamps/labels/comments facts; историю переходов расширяем лишь после проверки | Snapshot diagnostics; T01/T02/T03/T12/T38 частично | Нельзя получить cycle time или старый WIP из текущего статуса. Текущий ISSUES-PROXY-1 не flow engine |
| YouTrack | M2 | Issues/custom fields + activities с обязательными categories и pagination; agile resources отдельно | Flow, assignment history, links | Поля и значения имеют ids; набор history categories влияет на полноту. [Issue history](../../research/sources.md#ref-092) |
| Kaiten | M2 | Cards, boards/columns, перемещения, связи, оценки по текущему API | Flow и board-specific metrics | Старые sprint endpoints deprecated; нельзя обещать sprint analytics по старому примеру. [API](../../research/sources.md#ref-093) |
| Linear | M2 | GraphQL issues, workflow states, relations, cycles + webhooks | Flow и cycles при достаточной истории | Проверяем schema/history capability отдельно; archived resources по умолчанию скрыты, нужно includeArchived; GraphQL HTTP200 может содержать errors. [API](../../research/sources.md#ref-094), [pagination](../../research/sources.md#ref-095) |
| Azure Boards / Trello / Asana / ClickUp / EvaTeam | Кандидаты после M2 discovery | На старте generic import | Только метрики с подтверждёнными полями | Полные native adapters требуют отдельной проверки; подключать после приёмки соответствующих capabilities |
| Generic task import | M0 | Tasks + transitions + field changes + explicit links + workflow mapping + completeness manifest | Все T в пределах исходных фактов | CSV с текущим состоянием поддерживается как snapshot, не как восстановленная история |
| GitLab/GitHub CI, scanner/test/doc reports | M0/M1 через общие адаптеры | Подтверждённые task↔MR↔pipeline↔artifact↔release links | T25–T27/T35–T37 | Нужен тот же commit/version. Лишь зелёный pipeline не означает выполненный бизнес-результат |

Поддержка сервиса означает отдельно проверенные capabilities: `current_state`, `status_history`, `resolution_history`, `field_history`, `sprint_membership_history`, `sprint_metadata`, `issue_links`, `permissions`, `deletions`. Часть может отсутствовать при том же имени сервиса.

## Штатный движок по умолчанию

### Полная модель истории

Задача: `tenant/source/instance/project/issue_id`, mutable key, type, parent, explicit mirror group, lifecycle episodes, поля с effective time и provenance. Ключ `PROJ-42` не глобален. Перенос между проектами не должен терять identity. Сущности двух трекеров не склеиваем по одинаковому summary.

Событие: event id, issue id, field id, before/after, actor kind, event_time с исходным offset, observed_at, source URL, schema version. Храним валидную историю отдельно от текущего snapshot. Исправления и late events пересчитывают затронутые окна с новой calculation revision; старые отчёты воспроизводятся по сохранённой версии.

Первичная выгрузка охватывает **весь открытый backlog**, закрытую когорту за согласованный период и необходимую предысторию её начала. Фильтр `updated >= last_year` удобен для инкремента, но пропустит очень старые неподвижные открытые задачи и испортит WIP. После backfill - курсор/overlap window, дедупликация, reconciliation и tombstones в пределах API.

Историю пагинируем до подтверждённого конца. Jira Cloud/DC endpoints различаются. Если API предоставляет ограниченную часть истории, metric status BOUNDED/UNKNOWN; из отсутствующих событий не делаем вывод «перехода не было». Для Redmine отдельно проверяем journals visibility.

### Семантика workflow

Из API предлагаем mapping, пользователь видит пример из 10–20 задач и подтверждает Definition of Workflow: начатая работа, ожидание/блокировка, принятый результат, отмена, регресс/приёмка, production delivery. Используем status/resolution ids, а не английские строки Fixed/Done. Название служит подсказкой.

`created` - появление задачи; `started` - первое подтверждённое начало работы по политике. Если задачу действительно создали сразу в In Progress, start может совпасть с created. Если Needs Verification ошибочно входит в категорию In Progress, mapping исключает **этот статус**, а не любой статус создания всех задач.

`completed` - результат принят в рамках workflow и не отменён. `delivered` - связанное подтверждённое событие доставки/выпуска. Дата релиза fixVersion или status Done может быть бизнес-правилом только при явном подтверждении команды. По умолчанию называем показатель «завершённые задачи», не «выпуск в прод».

Reopen создаёт новый эпизод работы; сохраняем first-completed, final-completed-as-of, число эпизодов и причины. Метрика имеет выбранную единицу: уникальная задача или completion episode. По умолчанию throughput считает уникальные задачи, чьё первое принятое completion произошло в периоде; T08/T09 используют это первое completion. Повторные завершения идут в T36 и не переписывают дату первого результата. Если начало истории неизвестно, когорта BOUNDED. Другой cohort policy допускается как отдельно названная версия; rework отображается отдельно. Не меняем молча старую выборку, когда задача открылась снова.

### Интервалы и WIP

Для задачи интервал `started <= t < completed/cancelled` определяет WIP; переоткрытая работа снова входит в WIP по episode policy. **Blocked - подмножество WIP**, пока начатая работа не закончена. Рядом показываем active/waiting/blocked breakdown и полное число. Пауза может исключаться из отдельного active-clock, но не из end-to-end cycle time и полного WIP.

Статусная «активная работа» - proxy, а не измеренное touch time. Называем это «доля времени в состояниях active», не доказанная эффективность сотрудника. Даже In Progress включает встречи, ожидание и переключения.

Дата t восстанавливается по истории, а не по текущим полям. Для размера используем snapshot на start; для плана спринта - на фактический start спринта; team-at-time и epic history отдельно. Если прошлое неизвестно, доступен честный режим «группировка по текущей команде», визуально отделённый от исторической.

### Время, размер и targets

Calendar days - основной режим; business days - отдельная функция с календарём команды, праздниками и часовым поясом. Ночные переходы парсим с timezone offset, в UTC нормализуем преобразованием, а не подменой tzinfo. Windows `[from,to)` и as-of фиксируются.

Размер - локальный класс команды, не межкомандная валюта. Story points команды А не складываем со story points Б. Не приравниваем story point к дню и не умножаем throughput на «производительность». Missing size - отдельная когорта; показываем coverage и историю изменений размера.

Показываем p50/p85 и n для завершённой когорты; открытые задачи - отдельный aging chart. Метод percentile и его версия зафиксированы (например nearest-rank `ceil(p*n)`). Для n<20 p85 помечаем малой выборкой и не генерируем уверенный target; 20 - продуктовая защита, не закон статистики. Пользователь может изучить raw points при любом n.

Исторический p85 - ориентир, не автоматически обязательный срок. Target/SLE выбирает команда по размеру/типу; показываем исходную базу, вероятность/перцентиль, период, coverage и версию. Изменение target не переписывает историю нарушений. Стабильность процесса важнее красивого точного числа.

### План спринта

Источник истины - sprint ID и фактический start time. На эту дату фиксируем membership, размер, тип и team. Текущий список задач спринта не восстанавливает исходный план. Если есть полная история membership + metadata, backfill возможен; иначе начинаем снапшоты с подключения и старые спринты UNKNOWN.

Для исходного плана: `planned_at_start`, `planned_completed_by_end`, `removed_after_start`, `unfinished_at_end`. Для дополнительных: все add/remove события, в том числе добавили и убрали до конца. Задача может пройти несколько membership эпизодов, но в plan count учитывается один раз. Цель спринта - отдельное утверждение владельца, её достижение не равно доле закрытых tickets.

### Несколько трекеров и бизнес-срез

Одна рабочая область объединяет Jira и Redmine в общий список с исходными полями. Mirror/dedup mapping явный: источник истины по полям, тип связи, подтверждение и конфликт. Конфликт статуса показываем, а не выбираем «самый зелёный». Первая версия read-only; автоматическая двусторонняя синхронизация - отдельный продуктовый риск и scope.

Бизнес-эпик относится к подтверждённой бизнес-инициативе; label tech-debt нельзя выдать за денежную ценность. Иерархию, команды и сроки храним с effective dates. Агрегируем факты, показываем размер/тип и долю missing; не сравниваем команду поддержки с продуктовой только по throughput.

## Каталог: 42 метрики

| ID / этап | Определение | Срез / источник | Решение и ограничение |
|---|---|---|---|
| T01 M0 | Создано: уникальные eligible задачи с created в окне | Tracker; type/team-at-created | Баланс спроса. Импорт старых задач отдельно |
| T02 M0 | Backlog на дату: созданные, не начатые и не terminal на t | Full snapshot/history | Разобрать накопление. Нужна полнота всего open scope |
| T03 M0 | Возраст backlog: `t−created` для backlog задач, распределение | Type/priority | Приоритизировать triage; старость не автоматически просрочка |
| T04 M0 | Начато: уникальные задачи с первым valid start в окне | Workflow mapping + history | Сравнить вход в поток с выходом. Reopen-starts отдельно |
| T05 M0 | Завершено: уникальные задачи с первым принятым completion в окне (default); re-completions отдельно | Status + resolution history | Throughput результата workflow. Не production delivery |
| T06 M1 | Доставлено: задачи с подтверждённой первой delivery в окне | Task↔MR↔deploy/release | Отделить Done от ценности в эксплуатации; доставка не обязательно бизнес-эффект |
| T07 M0 | Отменено/дубликаты: terminal non-completion outcomes по причинам | Resolution at event | Не завышать throughput закрытиями без результата |
| T08 M0 | Cycle time: `completed−started`, p50/p85, n по завершённой когорте | Size-at-start/type/team | Найти длинный хвост. Open/cancelled исключения раскрыты отдельно |
| T09 M0 | Lead time: `completed−created`, p50/p85, n | Та же когорта, отдельная формула | Разделить ожидание до начала и работу после; не путать с DORA change lead time |
| T10 M0 | В пределах target: завершённые с cycle≤versioned target / eligible завершённые | Size/type, snapshot target | Проверить соглашение. Unknown start/size/target видны в coverage |
| T11 M0 | Aging vs target: `current_age/target` и число превышений среди WIP с target | Open episodes | Действовать до завершения. Отношение не вероятность опоздания |
| T12 M0 | Полный WIP: задачи с открытым started-episode на t | Started/terminal/reopen history | Ограничить начатую работу, включая blocked |
| T13 M0 | Blocked WIP: blocked subset / весь WIP + count | Flag/status history | Снять блокировки. Не вычитать из общего WIP |
| T14 M0 | Возраст WIP: `t−episode_started`, полный age первой работы отдельно | Open tasks | Найти застрявшие задачи; сравнение с завершённой когортой аккуратно |
| T15 M0 | Blocked duration: сумма blocked intervals, open interval до t; share elapsed | Blocker history | Оценить задержку состояния; причина может быть вне команды |
| T16 M1 | Время по состояниям: сумма непересекающихся интервалов в каждом status class | Full transitions | Найти очередь; active-state time не actual hands-on time |
| T17 M1 | Доля active-state time: интервалы active / full cycle для завершённых | Explicit class mapping | Проверить workflow; не называть доказанной производительностью |
| T18 M0 | Баланс потока: started vs completed по времени; рядом WIP trend/CFD | Same scope/units | Стабилизировать вход. Разность не полностью объясняет WIP при cancel/reopen/scope changes; добавляем bridge |
| T19 M1 | Исходный план: уникальные sprint members на exact start, count и local size sum отдельно | Sprint ID + membership/size history | Зафиксировать обещание. Нельзя считать из текущего списка |
| T20 M1 | Выполнение плана: planned tasks completed by end / planned-at-start; отмены видны | Frozen plan + lifecycle as-of-end | Не равняется выполнению sprint goal; removed не исчезают из знаменателя |
| T21 M1 | Scope changes: added/removed unique tasks и event counts после start, размер at change | Membership events | Отличить незапланированную работу; add-then-remove тоже учитывается |
| T22 M1 | Carry-over: planned-at-start не completed на end / planned-at-start, с причинами | Plan snapshot | Пересмотреть планирование; cancelled/removed показываем раздельными исходами |
| T23 M0 | Reopen: задачи завершённой когорты с reopen в фиксированное follow-up окно / задачи с полным таким окном | Reopen history | Измерять доработку workflow. Не называть возвратами с прода |
| T24 M1 | Возвраты из приёмки/регресса: eligible задачи с backward transition из согласованного gate / прошедшие gate | Versioned gate mapping | Найти качество до выпуска; predecessor Done сам по себе не регресс |
| T25 M1 | Escaped defects: подтверждённые production defects, связанные с delivered cohort, / delivered tasks; count defects отдельно | Defect origin + release link + follow-up | Несколько багов на задачу допустимы, rate может >100%; occurrence ≠ severity |
| T26 M1 | DoD coverage: задачи с машиночитаемыми обязательствами / eligible задачи | Fields/checklist/policy | Выявить пробел определения готовности, не качество реализации |
| T27 M1 | DoD verified: completed задачи со всеми обязательствами подтверждёнными версией evidence / eligible completed | Test/security/doc/business sign-off | Unknown evidence отдельно, не pass. Зелёный CI не закрывает ручную приёмку |
| T28 M0 | Task↔MR traceability: задачи code-required с подтверждённым merged MR / такие задачи | Explicit links + type policy | Открыть конкретный результат. Для research/ops типов N/A |
| T29 M1 | Дубликаты/конфликты трекеров: confirmed mirror groups и conflicts по полям | Cross-tracker mapping | Не удвоить планы/throughput; автоматического fuzzy merge нет |
| T30 M1 | Бизнес-покрытие: eligible work с подтверждённой initiative/epic связью / eligible work | Hierarchy-at-time | Показать структуру инвестиций, не доход |
| T31 M1 | Mix работ: count/локальный size по business/maintenance/security/support/unknown | Confirmed classification | Проверить согласованную квоту. Counts и points показываем отдельно, не выдумываем часы |
| T32 M1 | Срок бизнес-эпика: accepted completion vs согласованный due, объём/изменения scope | Epic policy + due versions | Показать обязательство. «Все дочерние закрыты» не всегда бизнес-приёмка |
| T33 M1 | Зависимости: blocked-by edges и возраст незакрытых внешних зависимостей | Explicit relations | Найти владельца препятствия. Отсутствие link не отсутствие зависимости |
| T34 M1 | Время передачи: team-owner change→следующий valid progress event, episodes | Team history | Найти межкомандное ожидание; updated/comment не обязательно progress |
| T35 M1 | Completed→delivered lag: delivery−completion для связанных задач | Release/deploy identity | Увидеть очередь выпуска; перенос отрицательных интервалов требует проверки lifecycle |
| T36 M1 | Rework episodes: число возвратных work episodes и elapsed duration | Transition history | Отдельно от новых задач; нельзя превратить elapsed duration в потраченные часы |
| T37 M2 | Прогноз: распределение даты/числа завершений из backtested исторических throughput samples | Stable scope + queue model | Вероятностный сценарий с assumptions, не обещание deadline |
| T38 M0 | Полнота полей: eligible tasks с size/team/epic/type/start по каждому полю отдельно | Snapshot/history capability | Missingness влияет на все срезы; одна общая «полнота 90%» скрывает проблемы |
| T39 M0 | Полнота истории: подтверждённо полные истории / проверенные сущности; earliest event, gaps | Pages/checkpoints/schema checks | Не путать с наличием поля changelog |
| T40 M0 | Свежесть и lag: watermark, last success, event-to-observed p95 | Sync state | Решить, можно ли делать вывод сегодня |
| T41 M0 | Mapping coverage: задачи/переходы с подтверждённой семантикой / eligible | Mapping version + sampled validation | Неизвестный status не автоматически backlog/done |
| T42 M0 | Reconciliation variance: разница с эталонной исходной выборкой на тот же as-of; причины по категориям | Source export + query/version | Восстановить доверие; сравниваем одинаковые timezone/scope/resolution rules |

### Разбор неоднозначных показателей

**Cycle time и throughput.** Среднее завершённых может улучшиться, пока тяжёлые задачи остаются открытыми. Поэтому T08 всегда рядом с T14 и scatterplot, а throughput - с размерностью и типом. Нет единственного «скорость команды +20%» из этих чисел.

**Reopen и production defects.** Для T23 задаём, например, 30 календарных дней наблюдения как локальную настройку; последнюю месячную когорту не сравниваем со зрелой без цензурирования. Для T25 требуется release/deploy и подтверждённый origin=production. Баг может быть отдельной задачей, не reopen старой.

**SLE.** Фраза «85% похожих задач завершались не более чем за 12 дней» описывает прошлое выбранной когорты. Она не означает «каждая новая задача закончится к 12-му дню с вероятностью 85%» без assumptions о сопоставимости и модели. Отдельный контракт клиента не выводится из historical percentile.

**Прогноз T37.** На M2 выбираем единицу задачи и тип потока; строим throughput samples, сохраняем zero days и календарь; bootstrap/Monte Carlo даёт сценарии «при неизменном scope и подобном процессе». Rolling backtest без использования будущего проверяет bias, coverage интервалов и ошибку по горизонтам. При смене состава команд/размера задач, малой выборке, неизвестном future scope показываем сценарий или отключаем прогноз. Командные points не смешиваются. Little’s Law используем как проверку согласованности при подходящих условиях, а не гарантированный forecast каждой задачи.

## Штатная панель и настройка

По умолчанию: вход/выход по месяцам и размеру; cycle-time scatterplot + p50/p85/n; WIP aging, blocked subset; target adherence, список риска; source coverage. При доступной sprint history включаются frozen plan, added/removed и результаты на end. Бизнес-эпики и DoD - дополнительные slots после mapping. Пользователь меняет фильтры/targets, а не пишет SQL для основных сценариев.

Drill-down числа содержит исходную выборку task ids, интервалы, исключения, период, timezone, версию mapping и ссылку на трекер. Export/API возвращает эти же определения, чтобы внешний BI не считал третью «правду». В первой версии не нужен произвольный SQL editor с доступом к общим таблицам.

Настройка под ключ: preview на примерах → подтверждение start/completed/cancelled/gates → сопоставление team/size/epic → проверка 3 ручных расчётов → включение ежедневного отчёта. Автоматическое предположение не скрывает необходимость этой небольшой проверки.

## Что берём из внешних решений

[Kanban Guide May 2025](../../research/sources.md#ref-073) даёт определения WIP, throughput, work-item age и cycle time, привязанные к согласованному workflow. Берём базовые определения; blocked остаётся начатой незавершённой работой. Наши правила когорт, snapshot и семантики delivery - отдельный продуктовый дизайн.

[Jira Agile API](../../research/sources.md#ref-087) даёт исходные board/sprint сущности; наша ценность не в воспроизведении burndown, а в нескольких трекерах, понятной семантике, size/team/business срезах и связи с инженерным evidence. [DevLake](../../research/sources.md#ref-070) уже собирает инженерные данные из многих систем; отсутствие отдельного коннектора у конкурента не означает уникальность всего продукта.

## Пользовательские сценарии

**Карточка исполнения обещания.** Бизнес-эпик → исходный/изменённый scope → задачи → MR → проверенные артефакты → delivery. Руководитель видит, где именно остановилось обязательство, а не сводный балл. Не присваиваем бизнес-ценность автоматически по title. Проверка: время подготовки отчёта и число ручных сверок против нынешнего Python/таблиц.

**Раннее предупреждение с объяснением.** Сочетание возраста задачи, target, blocked intervals, review queue и CI failures формирует список конкретных препятствий. Сначала rule-based: «задача 14 дней в работе, target12; MR ждёт review2д; CI passed». Вероятность срыва допускается только у отдельно калиброванной модели. Проверка: precision алертов, пропущенные случаи и действия владельцев.

**Два трекера без двойного результата.** Jira + Redmine объединяются в read-only view с подтверждёнными зеркалами и конфликтами. Источник истины виден; ничего не перезаписывается само. Проверка на проекте Micha: совпадение результатов с ручной сверкой и сокращение времени отчёта.

**Сравнение инициатив с сохранением контекста.** Отмечаем дату изменения процесса; сравниваем matched по типу/размеру/team когорты, качество и WIP до/после; показываем смену состава и missingness. Формулировка «после инициативы изменилось» до причинного исследования. Это закрывает запрос Кучанова оценивать улучшения.

## Реализация и приёмка

M0: Jira DC + GitLab + generic import; event store, workflow mapping, full open backlog, as-of flow, T01–T05/T07–T15/T18/T28/T38–T42, ежедневная панель и export. Один источник сначала позволяет проверить правильность engine.

M1: Redmine первым под прямой cross-tracker пилот; Яндекс Трекер/SourceCraft и Jira Cloud по выбранному коммерческому сегменту; sprint historical reconstruction, DoD и delivery evidence. M0 закрывает один законченный сквозной срез; следующие адаптеры принимаются отдельно. M2: прочие трекеры и backtested forecasting.

Приёмочные сценарии: создана сразу в active; создана в intake с неправильной категорией; blocked→active без завершения; done→reopened; cancelled/duplicate; изменился размер/команда; история обрезана; миграция проекта; старый open task без updates; две одинаковые задачи из разных instance; mirrored tasks; add/remove/re-add в sprint; task done до старта или после конца; событие в +05:30; повтор/late webhook; скрытая задача по ACL. Все ожидаемые значения фиксируются в проверяемых примерах, а не только в красивом dashboard.

Перед подключением фиксируются определения completion/delivery, IDs полей размера/команды/эпика, полнота changelog и правила объединения двух трекеров.
