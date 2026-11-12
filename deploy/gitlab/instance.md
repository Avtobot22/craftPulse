# Экземпляр GitLab для MVP

| Параметр | Значение |
| --- | --- |
| GitLab | `https://gitlab.defects.team` |
| Стенд craftPulse | `https://craftpulse.defects.team` |
| Хост | `defects`, Ubuntu 24.04, x86_64 |
| Редакция и версия GitLab | Community Edition 19.5.5 |
| Образ GitLab | `gitlab/gitlab-ce:19.5.5-ce.0@sha256:392e6bd5bf118ae520e4038261a5ae39aea05de0d2299a162b4c822a9684cf9b` |
| Версия runner | 19.5.0, Docker executor |
| Образ runner | `gitlab/gitlab-runner:v19.5.0@sha256:f1c76f915d7f70d8b3e25cac58da3d87d3d10106dda82ee780409440e1dc1ccc` |
| Размещение | Два Compose-проекта на `defects`: `craftpulse-gitlab`, `craftpulse-gitlab-runner` |
| Лимит GitLab | 1.5 CPU, 2300 MiB RAM, 3300 MiB RAM + swap |
| Лимит процесса runner | 0.25 CPU, 128 MiB RAM |
| Лимит job | 0.5 CPU, 512 MiB RAM, 768 MiB RAM + swap; одна job одновременно |
| HTTPS | Caddy, Let's Encrypt YE1; upstream `127.0.0.1:8181`, сертификат до 10.02.2027, автопродление Caddy |
| Конфигурация | `/opt/craftpulse/gitlab` |
| Состояние GitLab | `/srv/craftpulse-gitlab/{config,data,logs}` |
| Журнал и секреты | `/etc/craftpulse-gitlab/secrets`, каталог `0700`, файлы `0600` |
| Конфигурация runner | `/etc/craftpulse-gitlab-runner/config.toml`, `0600` |
| Состояние проверки | 12.11.2026: API, OIDC, права, clone-read и CI проверены на `defects` |

## Организации и проекты

Группы `cp87-org-a` и `cp87-org-b` соответствуют организациям `org-a` и `org-b`
стенда. В каждой создаются проекты `private` и `internal` соответствующей
видимости. Shared runners, встроенные Issues и wiki отключены. Runner имеет тег
`cp87-test`, выполняет только задания этих четырёх проектов и заблокирован для
подключения к другим проектам.

| Группа | ID группы | Private-проект | Internal-проект |
| --- | --- | --- | --- |
| `cp87-org-a` | 10 | `cp87-org-a/private`, ID 1 | `cp87-org-a/internal`, ID 2 |
| `cp87-org-b` | 13 | `cp87-org-b/private`, ID 3 | `cp87-org-b/internal`, ID 4 |

Runner ID - 1. Контрольная группа `cp87-control`, ID 16: private-проект ID 5,
internal-проект ID 6. В каждом контрольном проекте создан MR `!1` для проверки
отказа в комментировании.

Контрольные проекты вне этих групп используются только для отрицательных проверок
прав. Учётные записи интеграций и runner к ним не подключаются. Учебные данные
продукта не загружаются; smoke commit и MR служат проверке инфраструктуры.

## Учётные записи

Все перечисленные учётные записи имеют `external = true`, без прав администратора
GitLab. Членство в тестовых проектах задаётся явно. Владелец группы наследует права
в её проектах.

| Учётная запись | ID | Права и назначение | Scope токена |
| --- | --- | --- | --- |
| `cp87-collector` | 2 | Reporter в четырёх тестовых проектах; сбор API и проверка членства | `read_api` |
| `cp87-clone` | 3 | Reporter в четырёх тестовых проектах; отдельное чтение Git по HTTPS | `read_repository` |
| `cp87-mr-bot` | 4 | Reporter в четырёх тестовых проектах; комментарии MR, без записи кода | `api` |
| `cp87-admin-a` | 5 | Owner группы `cp87-org-a`; первый администратор организации `org-a` | Не выпускается |
| `cp87-admin-b` | 6 | Owner группы `cp87-org-b`; первый администратор организации `org-b` | Не выпускается |
| `cp87-member` | 7 | Reporter только в проектах `cp87-org-a` | Не выпускается |
| `cp87-no-repo` | 8 | Нет членства в группах и проектах | Не выпускается |
| `cp87-inactive` | 9 | Учётная запись заблокирована | Не выпускается |

У бота нет доступа к трекеру Jira; Issues в тестовых GitLab-проектах отключены.
Токены сборщика, clone-read и бота раздельные, срок - 30 дней с выпуска. Временный
PAT администратора используется только при настройке и отзывается после приёмки.
Срок действия текущих трёх токенов интеграций - до 12.12.2026.
Пароли и значения токенов в карточку не включаются.

## OIDC

| Приложение | Организация | Callback | Scope |
| --- | --- | --- | --- |
| `craftpulse-org-a` | `org-a` | `https://craftpulse.defects.team/o/org-a/callback` | `openid profile` |
| `craftpulse-org-b` | `org-b` | `https://craftpulse.defects.team/o/org-b/callback` | `openid profile` |

Issuer обоих приложений - `https://gitlab.defects.team`, discovery -
`https://gitlab.defects.team/.well-known/openid-configuration`.
Client ID и client secret передаются владельцу секретов в SOPS + age файле.
Секреты приложений и токены интеграций не передаются в CI.

Приложения имеют ID 1 и 2 соответственно. Client ID, секреты и пароль root
хранятся в локальном `secrets.sops.json`, исключённом из Git; зашифрованный файл
проверен расшифрованием и передан в отдельном комплекте доступа.

## Проверка 12.11.2026

- `/api/v4/version` доступен с `defects` токеном сборщика по доверенному HTTPS;
  версия 19.5.5, редакция CE подтверждена `/api/v4/metadata`.
- [Pipeline #5](https://gitlab.defects.team/cp87-org-a/private/-/pipelines/5),
  [job #3](https://gitlab.defects.team/cp87-org-a/private/-/jobs/3): успешная job
  `stand-connectivity` на runner #1 открыла HTTPS-адрес craftPulse.
  SHA - `b6d1306018bad7f6a2335960aa41849bc03a3461`.
- Сборщик читает проекты 1-4 и получает `404` на проекты 5 и 6. Clone-read
  разрешён в проектах 1-4 и запрещён в проектах 5 и 6.
- Бот оставил комментарий в `cp87-org-a/private!1`; запись комментария в двух
  контрольных MR `!1` отклонена. Наличие контрольных MR подтверждено администратором.
- Оба OIDC-приложения выдали authorization code на точный callback; выполнен
  обмен code, проверены RSA-подпись ID token, issuer, audience и nonce.
- `cp87-inactive` не может войти; `cp87-no-repo` не видит четыре проекта;
  `cp87-member` видит private-проект первой организации и не видит проект второй.
- Саморегистрация и shared runners отключены. Секреты в открытом виде в файлы
  репозитория не включены.

## Копирование и восстановление

Используются штатные `gitlab-backup create` и `gitlab-ctl backup-etc`.
Копия включает данные, конфигурацию GitLab с `gitlab-secrets.json`, журнал
provisioning и конфигурацию runner. Архив шифруется age и хранится вне `defects`.
Восстановление требует той же версии и редакции GitLab, исходных файлов секретов
и закрытого ключа age. После восстановления повторяются проверки API, OIDC,
изоляции доступа и smoke pipeline.

Полная инструкция оператора находится в общей папке команды:
`shared/87-gitlab-test-instance/README.md`.
