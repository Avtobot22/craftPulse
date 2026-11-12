# Тестовый GitLab craftPulse

GitLab доступен по `https://gitlab.defects.team`, стенд - по
`https://craftpulse.defects.team`. TLS завершает Caddy на `defects`.
Контейнер GitLab принимает HTTP только на `127.0.0.1:8181`. Runner работает
отдельным контейнером и выполняет по одной job. Версии и права записаны в
[карточке экземпляра](instance.md).

Новые сервисы подключаются на своих поддоменах `defects.team` отдельными блоками
Caddy. TLS завершает Caddy, upstream слушает локальный порт. Существующие маршруты
сохраняются; перед reload выполняется `caddy validate`.

## Развёртывание

Нужны Linux, Docker Engine с Compose, Python 3.12+, DNS и доверенный HTTPS
на обоих адресах. Команды выполняются из каталога установленной конфигурации,
например `/opt/craftpulse/gitlab`, с правами администратора машины.

1. Скопировать файлы этого каталога на сервер. Скопировать `.env.example` в `.env`.
   Подготовить каталоги из `.env`. Файл начального пароля root создать с правами
   `0600`, каталог секретов - `0700`; пароль сгенерировать случайно.
2. Добавить блок из `Caddyfile` в существующую конфигурацию Caddy. Сохранить
   предыдущий файл, выполнить `caddy validate`, затем reload.
3. Проверить Compose и запустить GitLab:

```sh
docker compose --env-file .env -f compose.yaml config --quiet
docker compose --env-file .env -f compose.yaml up -d
curl --fail --show-error https://gitlab.defects.team/.well-known/openid-configuration
```

4. Войти как root и создать временный PAT со scope `api create_runner manage_runner`.
   Сохранить его в `/etc/craftpulse-gitlab/secrets/admin-token` с правами `0600`.
   Токен нужен только оператору настройки. Не передавать его в runner или CI.
5. Создать учётные записи, проекты, OIDC-приложения и project runner:

```sh
python3 manage.py bootstrap \
  --url https://gitlab.defects.team \
  --stand-url https://craftpulse.defects.team \
  --org-a org-a --org-b org-b \
  --admin-token-file /etc/craftpulse-gitlab/secrets/admin-token \
  --state /etc/craftpulse-gitlab/secrets/state.json
```

`state.json` содержит пароли, токены и секреты приложений. Скрипт записывает его
атомарно с правами `0600`, вне checkout, и сохраняет прогресс после каждого
созданного объекта. Повторный запуск с тем же журналом продолжает настройку.
Объект с занятым именем без записи в журнале требует ручной сверки: скрипт его
не присваивает. При потере ответа API после создания секрета оператор отзывает
неучтённый токен или приложение перед повтором.

Имена `org-a` и `org-b` должны совпадать со slug организаций craftPulse. Приложения
создаются со scope `openid profile`, отдельным client ID и единственным callback
`https://craftpulse.defects.team/o/<slug>/callback`. Discovery общий для экземпляра
GitLab. Реализация обработчика callback относится к приложению craftPulse.

6. Подготовить каталог `/etc/craftpulse-gitlab-runner` с правами `0700`. В его
   подкаталог `certs` положить публичный CA bundle `ca-bundle.crt`, которому
   доверяют сертификаты GitLab и стенда; каталог `certs` и файл доступны на чтение
   контейнеру job. Для публичных сертификатов подходит системный CA bundle.
7. Сформировать конфигурацию runner и запустить отдельный сервис:

```sh
python3 manage.py runner-config \
  --state /etc/craftpulse-gitlab/secrets/state.json \
  --output /etc/craftpulse-gitlab-runner/config.toml
docker compose --env-file runner/.env.example -f runner/compose.yaml up -d
```

Runner использует authentication token `glrt-`, ограничен четырьмя тестовыми
проектами, заблокирован для подключения к другим проектам и принимает только
тег `cp87-test`. Docker socket доступен процессу runner; в контейнеры job он
не передаётся. `privileged = false`. Секреты интеграций и OIDC в CI отсутствуют.

## Проверка

С машины стенда:

```sh
python3 manage.py inspect \
  --url https://gitlab.defects.team \
  --admin-token-file /etc/craftpulse-gitlab/secrets/admin-token \
  --state /etc/craftpulse-gitlab/secrets/state.json
curl --fail --show-error https://gitlab.defects.team/.well-known/openid-configuration
```

`inspect` сверяет версию и редакцию, роли интеграций, блокировку неактивного
пользователя, callback/scopes приложений, online-состояние и проекты runner.
Для внутреннего CA передать `--ca /path/to/ca-bundle.crt`; отключать TLS-проверку
не требуется.

Для проверки CI положить `smoke.gitlab-ci.yml` как `.gitlab-ci.yml` в первый
тестовый private-проект. Задать несекретные переменные `CRAFTPULSE_URL` и
`EXPECTED_RUNNER_ID`. Job проверяет свой runner ID и открывает HTTPS стенда с
проверкой сертификата.

