# Развёртывание стенда craftPulse

[Манифест](instance.json) фиксирует образы и лимиты стенда после переноса [#397](https://github.com/Avtobot22/craftPulse/issues/397).
Compose-файлы соответствуют каталогам на сервере.

Для выката с Jira Cloud переведи приложение на профиль D по
[инструкции перехода](../profile-d/jira-cloud.md#переход-действующего-стенда).
Файлы ниже также нужны для восстановления зафиксированного снимка.

Перед показом выполни [шаг 0](snapshot/README.md). Команда восстанавливает начальный снимок
по [manifest стенда](manifest.json) и сверяет состояние и версии источников.

| Файл | Каталог |
| --- | --- |
| [GitLab](gitlab.compose.yaml) | `/opt/craftpulse/gitlab` |
| [Runner](gitlab-runner.compose.yaml) | `/opt/craftpulse/gitlab/runner` |
| [Jenkins и SonarQube](ci90.compose.yaml) | `/opt/craftpulse/ci90` |
| [Приложение](product.compose.yaml) | `/opt/craftpulse/product` |

На сервере имя каждого файла - `compose.yaml`. К GitLab приложи `.env` по [образцу](../gitlab/.env.example).
Секреты поставь в пути, указанные в volumes, из проверенных зашифрованных комплектов оператора.
Для приложения скопируй [init.sh](../postgres/init.sh) и [nginx.conf](frontend.nginx.conf) в каталог Compose.
Заполни [параметры установки](product.env.example) идентификаторами OIDC-клиентов и первыми администраторами из журнала GitLab.

При восстановлении Jenkins и SonarQube создай пять внешних volumes по именам из Compose,
заполни их из проверенного снимка с сохранением UID/GID, восстанови исходные права файлов секретов.
На восстановленном Jenkins положи [скрипт агента](jenkins-agent.groovy) в `init.groovy.d` тома Jenkins,
запусти контроллер и возьми `/jnlpJars/agent.jar` с его loopback-адреса для каталога
`/etc/craftpulse-ci90-agent`. Секрет агента создаётся контроллером.
Рабочие каталоги агента в `/var/lib/craftpulse-ci90-agent` выделены отдельно от Jenkins home.

Перед запуском выполни `docker compose config --quiet`. Для чистой базы приложения сначала
подними PostgreSQL и SeaweedFS, выполни `docker compose run --rm migrate` и запусти frontend,
API и worker. Добавь [маршруты](Caddyfile) к конфигурации Caddy, проверь их через
`caddy validate` и выполни reload. После настройки HTTPS выполни `docker compose run --rm install` и проверь
`/readyz` и `/version`. Для приёмки входа, выхода и изоляции через API выполни
[AC-01](ac01.md) до первого входа администраторов.
Порты 9191 и 9192 предназначены для локального получения метрик.

Версии GitLab и SonarQube восстанавливай по версии и редакции, указанным в архиве.
Перед обновлением сними отдельную копию данных и проверь её в изолированной среде.
Пути снимков, сведения о проверках и зашифрованные комплекты хранит оператор стенда.
