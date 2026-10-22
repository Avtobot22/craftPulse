# Метрики документации

## Продуктовое решение и границы спроса

Штатная функция - найти относящуюся к сервису документацию независимо от места хранения, показать её владельца и доказательства актуальности, обнаружить конкретный разрыв между изменением продукта и инструкцией. Проверка README/лицензии остаётся отдельным профилем публичного репозитория. Она не становится оценкой корпоративных знаний.

Блок подключается как ограниченная опция под конкретный сервис и изменение. Сначала проверяются выбранные roots, владельцы, links и API-spec; wiki, review и docs impact расширяют этот результат. Базовый поток задач и CI не зависит от подключения всех хранилищ документации.

Главный экран: критичные сервисы без подтверждённого владельца/доступного runbook; изменённые API без актуальной reference; нарушенные ссылки с точным местом; документы с просроченной проверкой владельцем; полнота охвата. Количество страниц, возраст правки и «ИИ понравился текст» не заменяют проверку полезности.

## Коннекторы: внешние системы

M0 здесь - тонкий Git-based pilot; расширенную wiki-интеграцию M1 запускаем при согласованном доступе и сценарии клиента. Все будущие коннекторы - план, не уже доступная функция.

| Источник | Этап | Что читаем | Что можем считать | Чего не обещаем |
|---|---|---|---|---|
| Git: GitLab/GitHub/SourceCraft/read-only clone | M0 | Явные docs roots, Markdown/MDX/AsciiDoc/RST как поддержанные parser profiles, история, owners, config; на старте Markdown | D01–D17, D20–D24, D27–D30 по capability | Парсинг всех форматов в M0. Не выполняем произвольный проектный build для чтения README |
| Confluence DC | M1, приоритет по клиенту | Space/page id, title, URL, body выбранного формата, version/time/editor, labels, ancestors; ACL/restrictions по доступному API | Инвентарь, структура, ссылки, версии, review metadata; D01–D12/D15/D18–D21 | Все страницы организации или эффективные права пользователя из одного restrictions response. [DC REST](../../research/sources.md#ref-099) |
| Confluence Cloud | M1, отдельный адаптер | Pages/versions через v2 cursor pagination, дополнительные permissions/restrictions endpoints по manifest | Аналогично в пределах прав integration | Полная совместимость с DC; встроенные analytics/views на любом плане. [v2](../../research/sources.md#ref-096), [versions](../../research/sources.md#ref-097), [restrictions caveat](../../research/sources.md#ref-098) |
| Яндекс Wiki | M1 кандидат для российского контура | Страницы по slug/id, body, дерево в доступной области; OAuth/организация | D01–D07/D09–D12/D20 с подтверждёнными полями | API может быть выключен администратором. UI history не доказывает доступный API архив версий. [API pages](../../research/sources.md#ref-100), [доступ](../../research/sources.md#ref-101) |
| Notion | M2 | Страницы с выданным доступом, metadata, рекурсивно block children, database/data-source возможности по pinned Notion-Version | Структура, текст, links, metadata; без full-history метрик | Page metadata не содержит весь body; search не гарантирует discovery всей workspace; unsupported blocks отдельно. [Children](../../research/sources.md#ref-103) |
| Google Drive / Docs | M2 | Файлы, mime, metadata, разрешённый export текстовых документов, changes cursor | Инвентарь, ссылки, обновления, извлекаемые артефакты | Changes возвращает текущее состояние, не полный edit log. Revisions не принимаем за гарантированную историю каждого редактирования. [Files](../../research/sources.md#ref-104), [changes](../../research/sources.md#ref-105), [revisions](../../research/sources.md#ref-106) |
| SharePoint / OneDrive | M2 | Microsoft Graph driveItems, metadata/content, delta nextLink→deltaLink, deletion facets | Инвентарь и изменения; текст после безопасного извлечения | Delta - sync-протокол, не семантическое ревью. Доступ зависит от выбранной схемы tenant consent. [delta](../../research/sources.md#ref-107) |
| Backstage / TechDocs | M1 импорт каталога; M2 отдельный адаптер | Entity ref, owner, lifecycle, relations, techdocs/source references, опубликованные артефакты | D03–D07/D20/D23; связи сервис↔доки | Backstage не заменяет исходный ACL и не доказывает качество текста. [TechDocs](../../research/sources.md#ref-076) |
| OpenAPI / AsyncAPI / protobuf specs | M0 OpenAPI file; остальные M2 | Версионируемые спецификации из repo/artifact, endpoints/operations/descriptions | D14–D17, D24–D25 | Схема сама по себе не доказывает соответствие production поведению |
| Локальные файлы / S3-compatible export | M1 | Выбранный каталог или версионируемый manifest; hash, source URL, owner, extracted body | Базовые D01–D12, D27 | Объекты без metadata не дают историю review/авторства; Office/PDF extraction capability отдельно |
| BookStack / Wiki.js / Outline / корпоративная wiki | M2 candidate | Вначале generic export; native connector после изучения API конкретной версии | Только подтверждённые поля manifest | Наличие API «в целом» не означает готовый продуктовый коннектор. Native matrix проверяется отдельно; generic import не означает поддержку каждого сервиса |
| Аналитика поиска/feedback/helpdesk | M2, отдельная опция | Агрегированные поисковые запросы/результаты, полезность статьи, связанные обращения | D31–D34 | Нельзя получить readership из page.modifiedTime; нужен отдельный источник и согласованный scope |

Общее: 401/403/429, недоступный HTML, частичный export и нераспознанный формат не равны «битая документация». Различаем `UNREACHABLE`, `ACCESS_DENIED`, `RATE_LIMITED`, `UNSUPPORTED`, `NOT_FOUND` и стабильный подтверждённый дефект.

## Штатная реализация: что поставляем сами

### Управляемый каталог и проверки

**Каталог.** Документ имеет стабильный `(source, id)`, canonical URL, service refs, type, language, owner, audience, lifecycle, source version/hash, observed_at и permission scope. Одна страница, доступная через две ссылки, не два документа. Сервис может ссылаться на несколько хранилищ; чужой документационный репозиторий не требуется копировать в основной код.

**Минимальный профиль M0.** Пользователь указывает doc roots и владельца сервиса. Собираем Markdown, локальные ссылки/anchors, внешние links в безопасном сетевом режиме, frontmatter, code fences, API-spec refs и историю изменений. Делаем inventory, проверяем структуру и связи. Без настроенного root результат UNKNOWN, а не «0% документации».

**Профили по сценарию.** `service-onboarding` требует owner, запуск/окружение и путь к архитектуре; `production-service` добавляет runbook/rollback/alerts; `public-library` - API/examples/contribution/license; `internal-api` - specification и compatibility notes. Набор обязателен только после выбора профиля. Для зрелого сервиса LICENSE в каждом repo не становится обязанностью.

**Детерминированные проверки.** AST Markdown, существование файлов и anchors, доступность URL, schema OpenAPI, требования шаблона, владельцы, review expiry, совпадение версии опубликованных docs и исходного commit. Сигналы дают точное правило, строку/блок, версию checker, fingerprint, suppression с причиной/сроком.

**Семантическая помощь.** Опциональная модель предлагает связь с сервисом, summary или возможное противоречие. Всегда хранит выдержки/ссылки на обе версии и статус `needs_review`. Она не подтверждает достоверность архитектуры или безопасность runbook. Неподтверждённые предложения не меняют обязательный pass/fail.

### Что заимствуем у зрелых инструментов

| Компонент | Использование в штатной поставке | Ограничение |
|---|---|---|
| [markdownlint](../../research/sources.md#ref-108) | Версионируемый Markdown style profile, JSON findings, правила можно отключать | Форматирование не равно полезности. Не делаем список MD-ошибок главным KPI |
| [Vale](../../research/sources.md#ref-109) и [наборы правил](../../research/sources.md#ref-110) | Терминология, запрещённые обороты, собственный словарь, разные языковые профили | Англоязычные rules не применяем вслепую к русскому тексту; стиль - advisory |
| [lychee](../../research/sources.md#ref-111) | Проверка URL и локальных ссылок в изолированной задаче; нормализация выводов | Любые shell/preprocessor опции из чужого repo не исполняем; retries и классификация auth отдельно |
| [Spectral](../../research/sources.md#ref-113) | Локальные policy rules для OpenAPI/AsyncAPI-кандидатов | Правило required description проверяет наличие, а не истинность |
| [oasdiff](../../research/sources.md#ref-114) | Diff спецификаций и потенциальные breaking changes | Это spec diff, не доказанный сбой клиента; переносим в task/release context |
| [Diátaxis](../../research/sources.md#ref-074) | Сценарные типы tutorial/how-to/reference/explanation | Не требуем четыре документа для каждого сервиса; типы - способ проектирования полезного набора |
| [TechDocs](../../research/sources.md#ref-076) | Документация связана с каталогом сервисов; build может жить в CI | Не переписываем портал и build engine ради панели здоровья |

Зависимости фиксируем по версии/digest, лицензии и notices проверяем перед поставкой. Пользователь получает поддерживаемую конфигурацию, отчёт и обновления; это наша штатная реализация на базе внешних движков, а не заявление об авторстве чужих анализаторов. Профиль поставки проверяется на зафиксированных версиях инструментов и контрольных примерах перед включением.

### Сеть, сборка и доступ

Link checker может получить из документа ссылку на внутренний адрес. По умолчанию внешняя проверка разрешает только выбранные HTTP(S) hosts и блокирует loopback, metadata endpoints, неожиданные redirects и выдачу credentials другому host. Для корпоративных intranet docs применяется явно настроенный runner внутри контура, без открытия произвольной сети SaaS.

Не собираем чужой MkDocs/Sphinx/MDX проект с произвольными plugins в основном backend. Предпочтительно импортируем build report клиента. Если managed build нужен - изолированный одноразовый sandbox с лимитами, pinned toolchain и выбранной сетевой политикой. Code examples не запускаются автоматически ради балла.

Service account, видящий всё, не разрешает показывать всё всем пользователям CraftPulse. В M0 допустим явно выделенный corpus с одной согласованной аудиторией. Для смешанных ACL нужны проверка эффективного доступа, partitioning индекса и корректная инвалидация после revocation. До этого чувствительный corpus не индексируем. Заголовки, embeddings, snippets и агрегаты тоже могут раскрывать данные; ограничения действуют и на них.

## Каталог метрик: 34 показателя

`eligible` - документы/сервисы, явно включённые выбранной политикой. Coverage всегда измеряется внутри объявленного scope; неизвестную за пределами scope документацию не выдаём за отсутствующую. M0-панель ограничена D01/D03–D07/D09–D11/D20/D27/D30 и доступными проверками OpenAPI, остальное - опции.

| ID / этап | Метрика / расчёт | Источник и действие | Ограничение |
|---|---|---|---|
| D01 M0 | Охват: число обнаруженных docs roots и успешно прочитанных документов; доля только если известен полный manifest | Git/wiki manifests; подключить пропущенный root | Не рассчитываем процент всей компании по search API |
| D02 M1 | Покрытие форматов: распарсенные / доступные eligible документы по MIME и языку | Parser result | Изображение/PDF без текста не «пустая страница» |
| D03 M0 | Владельцы: eligible документы с валидным owner / eligible документы | Frontmatter/catalog/custom metadata | Last editor не назначается owner автоматически |
| D04 M0 | Назначение/аудитория: документы с подтверждёнными type и audience / eligible | Profile metadata | Классификация моделью требует подтверждения |
| D05 M0 | Service coverage: сервисы с доступным doc root / сервисы в каталоге выбранного scope | Service↔doc links | Если каталога сервисов нет, знаменатель неизвестен |
| D06 M0 | Onboarding coverage: сервисы с выполненными требованиями выбранного onboarding profile / eligible сервисы | Проверки owner/start/config/architecture refs | Наличие раздела не подтверждает успешный запуск |
| D07 M0 | Runbook coverage: production-сервисы с доступным owner-assigned runbook / production-сервисы | Service lifecycle + docs | Runbook может находиться во внешнем corpus; это не штраф всем repo |
| D08 M1 | Просроченный review: `as_of > next_review_due` для eligible документов; count/share | Review metadata по классу критичности | Нет due-политики - нет нарушения. Last modified не заменяет review |
| D09 M0 | Возраст изменения: `as_of − last_content_change_at`, p50/p85 и список | Git/wiki versions | Описательный сигнал. Старый стабильный контракт может быть верным |
| D10 M0 | Локальные broken links: подтверждённые отсутствующие file/anchor / проверенные локальные targets | AST + resolver, case sensitivity | Относительная ссылка разрешается относительно файла/публикации, а не cwd backend |
| D11 M0 | Внешние link failures: устойчивые 404/410 после установленной проверки / проверенные разрешённые targets | URL runner, timestamps | 403/429/timeout и сеть отдельно; credentials не включаются в отчёт |
| D12 M1 | Orphan docs: документы без входящей связи из выбранной navigation/service graph | Wiki tree/Git nav + links | Прямая ссылка из задачи может делать страницу полезной; «orphan» - кандидат на разбор |
| D13 M1 | Структурные нарушения: уникальные rule findings и affected docs / проверенные docs | Markdown AST/profile | Heading style и длина текста не глобальный quality score |
| D14 M0 | API description coverage: операции/параметры со структурно непустыми descriptions / eligible операции/параметры | OpenAPI parser | Раздельные знаменатели; автогенерированный placeholder фильтруется, полезность требует review |
| D15 M1 | Example coverage: API операции или сценарии с валидно связанным примером / eligible | Spec/docs links | Число code fences не число работающих примеров |
| D16 M1 | Проверяемость примеров: examples с актуальным test/build report на ту же версию / eligible examples | CI artifact + spec hash | Report без matching commit/hash - UNKNOWN |
| D17 M1 | API/doc drift: изменённые операции с нарушенным обязательством обновить reference / операции с таким обязательством | Spec diff + ownership map + review | Любое изменение кода не требует переписывать docs; обновлённый timestamp не снимает drift |
| D18 M1 | Подтверждённость review: eligible документы с review на текущий content hash / eligible | Review record with reviewer/time/hash | Редактирование после review инвалидирует только релевантную область по политике |
| D19 M1 | Время восстановления docs: `verified_fix_at − confirmed_drift_at`, p50/p85 | Findings lifecycle | Открытый долг показываем отдельно; не скрываем за быстрыми закрытиями |
| D20 M0 | Task/doc traceability: задачи обязательных типов с подтверждённой doc/spec связью / такие задачи | Tracker↔docs explicit links | У остальных типов NOT_APPLICABLE; ссылка может требовать отдельного ACL |
| D21 M1 | Готовность к передаче: задачи с подтверждённым handover doc obligation / задачи с обязательством | Task DoD + artifact version | Это проверка конкретных обязательств, не «красивости документа» |
| D22 M1 | ADR coverage: подтверждённые архитектурные решения со связанным accepted/superseded ADR / решения в реестре | Decision register + ADR metadata | Нельзя вывести полный список решений из названий MR |
| D23 M1 | Publication parity: опубликованные docs с source commit/hash текущего релиза / eligible docs sets | CI publication manifest | «Последний main» не всегда версия, нужная пользователю релиза |
| D24 M0 | Spec validation: валидные specs / проверенные specs; rule findings отдельно | Pinned OpenAPI validator/rules | Schema-valid не означает implementation-correct |
| D25 M1 | Breaking API changes с migration guide: такие changes со связанным подтверждённым guide / confirmed breaking changes | oasdiff + human validation + release links | Детектор даёт candidate, scope compatibility зависит от контракта |
| D26 M2 | Runbook exercise success: успешно выполненные согласованные упражнения / проведённые упражнения | Human exercise record + version | Показатель operational readiness; текста недостаточно |
| D27 M0 | Полнота/свежесть сбора: corpus with complete pages + last successful sync + earliest version | Connector state | Не смешивать с актуальностью содержания |
| D28 M1 | Исключения: accepted findings с причиной, owner и expiry / suppressed findings | Policy registry | Массовое suppression не улучшает фактические docs; показываем отдельно |
| D29 M1 | Дубли/противоречия: кандидаты по hash/text similarity + подтверждённые случаи отдельно | Local similarity / optional AI | Похожесть не ошибка; версии документа для разных релизов нужны |
| D30 M0 | Типизированный doc debt: открытые подтверждённые findings по правилу, критичности и возрасту | Checker findings | Не суммируем в деньги/часы без независимой модели затрат |
| D31 M2 | Поиск без результата: запросы без результатов / eligible агрегированные запросы | Search analytics | Отсутствие результата может быть вызвано ACL, языком или поиском, а не отсутствием документа |
| D32 M2 | Успешность поиска: задачи поиска, завершённые подтверждённым нахождением ответа / согласованные задачи | Usability study или явный feedback | Click-through/длительность просмотра сами по себе не success |
| D33 M2 | Полезность: positive explicit feedback / все явные оценки, с n и response rate | Feedback service | Сильный selection bias; не сравнивать авторов по рейтингу |
| D34 M2 | Повторные обращения: обращения по подтверждённой теме документа за окно, до/после изменения | Helpdesk tagged cases | Корреляция не доказанная экономия; нужна стабильная классификация и объём использования |

### Пример расчёта и UNKNOWN

В каталоге 12 production-сервисов; для 9 есть подтверждённый доступный runbook с владельцем, для двух доступ к выбранной wiki ещё не выдан, один владелец подтвердил отсутствие runbook. Нельзя показать «runbook coverage 75%, три сервиса без документации». Показываем: 9 подтверждены, 1 отсутствует в проверенном scope, 2 неизвестны; assessed subset 9/10, total known coverage 9/12 с отдельным missingness. Не скрываем неизвестные сервисы исключением из интерфейса.

Документ изменён 180 дней назад, контракт не менялся, владелец проверил текущий hash вчера. D09=180 дней, D18=актуальный review. Это не красный «протухший документ». И наоборот, ежедневный автоматический rebuild не делает устаревший runbook проверенным.

## Пользовательские сценарии

**Docs impact в MR.** Меняется OpenAPI/конфигурационный контракт; через service map находим reference/runbook; показываем ровно затронутые обязательства. Карточка содержит diff контракта, документ и владельца. Нужны D17/D20/D23 и Git diff. Проверка: precision предложений владельцами, доля полезных обновлений до выпуска, число ложных блокировок. Сначала advisory.

**Свидетельство готовности сервиса.** В одной карточке: версия runbook, кто и когда выполнил упражнение, deployment version, rollback/test status из CI, незакрытые doc gaps. Это конкретные доказательства, а не ещё один score. Проверка на tabletop/учебном инциденте; не обещаем снижение MTTR из наличия страницы.

**Проверка принятого решения после изменения.** Из ADR и service dependency map выводим области, где MR может нарушить задокументированный контракт. Модель предлагает проверку с цитатами; инженер принимает/отклоняет. Нет автоматического «архитектурного нарушения» на основании similarity.

**Обзор незнакомого проекта с происхождением утверждений.** Owner + схема сервисов + актуальные решения + инструкции + последние связанные задачи. У каждого факта дата, источник и ACL. При недостатке данных ответ «не найдено в подключённом corpus». Проверка: время до успешного выполнения выбранного onboarding сценария, точность ответов и частота неподтверждённых утверждений.

## Этапы и приёмка

M0: каталог сервисов/doc roots, Markdown parser, локальные links/anchors, owner/profile checks, OpenAPI files, findings и generic import. Внешние URL checks включаются после выбора сетевого режима. Это самостоятельный небольшой полезный модуль рядом с задачей/MR.

M1: Confluence выбранного типа, API/doc drift, review records, CI artifact evidence, Яндекс Wiki после capability-проверки. M2: Notion/Drive/Graph и поиск/feedback при реальном спросе. Native корпоративные wiki не обещаем до исследования их API.

Приёмка: разрешение revoked убирает индекс/snippets; 403 не становится broken link; moved page сохраняет id; одинаковый title не склеивает документы; generated timestamp не обновляет content hash; пустой/обрезанный body не получает полный pass; вредоносный Markdown URL не вызывает запрос к metadata endpoint; чужие build plugins не исполняются; недоказанный AI вывод остаётся кандидатом; отключённый источник не снижает качество до нуля.

Польза блока проверяется на конкретном случае: найти инструкцию, обновить reference или подготовить передачу сервиса. Расширение требует повторяемого рабочего сценария и использования результата.
