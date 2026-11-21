# Адаптер Jira Cloud

Соберите бинарник из каталога `backend/`.

```sh
go build ./cmd/adapter-jiracloud
```

В `BUILTIN_ADAPTERS` ядра зарегистрируйте адрес адаптера под именем `jira_cloud`. В подключении задайте HTTPS-адрес Jira Cloud, поля `email` и `token` служебной учётной записи и числовой ID проекта в области `tracker_project`. После замены учётных данных повторите `Probe`.

`Probe` проверяет сборку Jira и читает задачу и страницу changelog в каждом доступном проекте. Возможность создавать задачи определяется по `CREATE_ISSUES`. Ключ экземпляра берётся из Cloud ID. Если Jira не публикует Cloud ID, используется нормализованный HTTPS-адрес. `CheckAccess` возвращает `unknown` до подтверждения своевременного отзыва прав в Jira Cloud. [Протокол проверки источников](../../deploy/source-checks/protocol.md)

`Fetch` с видом `tasks` возвращает текущие задачи и справочники статусов, типов, исходов и полей. Уровень безопасности сохраняется в `restriction`, тип и направление связи вместе с ID связанной задачи сохраняются в `raw_fields`. Вид `task_events` отдаёт события создания и изменения полей. Если история не помещается в обычный проход, страница перечисляет задачи в `selection.truncated`. Ядро дочитывает их адресно с `object_keys`, используя `startAt` REST API changelog. [Jira REST v3. Changelog](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-changelog-get)

`Manifest` перечисляет числовые ID задач и `updated_at` за период `[from, to)`. Между страницами адаптер продолжает с пары `updated_at` и ID. В начале следующего прохода ядро сдвигает границу на 10 минут назад, а сверка манифеста выявляет изменения, которые не попали в выдачу поиска. [Jira REST v3. Поиск JQL](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-get)

Вид `accounts` использует стабильный `accountId`. Для поиска по проекту Jira фильтрует только первые 1000 аккаунтов сайта. Адаптер помечает такие страницы как `partial`. [Jira REST v3. Поиск пользователей](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-user-search/#api-rest-api-3-user-viewissue-search-get)

Контрактные тесты запускаются с PostgreSQL в Docker. Тестовый сервер использует синтетические ответы REST v3 по протоколу стенда. Длинный changelog состоит из 122 записей на трёх страницах.

```sh
go test ./internal/app -run 'TestContract_JiraCloudAdapter|TestJiraCloudAdapter'
```
