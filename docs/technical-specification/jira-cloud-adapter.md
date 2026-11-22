# Адаптер Jira Cloud

Соберите бинарник из каталога `backend/`.

```sh
go build ./cmd/adapter-jiracloud
```

В `BUILTIN_ADAPTERS` ядра зарегистрируйте адрес адаптера под именем `jira_cloud`. В подключении задайте HTTPS-адрес Jira Cloud, поля `email` и `token` служебной учётной записи и числовой ID проекта в области `tracker_project`. После замены учётных данных повторите `Probe`.

`Probe` проверяет сборку Jira и читает задачу и страницу changelog в каждом доступном проекте. Возможность создавать задачи определяется по `CREATE_ISSUES`. Ключ экземпляра берётся из Cloud ID. Если Jira не публикует Cloud ID, используется нормализованный HTTPS-адрес.

`CheckAccess` берёт сохранённый OAuth-токен пользователя (`user_token`) и Cloud ID из `Probe`. Адаптер сверяет точный `accountId` через `myself` и читает запрошенные проекты и задачи по числовым ID. На пакет отведено 2 секунды, одновременно выполняется до восьми запросов на подключение. [Atlassian OAuth 2.0 (3LO)](https://developer.atlassian.com/cloud/jira/platform/oauth-2-3lo-apps/)

При чтении проекта или задачи `403` и `404` дают `denied` со свежестью до 60 секунд. Успешное чтение даёт `unknown`. В стендовом прогоне Jira разрешала чтение на 95-й секунде после отзыва. Для допуска нужен отдельный источник, подтверждающий актуальные права проекта и уровень безопасности задачи. Ошибка, истечение бюджета или отсутствие токена также дают `unknown`. Ядро получает нулевой срок кэширования для пакета с любым `unknown`. [Протокол проверки источников](../../deploy/source-checks/protocol.md)

`Fetch` с видом `tasks` возвращает текущие задачи и справочники статусов, типов, исходов и полей. Уровень безопасности сохраняется в `restriction`, тип и направление связи вместе с ID связанной задачи сохраняются в `raw_fields`. Вид `task_events` отдаёт события создания и изменения полей. Если история не помещается в обычный проход, страница перечисляет задачи в `selection.truncated`. Ядро дочитывает их адресно с `object_keys`, используя `startAt` REST API changelog. [Jira REST v3. Changelog](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-changelog-get)

`Manifest` перечисляет числовые ID задач и `updated_at` за период `[from, to)`. Между страницами адаптер продолжает с пары `updated_at` и ID. В начале следующего прохода ядро сдвигает границу на 10 минут назад, а сверка манифеста выявляет изменения, которые не попали в выдачу поиска. [Jira REST v3. Поиск JQL](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-get)

Вид `accounts` использует стабильный `accountId`. Для поиска по проекту Jira фильтрует только первые 1000 аккаунтов сайта. Адаптер помечает такие страницы как `partial`. [Jira REST v3. Поиск пользователей](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-user-search/#api-rest-api-3-user-viewissue-search-get)

Контрактные тесты запускаются с PostgreSQL в Docker. Тестовый сервер воспроизводит пагинацию, отказ в чтении истории и ответы с ошибками. Проверки `CheckAccess` используют обезличенные примеры ответов по сценарию из протокола, когда проект виден, а задача с уровнем безопасности скрыта. Отдельные сценарии проверяют отзыв доступа, тайм-аут и общий лимит запросов. Тест декодирования читает записанные ответы Jira Cloud, включая changelog из 122 записей на трёх страницах.

```sh
go test ./internal/app -run 'TestContract_JiraCloudAdapter|TestJiraCloud'
```
