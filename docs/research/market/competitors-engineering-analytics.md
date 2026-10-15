# Конкуренты craftPulse: engineering intelligence, developer productivity, порталы

Дата обращения ко всем источникам: **2026-10-14**. Использованы только первичные источники: официальные сайты, документация, страницы цен, условия использования, официальные пресс-релизы и репозитории. Вторичные источники (блоги конкурентов, агрегаторы) помечены отдельно.

---

## 0. Общий контекст: доступность для российских компаний

Это применимо ко всем вендорам из США и ЕС, ниже на него даются ссылки.

- **США (OFAC).** Определение по EO 14071 от 12.06.2024 действует с 12.09.2024. Оно запрещает американским лицам оказывать лицам в России «IT consultancy and design services», а также IT support и cloud-based services для enterprise management software и design and manufacturing software. Источник: OFAC FAQ 1184, https://ofac.treasury.gov/faqs/1184. Относятся ли EI-платформы к «enterprise management software», официально не разъяснено. Риск для покупателя из РФ есть.
- **ЕС.** Ст. 5n(2b) Регламента 833/2014 (12-й пакет, 18.12.2023) запрещает поставку «software for the management of enterprises» юрлицам, учреждённым в России. FAQ Еврокомиссии: https://finance.ec.europa.eu/system/files/2024-02/faqs-sanctions-russia-software_en%2C.pdf. Для Swarmia (Финляндия) это прямое ограничение, если продукт подпадает под Annex XXXIX (BI/управленческое ПО).
- **Практика.. Оплата идёт картой или переводом в USD/EUR. Пример: LinearB принимает wire, ACH, карту или AWS Marketplace (https://linearb.io/pricing).

---

## 1. DX (getdx.com), теперь часть Atlassian

**Что это.** Платформа «engineering intelligence»: SDLC-аналитика, фреймворк DX Core 4, опросы DevEx (DXI), измерение эффекта ИИ, каталог и скоркарты. Покупатели: VP Eng/CTO, платформенные команды, DevEx-лиды. Источник: https://getdx.com/.
**Сделка.** Atlassian объявил о покупке DX примерно за $1 млрд (18.09.2025) и закрыл её 10.11.2025: https://www.businesswire.com/news/home/20251110683591/en/Atlassian-Completes-Acquisition-of-DX-Advancing-Engineering-Intelligence-for-Enterprises. Условия на https://getdx.com/terms/ теперь от имени Atlassian.

1. **Источники данных** (https://docs.getdx.com/connectors/):
   - Git: GitHub (+Copilot, Security), GitLab (+Pipelines, Security Dashboard), Bitbucket Cloud и Data Center, Azure DevOps, Gerrit.
   - Трекеры: Jira Cloud и Data Center, Linear, Asana, ClickUp, Shortcut, Azure DevOps.
   - CI/CD: GitHub Actions/Deployments, GitLab Pipelines, Bitbucket Pipelines, CircleCI, Buildkite, Bitrise, Harness, Vercel.
   - Качество и безопасность: **SonarQube Server, SonarCloud, Snyk, Checkmarx, Wiz, CodeRabbit**.
   - Инциденты и observability: PagerDuty, Opsgenie, Datadog, Sentry и другие.
   - Свои данные: Data Cloud API, custom tables (https://docs.getdx.com/connectors/).
   - Self-hosted GitLab подключается через свой API URL или self-hosted extractor (https://docs.getdx.com/connectors/gitlab/).
   - YouTrack, Redmine, Yandex Tracker, GitFlic, GitVerse, SourceCraft: нет в списке интеграций.
2. **Роли и иерархия.** Есть Organization dashboard для руководителей и Group dashboard для EM и тимлидов; иерархии команд загружаются из CSV (https://docs.getdx.com/dashboard/overview/, https://docs.getdx.com/team-hierarchies/). Есть DX Core 4, отчёты по DORA, Allocation/CapEx (Jira, Linear, ADO) и опросы DevEx: в документации перечислены отчёты «DORA metrics», «Jira allocation», «Jira CapEx» (https://docs.getdx.com/connectors/).
3. **Объяснимость.** Group dashboard содержит вкладки Pull requests и Issues со списками PR и задач команды (https://docs.getdx.com/dashboard/overview/). Скоркарты пишутся на SQL, есть отчёт с drilldown по отдельной скоркарте (https://docs.getdx.com/scorecards/overview/).
4. **Доставка в рабочий процесс.** Есть Slack-приложение и приложение для MS Teams (пункты меню в docs: https://docs.getdx.com/connectors/).
5. **Развёртывание.** Multi-tenant SaaS, DX Dedicated (single-tenant), DX Managed (BYOC в AWS или GCP клиента) и self-hosted extractor данных (https://docs.getdx.com/deployment-options/).
6. **Цена.** Публичных цен нет, только «Get a demo» (https://getdx.com/pricing/). По запросу.
7. **Россия.** Atlassian официально свернул бизнес в России и Беларуси с 31.10.2022 и не продлевает лицензии клиентам из РФ (https://www.atlassian.com/blog/announcements/atlassian-stands-with-ukraine). В условиях есть запрет на экспорт в эмбарго-юрисдикции и лицам из санкционных списков (https://getdx.com/terms/). **Покупка из РФ фактически невозможна.**
8. **Слабые места для позиционирования craftPulse.**
   - Нет российских Git-хостингов и трекеров (см. п. 1).
   - Ориентация на enterprise, цены не публикуются.
   - Dashboard и system-based Core 4 доступны только клиентам Data Cloud с включённым DX AI (https://docs.getdx.com/dashboard/overview/).
   - Скоркарты только для клиентов Fabric (https://docs.getdx.com/scorecards/overview/).

## 2. Jellyfish

**Что это.** Платформа управления инженерией: AI Impact, Operational Effectiveness, Business Alignment (аллокация инвестиций), DevEx-опросы, DevFinOps (капитализация R&D). Покупатели: руководители инженерии, финансы, продакт-лидеры (https://jellyfish.co/pricing/).

1. **Источники.**
   - Git: GitHub, GitLab (SaaS и self-managed, https://jellyfish.co/integration/gitlab/), Bitbucket (https://jellyfish.co/integration/bitbucket/), Azure DevOps (https://jellyfish.co/integrations/).
   - Трекеры: Jira Cloud и Data Center, напрямую или через Jellyfish Agent (https://jellyfish.co/integration/jira/); Linear и Azure Boards (https://jellyfish.co/integrations/).
   - CI: Jenkins (https://jellyfish.co/integration/jenkins/), CircleCI (https://jellyfish.co/integration/circleci/).
   - Качество и безопасность: **SonarQube** (баги, покрытие, quality gates, https://jellyfish.co/integration/sonarqube/) и **Snyk** (https://jellyfish.co/integration/snyk/).
   - YouTrack: нет в списке интеграций.
2. **Роли.** Решения отдельно для Executives, Managers, Platform Eng, Finance, Developers. Есть DORA/SPACE, аллокация инвестиций, DevEx-опросы, бенчмарки (https://jellyfish.co/pricing/).
3. **Объяснимость.** Life Cycle Explorer: «Drill into the individual issues and outliers» (https://jellyfish.co/platform/life-cycle-explorer/).
4. **Доставка.** Slack используется для DM с опросами, напоминаний и алертов (https://jellyfish.co/integration/slack/).
5. **Развёртывание. Для on-prem Jira предусмотрен Jellyfish Agent (https://jellyfish.co/integration/jira/).
6. **Цена.** «Based on number of seats and specific modules selected», по запросу (https://jellyfish.co/pricing/).
7. **Россия.** Отдельной позиции по России нет. Условия: https://jellyfish.co/terms-of-service/. На практике действует контекст OFAC (раздел 0), оплата только в USD.
8. **Слабые места.** Дорого и непрозрачно (модули плюс места). Нет РФ-источников..

## 3. LinearB

**Что это.** Платформа developer productivity и workflow automation: метрики, DORA, AI code review, автоматизации PR через gitStream. Покупатели: VP Eng и EM (https://linearb.io/platform). Назван Leader в Gartner MQ for Developer Productivity Insight Platforms 2026 (по заявлению вендора: https://linearb.io/dora-metrics).

1. **Источники** (https://linearb.io/pricing, https://linearb.io/integrations):
   - Git: GitHub (cloud и on-prem), GitLab (cloud и on-prem), Bitbucket Cloud, Azure DevOps Cloud. На тарифе Essentials только GitHub Cloud.
   - Трекеры: Jira (cloud и on-prem), Azure Boards.
   - CI: CircleCI, Jenkins.
   - Качество: **SonarCloud**, Jit.
   - Есть API (ссылка «API Docs»).
   - YouTrack, Linear: нет в списке интеграций.
2. **Роли.** Есть DORA с бенчмарками (https://linearb.io/dora-metrics), Resource Allocation, Cost Capitalization, опросы DSAT и Dev Team Management (https://linearb.io/platform).
3. **Объяснимость.** «Drill into leading indicators and the individual pull requests that are slowing your team down» (https://linearb.io/dora-metrics).
4. **Доставка.** **Сильная сторона.** gitStream задаёт YAML-автоматизации в репозитории: метки, назначение ревьюеров, auto-merge, автоматические change requests, AI-описание PR и AI-ревью. Работает на GitHub, GitHub Server, GitLab, Bitbucket Cloud (https://docs.gitstream.cm/). Slack и Teams только на Enterprise (https://linearb.io/pricing).
5. **Развёртывание.** SaaS; на Enterprise есть on-prem agents, которые подключают локальные системы к облаку LinearB (https://linearb.io/pricing).
6. **Цена.** Essentials **$29**, Enterprise **$59** за контрибьютора в месяц, оплата за год, минимум 50 и 100 пользователей. Бесплатного тарифа нет, есть trial на 45 дней (https://linearb.io/pricing). Получается минимум около **$17 400/год**.
7. **Россия.** Отдельной позиции по России нет. Условия (https://linearb.io/terms) упоминаний России не содержат. Оплата в USD (wire, ACH, карта, AWS Marketplace).
8. **Слабые места.** Порог входа 50 разработчиков и выше: SMB из 10–49 человек отсекается. Нет РФ-хостингов и трекеров. Импорт анализаторов ограничен SonarCloud и Jit.

## 4. Swarmia (Финляндия)

**Что это.** Платформа engineering effectiveness: метрики, DORA, investment balance, опросы, working agreements, AI-адопция. Покупатели: EM и руководители инженерии (https://www.swarmia.com/product/).

1. **Источники** (https://www.swarmia.com/integrations/):
   - Git: GitHub (Cloud, Enterprise Cloud, Enterprise Server), GitLab (Cloud, Server).
   - Трекеры: Jira Cloud, Jira Server и Data Center, Linear, Azure Boards (beta), Shortcut (beta), GitHub Issues (beta).
   - CI: GitHub Actions и **Deployments API**, через который можно передавать деплои из любого CI.
   - Свои данные: Data cloud, Team API.
   - Bitbucket и Azure Repos в списке нет. Анализаторов кода (Sonar и т. п.) в списке нет.
2. **Роли.** Есть организационные метрики и drill-down представления команд (https://help.swarmia.com/use-cases/improve-pull-request-flow), DORA, investment balance, опросы с тепловой картой по командам (https://www.swarmia.com/product/).
3. **Объяснимость.** Клик по PR в любом месте приложения показывает разбивку cycle time (https://www.swarmia.com/changelog/2022-07-14-cycle-time-breakdown-for-prs). Клик по задаче показывает связанные PR (https://www.swarmia.com/changelog/2024-05-08-issue-activity-details). CI visibility раскрывается по цепочке репозиторий → workflow → job → run (https://help.swarmia.com/get-visibility-into-your-ci-pipeline).
4. **Доставка.** Slack- и Teams-уведомления: ревью, упавшие CI-сборки с логами прямо в Slack, ежедневные дайджесты (https://help.swarmia.com/personal-notifications, https://help.swarmia.com/notifications/team-notifications).
5. **Развёртывание.** SaaS на Google Cloud, данные во Франкфурте (https://www.swarmia.com/security/). On-prem есть только для интеграций (Enterprise, Proxy agent), не для самой платформы (https://www.swarmia.com/pricing/).
6. **Цена** (https://www.swarmia.com/pricing/):
   - **Free** до 9 разработчиков.
   - **Standard $45**, **Enterprise $55** за разработчика в месяц (при оплате за год).
   - Модули: AI $5, Surveys $9, Capitalization $18, Productivity $23.
7. **Россия.** Отдельной позиции по России нет. Право Финляндии (https://www.swarmia.com/terms/). Компания из ЕС, поэтому к ней применим запрет ЕС на поставку управленческого ПО юрлицам РФ (раздел 0).
8. **Слабые места.** Нет Bitbucket и Azure Repos. Нет импорта Sonar и других анализаторов. Нет РФ-трекеров. SaaS только в ЕС.

## 5. Hatica

**Что это.** Engineering analytics: DORA, cycle time, maker time, асинхронные стендапы, investment allocation (https://www.hatica.io/).

1. **Источники** (https://www.hatica.io/connectors/):
   - Git: GitHub, GitLab, Bitbucket, Azure DevOps.
   - Трекеры: Jira, Asana, ClickUp, Trello, Clubhouse, Aha!, ProductBoard, ProductPlan, Linear, Azure Boards.
   - CI: Jenkins, CircleCI, Phabricator, Harness.
   - Инциденты: OpsGenie, PagerDuty, VictorOps.
   - Анализаторы кода: нет в списке интеграций.
2. **Роли.** Есть представления Executives (org-wide), Managers (team) и Engineers (https://www.hatica.io/). На тарифе Business: Org-level metrics, Executive views, SPACE, Investments, Cost capitalisation (https://www.hatica.io/pricing/). Опросы DevEx входят в Free.
3. **Объяснимость.** «Aggregated work explorer» и метрики PR по стадиям (https://www.hatica.io/).
4. **Доставка.** Стендапы через Slack и email (https://www.hatica.io/).
5. **Развёртывание.** На главной: «Low maintenance on-prem hosting available». Enterprise: «Self-hosted/Private cloud» (https://www.hatica.io/, https://www.hatica.io/pricing/).
6. **Цена.** **Free** с историей за 1 месяц. **Pro $19**, **Business $29** за участника в месяц. Enterprise по запросу (https://www.hatica.io/pricing/).
7. **Россия.** Отдельной позиции по России нет. Право Делавэра (https://www.hatica.io/legal/terms/).
8. **Слабые места.** Последние публикации в блоге датированы январём 2025 года (https://www.hatica.io/blog/), активность продукта под вопросом. Нет анализаторов кода и РФ-источников.

## 6. Waydev

**Что это.** SEI-платформа: DORA, PR insights, resource allocation, cost capitalization, AI-адопция и AI-агенты. Покупатели: CTO и VP Eng (https://waydev.co/integrations/).

1. **Источники.**
   - Git: GitHub, GitLab, Bitbucket, Azure DevOps, Gerrit.
   - Трекеры: Jira, GitHub Issues, ClickUp, Azure Boards (https://waydev.co/pricing/); Linear указан на https://waydev.co/enterprise/.
   - CI/CD: «CI/CD tools» без перечня (https://waydev.co/pricing/).
   - Анализаторы кода: нет в списке интеграций.
   - Свои данные: API на тарифе Premium (https://waydev.co/pricing/), Git-hooks и AI-телеметрия (https://docs.waydev.co/docs/start-guide).
2. **Роли.** Есть представления для Executives, EM, Contributors, а также Team Insights, бенчмарки, DORA, SPACE, DX (https://waydev.co/integrations/). Индивидуальные представления на enterprise по умолчанию выключены (https://waydev.co/enterprise/).
3. **Объяснимость.** Вендор заявляет «Traceable: Every figure → query → source system» (https://waydev.co/enterprise/). По смыслу это ближе всего к объяснимому скору craftPulse.
4. **Доставка.** Slack Bot и MS Teams (https://docs.waydev.co/docs/start-guide), Signals (AI-алерты).
5. **Развёртывание.** Waydev Cloud или **Self-hosted** («AWS, Azure or GCP… your VPC») (https://waydev.co/enterprise/). On-prem только на Enterprise (https://waydev.co/pricing/).
6. **Цена.** «Ask for quote». В калькуляторе отображается «$899, billed annually» для Pro без пояснения единицы (https://waydev.co/pricing/). Бесплатного тарифа нет, есть бесплатный PoC и trial.
7. **Россия.** Отдельной позиции по России нет. Право Делавэра (https://waydev.co/terms-of-service/).
8. **Слабые места.** Цены непрозрачны. Нет анализаторов кода и РФ-источников.

## 7. GitClear

**Что это.** Аналитика кода и PR на основе метрики Diff Delta; исследования качества кода, написанного с ИИ. Покупатели: EM и CTO (https://www.gitclear.com/pricing).

1. **Источники.** Git: GitHub, GitLab, Bitbucket, Azure DevOps. AI-инструменты: Claude Code, Copilot, Cursor и другие. Трекер: Jira (https://www.gitclear.com/pricing).
2. **Роли.** Есть Create Teams, отчёты по разработчикам (Developer Highlights, Performance Review Reports), DORA (lead time, MTTR), опросы (https://www.gitclear.com/pricing).
3. **Объяснимость.** Commit Activity Browser, Per-line history, Directory Browser (https://www.gitclear.com/pricing). Детализация до коммита и строки сильная.
4. **Доставка.** Chrome-расширение для GitHub PR и Slack-алерты по целям, судя по заголовкам статей справки: https://www.gitclear.com/help/github_extension_reduce_pull_request_time, https://www.gitclear.com/help/slack_notifications_for_goals_and_alerts. Содержимое статей подгружается через JS и не проверено.
5. **Развёртывание.** Enterprise описан как «An on-prem option» (https://www.gitclear.com/pricing). Есть статья об установке on-prem, но её текст не загрузился (https://www.gitclear.com/help/on_prem_enterprise_gitclear).
6. **Цена.** Starter бесплатно (3 репозитория). Pro **$14.95** при оплате за год / $29 помесячно. Elite **$24.95** / $39. Enterprise (on-prem) **$34.95** / $49. Всё за контрибьютора в месяц (https://www.gitclear.com/pricing).
7. **Россия.** Отдельной позиции по России нет. Текст ToS не загрузился (https://www.gitclear.com/help/general/terms_of_service).
8. **Слабые места.** Фокус на коде, а не на трекерах и CI. Нет анализаторов. Нет РФ-источников. Индивидуальные метрики разработчиков могут вызывать сопротивление команд (это вывод из набора функций, а не из отзывов).

## 8. Apache DevLake (open source)

**Что это.** Open-source платформа данных для DevOps: сбор данных, единая доменная модель, дашборды Grafana, DORA. В 2025 году стала Top-Level Project ASF (https://news.apache.org/?p=7118). Лицензия Apache-2.0, репозиторий https://github.com/apache/devlake.

1. **Источники** (https://devlake.apache.org/docs/Overview/SupportedDataSources):
   - Git: GitHub, GitLab (Cloud и CE 11+), Bitbucket (beta), Azure DevOps (beta), Gitee.
   - Трекеры: Jira (Cloud и Server/DC), TAPD, Teambition, Zentao.
   - CI: Jenkins, GitHub Actions, GitLab CI, CircleCI, Bamboo.
   - Анализаторы: **SonarQube** 8.x–9.x.
   - Инциденты: PagerDuty, Opsgenie, incident.io.
   - В коде также есть плагины linear, youtrack, testmo, argocd, taiga, bitbucket_server, trello (https://github.com/apache/devlake/tree/main/backend/plugins). **Плагин YouTrack влит 2026-10-04** (коммит «feat(youtrack): add YouTrack plugin (#9169)»), в релизах пока не вышел: последний релиз v1.0.3-beta18 от 2026-09-27 (https://github.com/apache/devlake/releases).
   - Свои данные: Incoming Webhook для деплоев и инцидентов из любого CI (https://devlake.apache.org/docs/Plugins/webhook). CSV-импорт, **включая qa_test_cases и qa_test_case_executions** (https://devlake.apache.org/docs/Plugins/customize).
2. **Роли.** Проекты, команды, дашборды DORA в Grafana (https://devlake.apache.org/docs/Overview/SupportedDataSources).
3. **Объяснимость.** Сырые и доменные таблицы доступны через SQL и Grafana (доменная модель там же).
4. **Доставка.** Плагин slack есть в коде (https://github.com/apache/devlake/tree/main/backend/plugins).
5. **Развёртывание.** Только self-hosted (Docker или Helm), см. документацию https://devlake.apache.org/.
6. **Цена.** Бесплатно (Apache-2.0).
7. **Россия.** Ограничений в лицензии нет; скачивание с GitHub доступно.
8. **Слабые места.** Это конструктор, а не продукт: нужен свой DevOps для эксплуатации, UI собран на Grafana, нет ролевых представлений и объяснимого скора. **Для craftPulse это главный «бесплатный» конкурент и одновременно возможная база.**

## 9. Cortex

**Что это.** Internal Developer Portal с каталогом, скоркартами, workflows и модулем Eng Intelligence. Покупатели: платформенные команды, SRE, руководители инженерии (https://www.cortex.io/pricing).

1. **Источники** (https://docs.cortex.io/llms.txt, раздел integrations):
   - Git: GitHub, GitLab, Bitbucket, Azure DevOps.
   - Трекеры: Jira, ClickUp, Azure DevOps.
   - CI/CD: ArgoCD, Buildkite, CircleCI, Jenkins, Harness.
   - Качество и безопасность: **SonarQube, Codecov, Snyk, Checkmarx, Semgrep, Veracode, Mend, Wiz, Apiiro**.
   - Свои данные: Custom webhook, API, CQL (https://www.cortex.io/pricing, FAQ).
   - Внутренние системы подключаются через Axon Relay.
2. **Роли.** Eng Intelligence включает дашборды DORA и Velocity и Data Explorer с иерархией «drill down into nested groups» (Bird's Eye) (https://docs.cortex.io/improve/eng-intelligence/data-explorer.md). Скоркарты строятся по уровням или по баллам.
3. **Объяснимость.** «Drill into individual data points» (https://docs.cortex.io/improve/eng-intelligence/data-explorer.md). Есть гайд «Surface code coverage metrics from SonarQube in failed Scorecard rules» (https://docs.cortex.io/llms.txt).
4. **Доставка.** Workflows умеют «Create a pull request comment». GitHub App комментирует PR при невалидном дескрипторе. Slack-уведомления по скоркартам и initiatives. Источник: https://docs.cortex.io/llms-full.txt.
5. **Развёртывание.** Данные противоречат друг другу. В FAQ на странице цен сказано «Cortex is cloud-hosted SaaS, running on GCP» (https://www.cortex.io/pricing). В документации описан **Self-managed Cortex** с установкой Helm-чартом в Kubernetes клиента (https://docs.cortex.io/self-managed.md).
6. **Цена.** «Per-seat model… Pricing is customized», по запросу (https://www.cortex.io/pricing).
7. **Россия.** Отдельной позиции по России нет. Право Калифорнии (https://www.cortex.io/legal/terms-of-service).
8. **Слабые места.** Порог внедрения высокий: нужно наполнить каталог и поручить его кому-то сопровождать (FAQ на https://www.cortex.io/pricing). Продукт про сервисы и стандарты, а не про задачи и MR. Нет РФ-источников.

## 10. Port (port.io, ранее getport.io)

**Что это.** Internal developer portal / «Agentic engineering platform»: Context Lake, каталог, self-service actions, скоркарты, workflows, engineering intelligence (https://www.port.io/pricing).

1. **Источники** (https://docs.port.io/llms.txt):
   - Git: GitHub (Ocean), GitLab (v2), Bitbucket Cloud, **Bitbucket (self-hosted)**, Azure DevOps.
   - Трекеры: Jira, **Jira Server (self-hosted)**, Linear.
   - CI/CD: Jenkins, Octopus, Argo CD, AWS CodePipeline.
   - Качество и безопасность: **SonarQube, Snyk, Checkmarx One, Mend, Wiz, Aikido, ArmorCode**; Codecov через webhook.
   - Свои данные: webhook, API, custom Ocean-интеграции, в том числе self-hosted.
   - YouTrack, Redmine: нет в списке интеграций.
2. **Роли.** Есть «Visualize organization hierarchy» (группы → команды → сервисы), дашборды в Interface builder, скоркарты, в том числе для DORA (https://docs.port.io/llms.txt). Метрики и опросы упомянуты на https://www.port.io/solutions/engineering-intelligence.
3. **Объяснимость.** Скоркарты по сущностям каталога. Метрики считаются по сущностям (агрегаты).
4. **Доставка.** Workflows и automations: Slack и Teams, управление PR, гайд с Claude-агентом, который комментирует и назначает ревьюеров в PR (https://docs.port.io/llms.txt). Self-service actions запускают GitHub Actions, GitLab pipelines и Jenkins.
5. **Развёртывание.** SaaS с регионами EU и US. Интеграции (Ocean) можно хостить у себя (https://docs.port.io/llms.txt).
6. **Цена** (https://www.port.io/pricing):
   - **Free** до 15 мест.
   - **Basic от $30** за место в месяц (пакет на 50 мест).
   - **Standard от $40** за место в месяц (до 200 мест).
   - Enterprise по запросу.
7. **Россия.** MSA п. 13: запрет использования субъектами «comprehensive sanctions», лицами из списков OFAC, EU и UK; Port вправе приостановить доступ (https://www.port.io/legal/master-services-agreement). Россия не находится под полным эмбарго США, но см. раздел 0. Право Израиля (https://www.port.io/legal/terms-of-service).
8. **Слабые места.** Это конструктор портала: метрики приходится собирать самому через blueprints. Нет РФ-источников.

## 11. Дополнительные игроки: статус на 2026-10-14

| Продукт | Статус | Источник |
|---|---|---|
| **Pluralsight Flow** (бывш. GitPrime) | **Куплен Appfire 05.02.2025**, теперь «Flow by Appfire», вход через appfireflow.com с 23.07.2025. Сторонние блоги (Larridin, Waydev) пишут о закрытии в 2027 году, официального подтверждения нет | https://appfire.com/resources/blog/appfire-acquires-sei-platform-flow ; https://appfire.atlassian.net/wiki/spaces/FD/pages/2208239236/Flow+transition+to+Appfire+API+and+callback+URI+updates ; (вторичный) https://larridin.com/blog/why-appfire-flow-shut-down |
| **Code Climate Velocity** | Бесплатный «Velocity for Teams» закрыт 18.03.2024. Сейчас главная codeclimate.com продаёт другой продукт: контекст об инженерной организации для AI-ассистентов (MCP, «Request early access»). Velocity как продукт на сайте не представлен | https://docs.velocity.codeclimate.com/en/articles/8719212-ending-support-for-velocity-for-teams ; https://codeclimate.com/ |
| **Sleuth** | Сменил фокус: главная продвигает «Sleuth Skills» (управление agent skills, MCP), DORA Metrics остаётся пунктом меню | https://www.sleuth.io/ |
| **Haystack** | Сменил позиционирование на «Delivery Ops»: доски, риски и статусы в Slack, интеграция Jira + Git | https://www.usehaystack.io/ |

---

## 12. Сводная матрица

Обозначения: ✅ есть (подтверждено), ◐ частично или с оговорками, ❌ нет в официальном списке, «-» нет данных. Источник каждой ячейки указан в разделе продукта.

| Продукт | Git-хостинги | Трекеры | Импорт CI | Импорт анализаторов | Иерархия ролей | Drilldown до PR/задачи | Доставка в CI/MR | On-prem | Цена | РФ |
|---|---|---|---|---|---|---|---|---|---|---|
| DX (Atlassian) | GH, GL (+self-hosted), BB Cloud/DC, ADO, Gerrit | Jira Cloud/DC, Linear, Asana, ClickUp, Shortcut, ADO | ✅ GHA, GL CI, BB Pipelines, CircleCI, Buildkite и др. | ✅ SonarQube/Cloud, Snyk, Checkmarx, Wiz | ✅ Org → Group → IC | ◐ списки PR и задач команды | ◐ Slack/Teams; PR-комментарии ? | ◐ Dedicated / BYOC (AWS, GCP) | по запросу | ❌ Atlassian ушёл из РФ 31.10.2022 |
| Jellyfish | GH, GL (+self-managed), BB, ADO | Jira Cloud/DC, Linear, Azure Boards | ✅ Jenkins, CircleCI | ✅ SonarQube, Snyk | ✅ exec / manager / finance | ✅ Life Cycle Explorer | ◐ Slack (опросы, алерты) | ◐ только агент для on-prem источников | по запросу (места + модули) | ? офиц. заявления нет; USD |
| LinearB | GH, GL (cloud/on-prem), BB Cloud, ADO | Jira (cloud/on-prem), Azure Boards | ✅ CircleCI, Jenkins | ◐ SonarCloud, Jit | ✅ команды, DORA, аллокация | ✅ до отдельных PR | ✅ gitStream (метки, ревьюеры, AI-ревью в PR) | ◐ on-prem agents | $29 / $59 за контрибьютора в мес., мин. 50 / 100 |  USD |
| Swarmia | GH (+GHES), GL (+Server) | Jira (+DC), Linear, Azure Boards, Shortcut, GH Issues | ◐ GHA + Deployments API | ❌ | ✅ org → team | ✅ PR, задача → PR, CI-run | ◐ Slack/Teams (в т. ч. упавший CI) | ❌ SaaS (Франкфурт) | Free ≤9; $45 / $55 за разработчика в мес. |  ЕС-вендор (санкции ЕС) |
| Hatica | GH, GL, BB, ADO | Jira, Linear, Asana, ClickUp, Trello и др. | ✅ Jenkins, CircleCI, Harness | - | ✅ exec / manager / engineer | - | ◐ Slack-стендапы | ✅ self-hosted (Enterprise) | Free; $19 / $29 за участника в мес. | - |
| Waydev | GH, GL, BB, ADO, Gerrit | Jira, GH Issues, ClickUp, Azure Boards, Linear | ◐ «CI/CD tools» | - | ✅ exec / EM / contributor | ✅ «figure → query → source» | ◐ Slack/Teams | ✅ self-hosted / VPC | по запросу | - |
| GitClear | GH, GL, BB, ADO | Jira | - | - | ◐ команды и разработчики | ✅ коммит / строка | ◐ GH-расширение, Slack | ✅ Enterprise on-prem | Free; $14.95–$34.95 за контрибьютора в мес. | - |
| Apache DevLake | GH, GL (CE), BB (+Server), ADO, Gitee | Jira (+DC), Linear, YouTrack (main, 10.2026), TAPD, Zentao, Trello | ✅ Jenkins, GHA, GL CI, CircleCI, Bamboo, webhook | ✅ SonarQube; CSV тест-кейсов | ◐ проекты / команды | ◐ SQL / Grafana | - | ✅ только self-hosted | бесплатно (Apache-2.0) | ✅ OSS без ограничений |
| Cortex | GH, GL, BB, ADO | Jira, ClickUp, ADO | ✅ Jenkins, CircleCI, Buildkite, ArgoCD, Harness | ✅ SonarQube, Codecov, Snyk, Semgrep, Veracode и др. | ✅ вложенные группы (Bird's Eye) | ✅ до data points | ✅ PR-комментарий из Workflows, Slack | ◐ противоречие: SaaS (FAQ) vs Self-managed Helm (docs) | по запросу (за место) | - |
| Port | GH, GL, BB Cloud/self-hosted, ADO | Jira (+Server), Linear | ✅ Jenkins, Octopus, ArgoCD | ✅ SonarQube, Snyk, Checkmarx, Mend, Wiz | ✅ группы → команды → сервисы | - | ✅ workflows (PR, Slack/Teams) | ◐ SaaS; интеграции self-hosted | Free ≤15; от $30 / $40 за место в мес. | ◐ MSA: санкционная оговорка |

**Российские хостинги и трекеры** (SourceCraft, GitFlic, GitVerse, Redmine, Yandex Tracker) не найдены ни у одного продукта. Исключение: YouTrack в main-ветке DevLake (без релиза).

---

## 13. Выводы для позиционирования craftPulse

Что craftPulse может заявлять обоснованно:
1. **Российские источники.** Ни один из 10 продуктов не поддерживает SourceCraft, GitFlic, GitVerse, Yandex Tracker и Redmine. YouTrack есть только в DevLake, и то в неотрелизной ветке.
2. **Легальная покупка в РФ.** DX/Atlassian ушёл официально. Остальные продаются в USD/EUR по договорам под правом США, ЕС или Израиля и попадают под режимы OFAC и ЕС (раздел 0). Ни у кого нет рублёвой оплаты и российского юрлица.
3. **Цена для SMB.** У LinearB минимум 50 мест, то есть около $17,4 тыс. в год. Swarmia обойдётся в $45–55 за разработчика в месяц. Гипотеза craftPulse «10 000 ₽ за организацию» ниже на порядок. Ближайшие альтернативы - бесплатные: DevLake (OSS), Swarmia Free (до 9 разработчиков), Port Free (до 15 мест).
4. **Объяснимый скор с drilldown до сырых данных** заявлен частично. Ближе всех Waydev («figure → query → source system»), Cortex (data points) и LinearB (до PR). Сквозного «скор → вклад компонент → PR / check / задача» в документации нет.
5. **Результат в CI/MR (pipeline step, MR-комментарий).** Есть у LinearB (gitStream), Cortex и Port (через workflows). У DX, Jellyfish, Swarmia и Hatica нет. Уникальным это свойство не является.
6. **Импорт результатов клиентского CI (тесты, покрытие)** редок. Покрытие из SonarQube и Codecov импортируют Jellyfish, Cortex и Port. Тест-кейсы через CSV - DevLake. Модель «тесты + покрытие + анализаторы в одном скоре» - разумная ниша.
