# Платформы и интеграции для craftPulse: встроенная аналитика и российская экосистема

Дата обращения ко всем источникам: 2026-10-14. Источники только первичные: сайты, документация и прайсы вендоров, официальные заявления, регуляторы, отраслевой отчёт State of DevOps Russia. Пересказы СМИ используются только там, где первоисточник не нашёлся, и это помечено. Это не значит, что его нет.

---

## 0. Главное

1. **Что реально стоит у российских команд (State of DevOps Russia 2025, Express 42 / «Флант», >3300 респондентов):**
   - Хостинг кода: **GitLab CE on‑prem 47,2 %**, GitLab EE on‑prem 18,6 %, GitHub 28 %, Bitbucket на своих серверах 13,3 %, gitlab.com бесплатный 7,1 % и платный 4,4 %, Bitbucket Cloud 4,4 %, Gogs/Gitea 5,2 %, **GitFlic 2,9 %** (в 2024 было 3,7 %), «другое российское решение» 8,3 %.
   - CI/CD: **GitLab CI 64,5 %** (в 2024 было 56,7 %), **Jenkins 34,7 %**, Argo CD 21,8 %, GitHub Actions 12 %, TeamCity 8,5 %, Flux 6,2 %, Bamboo 3,1 %, другое 10,4 %. Без CI/CD работают 7,6 %.
   - Трекеры по категориям: зарубежный SaaS (GitLab/GitHub/Bitbucket Issues, Jira, YouTrack…) 37,1 %; **зарубежный on‑prem (Jira, YouTrack) 35,1 %, и эта доля растёт**; российский SaaS (Yandex Tracker, Kaiten, «ПланФикс», «Мегаплан», Битрикс24) 24,1 %; российский on‑prem (YouGile, Битрикс24) 15,2 %; open source на своих серверах (Redmine, Taiga, GitLab Issues) 18,4 %.
   - Облака: Yandex Cloud 47,4 %, Selectel 17,5 %, Cloud.ru 14,3 %, AWS 14,1 %.
   - Источник: PDF‑отчёт, страницы 20–24 (https://express42.com/wp-content/themes/express42/assets/docs/StateOfDevOpsRussia2025.pdf, лендинг https://express42.com/state-of-devops/).
2. **«Ничего не делать» - это в первую очередь self‑managed GitLab CE.** В Free почти нет аналитики: DORA и Insights есть только в Ultimate, Productivity, MR и Contribution analytics - с Premium. Купить Premium или Ultimate напрямую у GitLab в РФ нельзя: в марте 2022 GitLab приостановил новый бизнес в России. У команд на CE остаётся дыра в аналитике, и её закрывает агрегатор.
3. **Jira и YouTrack живут в РФ как необновляемые on‑prem инсталляции.** Atlassian перестал продлевать лицензии с 31.10.2022, Data Center уходит в read‑only. JetBrains свернул продажи в РФ. При этом Jira on‑prem всё ещё массово используется, и миграции с неё идут медленно.
4. **Российские DevOps‑платформы (SourceCraft, GitVerse, GitFlic) дают API, но не дают инженерной аналитики** уровня DORA/VSA. Нашлись только статистика коммитов в GitFlic, статистика Code Assistant и дашборды безопасности в SourceCraft.
5. **Специализированного российского продукта инженерной аналитики** (аналога Jellyfish/LinearB) первичными источниками не найдено. Ближайший кандидат - **PanDev Metrics**, казахстанский стартап с русскоязычным рынком. Остальное - open source (Apache DevLake), самописные решения на Grafana/Metabase или внутренние инструменты компаний (кейсы на Хабре).

---

## Часть A. Встроенная аналитика платформ, которые у клиента уже есть («do nothing»)

### A1. GitLab

| Функция | Тариф (по docs.gitlab.com) | Комментарий |
|---|---|---|
| Value Stream Analytics | Free / Premium / Ultimate | В Free только уровень проекта, один дефолтный value stream. Нет кастомных стадий, агрегации, DORA и сводки cycle/lead time. Групповой уровень - с Premium. |
| DORA metrics (+ REST API) | **Ultimate** | Стабильностные метрики требуют инцидентов в GitLab. Если инциденты в Jira, их нужно реплицировать. |
| Insights | **Ultimate** | - |
| Productivity analytics | Premium, Ultimate | - |
| Merge request analytics | Premium, Ultimate | - |
| Contribution analytics | Premium, Ultimate | - |
| Code Quality | Виджет в MR есть во всех тарифах. Отчёт в пайплайне - Premium+. MR changes view и сводка качества проекта - Ultimate (сводка в beta, за фичефлагом). | Сканирование через CodeClimate deprecated с 17.3, удаление запланировано в 19.0. Рекомендуется импортировать отчёты внешних анализаторов. |

- Цены: Free $0; Premium $29/польз./мес (при оплате за год); Ultimate - по запросу. https://about.gitlab.com/pricing/
- **Россия.** Официальная позиция: «GitLab has suspended new business in Russia and Belarus» (11.03.2022). Отдельного заявления про действующие self‑managed лицензии на странице нет. https://about.gitlab.com/blog/gitlab-actions-to-date-regarding-russian-invasion-of-ukraine
- Договор подписки ссылается на «Embargoed Countries» по санкционному режиму США. https://handbook.gitlab.com/handbook/legal/subscription-agreement/
- Реселлер Softmart заявляет, что продаёт лицензии GitLab в РФ и СНГ, называет себя «GitLab Open Partner» и параллельно продвигает российский AppSec.Code. Официальный статус партнёрства не подтверждён. https://gitlab.softmart.ru/
- **Yandex Cloud Managed Service for GitLab** - управляемые инстансы GitLab в Yandex Cloud. Оплачиваются ВМ и диск. Редакция в документации не названа, но есть собственные «правила ревью» как замена EE Approval Rules. https://yandex.cloud/ru/docs/managed-gitlab/ , https://yandex.cloud/ru/docs/managed-gitlab/concepts/managed-gitlab-vs-custom-installation , https://yandex.cloud/ru/docs/managed-gitlab/pricing
- Mos.Hub (ДИТ Москвы) построен на базе GitLab, вход через Mos ID. Данные из СМИ (CNews/Хабр), официальную документацию открыть не удалось. https://zoom.cnews.ru/news/item/571881 , https://habr.com/ru/news/757280

Для craftPulse: GitLab REST/GraphQL API есть во всех редакциях. API DORA доступен только в Ultimate, поэтому для CE DORA придётся считать самим из deployments, MR и pipelines.

Источники: https://docs.gitlab.com/user/analytics/ , https://docs.gitlab.com/user/group/value_stream_analytics/ , https://docs.gitlab.com/user/analytics/dora_metrics/ , https://docs.gitlab.com/user/analytics/productivity_analytics/ , https://docs.gitlab.com/user/project/insights/ , https://docs.gitlab.com/user/analytics/merge_request_analytics/ , https://docs.gitlab.com/user/group/contribution_analytics/ , https://docs.gitlab.com/ci/testing/code_quality/

### A2. GitHub

- Цены: Free $0; Team $4/польз./мес; Enterprise от $21/польз./мес (на странице это цена первых 12 месяцев). Code Security, Secret Protection и Advanced Security продаются как add‑on, их цены на странице тарифов не указаны. https://github.com/pricing
- Copilot metrics: API на уровнях enterprise, org, repo и user; дашборды usage, code generation и impact; NDJSON‑экспорт. Метрики: DAU, acceptance rate, строки кода, жизненный цикл PR (число созданных и смёрдженных PR, медианное время до merge). Аналитика на уровне org доступна с 12.12.2025. https://docs.github.com/en/copilot/concepts/copilot-metrics
- **Россия (GitHub and Trade Controls):** Россия не перечислена среди санкционных регионов для аккаунтов GitHub.com (там Крым, ДНР/ЛНР, Куба, Иран, КНДР). Но **GitHub Enterprise Server нельзя продавать или экспортировать в страны группы E:1, куда входят Россия и Беларусь, и Copilot тоже нельзя продавать в Россию.** https://docs.github.com/en/site-policy/other-site-policies/github-and-trade-controls
- Доля GitHub среди российских респондентов - 28 %, GitHub Actions - 12 % (SoDR 2025).

### A3. Jira / Atlassian

- **Уход из РФ.** С марта 2022 прекращены продажи новых лицензий и приложений Marketplace. С 31.10.2022 лицензии клиентов из РФ и Беларуси не продлеваются. Для Server перестают продлеваться поддержка и обновления. **Data Center после окончания лицензии переходит в read‑only**, без новых версий и поддержки. https://www.atlassian.com/blog/announcements/atlassian-stands-with-ukraine
- Глобальный EOL Data Center: прекращение продаж новым клиентам 30.03.2026, последнее продление 30.03.2028, конец жизни в 2029 году. Официальный пост Atlassian не открывался, данные из Atlassian Developer Community. https://community.developer.atlassian.com/t/from-data-center-to-cloud-the-next-chapter-for-marketplace-apps/95173
- **Atlassian Analytics + Data Lake** входят только в Cloud Enterprise (Jira, JSM, Confluence), для РФ недоступны. https://www.atlassian.com/platform/analytics , https://www.atlassian.com/blog/company-news/analytics-general-availability
- **Compass** (каталог компонентов, scorecards, метрики «здоровья»): Free $0 до 3 полных пользователей; Standard $7,67; Premium $23,96 за польз./мес. DORA на странице тарифов не упоминается. Это Cloud‑продукт, клиенту из РФ его купить нельзя. https://www.atlassian.com/software/compass/pricing
- Для craftPulse: Jira Server/DC REST API v2 стоит в РФ массово (зарубежные on‑prem трекеры - 35,1 %). Коннектор к Jira Server/DC остаётся приоритетным, хотя вендор ушёл.

### A4. JetBrains (YouTrack, TeamCity)

- 11.03.2022 JetBrains приостановила продажи и R&D в России на неопределённый срок. В декабре 2022 сообщила о закрытии российских офисов и ликвидации юрлица. https://blog.jetbrains.com/blog/2022/03/11/jetbrains-statement-on-ukraine/ , https://blog.jetbrains.com/blog/2022/12/06/update-on-jetbrains-statement-on-ukraine/
- TeamCity по SoDR 2025 - 8,5 % (в 2024 было 11,3 %), доля падает.

### A5. Санкционный контекст для западных SaaS (важно для позиционирования)

- **OFAC (США), действует с 12.09.2024:** запрещено предоставлять лицам в России IT‑консалтинг, а также поддержку и облачные (SaaS) услуги для «enterprise management software». В определение прямо входят **BI и project management software**. https://ofac.treasury.gov/faqs/1187 , https://ofac.treasury.gov/faqs/1184
- **ЕС, 12‑й пакет (рег. 2023/2878):** запрет на поставку enterprise management software и ПО для проектирования и производства российскому правительству и российским лицам. Источник - обзор юрфирмы, текст Official Journal не открывался. https://www.clearygottlieb.com/news-and-insights/publication-listing/further-eu-sanctions-against-russia-council-adopts-12th-package
- Вывод: Jellyfish, LinearB, Atlassian Analytics, Compass и подобные облачные продукты для российских юрлиц фактически закрыты. Это аргумент в пользу российского агрегатора.

### A6. Grafana и self‑built (Apache)

- **Grafana** - самый популярный инструмент визуализации в РФ: 69,8 % по SoDR 2025. Плагины GitLab и Jira data source - **Enterprise‑плагины** с лицензией enterprise plugins, в OSS‑Grafana их нет. Плагин GitLab заявляет топ контрибьюторов, коммиты в день и деплои в день. https://grafana.com/grafana/plugins/grafana-gitlab-datasource/ , https://grafana.com/grafana/plugins/grafana-jira-datasource/
- Доступ к enterprise‑бинарникам Grafana из РФ: на форуме Grafana есть сообщения об ошибке 451 «geofence:blocked» и ответ, что enterprise‑бинарники не распространяются в РФ из‑за экспортных ограничений. Это форум, не официальная политика. https://community.grafana.com/t/cant-download-grafana-9-0-2-from-russia/68391
- Российский аналог Grafana «Графиня» («Лаборатория Числитель», 2025) - только по СМИ. https://www.anti-malware.ru/news/2025-07-29-111332/46818
- **Apache DevLake** (инкубатор ASF, стабильная версия v1.0). Коннекторы: GitHub, GitLab, Jira, Jenkins, SonarQube, CircleCI, PagerDuty, Opsgenie; в beta Bitbucket, Azure DevOps, Zentao, TAPD; есть incoming webhook. Встроенные DORA‑дашборды на Grafana. **Коннекторов к Yandex Tracker, GitFlic, SourceCraft, GitVerse и Kaiten нет.** Это главный бесплатный конкурент «собери сам». https://devlake.apache.org/ , https://devlake.apache.org/docs/Overview/SupportedDataSources

---

## Часть B. Российская экосистема

### B1. Git‑хостинг / DevOps‑платформы

#### SourceCraft (Yandex B2B Tech)
- Что это: Git‑хостинг, PR и код‑ревью, CI/CD («кубы», поддержка конфигов GitHub Actions с конвертацией), Issues, Packages, Sites, AI Code Assistant (агентский режим), миграция и зеркалирование с GitHub (PR и issues переносятся только при первичной синхронизации). Открыт публично с 2025 года. https://sourcecraft.dev/ , https://yandex.cloud/ru/blog/posts/2025/02/sourcecraft
- AppSec (SourceCraft Security): сканер секретов, SCA, SAST, лицензионные риски и Policy as Code, KICS для IaC, история сканирований, сводная статистика рисков. Платформа заявляет соответствие 152‑ФЗ, PCI DSS и ГОСТ 57580. https://sourcecraft.dev/portal/docs/ru/sourcecraft/security/security-overview
- Аналитика для craftPulse: **DORA, VSA и метрик ревью нет**. Есть статистика использования Code Assistant (май–июнь 2026), security‑дашборды и рейтинг репозиториев. https://sourcecraft.dev/portal/docs/en/sourcecraft/release-notes.md
- **Публичный REST API**: `https://api.sourcecraft.tech`, Bearer PAT; в CI есть токен `SOURCECRAFT_TOKEN`. Версия 0.0.1, «Bleeding edge». Разделы: Issues (статусы, лейблы, связанные PR), Repository (PullRequest с комментариями, ревьюерами и операциями; релизы, milestones), Organization (команды, роли), Quota, Secrets, CI/CD. Часть CI‑эндпоинтов (запуск workflow, артефакты) в разделе **Withdrawn**, поэтому чтение результатов CI через API под вопросом. Отдельных разделов про branches/commits нет. Вебхуки есть с Q4 2025. https://sourcecraft.dev/portal/docs/en/api-ref/ , https://sourcecraft.dev/portal/docs/en/sourcecraft/operations/api-start.md
- Интеграция с **Yandex Tracker**: PR и задачи связываются с задачами Трекера, статус виден в Трекере (май–июнь 2026).
- **Цены (с 01.10.2026), за активного участника в месяц, с НДС:** Free 0 ₽ (1000 мин CI, 2 ГБ); Start 1190 ₽; Go 2490 ₽; Go X5 5490 ₽; Go X15 11490 ₽ (платные: 3000 мин CI, 10 ГБ, 20 параллельных CI). Тарифы в основном различаются нейрокредитами. **SourceCraft Security - 3000 ₽ за активного коммиттера в месяц**, триал 90 дней. Дополнительный CI - 0,8 ₽/мин. Архивный тариф SourceCraft Pro - 250 ₽. https://sourcecraft.dev/portal/docs/ru/sourcecraft/pricing
 Есть только кастомный воркер CI в Kubernetes.

#### GitVerse (СберТех)
- Что это: Git‑хостинг, код‑ревью, CI/CD (облачные сборки, своё описание пайплайнов), трекер задач (Kanban/Scrum, «Задачи 2.0»), wiki, артефакты, AI‑ассистент GigaCode. https://gitverse.ru/ , https://gitverse.ru/docs
- **On‑prem** анонсирован на ЦИПР‑2026 (май 2026), в том числе для изолированных контуров. Корпоративные клиенты могут тестировать бесплатно. https://www.comnews.ru/content/245326/2026-05-18/2026-w21/1018/platforma-gitverse-sbera-teper-dostupna-lokalno-polnyy-kontrol-koda-i-bezopasnost-dlya-rossiyskogo-biznesa
- **Публичный API**: `https://api.gitverse.ru`, Bearer‑токен, заголовок `Accept: application/vnd.gitverse.object+json;version=1`. Ресурсы: репозитории, ветки, коммиты, compare, **pull requests (файлы, коммиты, ревью, комментарии)**, issues, проекты, **CI/CD (workflows, runs, jobs, logs, artifacts, runners)**, вебхуки, организации и команды. Пагинация через Link‑заголовок, есть 429. Структура похожа на GitHub REST, но совместимость не заявлена. Статус (GA/beta) на странице не указан. По СМИ - «ранний доступ». https://gitverse.ru/docs/developers/public-api , https://gitverse.ru/docs/developers/webhooks
 Есть только аудит‑лог (Enterprise и OnPrem).
- **Цены:** Free 0 ₽ (2000 мин CI); Pro 399 ₽/польз./мес (при оплате за год, 5000 мин); Enterprise 1666 ₽ (50 000 мин, сканирование безопасности, SAML/LDAP); OnPrem по запросу, минимум 5 мест. https://gitverse.ru/home/pricing/
- Доля в SoDR отдельно не выделена (входит в «другое российское решение», 8,3 %).

#### GitFlic («Группа Астра»)
- Что это: Git‑хостинг, MR, CI/CD, «Проблемы» (issues), реестр пакетов (Docker, Maven, npm, PyPI, Go и др.), SAST/DAST/SCA. Есть SaaS и Self‑hosted. https://gitflic.ru/
- **API**: REST (`api.gitflic.ru`, access token). Группы: project, mergeRequest и обсуждения MR, commit, **cicd** (список пайплайнов с фильтром по ветке и тегу, jobs, статусы SUCCESS/FAILED/…, артефакты), issue и комментарии, webhooks. Эндпоинтов отчётов о тестах в CI API нет. https://docs.gitflic.ru/ , https://docs.gitflic.ru/api/cicd/
- Аналитика: вкладка «Статистика» проекта - графики коммитов по ветке и периоду, форки, подписчики. Для self‑hosted есть дашборд администратора и metrics‑сервер (actuator). Метрик MR и CI нет. https://docs.gitflic.ru/project/stat/
- **Цены:** SaaS бесплатно до 5 человек; SaaS «Компания» от 750 ₽/польз./мес; Self‑hosted бесплатный (без ограничения пользователей, с CI/CD и API); **Self‑hosted Pro от 243 000 ₽/год** (SAST, правила апрува, поиск по коду); Enterprise по запросу (merge trains, SAST/DAST/SCA, SSO SAML, аудит). https://gitflic.ru/price
- Доля: 2,9 % в SoDR 2025 (3,7 % в 2024).

#### EvaGit / EvaCI (EvaTeam)
- Экосистема EvaTeam позиционируется как «замена Jira, Confluence, MS Project, GitLab». EvaGit - «российская замена Bitbucket, GitLab, GitHub». Вендор заявляет 650 компаний‑клиентов. https://evateam.ru/
- Пакет EvaDev «Расширенный» (EvaProject + EvaGit + EvaCI + EvaTest + EvaWiki и др.) - 2590 ₽/польз./мес, минимум 5 пользователей. Есть бесплатная on‑prem версия «конвейер разработки на 10 пользователей». https://evateam.ru/evaproject/pricing/

#### «Сфера» (Т1)
- По СМИ: «Сфера.Код» (хранение кода), «Сфера.CI/CD», «Сфера.Дистрибутивы», есть набор для 1С и AI‑ассистент для ревью MR. https://zoom.cnews.ru/soft/news/line/2024-11-28_platforma_sfera_predstavila/print

#### Mos.Hub
- Городская платформа ДИТ Москвы на базе GitLab (по СМИ). https://zoom.cnews.ru/news/item/571881

#### VK Tech Dev Platform
- По СМИ: в Dev Platform VK Cloud интегрирован трекер TeamStorm. https://hi-tech.mail.ru/news/121427-vk-tech-integriroval-v-reshenie-dlya-razrabotchikov-dev-platform-instrument-upravleniya-proektami/

### B2. Трекеры задач

#### Yandex Tracker
- Цена: **485 ₽/польз./мес** (по странице 360.yandex.ru). Подключается в составе тарифов Яндекс 360 для бизнеса (для организаций до 250 человек) или как отдельная опция. Бесплатного тарифа на 5 пользователей официальные источники не подтверждают. https://360.yandex.ru/business/tracker/ , https://yandex.ru/support/tracker/ru/pricing
- Встроенная аналитика (виджеты дашборда): **Cycle time** (перцентили, разбивка по статусам, целевая линия), **Flow**, **Team velocity**, сводные таблицы, статистика задач, график событий, создано/решено. Burndown и CFD в списке виджетов нет. https://yandex.ru/support/tracker/ru/user/widgets , https://yandex.ru/support/tracker/ru/user/cycle-time
- Интеграция с Git: подключение репозиториев GitHub, GitLab и Bitbucket (облачные опрашиваются, для self‑hosted GitHub/GitLab настраиваются вебхуки). Коммиты и MR связываются по ключу задачи. Есть интеграция с SourceCraft. Метрик по коду нет. https://yandex.ru/support/tracker/ru/user/add-repository
- **API**: REST, `https://api.tracker.yandex.net/v3/` (v2 устарела). Авторизация OAuth или IAM; заголовок `X-Org-ID` (Яндекс 360) или `X-Cloud-Org-ID` (Yandex Identity Hub). Пагинация `perPage`/`page` с заголовками `X-Total-Count`. Сущности: задачи, очереди, проекты, портфели, цели, доски, спринты, триггеры и автоматизация. https://yandex.ru/support/tracker/ru/about-api , https://yandex.ru/support/tracker/ru/common-format
- Популярность: SoDR перечисляет Yandex Tracker первым в категории «российские SaaS» (24,1 %).

#### Kaiten
- Цены за польз./мес при помесячной оплате: Free 0 ₽ (до 5 пользователей и 5 гостей, без отчётов); **Старт 250 ₽, Стандарт 580 ₽, Бизнес 780 ₽** (при оплате на 3 года 185, 430 и 580 ₽); «Корпорация» (250+ пользователей, on‑prem) - по запросу. https://kaiten.ru/tariffs
- Отчёты (со «Старт»): выполненные задачи, распределение карточек, история блокеров, Kanban‑ и Scrum‑отчёты, «аналитика потока». Отчёты по учёту времени - отдельный модуль. https://kaiten.ru/features/reports/
- **API на всех тарифах, включая Free.** `https://<domain>.kaiten.ru/api/v1` (есть `/api/latest`), Bearer‑токен. Сущности: карточки, история перемещений карточки (location history), итерации, time logs, доски, пространства, колонки, блокеры, внешние ссылки, аудит‑лог, вебхуки. **Лимит 50 запросов/с**, при превышении 429. Есть официальный MCP‑сервер. https://developers.kaiten.ru/

#### EvaTeam / EvaProject
- Облако, за польз./мес: Начальный 450 ₽, Базовый 690 ₽, Расширенный 1500 ₽ (минимум 5 пользователей); EvaDev 2590 ₽; Корпоративный - по запросу. On‑prem - по запросу. https://evateam.ru/evaproject/pricing/
- Импорт из Jira через Jira API.

#### YouTrack (JetBrains)
- Продажи в РФ прекращены (см. A4). В РФ используется как унаследованная on‑prem инсталляция (категория «зарубежный on‑prem», 35,1 %).

#### Redmine
- Open source. По SoDR входит в категорию «open source на своих серверах» (18,4 % вместе с Taiga и GitLab Issues).

#### Битрикс24
- Цены за всю компанию, не за пользователя: Free; Базовый 1990 ₽/мес (5 пользователей); Стандартный 5590 ₽ (50); Профессиональный 11 190 ₽ (100); Энтерпрайз от 27 190 ₽ (250). Есть Scrum. https://www.bitrix24.ru/prices/

#### Pyrus
- Free 0 ₽; Cloud 500 ₽/польз./мес (API и сводные отчёты); on‑prem 990 000 ₽/год за 100 пользователей. Для dev‑команд нетипичен, в SoDR не назван. https://pyrus.com/ru/pricing

#### Остальные
- Wildberries Tracker - внутренний инструмент RWB (CNews/Forbes, апрель 2026). Данных о внешних продажах нет. https://cnews.ru/link/b6253

### B3. Российские продукты инженерной аналитики / «метрики разработки»

Поиск по запросам «аналитика процессов разработки», «метрики команды разработки сервис», «DORA метрики российский», «инженерная аналитика платформа», «замена Jellyfish/LinearB» и «Engineering Intelligence».

| Продукт | Что делает | Интеграции | Цена | Статус |
|---|---|---|---|---|
| **PanDev Metrics** (pandev-metrics.com) | Аналитика работы инженеров: данные из IDE, терминала, браузера и AI‑агентов; DORA; стоимость задач (часы IDE, токены AI, ставки); аналитика по разработчику, команде и отделу; on‑prem трекер и база знаний | GitHub, GitLab, Jira; плагины JetBrains и VS Code; импорт из YouTrack, ClickUp, Notion. **Yandex Tracker и Bitbucket не упомянуты** | - | Казахстанский стартап (клиенты Halyk Finance, Damumed; раунд $400k при оценке $5M, 2025). Есть on‑prem (Docker, PostgreSQL, LDAP). https://pandev-metrics.com/ , https://pandev-metrics.com/docs/ , https://digitalbusiness.kz/2025-10-01/ma7-ventures-investirovali-400-tisyach-pri-otsenke-v-5-mln-v-startap-iz-kazahstana/amp/ |
| Лаборатория DX (dev-ex.ru) | **Не продукт**, а открытая база знаний по DevEx, DORA, SPACE и DX Core 4 (автор А. Синицын) | - | - | https://dev-ex.ru/ |
| Team Meter (кейс на Хабре) | Внутренний инструмент: витрины поверх Jira в DWH, визуализация в Metabase | Jira | - | Не коммерческий продукт. https://habr.com/ru/companies/tbank/articles/684608/comments |
| Внутренние решения (Ostrovok, Flowwow, Альфа‑Банк, Lamoda) | Статьи о самостоятельном внедрении DORA и cycle time | - | - | Подтверждают спрос и типовое решение «сделай сам». https://habr.com/ru/companies/ostrovok/articles/1024148/ , https://habr.com/ru/companies/flowwow/articles/1054042/ , https://habr.com/ru/companies/alfa/articles/1043524/ |
| Встроенные метрики трекеров (Kaiten, Yandex Tracker, EvaProject, SimpleOne) | Cycle time, velocity, flow и отчёты по задачам **без связи с кодом и CI** | - | см. B2 | - |

**Вывод:** российского коммерческого аналога Jellyfish, LinearB или Swarmia, который объединял бы Git, трекер и CI в ролевые дашборды с drilldown и умел бы работать с Yandex Tracker, Kaiten, GitFlic или SourceCraft, первичными источниками не найдено. Ближайшие конкуренты - PanDev Metrics (зарубежный, без Yandex Tracker), Apache DevLake (бесплатный, без российских коннекторов) и самописные решения на Grafana или Metabase.

---

## C. Рекомендованный приоритет коннекторов

Основания: SoDR 2025 (доли), статус API (раздел B), санкционные ограничения (A5).

| # | Коннектор | Почему | Тип API |
|---|---|---|---|
| 1 | **GitLab self‑managed (CE/EE) + GitLab CI** | 47,2 % CE и 18,6 % EE on‑prem; GitLab CI 64,5 %. Клиентам на CE недоступны DORA, Insights и MR analytics: ровно та дыра, которую закрывает craftPulse | REST/GraphQL v4, вебхуки. On‑prem установка, нужен доступ в периметр (агент или пул) |
| 2 | **Jira Server/Data Center** | Зарубежный on‑prem трекер - 35,1 %, доля растёт. Вендор ушёл, лицензии не продлеваются: значит, Atlassian Analytics и новые плагины клиенту недоступны | Jira REST v2 (Server/DC) |
| 3 | **Yandex Tracker** | Лидер российских SaaS‑трекеров (24,1 % категория, назван первым); Yandex Cloud - облако №1 (47,4 %); уже интегрирован с GitHub, GitLab и SourceCraft | REST v3, OAuth/IAM |
| 4 | **Jenkins** | CI №2 (34,7 %), часто вместе с GitLab или Bitbucket | Jenkins REST/JSON API |
| 5 | **GitHub (.com)** | 28 % респондентов, Actions 12 %. Аккаунты GitHub.com из РФ формально не запрещены, но GHES и Copilot в РФ не продаются | REST/GraphQL |
| 6 | **Bitbucket Server/DC** | 13,3 % on‑prem; та же ситуация, что с Jira (вендор Atlassian ушёл) | Bitbucket Server REST |
| 7 | **Kaiten** | Заметный российский SaaS‑трекер; API на всех тарифах, лимит 50 rps, есть история перемещений карточек для cycle time | REST v1 |
| 8 | **SourceCraft** | Платформа Яндекса с агрессивным AI‑позиционированием и связкой с Tracker и Yandex Cloud. API 0.0.1 нестабилен, CI‑эндпоинты частично withdrawn. Сейчас имеет смысл как дифференциатор и партнёрство, а не по объёму | REST 0.0.1 + вебхуки |
| 9 | **GitVerse** | Растущая платформа Сбера: on‑prem с 2026, полный GitHub‑подобный API (PR, CI runs и jobs) | REST (api.gitverse.ru) |
| 10 | **GitFlic** | 2,9 % и падает, но важна для госсектора и «Астры»; есть полноценный API MR и CI | REST (api.gitflic.ru) |
| 11 | Внешние анализаторы: SonarQube и AppSec‑сканеры (SARIF), YouTrack, EvaTeam, Redmine | По запросам пилотных клиентов | - |

Практические выводы:
- **Минимальный продукт для РФ: GitLab (CE/EE + CI) + Jira DC + Yandex Tracker + Jenkins.** Этот набор покрывает большинство стеков из SoDR.
- Обязательна **on‑prem или агентная поставка**: 61 % компаний держат код на своих инсталляциях (SoDR 2024), и доля on‑prem GitLab растёт.
- DORA и cycle time придётся **вычислять самим** из сырых событий (deployments, MR, pipelines, переходы статусов). Встроенные метрики есть либо только в дорогих тарифах (GitLab Ultimate), либо только по задачам (Tracker, Kaiten).
- Для российских платформ нужен **универсальный ingest через вебхуки или CI‑шаг**: API SourceCraft и GitVerse молодые, а CI‑эндпоинты могут меняться.

---

## Список источников (дата обращения 2026‑10‑14)

- State of DevOps Russia 2025 (PDF): https://express42.com/wp-content/themes/express42/assets/docs/StateOfDevOpsRussia2025.pdf ; 2024: https://express42.com/wp-content/themes/express42/assets/docs/StateOfDevOpsRussia2024.pdf ; https://express42.com/state-of-devops/
- Yandex B2B Tech и ИТМО, исследование devtools 2025: https://yandex.cloud/ru/blog/devtools-research-2025 (GitHub и GitLab используют 75 %, локальные платформы - 17 %)
- GitLab: https://about.gitlab.com/pricing/ ; https://about.gitlab.com/blog/gitlab-actions-to-date-regarding-russian-invasion-of-ukraine ; https://handbook.gitlab.com/handbook/legal/subscription-agreement/ ; docs (см. A1)
- GitHub: https://github.com/pricing ; https://docs.github.com/en/site-policy/other-site-policies/github-and-trade-controls ; https://docs.github.com/en/copilot/concepts/copilot-metrics
- Atlassian: https://www.atlassian.com/blog/announcements/atlassian-stands-with-ukraine ; https://www.atlassian.com/software/compass/pricing ; https://www.atlassian.com/platform/analytics ; https://community.developer.atlassian.com/t/from-data-center-to-cloud-the-next-chapter-for-marketplace-apps/95173
- JetBrains: https://blog.jetbrains.com/blog/2022/03/11/jetbrains-statement-on-ukraine/ ; https://blog.jetbrains.com/blog/2022/12/06/update-on-jetbrains-statement-on-ukraine/
- OFAC: https://ofac.treasury.gov/faqs/1187 ; https://ofac.treasury.gov/faqs/1184
- Grafana: https://grafana.com/grafana/plugins/grafana-gitlab-datasource/ ; https://grafana.com/grafana/plugins/grafana-jira-datasource/ ; https://community.grafana.com/t/cant-download-grafana-9-0-2-from-russia/68391
- Apache DevLake: https://devlake.apache.org/ ; https://devlake.apache.org/docs/Overview/SupportedDataSources
- SourceCraft: https://sourcecraft.dev/ ; https://sourcecraft.dev/portal/docs/ru/sourcecraft/pricing ; https://sourcecraft.dev/portal/docs/en/api-ref/ ; https://sourcecraft.dev/portal/docs/en/sourcecraft/release-notes.md
- GitVerse: https://gitverse.ru/ ; https://gitverse.ru/home/pricing/ ; https://gitverse.ru/docs/developers/public-api ; ComNews 18.05.2026 (ссылка выше)
- GitFlic: https://gitflic.ru/ ; https://gitflic.ru/price ; https://docs.gitflic.ru/ ; https://docs.gitflic.ru/api/cicd/ ; https://docs.gitflic.ru/project/stat/
- Yandex Cloud Managed GitLab: https://yandex.cloud/ru/docs/managed-gitlab/
- Yandex Tracker: https://360.yandex.ru/business/tracker/ ; https://yandex.ru/support/tracker/ru/pricing ; https://yandex.ru/support/tracker/ru/about-api ; https://yandex.ru/support/tracker/ru/common-format ; https://yandex.ru/support/tracker/ru/user/widgets ; https://yandex.ru/support/tracker/ru/user/add-repository
- Kaiten: https://kaiten.ru/tariffs ; https://kaiten.ru/features/reports/ ; https://developers.kaiten.ru/
- EvaTeam: https://evateam.ru/ ; https://evateam.ru/evaproject/pricing/
- Битрикс24: https://www.bitrix24.ru/prices/ ; Pyrus: https://pyrus.com/ru/pricing
- PanDev Metrics: https://pandev-metrics.com/ ; https://pandev-metrics.com/docs/
- Softmart GitLab: https://gitlab.softmart.ru/
