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

## Секреты и TLS

[stand.sops.json](stand.sops.json) хранится в Git рядом с Compose. Это позволяет восстановить конфигурацию из того же коммита, которым описана установка. Шифрование даёт доступ двум получателям из [правил SOPS](../../.sops.yaml). Один ключ уже используется оператором. Второй подготовлен для передачи ответственному за продукт. Закрытые ключи хранятся вне сервера и Git. Файл содержит новые секреты для чистой установки профиля.

На машине оператора, где доступен закрытый ключ age, выполни из корня репозитория. Скрипт `prepare.py` должен быть в том же checkout на VPS. Каталог секретов создаётся атомарно один раз. Каждый файл читается только UID процесса, которому передаётся в Compose.

```sh
export SOPS_AGE_KEY_FILE=/secure/operator.age.txt
sudo -v
sops decrypt deploy/profile-d/stand.sops.json |
  sudo python3 deploy/profile-d/prepare.py materialize /var/lib/craftpulse-profile-d-secrets
```

Если checkout и Docker находятся на VPS, путь к закрытому ключу остаётся на рабочей станции:

```sh
sops decrypt deploy/profile-d/stand.sops.json |
  ssh defects 'sudo -n python3 /opt/craftpulse/profile-d/deploy/profile-d/prepare.py materialize /var/lib/craftpulse-profile-d-secrets'
```

В `SECRETS_DIR` укажи созданный каталог. Установи файлы `tls-cert.pem` (цепочка, 0444) и `tls-key.pem` (закрытый ключ сервиса, 0400) в этот каталог от root. На новом VPS оставь `BIND_IP=127.0.0.1`, укажи `SITE_URL=https://craftpulse.defects.team` и `EDGE_PROXY_IP=172.31.84.1` для локального Caddy. В самостоятельной установке можно публиковать порты 80 и 443 напрямую. Порты Grafana, VictoriaLogs и readiness worker привязаны к loopback.

## Запуск стенда

Нужны Docker Engine с systemd cgroup и Compose v2, Python 3.9+, SOPS и age на машине оператора. Получи `release-manifest.json` по [инструкции поставки OCI](../../docs/oci-release.md). Выбирай релиз, в котором `api` и `worker` уже поддерживают `healthcheck`. Укажи в `CHECK_PROFILES_PATH` абсолютный путь к YAML-каталогу профилей проверок этого выпуска. API читает каталог при старте. Файл должен быть доступен для чтения UID 65532. Формат описан в [ТЗ](../../docs/technical-specification/technical-specification.md).

```sh
cd deploy/profile-d
cp .env.example .env
# Укажи SECRETS_DIR, SITE_URL, EDGE_PROXY_IP, CGROUP_PARENT и CHECK_PROFILES_PATH.
python3 prepare.py release-env /secure/release-manifest.json >> .env
docker compose -f compose.yaml -f compose.stand.yaml config --quiet
docker compose -f compose.yaml -f compose.stand.yaml up -d --wait
docker compose -f compose.yaml -f compose.stand.yaml ps -a
docker compose -f compose.yaml -f compose.stand.yaml logs migrate
```

Для инициализации организаций и первого администратора используй параметры `api install` из [backend/.env.example](../../backend/.env.example). Запусти `docker compose -f compose.yaml -f compose.stand.yaml run --rm api /usr/local/bin/api install`, передав `INSTALL_ORGANIZATIONS` и `ORG_<SLUG>_*` через операторское дополнение к Compose с отдельными файловыми монтированиями для OIDC-секретов. При обновлении действующей базы используй исходные зашифрованные ключи этой базы и её резервную копию.

Проверь `/readyz` приложения через nginx и worker на `http://127.0.0.1:18082/readyz`. Контейнерные пробы API и worker обращаются к `/livez`. Если БД недоступна, `/readyz` отвечает 503, а `/livez` продолжает отвечать, пока жив процесс. nginx повторяет запрос при 503, если за DNS-именем API есть другой экземпляр. Для заявок с побочными эффектами nginx сохраняет стандартные ограничения на повторы.

При повторном `up` сравни `SELECT id, version_id, is_applied, tstamp FROM goose_db_version ORDER BY id` в базе craftpulse. Если Compose запустит завершённый сервис повторно, goose сверит версию без новых записей. Сравни вывод `docker stats` с контейнерными лимитами, а пик памяти slice с общим лимитом.

```sh
slice_path=$(systemctl show craftpulse-d.slice -p ControlGroup --value)
cat "/sys/fs/cgroup${slice_path}/memory.peak"
```

Проверь `413` запросом 26 MiB, `429` серией запросов к входу, перенаправление HTTP → HTTPS и наличие логов и метрик в сервисах наблюдаемости.

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
