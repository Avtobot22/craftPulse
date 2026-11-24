# Профиль D на defects

Площадка - VPS Ubuntu по адресу `193.34.161.188`. Оператор подключается как `nikita` через SSH-порт 22 или `ssh defects`. Публичное имя приложения - `craftpulse.defects.team`. На общем сервере публичный TLS обслуживает Caddy с сертификатом Let's Encrypt. nginx профиля принимает HTTPS на loopback. Для него оператор устанавливает сертификат площадки с цепочкой и ключом и указывает в Caddy имя для проверки TLS `craftpulse.defects.team`.

## Ресурсы

На VPS 8 vCPU и около 15 ГиБ RAM. GitLab, Jenkins и SonarQube работают на той же машине. Для craftPulse задаётся общий потолок 4 vCPU и 5,5 ГиБ. На запуске остаётся 416 MiB внутри этого потолка сверх суммы ограничений контейнеров. Основной запас памяти остаётся снаружи slice.

| Компонент | Лимит MiB | Дополнительная настройка |
| --- | ---: | --- |
| PostgreSQL 18.7 | 1536 | `shared_buffers=512MB`, `work_mem=8MB`, 40 соединений |
| SeaweedFS 4.55 | 512 | Один узел, бакет `craftpulse` |
| API | 512 | `GOMEMLIMIT=416MiB`, пул 10 |
| Worker | 1024 | 2 vCPU, `GOMEMLIMIT=850MiB`, пул 10, по 2 слота для `interactive` и `bulk` |
| Миграция | 512 | Одноразовый сервис под ролью-владельцем |
| nginx с frontend | 96 | TLS, 25 MiB, ограничение частоты входа и webhook |
| VictoriaMetrics и VictoriaLogs | 256 + 256 | Метрики 13 месяцев, логи 30 дней |
| Grafana | 512 | Источник метрик и дашборд состояния |

Запущенные сервисы ограничены суммарно 4704 MiB. С миграцией - 5216 MiB. Лимит swap равен лимиту памяти контейнера. На VPS оператор устанавливает [systemd slice](craftpulse-d.slice):

```sh
sudo install -m 0644 craftpulse-d.slice /etc/systemd/system/craftpulse-d.slice
sudo systemctl daemon-reload
sudo systemctl start craftpulse-d.slice
```

В файле окружения укажи `CGROUP_PARENT=craftpulse-d.slice`. Для устойчивости при совместных сборках проверь `systemctl show craftpulse-d.slice -p CPUQuotaPerSecUSec -p MemoryMax -p MemorySwapMax` и фактическую память у источников перед запуском.

## Подготовка к первому выкату

На Linux-стенде нужны Docker Engine, Compose v2, Python 3.9+ и age. На машине оператора
нужны Python 3.9+, OpenSSH, ORAS, SOPS, age и OpenSSL. Если пакеты реестра закрытые,
выполни вход в GHCR на обоих компьютерах под учётными записями, от которых
запускаются ORAS и Docker. На стенде Docker запускается от root.

1. Размести checkout с командой выката в `/opt/craftpulse/profile-d`. Подключись
   через проверенный SSH-ключ хоста. У оператора должен быть доступ к запуску
   `rollout_host.py` через `sudo -n`. Checkout и файлы настроек, которые читает
   команда от root, должны быть доступны на запись только администратору стенда.
2. Создай `deploy/profile-d/.env` по [.env.example](.env.example). Задай
   `CGROUP_PARENT=craftpulse-d.slice`, `BIND_IP=127.0.0.1`,
   `SITE_URL=https://craftpulse.defects.team` и `EDGE_PROXY_IP=172.31.84.1`.
   Выкат сам задаёт `SECRETS_DIR`, `CHECK_PROFILES_PATH` и ссылки на образы.
3. Создай `deploy/profile-d/rollout.json` по [rollout.example.json](rollout.example.json).
   Укажи каталог TLS с `tls-cert.pem` (цепочка площадки) и `tls-key.pem`, YAML-каталог
   профилей проверок нужного выпуска и публичного получателя резервных копий age.
   В `install` задай параметры служебной организации и её OIDC-клиента.
   `installation_networks` и `source_private_networks` задают сетевые области
   адаптеров. При прямом доступе к локальному GitLab добавь его адрес в разрешённую
   область источников, если он находится в приватной сети.
4. На машине оператора дополни [stand.sops.json](stand.sops.json) через `sops edit`.
   Сохрани исходные пароли БД и мастер-ключи действующего стенда. Добавь
   `oidc_client_secrets` как объект «slug организации → секрет клиента».
   Для встроенных адаптеров добавь объект `adapters`, где имя без префикса
   `craftpulse-adapter-` указывает на `owner_password`, `app_password`,
   `master_key` (32 байта в Base64) и `service_key`. Добавь PEM сертификата и
   закрытого ключа внутреннего центра в `adapter_ca_cert` и `adapter_ca_key`.
   Центр должен иметь назначение CA и срок действия больше года. Используй один
   центр для установки, чтобы новый выпуск ядра доверял уже запущенным адаптерам.
   Для хранения ключей используй получателей из [.sops.yaml](../../.sops.yaml).

Для новой установки создай центр на рабочей станции в закрытом каталоге:

