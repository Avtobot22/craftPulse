# Протокол проверки источников MVP-01-T17

Прогон от 21.11.2026 для [#91](https://github.com/Avtobot22/craftPulse/issues/91) и вопроса Q-10 [раздела 13.5 ТЗ](../../docs/technical-specification/technical-specification.md#135-проверка-прав-в-источниках). Atlassian-проверки выполнены на Jira Cloud и Confluence Cloud из [#88](https://github.com/Avtobot22/craftPulse/issues/88), GitLab-проверки - на стенде из [#87](https://github.com/Avtobot22/craftPulse/issues/87).

| Источник | Экземпляр на дату прогона | Режим для `CheckAccess` | Результат |
| --- | --- | --- | --- |
| Jira Cloud | `craftpulse-mvp.atlassian.net`, REST v3, `serverInfo.version=1001.0.0-SNAPSHOT`, build `100296` | `user_token` как часть проверки. Выдача закрыта до подтверждения отдельного контроля отзыва | Прямое чтение и `BROWSE_PROJECTS` разрешали доступ после отзыва в серии на 95-й секунде |
| Confluence Cloud | Тот же сайт, `edition=premium`, REST v2 для пространств, страниц и версий, REST v1 для CQL | `user_token`, то же приложение 3LO | Права на пространства и страницы, OAuth refresh и `Probe` подтверждены на стенде |
| GitLab self-managed | `gitlab.defects.team`, CE 19.5.5, revision `342e9f9e0b4`, REST v4 | Служебный Reporter с `read_api` | Режим подтверждён для проверенных private/internal-проектов. Допуск требует активного аккаунта и уровня от Reporter |

Для всех трёх источников изменившиеся во время обхода объекты пропали из отдельных проходов. Полные контрольные перечитывания вернули исходное множество идентификаторов. В GitLab отдельные MR ещё и повторились.

## Atlassian Cloud. Доступ

OAuth-проверку выполнил сервис стенда из [#88](https://github.com/Avtobot22/craftPulse/pull/308) токенами пяти тестовых ролей. Команда `inspect` возвращает HTTP-статусы и длительность отдельных запросов. `GET /rest/api/3/myself` сверяет точный `accountId` и активность пользователя до чтения. Запросы Jira идут через `https://api.atlassian.com/ex/jira/{cloudId}`, запросы Confluence - через `https://api.atlassian.com/ex/confluence/{cloudId}`.

| Роль | Jira проекты `CPA / CPB / CPZ` | Jira задача `CPA-1` | Confluence пространства и контрольные страницы A / B | Confluence ограниченная страница |
| --- | --- | --- | --- | --- |
| A с доступом к ограничениям | `200 / 200 / 404` | `200` | `200 / 404` | `200` |
| A без доступа к ограничениям | `200 / 200 / 404` | `404` | `200 / 404` | `404` |
| B | `404 / 404 / 200` | `404` | `404 / 200` | `404` |
| Сборщик | `200 / 200 / 200` | `200` | `200 / 200` | `200` |
| Владелец стенда | `200 / 200 / 200` | `200` | `200 / 200` | `200` |

- Jira использует `GET /rest/api/3/project/{key}` и `GET /rest/api/3/issue/{key}?fields=id`. Требуются `read:jira-work`, `read:jira-user`, право `Browse projects` и допуск уровня безопасности задачи. Токен сборщика предназначен для импорта. Контроль под сборщиком вызвал `GET /rest/api/3/user/viewissue/search` с `accountId` каждой роли, `maxResults=100` и одним из параметров `projectKey=CPA` или `issueKey=CPA-1`. Все шесть запросов вернули `200` за 1047-1500 мс. Оба фильтра включили две роли A и исключили B. Поиск по `issueKey` включил роль A без доступа к `CPA-1`, хотя прямое чтение вернуло ей `404`. Идентификаторы сравнивались по `accountId`. [Jira REST v3. User search](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-user-search/)
- Confluence использует `GET /wiki/api/v2/spaces/{id}` и `GET /wiki/api/v2/pages/{id}?body-format=storage`. Контрольные страницы имеют ID `360481`, `360631` и `491521`. Нужны право чтения пространства, право чтения ограниченной страницы и scopes `read:space:confluence`, `read:page:confluence`, `read:content-details:confluence`. Запреты в матрице вернули `404`. Обработка `403` в адаптере также должна закрывать объект. [Confluence REST v2. Page](https://developer.atlassian.com/cloud/confluence/rest/v2/api-group-page/)
- В каждом из двух OAuth-прогонов пять ролей дали 45 ожидаемых ответов, включая пространства Confluence `360451` и `360600`. В прямых замерах с сервера стенда запросы Jira заняли 409-1204 мс, Confluence - 357-1123 мс. Каждый запрос уложился в 2 с. Бюджет одной проверки в адаптере зависит также от числа запросов и времени обновления токена.
- Стенд использует `offline_access` и `POST https://auth.atlassian.com/oauth/token` с `grant_type=refresh_token`. Команда `refresh a_allowed` подтвердила HTTP `200` и ротацию refresh token. Чтения той же роли после обновления прошли. Для `Probe` сервис вызывает `GET https://api.atlassian.com/oauth/token/accessible-resources` с пользовательским Bearer и сверяет `cloudId`, URL и scopes. Ресурсы Jira и Confluence могут иметь один `id`. Scopes проверяются в ресурсе соответствующего продукта. Запрос вернул `200` за 265-307 мс на всех пяти ролях. URL стенда и scopes обоих продуктов совпали. [Atlassian OAuth 2.0 (3LO)](https://developer.atlassian.com/cloud/confluence/oauth-2-3lo-apps/)

### Jira. Отзыв доступа

Тестовую роль A с доступом к ограничениям убрали из группы `cp88-a-read`. `DELETE /rest/api/3/group/user?groupId={id}&accountId={id}` вернул `200`, отсутствие пользователя в группе подтверждено чтением списка участников. Другие группы роли сохранились.

| Время завершения контрольной серии после подтверждения отзыва | `CPA` | `CPB` | `CPA-1` | Confluence A / ограниченная страница |
| --- | --- | --- | --- | --- |
| До отзыва | 200 | 200 | 200 | 200 / 200 |
| 9 с | 404 | 200 | 200 | 200 / 404 |
| 21-94 с, семь серий | 200 | 200 | 200 | 404 / 404 |
| После восстановления членства | 200 | 200 | 200 | 200 / 200 |

В первом прогоне с ролью A без доступа к ограничениям оба Jira-проекта также продолжали возвращать `200`. В обоих прогонах членство восстановлено. Результаты относятся к чтению через источник и указывают, что свежий ответ Jira на чтение проекта или задачи сам по себе не подтвердил прекращение доступа за 60 с.

В повторном прогоне запрос `GET /rest/api/3/mypermissions?permissions=BROWSE_PROJECTS` выполнялся с `projectKey=CPA`, `projectKey=CPB` и `issueKey=CPA-1` тем же OAuth-токеном. Прямые чтения Jira все пять серий после отзыва дали `200` по обоим проектам и задаче.

| Секунд после ответа на отзыв | Browse `CPA` | Browse `CPB` | Browse `CPA-1` | Пространство A и ограниченная страница Confluence |
| --- | --- | --- | --- | --- |
| 15 | `200 / true` | `200 / true` | `404` | `404 / 404` |
| 34 и 54 | `200 / true` | `200 / true` | `200 / true` | `404 / 404` |
| 75 | `404` | `200 / true` | `200 / true` | `404 / 404` |
| 95 | `200 / true` | `200 / true` | `200 / true` | `404 / 404` |

Перед восстановлением членство ещё отсутствовало. После его восстановления прямые чтения Jira и Confluence снова дали `200`. Результат отдельного `404` Jira менялся между сериями, поэтому адресная проверка `BROWSE_PROJECTS` тоже не обеспечила устойчивый запрет за 60 с. [Jira REST v3. Get my permissions](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-permissions/#api-rest-api-3-mypermissions-get)

## GitLab. Доступ

Сборщик `cp87-collector` использовал штатный PAT `read_api` из комплекта #87. `GET /api/v4/projects/1/members/2` подтверждает роль Reporter. Для проверки пользователя применяется `GET /api/v4/projects/{id}/members/all/{user_id}`. Результаты включают сетевую задержку через SSH-туннель с проверкой HTTPS GitLab.

| Сценарий | Проект / пользователь | HTTP | Поля | Время |
| --- | --- | --- | --- | --- |
| Участник через группу | `1 / 5` | 200 | `access_level=50`, `state=active` | 593 мс |
| Прямой участник | `1 / 7` | 200 | `access_level=20`, `state=active`; прямой `/members/7` тоже 200 | 704 мс |
| Неучастник private-проекта | `1 / 8` | 404 | - | 563 мс |
| Заблокирован, членства нет | `1 / 9` | 404 | `GET /api/v4/users/9` вернул `200` и `state=blocked` | 640 мс |
| Заблокирован, выдан Reporter | `7 / 9` | 200 | `access_level=20`, `state=blocked`. Тестовое членство снято после замера | 657 мс |
| Участник internal-проекта | `2 / 7` | 200 | `access_level=20`, `state=active` | 594 мс |
| Неучастник internal-проекта | `2 / 8` | 404 | - | 656 мс |
| Сборщик вне области internal-проекта | `6 / 7` | 404 | Контрольный проект #87 существует | 422 мс |

На проверенных private/internal-проектах разрешение требует `200`, `state=active` и `access_level>=20`. Поле `external` для других пользователей в ответе о членстве отсутствовало. Для internal-проекта применяется граница из раздела 13.5 - только подтверждённые участники. Запросы уложились в 2 с. [GitLab API участников](https://docs.gitlab.com/api/project_members/)

## История и пагинация

### Полное чтение истории

| Запрос | Результат | Время на страницу |
| --- | --- | --- |
| Jira `GET /rest/api/3/issue/CPA-8?expand=changelog&fields=updated` | `total=122`, при прямом чтении пришло 100 встроенных записей. Поиск `GET /rest/api/3/search/jql` с `expand=changelog` вернул для той же задачи 40 записей | 1406 мс для чтения задачи |
| Jira `GET /rest/api/3/issue/CPA-8/changelog?maxResults=25&startAt={offset}` | 25 / 25 / 25 / 25 / 22, всего 122 уникальных `id`. Контроль с `maxResults=100` дал 100 / 22. Дочитывать с `startAt + len(values)` до `isLast` | 1125-1312 мс |
| Confluence `GET /wiki/api/v2/pages/491521/versions?limit=1` | 2 версии на 2 страницах, номера совпали с контрольным `limit=50` | 938-1468 мс |
| Confluence `GET /wiki/api/v2/spaces/360451/pages?limit=2` | 31 страница Confluence за 16 HTTP-запросов, набор совпал с `limit=50`. Переход по `Link rel=next` или `_links.next` | 797-1344 мс |
| GitLab `GET /api/v4/projects/7/merge_requests/1/discussions?per_page=2` | 12 обсуждений за 6 запросов; идентификаторы совпали с контрольной выдачей `per_page=100` | 515-672 мс |
| GitLab `GET /api/v4/projects/7/merge_requests/1/resource_state_events?per_page=2` | 8 событий закрытия и повторного открытия за 4 запроса; набор совпал с `per_page=100`. Переход через `X-Next-Page` или `Link rel=next` | 500-719 мс |

У Jira отдельный changelog покрывает всю историю с первой страницы. Для GitLab обсуждения и смены состояния читаются двумя коллекциями. [Jira REST v3. Issues](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/), [GitLab Discussions API](https://docs.gitlab.com/api/discussions/), [GitLab Resource state events API](https://docs.gitlab.com/api/resource_state_events/)

### Изменение объектов во время чтения приращения

Для каждого источника контрольная выдача содержала 5 объектов. В основном проходе размер страницы был равен 2. После страницы 1 оператор изменил два последних объекта и дождался их появления в начале выдачи. После обхода контрольный запрос снова вернул все 5 объектов.

| Источник и точный запрос | Страницы основного обхода | Сверка с полным перечнем |
| --- | --- | --- |
| Jira `GET /rest/api/3/search/jql`, `maxResults=2`, `fields=updated`, `jql=project = CPA AND labels = cp91-pagination AND updated >= "2026-11-20" ORDER BY updated DESC, id DESC`. Продолжение по `nextPageToken` до `isLast` | 2 / 1 строки, 953-1047 мс | Пропущены `CPA-52` и `CPA-53`, повторов 0. Полный перечень содержит 5 задач |
| Confluence `GET /wiki/rest/api/search`, `limit=2`, `cql=type = page AND space = ORGA AND title ~ "cp91paginationfixture" AND lastmodified >= "2026-11-20" ORDER BY lastmodified DESC`. Продолжение по ссылке `next` | 2 / 1 строки, 891-938 мс | Пропущены страницы `2392066` и `2424833`, повторов 0. Полный перечень содержит 5 страниц |
| GitLab `GET /api/v4/projects/7/merge_requests?updated_after=2026-11-20T08:27:55Z&state=all&order_by=updated_at&sort=desc&per_page=2`. Продолжение по `X-Next-Page` или `Link rel=next` | 2 / 2 / 1 строки, 453-641 мс | Пропущены MR `!2` и `!3`; повторились `!1` и `!5`. Полный перечень содержит 5 MR |

Для приращений Jira, Confluence и GitLab нужно перечитывать перекрывающийся интервал, применять запись идемпотентно по нативному ID и версии и обновлять checkpoint после сверки. Размер перекрытия и период полной сверки зависят от задержки индексации и частоты изменений. [Jira Enhanced JQL search](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/), [Confluence CQL search](https://developer.atlassian.com/cloud/confluence/rest/v1/api-group-search/), [GitLab Merge requests API](https://docs.gitlab.com/api/merge_requests/)

## Решение по Q-10

В продакшене выдача Jira Cloud закрыта по умолчанию. Сбор может продолжаться. Проект Jira открывается только после проверки AC-12 и NFR-011 для подключения. Нужны пользовательский OAuth и отдельное свежее подтверждение проектного права и ограничения каждой задачи. При неопределённости `CheckAccess` возвращает `unknown`. Недоступные задачи и их вклад исключаются из выдачи, включая сохранённые срезы, прямые ссылки и выгрузки. Это правило закреплено в [разделе 13.5 ТЗ](../../docs/technical-specification/technical-specification.md#135-проверка-прав-в-источниках).

Прямое чтение и адресный `BROWSE_PROJECTS` дали разрешение в серии на 95-й секунде после отзыва. Обновление TTL по этим ответам и сверка по ночному расписанию не подходят для допуска. Webhook можно использовать для раннего закрытия и сверки. Сквозная проверка подтверждает, что после удаления из группы, смены схемы проекта и нового ограничения задачи ни одна поверхность не выдаёт её данные или вклад позже 60 с. Отдельно проверяются потеря уведомления, недоступность прав, перезапуск и повторное открытие после свежей сверки.

## Повтор прогона

Из корня репозитория запусти [source_checks.py](source_checks.py) с копией [stand.example.json](stand.example.json). Нужен Python 3.10+ и доступ по HTTPS к выбранному источнику. В копии плана укажи адреса экземпляра и идентификаторы тестовых проектов, пользователей и объектов. Замени `...-accountId` на ID тестовых ролей из стендового перечня. Переменные `CP91_*_BEARER` получают собственный OAuth access token роли, `CP91_ATLASSIAN_BASIC` - Base64 от пары email и API-токена сборщика, `CP91_GITLAB_TOKEN` - PAT `read_api` из комплекта #87.

```sh
python deploy/source-checks/source_checks.py stand.local.json --source jira_a_allowed --source confluence_a_allowed --source atlassian_discovery
python deploy/source-checks/source_checks.py stand.local.json --source jira_user_search
python deploy/source-checks/source_checks.py stand.local.json --source atlassian_collection --interactive
python deploy/source-checks/source_checks.py stand.local.json --source gitlab --interactive
```

В проверке отзыва зафиксируй исходное членство и положительные ответы. От имени оператора удали тестовую роль из группы Jira указанным запросом, подтвердив отсутствие через `GET /rest/api/3/group/member?groupId={id}`. С тем же OAuth-токеном опрашивай оба проекта и ограниченную задачу не менее 60 с. Зафиксируй первый запрет и все последующие ответы. Верни членство `POST /rest/api/3/group/user?groupId={id}` с телом `{"accountId":"..."}`, затем проверь восстановление чтения. Режим `gitlab_blocked_fixture` запускай отдельно между выдачей и снятием временного Reporter для заблокированной тестовой учётной записи в проекте 7. Для прямого прогона на стенде вызови `atlassian-check -config <config> refresh a_allowed` и затем `atlassian-check -config <config> inspect a_allowed`. `inspect` выводит время чтения объектов, результат `accessible-resources` и адресные проверки Jira `BROWSE_PROJECTS`. Для матрицы повтори `inspect` с ролями `a_denied`, `b`, `collector` и `owner`.

Для приращения используй по пять стендовых объектов. Проверь дату нижней границы и инвентарь в плане. На паузе после первой страницы обнови два последних объекта из показанного списка. Для Jira можно поменять summary через `PUT /rest/api/3/issue/{key}`, для Confluence - тело через `PUT /wiki/api/v2/pages/{id}` с увеличением версии, для GitLab - описание через `PUT /api/v4/projects/{id}/merge_requests/{iid}`. Дождись появления обеих правок в начале отдельной контрольной выдачи и продолжи Enter. Скрипт читает страницы и показывает `missing`, `extra` и `duplicates`. При `allow_drift` успешный результат означает, что эксперимент завершился и контрольный набор совпал с ожидаемым размером. Пропуски и повторы остаются в отчёте.