Создать по контрольному private- и internal-проекту вне двух тестовых групп,
без членства интеграционных пользователей. Создать реальный MR в первом тестовом
проекте и по MR с одинаковым IID в обоих контрольных проектах. Затем выполнить:

```sh
python3 manage.py acceptance \
  --url https://gitlab.defects.team \
  --admin-token-file /etc/craftpulse-gitlab/secrets/admin-token \
  --state /etc/craftpulse-gitlab/secrets/state.json \
  --control-private <id> --control-internal <id> \
  --test-mr <iid> --control-mr <iid> --pipeline <id>
```

Проверка читает тестовые проекты токеном сборщика, получает `404` на оба
контрольных проекта, оставляет комментарий ботом в тестовом MR и проверяет
отказ в обоих контрольных MR. Наличие контрольных MR подтверждается токеном
администратора. Проверяются issuer OIDC и успешная smoke job на нужном runner.
Вывод содержит только несекретные ID и результаты.

Clone-read проверяется отдельно через `git ls-remote` с HTTPS и временным
credential helper, читающим только токен `clone-token` из закрытого журнала.
Токен не включается в URL или аргументы процесса. Тот же доступ должен получить
отказ на оба контрольных репозитория. Для `cp87-no-repo` и `cp87-inactive`
проверить отсутствие доступа и отказ входа соответственно.

После проверки отозвать временный административный PAT. Токены интеграций
действуют 30 дней; до истечения оператор выпускает замену и передаёт её владельцу
секретов. Регистрацию пользователей и shared runners скрипт отключает.

## Секреты

Открытые секреты хранятся только в закрытых файлах на машине. Для передачи
используется локальный `secrets.sops.json`, зашифрованный SOPS + age. Он исключён
из Git: комплект доступа передаётся владельцем напрямую участникам команды.
Закрытый ключ age хранится у владельца стенда вне Git и вне `defects`.
Публичный recipient можно хранить в конфигурации шифрования.

```sh
sops encrypt --age <public-recipient> --input-type json --output-type json \
  /secure/state.json > secrets.sops.json
sops decrypt secrets.sops.json > /secure/restored-state.json
```

Команды выполняются с `umask 077`. Не сохранять расшифрованный журнал в checkout.
Runner получает только свой `config.toml`; секреты OIDC и токены сборщика/бота
передаются владельцу конфигурации craftPulse. Закрытый ключ age скопировать
на независимый носитель владельца перед удалением исходного окружения.

## Снять и восстановить состояние

GitLab backup не содержит `gitlab-secrets.json` и конфигурацию. Для полного
восстановления нужны одновременно backup данных, каталог конфигурации GitLab,
журнал provisioning, конфигурация runner и закрытый ключ age. Копии шифруются
публичным ключом владельца и хранятся вне сервера.

Снятие копии:

```sh
docker compose --env-file runner/.env.example -f runner/compose.yaml stop runner
docker compose --env-file .env -f compose.yaml exec -T gitlab gitlab-backup create
docker compose --env-file .env -f compose.yaml exec -T gitlab gitlab-ctl backup-etc
```

Зафиксировать имя созданного backup и версию образа.
Упаковать backup, `/srv/craftpulse-gitlab/config`,
`/etc/craftpulse-gitlab/secrets`, `/etc/craftpulse-gitlab-runner` и Compose/env/Caddy
в закрытый архив, зашифровать age и перенести за пределы `defects`. Проверить
расшифрование и состав архива, затем возобновить runner.

Восстановление выполнять на чистом экземпляре той же версии **19.5.5 CE**:

1. Расшифровать копию в каталог `0700`, восстановить конфигурацию и файлы секретов
   с исходными владельцами и правами. Установить тот же digest образа GitLab.
2. Запустить GitLab, дождаться первоначальной конфигурации. Положить backup
   в `/var/opt/gitlab/backups`, выставить владельца `git:git` внутри контейнера.
3. Остановить Puma и Sidekiq через `gitlab-ctl stop puma` и
   `gitlab-ctl stop sidekiq`. Выполнить внутри контейнера
   `gitlab-backup restore BACKUP=<имя-без-_gitlab_backup.tar>`, затем
   `gitlab-ctl reconfigure` и `gitlab-ctl restart`.
4. Восстановить конфигурацию runner, запустить его. Выполнить `inspect`,
   проверки прав, OIDC и smoke pipeline. Если authentication token runner
   отозван, выпустить новый и обновить зашифрованные секреты.

Для отката изменения конфигурации вернуть сохранённый Compose/Caddy и тот же
образ. Откат версии после миграций требует восстановления соответствующей копии
данных; запуск старого образа на новой базе не используется.

Источники проверены 12.11.2026: [Docker](https://docs.gitlab.com/install/docker/installation/),
[настройки памяти](https://docs.gitlab.com/omnibus/settings/memory_constrained_envs/),
[runner](https://docs.gitlab.com/runner/register/),
[OIDC](https://docs.gitlab.com/integration/openid_connect_provider/),
[external users](https://docs.gitlab.com/administration/external_users/),
[восстановление](https://docs.gitlab.com/administration/backup_restore/restore_gitlab/).
