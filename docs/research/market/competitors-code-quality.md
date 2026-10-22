# Конкуренты craftPulse: качество кода, безопасность, AI-review

Дата обращения ко всем источникам: 2026-10-14. Использованы только первичные источники: сайты вендоров, документация, прайсы, официальные новости. Несколько фактов взяты из отраслевых СМИ (anti-malware.ru), это помечено отдельно.

---

## 1. CodeScene (Швеция)

1. **Что это.** Платформа поведенческого анализа кода: CodeHealth, технический долг, хотспоты, распределение знаний, delivery-метрики. Покупатели: техлиды и руководители разработки, в тарифе Pro ещё и менеджмент (портфель, команды). [pricing](../sources.md#ref-115)
2. **Что анализирует.** CodeHealth и техдолг, хотспоты, knowledge distribution, автоматическое ревью PR, покрытие кода (Pro), 30+ языков. [pricing](../sources.md#ref-115). Через трекер считает долю незапланированной работы и стоимость по хотспотам. [software-delivery](../sources.md#ref-116), [FAQ](../sources.md#ref-117)
3. **Интеграции.**
   - Git-хостинги: в облаке GitHub, Bitbucket, GitLab, Azure DevOps. On-prem работает с любым git по remote URL «на базовом уровне». [FAQ](../sources.md#ref-117)
   - Ревью PR/MR: GitHub, Bitbucket, GitLab, Azure, Gerrit (Gerrit только on-prem). Есть quality gates с профилями и отдельные гейты по путям или командам. [docs PR](../sources.md#ref-118)
   - Трекеры: Jira, Trello, Azure DevOps, GitHub Issues, GitLab, YouTrack. [FAQ](../sources.md#ref-117). Номер задачи нужно указывать в сообщении коммита. [docs PM 3.6.5](../sources.md#ref-119)
   - SourceCraft, GitFlic, Redmine, Yandex Tracker: нет в списке интеграций.
4. **Управленческие представления.** Pro: Software Portfolio overview, Team insights, Delivery insights. Enterprise: CodeHealth KPIs across teams. [pricing](../sources.md#ref-115). Также есть планируемая и незапланированная работа, lead times по веткам, статистика PR и квалити-гейтов, knowledge islands, анализ по закону Конвея. [software-delivery](../sources.md#ref-116).
5. **API и экспорт.** REST API v2 отдаёт проекты, анализы, CodeHealth, хотспоты, техдолг, delivery KPI, команды и разработчиков, авторизация по Bearer-токену. [REST API](../sources.md#ref-120). FAQ при этом пишет, что REST API пока есть только on-prem, а в облаке «скоро». Источники противоречат друг другу. [FAQ](../sources.md#ref-117). В Community Edition есть PDF-отчёты.
6. **Развёртывание.** Облако и on-prem (Docker), есть офлайн-режим. Сертификат ISO/IEC 27001. [FAQ](../sources.md#ref-117). Реестр РФ и ФСТЭК неприменимы.
7. **Цена.** Standard €18, Pro €27 за активного автора в месяц при годовой оплате. Enterprise по запросу. Активный автор: человек с коммитами за последние 3 месяца. Community Edition бесплатна для open source. [pricing](../sources.md#ref-115)
8. **Доступность в РФ.** Отдельной позиции по России нет, в FAQ нет ограничений по странам. [FAQ](../sources.md#ref-117). Иностранный вендор, оплата картой или по инвойсу.
9. **Пробелы для craftPulse.** Это ближайший концептуальный конкурент: есть exec-слой, delivery, связь с трекером. Но нет Redmine, Yandex Tracker, SourceCraft и GitFlic. Цена для 50 разработчиков на Pro составит около €16 тыс. в год, что заметно дороже гипотезы craftPulse. Внешние CI и сторонние анализаторы как источники данных не агрегирует. Риски оплаты и поставки для РФ.

## 2. CodeScoring, включая TQI (Россия)

1. **Что это.** Российская платформа безопасной разработки: модули Save (хранилище артефактов), OSA, SCA, Secrets, TQI. Покупатели: AppSec/DevSecOps, руководители разработки (для TQI). [codescoring.ru](../sources.md#ref-121)
2. **Что анализирует.** SCA и лицензии, секреты. TQI (Teams & Quality Intelligence) считает цикломатическую сложность во времени, клоны внутри проекта и между проектами, техдолг, профили авторов и их сходство, нагрузку и качество изменений разработчика, ретроспективные карты проекта. [docs functionality](../sources.md#ref-122), [TQI](../sources.md#ref-123). Процессные метрики TQI (Code rate, Velocity rate, Density rate) считаются на уровне проекта, минимальный период неделя. [TQI metrics](../sources.md#ref-124)
3. **Интеграции.** VCS: GitFlic, GitLab, GitHub, Bitbucket, Azure DevOps. CI: Jenkins, GitLab, GitFlic, TeamCity, плюс универсальный агент. Проверки в CI могут блокировать сборку. Трекер: только Jira. [docs functionality](../sources.md#ref-122). Результаты в MR выводятся через экспорт в GitLab Code Quality Report.
4. **Управленческие представления.** Есть профили проектов и разработчиков (Developers Inventory). Агрегации по команде или организации в документации метрик TQI нет. [TQI metrics](../sources.md#ref-124), [codescoring.ru/tqi](../sources.md#ref-125).
5. **API и экспорт.** Открытый документированный REST API. Экспорт в CycloneDX, SPDX, JUnit, SARIF, CSV, GitLab Dependency Scanning и Code Quality. [docs functionality](../sources.md#ref-122)
6. **Развёртывание.** Self-hosted: k3s для небольших команд, Kubernetes с PostgreSQL и S3 для продакшена. Только self-hosted. [docs functionality](../sources.md#ref-122). В реестр российского ПО внесён в марте 2022 года (по данным СМИ). [anti-malware](../sources.md#ref-126).
7. **Цена.** По запросу. Есть образовательная лицензия. [codescoring.ru](../sources.md#ref-121)
8. **Доступность в РФ.** Российский вендор, продаётся в РФ. [codescoring.ru](../sources.md#ref-121)
9. **Пробелы для craftPulse.** Ближайший российский конкурент по метрикам качества и авторства. Но из трекеров только Jira, нет процессного и delivery-контекста, нет вложенных ролевых дашбордов уровня организации. CodeScoring можно рассматривать и как источник данных: SARIF и REST API.

## 3. SonarQube (Server / Cloud / Community Build), SonarSource (Швейцария)

1. **Что это.** Стандартный инструмент статического анализа качества и безопасности с quality gates. Покупатели: техлиды, платформенные команды, AppSec. [plans](../sources.md#ref-127)
2. **Что анализирует.** Баги, code smells, уязвимости. Developer Edition поддерживает 37 языков, Enterprise 48. SCA и Advanced SAST входят в Advanced Security, который подключается к Enterprise. [server plans](../sources.md#ref-128), [editions](../sources.md#ref-129)
3. **Интеграции.** Декорация PR со статусом quality gate и блокировкой merge: GitHub, Bitbucket DC/Cloud, GitLab, Azure DevOps, начиная с Developer Edition. [editions](../sources.md#ref-129).
4. **Управленческие представления.** Портфели, дашборды и PDF/compliance-отчёты есть только в Enterprise и Data Center. [editions](../sources.md#ref-129). Процессных и delivery-метрик нет, анализ только на уровне кода.
5. **API и экспорт.** Web API и Web API v2 (measures, metrics и др.) с Bearer-токеном. [Web API](../sources.md#ref-002).
6. **Развёртывание.** Server self-managed, Cloud SaaS, Community Build open source. [plans](../sources.md#ref-127)
7. **Цена.**
   - Cloud: Free до 50k LOC в приватных проектах. Team от $34 в месяц до 100k LOC, но в FAQ той же страницы указано «от $32». Enterprise по запросу. [plans](../sources.md#ref-127)
   - Server: цена за инстанс в год по LOC, цифр на сайте нет. [server plans](../sources.md#ref-128). В блоге старше года для Developer указано «от $720 в год». [blog](../sources.md#ref-130)
   - Community Build: бесплатно.
8. **Доступность в РФ.** Отдельной позиции по России нет. Community Build как open source доступна.
9. **Пробелы для craftPulse.** Нет трекеров, процессов и delivery. Портфели только в дорогом Enterprise. Хороший источник данных через Web API.

## 4. JetBrains Qodana

1. **Что это.** Платформа качества кода на инспекциях JetBrains, с Qodana Cloud. Покупатели: техлиды, команды разработки. [about](../sources.md#ref-131)
2. **Что анализирует.** Статический анализ, 60+ языков. Покрытие доступно в Ultimate. Taint-анализ, vulnerability checker и license audit доступны в Ultimate Plus. [editions](../sources.md#ref-132), [about](../sources.md#ref-131)
3. **Интеграции.** Нативные интеграции с Azure Pipelines, TeamCity и GitLab CI/CD. Jenkins и Bitbucket Cloud подключаются через Docker. [CI](../sources.md#ref-133). Есть комментарий в GitLab MR и Code Quality report. [GitLab](../sources.md#ref-134). Анализ PR на GitHub. Quality gate есть во всех редакциях. [about](../sources.md#ref-131).
4. **Управленческие представления.** Qodana Insights показывает агрегаты по организации и командам: проблемы по severity, проекты, тренды, покрытие. Доступно только в Ultimate Plus. [insights](../sources.md#ref-135). Процессных метрик нет.
5. **API и экспорт.** Cloud API доступен в Ultimate Plus. [editions](../sources.md#ref-132). Результаты хранятся в SARIF (baseline). [CI](../sources.md#ref-133)
6. **Развёртывание.** Qodana Cloud. Self-hosted: минимум 5 контрибьюторов, цена индивидуальная (по данным поисковой выдачи со страницы прайса, сама страница отрисовывается JS). Community запускается локально через Docker и CLI. [blog 2026-03](../sources.md#ref-136)
7. **Цена.** Community бесплатно. Ultimate $5, Ultimate Plus $15 за активного контрибьютора в месяц при годовой оплате ($180 в год), минимум 3 контрибьютора. [blog 2026-03](../sources.md#ref-136), [licenses](../sources.md#ref-137)
8. **Доступность в РФ.** JetBrains 11.03.2022 приостановила продажи в России на неопределённый срок. [JetBrains blog](../sources.md#ref-138). Российские офисы закрыты, юрлицо ликвидируется. [update](../sources.md#ref-139)
9. **Пробелы для craftPulse.** Официально в РФ не купить. Нет трекеров и delivery. Exec-представления только в Ultimate Plus.

## 5. Snyk (США)

1. **Что это.** Платформа безопасности для разработчиков: SCA, SAST (Snyk Code), Container, IaC. Покупатели: AppSec, DevSecOps. [plans](../sources.md#ref-140)
2. **Что анализирует.** SCA, SAST, IaC, контейнеры. Enterprise добавляет AI Pentesting и безопасность coding-агентов. [plans](../sources.md#ref-140)
3. **Интеграции.** GitHub (включая Enterprise), GitLab, Bitbucket Cloud и Server/DC, Azure Repos. К self-hosted инстансам подключается через Snyk Broker. [SCM](../sources.md#ref-141). Jira: задачи заводятся вручную из UI, начиная с тарифа Team. [Jira](../sources.md#ref-142), [plans](../sources.md#ref-140)
4. **Управленческие представления. Процессных метрик нет.
5. **API и экспорт.** Большая часть API доступна только в Enterprise. [API](../sources.md#ref-143). CLI умеет выдавать SARIF (`--sarif`, `--sarif-file-output`). [CLI](../sources.md#ref-144)
6. **Развёртывание.** SaaS плюс Broker для приватных SCM.
7. **Цена.** Free $0 (5 проектов, 100 тестов SAST в месяц). Team от $25 в месяц, до 10 разработчиков. Enterprise на кредитах (1 кредит = $1; Snyk Code стоит 1 кредит за активного контрибьютора в день). [plans](../sources.md#ref-140)
8. **Доступность в РФ.** 15.03.2022 Snyk прекратил бизнес в России и Беларуси: ограничен доступ к бесплатному ПО, расторгнуты контракты, закрыты будущие продажи. [Snyk blog](../sources.md#ref-145)
9. **Пробелы для craftPulse.** Недоступен в РФ. Только безопасность, нет качества и процессов.

## 6. Semgrep (США)

1. **Что это.** AppSec-платформа: SAST (Code), SCA (Supply Chain), Secrets, AI-триаж и автофикс. Покупатели: AppSec. [pricing](../sources.md#ref-146)
2. **Что анализирует.** SAST на 35+ языков, SCA с reachability и SBOM, секреты, AI-детекция. [pricing](../sources.md#ref-146)
3. **Интеграции.** Облачные GitHub, GitLab, Bitbucket, Azure DevOps. Self-managed GitHub Enterprise Server, GitLab Self-Managed, Bitbucket DC. Network Broker для внутренних инстансов. [connect SCM](../sources.md#ref-147). PR/MR-комментарии, тикеты в Jira, CI на всех тарифах. [pricing](../sources.md#ref-146).
4. **Управленческие представления.** Dashboard показывает security posture: бэклог, guardrails (что поймано в PR), медианный возраст находок, есть экспорт в PDF. [dashboard](../sources.md#ref-148). Процессных метрик нет.
5. **API и экспорт.** REST API на тарифах Teams и Enterprise. [pricing](../sources.md#ref-146). Findings API: `GET /api/v1/deployments/{slug}/findings`. [API](../sources.md#ref-149). CLI выдаёт SARIF, экспорт в CSV ограничен 10 000 находок. [SARIF](../sources.md#ref-150)
6. **Развёртывание.** SaaS-платформа. Self-managed репозитории поддерживаются только в Enterprise. Community Edition (CLI) open source. [pricing](../sources.md#ref-146)
7. **Цена.** Free до 10 контрибьюторов и 10 приватных репозиториев. Teams: Code $30, Supply Chain $30, Secrets $15 за контрибьютора в месяц. Enterprise по запросу. [pricing](../sources.md#ref-146)
8. **Доступность в РФ.** Отдельной позиции по России нет.
9. **Пробелы для craftPulse.** Только AppSec, нет трекеров с процессным контекстом и delivery. Подходит как источник SARIF и данных через API.

## 7. CodeRabbit (США)

1. **Что это.** AI-ревью pull request'ов, плюс IDE и CLI. Покупатели: руководители разработки, команды. [pricing](../sources.md#ref-151)
2. **Что анализирует.** AI-ревью диффов, кастомные pre-merge checks (начиная с Team), Deep Scan и модуль Security. [pricing](../sources.md#ref-151)
3. **Интеграции.** GitHub.com, GitHub Enterprise Server, GitLab.com и self-managed, Bitbucket Cloud и DC, Azure DevOps. [platforms](../sources.md#ref-152). Jira и Linear на всех тарифах. [pricing](../sources.md#ref-151).
4. **Управленческие представления.** Dashboard: Summary (throughput, сэкономленное время, принятие комментариев), Time Metrics (ожидание до, во время и после ревью), Organization Trends, отчёты по расписанию. Фильтры по репозиторию, пользователю и команде. [dashboard](../sources.md#ref-153). Это частично процессные метрики, но только по PR.
5. **API и экспорт.** CSV по PR. Metrics Data API и API access только в Enterprise. [dashboard](../sources.md#ref-153), [pricing](../sources.md#ref-151)
6. **Развёртывание.** SaaS. Self-hosted только для Enterprise от 500 мест. [platforms](../sources.md#ref-152)
7. **Цена.** Essentials $24, Team $48, Advanced $72 за пользователя в месяц при годовой оплате. Enterprise по запросу. Бесплатно для публичных репозиториев. Перерасход: $0,25 за файл. [pricing](../sources.md#ref-151)
8. **Доступность в РФ.** В ToS есть общая оговорка о соблюдении экспортного и санкционного законодательства, Россия не названа. [ToS](../sources.md#ref-154).
9. **Пробелы для craftPulse.** Метрики относятся к PR; сквозные сценарии с несколькими трекерами и уровнями доступа требуют отдельного сравнения. API закрыт в Enterprise.

## 8. Svace (ИСП РАН, Россия)

1. **Что это.** Глубокий статический анализатор для высоконадёжного и сертифицируемого ПО. Покупатели: разработчики системного ПО, испытательные лаборатории. [ispras svace](../sources.md#ref-155)
2. **Что анализирует.** C/C++ (до C++20), C#, Java, Kotlin, Go, Python, Scala, VB.NET. JavaScript поддерживается облегчённо. Больше 1000 типов ошибок, больше 70 критических классов. [ispras svace](../sources.md#ref-155)
3. **Интеграции.** CLI для CI/CD, VS Code, web-IDE. [ispras svace](../sources.md#ref-155)
4. **Управленческие представления.** Svacer (сервер истории) даёт дашборды, роли, разметку, API.
5. **API и экспорт.** API Svacer, экспорт в PDF, CSV, JSON, SARIF (в обе стороны: импорт и экспорт). [ispras svace](../sources.md#ref-155)
6. **Развёртывание.** On-prem (Linux, Windows, macOS). Реестр российского ПО №4047. Заявлено соответствие ГОСТ Р 71207-2024 и требованиям ФСТЭК. Наличие сертификата ФСТЭК не заявлено. [ispras svace](../sources.md#ref-155)
7. **Цена.** По запросу, sdl@ispras.ru. [ispras svace](../sources.md#ref-155)
8. **Доступность в РФ.** Российский разработчик, больше 200 компаний в РФ. [ispras svace](../sources.md#ref-155)
9. **Пробелы для craftPulse.** Инструмент для инженеров и лабораторий. Нет трекеров и exec-слоя. Потенциальный источник данных через SARIF.

## 9. Solar appScreener («Солар», группа «Ростелеком»)

1. **Что это.** AppSec-платформа SAST, DAST и OSA (SCA, supply chain, лицензии), умеет анализировать бинарный код. Покупатели: служба ИБ, офицеры безопасности. [rt-solar](../sources.md#ref-156)
2. **Что анализирует.** Уязвимости и недекларированные возможности. На странице заявлено 36 языков, в FAQ той же страницы 33. Есть бинарный анализ. [rt-solar](../sources.md#ref-156)
3. **Интеграции.** VCS-хостинги GitHub, GitLab, Bitbucket: запуск сканирования по webhook на push и tag. CI: Jenkins, TeamCity, Azure DevOps. Трекеры: Jira. Есть интеграция с SonarQube. [user guide 3.14.2](../sources.md#ref-030).
4. **Управленческие представления.** Мониторинг изменения уровня безопасности приложения. [user guide](../sources.md#ref-030).
5. **API и экспорт.** Отчёты в HTML, PDF, CSV, DOCX. JSON и SARIF доступны через API. [user guide](../sources.md#ref-030). Открытый JSON API и CLI. [брошюра](../sources.md#ref-157)
6. **Развёртывание.** On-premise и SaaS из облака «Солар». [брошюра](../sources.md#ref-157). Сертифицирован ФСТЭК (ОУД4), входит в реестр. [rt-solar](../sources.md#ref-156)
7. **Цена.** По запросу, есть бесплатный пробный период. [rt-solar](../sources.md#ref-156)
8. **Доступность в РФ.** Российский вендор.
9. **Пробелы для craftPulse.** Продукт для ИБ, не для менеджмента разработки. Нет качества и delivery. Источник данных через SARIF и JSON API.

## 10. PVS-Studio (ООО «ПВС», Россия), добавлен

1. **Что это.** Статический анализатор качества и безопасности (SAST) для C, C++, C#, Java, TS, JS, Go. [pvs fstec](../sources.md#ref-158)
2. **Что анализирует.** Больше 1000 диагностик. Заявлено соответствие ГОСТ Р 71207-2024 и Методике ФСТЭК от 12.05.2026. [pvs fstec](../sources.md#ref-158)
3. **Интеграции.** Проверка PR/MR (только для изменённых файлов), рассылка уведомлений авторам кода, экспорт в SonarQube: всё это начиная с Enterprise. DefectDojo доступен в Team. [лицензии](../sources.md#ref-159). GitLab Code Quality (отображается в MR) и SARIF для GitHub. [docs 0038](../sources.md#ref-013).
4. **Управленческие представления.** PVS-Studio Atlas: разметка, статистика, оценка качества проекта. Вышел в 2026 году, в прайсе помечен «скоро». [Atlas docs](../sources.md#ref-160), [лицензии](../sources.md#ref-159). Процессных метрик нет.
5. **API и экспорт.** plog-converter выдаёт SARIF, GitLab, DefectDojo, HTML и др. [docs 0038](../sources.md#ref-013)
6. **Развёртывание.** Локально и на CI, есть поддержка отечественных ОС (Astra, РЕД ОС). Реестр российского ПО №9837 от 18.03.2021. Сертификата ФСТЭК нет: вендор называет сертификацию добровольной. [pvs fstec](../sources.md#ref-158)
7. **Цена.** По запросу в RUB, EUR или USD. Тарифы: Team (до 10 разработчиков), Enterprise 30 (10–30), Enterprise 50+. Срок лицензии 1–3 года. Бесплатные лицензии для студентов, open source и MVP. [покупка](../sources.md#ref-161), [лицензии](../sources.md#ref-159)
8. **Доступность в РФ.** Российский вендор, продаётся в РФ.
9. **Пробелы для craftPulse.** Анализатор, а не агрегатор. Хороший источник SARIF.

## 11. PT Application Inspector (Positive Technologies, Россия), добавлен

1. **Что это.** AppSec-платформа: SAST, SCA, встроенный DAST с автоподтверждением уязвимостей, ML-модуль MOLOT для поиска закладок, IaC, поиск секретов, ИИ-триаж PT Naira. [ptsecurity.com/products/ai](../sources.md#ref-162)
2. **Языки.** Java, Scala, C#, PHP, JS/TS, Python, Go, Ruby, Kotlin, Objective-C, Swift, SQL, Solidity, Dart, 1С. [ptsecurity](../sources.md#ref-162)
3. **Интеграции.** CI: Jenkins, TeamCity, GitLab CI (через CLI), Azure. IDE: JetBrains. Трекер: Jira. [описание продукта, PDF](../sources.md#ref-163). Централизованные quality gates в CI/CD. [datasheet](../sources.md#ref-164).
4. **Управленческие представления.** Сервис управления сканированиями (очереди, приоритеты).
5. **API и экспорт.** REST API (Swagger). [описание продукта](../sources.md#ref-163).
6. **Развёртывание.** On-prem (пилоты разворачиваются у заказчика). Реестр российского ПО №1253, сертификат ФСТЭК есть (PDF на странице продукта). [ptsecurity](../sources.md#ref-162)
7. **Цена.** По запросу, зависит от объёма кода и размера команды. Базовый ИИ-триаж бесплатен до 01.03.2027. [ptsecurity](../sources.md#ref-162)
8. **Доступность в РФ.** Российский вендор, есть отдельный поток продаж для российских компаний.
9. **Пробелы для craftPulse.** Только безопасность, нет качества и delivery. Источник данных через REST API.

## Сводная матрица

| Продукт | Охват | Git-хостинги | MR/CI gate | Связь с трекером | Процессные / exec-представления | API / экспорт | On-prem | Цена (публичная) | РФ |
|---|---|---|---|---|---|---|---|---|---|
| CodeScene | Качество, техдолг, поведенческий анализ, delivery | GitHub, GitLab, Bitbucket, Azure; on-prem любой git (базово); Gerrit | Да (PR review + gates) | Jira, Trello, Azure DevOps, GitHub Issues, GitLab, YouTrack | **Да**: портфель, команды, delivery (Pro), KPI (Ent) | REST API v2 (по FAQ только on-prem); PDF | Да | €18 / €27 за активного автора в месяц | Иностранный вендор |
| CodeScoring (+TQI) | SCA, секреты, качество (TQI) | GitFlic, GitLab, GitHub, Bitbucket, Azure | CI-блокировка; GitLab Code Quality | Jira | Частично: профили авторов и проектов, метрики изменений по проекту | REST API, SARIF, CycloneDX, SPDX, CSV | Да (k3s/K8s) | По запросу | Российский, реестр (2022) |
| SonarQube | Качество + SAST (SCA в Ent) | GitHub, GitLab, Bitbucket, Azure | Да | - | Портфели только Ent/DC; процессов нет | Web API v1/v2 | Да (Server) | Cloud: Free до 50k LOC, Team от $34 в месяц; Server по запросу | - |
| Qodana | Качество + безопасность (Ult+) | GitHub, GitLab, Bitbucket (CI), Azure | Да (quality gate, MR-комментарий в GitLab) | - | Insights по организации и командам (Ult+) | Cloud API (Ult+), SARIF | Self-hosted (≥5 контрибьюторов) | Community бесплатно; $5 / $15 за контрибьютора в месяц | **Продажи в РФ приостановлены (2022)** |
| Snyk | SCA, SAST, IaC, контейнеры | GitHub, GitLab, Bitbucket, Azure (+Broker) | Да | Jira (Team+) | Нет | API (в основном Ent), SARIF | Нет (Broker) | Free; Team от $25 в месяц; Ent на кредитах | **Ушёл из РФ (15.03.2022)** |
| Semgrep | SAST, SCA, секреты, AI | GitHub, GitLab, Bitbucket, Azure (+self-managed) | Да | Jira | Только security posture | REST API (Teams+), SARIF | Только репозитории (Ent) | Free до 10; $30 / $30 / $15 за контрибьютора в месяц | - |
| CodeRabbit | AI-ревью PR | GitHub, GitLab, Bitbucket, Azure (+self-managed) | Да (pre-merge checks) | Jira, Linear | Метрики PR и времени ревью | CSV; API только Ent | Ent от 500 мест | $24 / $48 / $72 за пользователя в месяц | Общая санкционная оговорка в ToS |
| Svace | Глубокий SAST (C/C++ и др.) | - | CLI в CI | - | Нет (дашборды Svacer) | API Svacer, SARIF, JSON, CSV, PDF | Да | По запросу | Российский, реестр №4047 |
| Solar appScreener | SAST, DAST, SCA, бинарный анализ | GitHub, GitLab, Bitbucket (webhook) | CI (Jenkins, TeamCity, Azure) | Jira | Нет | JSON API, SARIF; PDF, DOCX, CSV, HTML | Да + SaaS | По запросу | Российский, ФСТЭК, реестр |
| PVS-Studio | SAST, качество | GitLab (Code Quality), GitHub (SARIF) | PR/MR-проверка (Ent) | - | Нет (Atlas, 2026) | SARIF, GitLab, DefectDojo, SonarQube | Да | По запросу (Team / Ent30 / Ent50+) | Российский, реестр №9837; без сертификата ФСТЭК |
| PT Application Inspector | SAST, SCA, DAST, IaC, секреты, ML | GitLab CI через CLI | Quality gates в CI | Jira | Нет | REST API | Да | По запросу | Российский, ФСТЭК, реестр №1253 |

## Выводы для craftPulse (по фактам выше)

- Из всей группы только CodeScene полноценно закрывает exec-, портфельный и delivery-слой с привязкой к трекерам. Но Redmine, Yandex Tracker, SourceCraft и GitFlic он не поддерживает, а цена за автора на порядок выше гипотезы craftPulse.
- Среди российских игроков ближе всего к craftPulse стоит CodeScoring.TQI с метриками авторов и сложности и интеграцией с GitFlic. Но у него нет трекерного контекста, кроме Jira, и нет агрегации по организации.
- Остальные (Sonar, Semgrep, Svace, appScreener, PVS, PT AI) работают на уровне кода и AppSec. Почти все отдают SARIF или имеют REST API, поэтому они скорее источники данных для craftPulse, чем прямые конкуренты.
- JetBrains и Snyk официально ушли из РФ. У Sonar, Semgrep и CodeRabbit отдельной позиции по России нет.