```sh
umask 077
openssl req -x509 -newkey ed25519 -nodes -days 1826 \
  -subj '/CN=craftPulse installation CA' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -keyout adapter-ca.key -out adapter-ca.pem
```

Перенеси PEM из этих файлов в соответствующие поля через `sops edit`.
Храни закрытый ключ центра в зашифрованном комплекте установки.

В CI публикуются [OCI-образы и манифест](../../docs/oci-release.md). Закрытый ключ
age и ключ центра остаются на машине оператора. SOPS расшифровывает документ в
памяти, команда передаёт по SSH только манифест, имена запрошенных адаптеров и
отдельные файлы процессов. Для адаптеров OpenSSL выдаёт сертификаты на 365 дней
с DNS-именами Compose. Ядро и проверки адаптеров используют сертификат центра.

## Выкат

На машине оператора из корня checkout выполни:

```sh
export SOPS_AGE_KEY_FILE=/secure/operator.age.txt
python3 deploy/profile-d/rollout.py deploy <SHA>
```

Для другой площадки перед `deploy` укажи `--host`, `--stand-dir` или `--state`.
Подкоманда принимает полный SHA из успешного прогона OCI. Чтобы взять отдельный
зашифрованный файл, передай `deploy --bundle /secure/stand.sops.json <SHA>`.
Первый выкат создаёт инфраструктуру профиля D, базы выбранных адаптеров и
служебную организацию. Для действующего профиля первый успешный запуск задаёт
выпуск, к которому команда сможет возвращаться автоматически.

Выкат скачивает образы ядра, frontend и выбранных адаптеров по digest. Перед
повышением версии схемы он снимает `pg_dump -Fc` под ролью копирования, шифрует
архив публичным ключом age и сохраняет его в каталоге `backups/` на хосте.
Миграции выполняются последовательно под ролями-владельцами. После них
Compose перезапускает `worker` с SIGTERM и 90 с на остановку, затем `api` и по
одному адаптеры с новым образом или изменившимися параметрами. Для каждого
процесса скрипт ждёт совпадения SHA в `/version` и готовности в `/readyz`.
Операцию завершает протокол `api install` с исходом `passed` и SHA выпуска.
Параллельный запуск на том же стенде отклоняется.

## Откат

При сбое после начала переключения команда возвращает последний успешный выпуск
и проверяет его `/version` и `/readyz`. Исходная команда завершается ненулевым
кодом и после успешного возврата. Первый выкат без сохранённого выпуска
останавливает новые процессы приложения. При потерянном SSH-соединении прочитай
журнал. Следующий запуск восстановит сохранённый выпуск, если переключение
было прервано.

Для выбора ранее успешно установленного выпуска выполни:

```sh
python3 deploy/profile-d/rollout.py rollback <прежний-SHA>
```

Ручной откат использует сохранённую конфигурацию выпуска и также выполняет
`api install`. Схема базы остаётся на применённой версии. Для отката нужны
доступные digest, совместимость кода с текущей схемой и действующие сертификаты
адаптеров выбранного выпуска.

Журнал на стенде находится в
`/var/lib/craftpulse-profile-d/rollout/journal.jsonl`. Каждая запись содержит
SHA, digest образов, время, исход и признак отката. `state.json` указывает на
активный выпуск. Каталог `releases/` хранит приватные файлы процессов и конфигурацию
для отката. Оставляй каталоги выпусков, которые нужны для возврата или всё ещё
смонтированы в контейнеры инфраструктуры.

Для приёмки выкати два опубликованных SHA, второй с повышением версии схемы.
На стенде сравни `/version`, запись успешного протокола и
`SELECT version_id, is_applied, tstamp FROM goose_db_version ORDER BY id`.
Проверь, что файл `*.dump.age` появился перед применением новой миграции.
На отдельной установке выкати третий SHA с YAML-каталогом профилей, который API
отвергает при старте. Журнал должен показать сбой и успешный автоматический
возврат второго SHA. Верни первый SHA вручную и проверь, что версия кода
изменилась, а записи миграций сохранились.

## Локальный запуск

На Linux с работающим Docker подготовь закрытый каталог и локальный сертификат:

```sh
sudo python3 prepare.py init-local /var/lib/craftpulse-profile-d-local
sudo openssl req -x509 -newkey ed25519 -nodes -days 7 -subj /CN=localhost \
  -addext subjectAltName=DNS:localhost,IP:127.0.0.1 \
  -keyout /var/lib/craftpulse-profile-d-local/tls-key.pem \
  -out /var/lib/craftpulse-profile-d-local/tls-cert.pem
sudo chmod 0400 /var/lib/craftpulse-profile-d-local/tls-key.pem
cp .env.example .env
# Укажи SECRETS_DIR=/var/lib/craftpulse-profile-d-local.
docker compose -f compose.yaml -f compose.local.yaml build migrate nginx
docker compose -f compose.yaml -f compose.local.yaml up -d --wait
```

Этот запуск собирает те же Dockerfile, что и OCI workflow. Локальные значения и сертификат создаются вне checkout. Локальный каталог проверок пуст и предназначен для проверки запуска. Для проверки HTTPS используй локальный сертификат через `curl --cacert`. Для обращений к Grafana и VictoriaLogs используй loopback или SSH-перенаправление портов.
