// Generated from openapi.yaml. Run npm run generate:api.
export interface paths {
    "/o/{org_slug}/login": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Начать вход через провайдера организации
         * @description Создаёт запись предвхода (`state`, `nonce`, PKCE) на 10 минут и перенаправляет браузер к
         *     OIDC-провайдеру организации (раздел 13.1). `return_to` принимается только как
         *     относительный путь сайта. Несуществующая организация даёт ту же страницу ошибки входа,
         *     что и недоступный провайдер, без сведений об организации.
         */
        get: operations["startLogin"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/o/{org_slug}/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Завершить вход по ответу провайдера
         * @description Проверяет `state`, `iss`, обменивает code с PKCE, проверяет ID-токен и создаёт сессию
         *     (раздел 13.1, шаги 4-8). Первый вход учётной записи, заданной при установке,
         *     назначает роль администратора (FR-064). Любая ошибка не создаёт сессию и ведёт на
         *     страницу входа UI-03 с классом причины в параметре `login_error`.
         */
        get: operations["completeLogin"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/session/logout": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Выйти из текущей сессии
         * @description Ставит отметку завершения текущей сессии и удаляет cookie. Остальные сессии пользователя продолжают работать (раздел 7.2). Повтор с уже завершённой сессией даёт `401`.
         */
        post: operations["logout"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/me": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Получить текущего пользователя, организацию, роли и доступные действия
         * @description Данные для оболочки UI-01. Роли и область читаются при каждом запросе, поэтому их смена
         *     видна без повторного входа. Пользователь без области получает `areas` с пустыми
         *     списками и показывает экран «Нет доступной области».
         */
        get: operations["getCurrentUser"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/me/review-participation": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Включить или выключить участие в рекомендациях ревью
         * @description Личная настройка UI-69. Отказ исключает пользователя из групп рекомендованных ревьюеров (FR-050 A2). Повтор с тем же значением ничего не меняет.
         */
        put: operations["setReviewParticipation"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/me/source-links": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список подключений, которым нужен доступ пользователя
         * @description Подключения с режимом проверки прав токеном пользователя, например Confluence Cloud (раздел 13.5), и состояние связи пользователя с каждым.
         */
        get: operations["listSourceLinks"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/me/source-links/{connection_id}/authorization": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Начать разрешение доступа к источнику через OAuth 2.0
         * @description Возвращает адрес авторизации источника с одноразовым `state` (раздел 13.6). Адрес строит адаптер подключения (`AuthorizeUrl`): идентификатор клиента OAuth и адрес авторизации знает только он. Браузер переходит по адресу, источник возвращает код на `completeSourceLink`. `return_to` принимается только как относительный путь сайта; без него возврат идёт на UI-73.
         */
        post: operations["startSourceLink"];
        /**
         * Отозвать доступ craftPulse к источнику от имени пользователя
         * @description Адаптер удаляет токены пользователя (`DeleteCredentials`); документы этого источника закрываются для пользователя. Повтор даёт тот же результат.
         */
        delete: operations["deleteSourceLink"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/source-links/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Принять код авторизации источника
         * @description Проверяет одноразовый `state`, передаёт код адаптеру через `PutCredentials` и возвращает браузер на сохранённый `return_to` или на экран доступа к источникам UI-73. Токены в браузер не попадают.
         */
        get: operations["completeSourceLink"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/organization/structure": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Получить структуру организации
         * @description Команды, проекты, сервисы, компоненты с предложениями из manifests, участники команд и области источников проектов (UC-02). `ETag` - версия структуры.
         */
        get: operations["getOrganizationStructure"];
        /**
         * Подтвердить новую структуру организации
         * @description Полная замена структуры, которую администратор просмотрел в UI-17. Предложенный
         *     компонент действует только после включения в тело со статусом `confirmed`. На M0
         *     интервалы принимают текущий момент; даты действия - M1. Перекрытие интервалов одной
         *     пары - `422` (ограничение `EXCLUDE`, раздел 12.5.1). Операция идемпотентна: повтор того
         *     же тела с новым `If-Match` ничего не меняет.
         */
        put: operations["replaceOrganizationStructure"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список доступных команд
         * @description Команды в области пользователя для выбора в UI-01. Выбор команды не расширяет права.
         */
        get: operations["listTeams"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список доступных проектов
         * @description Проекты в области пользователя, доступные хотя бы в одном источнике проекта (раздел 7.7).
         *     Проект без областей источников виден по роли. Права источника не старше 60 с; скрытые
         *     проекты не дают строк и количества, ответ помечается `access_limited` с причинами в `access_notes`.
         */
        get: operations["listProjects"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/services": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список доступных сервисов
         * @description Сервисы в области пользователя.
         */
        get: operations["listServices"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/areas": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список областей источников
         * @description Проекты трекеров, репозитории и пространства wiki, известные по подключениям. Используется в UI-07 для привязки к проектам и в UI-64 для корней документации.
         */
        get: operations["listAreas"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/users": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список пользователей организации
         * @description Пользователи craftPulse с ролями для UI-08.
         */
        get: operations["listUsers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/users/{user_id}/roles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Роли пользователя
         * @description Действующие назначения ролей. `ETag` - версия набора ролей.
         */
        get: operations["listUserRoles"];
        /**
         * Заменить роли пользователя
         * @description Полный набор ролей пользователя. Изменение действует со следующего запроса пользователя без повторного входа (SEC-14). Роль не расширяет права в источниках.
         */
        put: operations["replaceUserRoles"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/users/{user_id}/block": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Заблокировать пользователя
         * @description В одной транзакции завершает все сессии пользователя и отключает его ключи API (раздел 13.2). Повтор для заблокированного пользователя даёт тот же результат.
         */
        post: operations["blockUser"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/users/{user_id}/unblock": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Разблокировать пользователя
         * @description Снимает блокировку. Завершённые сессии и отключённые ключи не восстанавливаются.
         */
        post: operations["unblockUser"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/users/{user_id}/sessions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Завершить все сессии пользователя
         * @description Следующий запрос пользователя получает `401`. Повтор даёт тот же результат (SEC-08).
         */
        delete: operations["endUserSessions"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/persons": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список людей и ботов
         * @description Люди, боты и служебные участники, к которым привязываются учётные записи источников (UC-08).
         */
        get: operations["listPersons"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/source-accounts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Учётные записи источников и кандидаты сопоставления
         * @description Учётные записи с действующей привязкой и предложения по логину или почте. Совпадение отображаемого имени предложения не создаёт (FR-018).
         */
        get: operations["listSourceAccounts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/account-assignments": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить привязки учётных записей и вид участника
         * @description Пакет решений UI-15. Каждый элемент обрабатывается отдельно, результат у каждого свой
         *     (best effort). Изменение признака бота создаёт новую версию привязки и пересчёт
         *     (FR-018). Для элемента с устаревшей версией - исход `version_conflict`.
         */
        post: operations["createAccountAssignments"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api-keys": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Ключи API организации
         * @description Ключи с владельцем, областью и сроком. Значение ключа не возвращается.
         */
        get: operations["listApiKeys"];
        put?: never;
        /**
         * Выпустить ключ API
         * @description Значение показывается один раз в ответе; в базе хранится его SHA-256. Срок не больше
         *     365 дней. Повтор запроса выпускает второй ключ, поэтому клиент не повторяет запрос
         *     вслепую после таймаута, а проверяет список ключей.
         */
        post: operations["createApiKey"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api-keys/{api_key_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Отозвать ключ API
         * @description Ключ отклоняется со следующего запроса. Повтор даёт `204`.
         */
        delete: operations["revokeApiKey"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/upload-tokens": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Токены загрузки CI
         * @description Токены проекта с номером слота; значение не возвращается.
         */
        get: operations["listUploadTokens"];
        put?: never;
        /**
         * Выпустить токен загрузки CI
         * @description Значение показывается один раз. У проекта не больше двух действующих токенов: новый
         *     занимает свободный слот, при двух действующих - `409 state_conflict`, сначала отзовите
         *     прежний (раздел 13.6).
         */
        post: operations["createUploadToken"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/upload-tokens/{upload_token_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Отозвать токен загрузки CI
         * @description Токен отклоняется со следующего запроса. Повтор даёт `204`.
         */
        delete: operations["revokeUploadToken"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/installation/protocols/latest": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Последний протокол проверки установки
         * @description Исход каждой проверки установки с версией поставки (FR-011). Сам запуск выполняется вне сайта (UI-05).
         */
        get: operations["getLatestInstallationProtocol"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/audit-events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Журнал аудита организации
         * @description Записи раздела 13.8 без секретов и содержимого закрытых объектов (DR-018). На M0 журнал читается только через API, отдельного экрана нет.
         */
        get: operations["listAuditEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/adapters": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Список поддержанных адаптеров
         * @description Встроенные адаптеры установки с поддержанными редакциями и версиями контракта для формы подключения UI-10. Адаптер клиента подключается по адресу и ключу.
         */
        get: operations["listAdapters"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Подключения и сводное состояние сбора
         * @description Подключения организации для UI-09 с состоянием и временем последнего успешного сбора.
         */
        get: operations["listConnections"];
        put?: never;
        /**
         * Создать подключение в состоянии черновика
         * @description Сохраняет подключение и передаёт учётные данные адаптеру (`PutCredentials`); ядро их не
         *     хранит и не возвращает (раздел 13.6). Адрес проверяется по сетевой области установки
         *     при каждом соединении (раздел 13.7); адрес вне области - `422` с причиной
         *     `outside_scope`. Подключение становится рабочим только после пробного чтения и
         *     подтверждения возможностей (`activateConnection`). Область источника, которую уже
         *     собирает другое активное подключение того же экземпляра, - `409 state_conflict`.
         */
        post: operations["createConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections/{connection_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Получить подключение
         * @description Подключение с областями и проверенными возможностями. Учётные данные представлены только датой замены.
         */
        get: operations["getConnection"];
        put?: never;
        post?: never;
        /**
         * Удалить подключение
         * @description Сразу закрывает чтение и выдачу областей подключения, в том числе из сохранённых срезов;
         *     учётные данные удаляются в адаптере. Факты областей удаляются по классу хранения
         *     `DISCONNECTED` (раздел 12.9). Повтор даёт `204`.
         */
        delete: operations["deleteConnection"];
        options?: never;
        head?: never;
        /**
         * Изменить подключение
         * @description JSON Merge Patch: имя, учётные данные, области, пауза сбора (UI-11). Новые учётные
         *     данные передаются адаптеру; прежнее значение удаляется после успешного пробного чтения
         *     (раздел 13.6). Смена адреса или областей возвращает подключение в черновик до нового
         *     пробного чтения.
         */
        patch: operations["updateConnection"];
        trace?: never;
    };
    "/connections/{connection_id}/probes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Запустить пробное чтение
         * @description Пробное чтение выполняет адаптер (`Probe`) асинхронно: в ответ - ресурс-операция. Запись
         *     в источник при пробе не выполняется: права записи проверяются по разрешениям
         *     учётной записи (FR-014). Повтор во время идущей пробы возвращает её же.
         *
         *     Проба и задание создаются в одной транзакции; результат хранится в connect.probe
         *     и доступен после перезапуска. configuration_version фиксирует row_version
         *     подключения на момент запуска. Повтор возвращает идущую пробу только для той же
         *     версии; при идущей пробе другой версии - 409 state_conflict.
         */
        post: operations["startConnectionProbe"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections/{connection_id}/probes/{probe_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Результат пробного чтения
         * @description Статус пробы и проверенные возможности с причинами недоступных. Пока проба идёт,
         *     `retry_after_seconds` подсказывает, когда спросить снова. Неверная редакция
         *     (неподдержанная редакция источника) - исход `failed` с `edition_mismatch` (V-12).
         *
         *     Проба другого подключения или удалённого подключения и истёкшая проба дают одинаковый
         *     404. Изменение настроек или учётных данных во время пробы делает её результат
         *     непригодным для активации: failed с reason_code configuration_changed.
         *     Неактивированные завершённые пробы хранятся 24 ч; проба последней активации
         *     сохраняется, пока на неё ссылается подключение.
         */
        get: operations["getConnectionProbe"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections/{connection_id}/activation": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить возможности и запустить загрузку истории
         * @description Администратор подтверждает результат успешной пробы (UI-17). Активируется подключение
         *     в состоянии draft или access_lost: после потери доступа нужны замена учётных данных
         *     и новая проба, прежняя история и курсоры сохраняются. Проба должна принадлежать
         *     этому подключению, быть последней, иметь status succeeded и configuration_version,
         *     равную текущей row_version; иначе 409 state_conflict. If-Match должен совпадать
         *     с текущим ETag, иначе 412. Подтверждённые возможности копируются из connect.probe
         *     в connect.capability только в транзакции активации. В той же транзакции подключение
         *     становится active, row_version увеличивается, сохраняются activated_probe_id и
         *     activated_row_version и ставится задание загрузки истории (UC-05).
         *     Повтор той же активации после таймаута определяется по activated_probe_id и
         *     activated_row_version: если подключение active и конфигурация с активации не менялась, возвращаются
         *     200 и текущий ETag до сравнения прежнего If-Match. Отсутствующий If-Match всегда
         *     даёт 428. После изменения настроек это уже не повтор прежней активации;
         *     нужно перечитать подключение и выполнить новую пробу. Проверки прав выполняются
         *     до обработки повтора; повтор не ставит второй сбор.
         */
        post: operations["activateConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections/{connection_id}/sync-status": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Состояние сбора подключения
         * @description По каждому виду данных - последняя попытка, последний успех, отставание, точка
         *     продолжения, страница с ошибкой, границы истории, неизвестные поля и затронутые
         *     показатели (FR-020). Ошибка, последний успешный снимок и полнота не объединяются в один
         *     статус.
         */
        get: operations["getSyncStatus"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/connections/{connection_id}/sync-runs": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Запросить повтор сбора или сверку
         * @description Ставит сбор с сохранённой точки продолжения или сверку с манифестом источника
         *     (UI-11). Одна активная попытка на подключение: повтор во время идущей попытки ничего не
         *     добавляет (NFR-006). Подключение без доступа (`access_lost`) - `409 state_conflict`
         *     до замены учётных данных.
         *
         *     Ответ содержит дескриптор запуска; GET по Location возвращает queued/running/succeeded/failed,
         *     задержку опроса и безопасную ошибку. Ссылка result_uri появляется после успеха.
         *     Повтор с теми же параметрами во время активного запуска возвращает тот же id.
         *     Иные параметры для этого ресурса во время запуска дают 409 state_conflict;
         *     после завершения новый запрос создаёт новый запуск.
         */
        post: operations["requestSyncRun"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/webhooks/{route_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Принять webhook источника
         * @description Адрес содержит случайный ID маршрута подключения; полный адрес для настройки в
         *     источнике отдаёт `Connection.webhook_url`. `api` проверяет только маршрут и
         *     размер тела и сразу отвечает `200`; подпись проверяет адаптер в задании
         *     `sync.webhook`, а webhook служит только сигналом к перечитыванию (раздел 10.6).
         *     Неверная подпись не меняет ответ отправителю, но учитывается в диагностике.
         */
        post: operations["receiveWebhook"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ci/check-profile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Профиль проверок проекта для шага CI
         * @description Читает `craftpulse-ci` по токену загрузки: действующая матрица проверок проекта, версии
         *     образов профилей, ожидаемые отчёты и пределы пакета (UC-25). Токен даёт доступ только
         *     к своему проекту.
         */
        get: operations["getCiCheckProfile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ingest/packages": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Загрузить пакет отчётов из CI
         * @description Тело - архив tar.gz, первым файлом идёт манифест (раздел 10.5.2). Пакет не больше
         *     25 MiB, после распаковки не больше 100 MiB и 1 000 файлов; пути, ссылки и envelope
         *     проверяются по ходу потока (NFR-015). Невалидный пакет получает отказ сразу.
         *     Валидный - `202` и ID приёма; итог разбора - в квитанции. Повтор того же пакета
         *     распознаётся по хэшу: ответ `202` со статусом `duplicate` и ссылкой на первый приём,
         *     вклад не удваивается (FR-004). Записи вне репозиториев проекта токена отклоняются
         *     (раздел 12.6).
         */
        post: operations["uploadPackage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/imports": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Передать файл универсального импорта
         * @description Форма UI-12: файл и поля манифеста. Факты задач, событий, связей, MR, запусков CI и
         *     документов передаются JSON или CSV; отчёты находок, тестов и покрытия - файлом
         *     поддержанного формата с `report_format` и `commit_sha`. `api` упаковывает файл в такой же
         *     пакет, как из CI, и дальше он проходит общий приём (раздел 10.5.2). Неподдержанная
         *     схема отклоняет пакет целиком, прежний результат сохраняется (FR-016). Записи
         *     принимаются только для областей, где у пользователя есть право импорта.
         */
        post: operations["createImport"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/receipts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Журнал приёма пакетов
         * @description Приёмы сбора, CI и импорта для диагностики UI-11 и UI-13, новые первыми.
         */
        get: operations["listReceipts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/receipts/{receipt_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Квитанция приёма
         * @description Итог разбора пакета (UI-13, вывод шага CI UI-51): принятое, отклонённое, повторы и
         *     конфликты по количеству, причина отказа пакета и затронутые показатели. Токен загрузки
         *     читает только квитанции своих пакетов. Пока пакет разбирается, статус `quarantined`
         *     и `retry_after_seconds`.
         */
        get: operations["getReceipt"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/receipts/{receipt_id}/records": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Записи пакета и их исходы
         * @description Записи пакета с исходом и причиной отклонения, без содержимого записей (DR-003).
         */
        get: operations["listReceiptRecords"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ingest-conflicts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Конфликты приёма
         * @description Записи с тем же `source_revision` и другим содержимым (BR-002). Принятый факт не изменён до решения.
         */
        get: operations["listIngestConflicts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ingest-conflicts/{conflict_id}/resolution": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Решить конфликт приёма
         * @description `kept_existing` оставляет принятый факт; `accepted_new` принимает новое содержимое как
         *     исправление с новым номером знания и пересчётом (FR-005); `source_corrected` отмечает,
         *     что источник исправлен и будет перечитан. Решённый конфликт повторно не решается -
         *     `409 state_conflict`.
         */
        post: operations["resolveIngestConflict"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/workflow-catalog": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Справочники источников для сопоставления команды
         * @description Статусы, исходы, типы задач и поля проектов команды по ID конкретного экземпляра, включая значения без правил (FR-017). Категория статуса в трекере показана, но смысла не задаёт.
         */
        get: operations["getWorkflowCatalog"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/mapping-previews": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Предпросмотр сопоставления на задачах
         * @description Применяет черновик правил к задачам проектов команды, которые пользователь видит сейчас
         *     с учётом прав в источниках, и показывает до 20 из них: сначала по одной задаче каждого
         *     вида (в работе, не начата, завершена, отменена или дубликат, с блокировкой, без размера,
         *     с неизвестным статусом), затем недавно изменённые. Задачи закрытых для пользователя
         *     проектов и задачи с собственным ограничением без доступа в выборку и расчёт не попадают.
         *
         *     Контрольные показатели считаются тем же определением, что в отчёте, по всем видимым
         *     задачам команды на одну дату `as_of`: T05 (завершено) и T08 с оценкой p50 (медиана
         *     цикла) за неделю отчёта `period` от понедельника 00:00 в часовом поясе черновика до
         *     `as_of`, T12 (в работе) на `as_of` (FR-017).
         *
         *     Ничего не сохраняет: версии, ревизии расчёта и записи аудита не создаются, повтор
         *     безопасен. Черновик с правилом для неизвестного ID экземпляра или недопустимым `meaning`
         *     - `422` с ошибками полей.
         */
        post: operations["previewMapping"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/mapping-versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии сопоставления команды
         * @description Версии новыми первыми; `ETag` ответа - номер последней версии для `createMappingVersion`.
         */
        get: operations["listMappingVersions"];
        put?: never;
        /**
         * Подтвердить новую версию сопоставления
         * @description Создаёт версию `If-Match + 1` после просмотра в UI-17 (FR-017). Форма на основе
         *     заменённой версии - `412 version_conflict` (FR-008). Повтор с тем же
         *     `Idempotency-Key` возвращает сохранённую версию; содержимое, равное действующей версии,
         *     новую версию не создаёт и возвращает действующую с `200` (FR-007). Прежние ревизии
         *     расчёта остаются на прежней версии, новая версия запускает пересчёт.
         */
        post: operations["createMappingVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/calendar-versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии рабочего календаря команды
         * @description Версии новыми первыми; `ETag` - номер последней.
         */
        get: operations["listCalendarVersions"];
        put?: never;
        /**
         * Подтвердить новую версию календаря
         * @description Правила версии как у `createMappingVersion`. Часовой пояс - имя IANA; неизвестное имя - `422`.
         *     Если содержимое равно действующей версии, возвращается 200 без создания версии.
         *     Повтор Idempotency-Key возвращает исходный статус и ответ до проверки If-Match.
         */
        post: operations["createCalendarVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/target-baselines": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Исторический p85 как ориентир целевого срока
         * @description p85 длительности цикла T08 по типу и размеру за выбранный период с n и полнотой (FR-019).
         *     Это ориентир, а не срок: при n < 20 `low_sample = true`, и надёжного автоматического срока
         *     нет. Задачи без размера идут отдельной группой с `size_label = null` и
         *     `reason_code = size_unknown`; срок для неё не задаётся. Задачи без типа в группы не входят.
         *
         *     Значение строится по сохранённым вкладам T08 текущей ревизии недельного ряда команды
         *     `revision_id`: в период входят недели, целиком лежащие в `[from,to)`, задача считается по
         *     типу и размеру на своём первом начале. `missing_count` - задачи с неполной историей,
         *     `excluded_count` - завершения без начала и исходы без результата. Пока у команды нет
         *     опубликованной ревизии ряда - `404`.
         */
        get: operations["getTargetBaselines"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/target-versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии целевых сроков команды
         * @description Действующие и прежние версии по группам «тип, размер». Для редактирования одной группы передайте одновременно task_type и size_label. Только такой запрос возвращает ETag последней версии этой группы, в том числе "0" при отсутствии версий; для общего списка ETag отсутствует. Курсор не меняет версию группы.
         */
        get: operations["listTargetVersions"];
        put?: never;
        /**
         * Подтвердить целевой срок группы
         * @description Новая версия срока для группы «тип, размер» (FR-019). Срок больше нуля, размер обязателен:
         *     без размера срок не применяется. Номер версии считается по группе; форма на основе
         *     заменённой версии - `412`. Прежние нарушения сохраняют свою версию срока (V-16).
         *
         *     If-Match берётся из listTargetVersions с теми же task_type и size_label;
         *     для группы без версий передаётся "0". Идемпотентный повтор проверяется до If-Match:
         *     возвращается сохранённый результат, даже если текущая версия уже изменилась.
         *
         *     Если содержимое равно действующей версии, возвращается 200 без создания версии.
         *     Повтор Idempotency-Key возвращает исходный статус и ответ до проверки If-Match.
         *
         *     Для `historical_p85` передаются ревизия ориентира `basis_revision_id` и его период
         *     `basis_period`, как их показал `getTargetBaselines`: значение основания, n и окно
         *     (период в пределах окна ревизии) считываются из этой ревизии и сохраняются в версии,
         *     даже когда ревизия потом удаляется по сроку хранения. Ревизия другой команды или группа
         *     без значения p85 за период - `422`. Для `manual` оба поля - `null`.
         *
         *     Новая версия ставит пересчёт рядов команды в той же транзакции; прежние ревизии
         *     сохраняют версию срока, с которой они посчитаны.
         */
        post: operations["createTargetVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/report": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Ежедневный отчёт команды
         * @description Графики потока работы T04/T05/T08/T10-T15/T18/T23 и очередь MR по актуальной или
         *     указанной ревизии (FR-023). Каждая точка - значение с состоянием данных; раскрытие -
         *     через `revision_id` и `listRevisionCohort`. Без подтверждённого сопоставления отчёт
         *     отдаёт `NOT_EVALUATED` с причиной `mapping_missing`. Фильтры проекта, типа и размера
         *     выбирают ряд с той же областью и теми же определениями; знаменатели и когорты точек
         *     соответствуют фильтру (AC-18). Когорта ограничена правами:
         *     значения строятся по разрешённой части, `access_limited` без числа скрытых (FR-001).
         *
         *     A05, A06, A07, A10, история событий A22 и показатели X01/X02 входят в ту же ревизию.
         *     Шаг `day` даёт дневные точки. Для A22 группа содержит вид события и участника.
         *     В X01/X02 `n` содержит размер доступного WIP, `denominator` - размер оценённой части,
         *     `excluded_count` учитывает пробелы связи и неприменимые задачи, `missing_count` - неизвестные.
         *     Задача с совпавшим признаком учитывается в числителе и знаменателе, в когорте имеет роль `numerator`.
         *
         *     UC-15 использует этот отчёт и listRevisionCohort: T23 - переоткрытия в M0; T24 и T25
         *     выдаются отдельно с M1, только при заданном этапе приёмки и подтверждённом production origin.
         *     Для T23 видны зрелая когорта, окно наблюдения и незрелые задачи; виды возвратов не суммируются.
         */
        get: operations["getTeamReport"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/metric-definitions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Определения показателей
         * @description Реестр определений с версиями по каталогу метрик (DR-011).
         */
        get: operations["listMetricDefinitions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/series": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Ряды показателей в доступной области
         * @description Ряды для машинного чтения BI (UI-36) и сохранённых срезов. Ряд, область которого субъекту закрыта целиком, не показывается.
         */
        get: operations["listSeries"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/series/{series_id}/data": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Значения ряда
         * @description Точки ряда по актуальной или указанной ревизии с определениями и состояниями данных; те же значения, что в UI и выгрузке при одинаковых правах (FR-009).
         */
        get: operations["getSeriesData"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/series/{series_id}/refreshes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Пересчитать ряд вручную
         * @description Ставит расчёт новой ревизии на текущих фактах (FR-023). Если расчёт ряда уже идёт, новый не ставится; ответ тот же. Действующая ревизия остаётся актуальной до публикации новой.
         *     Ответ содержит дескриптор запуска; GET по Location возвращает queued/running/succeeded/failed,
         *     задержку опроса и безопасную ошибку. Ссылка result_uri появляется после успеха.
         *     Повтор с теми же параметрами во время активного запуска возвращает тот же id.
         *     после завершения новый запрос создаёт новый запуск.
         */
        post: operations["refreshSeries"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/revisions/{revision_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Паспорт ревизии расчёта
         * @description Паспорт показателя UI-23: определения и версии, версии сопоставления, календаря, сроков,
         *     решений и связей, watermark источников, as_of и срез знания (DR-011). Невоспроизводимая
         *     ревизия имеет `irreproducible_reason`.
         *      Для I18 когорта закрепляет класс runner, окно и версию ставки (input_kind=ci_rate в revision.inputs); attempt-объекты дают вклад в job_seconds. Для I39-I41 используется ревизия getDeliveryStability.
         */
        get: operations["getRevision"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/revisions/{revision_id}/cohort": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Когорта точки показателя
         * @description Объекты, вошедшие в точку ряда, с ролью в формуле и значением вклада (UI-24, FR-021):
         *     по ним независимо получаются числитель, знаменатель и исключения. Объекты вне прав не
         *     возвращаются и не считаются; ответ `access_limited` без их числа. Удалённые основания
         *     помечаются `basis_deleted` без содержимого (V-09).
         *
         *     Для T23-T25 роли вклада отделяют числитель, знаменатель и незрелое наблюдение;
         *     клиент передаёт metric_id выбранного вида возврата (UC-15), не объединяя их.
         *      Для I18 когорта закрепляет класс runner, окно и версию ставки (input_kind=ci_rate в revision.inputs); attempt-объекты дают вклад в job_seconds. Для I39-I41 используется ревизия getDeliveryStability.
         */
        get: operations["listRevisionCohort"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/saved-views": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Сохранённые срезы пользователя
         * @description Срезы, сохранённые текущим пользователем (UI-37), от новых к старым по created_at и id. Курсор продолжает этот порядок; данные отчёта в список не входят.
         */
        get: operations["listSavedViews"];
        put?: never;
        /**
         * Сохранить срез
         * @description Срез с закреплённой ревизией (`pinned_revision_id`) удерживает её от удаления по сроку (раздел 12.7). Без закрепления срез показывает актуальную ревизию ряда.
         */
        post: operations["createSavedView"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/saved-views/{saved_view_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Открыть сохранённый срез
         * @description Срез строится по правам открывающего, а не автора (раздел 7.4). Ссылку на срез может открыть любой пользователь организации; поле report содержит отчёт по его правам, включая access_limited. Закреплённый срез использует прежнюю ревизию, обновляемый - текущую ревизию сохранённого ряда. Недоступный ряд или срез другой организации даёт 404.
         */
        get: operations["getSavedView"];
        put?: never;
        post?: never;
        /**
         * Удалить сохранённый срез
         * @description Удаляет срез автора и снимает закрепление ревизии. Повтор даёт `204`.
         */
        delete: operations["deleteSavedView"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exports": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Запросить выгрузку
         * @description Выгрузка разрешённой когорты ревизии с определениями, версиями, единицами и полнотой
         *     (FR-022). Формирование асинхронно: ответ - ресурс-операция. Формат `json` - один
         *     документ, `csv` - архив zip с `data.csv` и `metadata.json`. Повтор создаёт новое
         *     задание; оба будут с одинаковым содержимым при тех же правах и ревизии.
         */
        post: operations["createExport"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exports/{export_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Состояние выгрузки
         * @description Задание видно только запросившему субъекту; для другого - `404`.
         */
        get: operations["getExport"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exports/{export_id}/file": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Скачать файл выгрузки
         * @description Скачивание проверяет сессию или ключ того же субъекта и права заново (раздел 13.4). После
         *     отзыва права - `404`, файл не выдаётся. Уже скачанный файл craftPulse не отзывает.
         *     Неготовая выгрузка - `409 state_conflict`.
         */
        get: operations["downloadExport"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/overview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Обзор проекта
         * @description Управленческий контекст проекта UI-22 - текущий WIP, задачи старше срока, очередь MR и состояние источников по разрешённой области.
         */
        get: operations["getProjectOverview"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/timeline": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * События проекта на дату
         * @description События задач, MR, CI и документов проекта за выбранный интервал с разделением людей,
         *     ботов и систем (FR-030). У события есть время события и время получения; позднее и
         *     загруженное с историей событие отмечено. Срез знания задаётся `revision_id`: события,
         *     полученные после среза знания этой ревизии, не выдаются; без параметра - текущее
         *     состояние. Событие закрытого объекта не выдаётся.
         */
        get: operations["listProjectTimeline"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tasks": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Задачи в доступной области
         * @description Общий список задач проекта или команды UI-22 со смыслом статуса по действующему сопоставлению.
         */
        get: operations["listTasks"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tasks/{task_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Карточка задачи с наблюдением о задержке
         * @description Возраст и целевой срок, интервалы блокировки, подтверждённые MR, ожидание ревьюера,
         *     состояние CI и кандидаты связей (FR-029). Наблюдение строится только по основаниям;
         *     без фактов причина не придумывается (`observation.kind = unknown`). Задача с уровнем
         *     безопасности проверяется в источнике отдельно (раздел 13.5).
         */
        get: operations["getTask"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tasks/{task_id}/events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * История задачи
         * @description События задачи по времени события, с временем получения и видом участника (DR-002). Пользовательские поля показывают учётные записи, а не исходные строки имён.
         */
        get: operations["listTaskEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/links/{link_id}/decisions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить или отклонить связь-кандидат
         * @description Подтверждение создаёт связь с методом `user_confirmed`; отклонение убирает кандидата из
         *     оснований (BR-010). Нужен доступ к обоим концам связи (раздел 7.7). Повтор того же
         *     решения с тем же `Idempotency-Key` возвращает сохранённую связь и не создаёт второй
         *     (FR-007). Связь уже подтверждённая источником не отклоняется - `409 state_conflict`.
         *      Для UI-41 направление фиксировано в кандидате: caused_failure подтверждает deployment_incident, response_to_incident - incident_deployment; сервер возвращает Link того же типа и концов. Другой тип не подставляется при повторе.
         */
        post: operations["decideLink"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/merge-requests": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * MR в доступной области и очередь ревью
         * @description Фильтр `review_wait` даёт очередь UI-27: MR без ревьюера, ожидающие первого внешнего
         *     ответа человека и (M1) повторного ревью (FR-031). Возраст считается от готовности, при её
         *     отсутствии - от открытия с отметкой `fallback_open`. Неизвестный ревьюер не попадает в
         *     группу «нет ревьюера».
         */
        get: operations["listMergeRequests"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/merge-requests/{merge_request_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * MR
         * @description Текущее состояние MR, head и base, ожидание ревью и связанные задачи.
         */
        get: operations["getMergeRequest"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/merge-requests/{merge_request_id}/summary": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Сводка MR по точному head
         * @description Действующая ревизия сводки или ревизия с номером `revision_no` (FR-036, UI-53): новые,
         *     устранённые и впервые обнаруженные находки относительно сопоставимой базы, попытки CI,
         *     тесты, покрытие изменённых строк, непроверенные области и состояние публикации
         *     комментария. Сводка старого head помечена `stale`. Строки, требующие права на данные
         *     безопасности, без этого права удаляются из items; access_limited и access_notes
         *     дают только общую отметку без индексов и количества скрытых строк (SEC-16).
         *     item_no и rank назначаются после фильтрации по правам текущего субъекта. Пока ревизии
         *     нет - `404`.
         */
        get: operations["getMergeRequestSummary"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/pipelines/{pipeline_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Pipeline с оговорками
         * @description Исход pipeline отдельно от полноты проверок (FR-039, UI-55): повторы, разрешённые
         *     падения, ожидаемые и полученные отчёты, тесты и покрытие. Отсутствующий отчёт имеет своё
         *     состояние, а не ноль.
         */
        get: operations["getPipeline"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/pipelines/{pipeline_id}/attempts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Jobs и все известные попытки
         * @description Попытки с конфигурацией матрицы, первым и итоговым исходом и длительностью (FR-039, UI-56). Одноимённые jobs разных конфигураций различаются отпечатком. Неполная история даёт нижнюю границу расхода.
         */
        get: operations["listPipelineAttempts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/pipelines/{pipeline_id}/test-executions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Выполнения тестов pipeline
         * @description Исходы тестов из JUnit по конфигурациям (FR-062). Один тест в разных конфигурациях матрицы - разные выполнения, не попытки одного теста.
         */
        get: operations["listTestExecutions"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/check-profiles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Действующая матрица проверок проекта
         * @description Режим проверки по компоненту, языку и виду анализа с версиями инструментов и непокрытыми областями (FR-035, UI-49). `ETag` - номер версии матрицы.
         */
        get: operations["getCheckProfile"];
        put?: never;
        /**
         * Подтвердить новую матрицу проверок
         * @description Новая версия матрицы (FR-035). Законченный внешний отчёт заменяет только совпадающую
         *     область; внешний анализатор не выключает проверку секретов и зависимостей (FR-035 A1).
         *     Неподдержанный язык или вид анализа - `422`. Правила версии как у `createMappingVersion`.
         *
         *     Если содержимое равно действующей версии, возвращается 200 без создания версии.
         *     Повтор Idempotency-Key возвращает исходный статус и ответ до проверки If-Match.
         */
        post: operations["createCheckProfileVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/ci-configuration": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Фрагмент конфигурации CI
         * @description Фрагмент pipeline с образами профилей по digest и имя маскированной переменной токена (UI-50). Значение токена во фрагмент не входит; изменение CI клиент вносит обычным MR.
         */
        get: operations["getCiConfiguration"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/baselines": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Базовые линии проекта
         * @description Состояние базовой линии по компоненту и виду анализа (UI-52). Ожидание или оборванный прогон не дают готовой базы (FR-035 A3).
         */
        get: operations["listBaselines"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/components/{component_id}/overview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Технический обзор компонента
         * @description Факты для выбора работы по техдолгу UI-44 (FR-042): находки по видам и состояниям,
         *     возраст, исключения, сложность и копии с источником и окном истории. На M1 добавляются
         *     горячие точки и концентрация авторства; история изменений файлов - косвенный признак,
         *     а не владение. Компоненты с разными источниками не усредняются.
         */
        get: operations["getComponentOverview"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/findings": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Находки кода, безопасности и документации
         * @description Канонические находки в доступной области UI-45 и нарушения документации UI-65.
         *     Находки безопасности выдаются только при праве на данные безопасности; без него они не
         *     возвращаются и не считаются, ответ `access_limited` (SEC-15).
         */
        get: operations["listFindings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/findings/{finding_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Карточка находки
         * @description Правило, место, исходные и сопоставленная серьёзность, уверенность (неизвестная остаётся
         *     `null`), очищенное основание, отчёт и версии инструментов, решения craftPulse и внешнего
         *     триажа, доступные действия (FR-037, UI-46). Значение потенциального секрета удалено до
         *     сохранения и не возвращается никогда (BR-017). Находка безопасности без права - `404`.
         */
        get: operations["getFinding"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/technical-comparisons": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Причины изменения технического результата
         * @description Сравнение двух сопоставимых результатов компонента (FR-040, UI-57) с разложением по
         *     факторам: код, зависимости, правила, база уязвимостей, область, источник, неопределённая
         *     идентичность. Несопоставимые результаты дают `comparable = false` с причинами и без
         *     числовой разницы (BR-009).
         */
        get: operations["compareTechnicalResults"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exceptions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Исключения и ожидающие подтверждения решения
         * @description История и текущее состояние решений по находкам UI-48 (FR-038, FR-061). Истёкшее решение остаётся в истории.
         */
        get: operations["listExceptions"];
        put?: never;
        /**
         * Оформить исключение для находки
         * @description Решение с причиной, областью и сроком пересмотра (FR-038). Область не шире области
         *     автора и прав на находку; чужая область, пустая причина или срок не позже начала -
         *     `422`. Если политика требует подтверждения, решение ждёт его со статусом `pending` и не
         *     действует (FR-061). Принятый риск находку не закрывает.
         *
         *     Сначала проверяется видимость находки и исходной области: закрытая или чужая находка
         *     даёт 404 без сведений о ней. 422 outside_scope применяется только к расширению
         *     области исключения для уже доступной находки.
         */
        post: operations["createException"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exceptions/{exception_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Решение с версиями и переходами
         * @description Все версии и переходы статуса. `ETag` - номер последней версии.
         */
        get: operations["getException"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exceptions/{exception_id}/versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Продлить или изменить решение новой версией
         * @description Продление и изменение - новая версия, прежняя не переписывается (BR-012). Пока новая
         *     версия ждёт подтверждения, действует прежняя до своего срока (V-27). Форма на основе
         *     заменённой версии - `412`.
         */
        post: operations["createExceptionVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/exceptions/{exception_id}/decisions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить или отклонить ожидающую версию решения
         * @description Ревьюер подтверждает или отклоняет ожидающую версию (UC-28 A3, UI-48).
         *     Решение относится к версии из `If-Match`. Переход из конечного статуса - `409
         *     state_conflict`. Автор не подтверждает своё решение, если политика требует ревьюера
         *     (`403`).
         */
        post: operations["decideException"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/services/{service_id}/documentation-roots": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Корни документации сервиса
         * @description Корни, владелец, аудитория и профиль проверки (FR-046, UI-64). `ETag` - версия набора корней.
         */
        get: operations["listDocumentationRoots"];
        /**
         * Подтвердить корни документации сервиса
         * @description Полный набор корней сервиса (FR-046). Владелец задаётся явно и не вычисляется по
         *     последнему редактору. Корень в области, к которой у владельца сервиса нет доступа, -
         *     `422 outside_scope`. Источник wiki подключается отдельно через подключения.
         */
        put: operations["replaceDocumentationRoots"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/services/{service_id}/documents": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Корпус документации сервиса
         * @description Документы текущей версии в корнях сервиса с результатом проверок профиля (UI-65). Документ
         *     с неизвестными правами не выдаётся и не индексируется (BR-019). Без корней - пустой список
         *     и `documentation_state = UNKNOWN`.
         */
        get: operations["listServiceDocuments"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/services/{service_id}/documentation-checks": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Запросить повторную проверку документации
         * @description Исправление нарушения подтверждается только повторной проверкой той же области (FR-047).
         *     Для Git-корней проверку выполняет шаг CI клиента: запрос её не запускает, результат
         *     приходит со следующим pipeline, и запуск завершается после его отчёта. Для источников,
         *     которые читает адаптер (wiki), запрос ставит перечитывание. Ответ - `202`; итог виден в
         *     находках.
         *
         *     Ответ содержит дескриптор запуска; GET по Location возвращает queued/running/succeeded/failed,
         *     задержку опроса и безопасную ошибку. Ссылка result_uri появляется после успеха.
         *     Повтор с теми же параметрами во время активного запуска возвращает тот же id.
         *     Иные параметры для этого ресурса во время запуска дают 409 state_conflict;
         *     после завершения новый запрос создаёт новый запуск.
         */
        post: operations["requestDocumentationCheck"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/organization/report": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Срез организации по командам
         * @description Сравнение команд без сложения разных шкал размера (FR-024, UI-21). Закрытая команда не раскрывается через общий итог.
         */
        get: operations["getOrganizationReport"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/mirror-candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Кандидаты зеркал задач двух трекеров
         * @description Пары задач двух трекеров проекта по явным связям и совпадению ключей (FR-033, UI-19). До подтверждения задачи раздельны.
         */
        get: operations["listMirrorCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/mirror-groups": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Подтверждённые группы зеркал
         * @description Группы проекта с источником истины статуса и видимыми конфликтами полей (FR-033).
         */
        get: operations["listMirrorGroups"];
        put?: never;
        /**
         * Подтвердить группу зеркал
         * @description После подтверждения задачи дают один вклад, поля берутся из выбранного источника истины (BR-016). В трекеры ничего не пишется. Задача уже в другой группе - `409 state_conflict`.
         */
        post: operations["createMirrorGroup"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/mirror-groups/{mirror_group_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        /**
         * Снять подтверждение группы зеркал
         * @description Задачи снова считаются раздельно в новых ревизиях; прежние ревизии сохраняют группу. Повтор даёт `204`.
         */
        delete: operations["deleteMirrorGroup"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/sprints": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Спринты проекта
         * @description Спринты досок проекта с фактическими датами (FR-032, UI-32).
         */
        get: operations["listSprints"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/sprints/{sprint_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * План спринта и результат
         * @description Неизменяемый исходный состав на фактический старт, изменения после старта и результат (FR-032). Без истории состава план `UNKNOWN`, текущий список его не заменяет.
         */
        get: operations["getSprint"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/dod-versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии Definition of Done команды
         * @description Обязательства DoD по типам задач (FR-063). Без task_type возвращается общий список без ETag. Для формы одного типа передайте task_type: ETag содержит последнюю версию именно этого типа, включая "0" при отсутствии версий. Курсор не меняет версию выбранного типа.
         */
        get: operations["listDodVersions"];
        put?: never;
        /**
         * Подтвердить версию DoD
         * @description Обязательство без способа подтверждения не объявляется машинно проверенным (FR-063). Правила версии как у `createMappingVersion`.
         *     Если содержимое равно действующей версии, возвращается 200 без создания версии.
         *     Повтор Idempotency-Key возвращает исходный статус и ответ до проверки If-Match.
         *
         *     If-Match берётся из listDodVersions с тем же task_type, что и в теле; версия
         *     одного типа не служит предусловием другого. Для нового типа передаётся "0".
         */
        post: operations["createDodVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tasks/{task_id}/dod": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Выполнение DoD задачи
         * @description Состояние и основание каждого обязательства для подтверждённой версии результата задачи (FR-026, UI-29). Кандидат связи и чужой SHA основанием не являются.
         */
        get: operations["getTaskDod"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/initiatives": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Инициативы
         * @description Инициативы с датой и командами (FR-027, UI-30).
         */
        get: operations["listInitiatives"];
        put?: never;
        /**
         * Сохранить инициативу
         * @description Дата инициативы и затронутые команды (FR-027). Команды вне области пользователя - `422 outside_scope`.
         */
        post: operations["createInitiative"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/initiatives/{initiative_id}/comparison": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Сравнение периодов до и после инициативы
         * @description Только сопоставимые показатели с составом, объёмом, областью и версиями обоих периодов (FR-027). Несопоставимый показатель - без числовой разницы с причиной (BR-009). Разница не утверждается эффектом.
         */
        get: operations["getInitiativeComparison"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tasks/{task_id}/delivery-chain": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Цепочка бизнес-обязательства до поставки
         * @description Исходный и изменённый состав эпика до последнего подтверждённого звена задачи, MR, артефакта и доставки (FR-028, UI-31). Недоказанная связь остаётся разрывом.
         */
        get: operations["getDeliveryChain"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/check-policies": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии политики обязательных проверок
         * @description Категории, поведение при неизвестном результате и правила обхода (FR-041, UI-58). `ETag` - номер последней версии.
         */
        get: operations["listCheckPolicyVersions"];
        put?: never;
        /**
         * Подтвердить версию политики проверок
         * @description Явное включение обязательных проверок (BR-018). Реальный запрет merge зависит от настройки Git-хостинга, которую клиент включает сам. Правила версии как у `createMappingVersion`.
         *     Если содержимое равно действующей версии, возвращается 200 без создания версии.
         *     Повтор Idempotency-Key возвращает исходный статус и ответ до проверки If-Match.
         */
        post: operations["createCheckPolicyVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/policy-bypasses": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Оформить разрешённый обход политики
         * @description Обход с причиной, областью и сроком от ответственного по политике (FR-041 A2). Находку не удаляет; вердикт меняется после перезапуска pipeline во внешнем CI.
         */
        post: operations["createPolicyBypass"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/policy-evaluations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Вердикты политики и история обходов
         * @description Вердикт версии политики отдельно от фактических исходов проверок; разрешённый обход, нарушение и отсутствие merge gate - разные исходы (FR-041, UI-60).
         */
        get: operations["listPolicyEvaluations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tech-debt-drafts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Создать черновик задачи техдолга
         * @description Черновик из находок с проектом, типом и текстом (FR-043, UI-61). Черновик ничего не пишет в трекер. У подключения нет проверенной возможности записи - черновик создаётся, подтверждение будет отклонено.
         */
        post: operations["createTechDebtDraft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tech-debt-drafts/{draft_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Черновик и исход записи в трекер
         * @description После подтверждения - исход внешней записи `pending`, `succeeded`, `failed` или `unknown` до сверки по маркеру (FR-043).
         */
        get: operations["getTechDebtDraft"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/tech-debt-drafts/{draft_id}/confirmation": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить запись задачи в трекер
         * @description Отдельное подтверждение пользователя с правом записи создаёт не больше одной задачи на
         *     действие (FR-043). `Idempotency-Key` - ID подтверждения: повтор возвращает то же
         *     действие и ту же задачу. Потерянный ответ трекера даёт `unknown`; перед любым созданием
         *     выполняется поиск по маркеру (раздел 10.5.4). Нет проверенной возможности
         *     `write_tracker_issue` - `409 capability_missing`.
         */
        post: operations["confirmTechDebtDraft"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/artifacts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Выпущенные артефакты
         * @description Артефакты по digest; фильтр `affected` даёт затронутые новой версией базы уязвимостей (FR-045, UI-63). Неизвестный состав показан отдельно и не считается незатронутым.
         */
        get: operations["listArtifacts"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/artifacts/{artifact_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Паспорт артефакта по digest
         * @description Исходная ревизия, сборка, развёртывания, проверки и исключения на момент решения о выпуске; поздние проверки отдельно (FR-044, UI-62). На M2 - доступность каждого развёртывания извне (FR-057).
         */
        get: operations["getArtifactPassport"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/documentation-obligations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Обязательства документации по версии контракта
         * @description Обязательства обновить reference, примеры и runbook при изменении API-контракта (FR-048, UI-67).
         */
        get: operations["listDocumentationObligations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/documentation-obligations/{obligation_id}/decisions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Подтвердить или отклонить кандидат изменения контракта
         * @description Возможное несовместимое изменение остаётся кандидатом до решения владельца сервиса (FR-048 A1). Закрытие обязательства - только обновлением и ревью документа текущей версии, не этим вызовом.
         */
        post: operations["decideDocumentationObligation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/handover": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Комплект передачи проекта
         * @description Сервисы, владельцы, текущие версии, открытые задачи, находки и проверки, документы с основаниями ревью и пробелами (FR-034, UI-33). Закрытый документ не раскрывает даже заголовок.
         */
        get: operations["getHandoverPackage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/handover/decisions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Принять комплект или запросить дополнение
         * @description Решение принимающей стороны с перечнем запрошенных дополнений (FR-034).
         */
        post: operations["decideHandover"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/merge-requests/{merge_request_id}/review-route": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Маршрут ревью MR
         * @description Места изменения с основаниями и допустимые группы ревьюеров (FR-050, UI-68). История изменений файлов - косвенный признак владения; участник, отказавшийся от рекомендаций, не попадает в группы. Без истории групп нет, места по находкам и тестам остаются.
         */
        get: operations["getReviewRoute"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/merge-requests/{merge_request_id}/review-route/feedback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Отметить рекомендацию маршрута
         * @description Отметка «принята» или «неверная группа» сохраняется для оценки метода (FR-050 A3).
         */
        post: operations["createReviewRouteFeedback"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/forecasts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Запросить прогноз срока или объёма
         * @description Расчёт распределения по истории версионным методом с проверкой на истории (FR-049,
         *     UI-38). Асинхронно: ответ - ресурс-операция. Метод, критерий достаточности и качество
         *     фиксируются до включения по Q-05; недостаточная история - исход `refused` с причиной.
         */
        post: operations["createForecast"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/forecasts/{forecast_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Результат прогноза
         * @description Распределение с уровнями вероятности, ошибкой проверки на истории и допущениями; срок не гарантируется (FR-049).
         */
        get: operations["getForecast"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/projects/{project_id}/test-selection-experiments": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Включить теневой эксперимент отбора тестов
         * @description Рекомендация набора тестов сохраняется и сравнивается с полным прогоном без изменения pipeline (FR-051, UI-70). Сокращение прогонов требует отдельного решения после оценки.
         */
        post: operations["createTestSelectionExperiment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/test-selection-experiments/{experiment_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Оценка теневого отбора тестов
         * @description Пропущенные падения и оценка экономии по сопоставимым полным прогонам (FR-051). Неполное сравнение не даёт «0 пропусков».
         */
        get: operations["getTestSelectionExperiment"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/test-selection-experiments/{experiment_id}/termination": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        /**
         * Прекратить теневой эксперимент отбора тестов
         * @description Выключает дальнейшие рекомендации и новые сравнения эксперимента (UC-41, UI-70). PUT без тела идемпотентен: повтор возвращает тот же stopped_at. Результаты уже зафиксированных сравнений остаются доступны через getTestSelectionExperiment. Выполняющееся сравнение после остановки не добавляется к итогам эксперимента. Pipeline клиента не меняется, полный набор тестов продолжает выполняться. Возобновление этим ресурсом не поддерживается; новый эксперимент создаётся отдельно.
         */
        put: operations["stopTestSelectionExperiment"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/documents/{document_id}/drills": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Учения по runbook
         * @description Учения по версиям runbook; новая версия не наследует успех прежней (FR-052, UI-71).
         */
        get: operations["listRunbookDrills"];
        put?: never;
        /**
         * Записать учение по версии runbook
         * @description Неуспешное учение сохраняется с шагом остановки и не становится успешным (FR-052).
         */
        post: operations["createRunbookDrill"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/document-search": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Поиск по разрешённой документации
         * @description Выдержки только из документов, доступных пользователю (FR-053, UI-72). Пустой результат не доказывает отсутствие документа и не раскрывает закрытый. Включается после Q-02 и Q-08.
         */
        get: operations["searchDocuments"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/document-search/{query_id}/feedback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Отметить, помог ли найденный документ
         * @description Полезность учитывается только по явной отметке; переход по ссылке ею не считается (FR-053). Повтор отметки того же документа заменяет её.
         */
        post: operations["createSearchFeedback"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/surveys": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Провести pulse-опрос
         * @description Опрос с согласованными вопросами, группой и порогом публикации (FR-054, UI-40). Сбор не включается до решения Q-02; до него операция отвечает `409 state_conflict`.
         */
        post: operations["createSurvey"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/surveys/{survey_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Опрос и групповая сводка
         * @description Сводка публикуется только после закрытия и при достижении согласованного порога ответов (FR-054). Списка ответивших и отказавшихся нет.
         */
        get: operations["getSurvey"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/survey-invitations/current": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Форма опроса по приглашению
         * @description Вопросы опроса для участника по приглашению (UI-39). Участник может пропустить любой вопрос.
         */
        get: operations["getSurveyForm"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/survey-responses": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Отправить ответ на опрос
         * @description Ответ сохраняется без ID участника и точного времени; факт участия хранится отдельно (раздел 12.5.10). Второй ответ того же участника - `409 state_conflict`. Закрытие формы без отправки ответа не создаёт.
         */
        post: operations["submitSurveyResponse"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/services/{service_id}/delivery-stability": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Стабильность поставки сервиса
         * @description I39-I41 по подтверждённым связям развёртывания в production и инцидента с числителями, знаменателями и временем восстановления (FR-055, UI-41). Совпадение по времени - только кандидат связи. Ответ закрепляет revision_id; подтверждённые пары deployment/incident находятся в confirmed_links с cursor-пагинацией. При продолжении клиент передаёт ту же revision_id; cursor привязан к ревизии и субъекту. Агрегаты относятся ко всей разрешённой когорте, а не к текущей странице. Закрытые объекты и их вклад исключены; access_limited/access_notes не раскрывают число скрытых.
         */
        get: operations["getDeliveryStability"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ci-cost": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Расход и расчётная стоимость CI
         * @description Расход jobs и стоимость по применимым версиям ставок, фиксированные платежи и фактический счёт раздельно (FR-056, UI-42). Без ставки стоимость `UNKNOWN`, а не ноль; разные валюты не складываются. Ответ закреплён revision_id; usage разделяется по классу runner и применённой версии ставки. Для раскрытия строки getRevision(basis_revision_id) возвращает определения и версию ci_rate, listRevisionCohort с metric_id=I18 и point_start - те же attempt-основания и их вклад в job_seconds. Ссылки attempts ведут к исходному job; pipeline и все попытки доступны через getPipeline/listPipelineAttempts. Смена ставки не меняет закреплённую когорту. Права заново проверяются на каждом чтении, закрытые attempts и их вклад исключаются без скрытого количества.
         */
        get: operations["getCiCost"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ci-rates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Версии ставок стоимости CI
         * @description Ставки по классу runner с периодом действия (DR-017).
         */
        get: operations["listCiRates"];
        put?: never;
        /**
         * Задать версию ставки
         * @description Новая ставка не переписывает прежние оценки. Перекрытие периодов одного класса и валюты - `422` (ограничение `EXCLUDE`).
         */
        post: operations["createCiRateVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/teams/{team_id}/transfers": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Межкомандные передачи и ожидания
         * @description Подтверждённые эпизоды смены команды-владельца и ожидание после передачи (FR-058, UI-43). Недоступная команда не раскрывает объекты и скрытые итоги; длительность не приписывается виновнику.
         */
        get: operations["listTeamTransfers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/ci-invoices": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        /**
         * Записать фактический счёт поставщика CI
         * @description Счёт хранится отдельно от расчётной оценки и её не перезаписывает (FR-056 A2).
         */
        post: operations["createCiInvoice"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/operations/{operation_id}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        /**
         * Получить состояние фоновой операции
         * @description Доступна только инициатору в той же организации, с повторной проверкой исходного действия и области: connections.manage для sync_run, report.read для series_refresh, docs.manage для documentation_check. Чужая или истёкшая операция и отзыв доступа к исходному ресурсу дают одинаковый 404. Состояние и безопасная ошибка сохраняются 24 часа после завершения; очистка задания River не удаляет дескриптор раньше этого срока. Внутренние ID и сообщения очереди не выдаются.
         */
        get: operations["getBackgroundOperation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        /** @description Ошибка по RFC 9457 с расширениями craftPulse. Стек, SQL, внутренние адреса и значения параметров в ответ не попадают (SEC-23). */
        Problem: {
            /**
             * Format: uri
             * @description `urn:craftpulse:problem:<code>`.
             */
            type: string;
            /** @description Краткое название типа ошибки для разработчика. */
            title: string;
            /** @description Совпадает с HTTP-кодом ответа. */
            status: number;
            /** @description Пояснение конкретного случая; не показывается пользователю как есть. */
            detail?: string;
            /**
             * @description Стабильный код для ветвления клиента. Каталог - раздел 5 ТЗ.
             * @example not_found
             */
            code: string;
            /**
             * Format: uuid
             * @description ID для поиска записи в журнале (NFR-018).
             */
            correlation_id: string;
            /** @description Ошибки полей для `validation_failed`. */
            errors?: components["schemas"]["FieldError"][];
        };
        FieldError: {
            /** @description JSON Pointer поля в теле или имя параметра. */
            field: string;
            /**
             * @description Причина ошибки поля.
             * @enum {string}
             */
            code: "required" | "invalid_format" | "out_of_range" | "too_long" | "unknown_field" | "not_allowed_value" | "duplicate" | "outside_scope" | "unknown_reference";
            detail?: string;
        };
        /** @description Имена отсутствующих необязательных полей, закрытых правами. Индексы и пути к скрытым элементам коллекции запрещены: элементы отфильтровываются, а ограничение отмечается access_limited/access_notes. Обязательные поля схемы не маскируются отдельно; отсутствие права на такой объект закрывает весь объект. */
        Redactions: string[];
        /** @description Отметка ограничения выдачи без числа и названий скрытых объектов (FR-001). */
        AccessNote: {
            /**
             * @description `restricted_by_access` - часть объектов закрыта правами; `access_check_unavailable` -
             *     проверка прав источника недоступна, его область не выдаётся; `source_access_lost` -
             *     подключение потеряло доступ.
             * @enum {string}
             */
            code: "restricted_by_access" | "access_check_unavailable" | "source_access_lost";
            /** Format: uuid */
            connection_id?: string | null;
        };
        /**
         * @description Состояние данных показателя по модели метрик (раздел 4.3 ТЗ).
         * @enum {string}
         */
        DataState: "ASSESSED" | "PROVISIONAL" | "BOUNDED" | "UNKNOWN" | "NOT_EVALUATED" | "NOT_APPLICABLE";
        /**
         * @description Значение точки ряда. `value` есть только у `ASSESSED` и `PROVISIONAL`, `bound` - только у
         *     `BOUNDED`; у остальных состояний значения нет. Процент без известного знаменателя не
         *     выдаётся (FR-003).
         */
        MetricValue: {
            state: components["schemas"]["DataState"];
            value: number | null;
            /**
             * @example seconds
             * @example count
             * @example ratio
             */
            unit: string;
            numerator: number | null;
            denominator: number | null;
            n: number | null;
            bound: {
                /** @enum {string} */
                kind: "lower" | "upper";
                value: number;
            } | null;
            /** @description n < 20: отметка малой выборки, не замена состояния. */
            low_sample: boolean;
            excluded_count: number;
            missing_count: number;
            /** @description Причина состояния, например `history_incomplete` или `scan_missing`. */
            reason_code: string | null;
        };
        Period: {
            /** Format: date-time */
            from: string;
            /** Format: date-time */
            to: string;
        };
        /** @description Ссылка на объект craftPulse с данными для показа и адресом в источнике. */
        ObjectRef: {
            /** @enum {string} */
            kind: "task" | "merge_request" | "pipeline" | "attempt" | "finding" | "document" | "deployment" | "artifact" | "incident" | "test_case";
            /** Format: uuid */
            id: string;
            /** @description Ключ источника для показа, например `CORE-12` или `!42`. */
            display_key: string | null;
            title: string | null;
            /** Format: uri */
            source_url: string | null;
        };
        /** @description Область источника - проект трекера, репозиторий, пространство wiki. */
        AreaRef: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "tracker_project" | "repository" | "wiki_space" | "ci_project" | "analysis_project";
            display_name: string;
            /** Format: uuid */
            connection_id: string | null;
        };
        ActorRef: {
            /** Format: uuid */
            account_id: string | null;
            /** @enum {string} */
            kind: "human" | "bot" | "system" | "unknown";
            display_name: string | null;
        };
        /** @description Пакет принят к разбору. Итог - в квитанции по адресу `Location`. Пакет, который нарушает лимит, правила путей или конверт либо имеет неподдержанную схему, отклоняется сразу со статусом `rejected`, причина - в квитанции. */
        ReceiptAccepted: {
            /** Format: uuid */
            receipt_id: string;
            /** @enum {string} */
            status: "quarantined" | "duplicate" | "rejected";
            /** Format: uuid */
            duplicate_of?: string | null;
            /** @description Через сколько секунд прочитать квитанцию по Location; null, если результат уже конечный. Для duplicate опрашивается первая квитанция. */
            retry_after_seconds: number | null;
        };
        Team: {
            /** Format: uuid */
            id: string;
            name: string;
            /** @enum {string} */
            status: "active" | "archived";
        };
        Project: components["schemas"]["Team"];
        Service: components["schemas"]["Team"];
        TeamPage: {
            data: components["schemas"]["Team"][];
            next_cursor: string | null;
        };
        ProjectPage: {
            data: components["schemas"]["Project"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        ServicePage: {
            data: components["schemas"]["Service"][];
            next_cursor: string | null;
        };
        AreaPage: {
            data: components["schemas"]["AreaRef"][];
            next_cursor: string | null;
        };
        User: {
            /** Format: uuid */
            id: string;
            display_name: string | null;
            /** @enum {string} */
            status: "active" | "blocked";
            /** Format: uuid */
            person_id: string | null;
        };
        UserPage: {
            data: components["schemas"]["User"][];
            next_cursor: string | null;
        };
        /**
         * @description Роль craftPulse (раздел 2.2, 7.7). `security_access` и `wiki_access` - отдельные права на данные безопасности и wiki.
         * @enum {string}
         */
        Role: "administrator" | "dev_lead" | "project_manager" | "tech_lead" | "developer" | "process_owner" | "service_owner" | "security_access" | "wiki_access";
        /** @description Область роли - не больше одного из `team_id`, `project_id`, `service_id`; все пустые - вся организация. `administrator` - только вся организация, `process_owner` - команда, `service_owner` - сервис. */
        RoleAssignmentInput: {
            role: components["schemas"]["Role"];
            /** Format: uuid */
            team_id: string | null;
            /** Format: uuid */
            project_id: string | null;
            /** Format: uuid */
            service_id: string | null;
        };
        RoleAssignment: {
            role: components["schemas"]["Role"];
            /** Format: uuid */
            team_id: string | null;
            /** Format: uuid */
            project_id: string | null;
            /** Format: uuid */
            service_id: string | null;
            /** Format: date-time */
            assigned_at: string;
        };
        RoleAssignmentList: {
            data: components["schemas"]["RoleAssignment"][];
        };
        RoleAssignmentListInput: {
            data: components["schemas"]["RoleAssignmentInput"][];
        };
        CurrentUser: {
            user: components["schemas"]["User"];
            organization: {
                /** Format: uuid */
                id: string;
                name: string;
                slug: string;
                /** @enum {string} */
                status: "setup" | "active" | "blocked" | "deleting";
            };
            roles: components["schemas"]["RoleAssignment"][];
            /** @description Назначенная область. Пустые списки - область не назначена. */
            areas: {
                teams: components["schemas"]["Team"][];
                projects: components["schemas"]["Project"][];
                services: components["schemas"]["Service"][];
            };
            /** @description Значения `x-craftpulse-action`, разрешённые ролями пользователя хотя бы в одной области. Для показа элементов интерфейса; сервер проверяет право на каждом запросе. */
            actions: string[];
            session: {
                /**
                 * Format: date-time
                 * @description Конец общего срока 12 ч.
                 */
                expires_at: string;
                /**
                 * Format: date-time
                 * @description Конец срока неактивности 1 ч на момент ответа.
                 */
                idle_expires_at: string;
            };
            /** @description Личное участие в рекомендациях ревью (UI-69). Передаётся с M1 при доступной возможности; отсутствие поля означает, что настройка сейчас недоступна, а не согласие на участие. */
            review_participation?: components["schemas"]["ReviewParticipation"];
        };
        ReviewParticipation: {
            participates: boolean;
        };
        SourceLink: {
            /** Format: uuid */
            connection_id: string;
            provider: components["schemas"]["Provider"];
            display_name: string;
            /** @enum {string} */
            state: "not_linked" | "linked" | "expired" | "revoked";
            /** Format: date-time */
            linked_at: string | null;
        };
        SourceLinkList: {
            data: components["schemas"]["SourceLink"][];
        };
        AuthorizationRedirect: {
            /** Format: uri */
            authorization_url: string;
        };
        /** @description Интервал действия `[valid_from, valid_to)`. `null` в `valid_to` - открытый интервал. */
        Interval: {
            /** Format: date-time */
            valid_from: string;
            /** Format: date-time */
            valid_to: string | null;
        };
        Component: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            service_id: string;
            /** Format: uuid */
            repository_area_id: string;
            /** @description Пусто - весь репозиторий. */
            path_prefix: string;
            /** @enum {string} */
            status: "proposed" | "confirmed" | "rejected";
            /** @enum {string} */
            origin: "manifest" | "manual";
        };
        ComponentInput: {
            /**
             * Format: uuid
             * @description `null` - новый компонент.
             */
            id: string | null;
            /** Format: uuid */
            service_id: string;
            /** Format: uuid */
            repository_area_id: string;
            path_prefix: string;
            /** @enum {string} */
            status: "confirmed" | "rejected";
        };
        TeamProjectLink: {
            /** Format: uuid */
            team_id: string;
            /** Format: uuid */
            project_id: string;
        } & components["schemas"]["Interval"];
        ProjectAreaLink: {
            /** Format: uuid */
            project_id: string;
            /** Format: uuid */
            area_id: string;
        } & components["schemas"]["Interval"];
        TeamMemberLink: {
            /** Format: uuid */
            team_id: string;
            /** Format: uuid */
            person_id: string;
        } & components["schemas"]["Interval"];
        NamedEntityInput: {
            /**
             * Format: uuid
             * @description `null` - новая запись.
             */
            id: string | null;
            name: string;
            /** @enum {string} */
            status: "active" | "archived";
        };
        OrganizationStructure: {
            version: number;
            teams: components["schemas"]["Team"][];
            projects: components["schemas"]["Project"][];
            services: components["schemas"]["Service"][];
            components: components["schemas"]["Component"][];
            team_projects: components["schemas"]["TeamProjectLink"][];
            project_areas: components["schemas"]["ProjectAreaLink"][];
            team_members: components["schemas"]["TeamMemberLink"][];
        };
        OrganizationStructureInput: {
            teams: components["schemas"]["NamedEntityInput"][];
            projects: components["schemas"]["NamedEntityInput"][];
            services: components["schemas"]["NamedEntityInput"][];
            components: components["schemas"]["ComponentInput"][];
            team_projects: {
                /** Format: uuid */
                team_id: string;
                /** Format: uuid */
                project_id: string;
                /**
                 * Format: date-time
                 * @description На M0 `null`: интервал начинается с момента сохранения.
                 */
                valid_from: string | null;
                /** Format: date-time */
                valid_to: string | null;
            }[];
            project_areas: {
                /** Format: uuid */
                project_id: string;
                /** Format: uuid */
                area_id: string;
                /**
                 * Format: date-time
                 * @description На M0 `null`: интервал начинается с момента сохранения.
                 */
                valid_from: string | null;
                /** Format: date-time */
                valid_to: string | null;
            }[];
            team_members: {
                /** Format: uuid */
                team_id: string;
                /** Format: uuid */
                person_id: string;
                /**
                 * Format: date-time
                 * @description На M0 `null`: интервал начинается с момента сохранения.
                 */
                valid_from: string | null;
                /** Format: date-time */
                valid_to: string | null;
            }[];
        };
        /** @enum {string} */
        ActorKind: "human" | "bot" | "system" | "unknown";
        Person: {
            /** Format: uuid */
            id: string;
            /** @description `null` после обезличивания. */
            display_name: string | null;
            kind: components["schemas"]["ActorKind"];
        };
        PersonPage: {
            data: components["schemas"]["Person"][];
            next_cursor: string | null;
        };
        /**
         * @description Вид источника. Клиент показывает неизвестное значение как «другой источник».
         * @enum {string}
         */
        Provider: "jira_cloud" | "gitlab" | "github" | "jenkins" | "sonarqube" | "confluence_cloud" | "git" | "universal" | "client";
        SourceAccount: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            instance_id: string;
            provider: components["schemas"]["Provider"];
            native_account_id: string;
            login: string | null;
            display_name: string | null;
            /** @description Вид участника по данным источника (признак бота или служебной записи); предложение, действует только подтверждённое `assignment.actor_kind`. */
            proposed_kind: components["schemas"]["ActorKind"];
            assignment: {
                /** Format: uuid */
                id: string;
                /** Format: uuid */
                person_id: string | null;
                actor_kind: components["schemas"]["ActorKind"];
                /** Format: date-time */
                confirmed_at: string;
            } | null;
            /** @description Предложения сопоставления; совпадение имени предложения не создаёт. */
            proposals: {
                /** Format: uuid */
                person_id: string;
                /** @enum {string} */
                basis: "login_match" | "email_match" | "provider_identity";
            }[];
        };
        SourceAccountPage: {
            data: components["schemas"]["SourceAccount"][];
            next_cursor: string | null;
        };
        AccountAssignmentBatch: {
            items: {
                /** Format: uuid */
                source_account_id: string;
                /**
                 * Format: uuid
                 * @description Обязателен для `human`, если не задан `create_person`.
                 */
                person_id: string | null;
                /**
                 * @description Создать нового человека с отображаемым именем учётной записи и привязать её к нему; `person_id` при этом `null`. Так заводится участник, которого ещё нет в `listPersons`.
                 * @default false
                 */
                create_person: boolean;
                actor_kind: components["schemas"]["ActorKind"];
                /**
                 * Format: uuid
                 * @description Действующая привязка, которую видел администратор; `null` - привязки не было.
                 */
                base_assignment_id: string | null;
            }[];
        };
        AccountAssignmentBatchResult: {
            items: {
                /** Format: uuid */
                source_account_id: string;
                /** @enum {string} */
                outcome: "saved" | "unchanged" | "version_conflict" | "rejected";
                /** Format: uuid */
                assignment_id: string | null;
                /** @description Для `rejected`: `not_found`, `person_required` и т. п. */
                error_code: string | null;
            }[];
        };
        /** @description Ровно одно из полей задано. */
        ScopeGrant: {
            /** Format: uuid */
            team_id: string | null;
            /** Format: uuid */
            project_id: string | null;
            /** Format: uuid */
            service_id: string | null;
        };
        ApiKey: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            owner_user_id: string;
            grants: components["schemas"]["ScopeGrant"][];
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: date-time */
            revoked_at: string | null;
        };
        ApiKeyCreated: components["schemas"]["ApiKey"] & {
            /** @description Значение ключа; показывается один раз. */
            value: string;
        };
        ApiKeyCreate: {
            /** Format: uuid */
            owner_user_id: string;
            grants: components["schemas"]["ScopeGrant"][];
            /**
             * Format: date-time
             * @description Не позже 365 дней от выпуска.
             */
            expires_at: string;
        };
        ApiKeyPage: {
            data: components["schemas"]["ApiKey"][];
            next_cursor: string | null;
        };
        UploadToken: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            project_id: string;
            /** @enum {integer} */
            slot: 1 | 2;
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            revoked_at: string | null;
        };
        UploadTokenCreated: components["schemas"]["UploadToken"] & {
            /** @description Значение токена; показывается один раз. */
            value: string;
        };
        UploadTokenCreate: {
            /** Format: uuid */
            project_id: string;
        };
        UploadTokenPage: {
            data: components["schemas"]["UploadToken"][];
            next_cursor: string | null;
        };
        InstallationProtocol: {
            /** Format: uuid */
            id: string;
            release_sha: string;
            /** Format: date-time */
            started_at: string;
            /** Format: date-time */
            finished_at: string | null;
            /** @enum {string} */
            outcome: "running" | "passed" | "failed";
            checks: {
                /** @enum {string} */
                check_kind: "site" | "adapter" | "adapter_store" | "identity_provider" | "update_source" | "database" | "object_store" | "runner_network";
                /** @enum {string} */
                outcome: "passed" | "failed" | "skipped";
                reason_code: string | null;
            }[];
        };
        AuditEvent: {
            /** Format: uuid */
            id: string;
            /** Format: date-time */
            occurred_at: string;
            /** @enum {string} */
            actor_kind: "user" | "api_key" | "upload_token" | "system" | "source";
            /** Format: uuid */
            actor_id: string | null;
            operation: string;
            object_kind: string | null;
            /** Format: uuid */
            object_id: string | null;
            /** @enum {string} */
            outcome: "succeeded" | "denied" | "failed";
            reason_code: string | null;
            /** @description Версия объекта до изменения: структуры, набора ролей, политики (раздел 13.8). `null` - у события нет версии. */
            base_version: string | null;
            /** @description Версия объекта после изменения. */
            new_version: string | null;
            /** Format: uuid */
            correlation_id: string;
        };
        AuditEventPage: {
            data: components["schemas"]["AuditEvent"][];
            next_cursor: string | null;
        };
        /** @enum {string} */
        DataKind: "tasks" | "merge_requests" | "ci" | "reports" | "documents" | "accounts" | "permissions";
        Adapter: {
            provider: components["schemas"]["Provider"];
            display_name: string;
            editions: string[];
            contract_versions: string[];
            data_kinds: components["schemas"]["DataKind"][];
            /** @description Виды областей, которые принимает адаптер в `ConnectionScopeInput.kind`. */
            scope_kinds: ("tracker_project" | "repository" | "wiki_space" | "ci_project" | "analysis_project")[];
            /** @description Поля `Credentials` этого адаптера; по ним строится блок «Учётные данные» формы UI-10. */
            credential_fields: {
                /** @description Ключ в `Credentials`. */
                name: string;
                label: string;
                required: boolean;
                /** @description Значение вводится скрытым полем и после сохранения не показывается. */
                secret: boolean;
            }[];
        };
        AdapterList: {
            data: components["schemas"]["Adapter"][];
        };
        Capability: {
            /** @enum {string} */
            capability: "read_current" | "read_status_history" | "read_field_history" | "read_resolution_history" | "read_sprint_history" | "read_links" | "read_permissions" | "read_deletions" | "write_mr_comment" | "write_tracker_issue" | "webhook";
            /** @enum {string} */
            state: "verified" | "partial" | "missing";
            /** @description Причина для `partial` и `missing`, например `permission_missing`. */
            reason_code: string | null;
            /** Format: date-time */
            checked_at: string;
        };
        ConnectionScope: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "tracker_project" | "repository" | "wiki_space" | "ci_project" | "analysis_project";
            native_scope_id: string;
            display_name: string;
            /**
             * Format: uuid
             * @description Область появляется после первого сбора.
             */
            area_id: string | null;
            /** Format: uuid */
            repository_scope_id?: string | null;
            /** Format: date-time */
            history_from: string | null;
        };
        ConnectionScopeInput: {
            /** @enum {string} */
            kind: "tracker_project" | "repository" | "wiki_space" | "ci_project" | "analysis_project";
            native_scope_id: string;
            /**
             * Format: date-time
             * @description Начало загрузки истории; `null` - вся доступная история.
             */
            history_from: string | null;
            /**
             * Format: uuid
             * @description Для `ci_project` и `analysis_project` вне Git-хостинга (Jenkins, SonarQube) - область репозитория, к которой относятся их запуски и отчёты (раздел 13.5).
             */
            repository_scope_id?: string | null;
        };
        Connection: {
            /** Format: uuid */
            id: string;
            name: string;
            provider: components["schemas"]["Provider"];
            instance: {
                /** Format: uuid */
                id: string;
                /** Format: uri */
                base_url: string | null;
                edition: string | null;
                source_version: string | null;
            };
            /**
             * @description Из `access_lost` подключение возвращается в `active` так же, как из `draft` - после замены учётных данных (`updateConnection`), новой успешной пробы и `activateConnection`; сбор продолжается с сохранённого курсора и заканчивается сверкой.
             * @enum {string}
             */
            status: "draft" | "active" | "access_lost" | "deleted";
            /** @enum {string} */
            adapter_kind: "builtin" | "client";
            /** @enum {string} */
            access_mode: "service_account" | "user_token" | "synced";
            /**
             * Format: uri
             * @description Адрес `receiveWebhook` этого подключения для настройки в источнике; `null`, если адаптер не поддерживает webhook. Секрет подписи вводится вместе с учётными данными и хранится адаптером.
             */
            webhook_url?: string | null;
            /**
             * Format: date-time
             * @description Учётные данные заданы; их значение не возвращается.
             */
            credentials_set_at: string | null;
            sync_paused: boolean;
            /**
             * Format: date-time
             * @description Пауза лимита источника по `Retry-After`.
             */
            paused_until: string | null;
            /** Format: date-time */
            last_success_at: string | null;
            scopes: components["schemas"]["ConnectionScope"][];
            capabilities: components["schemas"]["Capability"][];
        };
        ConnectionPage: {
            data: components["schemas"]["Connection"][];
            next_cursor: string | null;
        };
        /** @description Учётные данные в формате адаптера. Передаются адаптеру и не сохраняются ядром; в ответах отсутствуют. */
        Credentials: {
            [key: string]: string;
        };
        ConnectionCreate: {
            name: string;
            provider: components["schemas"]["Provider"];
            /**
             * Format: uri
             * @description Адрес источника. `ssh://` принимается только для адаптера `git`; для остальных адаптеров - 422.
             */
            base_url: string;
            /** @enum {string} */
            adapter_kind: "builtin" | "client";
            /**
             * Format: uri
             * @description Обязателен для адаптера клиента.
             */
            adapter_url: string | null;
            /** @description Ключ адаптера клиента; хранится зашифрованным в `connect`. */
            adapter_key: string | null;
            credentials: components["schemas"]["Credentials"];
            scopes: components["schemas"]["ConnectionScopeInput"][];
        };
        /** @description JSON Merge Patch. Отсутствующее поле не меняется. */
        ConnectionPatch: {
            name?: string;
            /** Format: uri */
            base_url?: string;
            /** Format: uri */
            adapter_url?: string;
            adapter_key?: string;
            credentials?: components["schemas"]["Credentials"];
            scopes?: components["schemas"]["ConnectionScopeInput"][];
            sync_paused?: boolean;
        };
        ConnectionProbe: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            status: "pending" | "running" | "succeeded" | "failed";
            /** Format: date-time */
            started_at: string;
            /** Format: date-time */
            finished_at: string | null;
            edition: string | null;
            source_version: string | null;
            contract_version: string | null;
            capabilities: components["schemas"]["Capability"][];
            /** @description Причина неуспеха: `edition_mismatch`, `auth_failed`, `outside_network_scope`, `certificate_untrusted`, `timeout`, `adapter_unavailable`, `contract_unsupported`. Изменение конфигурации во время пробы - configuration_changed. */
            reason_code: string | null;
            retry_after_seconds: number | null;
            /** @description row_version подключения, для которой выполнена проба; сверяется при активации. */
            configuration_version: number;
        };
        ConnectionActivation: {
            /** Format: uuid */
            probe_id: string;
        };
        SyncDataKindStatus: {
            data_kind: components["schemas"]["DataKind"];
            /** @enum {string} */
            phase: "backfill" | "incremental" | "paused" | "stopped";
            /** Format: date-time */
            last_attempt_at: string | null;
            /** @enum {string|null} */
            last_outcome: "success" | "partial" | "error" | null;
            /** @description `unauthorized`, `forbidden`, `rate_limited`, `unavailable`, `timeout`, `adapter_unavailable`, `invalid_response`. */
            last_error_code: string | null;
            /** @description Есть страница, на которой сбор остановился; повтор продолжит с неё. */
            failed_page_present: boolean;
            /** Format: date-time */
            last_success_at: string | null;
            /**
             * Format: date-time
             * @description Время, до которого выборка подтверждена сверкой.
             */
            watermark: string | null;
            lag_seconds: number | null;
            /** Format: date-time */
            history_from: string | null;
            /** @enum {string} */
            completeness: "complete" | "partial" | "unknown";
            unknown_fields: string[];
            affected_metrics: string[];
        };
        SyncStatus: {
            /** Format: uuid */
            connection_id: string;
            /** @enum {string} */
            connection_status: "draft" | "active" | "access_lost" | "deleted";
            /** Format: date-time */
            paused_until: string | null;
            data_kinds: components["schemas"]["SyncDataKindStatus"][];
            last_reconcile: {
                /** Format: date-time */
                started_at: string;
                /** Format: date-time */
                finished_at: string | null;
                /** @enum {string} */
                outcome: "running" | "complete" | "incomplete" | "failed";
                expected_count: number | null;
                observed_count: number | null;
                missing_count: number | null;
                changed_count: number | null;
            } | null;
            /** @description Диагностика webhook подключения; `null`, если webhook не поддерживается или ещё не приходил. */
            webhooks?: {
                /** Format: date-time */
                last_received_at: string | null;
                /** @description Отклонённые доставки за последние 24 ч. */
                rejected_count: number;
                /** Format: date-time */
                last_rejected_at: string | null;
                /** @description `signature_invalid`, `payload_invalid`, `adapter_unavailable`. */
                last_reject_reason: string | null;
            } | null;
        };
        SyncRunRequest: {
            /** @enum {string} */
            kind: "resume" | "reconcile";
            data_kind: components["schemas"]["DataKind"];
        };
        CiCheckProfile: {
            /** Format: uuid */
            project_id: string;
            profile_version_no: number;
            entries: {
                /** Format: uuid */
                component_id: string;
                path_prefix: string;
                language: string;
                analysis_kind: components["schemas"]["AnalysisKind"];
                /** @enum {string} */
                mode: "import" | "builtin";
                profile_id: string;
                /** @description Образ штатного профиля; `null` для режима импорта. */
                image_digest: string | null;
            }[];
            expected_reports: components["schemas"]["AnalysisKind"][];
            /** @description Корни документации проекта для профилей `docs` и `openapi` (UC-36); пусто, если корни не назначены. */
            documentation_roots: {
                /** Format: uuid */
                root_id: string;
                /** @enum {string} */
                kind: "docs" | "openapi";
                /** @description Путь корня в репозитории; пустая строка - корень репозитория. */
                path_prefix: string;
            }[];
            limits: {
                max_package_bytes: number;
                max_unpacked_bytes: number;
                max_files: number;
            };
        };
        /** @enum {string} */
        AnalysisKind: "code" | "sast" | "sca" | "secrets" | "iac" | "image" | "junit" | "coverage" | "docs" | "openapi" | "sonar";
        ImportUpload: {
            /** @description Отчёт передаётся как JSON, XML или текст LCOV. Для других схем JSON содержит массив записей `Record` из `envelope.yaml`, а CSV содержит снимок текущих задач по `TaskCsvRow`. */
            file: string;
            /**
             * @description `merge_requests` и `ci_runs` несут те же факты MR, запусков, jobs и попыток, что отдают адаптеры Git и CI.
             * @enum {string}
             */
            schema_id: "tasks" | "task_events" | "relations" | "merge_requests" | "ci_runs" | "findings" | "junit" | "coverage" | "documents";
            /**
             * @description Для `findings` укажите `envelope`, `sarif`, `gitlab_security` или `gitlab_code_quality`.
             *     Файл `envelope` содержит JSON-массив записей `Record` со схемой `finding/1` и ревизией `commit_sha`.
             *     Для `junit` укажите `junit_xml`. Для `coverage` доступны `lcov`, `cobertura` и `jacoco`.
             *     Остальные схемы это поле не используют.
             * @enum {string}
             */
            report_format?: "envelope" | "sarif" | "gitlab_security" | "gitlab_code_quality" | "junit_xml" | "lcov" | "cobertura" | "jacoco";
            /**
             * @description Вид проверки для SARIF. Если поле пропущено, используется `code`. Для GitLab Security вид определяется по `scan.type`.
             * @enum {string}
             */
            report_kind?: "code" | "sast" | "sca" | "secrets" | "iac" | "image" | "docs" | "openapi";
            /** @description Абсолютный путь корня репозитория на машине, где получен отчёт покрытия. Нужен для сопоставления абсолютных путей отчёта. */
            source_root?: string;
            /** @description Каталог от корня репозитория, относительно которого указаны пути в отчёте покрытия. По умолчанию используется корень. */
            source_dir?: string;
            /** @description Ревизия, к которой относится отчёт; обязательна для `findings`, `junit` и `coverage`. Без неё отчёт отклоняется: результат не привязывается к head по догадке. */
            commit_sha?: string;
            schema_version: string;
            source_provider: components["schemas"]["Provider"];
            /** @description Экземпляр системы-источника; совпадение с подключённым экземпляром даёт те же идентичности (раздел 12.4). */
            source_instance_key: string;
            /** Format: uuid */
            project_id: string;
            /**
             * @description Файл содержит только текущее состояние без истории (CSV); прошлый WIP из него не строится.
             * @default false
             */
            snapshot: boolean;
        };
        Receipt: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            channel: "sync" | "ci" | "import";
            /** @enum {string} */
            status: "quarantined" | "accepted" | "partial" | "rejected" | "duplicate";
            /** Format: date-time */
            received_at: string;
            schema_id: string | null;
            schema_version: string | null;
            record_count: number;
            accepted_count: number;
            rejected_count: number;
            replay_count: number;
            conflict_count: number;
            /** @description Причина отказа пакета целиком. Возможные значения: `schema_unsupported`, `envelope_invalid`, `path_unsafe`, `limit_exceeded`, `outside_scope`. */
            reason_code: string | null;
            /** Format: uuid */
            duplicate_of: string | null;
            affected_metrics: string[];
            retry_after_seconds: number | null;
            /**
             * Format: uri-reference
             * @description Экран с результатом, если он доступен субъекту.
             */
            result_url: string | null;
        };
        ReceiptSummary: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            channel: "sync" | "ci" | "import";
            /** @enum {string} */
            status: "quarantined" | "accepted" | "partial" | "rejected" | "duplicate";
            /** Format: date-time */
            received_at: string;
            schema_id: string | null;
            schema_version: string | null;
            record_count: number;
            accepted_count: number;
            rejected_count: number;
            reason_code: string | null;
            /** Format: uuid */
            duplicate_of: string | null;
            affected_metrics: string[];
        };
        ReceiptPage: {
            data: components["schemas"]["ReceiptSummary"][];
            next_cursor: string | null;
        };
        ReceiptRecord: {
            record_no: number;
            record_kind: string;
            /** @description Исходные координаты записи для показа; `null`, если идентичность не определена. */
            native_key: string | null;
            /** @enum {string} */
            outcome: "accepted" | "replay" | "conflict" | "rejected" | "suppressed";
            reason_code: string | null;
        };
        ReceiptRecordPage: {
            data: components["schemas"]["ReceiptRecord"][];
            next_cursor: string | null;
        };
        IngestConflict: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            receipt_id: string;
            record_no: number;
            /** Format: uuid */
            existing_receipt_id: string;
            existing_record_no: number;
            native_key: string;
            /** Format: date-time */
            detected_at: string;
            /** @enum {string} */
            status: "open" | "resolved";
            /** @enum {string|null} */
            resolution: "source_corrected" | "kept_existing" | "accepted_new" | null;
        };
        IngestConflictPage: {
            data: components["schemas"]["IngestConflict"][];
            next_cursor: string | null;
        };
        IngestConflictResolutionInput: {
            /** @enum {string} */
            resolution: "source_corrected" | "kept_existing" | "accepted_new";
        };
        WorkflowCatalog: {
            instances: {
                /** Format: uuid */
                instance_id: string;
                provider: components["schemas"]["Provider"];
                statuses: {
                    native_id: string;
                    name: string;
                    /** @description Категория в трекере; смысла не задаёт. */
                    source_category: string | null;
                }[];
                resolutions: {
                    native_id: string;
                    name: string;
                }[];
                issue_types: {
                    native_id: string;
                    name: string;
                }[];
                fields: {
                    native_id: string;
                    name: string;
                    /** @enum {string} */
                    value_type: "string" | "number" | "option" | "user" | "date" | "other";
                }[];
            }[];
            unmapped_statuses: components["schemas"]["NativeStatusRef"][];
        };
        NativeStatusRef: {
            /** Format: uuid */
            instance_id: string;
            native_id: string;
        };
        /**
         * @description Правило сопоставления (раздел 12.5.6). Значение без правила остаётся неизвестным и не даёт начала или завершения.
         *     code_required_type отмечает ID типа задачи, для которого обязательна связь с MR (A20, T28);
         *     native_value пуст, meaning - code_required. Без таких правил A20 и T28 не рассчитываются.
         *     blocked_flag задаёт ID поля-флага в native_ref и его значение в native_value, при котором
         *     начатая задача заблокирована (T13, T15); meaning - blocked. Интервалы флага объединяются
         *     с интервалами статусов со смыслом blocked, перекрытия не суммируются (BR-005).
         *     Правила acceptance_gate, epic_field и business_epic_value доступны с M1; на M0
         *     их передача даёт 422 с not_allowed_value у rule_kind. acceptance_gate отмечает
         *     ID статуса приёмки отдельно от основного смысла этого статуса; native_value пуст.
         *     epic_field задаёт ID поля связи с эпиком; native_value пуст. business_epic_value
         *     задаёт ID поля и его исходное значение, подтверждающее признак бизнес-эпика.
         * @example {
         *       "rule_kind": "acceptance_gate",
         *       "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
         *       "native_ref": "51",
         *       "native_value": "",
         *       "meaning": "acceptance",
         *       "rank": null
         *     }
         * @example {
         *       "rule_kind": "epic_field",
         *       "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
         *       "native_ref": "customfield_10014",
         *       "native_value": "",
         *       "meaning": "parent_epic",
         *       "rank": null
         *     }
         * @example {
         *       "rule_kind": "business_epic_value",
         *       "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
         *       "native_ref": "customfield_10028",
         *       "native_value": "yes",
         *       "meaning": "business_epic",
         *       "rank": null
         *     }
         */
        MappingRule: {
            /** @enum {string} */
            rule_kind: "status" | "resolution" | "issue_type" | "code_required_type" | "size_field" | "size_value" | "team_field" | "team_value" | "blocked_flag" | "acceptance_gate" | "epic_field" | "business_epic_value";
            /** Format: uuid */
            instance_id: string;
            native_ref: string;
            /** @description Пустая строка для правил без значения. */
            native_value: string;
            /** @description Для `status` - `intake`, `active`, `wait`, `blocked`, `done`, `cancelled`, `duplicate`, `ignored`; для `resolution` - `accepted`, `cancelled`, `duplicate`, `ignored`; для `size_value` и `issue_type` - метка; для `team_value` - ID команды; для `blocked_flag` - `blocked`. С M1: acceptance_gate - acceptance, epic_field - parent_epic, business_epic_value - business_epic. Эти правила не заменяют обычное правило status. */
            meaning: string;
            /** @description Порядок размера; обязателен для `size_value`. */
            rank: number | null;
        };
        MappingDraft: {
            /** @description Имя часового пояса IANA. */
            timezone: string;
            calendar_version_no: number | null;
            rules: components["schemas"]["MappingRule"][];
        };
        MappingVersion: {
            version_no: number;
            base_version_no: number | null;
            timezone: string;
            calendar_version_no: number | null;
            rules: components["schemas"]["MappingRule"][];
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        MappingVersionPage: {
            data: components["schemas"]["MappingVersion"][];
            next_cursor: string | null;
        };
        MappingPreview: {
            /** Format: date-time */
            as_of: string;
            /** @description Неделя отчёта до `as_of`, за которую считаются T05 и T08. */
            period: components["schemas"]["Period"];
            tasks: {
                task: components["schemas"]["ObjectRef"];
                /** Format: date-time */
                started_at: string | null;
                /** Format: date-time */
                completed_at: string | null;
                /** @enum {string} */
                outcome: "open" | "completed" | "cancelled" | "duplicate" | "unknown";
                blocked_seconds: number | null;
                task_type: string | null;
                size_label: string | null;
                /** @description Значения задачи без правил. */
                unmapped: string[];
            }[];
            control_values: {
                metric_id: string;
                value: components["schemas"]["MetricValue"];
            }[];
            unmapped_statuses: components["schemas"]["NativeStatusRef"][];
        };
        /** @enum {string} */
        Weekday: "mon" | "tue" | "wed" | "thu" | "fri" | "sat" | "sun";
        CalendarDraft: {
            timezone: string;
            workdays: components["schemas"]["Weekday"][];
            work_start: string | null;
            work_end: string | null;
            holidays: string[];
        };
        CalendarVersion: {
            version_no: number;
            base_version_no: number | null;
            timezone: string;
            workdays: components["schemas"]["Weekday"][];
            work_start: string | null;
            work_end: string | null;
            holidays: string[];
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        CalendarVersionPage: {
            data: components["schemas"]["CalendarVersion"][];
            next_cursor: string | null;
        };
        TargetBaselineList: {
            /** Format: uuid */
            revision_id: string;
            data: {
                task_type: string;
                size_label: string | null;
                p85: components["schemas"]["MetricValue"];
            }[];
        };
        TargetDraft: {
            task_type: string;
            size_label: string;
            duration_seconds: number;
            /** @enum {string} */
            basis: "manual" | "historical_p85";
            /**
             * Format: uuid
             * @description Обязателен для `historical_p85`.
             */
            basis_revision_id: string | null;
            /** @description Период ориентира `[from,to)` из getTargetBaselines; обязателен для `historical_p85`. */
            basis_period: {
                /** Format: date-time */
                from: string;
                /** Format: date-time */
                to: string;
            } | null;
        };
        /** @description Версия действует с момента подтверждения `confirmed_at` до подтверждения следующей версии той же группы; задним числом срок не задаётся, прежние результаты сохраняют свою версию (FR-019). */
        TargetVersion: {
            task_type: string;
            size_label: string;
            version_no: number;
            base_version_no: number | null;
            duration_seconds: number;
            /** @enum {string} */
            basis: "manual" | "historical_p85";
            /** Format: uuid */
            basis_revision_id: string | null;
            basis_value_seconds: number | null;
            basis_n: number | null;
            /** @description Окно ориентира, по которому посчитаны значение и n основания. */
            basis_period: {
                /** Format: date-time */
                from: string;
                /** Format: date-time */
                to: string;
            } | null;
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        TargetVersionPage: {
            data: components["schemas"]["TargetVersion"][];
            next_cursor: string | null;
        };
        RevisionSummary: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            series_id: string;
            revision_no: number;
            /** Format: date-time */
            as_of: string;
            /** Format: date-time */
            published_at: string | null;
            /** @description Загрузка истории не завершена; значения PROVISIONAL. */
            loading: boolean;
            /** @enum {string|null} */
            irreproducible_reason: "basis_deleted" | "basis_expired" | "definition_retired" | null;
        };
        MetricPoint: {
            /** Format: date-time */
            point_start: string;
            value: components["schemas"]["MetricValue"];
        };
        MetricSeries: {
            metric_id: string;
            definition_version: number;
            unit: string;
            /** @description Оценка показателя с несколькими рядами: `p50` и `p85` у T08 и T14, поле у T38 (`size`, `type`, `team`, `start`), вид у T40 (`lag`, `last_success_age`), T41 (`tasks`, `transitions`) и T42 (`missing`, `changed`). X01 использует `no_reviewer` и `no_first_response`, X02 - `failed`, `passed_after_retry` и `missing_report`. У A22 ключ `actor_kind:event_kind` выбирает когорту группы. Отсутствует у показателя с одним рядом. */
            estimator?: string;
            /** @description Группа ряда при разбиении; отсутствует у ряда без разбиения. Для A22 - вид события и вид участника, для отчёта команды - тип или размер задачи. */
            group?: {
                task_type?: string | null;
                /** @description `null` - группа «без размера». */
                size_label?: string | null;
                event_kind?: string;
                actor_kind?: components["schemas"]["ActorKind"];
            };
            points: components["schemas"]["MetricPoint"][];
        };
        TeamReport: {
            /** Format: uuid */
            team_id: string;
            period: components["schemas"]["Period"];
            /** @enum {string} */
            step: "day" | "week" | "month";
            revision: components["schemas"]["RevisionSummary"];
            /**
             * @description `current_structure` - группировка по текущему составу (M0), подписывается в интерфейсе; `structure_at_date` - состав на дату (M1).
             * @enum {string}
             */
            grouping: "current_structure" | "structure_at_date";
            metrics: components["schemas"]["MetricSeries"][];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        MetricDefinition: {
            metric_id: string;
            definition_version: number;
            name: string;
            unit: string;
            /** @description Определение в каталоге метрик. */
            catalog_ref: string;
            /** @description Параметры версии определения, от которых зависит значение; например, для T23 версии 1 - `follow_up_days: 30`. Отсутствует, если параметров нет. */
            parameters?: {
                [key: string]: unknown;
            };
        };
        MetricDefinitionPage: {
            data: components["schemas"]["MetricDefinition"][];
            next_cursor: string | null;
        };
        Series: {
            /** Format: uuid */
            id: string;
            scope: {
                /** Format: uuid */
                team_id: string | null;
                project_ids: string[];
                filters: {
                    [key: string]: unknown;
                };
            };
            metric_set: string;
            /** @enum {string} */
            step: "day" | "week" | "month";
            /** Format: uuid */
            current_revision_id: string | null;
        };
        SeriesPage: {
            data: components["schemas"]["Series"][];
            next_cursor: string | null;
        };
        SeriesData: {
            /** Format: uuid */
            series_id: string;
            revision: components["schemas"]["RevisionSummary"];
            metrics: components["schemas"]["MetricSeries"][];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        Revision: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            series_id: string;
            revision_no: number;
            /** @description Срез знания: факты и настройки ревизии прочитаны на этот номер (раздел 12.6). */
            knowledge_seq: number;
            /** @enum {string} */
            status: "building" | "published" | "superseded" | "failed";
            window: components["schemas"]["Period"];
            /** Format: date-time */
            as_of: string;
            timezone: string;
            /** Format: date-time */
            published_at: string | null;
            /** Format: date-time */
            superseded_at: string | null;
            loading: boolean;
            /** @enum {string|null} */
            irreproducible_reason: "basis_deleted" | "basis_expired" | "definition_retired" | null;
            definitions: components["schemas"]["MetricDefinition"][];
            inputs: {
                /** @enum {string} */
                input_kind: "definition" | "mapping" | "calendar" | "target" | "decision" | "structure" | "accounts" | "links" | "source_watermark" | "ci_rate";
                input_id: string;
                version: string | null;
            }[];
            source_watermarks: {
                /** Format: uuid */
                connection_id: string;
                /** Format: date-time */
                watermark: string | null;
                /** @enum {string} */
                completeness: "complete" | "partial" | "unknown";
            }[];
        };
        CohortItem: {
            object: components["schemas"]["ObjectRef"];
            /** @description Номер среди разрешённых элементов выбранной когорты, начиная с 0. Перенумеровывается после фильтрации; номер строки хранилища не выдаётся. */
            item_no: number;
            /** @enum {string} */
            role: "numerator" | "denominator" | "excluded" | "missing";
            value: number | null;
            reason_code: string | null;
            /** @description Основание удалено по запросу или сроку; содержимое объекта не выдаётся. */
            basis_deleted: boolean;
            /** @description Тип задачи по сопоставлению; передаётся для когорт задач. */
            task_type?: string | null;
            /** @description Размер задачи на начало эпизода; `null` - группа «без размера». Передаётся для когорт задач. */
            size_label?: string | null;
            /** @description Интервал, давший вклад - эпизод работы, блокировка или ожидание ревью. Передаётся для показателей длительности и возраста. */
            interval?: {
                /** Format: date-time */
                from: string;
                /**
                 * Format: date-time
                 * @description `null` - интервал открыт на as_of.
                 */
                to: string | null;
            };
            /** @description Возраст текущего эпизода задачи на as_of; передаётся в срезах ожидания ревью и оговорок CI рядом с ожиданием MR в `value`. */
            task_age_seconds?: number | null;
            /**
             * Format: uuid
             * @description Для attempt-оснований I18 передаётся ID доступного pipeline для UI-55/UI-56; закрытый pipeline и его вклад не выдаются.
             */
            pipeline_id?: string;
            /** @description Доступные MR и текущие pipelines, проверенные для вклада задачи в X01 или X02. */
            basis?: components["schemas"]["ObjectRef"][];
            /** @description Признаки ожидания ревью и состояния CI, найденные среди доступных оснований X01 или X02. */
            signals?: string[];
        };
        CohortPage: {
            data: components["schemas"]["CohortItem"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        /** @description Параметры UI-20. Период задаётся парой from/to [from,to) или period_days (по умолчанию 90 суток до as_of выбранной ревизии). У закреплённого среза относительный период сохраняется при новых расчётах. step должен совпадать с шагом series_id. Остальные поля состояния интерфейса сохраняются без интерпретации; права и выбранную ревизию они не меняют. */
        SavedViewFilters: {
            /** Format: date-time */
            from?: string;
            /** Format: date-time */
            to?: string;
            period_days?: number;
            /** @enum {string} */
            step?: "week" | "month";
            project_id?: string[];
            task_type?: string;
            /** @description unknown - группа без размера. */
            size_label?: string;
            /** @enum {string} */
            group_by?: "task_type" | "size_label";
        } & {
            [key: string]: unknown;
        };
        SavedViewInput: {
            name: string;
            /** Format: uuid */
            series_id: string;
            filters: components["schemas"]["SavedViewFilters"];
            /** Format: uuid */
            pinned_revision_id: string | null;
        };
        SavedView: {
            /** Format: uuid */
            id: string;
            name: string;
            /** Format: uuid */
            series_id: string;
            filters: components["schemas"]["SavedViewFilters"];
            /** Format: uuid */
            pinned_revision_id: string | null;
            /** Format: date-time */
            created_at: string;
            /** @description Отчёт по правам открывающего; присутствует в getSavedView, отсутствует в списке и ответе создания. */
            report?: components["schemas"]["TeamReport"];
        };
        SavedViewPage: {
            data: components["schemas"]["SavedView"][];
            next_cursor: string | null;
        };
        ExportCreate: {
            /** Format: uuid */
            revision_id: string;
            metric_ids: string[];
            /** @enum {string} */
            format: "json" | "csv";
        };
        Export: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            status: "pending" | "ready" | "failed" | "expired";
            /** @enum {string} */
            format: "json" | "csv";
            /** Format: uuid */
            revision_id: string;
            /** Format: date-time */
            requested_at: string;
            /** Format: date-time */
            completed_at: string | null;
            /** Format: date-time */
            expires_at: string | null;
            access_limited: boolean;
            retry_after_seconds: number | null;
        };
        /** @description Выгрузка в формате JSON - тот же срез, что `getSeriesData`, с паспортами и разрешённой когортой. */
        ExportDocument: {
            revision: components["schemas"]["Revision"];
            definitions: components["schemas"]["MetricDefinition"][];
            metrics: components["schemas"]["MetricSeries"][];
            cohort: {
                metric_id: string;
                /** Format: date-time */
                point_start: string;
                items: components["schemas"]["CohortItem"][];
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
            /** Format: date-time */
            generated_at: string;
        };
        ProjectOverview: {
            project: components["schemas"]["Project"];
            /** Format: date-time */
            as_of: string;
            wip: components["schemas"]["MetricValue"];
            over_target: components["schemas"]["MetricValue"];
            awaiting_review: components["schemas"]["MetricValue"];
            sources: {
                /** Format: uuid */
                connection_id: string;
                provider: components["schemas"]["Provider"];
                /** Format: date-time */
                last_success_at: string | null;
                /** @enum {string} */
                completeness: "complete" | "partial" | "unknown";
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        TimelineEvent: {
            /** Format: uuid */
            id: string;
            object: components["schemas"]["ObjectRef"];
            /** @description Для задач - `created`, `field_change`, `comment`, `link_change`, `moved`; для MR - `ready`, `reviewer_assigned`, `reviewer_removed`, `review`, `comment`, `approval`, `head_pushed`, `merged`, `closed`, `reopened`; для pipeline - `pipeline_started`, `pipeline_finished`, `attempt_started`, `attempt_finished` (исход - в `after_value`); для документа - `version_created`. */
            event_kind: string;
            field: string | null;
            before_value: string | null;
            after_value: string | null;
            actor: components["schemas"]["ActorRef"];
            /** Format: date-time */
            event_time: string;
            /** @description Смещение часового пояса источника в момент события. */
            event_offset_seconds: number;
            /** Format: date-time */
            observed_at: string;
            /** @enum {string} */
            observed_via: "webhook" | "incremental" | "backfill" | "reconcile" | "import";
            /** @description Событие получено позже, чем было учтено в ранее опубликованных ревизиях. */
            late: boolean;
        };
        TimelinePage: {
            data: components["schemas"]["TimelineEvent"][];
            /**
             * Format: date-time
             * @description Самая поздняя из границ истории подключений проекта: раньше неё часть событий отсутствует в данных, а не в источнике.
             */
            history_from: string | null;
            /** @description Граница и полнота истории по каждому подключению проекта. */
            sources?: {
                /** Format: uuid */
                connection_id: string;
                provider: components["schemas"]["Provider"];
                /** Format: date-time */
                history_from: string | null;
                /** @enum {string} */
                completeness: "complete" | "partial" | "unknown";
            }[];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        TaskStatus: {
            native_id: string;
            name: string;
            /** @enum {string} */
            meaning: "intake" | "active" | "wait" | "blocked" | "done" | "cancelled" | "duplicate" | "unknown";
        };
        TaskSummary: {
            /** Format: uuid */
            id: string;
            key: string | null;
            title: string | null;
            /** Format: uuid */
            project_id: string | null;
            status: components["schemas"]["TaskStatus"];
            task_type: string | null;
            /** @description `null` - размер неизвестен; задача в группе «без размера». */
            size_label: string | null;
            /**
             * Format: uuid
             * @description `null` - команда не определена правилами сопоставления.
             */
            team_id: string | null;
            /** @description Возраст текущего эпизода; `null` при неизвестном начале. */
            age_seconds: number | null;
            /** @description `null` - срок для группы не задан. */
            over_target: boolean | null;
            /** Format: date-time */
            updated_at: string | null;
            /** Format: uri */
            source_url: string | null;
        };
        TaskPage: {
            data: components["schemas"]["TaskSummary"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        Link: {
            /** Format: uuid */
            id: string;
            /**
             * @description deployment_incident: from_object_id развёртывания вызвал to=incident (I39/I40). incident_deployment: from_object_id инцидента вызвал to=deployment, внеплановый выпуск (I41). Подтверждение требует текущего доступа к обоим концам.
             * @enum {string}
             */
            link_type: "task_mr" | "mr_run" | "run_report" | "task_parent" | "finding_task" | "document_service" | "task_task" | "deployment_incident" | "incident_deployment";
            /** @enum {string} */
            method: "source_explicit" | "user_confirmed" | "branch_key" | "title_key" | "time_window" | "path_match";
            /** @enum {string} */
            status: "candidate" | "confirmed" | "rejected";
            /** Format: uuid */
            from_object_id: string;
            to: components["schemas"]["ObjectRef"];
            /** Format: date-time */
            decided_at: string | null;
        };
        LinkDecision: {
            /** @enum {string} */
            decision: "confirm" | "reject";
        };
        TaskDetail: {
            /** Format: uuid */
            id: string;
            key: string | null;
            title: string | null;
            /** Format: uuid */
            project_id: string | null;
            status: components["schemas"]["TaskStatus"];
            task_type: string | null;
            size_label: string | null;
            /** Format: uuid */
            team_id: string | null;
            age_seconds: number | null;
            over_target: boolean | null;
            /** Format: date-time */
            updated_at: string | null;
            /** Format: uri */
            source_url: string | null;
            target: {
                duration_seconds: number;
                version_no: number;
            } | null;
            blocked_intervals: {
                /** Format: date-time */
                from: string;
                /**
                 * Format: date-time
                 * @description `null` - блокировка открыта на as_of.
                 */
                to: string | null;
            }[];
            /**
             * @description Наблюдения о текущем эпизоде задачи по основаниям (FR-029), от самого раннего. У задачи их может быть
             *     несколько: например, MR без ревьюера и успешный pipeline после повторов. Если фактов для вывода
             *     не хватает, список содержит одно наблюдение `unknown` с действием `clarify_reason`; причина
             *     задержки не придумывается. Наблюдения о находках и отчётах строятся только по доступным
             *     пользователю данным.
             */
            observations: {
                /**
                 * @description `ci_passed_after_retry` - pipeline текущего head успешен после повторных попыток; `missing_report` - не получен отчёт, ожидаемый по профилю; `new_finding` - новая находка относительно сопоставимой базы в связанном MR.
                 * @enum {string}
                 */
                kind: "awaiting_reviewer" | "awaiting_first_response" | "ci_failed" | "ci_running" | "ci_passed_after_retry" | "missing_report" | "new_finding" | "blocked" | "no_merge_request" | "over_target" | "unknown";
                /** Format: date-time */
                since: string | null;
                basis: components["schemas"]["ObjectRef"][];
                /**
                 * @description Следующее доступное действие; выполняется пользователем в исходной системе или на экране craftPulse. `clarify_reason` - вопрос к исполнителю, когда данных для вывода нет.
                 * @enum {string}
                 */
                action: "assign_reviewer" | "request_review" | "check_pipeline" | "restore_report" | "review_finding" | "resolve_blocker" | "link_merge_request" | "review_target" | "clarify_reason";
                /**
                 * @description Недостающий вопрос при действии `clarify_reason`: назначен ли ревьюер в MR (поле ревьюера недоступно), есть ли MR по задаче, что задерживает задачу. У остальных наблюдений отсутствует.
                 * @enum {string}
                 */
                question?: "reviewer_assigned" | "merge_request_exists" | "delay_reason";
            }[];
            links: components["schemas"]["Link"][];
            redactions: components["schemas"]["Redactions"];
        };
        ReviewWait: {
            /** @enum {string} */
            kind: "no_reviewer" | "awaiting_first_response" | "awaiting_re_review" | "responded" | "unknown";
            /** Format: date-time */
            since: string;
            wait_seconds: number;
            /** @description Готовность MR неизвестна; ожидание считается от открытия. */
            fallback_open: boolean;
        } | null;
        MergeRequest: {
            /** Format: uuid */
            id: string;
            iid: number | null;
            title: string | null;
            /** @enum {string} */
            state: "open" | "merged" | "closed";
            draft: boolean;
            area: components["schemas"]["AreaRef"];
            author: components["schemas"]["ActorRef"];
            head_sha: components["schemas"]["GitSha"];
            /** @description Базовая ревизия MR; null, если источник её не передал. */
            base_sha?: string | null;
            /** @description Часть связанных задач скрыта правами; число и имена не раскрываются. */
            access_limited?: boolean;
            access_notes?: components["schemas"]["AccessNote"][];
            /** Format: date-time */
            opened_at: string;
            /** Format: date-time */
            ready_at: string | null;
            review_wait: components["schemas"]["ReviewWait"];
            /** Format: uri */
            source_url: string | null;
            /** @description Связи MR с задачами, подтверждённые и кандидаты. В `getMergeRequest` from_object_id содержит MR, to - связанную задачу; задача без доступа у пользователя не включается. ID, тип, метод и статус связи сохраняются независимо от направления показа. */
            links?: components["schemas"]["Link"][];
        };
        GitSha: string;
        MergeRequestPage: {
            data: components["schemas"]["MergeRequest"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        CodeLocation: {
            path: string;
            line_start: number | null;
            line_end: number | null;
        } | null;
        SummaryItem: {
            /** @description Номер в отфильтрованной сводке, начиная с 1. Номер строки хранилища не выдаётся. */
            item_no: number;
            /**
             * @description `accepted` - находка с действующим исключением: она остаётся в сводке отдельно и не считается исправленной.
             * @enum {string}
             */
            item_kind: "new" | "resolved" | "newly_observed" | "unchanged" | "accepted" | "not_evaluated" | "ci_attempts" | "coverage" | "tests";
            /** @description Вид анализа строки; отсутствует у строк попыток CI. */
            analysis_kind?: components["schemas"]["AnalysisKind"];
            /**
             * @description Отчёт клиента или штатный профиль; передаётся для строк находок и непроверенных областей.
             * @enum {string}
             */
            mode?: "import" | "builtin";
            /** Format: uuid */
            finding_id: string | null;
            rule_id: string | null;
            /** @description Сопоставленная серьёзность; исходная - в карточке находки. */
            severity: string | null;
            location: components["schemas"]["CodeLocation"];
            /** @description Порядок среди пяти важнейших разрешённых сигналов после фильтрации по аудитории (FR-060); без пропусков от скрытых строк. */
            rank: number | null;
        };
        MergeRequestSummary: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            merge_request_id: string;
            revision_no: number;
            /** @enum {string} */
            status: "building" | "ready" | "stale" | "superseded";
            head_sha: components["schemas"]["GitSha"];
            /** @description `null` - сопоставимой базы нет; находки `newly_observed`. */
            base_sha: string | null;
            /** @enum {string} */
            pipeline_state: "running" | "complete" | "reports_missing";
            /** Format: date-time */
            built_at: string | null;
            /** @description `new_head` - пришёл новый коммит; сводка не публикуется. */
            stale_reason: string | null;
            items: components["schemas"]["SummaryItem"][];
            not_evaluated: {
                analysis_kind: components["schemas"]["AnalysisKind"];
                reason_code: string;
            }[];
            coverage_changed_lines: components["schemas"]["MetricValue"];
            /** @description Размер и структура изменения относительно базы (C15-C17). Значение без сопоставимой базы или анализа имеет состояние `NOT_EVALUATED`. */
            change?: {
                lines_added: components["schemas"]["MetricValue"];
                lines_removed: components["schemas"]["MetricValue"];
                files_changed: components["schemas"]["MetricValue"];
                complexity_increased_functions: components["schemas"]["MetricValue"];
                new_clone_groups: components["schemas"]["MetricValue"];
            };
            /** @description Исходы тестов head по отчётам JUnit (I19); без отчёта значения `NOT_EVALUATED`. */
            tests?: {
                passed: components["schemas"]["MetricValue"];
                failed: components["schemas"]["MetricValue"];
                errored: components["schemas"]["MetricValue"];
                skipped: components["schemas"]["MetricValue"];
            };
            /** @description Ожидаемый состав проверок и отчётов профиля (I08/I09). */
            expected?: {
                missing_checks: components["schemas"]["MetricValue"];
                missing_reports: components["schemas"]["MetricValue"];
            };
            /** @description Последняя ревизия MR с полным набором ожидаемых отчётов; показывается отдельно с датой и не переносится на текущий head. */
            last_good?: {
                head_sha: components["schemas"]["GitSha"];
                revision_no: number;
                /** Format: date-time */
                built_at: string;
            } | null;
            ci: {
                /** Format: uuid */
                pipeline_id: string;
                /** @enum {string} */
                outcome: "running" | "success" | "failed" | "canceled" | "skipped" | "unknown";
                attempts: number;
                retried_jobs: number;
                /**
                 * Format: uuid
                 * @description Родительский pipeline у дочернего.
                 */
                parent_pipeline_id?: string | null;
            } | null;
            publication: {
                /** @enum {string} */
                status: "not_requested" | "pending" | "succeeded" | "failed" | "unknown" | "stale";
                external_id: string | null;
                /** @description `write_permission_missing`, `head_changed`, `provider_error`. */
                error_code: string | null;
                /** Format: date-time */
                published_at: string | null;
            };
            redactions: components["schemas"]["Redactions"];
            /** @description Часть строк скрыта правами; число и позиции скрытых строк не выдаются. */
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        Pipeline: {
            /** Format: uuid */
            id: string;
            ref_name: string;
            sha: components["schemas"]["GitSha"];
            /** Format: uuid */
            merge_request_id: string | null;
            /** @enum {string} */
            outcome: "running" | "success" | "failed" | "canceled" | "skipped" | "unknown";
            native_status: string;
            /** Format: date-time */
            started_at: string | null;
            /** Format: date-time */
            finished_at: string | null;
            attempts_total: number;
            retried_jobs: number;
            allowed_failures: number;
            /**
             * @description `truncated` - внутренние повторы известны не все (например, Jenkins без манифеста).
             * @enum {string}
             */
            attempt_history: "complete" | "truncated" | "unknown";
            reports: {
                analysis_kind: components["schemas"]["AnalysisKind"];
                /** @enum {string} */
                state: "received" | "missing" | "malformed" | "partial";
            }[];
            tests: {
                state: components["schemas"]["DataState"];
                passed: number | null;
                failed: number | null;
                error: number | null;
                skipped: number | null;
                distinct_cases?: number | null;
                executions?: number | null;
            };
            coverage?: components["schemas"]["PipelineCoverage"];
            /** Format: uri */
            source_url: string | null;
        };
        PipelineCoverage: {
            line: components["schemas"]["MetricValue"];
            branch: components["schemas"]["MetricValue"];
        };
        Attempt: {
            /** Format: uuid */
            id: string;
            job_key: string;
            config_fingerprint: string;
            attempt_no: number;
            /** @enum {string} */
            outcome: "running" | "success" | "failed" | "canceled" | "skipped" | "unknown";
            native_status: string;
            allow_failure: boolean;
            /** @description `null` - причина неизвестна и коду не приписывается. */
            failure_kind: string | null;
            runner_class: string | null;
            /** Format: date-time */
            started_at: string | null;
            /** Format: date-time */
            finished_at: string | null;
            duration_seconds: number | null;
            /** Format: uri */
            source_url: string | null;
        };
        AttemptPage: {
            data: components["schemas"]["Attempt"][];
            next_cursor: string | null;
            usage_seconds: components["schemas"]["MetricValue"];
        };
        TestExecution: {
            test_case: {
                /** Format: uuid */
                id: string;
                key: string;
            };
            /** Format: uuid */
            attempt_id: string | null;
            config_fingerprint: string;
            /** @enum {string} */
            outcome: "passed" | "failed" | "error" | "skipped";
            duration_ms: number | null;
        };
        TestExecutionPage: {
            data: components["schemas"]["TestExecution"][];
            next_cursor: string | null;
        };
        ProfileEntryInput: {
            /** Format: uuid */
            component_id: string;
            /** @description Пустая строка для проверок без языка. */
            language: string;
            analysis_kind: components["schemas"]["AnalysisKind"];
            /** @enum {string} */
            mode: "import" | "builtin";
            profile_id: string;
        };
        CheckProfileDraft: {
            entries: components["schemas"]["ProfileEntryInput"][];
        };
        CheckProfileVersion: {
            version_no: number;
            base_version_no: number | null;
            entries: {
                /** Format: uuid */
                component_id: string;
                language: string;
                analysis_kind: components["schemas"]["AnalysisKind"];
                /** @enum {string} */
                mode: "import" | "builtin";
                profile_id: string;
                tool_versions: {
                    [key: string]: string;
                };
            }[];
            uncovered: {
                /** Format: uuid */
                component_id: string;
                language: string;
                reason_code: string;
            }[];
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        CiConfiguration: {
            /** @enum {string} */
            ci_system: "gitlab_ci" | "github_actions" | "jenkins";
            profile_version_no: number;
            /** @description Текст фрагмента конфигурации; секретов не содержит. */
            snippet: string;
            variables: {
                name: string;
                masked: boolean;
            }[];
        };
        BaselineList: {
            data: {
                /** Format: uuid */
                component_id: string;
                language: string;
                analysis_kind: components["schemas"]["AnalysisKind"];
                /** @enum {string} */
                state: "ready" | "pending" | "failed";
                /** Format: uuid */
                report_id: string | null;
                branch: string;
                sha: string | null;
                /** Format: date-time */
                established_at: string | null;
                reason_code: string | null;
            }[];
        };
        ComponentOverview: {
            component: components["schemas"]["Component"];
            findings_by_state: {
                finding_kind: components["schemas"]["FindingKind"];
                state: components["schemas"]["FindingState"];
                count: components["schemas"]["MetricValue"];
            }[];
            /** @description Факты кода (сложность, копии, размер изменений) с источником и окном истории. */
            facts: {
                metric_id: string;
                /** @enum {string} */
                source: "builtin" | "import";
                window: components["schemas"]["Period"];
                value: components["schemas"]["MetricValue"];
            }[];
            /** @description Горячие точки M1; на M0 пустой список. */
            hotspots: {
                path: string;
                complexity: number | null;
                churn: number | null;
                /** @description Доля изменений основного автора - косвенный признак концентрации знаний. */
                authors_share: number | null;
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
            redactions: components["schemas"]["Redactions"];
        };
        /** @enum {string} */
        FindingKind: "code" | "security" | "secret" | "dependency" | "iac" | "docs" | "openapi";
        /**
         * @description `accepted_risk` не равно исправлению: находка остаётся открытой в истории (BR-012).
         * @enum {string}
         */
        FindingState: "open" | "resolved" | "accepted_risk" | "false_positive" | "not_applicable" | "newly_observed";
        Severity: {
            /** @description Исходная серьёзность анализатора. */
            native: string | null;
            /** @enum {string|null} */
            mapped: "critical" | "high" | "medium" | "low" | "info" | "unknown" | null;
            mapping_version: number | null;
        };
        Finding: {
            /** Format: uuid */
            id: string;
            finding_kind: components["schemas"]["FindingKind"];
            rule_id: string;
            state: components["schemas"]["FindingState"];
            severity: components["schemas"]["Severity"];
            /** @description `null` - уверенность неизвестна; не трактуется как высокая. */
            confidence: string | null;
            location: components["schemas"]["CodeLocation"];
            package: {
                purl: string;
                version: string | null;
            } | null;
            /** Format: uuid */
            component_id: string | null;
            /** Format: date-time */
            first_seen_at: string;
        };
        FindingPage: {
            data: components["schemas"]["Finding"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        FindingDetail: {
            /** Format: uuid */
            id: string;
            finding_kind: components["schemas"]["FindingKind"];
            rule_id: string;
            state: components["schemas"]["FindingState"];
            severity: components["schemas"]["Severity"];
            confidence: string | null;
            location: components["schemas"]["CodeLocation"];
            package: {
                purl: string;
                version: string | null;
            } | null;
            /** Format: uuid */
            component_id: string | null;
            /** Format: date-time */
            first_seen_at: string;
            /** @description Очищенное основание. Значение секрета и его фрагменты отсутствуют на всех путях (BR-017). */
            evidence: {
                [key: string]: string;
            };
            report: {
                /** Format: uuid */
                id: string;
                producer: string;
                producer_version: string;
                /** @description Профиль проверки, которым получен отчёт; `null` для отчёта клиента без профиля. */
                profile_id?: string | null;
                /** @enum {string} */
                mode?: "import" | "builtin";
                rules_version: string | null;
                db_version: string | null;
                sha: string | null;
                /** @enum {string} */
                completeness: "complete" | "partial" | "unknown";
            };
            external_triage: {
                authority: string;
                state: string;
                /** Format: uri */
                source_url: string | null;
            }[];
            exceptions: {
                /** Format: uuid */
                exception_id: string;
                /** @enum {string} */
                state: "pending" | "active" | "rejected" | "withdrawn" | "expired";
                /** Format: date-time */
                expires_at: string;
            }[];
            /** @description Действия, доступные субъекту. */
            actions: ("create_exception" | "review_exception" | "create_tech_debt_task")[];
            redactions: components["schemas"]["Redactions"];
        };
        TechnicalComparison: {
            comparable: boolean;
            incomparable_reasons: ("profile_changed" | "rules_changed" | "scope_changed" | "base_scan_missing" | "head_scan_missing" | "scan_incomplete")[];
            base: {
                sha: components["schemas"]["GitSha"];
                /** Format: uuid */
                report_id: string | null;
            };
            head: {
                sha: components["schemas"]["GitSha"];
                /** Format: uuid */
                report_id: string | null;
            };
            factors: {
                /** @enum {string} */
                factor: "code_changed" | "dependency_changed" | "rules_changed" | "db_changed" | "scope_changed" | "source_recovered" | "source_lost" | "identity_uncertain";
                new_count: number;
                resolved_count: number;
                finding_ids: string[];
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        /** @description Задана находка или правило; область не шире области автора. */
        ExceptionScope: {
            /** Format: uuid */
            area_id: string;
            /** Format: uuid */
            component_id: string | null;
            /** Format: uuid */
            finding_id: string | null;
            rule_id: string | null;
            path_prefix: string | null;
            package_purl: string | null;
        };
        ExceptionDraft: {
            /** @enum {string} */
            decision_type: "false_positive" | "accepted_risk" | "not_applicable";
            reason: string;
            scope: components["schemas"]["ExceptionScope"];
            /** Format: date-time */
            effective_from: string;
            /**
             * Format: date-time
             * @description Позже `effective_from`.
             */
            expires_at: string;
        };
        ExceptionVersion: {
            version_no: number;
            /** @enum {string} */
            decision_type: "false_positive" | "accepted_risk" | "not_applicable" | "bypass";
            reason: string;
            scope: components["schemas"]["ExceptionScope"];
            /** Format: date-time */
            effective_from: string;
            /** Format: date-time */
            expires_at: string;
            /** @description На M0 - `true` для `accepted_risk` по находкам видов security, secret, dependency и iac (BR-012). */
            approval_required: boolean;
            external_authority: string | null;
            external_ref: string | null;
            /** Format: uuid */
            author_user_id: string;
            /** @enum {string} */
            status: "pending" | "approved" | "rejected" | "withdrawn";
            /** Format: date-time */
            created_at: string;
        };
        ExceptionTransition: {
            version_no: number;
            /** @enum {string} */
            status: "pending" | "approved" | "rejected" | "withdrawn";
            /** Format: uuid */
            actor_user_id: string;
            /** Format: date-time */
            occurred_at: string;
        };
        ExceptionSummary: {
            /** Format: uuid */
            id: string;
            /**
             * @description Действие решения на текущий момент (FR-061).
             * @enum {string}
             */
            state: "pending" | "active" | "rejected" | "withdrawn" | "expired";
            current_version: components["schemas"]["ExceptionVersion"];
        };
        ExceptionDetail: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            state: "pending" | "active" | "rejected" | "withdrawn" | "expired";
            current_version: components["schemas"]["ExceptionVersion"];
            /** @description Прежние версии новыми первыми. */
            versions: components["schemas"]["ExceptionVersion"][];
            transitions: components["schemas"]["ExceptionTransition"][];
        };
        ExceptionPage: {
            data: components["schemas"]["ExceptionSummary"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        ExceptionDecision: {
            /** @enum {string} */
            decision: "approve" | "reject";
            comment?: string;
        };
        DocumentationRoot: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            area_id: string;
            path_prefix: string;
            profile_id: string;
            /**
             * Format: uuid
             * @description `null` - владелец не назначен; это пробел комплекта (FR-034).
             */
            owner_person_id: string | null;
            /** @enum {string} */
            audience: "team" | "organization" | "external";
        };
        DocumentationRootList: {
            data: components["schemas"]["DocumentationRoot"][];
        };
        DocumentationRootListInput: {
            data: {
                /** Format: uuid */
                area_id: string;
                path_prefix: string;
                profile_id: string;
                /** Format: uuid */
                owner_person_id: string | null;
                /** @enum {string} */
                audience: "team" | "organization" | "external";
            }[];
        };
        Document: {
            /** Format: uuid */
            id: string;
            title: string | null;
            /** @enum {string} */
            doc_format: "markdown" | "openapi" | "confluence";
            path: string | null;
            version_ref: string;
            /** Format: date-time */
            last_checked_at: string | null;
            open_violations: number | null;
            /** Format: uri */
            source_url: string | null;
        };
        DocumentPage: {
            documentation_state: components["schemas"]["DataState"];
            data: components["schemas"]["Document"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        DocumentationCheckRequest: {
            /** Format: uuid */
            root_id: string;
        };
        OrganizationReport: {
            period: components["schemas"]["Period"];
            /** @description Ряды каждой команды отдельно; шкалы размера команд не складываются. */
            teams: {
                team: components["schemas"]["Team"];
                /** Format: uuid */
                revision_id: string | null;
                metrics: components["schemas"]["MetricSeries"][];
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        MirrorCandidate: {
            left: components["schemas"]["ObjectRef"];
            right: components["schemas"]["ObjectRef"];
            /** @enum {string} */
            basis: "explicit_link" | "key_match" | "manual";
        };
        MirrorCandidatePage: {
            data: components["schemas"]["MirrorCandidate"][];
            next_cursor: string | null;
        };
        MirrorGroupCreate: {
            task_ids: string[];
            /** Format: uuid */
            status_authority_task_id: string;
        };
        MirrorGroup: {
            /** Format: uuid */
            id: string;
            members: string[];
            /** Format: uuid */
            status_authority_task_id: string;
            conflicts: {
                field: string;
                values: {
                    /** Format: uuid */
                    task_id: string;
                    value: string | null;
                }[];
            }[];
            /** Format: date-time */
            confirmed_at: string;
        };
        MirrorGroupPage: {
            data: components["schemas"]["MirrorGroup"][];
            next_cursor: string | null;
        };
        Sprint: {
            /** Format: uuid */
            id: string;
            name: string;
            /** @enum {string} */
            state: "future" | "active" | "closed";
            /** Format: date-time */
            started_at: string | null;
            /** Format: date-time */
            ended_at: string | null;
        };
        SprintPage: {
            data: components["schemas"]["Sprint"][];
            next_cursor: string | null;
        };
        SprintDetail: {
            /** Format: uuid */
            id: string;
            name: string;
            /** @enum {string} */
            state: "future" | "active" | "closed";
            /** Format: date-time */
            started_at: string | null;
            /** Format: date-time */
            ended_at: string | null;
            plan_state: components["schemas"]["DataState"];
            /** @description Задачи на фактический старт; снятые остаются в исходном плане. */
            initial_plan: string[];
            changes: {
                /** Format: uuid */
                task_id: string;
                /** @enum {string} */
                kind: "added" | "removed";
                /** Format: date-time */
                event_time: string;
            }[];
            completion: components["schemas"]["MetricValue"];
        };
        DodObligationInput: {
            /** @enum {string} */
            evidence_kind: "check" | "document" | "business_signoff";
            /** @description Способ подтверждения - вид отчёта, поле или статус трекера. */
            source_rule: {
                [key: string]: unknown;
            };
            required: boolean;
        };
        DodDraft: {
            task_type: string;
            obligations: components["schemas"]["DodObligationInput"][];
        };
        DodVersion: {
            version_no: number;
            base_version_no: number | null;
            task_type: string;
            mapping_version_no: number;
            obligations: {
                obligation_no: number;
                /** @enum {string} */
                evidence_kind: "check" | "document" | "business_signoff";
                source_rule: {
                    [key: string]: unknown;
                };
                required: boolean;
            }[];
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        DodVersionPage: {
            data: components["schemas"]["DodVersion"][];
            next_cursor: string | null;
        };
        TaskDod: {
            /** Format: uuid */
            task_id: string;
            /** @description `null` - DoD для типа задачи не описан (T26). */
            dod_version_no: number | null;
            verified: boolean;
            obligations: {
                obligation_no: number;
                /** @enum {string} */
                evidence_kind: "check" | "document" | "business_signoff";
                /** @enum {string} */
                state: "verified" | "missing" | "unknown";
                basis: components["schemas"]["ObjectRef"][];
                reason_code: string | null;
            }[];
        };
        InitiativeCreate: {
            name: string;
            /** Format: date-time */
            effective_at: string;
            team_ids: string[];
        };
        Initiative: {
            /** Format: uuid */
            id: string;
            name: string;
            /** Format: date-time */
            effective_at: string;
            team_ids: string[];
        };
        InitiativePage: {
            data: components["schemas"]["Initiative"][];
            next_cursor: string | null;
        };
        InitiativeComparison: {
            /** Format: uuid */
            initiative_id: string;
            metrics: {
                metric_id: string;
                comparable: boolean;
                incomparable_reasons: string[];
                before: components["schemas"]["MetricValue"];
                after: components["schemas"]["MetricValue"];
                composition_changed: boolean;
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        DeliveryChain: {
            /** Format: uuid */
            epic_id: string;
            items: {
                task: components["schemas"]["ObjectRef"];
                /** @enum {string} */
                last_confirmed_link: "task" | "merge_request" | "artifact" | "deployment";
                /** @description Отсутствующее основание, например `deployment_missing`. */
                gap_reason: string | null;
                links: components["schemas"]["Link"][];
            }[];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        CheckPolicyDraft: {
            /** @enum {string} */
            unknown_behavior: "pass" | "fail";
            bypass_authority: components["schemas"]["Role"];
            rules: {
                /** @enum {string} */
                category: "new_verified_secret" | "new_critical_vulnerability" | "new_high_finding" | "required_scan_missing" | "tests_failed";
                condition: {
                    [key: string]: unknown;
                };
            }[];
        };
        CheckPolicyVersion: {
            version_no: number;
            base_version_no: number | null;
            /** @enum {string} */
            unknown_behavior: "pass" | "fail";
            bypass_authority: components["schemas"]["Role"];
            rules: {
                rule_no: number;
                category: string;
                condition: {
                    [key: string]: unknown;
                };
            }[];
            /** Format: uuid */
            confirmed_by: string;
            /** Format: date-time */
            confirmed_at: string;
        };
        CheckPolicyPage: {
            data: components["schemas"]["CheckPolicyVersion"][];
            next_cursor: string | null;
        };
        PolicyBypassCreate: {
            /** Format: uuid */
            merge_request_id: string;
            policy_version_no: number;
            reason: string;
            /** Format: date-time */
            expires_at: string;
        };
        PolicyBypass: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            merge_request_id: string;
            policy_version_no: number;
            reason: string;
            /** Format: date-time */
            expires_at: string;
            /** Format: uuid */
            author_user_id: string;
            /** Format: date-time */
            created_at: string;
        };
        PolicyEvaluationPage: {
            data: {
                /** Format: uuid */
                merge_request_id: string;
                head_sha: components["schemas"]["GitSha"];
                policy_version_no: number;
                /** @enum {string} */
                outcome: "pass" | "fail" | "bypassed" | "unauthorized_merge" | "no_merge_gate";
                /** Format: uuid */
                bypass_id: string | null;
                /** Format: date-time */
                evaluated_at: string;
            }[];
            next_cursor: string | null;
        };
        TechDebtDraftInput: {
            finding_ids: string[];
            /** Format: uuid */
            tracker_area_id: string;
            issue_type: string;
            title: string;
            body: string;
        };
        TechDebtDraft: {
            /** Format: uuid */
            id: string;
            finding_ids: string[];
            /** Format: uuid */
            tracker_area_id: string;
            issue_type: string;
            title: string;
            body: string;
            /** @enum {string} */
            status: "draft" | "pending" | "succeeded" | "failed" | "unknown";
            external_task: components["schemas"]["ObjectRef"] | null;
            error_code: string | null;
            /** @description Задержка опроса для pending; в остальных состояниях null. */
            retry_after_seconds: number | null;
        };
        Artifact: {
            /** Format: uuid */
            id: string;
            digest: string;
            source_sha: string | null;
            /** @enum {string} */
            sbom_completeness: "complete" | "partial" | "unknown";
            /** @enum {string} */
            affected: "affected" | "not_affected" | "unknown";
            db_version: string | null;
            /** Format: date-time */
            last_assessed_at: string | null;
        };
        ArtifactPage: {
            data: components["schemas"]["Artifact"][];
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        ArtifactPassport: {
            /** Format: uuid */
            id: string;
            digest: string;
            source_sha: string | null;
            /** @enum {string} */
            sbom_completeness: "complete" | "partial" | "unknown";
            deployments: {
                /** Format: uuid */
                id: string;
                environment: string;
                /** Format: date-time */
                decided_at: string | null;
                /** Format: date-time */
                deployed_at: string | null;
                /** @enum {string} */
                outcome: "succeeded" | "failed" | "unknown";
                /**
                 * @description M2; без сведений инвентаря - `unknown`, не `internal`.
                 * @enum {string}
                 */
                exposure: "public" | "internal" | "unknown";
            }[];
            checks_before_decision: components["schemas"]["ArtifactCheck"][];
            /** @description Проверки после решения о выпуске; не влияют на прошлую готовность (BR-020). */
            later_checks: components["schemas"]["ArtifactCheck"][];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        ArtifactCheck: {
            /** Format: uuid */
            report_id: string;
            analysis_kind: components["schemas"]["AnalysisKind"];
            /** Format: date-time */
            finished_at: string | null;
        };
        DocumentationObligation: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            service_id: string;
            /** Format: uuid */
            contract_document_id: string;
            contract_version_ref: string;
            /** @enum {string} */
            status: "candidate" | "open" | "closed" | "rejected";
            documents: components["schemas"]["ObjectRef"][];
            /** Format: uuid */
            owner_person_id: string | null;
        };
        DocumentationObligationPage: {
            data: components["schemas"]["DocumentationObligation"][];
            next_cursor: string | null;
        };
        CandidateDecision: {
            /** @enum {string} */
            decision: "confirm" | "reject";
        };
        HandoverPackage: {
            /** Format: uuid */
            project_id: string;
            services: {
                service: components["schemas"]["Service"];
                /** Format: uuid */
                owner_person_id: string | null;
                gaps: string[];
            }[];
            decisions: components["schemas"]["HandoverDecision"][];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        HandoverDecisionInput: {
            /** @enum {string} */
            decision: "accept" | "request_additions";
            requested_additions: string[];
        };
        HandoverDecision: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            decision: "accept" | "request_additions";
            requested_additions: string[];
            /** Format: uuid */
            decided_by: string;
            /** Format: date-time */
            decided_at: string;
        };
        MethodRef: {
            method_id: string;
            method_version: number;
        };
        ReviewRoute: {
            /** Format: uuid */
            method_run_id: string;
            method: components["schemas"]["MethodRef"];
            places: {
                location: components["schemas"]["CodeLocation"];
                reasons: ("finding" | "hotspot" | "test_gap" | "co_change")[];
            }[];
            groups: {
                person_ids: string[];
                reasons: string[];
            }[];
            history_window: components["schemas"]["Period"] | null;
        };
        RecommendationFeedbackInput: {
            /** Format: uuid */
            method_run_id: string;
            /** @enum {string} */
            verdict: "accepted" | "rejected";
        };
        ForecastRequest: {
            /** Format: uuid */
            team_id: string;
            /** @enum {string} */
            question: "completion_date" | "count_by_date";
            /** @description Для `completion_date` - объём работы в задачах. */
            target_count: number | null;
            /**
             * Format: date-time
             * @description Для `count_by_date`.
             */
            target_date?: string;
            history: components["schemas"]["Period"];
        };
        Forecast: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            status: "pending" | "succeeded" | "refused" | "failed";
            method: components["schemas"]["MethodRef"];
            distribution: {
                probability: number;
                /**
                 * Format: date-time
                 * @description Для `completion_date`.
                 */
                date: string | null;
                /** @description Для `count_by_date`. */
                count: number | null;
            }[];
            backtest_error: number | null;
            assumptions: string[];
            refusal_reason: string | null;
            /** @description Задержка опроса для pending; в остальных состояниях null. */
            retry_after_seconds: number | null;
        };
        TestSelectionExperimentCreate: {
            /** Format: uuid */
            component_id: string;
            method_id: string;
            method_version: number;
        };
        TestSelectionExperiment: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            component_id: string;
            /** @enum {string} */
            mode: "shadow";
            runs_compared: number;
            missed_failures: components["schemas"]["MetricValue"];
            estimated_savings_seconds: number | null;
            /**
             * Format: date-time
             * @description null - теневой эксперимент активен; дата - остановлен. mode остаётся shadow, результаты сохраняются, новые рекомендации и сравнения не добавляются.
             */
            stopped_at: string | null;
        };
        RunbookDrillInput: {
            version_ref: string;
            scenario: string;
            /** Format: date-time */
            held_at: string;
            /** @enum {string} */
            outcome: "success" | "failed";
            /** @description Обязателен для `failed`. */
            stopped_at_step: number | null;
            participant_person_ids: string[];
        };
        RunbookDrill: {
            /** Format: uuid */
            id: string;
            version_ref: string;
            scenario: string;
            /** Format: date-time */
            held_at: string;
            /** @enum {string} */
            outcome: "success" | "failed";
            stopped_at_step: number | null;
            participant_person_ids: string[];
        };
        RunbookDrillPage: {
            data: components["schemas"]["RunbookDrill"][];
            next_cursor: string | null;
        };
        SearchResult: {
            /** Format: uuid */
            query_id: string;
            data: {
                document: components["schemas"]["ObjectRef"];
                excerpt: string;
            }[];
        };
        SearchFeedbackInput: {
            /** Format: uuid */
            document_id: string;
            /** @enum {string} */
            verdict: "helped" | "not_helped";
        };
        SurveyCreate: {
            /** Format: uuid */
            team_id: string;
            question_set_version: number;
            /** Format: date-time */
            opens_at: string;
            /** Format: date-time */
            closes_at: string;
        };
        Survey: {
            /** Format: uuid */
            id: string;
            /** Format: uuid */
            team_id: string;
            question_set_version: number;
            /** Format: date-time */
            opens_at: string;
            /** Format: date-time */
            closes_at: string;
            /** @enum {string} */
            status: "scheduled" | "open" | "closed";
            /** @description Есть только у закрытого опроса с числом ответов не ниже порога. */
            summary: {
                [key: string]: number;
            } | null;
            published: boolean;
        };
        SurveyForm: {
            /** Format: uuid */
            survey_id: string;
            /** Format: date-time */
            closes_at: string;
            questions: {
                question_id: string;
                text: string;
                scale_min: number;
                scale_max: number;
            }[];
            already_answered: boolean;
        };
        SurveyResponseInput: {
            /** @description Пропущенный вопрос не передаётся и не считается нулём. */
            answers: {
                question_id: string;
                value: number;
            }[];
        };
        DeliveryStability: {
            /** Format: uuid */
            service_id: string;
            period: components["schemas"]["Period"];
            change_failure_rate: components["schemas"]["MetricValue"];
            recovery_p50_seconds: components["schemas"]["MetricValue"];
            incident_link_candidates: components["schemas"]["DeploymentIncidentLink"][];
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
            /** @description p85 восстановления по завершённым подтверждённым сбоям развёртываний той же доступной когорты; единица seconds, n и low_sample обязательны внутри MetricValue. */
            recovery_p85_seconds: components["schemas"]["MetricValue"];
            /** @description I41: доля внеплановых production-развёртываний из-за подтверждённого инцидента. numerator - подтверждённые внеплановые развёртывания, denominator - все разрешённые production-развёртывания периода. Hotfix без связи - кандидат; неизвестная история не точный ноль. */
            unplanned_deployment_rate: components["schemas"]["MetricValue"];
            /**
             * Format: uuid
             * @description Закреплённая ревизия показателей и оснований; getRevision/listRevisionCohort раскрывают I39/I40/I41 того же периода.
             */
            revision_id: string;
            confirmed_links: components["schemas"]["DeploymentIncidentPage"];
        };
        /** @description Обе безопасные стороны связи и направление причинности для подтверждения или раскрытия. Выдаётся только при доступе к обоим концам. Кандидат не входит в числитель; подтверждённые caused_failure дают I39/I40, response_to_incident даёт I41. Несколько инцидентов одного deployment не удваивают deployment в числителе. */
        DeploymentIncidentLink: {
            deployment: components["schemas"]["ObjectRef"];
            /** Format: uuid */
            artifact_id: string | null;
            incident: components["schemas"]["ObjectRef"];
            /** Format: uuid */
            link_id: string;
            /**
             * @description caused_failure соответствует Link.link_type=deployment_incident; response_to_incident соответствует incident_deployment. Тип подтверждается вместе с обоими концами; не выводится только из близости времени.
             * @enum {string}
             */
            relation: "caused_failure" | "response_to_incident";
            /**
             * Format: date-time
             * @description Начало связанного инцидента; null если неизвестно.
             */
            started_at: string | null;
            /**
             * Format: date-time
             * @description Подтверждённое восстановление; null для открытого/неизвестного исхода.
             */
            restored_at: string | null;
        };
        DeploymentIncidentPage: {
            /** @description Только подтверждённые связи с доступом к обоим концам; кандидаты находятся отдельно. */
            data: components["schemas"]["DeploymentIncidentLink"][];
            next_cursor: string | null;
        };
        Money: {
            amount: string;
            currency: string;
        };
        CiCost: {
            /** Format: uuid */
            project_id: string;
            period: components["schemas"]["Period"];
            usage: {
                runner_class: string;
                job_seconds: components["schemas"]["MetricValue"];
                /** @description Снимок ставки для читателя отчёта; null при неизвестной ставке. Версия совпадает с estimate.rate_version_no и основанием ci_rate. */
                rate: components["schemas"]["CiRate"] | null;
                /** @description `null` - ставки нет; стоимость неизвестна. */
                estimate: {
                    amount: string;
                    currency: string;
                    rate_version_no: number;
                } | null;
                /**
                 * Format: uuid
                 * @description Ревизия когорты I18 именно этой строки, класса runner и версии ставки. getRevision содержит input_kind=ci_rate с идентификатором класса и версией; при отсутствии ставки этого input нет. listRevisionCohort(metric_id=I18,point_start) возвращает разрешённые attempt-основания; value - вклад в job_seconds. Одна строка не смешивает разные версии ставок.
                 */
                basis_revision_id: string;
                /**
                 * Format: date-time
                 * @description Точка I18 для listRevisionCohort; окно и ставка закреплены basis_revision_id.
                 */
                point_start: string;
            }[];
            fixed_fees: components["schemas"]["Money"][];
            invoices: {
                invoice_ref: string;
                amount: string;
                currency: string;
            }[];
            /**
             * Format: uuid
             * @description Закреплённый расчёт стоимости; новая ставка/поздние attempts создают новую ревизию.
             */
            revision_id: string;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        CiRateInput: {
            runner_class: string;
            /**
             * @description `period` - фиксированный платёж за тарифный период.
             * @enum {string}
             */
            unit: "minute" | "hour" | "period";
            amount: string;
            currency: string;
            /** Format: date-time */
            valid_from: string;
            /** Format: date-time */
            valid_to: string | null;
        };
        CiRate: {
            version_no: number;
            runner_class: string;
            /** @enum {string} */
            unit: "minute" | "hour" | "period";
            amount: string;
            currency: string;
            /** Format: date-time */
            valid_from: string;
            /** Format: date-time */
            valid_to: string | null;
        };
        CiRatePage: {
            data: components["schemas"]["CiRate"][];
            next_cursor: string | null;
        };
        CiInvoiceInput: {
            invoice_ref: string;
            /** Format: date-time */
            period_from: string;
            /** Format: date-time */
            period_to: string;
            amount: string;
            currency: string;
        };
        CiInvoice: {
            /** Format: uuid */
            id: string;
            invoice_ref: string;
            /** Format: date-time */
            period_from: string;
            /** Format: date-time */
            period_to: string;
            amount: string;
            currency: string;
        };
        TransferPage: {
            data: {
                task: components["schemas"]["ObjectRef"];
                /** Format: uuid */
                from_team_id: string | null;
                /**
                 * Format: uuid
                 * @description `null` - принимающая команда вне области пользователя.
                 */
                to_team_id: string | null;
                /** Format: date-time */
                effective_at: string;
                wait_seconds: number | null;
            }[];
            /** Format: date-time */
            history_from: string | null;
            next_cursor: string | null;
            access_limited: boolean;
            access_notes: components["schemas"]["AccessNote"][];
        };
        /** @description Публичный дескриптор запуска, отдельный от задания очереди. queued/running: retry_after_seconds >= 1, result_uri и error равны null. succeeded: result_uri указывает на зафиксированный результат, error и retry_after_seconds равны null. failed: error заполнена, result_uri и retry_after_seconds равны null. Для sync_run результат - sync-status исходного подключения, для series_refresh - точная новая ревизия, для documentation_check - документы проверенного сервиса; пустые findings сами по себе не означают успешную проверку. */
        BackgroundOperation: {
            /** Format: uuid */
            id: string;
            /** @enum {string} */
            kind: "sync_run" | "series_refresh" | "documentation_check";
            /** @enum {string} */
            status: "queued" | "running" | "succeeded" | "failed";
            /** Format: date-time */
            created_at: string;
            /** Format: date-time */
            updated_at: string;
            /** Format: uri-reference */
            result_uri: string | null;
            error: {
                /** @description Безопасный класс причины: source_unavailable, access_lost, calculation_failed или check_failed; неизвестный код клиент показывает как ошибку операции. */
                code: string;
                /** Format: uuid */
                correlation_id: string;
            } | null;
            retry_after_seconds: number | null;
        };
    };
    responses: {
        /** @description Запрос не разобран или содержит неизвестный параметр. */
        BadRequest: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:unknown_parameter",
                 *       "title": "Unknown query parameter",
                 *       "status": 400,
                 *       "code": "unknown_parameter",
                 *       "detail": "Parameter \"sorting\" is not supported; supported: sort, limit, cursor",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b11"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Нет действующей сессии, ключа или токена. Данных в ответе нет. */
        Unauthorized: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:auth_required",
                 *       "title": "Authentication required",
                 *       "status": 401,
                 *       "code": "auth_required",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b12"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /**
         * @description Субъект видит область, но его роль не содержит действия (`forbidden`), или изменяющий
         *     запрос пришёл с чужим `Origin` (`origin_rejected`). Если область субъекту не видна,
         *     ответ - `404 not_found`.
         */
        Forbidden: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:forbidden",
                 *       "title": "Action is not allowed",
                 *       "status": 403,
                 *       "code": "forbidden",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b13"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Объект не существует, принадлежит другой организации или закрыт для субъекта. Ответ одинаков во всех трёх случаях. */
        NotFound: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:not_found",
                 *       "title": "Not found",
                 *       "status": 404,
                 *       "code": "not_found",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b14"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /**
         * @description Операция невозможна в текущем состоянии ресурса (`state_conflict`), у подключения нет
         *     нужной проверенной возможности (`capability_missing`) или сводка MR относится к
         *     устаревшему head (`stale_head`). Повтор того же запроса не поможет.
         */
        Conflict: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:capability_missing",
                 *       "title": "Connection capability is missing",
                 *       "status": 409,
                 *       "code": "capability_missing",
                 *       "detail": "Connection has no verified capability write_tracker_issue",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b15"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Версия в `If-Match` устарела. Перечитайте ресурс, покажите пользователю актуальную версию и повторите (FR-008). */
        PreconditionFailed: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:version_conflict",
                 *       "title": "Resource version has changed",
                 *       "status": 412,
                 *       "code": "version_conflict",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b16"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Для изменения версионируемого ресурса нужен заголовок `If-Match`. */
        PreconditionRequired: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:precondition_required",
                 *       "title": "If-Match header is required",
                 *       "status": 428,
                 *       "code": "precondition_required",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b17"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Тело больше предела операции (NFR-015). */
        ContentTooLarge: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:payload_too_large",
                 *       "title": "Payload is too large",
                 *       "status": 413,
                 *       "code": "payload_too_large",
                 *       "detail": "Package exceeds 26214400 bytes",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b18"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Неподдержанный `Content-Type` тела. */
        UnsupportedMediaType: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:unsupported_media_type",
                 *       "title": "Unsupported media type",
                 *       "status": 415,
                 *       "code": "unsupported_media_type",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b19"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /**
         * @description Данные разобраны, но не проходят по смыслу. Все ошибки полей возвращаются одним
         *     ответом (`validation_failed`). Тот же `Idempotency-Key` с другим телом -
         *     `idempotency_key_reused`.
         */
        ValidationFailed: {
            headers: {
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:validation_failed",
                 *       "title": "Validation failed",
                 *       "status": 422,
                 *       "code": "validation_failed",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b1a",
                 *       "errors": [
                 *         {
                 *           "field": "/reason",
                 *           "code": "required",
                 *           "detail": "Reason must not be empty"
                 *         },
                 *         {
                 *           "field": "/expires_at",
                 *           "code": "out_of_range",
                 *           "detail": "Must be later than effective_from"
                 *         }
                 *       ]
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Превышен лимит частоты на границе (адреса входа, webhook, загрузка пакетов). Повторить через `Retry-After`. */
        TooManyRequests: {
            headers: {
                "Retry-After": components["headers"]["RetryAfter"];
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:rate_limited",
                 *       "title": "Too many requests",
                 *       "status": 429,
                 *       "code": "rate_limited",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b1b"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
        /** @description Хранилище объектов или база недоступны; запрос не выполнен. Повторить через `Retry-After` (раздел 10.7). */
        ServiceUnavailable: {
            headers: {
                "Retry-After": components["headers"]["RetryAfter"];
                [name: string]: unknown;
            };
            content: {
                /**
                 * @example {
                 *       "type": "urn:craftpulse:problem:service_unavailable",
                 *       "title": "Service temporarily unavailable",
                 *       "status": 503,
                 *       "code": "service_unavailable",
                 *       "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b1c"
                 *     }
                 */
                "application/problem+json": components["schemas"]["Problem"];
            };
        };
    };
    parameters: {
        /** @description Размер страницы. */
        Limit: number;
        /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
        Cursor: string;
        /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
        IdempotencyKey: string;
        /** @description Значение `ETag` прочитанной версии ресурса. */
        IfMatch: string;
        /** @description Начало периода включительно, RFC 3339. */
        From: string;
        /** @description Конец периода исключительно, RFC 3339 (BR-006). */
        To: string;
        /** @description Дата состояния расчёта. По умолчанию - конец периода или текущий момент. */
        AsOf: string;
        /** @description Шаг ряда. */
        Step: "day" | "week" | "month";
        /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
        RevisionQuery: string;
        TeamId: string;
        ProjectId: string;
        ServiceId: string;
        ComponentId: string;
        UserId: string;
        ApiKeyId: string;
        UploadTokenId: string;
        ConnectionId: string;
        ProbeId: string;
        ReceiptId: string;
        ConflictId: string;
        SeriesId: string;
        RevisionId: string;
        SavedViewId: string;
        ExportId: string;
        TaskId: string;
        LinkId: string;
        MergeRequestId: string;
        PipelineId: string;
        FindingId: string;
        ExceptionId: string;
        DocumentId: string;
    };
    requestBodies: never;
    headers: {
        /** @description Версия ресурса для `If-Match`. */
        ETag: string;
        /** @description Адрес созданного ресурса или ресурса-операции. */
        Location: string;
        /** @description Через сколько секунд повторить запрос. */
        RetryAfter: number;
    };
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    startLogin: {
        parameters: {
            query?: {
                /** @description Относительный путь сайта. После декодирования и нормализации запрещены //, обратная косая черта и управляющие символы; итоговый origin должен совпадать с origin установки. */
                return_to?: string;
            };
            header?: never;
            path: {
                org_slug: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Переход к провайдеру. Устанавливается cookie `__Host-cp_login`. */
            302: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content?: never;
            };
            429: components["responses"]["TooManyRequests"];
        };
    };
    completeLogin: {
        parameters: {
            query: {
                code?: string;
                state: string;
                iss?: string;
                error?: string;
            };
            header?: never;
            path: {
                org_slug: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /**
             * @description Успех - переход на сохранённый путь с cookie `__Host-cp_session`. Отказ - переход на
             *     страницу входа с `login_error` одним из `provider_unavailable`, `provider_rejected`,
             *     `invalid_response`, `organization_not_configured`, `user_blocked`.
             */
            302: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content?: never;
            };
            429: components["responses"]["TooManyRequests"];
        };
    };
    logout: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Сессия завершена. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    getCurrentUser: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Текущий пользователь. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "user": {
                     *         "id": "0193a5b0-1111-7000-8000-000000000001",
                     *         "display_name": "Анна Смирнова",
                     *         "status": "active",
                     *         "person_id": "0193a5b0-2222-7000-8000-000000000001"
                     *       },
                     *       "organization": {
                     *         "id": "0193a5b0-0000-7000-8000-000000000001",
                     *         "name": "Пример",
                     *         "slug": "example",
                     *         "status": "active"
                     *       },
                     *       "roles": [
                     *         {
                     *           "role": "dev_lead",
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": null,
                     *           "service_id": null,
                     *           "assigned_at": "2026-10-01T09:00:00Z"
                     *         }
                     *       ],
                     *       "areas": {
                     *         "teams": [
                     *           {
                     *             "id": "0193a5b0-3333-7000-8000-000000000001",
                     *             "name": "Платформа",
                     *             "status": "active"
                     *           }
                     *         ],
                     *         "projects": [
                     *           {
                     *             "id": "0193a5b0-4444-7000-8000-000000000001",
                     *             "name": "Core",
                     *             "status": "active"
                     *           }
                     *         ],
                     *         "services": []
                     *       },
                     *       "actions": [
                     *         "session.read",
                     *         "report.read",
                     *         "tasks.read",
                     *         "export.create"
                     *       ],
                     *       "session": {
                     *         "expires_at": "2026-11-08T21:00:00Z",
                     *         "idle_expires_at": "2026-11-08T10:00:00Z"
                     *       },
                     *       "review_participation": {
                     *         "participates": false
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["CurrentUser"];
                };
            };
            401: components["responses"]["Unauthorized"];
        };
    };
    setReviewParticipation: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "participates": false
                 *     }
                 */
                "application/json": components["schemas"]["ReviewParticipation"];
            };
        };
        responses: {
            /** @description Сохранённая настройка. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "participates": false
                     *     }
                     */
                    "application/json": components["schemas"]["ReviewParticipation"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listSourceLinks: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Подключения и состояние связи. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "connection_id": "0193a5b0-5555-7000-8000-000000000003",
                     *           "provider": "confluence_cloud",
                     *           "display_name": "Confluence",
                     *           "state": "not_linked",
                     *           "linked_at": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["SourceLinkList"];
                };
            };
            401: components["responses"]["Unauthorized"];
        };
    };
    startSourceLink: {
        parameters: {
            query?: {
                /** @description Относительный путь сайта для возврата после связывания; правила те же, что у `startLogin`. */
                return_to?: string;
            };
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Адрес авторизации. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "authorization_url": "https://auth.atlassian.com/authorize?audience=api.atlassian.com&client_id=example-client-id&response_type=code&scope=read%3Aspace%3Aconfluence%20read%3Apage%3Aconfluence%20read%3Acontent-details%3Aconfluence%20offline_access&redirect_uri=https%3A%2F%2Fcraftpulse.example%2Fsource-links%2Fcallback&state=example-random-user-bound-state&prompt=consent"
                     *     }
                     */
                    "application/json": components["schemas"]["AuthorizationRedirect"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    deleteSourceLink: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Доступ отозван или не был выдан. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    completeSourceLink: {
        parameters: {
            query: {
                code?: string;
                state: string;
                error?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Переход на `return_to` или UI-73 с исходом `linked` или `link_failed`. */
            302: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
        };
    };
    getOrganizationStructure: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Структура и её версия. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version": 7,
                     *       "teams": [
                     *         {
                     *           "id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "name": "Платформа",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "projects": [
                     *         {
                     *           "id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "name": "Core",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "services": [
                     *         {
                     *           "id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "name": "billing",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "components": [
                     *         {
                     *           "id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "repository_area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "path_prefix": "services/billing",
                     *           "status": "proposed",
                     *           "origin": "manifest"
                     *         }
                     *       ],
                     *       "team_projects": [
                     *         {
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ],
                     *       "project_areas": [
                     *         {
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ],
                     *       "team_members": [
                     *         {
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "person_id": "0193a5b0-2222-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["OrganizationStructure"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    replaceOrganizationStructure: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "teams": [
                 *         {
                 *           "id": "0193a5b0-3333-7000-8000-000000000001",
                 *           "name": "Платформа",
                 *           "status": "active"
                 *         }
                 *       ],
                 *       "projects": [
                 *         {
                 *           "id": "0193a5b0-4444-7000-8000-000000000001",
                 *           "name": "Core",
                 *           "status": "active"
                 *         }
                 *       ],
                 *       "services": [
                 *         {
                 *           "id": "0193a5b0-6666-7000-8000-000000000001",
                 *           "name": "billing",
                 *           "status": "active"
                 *         }
                 *       ],
                 *       "components": [
                 *         {
                 *           "id": "0193a5b0-7777-7000-8000-000000000001",
                 *           "service_id": "0193a5b0-6666-7000-8000-000000000001",
                 *           "repository_area_id": "0193a5b0-8888-7000-8000-000000000001",
                 *           "path_prefix": "services/billing",
                 *           "status": "confirmed"
                 *         }
                 *       ],
                 *       "team_projects": [
                 *         {
                 *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                 *           "valid_from": null,
                 *           "valid_to": null
                 *         }
                 *       ],
                 *       "project_areas": [
                 *         {
                 *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                 *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                 *           "valid_from": null,
                 *           "valid_to": null
                 *         }
                 *       ],
                 *       "team_members": [
                 *         {
                 *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *           "person_id": "0193a5b0-2222-7000-8000-000000000001",
                 *           "valid_from": null,
                 *           "valid_to": null
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["OrganizationStructureInput"];
            };
        };
        responses: {
            /** @description Сохранённая структура и новая версия. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version": 8,
                     *       "teams": [
                     *         {
                     *           "id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "name": "Платформа",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "projects": [
                     *         {
                     *           "id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "name": "Core",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "services": [
                     *         {
                     *           "id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "name": "billing",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "components": [
                     *         {
                     *           "id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "repository_area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "path_prefix": "services/billing",
                     *           "status": "confirmed",
                     *           "origin": "manifest"
                     *         }
                     *       ],
                     *       "team_projects": [
                     *         {
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ],
                     *       "project_areas": [
                     *         {
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ],
                     *       "team_members": [
                     *         {
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "person_id": "0193a5b0-2222-7000-8000-000000000001",
                     *           "valid_from": "2026-10-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["OrganizationStructure"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listTeams: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                sort?: "name" | "-name";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница команд. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "name": "Платформа",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["TeamPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    listProjects: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                team_id?: string;
                sort?: "name" | "-name";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница проектов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "name": "Core",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["ProjectPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    listServices: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                sort?: "name" | "-name";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница сервисов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "name": "billing",
                     *           "status": "active"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["ServicePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    listAreas: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                kind?: "tracker_project" | "repository" | "wiki_space" | "ci_project" | "analysis_project";
                connection_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница областей. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "kind": "repository",
                     *           "display_name": "platform/core",
                     *           "connection_id": "0193a5b0-5555-7000-8000-000000000002"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["AreaPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listUsers: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                status?: "active" | "blocked";
                sort?: "display_name" | "-display_name" | "created_at" | "-created_at";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница пользователей. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-1111-7000-8000-000000000001",
                     *           "display_name": "Анна Смирнова",
                     *           "status": "active",
                     *           "person_id": "0193a5b0-2222-7000-8000-000000000001"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["UserPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listUserRoles: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                user_id: components["parameters"]["UserId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Роли пользователя. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "role": "dev_lead",
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": null,
                     *           "service_id": null,
                     *           "assigned_at": "2026-10-01T09:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["RoleAssignmentList"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    replaceUserRoles: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                user_id: components["parameters"]["UserId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "data": [
                 *         {
                 *           "role": "dev_lead",
                 *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *           "project_id": null,
                 *           "service_id": null
                 *         },
                 *         {
                 *           "role": "security_access",
                 *           "team_id": null,
                 *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                 *           "service_id": null
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["RoleAssignmentListInput"];
            };
        };
        responses: {
            /** @description Сохранённые роли. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "role": "dev_lead",
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": null,
                     *           "service_id": null,
                     *           "assigned_at": "2026-10-01T09:00:00Z"
                     *         },
                     *         {
                     *           "role": "security_access",
                     *           "team_id": null,
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "service_id": null,
                     *           "assigned_at": "2026-11-08T10:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["RoleAssignmentList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    blockUser: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                user_id: components["parameters"]["UserId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Пользователь заблокирован. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    unblockUser: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                user_id: components["parameters"]["UserId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Пользователь разблокирован. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    endUserSessions: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                user_id: components["parameters"]["UserId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Сессии завершены. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listPersons: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                kind?: "human" | "bot" | "system" | "unknown";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница людей. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-2222-7000-8000-000000000001",
                     *           "display_name": "Анна Смирнова",
                     *           "kind": "human"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["PersonPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listSourceAccounts: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                /** @description Фильтр по наличию подтверждённой привязки. */
                assignment?: "assigned" | "unassigned";
                instance_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница учётных записей. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-9999-7000-8000-000000000001",
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "provider": "jira_cloud",
                     *           "native_account_id": "JIRAUSER10021",
                     *           "login": "asmirnova",
                     *           "display_name": "Анна Смирнова",
                     *           "proposed_kind": "human",
                     *           "assignment": null,
                     *           "proposals": [
                     *             {
                     *               "person_id": "0193a5b0-2222-7000-8000-000000000001",
                     *               "basis": "login_match"
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["SourceAccountPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    createAccountAssignments: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "items": [
                 *         {
                 *           "source_account_id": "0193a5b0-9999-7000-8000-000000000001",
                 *           "person_id": "0193a5b0-2222-7000-8000-000000000001",
                 *           "actor_kind": "human",
                 *           "base_assignment_id": null
                 *         },
                 *         {
                 *           "source_account_id": "0193a5b0-9999-7000-8000-000000000002",
                 *           "person_id": null,
                 *           "actor_kind": "bot",
                 *           "base_assignment_id": null
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["AccountAssignmentBatch"];
            };
        };
        responses: {
            /** @description Исход каждого элемента. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "items": [
                     *         {
                     *           "source_account_id": "0193a5b0-9999-7000-8000-000000000001",
                     *           "outcome": "saved",
                     *           "assignment_id": "0193a5b0-bbbb-7000-8000-000000000001",
                     *           "error_code": null
                     *         },
                     *         {
                     *           "source_account_id": "0193a5b0-9999-7000-8000-000000000002",
                     *           "outcome": "saved",
                     *           "assignment_id": "0193a5b0-bbbb-7000-8000-000000000002",
                     *           "error_code": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["AccountAssignmentBatchResult"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listApiKeys: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница ключей. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-cccc-7000-8000-000000000001",
                     *           "owner_user_id": "0193a5b0-1111-7000-8000-000000000001",
                     *           "grants": [
                     *             {
                     *               "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *               "project_id": null,
                     *               "service_id": null
                     *             }
                     *           ],
                     *           "created_at": "2026-11-01T10:00:00Z",
                     *           "expires_at": "2027-11-01T10:00:00Z",
                     *           "revoked_at": null
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["ApiKeyPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    createApiKey: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "owner_user_id": "0193a5b0-1111-7000-8000-000000000001",
                 *       "grants": [
                 *         {
                 *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *           "project_id": null,
                 *           "service_id": null
                 *         }
                 *       ],
                 *       "expires_at": "2027-11-01T10:00:00Z"
                 *     }
                 */
                "application/json": components["schemas"]["ApiKeyCreate"];
            };
        };
        responses: {
            /** @description Ключ выпущен. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-cccc-7000-8000-000000000002",
                     *       "owner_user_id": "0193a5b0-1111-7000-8000-000000000001",
                     *       "grants": [
                     *         {
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "project_id": null,
                     *           "service_id": null
                     *         }
                     *       ],
                     *       "created_at": "2026-11-08T10:00:00Z",
                     *       "expires_at": "2027-11-01T10:00:00Z",
                     *       "revoked_at": null,
                     *       "value": "cpk_EXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEX"
                     *     }
                     */
                    "application/json": components["schemas"]["ApiKeyCreated"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    revokeApiKey: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                api_key_id: components["parameters"]["ApiKeyId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Ключ отозван. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listUploadTokens: {
        parameters: {
            query: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                project_id: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница токенов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-dddd-7000-8000-000000000001",
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "slot": 1,
                     *           "created_at": "2026-10-05T10:00:00Z",
                     *           "revoked_at": null
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["UploadTokenPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createUploadToken: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "project_id": "0193a5b0-4444-7000-8000-000000000001"
                 *     }
                 */
                "application/json": components["schemas"]["UploadTokenCreate"];
            };
        };
        responses: {
            /** @description Токен выпущен. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-dddd-7000-8000-000000000002",
                     *       "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *       "slot": 2,
                     *       "created_at": "2026-11-08T10:00:00Z",
                     *       "revoked_at": null,
                     *       "value": "cpt_EXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEX"
                     *     }
                     */
                    "application/json": components["schemas"]["UploadTokenCreated"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    revokeUploadToken: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                upload_token_id: components["parameters"]["UploadTokenId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Токен отозван. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getLatestInstallationProtocol: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Протокол. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-eeee-7000-8000-000000000001",
                     *       "release_sha": "3f9c2a7b1d4e5f60718293a4b5c6d7e8f9012345",
                     *       "started_at": "2026-11-08T08:00:00Z",
                     *       "finished_at": "2026-11-08T08:01:10Z",
                     *       "outcome": "passed",
                     *       "checks": [
                     *         {
                     *           "check_kind": "identity_provider",
                     *           "outcome": "passed",
                     *           "reason_code": null
                     *         },
                     *         {
                     *           "check_kind": "update_source",
                     *           "outcome": "passed",
                     *           "reason_code": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["InstallationProtocol"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listAuditEvents: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                actor_id?: string;
                outcome?: "succeeded" | "denied" | "failed";
                occurred_after?: string;
                occurred_before?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница записей, новые первыми. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-ffff-7000-8000-000000000001",
                     *           "occurred_at": "2026-11-08T10:05:00Z",
                     *           "actor_kind": "user",
                     *           "actor_id": "0193a5b0-1111-7000-8000-000000000002",
                     *           "operation": "getTask",
                     *           "object_kind": "task",
                     *           "object_id": "0193a5b0-f000-7000-8000-000000000009",
                     *           "outcome": "denied",
                     *           "reason_code": "outside_scope",
                     *           "base_version": null,
                     *           "new_version": null,
                     *           "correlation_id": "0193a5b0-7c2e-7d41-8a10-5e2f0c9d4b14"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["AuditEventPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listAdapters: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Адаптеры. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "provider": "jira_cloud",
                     *           "display_name": "Jira Cloud",
                     *           "editions": [
                     *             "cloud"
                     *           ],
                     *           "contract_versions": [
                     *             "1"
                     *           ],
                     *           "data_kinds": [
                     *             "tasks",
                     *             "accounts",
                     *             "permissions"
                     *           ],
                     *           "scope_kinds": [
                     *             "tracker_project"
                     *           ],
                     *           "credential_fields": [
                     *             {
                     *               "name": "email",
                     *               "label": "Email служебной учётной записи Atlassian",
                     *               "required": true,
                     *               "secret": false
                     *             },
                     *             {
                     *               "name": "token",
                     *               "label": "API token служебной учётной записи Atlassian",
                     *               "required": true,
                     *               "secret": true
                     *             }
                     *           ]
                     *         },
                     *         {
                     *           "provider": "gitlab",
                     *           "display_name": "GitLab self-managed",
                     *           "editions": [
                     *             "ce",
                     *             "ee"
                     *           ],
                     *           "contract_versions": [
                     *             "1"
                     *           ],
                     *           "data_kinds": [
                     *             "merge_requests",
                     *             "ci",
                     *             "reports",
                     *             "accounts",
                     *             "permissions"
                     *           ],
                     *           "scope_kinds": [
                     *             "repository"
                     *           ],
                     *           "credential_fields": [
                     *             {
                     *               "name": "read_token",
                     *               "label": "Токен чтения API и репозитория",
                     *               "required": true,
                     *               "secret": true
                     *             },
                     *             {
                     *               "name": "write_token",
                     *               "label": "Токен бота для комментария MR",
                     *               "required": false,
                     *               "secret": true
                     *             },
                     *             {
                     *               "name": "webhook_secret",
                     *               "label": "Секрет подписи webhook",
                     *               "required": false,
                     *               "secret": true
                     *             }
                     *           ]
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["AdapterList"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listConnections: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                status?: "draft" | "active" | "access_lost" | "deleted";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница подключений. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-5555-7000-8000-000000000001",
                     *           "name": "Jira основной",
                     *           "provider": "jira_cloud",
                     *           "instance": {
                     *             "id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *             "base_url": "https://example.atlassian.net",
                     *             "edition": "cloud",
                     *             "source_version": "1001.0.0-SNAPSHOT"
                     *           },
                     *           "status": "active",
                     *           "adapter_kind": "builtin",
                     *           "access_mode": "service_account",
                     *           "credentials_set_at": "2026-10-01T09:00:00Z",
                     *           "sync_paused": false,
                     *           "paused_until": null,
                     *           "last_success_at": "2026-11-08T03:12:00Z",
                     *           "scopes": [
                     *             {
                     *               "id": "0193a5b0-5656-7000-8000-000000000001",
                     *               "kind": "tracker_project",
                     *               "native_scope_id": "10200",
                     *               "display_name": "CORE",
                     *               "area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *               "history_from": "2026-01-01T00:00:00Z"
                     *             }
                     *           ],
                     *           "capabilities": [
                     *             {
                     *               "capability": "read_current",
                     *               "state": "verified",
                     *               "reason_code": null,
                     *               "checked_at": "2026-10-01T09:05:00Z"
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    createConnection: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "name": "Jira основной",
                 *       "provider": "jira_cloud",
                 *       "base_url": "https://example.atlassian.net",
                 *       "adapter_kind": "builtin",
                 *       "adapter_url": null,
                 *       "adapter_key": null,
                 *       "credentials": {
                 *         "email": "svc-craftpulse@example.org",
                 *         "token": "example-token-not-a-secret"
                 *       },
                 *       "scopes": [
                 *         {
                 *           "kind": "tracker_project",
                 *           "native_scope_id": "10200",
                 *           "history_from": "2026-01-01T00:00:00Z"
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {
            /** @description Черновик подключения. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5555-7000-8000-000000000001",
                     *       "name": "Jira основной",
                     *       "provider": "jira_cloud",
                     *       "instance": {
                     *         "id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *         "base_url": "https://example.atlassian.net",
                     *         "edition": null,
                     *         "source_version": null
                     *       },
                     *       "status": "draft",
                     *       "adapter_kind": "builtin",
                     *       "access_mode": "service_account",
                     *       "credentials_set_at": "2026-11-08T10:00:00Z",
                     *       "sync_paused": false,
                     *       "paused_until": null,
                     *       "last_success_at": null,
                     *       "scopes": [
                     *         {
                     *           "id": "0193a5b0-5656-7000-8000-000000000001",
                     *           "kind": "tracker_project",
                     *           "native_scope_id": "10200",
                     *           "display_name": "10200",
                     *           "area_id": null,
                     *           "history_from": "2026-01-01T00:00:00Z"
                     *         }
                     *       ],
                     *       "capabilities": []
                     *     }
                     */
                    "application/json": components["schemas"]["Connection"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getConnection: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Подключение. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5555-7000-8000-000000000001",
                     *       "name": "Jira основной",
                     *       "provider": "jira_cloud",
                     *       "instance": {
                     *         "id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *         "base_url": "https://example.atlassian.net",
                     *         "edition": "cloud",
                     *         "source_version": "1001.0.0-SNAPSHOT"
                     *       },
                     *       "status": "active",
                     *       "adapter_kind": "builtin",
                     *       "access_mode": "service_account",
                     *       "credentials_set_at": "2026-10-01T09:00:00Z",
                     *       "sync_paused": false,
                     *       "paused_until": null,
                     *       "last_success_at": "2026-11-08T03:12:00Z",
                     *       "scopes": [
                     *         {
                     *           "id": "0193a5b0-5656-7000-8000-000000000001",
                     *           "kind": "tracker_project",
                     *           "native_scope_id": "10200",
                     *           "display_name": "CORE",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *           "history_from": "2026-01-01T00:00:00Z"
                     *         }
                     *       ],
                     *       "capabilities": [
                     *         {
                     *           "capability": "read_current",
                     *           "state": "verified",
                     *           "reason_code": null,
                     *           "checked_at": "2026-10-01T09:05:00Z"
                     *         },
                     *         {
                     *           "capability": "write_tracker_issue",
                     *           "state": "missing",
                     *           "reason_code": "permission_missing",
                     *           "checked_at": "2026-10-01T09:05:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["Connection"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    deleteConnection: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Подключение удалено. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    updateConnection: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "sync_paused": true
                 *     }
                 */
                "application/merge-patch+json": components["schemas"]["ConnectionPatch"];
            };
        };
        responses: {
            /** @description Изменённое подключение. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5555-7000-8000-000000000001",
                     *       "name": "Jira основной",
                     *       "provider": "jira_cloud",
                     *       "instance": {
                     *         "id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *         "base_url": "https://example.atlassian.net",
                     *         "edition": "cloud",
                     *         "source_version": "1001.0.0-SNAPSHOT"
                     *       },
                     *       "status": "active",
                     *       "adapter_kind": "builtin",
                     *       "access_mode": "service_account",
                     *       "credentials_set_at": "2026-10-01T09:00:00Z",
                     *       "sync_paused": true,
                     *       "paused_until": null,
                     *       "last_success_at": "2026-11-08T03:12:00Z",
                     *       "scopes": [
                     *         {
                     *           "id": "0193a5b0-5656-7000-8000-000000000001",
                     *           "kind": "tracker_project",
                     *           "native_scope_id": "10200",
                     *           "display_name": "CORE",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *           "history_from": "2026-01-01T00:00:00Z"
                     *         }
                     *       ],
                     *       "capabilities": [
                     *         {
                     *           "capability": "read_current",
                     *           "state": "verified",
                     *           "reason_code": null,
                     *           "checked_at": "2026-10-01T09:05:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["Connection"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            415: components["responses"]["UnsupportedMediaType"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    startConnectionProbe: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Проба принята. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5757-7000-8000-000000000001",
                     *       "status": "pending",
                     *       "started_at": "2026-11-08T10:01:00Z",
                     *       "finished_at": null,
                     *       "edition": null,
                     *       "source_version": null,
                     *       "contract_version": null,
                     *       "capabilities": [],
                     *       "reason_code": null,
                     *       "retry_after_seconds": 3,
                     *       "configuration_version": 1
                     *     }
                     */
                    "application/json": components["schemas"]["ConnectionProbe"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
        };
    };
    getConnectionProbe: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
                probe_id: components["parameters"]["ProbeId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Проба. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5757-7000-8000-000000000001",
                     *       "status": "succeeded",
                     *       "started_at": "2026-11-08T10:01:00Z",
                     *       "finished_at": "2026-11-08T10:01:14Z",
                     *       "edition": "cloud",
                     *       "source_version": "1001.0.0-SNAPSHOT",
                     *       "contract_version": "1",
                     *       "capabilities": [
                     *         {
                     *           "capability": "read_current",
                     *           "state": "verified",
                     *           "reason_code": null,
                     *           "checked_at": "2026-11-08T10:01:14Z"
                     *         },
                     *         {
                     *           "capability": "read_status_history",
                     *           "state": "verified",
                     *           "reason_code": null,
                     *           "checked_at": "2026-11-08T10:01:14Z"
                     *         },
                     *         {
                     *           "capability": "write_tracker_issue",
                     *           "state": "missing",
                     *           "reason_code": "permission_missing",
                     *           "checked_at": "2026-11-08T10:01:14Z"
                     *         }
                     *       ],
                     *       "reason_code": null,
                     *       "retry_after_seconds": null,
                     *       "configuration_version": 1
                     *     }
                     */
                    "application/json": components["schemas"]["ConnectionProbe"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    activateConnection: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
            };
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "probe_id": "0193a5b0-5757-7000-8000-000000000001"
                 *     }
                 */
                "application/json": components["schemas"]["ConnectionActivation"];
            };
        };
        responses: {
            /** @description Активное подключение. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5555-7000-8000-000000000001",
                     *       "name": "Jira основной",
                     *       "provider": "jira_cloud",
                     *       "instance": {
                     *         "id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *         "base_url": "https://example.atlassian.net",
                     *         "edition": "cloud",
                     *         "source_version": "1001.0.0-SNAPSHOT"
                     *       },
                     *       "status": "active",
                     *       "adapter_kind": "builtin",
                     *       "access_mode": "service_account",
                     *       "credentials_set_at": "2026-11-08T10:00:00Z",
                     *       "sync_paused": false,
                     *       "paused_until": null,
                     *       "last_success_at": null,
                     *       "scopes": [
                     *         {
                     *           "id": "0193a5b0-5656-7000-8000-000000000001",
                     *           "kind": "tracker_project",
                     *           "native_scope_id": "10200",
                     *           "display_name": "CORE",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *           "history_from": "2026-01-01T00:00:00Z"
                     *         }
                     *       ],
                     *       "capabilities": [
                     *         {
                     *           "capability": "read_current",
                     *           "state": "verified",
                     *           "reason_code": null,
                     *           "checked_at": "2026-11-08T10:01:14Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["Connection"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    getSyncStatus: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Состояние сбора. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "connection_id": "0193a5b0-5555-7000-8000-000000000001",
                     *       "connection_status": "active",
                     *       "paused_until": null,
                     *       "data_kinds": [
                     *         {
                     *           "data_kind": "tasks",
                     *           "phase": "incremental",
                     *           "last_attempt_at": "2026-11-08T03:00:00Z",
                     *           "last_outcome": "error",
                     *           "last_error_code": "timeout",
                     *           "failed_page_present": true,
                     *           "last_success_at": "2026-11-07T03:12:00Z",
                     *           "watermark": "2026-11-07T03:00:00Z",
                     *           "lag_seconds": 111600,
                     *           "history_from": "2026-01-01T00:00:00Z",
                     *           "completeness": "partial",
                     *           "unknown_fields": [
                     *             "customfield_10031"
                     *           ],
                     *           "affected_metrics": [
                     *             "T08",
                     *             "T12"
                     *           ]
                     *         }
                     *       ],
                     *       "last_reconcile": {
                     *         "started_at": "2026-11-07T02:00:00Z",
                     *         "finished_at": "2026-11-07T02:04:00Z",
                     *         "outcome": "complete",
                     *         "expected_count": 1240,
                     *         "observed_count": 1240,
                     *         "missing_count": 0,
                     *         "changed_count": 3
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["SyncStatus"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    requestSyncRun: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                connection_id: components["parameters"]["ConnectionId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "kind": "resume",
                 *       "data_kind": "tasks"
                 *     }
                 */
                "application/json": components["schemas"]["SyncRunRequest"];
            };
        };
        responses: {
            /** @description Операция принята. Location указывает на getBackgroundOperation; её статус и ошибка относятся к этому запуску. */
            202: {
                headers: {
                    /**
                     * @description Адрес ресурса принятой операции.
                     * @example /api/v1/operations/0193a5b0-a0a0-7000-8000-000000000001
                     */
                    Location?: string;
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a0a0-7000-8000-000000000001",
                     *       "kind": "sync_run",
                     *       "status": "queued",
                     *       "created_at": "2026-11-08T12:00:00Z",
                     *       "updated_at": "2026-11-08T12:00:00Z",
                     *       "result_uri": null,
                     *       "error": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["BackgroundOperation"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    receiveWebhook: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                route_id: string;
            };
            cookie?: never;
        };
        /** @description Тело в формате источника, не больше 1 MiB. */
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "object_kind": "merge_request",
                 *       "object_attributes": {
                 *         "id": 10342,
                 *         "iid": 42,
                 *         "action": "update"
                 *       }
                 *     }
                 */
                "application/json": Record<string, never>;
            };
        };
        responses: {
            /** @description Webhook принят к обработке. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            404: components["responses"]["NotFound"];
            413: components["responses"]["ContentTooLarge"];
            429: components["responses"]["TooManyRequests"];
        };
    };
    getCiCheckProfile: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Профиль проверок. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *       "profile_version_no": 3,
                     *       "entries": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "path_prefix": "services/billing",
                     *           "language": "go",
                     *           "analysis_kind": "code",
                     *           "mode": "builtin",
                     *           "profile_id": "go-default",
                     *           "image_digest": "sha256:6b2f0c1d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c"
                     *         },
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "path_prefix": "services/billing",
                     *           "language": "",
                     *           "analysis_kind": "secrets",
                     *           "mode": "builtin",
                     *           "profile_id": "gitleaks-default",
                     *           "image_digest": "sha256:1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f809"
                     *         }
                     *       ],
                     *       "expected_reports": [
                     *         "code",
                     *         "secrets",
                     *         "sca",
                     *         "junit",
                     *         "coverage"
                     *       ],
                     *       "documentation_roots": [
                     *         {
                     *           "root_id": "0193a5b0-e4e4-7000-8000-000000000001",
                     *           "kind": "docs",
                     *           "path_prefix": "docs"
                     *         }
                     *       ],
                     *       "limits": {
                     *         "max_package_bytes": 26214400,
                     *         "max_unpacked_bytes": 104857600,
                     *         "max_files": 1000
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["CiCheckProfile"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    uploadPackage: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/gzip": string;
            };
        };
        responses: {
            /** @description Пакет принят к разбору. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "receipt_id": "0193a5b0-5858-7000-8000-000000000001",
                     *       "status": "quarantined",
                     *       "duplicate_of": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["ReceiptAccepted"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            413: components["responses"]["ContentTooLarge"];
            415: components["responses"]["UnsupportedMediaType"];
            422: components["responses"]["ValidationFailed"];
            429: components["responses"]["TooManyRequests"];
            503: components["responses"]["ServiceUnavailable"];
        };
    };
    createImport: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "multipart/form-data": components["schemas"]["ImportUpload"];
            };
        };
        responses: {
            /** @description Пакет принят к разбору. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "receipt_id": "0193a5b0-5858-7000-8000-000000000002",
                     *       "status": "quarantined",
                     *       "duplicate_of": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["ReceiptAccepted"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            413: components["responses"]["ContentTooLarge"];
            415: components["responses"]["UnsupportedMediaType"];
            422: components["responses"]["ValidationFailed"];
            503: components["responses"]["ServiceUnavailable"];
        };
    };
    listReceipts: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                channel?: "sync" | "ci" | "import";
                status?: "quarantined" | "accepted" | "partial" | "rejected" | "duplicate";
                connection_id?: string;
                project_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница приёмов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-5858-7000-8000-000000000002",
                     *           "channel": "import",
                     *           "status": "partial",
                     *           "received_at": "2026-11-08T10:10:00Z",
                     *           "schema_id": "tasks",
                     *           "schema_version": "1",
                     *           "record_count": 2,
                     *           "accepted_count": 1,
                     *           "rejected_count": 1,
                     *           "reason_code": null,
                     *           "duplicate_of": null,
                     *           "affected_metrics": [
                     *             "T04",
                     *             "T05"
                     *           ]
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["ReceiptPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    getReceipt: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                receipt_id: components["parameters"]["ReceiptId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Квитанция. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5858-7000-8000-000000000002",
                     *       "channel": "import",
                     *       "status": "partial",
                     *       "received_at": "2026-11-08T10:10:00Z",
                     *       "schema_id": "tasks",
                     *       "schema_version": "1",
                     *       "record_count": 2,
                     *       "accepted_count": 1,
                     *       "rejected_count": 1,
                     *       "replay_count": 0,
                     *       "conflict_count": 0,
                     *       "reason_code": null,
                     *       "duplicate_of": null,
                     *       "affected_metrics": [
                     *         "T04",
                     *         "T05"
                     *       ],
                     *       "retry_after_seconds": null,
                     *       "result_url": "/teams/0193a5b0-3333-7000-8000-000000000001/report"
                     *     }
                     */
                    "application/json": components["schemas"]["Receipt"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listReceiptRecords: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                outcome?: "accepted" | "replay" | "conflict" | "rejected" | "suppressed";
            };
            header?: never;
            path: {
                receipt_id: components["parameters"]["ReceiptId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница записей в порядке номера. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "record_no": 1,
                     *           "record_kind": "task",
                     *           "native_key": "J1/10200/10345",
                     *           "outcome": "accepted",
                     *           "reason_code": null
                     *         },
                     *         {
                     *           "record_no": 2,
                     *           "record_kind": "task",
                     *           "native_key": null,
                     *           "outcome": "rejected",
                     *           "reason_code": "missing_source_id"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["ReceiptRecordPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listIngestConflicts: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                status?: "open" | "resolved";
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница конфликтов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-5959-7000-8000-000000000001",
                     *           "receipt_id": "0193a5b0-5858-7000-8000-000000000003",
                     *           "record_no": 4,
                     *           "existing_receipt_id": "0193a5b0-5858-7000-8000-000000000001",
                     *           "existing_record_no": 4,
                     *           "native_key": "J1/10345/changelog-88213",
                     *           "detected_at": "2026-11-08T10:12:00Z",
                     *           "status": "open",
                     *           "resolution": null
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["IngestConflictPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    resolveIngestConflict: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                conflict_id: components["parameters"]["ConflictId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "resolution": "kept_existing"
                 *     }
                 */
                "application/json": components["schemas"]["IngestConflictResolutionInput"];
            };
        };
        responses: {
            /** @description Решённый конфликт. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-5959-7000-8000-000000000001",
                     *       "receipt_id": "0193a5b0-5858-7000-8000-000000000003",
                     *       "record_no": 4,
                     *       "existing_receipt_id": "0193a5b0-5858-7000-8000-000000000001",
                     *       "existing_record_no": 4,
                     *       "native_key": "J1/10345/changelog-88213",
                     *       "detected_at": "2026-11-08T10:12:00Z",
                     *       "status": "resolved",
                     *       "resolution": "kept_existing"
                     *     }
                     */
                    "application/json": components["schemas"]["IngestConflict"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getWorkflowCatalog: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Справочники по экземплярам. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "instances": [
                     *         {
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "provider": "jira_cloud",
                     *           "statuses": [
                     *             {
                     *               "native_id": "1",
                     *               "name": "Open",
                     *               "source_category": "new"
                     *             },
                     *             {
                     *               "native_id": "3",
                     *               "name": "In Progress",
                     *               "source_category": "indeterminate"
                     *             },
                     *             {
                     *               "native_id": "10001",
                     *               "name": "Intake",
                     *               "source_category": "indeterminate"
                     *             }
                     *           ],
                     *           "resolutions": [
                     *             {
                     *               "native_id": "10000",
                     *               "name": "Done"
                     *             },
                     *             {
                     *               "native_id": "10002",
                     *               "name": "Duplicate"
                     *             }
                     *           ],
                     *           "issue_types": [
                     *             {
                     *               "native_id": "10004",
                     *               "name": "Bug"
                     *             }
                     *           ],
                     *           "fields": [
                     *             {
                     *               "native_id": "customfield_10016",
                     *               "name": "Story Points",
                     *               "value_type": "number"
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "unmapped_statuses": [
                     *         {
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "native_id": "10001"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["WorkflowCatalog"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    previewMapping: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "timezone": "Europe/Moscow",
                 *       "calendar_version_no": 1,
                 *       "rules": [
                 *         {
                 *           "rule_kind": "status",
                 *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                 *           "native_ref": "10001",
                 *           "native_value": "",
                 *           "meaning": "intake",
                 *           "rank": null
                 *         },
                 *         {
                 *           "rule_kind": "status",
                 *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                 *           "native_ref": "3",
                 *           "native_value": "",
                 *           "meaning": "active",
                 *           "rank": null
                 *         },
                 *         {
                 *           "rule_kind": "resolution",
                 *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                 *           "native_ref": "10000",
                 *           "native_value": "",
                 *           "meaning": "accepted",
                 *           "rank": null
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["MappingDraft"];
            };
        };
        responses: {
            /** @description Результат предпросмотра. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "as_of": "2026-11-08T00:00:00Z",
                     *       "period": {
                     *         "from": "2026-11-01T21:00:00Z",
                     *         "to": "2026-11-08T00:00:00Z"
                     *       },
                     *       "tasks": [
                     *         {
                     *           "task": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000001",
                     *             "display_key": "CORE-12",
                     *             "title": "Ошибка экспорта",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-12"
                     *           },
                     *           "started_at": "2026-10-02T09:00:00Z",
                     *           "completed_at": "2026-10-05T15:00:00Z",
                     *           "outcome": "completed",
                     *           "blocked_seconds": 86400,
                     *           "task_type": "Bug",
                     *           "size_label": "S",
                     *           "unmapped": []
                     *         }
                     *       ],
                     *       "control_values": [
                     *         {
                     *           "metric_id": "T05",
                     *           "value": {
                     *             "state": "ASSESSED",
                     *             "value": 14,
                     *             "unit": "count",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 14,
                     *             "bound": null,
                     *             "low_sample": true,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": null
                     *           }
                     *         }
                     *       ],
                     *       "unmapped_statuses": [
                     *         {
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "native_id": "10010"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["MappingPreview"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listMappingVersions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница версий. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "version_no": 2,
                     *           "base_version_no": 1,
                     *           "timezone": "Europe/Moscow",
                     *           "calendar_version_no": 1,
                     *           "rules": [
                     *             {
                     *               "rule_kind": "status",
                     *               "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *               "native_ref": "3",
                     *               "native_value": "",
                     *               "meaning": "active",
                     *               "rank": null
                     *             }
                     *           ],
                     *           "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *           "confirmed_at": "2026-10-20T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["MappingVersionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createMappingVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "timezone": "Europe/Moscow",
                 *       "calendar_version_no": 1,
                 *       "rules": [
                 *         {
                 *           "rule_kind": "status",
                 *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                 *           "native_ref": "10001",
                 *           "native_value": "",
                 *           "meaning": "intake",
                 *           "rank": null
                 *         },
                 *         {
                 *           "rule_kind": "status",
                 *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                 *           "native_ref": "3",
                 *           "native_value": "",
                 *           "meaning": "active",
                 *           "rank": null
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["MappingDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "timezone": "Europe/Moscow",
                     *       "calendar_version_no": 1,
                     *       "rules": [
                     *         {
                     *           "rule_kind": "status",
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "native_ref": "3",
                     *           "native_value": "",
                     *           "meaning": "active",
                     *           "rank": null
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-10-20T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["MappingVersion"];
                };
            };
            /** @description Новая версия. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 3,
                     *       "base_version_no": 2,
                     *       "timezone": "Europe/Moscow",
                     *       "calendar_version_no": 1,
                     *       "rules": [
                     *         {
                     *           "rule_kind": "status",
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "native_ref": "10001",
                     *           "native_value": "",
                     *           "meaning": "intake",
                     *           "rank": null
                     *         },
                     *         {
                     *           "rule_kind": "status",
                     *           "instance_id": "0193a5b0-aaaa-7000-8000-000000000001",
                     *           "native_ref": "3",
                     *           "native_value": "",
                     *           "meaning": "active",
                     *           "rank": null
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-08T11:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["MappingVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listCalendarVersions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница версий. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "version_no": 1,
                     *           "base_version_no": null,
                     *           "timezone": "Europe/Moscow",
                     *           "workdays": [
                     *             "mon",
                     *             "tue",
                     *             "wed",
                     *             "thu",
                     *             "fri"
                     *           ],
                     *           "work_start": "10:00",
                     *           "work_end": "19:00",
                     *           "holidays": [
                     *             "2026-11-04"
                     *           ],
                     *           "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *           "confirmed_at": "2026-10-01T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["CalendarVersionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createCalendarVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "timezone": "Europe/Moscow",
                 *       "workdays": [
                 *         "mon",
                 *         "tue",
                 *         "wed",
                 *         "thu",
                 *         "fri"
                 *       ],
                 *       "work_start": "10:00",
                 *       "work_end": "19:00",
                 *       "holidays": [
                 *         "2026-11-04",
                 *         "2026-12-31"
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["CalendarDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "timezone": "Europe/Moscow",
                     *       "workdays": [
                     *         "mon",
                     *         "tue",
                     *         "wed",
                     *         "thu",
                     *         "fri"
                     *       ],
                     *       "work_start": "10:00",
                     *       "work_end": "19:00",
                     *       "holidays": [
                     *         "2026-11-04",
                     *         "2026-12-31"
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-08T11:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CalendarVersion"];
                };
            };
            /** @description Новая версия. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "timezone": "Europe/Moscow",
                     *       "workdays": [
                     *         "mon",
                     *         "tue",
                     *         "wed",
                     *         "thu",
                     *         "fri"
                     *       ],
                     *       "work_start": "10:00",
                     *       "work_end": "19:00",
                     *       "holidays": [
                     *         "2026-11-04",
                     *         "2026-12-31"
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-08T11:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CalendarVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    getTargetBaselines: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Ориентиры по группам. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000001",
                     *       "data": [
                     *         {
                     *           "task_type": "Bug",
                     *           "size_label": "S",
                     *           "p85": {
                     *             "state": "ASSESSED",
                     *             "value": 259200,
                     *             "unit": "seconds",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 9,
                     *             "bound": null,
                     *             "low_sample": true,
                     *             "excluded_count": 0,
                     *             "missing_count": 1,
                     *             "reason_code": null
                     *           }
                     *         },
                     *         {
                     *           "task_type": "Bug",
                     *           "size_label": null,
                     *           "p85": {
                     *             "state": "ASSESSED",
                     *             "value": 432000,
                     *             "unit": "seconds",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 4,
                     *             "bound": null,
                     *             "low_sample": true,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": "size_unknown"
                     *           }
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["TargetBaselineList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listTargetVersions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                task_type?: string;
                /** @description Размер группы; вместе с task_type выбирает группу для If-Match. */
                size_label?: string;
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница версий новыми первыми. */
            200: {
                headers: {
                    /**
                     * @description Только при обоих фильтрах: версия группы, например "3"; "0", если версия ещё не создана.
                     * @example "1"
                     */
                    ETag?: string;
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "task_type": "Bug",
                     *           "size_label": "S",
                     *           "version_no": 1,
                     *           "base_version_no": null,
                     *           "duration_seconds": 172800,
                     *           "basis": "manual",
                     *           "basis_revision_id": null,
                     *           "basis_value_seconds": null,
                     *           "basis_n": null,
                     *           "basis_period": null,
                     *           "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *           "confirmed_at": "2026-10-02T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["TargetVersionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createTargetVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "task_type": "Bug",
                 *       "size_label": "S",
                 *       "duration_seconds": 345600,
                 *       "basis": "historical_p85",
                 *       "basis_revision_id": "0193a5b0-a1a1-7000-8000-000000000001",
                 *       "basis_period": {
                 *         "from": "2026-08-03T00:00:00Z",
                 *         "to": "2026-11-02T00:00:00Z"
                 *       }
                 *     }
                 */
                "application/json": components["schemas"]["TargetDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "task_type": "Bug",
                     *       "size_label": "S",
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "duration_seconds": 345600,
                     *       "basis": "historical_p85",
                     *       "basis_revision_id": "0193a5b0-a1a1-7000-8000-000000000001",
                     *       "basis_value_seconds": 259200,
                     *       "basis_n": 9,
                     *       "basis_period": {
                     *         "from": "2026-08-03T00:00:00Z",
                     *         "to": "2026-11-02T00:00:00Z"
                     *       },
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-08T11:30:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["TargetVersion"];
                };
            };
            /** @description Новая версия срока. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "task_type": "Bug",
                     *       "size_label": "S",
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "duration_seconds": 345600,
                     *       "basis": "historical_p85",
                     *       "basis_revision_id": "0193a5b0-a1a1-7000-8000-000000000001",
                     *       "basis_value_seconds": 259200,
                     *       "basis_n": 9,
                     *       "basis_period": {
                     *         "from": "2026-08-03T00:00:00Z",
                     *         "to": "2026-11-02T00:00:00Z"
                     *       },
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-08T11:30:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["TargetVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    getTeamReport: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                /** @description Шаг ряда. */
                step?: components["parameters"]["Step"];
                /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
                revision_id?: components["parameters"]["RevisionQuery"];
                /** @description Ограничить отчёт проектами команды; без параметра - все её проекты. */
                project_id?: string[];
                /** @description Тип задачи по сопоставлению. */
                task_type?: string;
                /** @description Размер задачи; `unknown` - группа «без размера». */
                size_label?: string;
                /** @description Разбить ряды по типу или размеру: каждая группа - отдельный элемент `metrics` с заполненным `group`. */
                group_by?: "task_type" | "size_label";
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Отчёт. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *       "period": {
                     *         "from": "2026-10-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       },
                     *       "step": "week",
                     *       "revision": {
                     *         "id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *         "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *         "revision_no": 31,
                     *         "as_of": "2026-11-01T00:00:00Z",
                     *         "published_at": "2026-11-08T03:40:00Z",
                     *         "loading": false,
                     *         "irreproducible_reason": null
                     *       },
                     *       "grouping": "current_structure",
                     *       "metrics": [
                     *         {
                     *           "metric_id": "T05",
                     *           "definition_version": 1,
                     *           "unit": "count",
                     *           "points": [
                     *             {
                     *               "point_start": "2026-10-05T00:00:00Z",
                     *               "value": {
                     *                 "state": "ASSESSED",
                     *                 "value": 7,
                     *                 "unit": "count",
                     *                 "numerator": null,
                     *                 "denominator": null,
                     *                 "n": 7,
                     *                 "bound": null,
                     *                 "low_sample": true,
                     *                 "excluded_count": 1,
                     *                 "missing_count": 0,
                     *                 "reason_code": null
                     *               }
                     *             }
                     *           ]
                     *         },
                     *         {
                     *           "metric_id": "T08",
                     *           "definition_version": 1,
                     *           "unit": "seconds",
                     *           "estimator": "p50",
                     *           "points": [
                     *             {
                     *               "point_start": "2026-10-05T00:00:00Z",
                     *               "value": {
                     *                 "state": "PROVISIONAL",
                     *                 "value": 259200,
                     *                 "unit": "seconds",
                     *                 "numerator": null,
                     *                 "denominator": null,
                     *                 "n": 6,
                     *                 "bound": null,
                     *                 "low_sample": true,
                     *                 "excluded_count": 0,
                     *                 "missing_count": 1,
                     *                 "reason_code": "history_incomplete"
                     *               }
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TeamReport"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listMetricDefinitions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница определений. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "metric_id": "T08",
                     *           "definition_version": 1,
                     *           "name": "Cycle time",
                     *           "unit": "seconds",
                     *           "catalog_ref": "docs/product/metrics/tasks.md#t08"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["MetricDefinitionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    listSeries: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                team_id?: string;
                project_id?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница рядов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *           "scope": {
                     *             "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *             "project_ids": [],
                     *             "filters": {}
                     *           },
                     *           "metric_set": "team_flow",
                     *           "step": "week",
                     *           "current_revision_id": "0193a5b0-a1a1-7000-8000-000000000002"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["SeriesPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    getSeriesData: {
        parameters: {
            query?: {
                /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
                revision_id?: components["parameters"]["RevisionQuery"];
                /** @description Показатели через запятую; без параметра - все показатели ряда. */
                metric_id?: string;
            };
            header?: never;
            path: {
                series_id: components["parameters"]["SeriesId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Значения ряда. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *       "revision": {
                     *         "id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *         "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *         "revision_no": 31,
                     *         "as_of": "2026-11-01T00:00:00Z",
                     *         "published_at": "2026-11-08T03:40:00Z",
                     *         "loading": false,
                     *         "irreproducible_reason": null
                     *       },
                     *       "metrics": [
                     *         {
                     *           "metric_id": "T05",
                     *           "definition_version": 1,
                     *           "unit": "count",
                     *           "points": [
                     *             {
                     *               "point_start": "2026-10-05T00:00:00Z",
                     *               "value": {
                     *                 "state": "ASSESSED",
                     *                 "value": 7,
                     *                 "unit": "count",
                     *                 "numerator": null,
                     *                 "denominator": null,
                     *                 "n": 7,
                     *                 "bound": null,
                     *                 "low_sample": true,
                     *                 "excluded_count": 1,
                     *                 "missing_count": 0,
                     *                 "reason_code": null
                     *               }
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "access_limited": true,
                     *       "access_notes": [
                     *         {
                     *           "code": "restricted_by_access",
                     *           "connection_id": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["SeriesData"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    refreshSeries: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                series_id: components["parameters"]["SeriesId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Операция принята. Location указывает на getBackgroundOperation; её статус и ошибка относятся к этому запуску. */
            202: {
                headers: {
                    /**
                     * @description Адрес ресурса принятой операции.
                     * @example /api/v1/operations/0193a5b0-a0a0-7000-8000-000000000001
                     */
                    Location?: string;
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a0a0-7000-8000-000000000001",
                     *       "kind": "series_refresh",
                     *       "status": "queued",
                     *       "created_at": "2026-11-08T12:00:00Z",
                     *       "updated_at": "2026-11-08T12:00:00Z",
                     *       "result_uri": null,
                     *       "error": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["BackgroundOperation"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    getRevision: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                revision_id: components["parameters"]["RevisionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Ревизия. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *       "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *       "revision_no": 31,
                     *       "status": "published",
                     *       "window": {
                     *         "from": "2026-01-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       },
                     *       "as_of": "2026-11-01T00:00:00Z",
                     *       "timezone": "Europe/Moscow",
                     *       "knowledge_seq": 18452,
                     *       "published_at": "2026-11-08T03:40:00Z",
                     *       "superseded_at": null,
                     *       "loading": false,
                     *       "irreproducible_reason": null,
                     *       "definitions": [
                     *         {
                     *           "metric_id": "T08",
                     *           "definition_version": 1,
                     *           "name": "Cycle time",
                     *           "unit": "seconds",
                     *           "catalog_ref": "docs/product/metrics/tasks.md#t08"
                     *         }
                     *       ],
                     *       "inputs": [
                     *         {
                     *           "input_kind": "mapping",
                     *           "input_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "version": "2"
                     *         },
                     *         {
                     *           "input_kind": "calendar",
                     *           "input_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "version": "1"
                     *         }
                     *       ],
                     *       "source_watermarks": [
                     *         {
                     *           "connection_id": "0193a5b0-5555-7000-8000-000000000001",
                     *           "watermark": "2026-11-07T03:00:00Z",
                     *           "completeness": "complete"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["Revision"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listRevisionCohort: {
        parameters: {
            query: {
                metric_id: string;
                point_start: string;
                /** @description Оценка точки показателя с несколькими рядами, как `MetricSeries.estimator`; без параметра - первая оценка показателя. */
                estimator?: string;
                role?: "numerator" | "denominator" | "excluded" | "missing";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                revision_id: components["parameters"]["RevisionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница когорты. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "object": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000001",
                     *             "display_key": "CORE-12",
                     *             "title": "Ошибка экспорта",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-12"
                     *           },
                     *           "item_no": 0,
                     *           "role": "numerator",
                     *           "value": 259200,
                     *           "reason_code": null,
                     *           "basis_deleted": false
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["CohortPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listSavedViews: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница срезов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-a3a3-7000-8000-000000000001",
                     *           "name": "Платформа по неделям",
                     *           "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *           "filters": {
                     *             "period_days": 90
                     *           },
                     *           "pinned_revision_id": null,
                     *           "created_at": "2026-11-01T10:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["SavedViewPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    createSavedView: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "name": "Платформа по неделям",
                 *       "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                 *       "filters": {
                 *         "period_days": 90
                 *       },
                 *       "pinned_revision_id": null
                 *     }
                 */
                "application/json": components["schemas"]["SavedViewInput"];
            };
        };
        responses: {
            /** @description Срез сохранён. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a3a3-7000-8000-000000000002",
                     *       "name": "Платформа по неделям",
                     *       "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *       "filters": {
                     *         "period_days": 90
                     *       },
                     *       "pinned_revision_id": null,
                     *       "created_at": "2026-11-08T11:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["SavedView"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getSavedView: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                saved_view_id: components["parameters"]["SavedViewId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Срез. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a3a3-7000-8000-000000000001",
                     *       "name": "Платформа по неделям",
                     *       "series_id": "0193a5b0-a2a2-7000-8000-000000000001",
                     *       "filters": {
                     *         "period_days": 90
                     *       },
                     *       "pinned_revision_id": null,
                     *       "created_at": "2026-11-01T10:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["SavedView"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    deleteSavedView: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                saved_view_id: components["parameters"]["SavedViewId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Срез удалён. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createExport: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000002",
                 *       "metric_ids": [
                 *         "T05",
                 *         "T08"
                 *       ],
                 *       "format": "csv"
                 *     }
                 */
                "application/json": components["schemas"]["ExportCreate"];
            };
        };
        responses: {
            /** @description Задание выгрузки. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a4a4-7000-8000-000000000001",
                     *       "status": "pending",
                     *       "format": "csv",
                     *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *       "requested_at": "2026-11-08T11:00:00Z",
                     *       "completed_at": null,
                     *       "expires_at": null,
                     *       "access_limited": false,
                     *       "retry_after_seconds": 5
                     *     }
                     */
                    "application/json": components["schemas"]["Export"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
            503: components["responses"]["ServiceUnavailable"];
        };
    };
    getExport: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                export_id: components["parameters"]["ExportId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Задание выгрузки. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a4a4-7000-8000-000000000001",
                     *       "status": "ready",
                     *       "format": "csv",
                     *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *       "requested_at": "2026-11-08T11:00:00Z",
                     *       "completed_at": "2026-11-08T11:00:20Z",
                     *       "expires_at": "2026-11-15T11:00:20Z",
                     *       "access_limited": false,
                     *       "retry_after_seconds": null
                     *     }
                     */
                    "application/json": components["schemas"]["Export"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    downloadExport: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                export_id: components["parameters"]["ExportId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Файл выгрузки. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ExportDocument"];
                    "application/zip": string;
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            503: components["responses"]["ServiceUnavailable"];
        };
    };
    getProjectOverview: {
        parameters: {
            query?: {
                /** @description Дата состояния расчёта. По умолчанию - конец периода или текущий момент. */
                as_of?: components["parameters"]["AsOf"];
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Обзор проекта. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "project": {
                     *         "id": "0193a5b0-4444-7000-8000-000000000001",
                     *         "name": "Core",
                     *         "status": "active"
                     *       },
                     *       "as_of": "2026-11-08T00:00:00Z",
                     *       "wip": {
                     *         "state": "ASSESSED",
                     *         "value": 12,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 12,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "over_target": {
                     *         "state": "ASSESSED",
                     *         "value": 3,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 3,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "awaiting_review": {
                     *         "state": "ASSESSED",
                     *         "value": 2,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 2,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "sources": [
                     *         {
                     *           "connection_id": "0193a5b0-5555-7000-8000-000000000001",
                     *           "provider": "jira_cloud",
                     *           "last_success_at": "2026-11-08T03:12:00Z",
                     *           "completeness": "complete"
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["ProjectOverview"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listProjectTimeline: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                actor_kind?: components["schemas"]["ActorKind"];
                object_kind?: "task" | "merge_request" | "pipeline" | "document";
                /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
                revision_id?: components["parameters"]["RevisionQuery"];
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница событий по времени события. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e1e1-7000-8000-000000000001",
                     *           "object": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000002",
                     *             "display_key": "CORE-15",
                     *             "title": "Перенести отчёт",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-15"
                     *           },
                     *           "event_kind": "field_change",
                     *           "field": "status",
                     *           "before_value": "Open",
                     *           "after_value": "In Progress",
                     *           "actor": {
                     *             "account_id": "0193a5b0-9999-7000-8000-000000000001",
                     *             "kind": "human",
                     *             "display_name": "Анна Смирнова"
                     *           },
                     *           "event_time": "2026-10-02T09:00:00Z",
                     *           "event_offset_seconds": 10800,
                     *           "observed_at": "2026-10-06T00:00:00Z",
                     *           "observed_via": "backfill",
                     *           "late": true
                     *         }
                     *       ],
                     *       "history_from": "2026-01-01T00:00:00Z",
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TimelinePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listTasks: {
        parameters: {
            query?: {
                project_id?: string;
                team_id?: string;
                status_meaning?: "intake" | "active" | "wait" | "blocked" | "done" | "cancelled" | "duplicate" | "unknown";
                over_target?: boolean;
                sort?: "age" | "-age" | "updated_at" | "-updated_at" | "key" | "-key";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница задач. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-f000-7000-8000-000000000002",
                     *           "key": "CORE-15",
                     *           "title": "Перенести отчёт",
                     *           "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *           "status": {
                     *             "native_id": "3",
                     *             "name": "In Progress",
                     *             "meaning": "active"
                     *           },
                     *           "task_type": "Story",
                     *           "size_label": "M",
                     *           "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "age_seconds": 518400,
                     *           "over_target": true,
                     *           "updated_at": "2026-11-07T14:00:00Z",
                     *           "source_url": "https://example.atlassian.net/browse/CORE-15"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TaskPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    getTask: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                task_id: components["parameters"]["TaskId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Задача. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f000-7000-8000-000000000002",
                     *       "key": "CORE-15",
                     *       "title": "Перенести отчёт",
                     *       "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *       "status": {
                     *         "native_id": "3",
                     *         "name": "In Progress",
                     *         "meaning": "active"
                     *       },
                     *       "task_type": "Story",
                     *       "size_label": "M",
                     *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *       "age_seconds": 518400,
                     *       "over_target": true,
                     *       "updated_at": "2026-11-07T14:00:00Z",
                     *       "source_url": "https://example.atlassian.net/browse/CORE-15",
                     *       "target": {
                     *         "duration_seconds": 432000,
                     *         "version_no": 1
                     *       },
                     *       "blocked_intervals": [
                     *         {
                     *           "from": "2026-11-03T10:00:00Z",
                     *           "to": "2026-11-04T10:00:00Z"
                     *         }
                     *       ],
                     *       "observations": [
                     *         {
                     *           "kind": "awaiting_reviewer",
                     *           "since": "2026-11-06T12:00:00Z",
                     *           "basis": [
                     *             {
                     *               "kind": "merge_request",
                     *               "id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *               "display_key": "!42",
                     *               "title": "Перенос отчёта",
                     *               "source_url": "https://gitlab.example/core/-/merge_requests/42"
                     *             }
                     *           ],
                     *           "action": "assign_reviewer"
                     *         },
                     *         {
                     *           "kind": "ci_passed_after_retry",
                     *           "since": "2026-11-07T09:30:00Z",
                     *           "basis": [
                     *             {
                     *               "kind": "pipeline",
                     *               "id": "0193a5b0-d0d0-7000-8000-000000000001",
                     *               "display_key": "#8841",
                     *               "title": null,
                     *               "source_url": "https://gitlab.example/core/-/pipelines/8841"
                     *             }
                     *           ],
                     *           "action": "check_pipeline"
                     *         }
                     *       ],
                     *       "links": [
                     *         {
                     *           "id": "0193a5b0-c0c0-7000-8000-000000000001",
                     *           "link_type": "task_mr",
                     *           "method": "source_explicit",
                     *           "status": "confirmed",
                     *           "from_object_id": "0193a5b0-f000-7000-8000-000000000002",
                     *           "to": {
                     *             "kind": "merge_request",
                     *             "id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *             "display_key": "!42",
                     *             "title": "Перенос отчёта",
                     *             "source_url": "https://gitlab.example/core/-/merge_requests/42"
                     *           },
                     *           "decided_at": null
                     *         },
                     *         {
                     *           "id": "0193a5b0-c0c0-7000-8000-000000000002",
                     *           "link_type": "task_mr",
                     *           "method": "branch_key",
                     *           "status": "candidate",
                     *           "from_object_id": "0193a5b0-f000-7000-8000-000000000002",
                     *           "to": {
                     *             "kind": "merge_request",
                     *             "id": "0193a5b0-b0b0-7000-8000-000000000002",
                     *             "display_key": "!43",
                     *             "title": "Правка экспорта",
                     *             "source_url": "https://gitlab.example/core/-/merge_requests/43"
                     *           },
                     *           "decided_at": null
                     *         }
                     *       ],
                     *       "redactions": []
                     *     }
                     */
                    "application/json": components["schemas"]["TaskDetail"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listTaskEvents: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                task_id: components["parameters"]["TaskId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница событий. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e1e1-7000-8000-000000000002",
                     *           "object": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000002",
                     *             "display_key": "CORE-15",
                     *             "title": "Перенести отчёт",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-15"
                     *           },
                     *           "event_kind": "field_change",
                     *           "field": "status",
                     *           "before_value": "In Progress",
                     *           "after_value": "Blocked",
                     *           "actor": {
                     *             "account_id": "0193a5b0-9999-7000-8000-000000000001",
                     *             "kind": "human",
                     *             "display_name": "Анна Смирнова"
                     *           },
                     *           "event_time": "2026-11-03T10:00:00Z",
                     *           "event_offset_seconds": 10800,
                     *           "observed_at": "2026-11-03T10:00:04Z",
                     *           "observed_via": "webhook",
                     *           "late": false
                     *         }
                     *       ],
                     *       "history_from": "2026-01-01T00:00:00Z",
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TimelinePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    decideLink: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                link_id: components["parameters"]["LinkId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision": "confirm"
                 *     }
                 */
                "application/json": components["schemas"]["LinkDecision"];
            };
        };
        responses: {
            /** @description Связь после решения. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-c0c0-7000-8000-000000000003",
                     *       "link_type": "task_mr",
                     *       "method": "user_confirmed",
                     *       "status": "confirmed",
                     *       "from_object_id": "0193a5b0-f000-7000-8000-000000000002",
                     *       "to": {
                     *         "kind": "merge_request",
                     *         "id": "0193a5b0-b0b0-7000-8000-000000000002",
                     *         "display_key": "!43",
                     *         "title": "Правка экспорта",
                     *         "source_url": "https://gitlab.example/core/-/merge_requests/43"
                     *       },
                     *       "decided_at": "2026-11-08T11:40:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["Link"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listMergeRequests: {
        parameters: {
            query?: {
                project_id?: string;
                area_id?: string;
                state?: "open" | "merged" | "closed";
                review_wait?: "no_reviewer" | "awaiting_first_response" | "awaiting_re_review";
                sort?: "wait" | "-wait" | "opened_at" | "-opened_at";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница MR. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *           "iid": 42,
                     *           "title": "Перенос отчёта",
                     *           "state": "open",
                     *           "draft": false,
                     *           "area": {
                     *             "id": "0193a5b0-8888-7000-8000-000000000001",
                     *             "kind": "repository",
                     *             "display_name": "platform/core",
                     *             "connection_id": "0193a5b0-5555-7000-8000-000000000002"
                     *           },
                     *           "author": {
                     *             "account_id": "0193a5b0-9999-7000-8000-000000000003",
                     *             "kind": "human",
                     *             "display_name": "Иван Петров"
                     *           },
                     *           "head_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *           "opened_at": "2026-11-05T10:00:00Z",
                     *           "ready_at": "2026-11-06T12:00:00Z",
                     *           "review_wait": {
                     *             "kind": "no_reviewer",
                     *             "since": "2026-11-06T12:00:00Z",
                     *             "wait_seconds": 165600,
                     *             "fallback_open": false
                     *           },
                     *           "source_url": "https://gitlab.example/core/-/merge_requests/42"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["MergeRequestPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    getMergeRequest: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                merge_request_id: components["parameters"]["MergeRequestId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description MR. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *       "iid": 42,
                     *       "title": "Перенос отчёта",
                     *       "state": "open",
                     *       "draft": false,
                     *       "area": {
                     *         "id": "0193a5b0-8888-7000-8000-000000000001",
                     *         "kind": "repository",
                     *         "display_name": "platform/core",
                     *         "connection_id": "0193a5b0-5555-7000-8000-000000000002"
                     *       },
                     *       "author": {
                     *         "account_id": "0193a5b0-9999-7000-8000-000000000003",
                     *         "kind": "human",
                     *         "display_name": "Иван Петров"
                     *       },
                     *       "head_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *       "opened_at": "2026-11-05T10:00:00Z",
                     *       "ready_at": "2026-11-06T12:00:00Z",
                     *       "review_wait": {
                     *         "kind": "no_reviewer",
                     *         "since": "2026-11-06T12:00:00Z",
                     *         "wait_seconds": 165600,
                     *         "fallback_open": false
                     *       },
                     *       "source_url": "https://gitlab.example/core/-/merge_requests/42"
                     *     }
                     */
                    "application/json": components["schemas"]["MergeRequest"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getMergeRequestSummary: {
        parameters: {
            query?: {
                revision_no?: number;
            };
            header?: never;
            path: {
                merge_request_id: components["parameters"]["MergeRequestId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Сводка. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-d0d0-7000-8000-000000000001",
                     *       "merge_request_id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *       "revision_no": 3,
                     *       "status": "ready",
                     *       "head_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *       "base_sha": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
                     *       "pipeline_state": "complete",
                     *       "built_at": "2026-11-08T11:02:00Z",
                     *       "stale_reason": null,
                     *       "items": [
                     *         {
                     *           "item_no": 1,
                     *           "item_kind": "new",
                     *           "finding_id": "0193a5b0-e2e2-7000-8000-000000000001",
                     *           "rule_id": "go.lang.security.audit.sqli",
                     *           "severity": "high",
                     *           "location": {
                     *             "path": "services/billing/repo.go",
                     *             "line_start": 88,
                     *             "line_end": 88
                     *           },
                     *           "rank": 1
                     *         },
                     *         {
                     *           "item_no": 2,
                     *           "item_kind": "not_evaluated",
                     *           "finding_id": null,
                     *           "rule_id": null,
                     *           "severity": null,
                     *           "location": null,
                     *           "rank": null
                     *         }
                     *       ],
                     *       "not_evaluated": [
                     *         {
                     *           "analysis_kind": "coverage",
                     *           "reason_code": "report_missing"
                     *         }
                     *       ],
                     *       "coverage_changed_lines": {
                     *         "state": "NOT_EVALUATED",
                     *         "value": null,
                     *         "unit": "ratio",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": null,
                     *         "bound": null,
                     *         "low_sample": false,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": "report_missing"
                     *       },
                     *       "ci": {
                     *         "pipeline_id": "0193a5b0-d1d1-7000-8000-000000000001",
                     *         "outcome": "success",
                     *         "attempts": 4,
                     *         "retried_jobs": 1
                     *       },
                     *       "publication": {
                     *         "status": "succeeded",
                     *         "external_id": "88213",
                     *         "error_code": null,
                     *         "published_at": "2026-11-08T11:02:40Z"
                     *       },
                     *       "redactions": [],
                     *       "access_limited": true,
                     *       "access_notes": [
                     *         {
                     *           "code": "restricted_by_access"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["MergeRequestSummary"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getPipeline: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                pipeline_id: components["parameters"]["PipelineId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Pipeline. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-d1d1-7000-8000-000000000001",
                     *       "ref_name": "feature/report-move",
                     *       "sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *       "merge_request_id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *       "outcome": "success",
                     *       "native_status": "success",
                     *       "started_at": "2026-11-08T10:50:00Z",
                     *       "finished_at": "2026-11-08T11:01:00Z",
                     *       "attempts_total": 4,
                     *       "retried_jobs": 1,
                     *       "allowed_failures": 0,
                     *       "attempt_history": "complete",
                     *       "reports": [
                     *         {
                     *           "analysis_kind": "junit",
                     *           "state": "received"
                     *         },
                     *         {
                     *           "analysis_kind": "coverage",
                     *           "state": "missing"
                     *         }
                     *       ],
                     *       "tests": {
                     *         "state": "ASSESSED",
                     *         "passed": 412,
                     *         "failed": 0,
                     *         "error": 0,
                     *         "skipped": 3,
                     *         "distinct_cases": 415,
                     *         "executions": 415
                     *       },
                     *       "source_url": "https://gitlab.example/core/-/pipelines/7781"
                     *     }
                     */
                    "application/json": components["schemas"]["Pipeline"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listPipelineAttempts: {
        parameters: {
            query?: {
                outcome?: "success" | "failed" | "canceled" | "skipped" | "running" | "unknown";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                pipeline_id: components["parameters"]["PipelineId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница попыток. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-d2d2-7000-8000-000000000001",
                     *           "job_key": "test-linux",
                     *           "config_fingerprint": "4f1c0d9e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d",
                     *           "attempt_no": 1,
                     *           "outcome": "failed",
                     *           "native_status": "failed",
                     *           "allow_failure": false,
                     *           "failure_kind": null,
                     *           "runner_class": "linux-medium",
                     *           "started_at": "2026-11-08T10:51:00Z",
                     *           "finished_at": "2026-11-08T10:52:00Z",
                     *           "duration_seconds": 60,
                     *           "source_url": "https://gitlab.example/core/-/jobs/99101"
                     *         },
                     *         {
                     *           "id": "0193a5b0-d2d2-7000-8000-000000000002",
                     *           "job_key": "test-linux",
                     *           "config_fingerprint": "4f1c0d9e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d",
                     *           "attempt_no": 2,
                     *           "outcome": "success",
                     *           "native_status": "success",
                     *           "allow_failure": false,
                     *           "failure_kind": null,
                     *           "runner_class": "linux-medium",
                     *           "started_at": "2026-11-08T10:53:00Z",
                     *           "finished_at": "2026-11-08T10:53:30Z",
                     *           "duration_seconds": 30,
                     *           "source_url": "https://gitlab.example/core/-/jobs/99107"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "usage_seconds": {
                     *         "state": "ASSESSED",
                     *         "value": 130,
                     *         "unit": "seconds",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 3,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["AttemptPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listTestExecutions: {
        parameters: {
            query?: {
                outcome?: "passed" | "failed" | "error" | "skipped";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                pipeline_id: components["parameters"]["PipelineId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница выполнений. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "test_case": {
                     *             "id": "0193a5b0-d3d3-7000-8000-000000000001",
                     *             "key": "billing.TestExport/csv"
                     *           },
                     *           "attempt_id": "0193a5b0-d2d2-7000-8000-000000000001",
                     *           "config_fingerprint": "4f1c0d9e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c1d",
                     *           "outcome": "failed",
                     *           "duration_ms": 1840
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["TestExecutionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getCheckProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Матрица проверок. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 3,
                     *       "base_version_no": 2,
                     *       "entries": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "go",
                     *           "analysis_kind": "code",
                     *           "mode": "builtin",
                     *           "profile_id": "go-default",
                     *           "tool_versions": {
                     *             "golangci-lint": "2.6.1"
                     *           }
                     *         },
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "",
                     *           "analysis_kind": "secrets",
                     *           "mode": "builtin",
                     *           "profile_id": "gitleaks-default",
                     *           "tool_versions": {
                     *             "gitleaks": "8.28.0"
                     *           }
                     *         }
                     *       ],
                     *       "uncovered": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "kotlin",
                     *           "reason_code": "language_unsupported"
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *       "confirmed_at": "2026-10-10T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CheckProfileVersion"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createCheckProfileVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "entries": [
                 *         {
                 *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                 *           "language": "go",
                 *           "analysis_kind": "code",
                 *           "mode": "import",
                 *           "profile_id": "sonar-import"
                 *         },
                 *         {
                 *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                 *           "language": "",
                 *           "analysis_kind": "secrets",
                 *           "mode": "builtin",
                 *           "profile_id": "gitleaks-default"
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["CheckProfileDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 4,
                     *       "base_version_no": 3,
                     *       "entries": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "go",
                     *           "analysis_kind": "code",
                     *           "mode": "import",
                     *           "profile_id": "sonar-import",
                     *           "tool_versions": {}
                     *         },
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "",
                     *           "analysis_kind": "secrets",
                     *           "mode": "builtin",
                     *           "profile_id": "gitleaks-default",
                     *           "tool_versions": {
                     *             "gitleaks": "8.28.0"
                     *           }
                     *         }
                     *       ],
                     *       "uncovered": [],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *       "confirmed_at": "2026-11-08T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CheckProfileVersion"];
                };
            };
            /** @description Новая версия матрицы. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 4,
                     *       "base_version_no": 3,
                     *       "entries": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "go",
                     *           "analysis_kind": "code",
                     *           "mode": "import",
                     *           "profile_id": "sonar-import",
                     *           "tool_versions": {}
                     *         },
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "",
                     *           "analysis_kind": "secrets",
                     *           "mode": "builtin",
                     *           "profile_id": "gitleaks-default",
                     *           "tool_versions": {
                     *             "gitleaks": "8.28.0"
                     *           }
                     *         }
                     *       ],
                     *       "uncovered": [],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *       "confirmed_at": "2026-11-08T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CheckProfileVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    getCiConfiguration: {
        parameters: {
            query: {
                ci_system: "gitlab_ci" | "github_actions" | "jenkins";
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Фрагмент конфигурации. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "ci_system": "gitlab_ci",
                     *       "profile_version_no": 4,
                     *       "snippet": "craftpulse:\n  stage: test\n  image: registry.example/craftpulse-ci@sha256:6b2f0c1d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c\n  allow_failure: true\n  script: [craftpulse-ci run]\n",
                     *       "variables": [
                     *         {
                     *           "name": "CRAFTPULSE_UPLOAD_TOKEN",
                     *           "masked": true
                     *         },
                     *         {
                     *           "name": "CRAFTPULSE_URL",
                     *           "masked": false
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["CiConfiguration"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listBaselines: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Базовые линии. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "go",
                     *           "analysis_kind": "code",
                     *           "state": "ready",
                     *           "report_id": "0193a5b0-d4d4-7000-8000-000000000001",
                     *           "branch": "main",
                     *           "sha": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
                     *           "established_at": "2026-10-10T13:00:00Z",
                     *           "reason_code": null
                     *         },
                     *         {
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "language": "",
                     *           "analysis_kind": "sca",
                     *           "state": "pending",
                     *           "report_id": null,
                     *           "branch": "main",
                     *           "sha": null,
                     *           "established_at": null,
                     *           "reason_code": "full_run_missing"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["BaselineList"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getComponentOverview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                component_id: components["parameters"]["ComponentId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Обзор компонента. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "component": {
                     *         "id": "0193a5b0-7777-7000-8000-000000000001",
                     *         "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *         "repository_area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *         "path_prefix": "services/billing",
                     *         "status": "confirmed",
                     *         "origin": "manifest"
                     *       },
                     *       "findings_by_state": [
                     *         {
                     *           "finding_kind": "code",
                     *           "state": "open",
                     *           "count": {
                     *             "state": "ASSESSED",
                     *             "value": 14,
                     *             "unit": "count",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 14,
                     *             "bound": null,
                     *             "low_sample": true,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": null
                     *           }
                     *         }
                     *       ],
                     *       "facts": [
                     *         {
                     *           "metric_id": "C05",
                     *           "source": "builtin",
                     *           "window": {
                     *             "from": "2026-08-01T00:00:00Z",
                     *             "to": "2026-11-01T00:00:00Z"
                     *           },
                     *           "value": {
                     *             "state": "PROVISIONAL",
                     *             "value": 0.07,
                     *             "unit": "ratio",
                     *             "numerator": 812,
                     *             "denominator": 11600,
                     *             "n": null,
                     *             "bound": null,
                     *             "low_sample": false,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": "history_window_short"
                     *           }
                     *         }
                     *       ],
                     *       "hotspots": [],
                     *       "access_limited": false,
                     *       "access_notes": [],
                     *       "redactions": []
                     *     }
                     */
                    "application/json": components["schemas"]["ComponentOverview"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listFindings: {
        parameters: {
            query?: {
                component_id?: string;
                project_id?: string;
                service_id?: string;
                finding_kind?: "code" | "security" | "secret" | "dependency" | "iac" | "docs" | "openapi";
                state?: "open" | "resolved" | "accepted_risk" | "false_positive" | "not_applicable" | "newly_observed";
                severity?: "critical" | "high" | "medium" | "low" | "info" | "unknown";
                sort?: "severity" | "-severity" | "first_seen_at" | "-first_seen_at";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница находок. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e2e2-7000-8000-000000000001",
                     *           "finding_kind": "security",
                     *           "rule_id": "go.lang.security.audit.sqli",
                     *           "state": "open",
                     *           "severity": {
                     *             "native": "ERROR",
                     *             "mapped": "high",
                     *             "mapping_version": 1
                     *           },
                     *           "confidence": null,
                     *           "location": {
                     *             "path": "services/billing/repo.go",
                     *             "line_start": 88,
                     *             "line_end": 88
                     *           },
                     *           "package": null,
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "first_seen_at": "2026-11-08T11:02:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["FindingPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    getFinding: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                finding_id: components["parameters"]["FindingId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Находка. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *       "finding_kind": "secret",
                     *       "rule_id": "generic-api-key",
                     *       "state": "open",
                     *       "severity": {
                     *         "native": "high",
                     *         "mapped": "high",
                     *         "mapping_version": 1
                     *       },
                     *       "confidence": null,
                     *       "location": {
                     *         "path": "deploy/config.yaml",
                     *         "line_start": 12,
                     *         "line_end": 12
                     *       },
                     *       "package": null,
                     *       "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *       "first_seen_at": "2026-11-08T11:02:00Z",
                     *       "evidence": {
                     *         "snippet": "api_key: [REDACTED]",
                     *         "fingerprint_short": "3f9c2a7b"
                     *       },
                     *       "report": {
                     *         "id": "0193a5b0-d4d4-7000-8000-000000000002",
                     *         "producer": "gitleaks",
                     *         "producer_version": "8.28.0",
                     *         "rules_version": null,
                     *         "db_version": null,
                     *         "sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *         "completeness": "complete"
                     *       },
                     *       "external_triage": [
                     *         {
                     *           "authority": "sonarqube",
                     *           "state": "false_positive",
                     *           "source_url": "https://sonar.example/project/issues?id=billing&issues=AYk1"
                     *         }
                     *       ],
                     *       "exceptions": [],
                     *       "actions": [
                     *         "create_exception",
                     *         "create_tech_debt_task"
                     *       ],
                     *       "redactions": []
                     *     }
                     */
                    "application/json": components["schemas"]["FindingDetail"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    compareTechnicalResults: {
        parameters: {
            query: {
                component_id: string;
                base_sha: string;
                head_sha: string;
                analysis_kind: components["schemas"]["AnalysisKind"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Сравнение. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "comparable": true,
                     *       "incomparable_reasons": [],
                     *       "base": {
                     *         "sha": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
                     *         "report_id": "0193a5b0-d4d4-7000-8000-000000000001"
                     *       },
                     *       "head": {
                     *         "sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *         "report_id": "0193a5b0-d4d4-7000-8000-000000000003"
                     *       },
                     *       "factors": [
                     *         {
                     *           "factor": "code_changed",
                     *           "new_count": 1,
                     *           "resolved_count": 0,
                     *           "finding_ids": [
                     *             "0193a5b0-e2e2-7000-8000-000000000001"
                     *           ]
                     *         },
                     *         {
                     *           "factor": "db_changed",
                     *           "new_count": 2,
                     *           "resolved_count": 0,
                     *           "finding_ids": [
                     *             "0193a5b0-e2e2-7000-8000-000000000003",
                     *             "0193a5b0-e2e2-7000-8000-000000000004"
                     *           ]
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TechnicalComparison"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listExceptions: {
        parameters: {
            query?: {
                state?: "pending" | "active" | "rejected" | "withdrawn" | "expired";
                component_id?: string;
                finding_id?: string;
                sort?: "expires_at" | "-expires_at" | "created_at" | "-created_at";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница исключений. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e3e3-7000-8000-000000000001",
                     *           "state": "pending",
                     *           "current_version": {
                     *             "version_no": 1,
                     *             "decision_type": "accepted_risk",
                     *             "reason": "Ключ тестового стенда, ротация в CORE-77",
                     *             "scope": {
                     *               "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *               "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *               "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *               "rule_id": null,
                     *               "path_prefix": null,
                     *               "package_purl": null
                     *             },
                     *             "effective_from": "2026-11-08T12:00:00Z",
                     *             "expires_at": "2026-12-08T12:00:00Z",
                     *             "approval_required": true,
                     *             "external_authority": null,
                     *             "external_ref": null,
                     *             "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *             "status": "pending",
                     *             "created_at": "2026-11-08T12:00:00Z"
                     *           }
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["ExceptionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    createException: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision_type": "accepted_risk",
                 *       "reason": "Ключ тестового стенда, ротация в CORE-77",
                 *       "scope": {
                 *         "area_id": "0193a5b0-8888-7000-8000-000000000001",
                 *         "component_id": "0193a5b0-7777-7000-8000-000000000001",
                 *         "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                 *         "rule_id": null,
                 *         "path_prefix": null,
                 *         "package_purl": null
                 *       },
                 *       "effective_from": "2026-11-08T12:00:00Z",
                 *       "expires_at": "2026-12-08T12:00:00Z"
                 *     }
                 */
                "application/json": components["schemas"]["ExceptionDraft"];
            };
        };
        responses: {
            /** @description Решение создано. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e3e3-7000-8000-000000000001",
                     *       "state": "pending",
                     *       "current_version": {
                     *         "version_no": 1,
                     *         "decision_type": "accepted_risk",
                     *         "reason": "Ключ тестового стенда, ротация в CORE-77",
                     *         "scope": {
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *           "rule_id": null,
                     *           "path_prefix": null,
                     *           "package_purl": null
                     *         },
                     *         "effective_from": "2026-11-08T12:00:00Z",
                     *         "expires_at": "2026-12-08T12:00:00Z",
                     *         "approval_required": true,
                     *         "external_authority": null,
                     *         "external_ref": null,
                     *         "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *         "status": "pending",
                     *         "created_at": "2026-11-08T12:00:00Z"
                     *       },
                     *       "versions": [],
                     *       "transitions": [
                     *         {
                     *           "version_no": 1,
                     *           "status": "pending",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *           "occurred_at": "2026-11-08T12:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["ExceptionDetail"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getException: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                exception_id: components["parameters"]["ExceptionId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Решение. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e3e3-7000-8000-000000000001",
                     *       "state": "active",
                     *       "current_version": {
                     *         "version_no": 1,
                     *         "decision_type": "accepted_risk",
                     *         "reason": "Ключ тестового стенда, ротация в CORE-77",
                     *         "scope": {
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *           "rule_id": null,
                     *           "path_prefix": null,
                     *           "package_purl": null
                     *         },
                     *         "effective_from": "2026-11-08T12:00:00Z",
                     *         "expires_at": "2026-12-08T12:00:00Z",
                     *         "approval_required": true,
                     *         "external_authority": null,
                     *         "external_ref": null,
                     *         "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *         "status": "approved",
                     *         "created_at": "2026-11-08T12:00:00Z"
                     *       },
                     *       "versions": [],
                     *       "transitions": [
                     *         {
                     *           "version_no": 1,
                     *           "status": "pending",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *           "occurred_at": "2026-11-08T12:00:00Z"
                     *         },
                     *         {
                     *           "version_no": 1,
                     *           "status": "approved",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000005",
                     *           "occurred_at": "2026-11-08T13:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["ExceptionDetail"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createExceptionVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                exception_id: components["parameters"]["ExceptionId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision_type": "accepted_risk",
                 *       "reason": "Ротация перенесена на следующий квартал",
                 *       "scope": {
                 *         "area_id": "0193a5b0-8888-7000-8000-000000000001",
                 *         "component_id": "0193a5b0-7777-7000-8000-000000000001",
                 *         "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                 *         "rule_id": null,
                 *         "path_prefix": null,
                 *         "package_purl": null
                 *       },
                 *       "effective_from": "2026-12-08T12:00:00Z",
                 *       "expires_at": "2027-03-08T12:00:00Z"
                 *     }
                 */
                "application/json": components["schemas"]["ExceptionDraft"];
            };
        };
        responses: {
            /** @description Новая версия решения. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e3e3-7000-8000-000000000001",
                     *       "state": "active",
                     *       "current_version": {
                     *         "version_no": 2,
                     *         "decision_type": "accepted_risk",
                     *         "reason": "Ротация перенесена на следующий квартал",
                     *         "scope": {
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *           "rule_id": null,
                     *           "path_prefix": null,
                     *           "package_purl": null
                     *         },
                     *         "effective_from": "2026-12-08T12:00:00Z",
                     *         "expires_at": "2027-03-08T12:00:00Z",
                     *         "approval_required": true,
                     *         "external_authority": null,
                     *         "external_ref": null,
                     *         "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *         "status": "pending",
                     *         "created_at": "2026-11-20T09:00:00Z"
                     *       },
                     *       "versions": [
                     *         {
                     *           "version_no": 1,
                     *           "decision_type": "accepted_risk",
                     *           "reason": "Ключ тестового стенда, ротация в CORE-77",
                     *           "scope": {
                     *             "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *             "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *             "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *             "rule_id": null,
                     *             "path_prefix": null,
                     *             "package_purl": null
                     *           },
                     *           "effective_from": "2026-11-08T12:00:00Z",
                     *           "expires_at": "2026-12-08T12:00:00Z",
                     *           "approval_required": true,
                     *           "external_authority": null,
                     *           "external_ref": null,
                     *           "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *           "status": "approved",
                     *           "created_at": "2026-11-08T12:00:00Z"
                     *         }
                     *       ],
                     *       "transitions": [
                     *         {
                     *           "version_no": 2,
                     *           "status": "pending",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *           "occurred_at": "2026-11-20T09:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["ExceptionDetail"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    decideException: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                exception_id: components["parameters"]["ExceptionId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision": "approve"
                 *     }
                 */
                "application/json": components["schemas"]["ExceptionDecision"];
            };
        };
        responses: {
            /** @description Решение после перехода. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e3e3-7000-8000-000000000001",
                     *       "state": "active",
                     *       "current_version": {
                     *         "version_no": 1,
                     *         "decision_type": "accepted_risk",
                     *         "reason": "Ключ тестового стенда, ротация в CORE-77",
                     *         "scope": {
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *           "finding_id": "0193a5b0-e2e2-7000-8000-000000000002",
                     *           "rule_id": null,
                     *           "path_prefix": null,
                     *           "package_purl": null
                     *         },
                     *         "effective_from": "2026-11-08T12:00:00Z",
                     *         "expires_at": "2026-12-08T12:00:00Z",
                     *         "approval_required": true,
                     *         "external_authority": null,
                     *         "external_ref": null,
                     *         "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *         "status": "approved",
                     *         "created_at": "2026-11-08T12:00:00Z"
                     *       },
                     *       "versions": [],
                     *       "transitions": [
                     *         {
                     *           "version_no": 1,
                     *           "status": "pending",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *           "occurred_at": "2026-11-08T12:00:00Z"
                     *         },
                     *         {
                     *           "version_no": 1,
                     *           "status": "approved",
                     *           "actor_user_id": "0193a5b0-1111-7000-8000-000000000005",
                     *           "occurred_at": "2026-11-08T13:00:00Z"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["ExceptionDetail"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listDocumentationRoots: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                service_id: components["parameters"]["ServiceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Корни. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e4e4-7000-8000-000000000001",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "path_prefix": "docs",
                     *           "profile_id": "git-markdown-openapi",
                     *           "owner_person_id": "0193a5b0-2222-7000-8000-000000000002",
                     *           "audience": "organization"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["DocumentationRootList"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    replaceDocumentationRoots: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                service_id: components["parameters"]["ServiceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "data": [
                 *         {
                 *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                 *           "path_prefix": "docs",
                 *           "profile_id": "git-markdown-openapi",
                 *           "owner_person_id": "0193a5b0-2222-7000-8000-000000000002",
                 *           "audience": "organization"
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["DocumentationRootListInput"];
            };
        };
        responses: {
            /** @description Сохранённые корни. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e4e4-7000-8000-000000000001",
                     *           "area_id": "0193a5b0-8888-7000-8000-000000000001",
                     *           "path_prefix": "docs",
                     *           "profile_id": "git-markdown-openapi",
                     *           "owner_person_id": "0193a5b0-2222-7000-8000-000000000002",
                     *           "audience": "organization"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["DocumentationRootList"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    listServiceDocuments: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                service_id: components["parameters"]["ServiceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница документов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "documentation_state": "ASSESSED",
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e5e5-7000-8000-000000000001",
                     *           "title": "Billing API",
                     *           "doc_format": "openapi",
                     *           "path": "docs/openapi.yaml",
                     *           "version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                     *           "last_checked_at": "2026-11-08T11:02:00Z",
                     *           "open_violations": 1,
                     *           "source_url": "https://gitlab.example/core/-/blob/main/docs/openapi.yaml"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["DocumentPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    requestDocumentationCheck: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                service_id: components["parameters"]["ServiceId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "root_id": "0193a5b0-e4e4-7000-8000-000000000001"
                 *     }
                 */
                "application/json": components["schemas"]["DocumentationCheckRequest"];
            };
        };
        responses: {
            /** @description Операция принята. Location указывает на getBackgroundOperation; её статус и ошибка относятся к этому запуску. */
            202: {
                headers: {
                    /**
                     * @description Адрес ресурса принятой операции.
                     * @example /api/v1/operations/0193a5b0-a0a0-7000-8000-000000000001
                     */
                    Location?: string;
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-a0a0-7000-8000-000000000001",
                     *       "kind": "documentation_check",
                     *       "status": "queued",
                     *       "created_at": "2026-11-08T12:00:00Z",
                     *       "updated_at": "2026-11-08T12:00:00Z",
                     *       "result_uri": null,
                     *       "error": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["BackgroundOperation"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getOrganizationReport: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                /** @description Шаг ряда. */
                step?: components["parameters"]["Step"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Отчёт по командам. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "period": {
                     *         "from": "2026-10-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       },
                     *       "teams": [
                     *         {
                     *           "team": {
                     *             "id": "0193a5b0-3333-7000-8000-000000000001",
                     *             "name": "Платформа",
                     *             "status": "active"
                     *           },
                     *           "revision_id": "0193a5b0-a1a1-7000-8000-000000000002",
                     *           "metrics": [
                     *             {
                     *               "metric_id": "T05",
                     *               "definition_version": 1,
                     *               "unit": "count",
                     *               "points": []
                     *             }
                     *           ]
                     *         }
                     *       ],
                     *       "access_limited": true,
                     *       "access_notes": [
                     *         {
                     *           "code": "restricted_by_access",
                     *           "connection_id": null
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["OrganizationReport"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    listMirrorCandidates: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница кандидатов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "left": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000010",
                     *             "display_key": "CORE-40",
                     *             "title": "Отчёт",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-40"
                     *           },
                     *           "right": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000011",
                     *             "display_key": "#812",
                     *             "title": "Отчёт",
                     *             "source_url": "https://redmine.example/issues/812"
                     *           },
                     *           "basis": "explicit_link"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["MirrorCandidatePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listMirrorGroups: {
        parameters: {
            query: {
                project_id: string;
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница групп. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e6e6-7000-8000-000000000001",
                     *           "members": [
                     *             "0193a5b0-f000-7000-8000-000000000010",
                     *             "0193a5b0-f000-7000-8000-000000000011"
                     *           ],
                     *           "status_authority_task_id": "0193a5b0-f000-7000-8000-000000000010",
                     *           "conflicts": [
                     *             {
                     *               "field": "status",
                     *               "values": [
                     *                 {
                     *                   "task_id": "0193a5b0-f000-7000-8000-000000000010",
                     *                   "value": "active"
                     *                 },
                     *                 {
                     *                   "task_id": "0193a5b0-f000-7000-8000-000000000011",
                     *                   "value": "done"
                     *                 }
                     *               ]
                     *             }
                     *           ],
                     *           "confirmed_at": "2026-11-08T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["MirrorGroupPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createMirrorGroup: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "task_ids": [
                 *         "0193a5b0-f000-7000-8000-000000000010",
                 *         "0193a5b0-f000-7000-8000-000000000011"
                 *       ],
                 *       "status_authority_task_id": "0193a5b0-f000-7000-8000-000000000010"
                 *     }
                 */
                "application/json": components["schemas"]["MirrorGroupCreate"];
            };
        };
        responses: {
            /** @description Группа подтверждена. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e6e6-7000-8000-000000000001",
                     *       "members": [
                     *         "0193a5b0-f000-7000-8000-000000000010",
                     *         "0193a5b0-f000-7000-8000-000000000011"
                     *       ],
                     *       "status_authority_task_id": "0193a5b0-f000-7000-8000-000000000010",
                     *       "conflicts": [],
                     *       "confirmed_at": "2026-11-08T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["MirrorGroup"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    deleteMirrorGroup: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                mirror_group_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Подтверждение снято. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listSprints: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница спринтов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e7e7-7000-8000-000000000001",
                     *           "name": "Спринт 21",
                     *           "state": "closed",
                     *           "started_at": "2026-10-01T09:00:00Z",
                     *           "ended_at": "2026-10-15T18:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["SprintPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getSprint: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                sprint_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Спринт. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e7e7-7000-8000-000000000001",
                     *       "name": "Спринт 21",
                     *       "state": "closed",
                     *       "started_at": "2026-10-01T09:00:00Z",
                     *       "ended_at": "2026-10-15T18:00:00Z",
                     *       "plan_state": "ASSESSED",
                     *       "initial_plan": [
                     *         "0193a5b0-f000-7000-8000-000000000020",
                     *         "0193a5b0-f000-7000-8000-000000000021"
                     *       ],
                     *       "changes": [
                     *         {
                     *           "task_id": "0193a5b0-f000-7000-8000-000000000022",
                     *           "kind": "added",
                     *           "event_time": "2026-10-02T10:00:00Z"
                     *         },
                     *         {
                     *           "task_id": "0193a5b0-f000-7000-8000-000000000022",
                     *           "kind": "removed",
                     *           "event_time": "2026-10-03T10:00:00Z"
                     *         }
                     *       ],
                     *       "completion": {
                     *         "state": "ASSESSED",
                     *         "value": 0.5,
                     *         "unit": "ratio",
                     *         "numerator": 1,
                     *         "denominator": 2,
                     *         "n": 2,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["SprintDetail"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listDodVersions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
                /** @description Тип задачи для редактирования своей версии DoD. */
                task_type?: string;
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница версий. */
            200: {
                headers: {
                    /**
                     * @description Возвращается только при task_type: версия данного типа или "0", если версий нет.
                     * @example "1"
                     */
                    ETag?: string;
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "version_no": 1,
                     *           "base_version_no": null,
                     *           "task_type": "Story",
                     *           "mapping_version_no": 2,
                     *           "obligations": [
                     *             {
                     *               "obligation_no": 1,
                     *               "evidence_kind": "check",
                     *               "source_rule": {
                     *                 "analysis_kind": "junit"
                     *               },
                     *               "required": true
                     *             },
                     *             {
                     *               "obligation_no": 2,
                     *               "evidence_kind": "business_signoff",
                     *               "source_rule": {
                     *                 "field": "customfield_10050"
                     *               },
                     *               "required": true
                     *             }
                     *           ],
                     *           "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *           "confirmed_at": "2026-11-08T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["DodVersionPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    createDodVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "task_type": "Story",
                 *       "obligations": [
                 *         {
                 *           "evidence_kind": "check",
                 *           "source_rule": {
                 *             "analysis_kind": "junit"
                 *           },
                 *           "required": true
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["DodDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "task_type": "Story",
                     *       "mapping_version_no": 2,
                     *       "obligations": [
                     *         {
                     *           "obligation_no": 1,
                     *           "evidence_kind": "check",
                     *           "source_rule": {
                     *             "analysis_kind": "junit"
                     *           },
                     *           "required": true
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-09T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["DodVersion"];
                };
            };
            /** @description Новая версия DoD. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "task_type": "Story",
                     *       "mapping_version_no": 2,
                     *       "obligations": [
                     *         {
                     *           "obligation_no": 1,
                     *           "evidence_kind": "check",
                     *           "source_rule": {
                     *             "analysis_kind": "junit"
                     *           },
                     *           "required": true
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000003",
                     *       "confirmed_at": "2026-11-09T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["DodVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    getTaskDod: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                task_id: components["parameters"]["TaskId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Обязательства и основания. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "task_id": "0193a5b0-f000-7000-8000-000000000002",
                     *       "dod_version_no": 1,
                     *       "verified": false,
                     *       "obligations": [
                     *         {
                     *           "obligation_no": 1,
                     *           "evidence_kind": "check",
                     *           "state": "verified",
                     *           "basis": [
                     *             {
                     *               "kind": "pipeline",
                     *               "id": "0193a5b0-d1d1-7000-8000-000000000001",
                     *               "display_key": "#7781",
                     *               "title": null,
                     *               "source_url": "https://gitlab.example/core/-/pipelines/7781"
                     *             }
                     *           ],
                     *           "reason_code": null
                     *         },
                     *         {
                     *           "obligation_no": 2,
                     *           "evidence_kind": "business_signoff",
                     *           "state": "missing",
                     *           "basis": [],
                     *           "reason_code": "manual_signoff_missing"
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["TaskDod"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listInitiatives: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница инициатив. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-e8e8-7000-8000-000000000001",
                     *           "name": "Ревью в течение дня",
                     *           "effective_at": "2026-09-01T00:00:00Z",
                     *           "team_ids": [
                     *             "0193a5b0-3333-7000-8000-000000000001"
                     *           ]
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["InitiativePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    createInitiative: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "name": "Ревью в течение дня",
                 *       "effective_at": "2026-09-01T00:00:00Z",
                 *       "team_ids": [
                 *         "0193a5b0-3333-7000-8000-000000000001"
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["InitiativeCreate"];
            };
        };
        responses: {
            /** @description Инициатива сохранена. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e8e8-7000-8000-000000000001",
                     *       "name": "Ревью в течение дня",
                     *       "effective_at": "2026-09-01T00:00:00Z",
                     *       "team_ids": [
                     *         "0193a5b0-3333-7000-8000-000000000001"
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["Initiative"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getInitiativeComparison: {
        parameters: {
            query: {
                before_from: string;
                after_to: string;
            };
            header?: never;
            path: {
                initiative_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Сравнение. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "initiative_id": "0193a5b0-e8e8-7000-8000-000000000001",
                     *       "metrics": [
                     *         {
                     *           "metric_id": "T08",
                     *           "comparable": true,
                     *           "incomparable_reasons": [],
                     *           "before": {
                     *             "state": "ASSESSED",
                     *             "value": 432000,
                     *             "unit": "seconds",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 31,
                     *             "bound": null,
                     *             "low_sample": false,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": null
                     *           },
                     *           "after": {
                     *             "state": "ASSESSED",
                     *             "value": 345600,
                     *             "unit": "seconds",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 28,
                     *             "bound": null,
                     *             "low_sample": false,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": null
                     *           },
                     *           "composition_changed": true
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["InitiativeComparison"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getDeliveryChain: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                task_id: components["parameters"]["TaskId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Цепочка. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "epic_id": "0193a5b0-f000-7000-8000-000000000030",
                     *       "items": [
                     *         {
                     *           "task": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000002",
                     *             "display_key": "CORE-15",
                     *             "title": "Перенести отчёт",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-15"
                     *           },
                     *           "last_confirmed_link": "merge_request",
                     *           "gap_reason": "deployment_missing",
                     *           "links": []
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["DeliveryChain"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listCheckPolicyVersions: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница версий. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "version_no": 1,
                     *           "base_version_no": null,
                     *           "unknown_behavior": "fail",
                     *           "bypass_authority": "tech_lead",
                     *           "rules": [
                     *             {
                     *               "rule_no": 1,
                     *               "category": "new_verified_secret",
                     *               "condition": {}
                     *             }
                     *           ],
                     *           "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *           "confirmed_at": "2026-11-08T12:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["CheckPolicyPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createCheckPolicyVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description Значение `ETag` прочитанной версии ресурса. */
                "If-Match": components["parameters"]["IfMatch"];
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "unknown_behavior": "fail",
                 *       "bypass_authority": "tech_lead",
                 *       "rules": [
                 *         {
                 *           "category": "new_verified_secret",
                 *           "condition": {}
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["CheckPolicyDraft"];
            };
        };
        responses: {
            /** @description Содержимое совпало с действующей версией; новая версия не создана. */
            200: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "unknown_behavior": "fail",
                     *       "bypass_authority": "tech_lead",
                     *       "rules": [
                     *         {
                     *           "rule_no": 1,
                     *           "category": "new_verified_secret",
                     *           "condition": {}
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *       "confirmed_at": "2026-11-09T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CheckPolicyVersion"];
                };
            };
            /** @description Новая версия политики. */
            201: {
                headers: {
                    ETag: components["headers"]["ETag"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "base_version_no": 1,
                     *       "unknown_behavior": "fail",
                     *       "bypass_authority": "tech_lead",
                     *       "rules": [
                     *         {
                     *           "rule_no": 1,
                     *           "category": "new_verified_secret",
                     *           "condition": {}
                     *         }
                     *       ],
                     *       "confirmed_by": "0193a5b0-1111-7000-8000-000000000004",
                     *       "confirmed_at": "2026-11-09T12:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["CheckPolicyVersion"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            412: components["responses"]["PreconditionFailed"];
            422: components["responses"]["ValidationFailed"];
            428: components["responses"]["PreconditionRequired"];
        };
    };
    createPolicyBypass: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "merge_request_id": "0193a5b0-b0b0-7000-8000-000000000001",
                 *       "policy_version_no": 1,
                 *       "reason": "Срочное исправление инцидента INC-12",
                 *       "expires_at": "2026-11-09T12:00:00Z"
                 *     }
                 */
                "application/json": components["schemas"]["PolicyBypassCreate"];
            };
        };
        responses: {
            /** @description Обход сохранён. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e3e3-7000-8000-000000000009",
                     *       "merge_request_id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *       "policy_version_no": 1,
                     *       "reason": "Срочное исправление инцидента INC-12",
                     *       "expires_at": "2026-11-09T12:00:00Z",
                     *       "author_user_id": "0193a5b0-1111-7000-8000-000000000004",
                     *       "created_at": "2026-11-08T12:30:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["PolicyBypass"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listPolicyEvaluations: {
        parameters: {
            query: {
                project_id: string;
                outcome?: "pass" | "fail" | "bypassed" | "unauthorized_merge" | "no_merge_gate";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница вердиктов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "merge_request_id": "0193a5b0-b0b0-7000-8000-000000000001",
                     *           "head_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *           "policy_version_no": 1,
                     *           "outcome": "bypassed",
                     *           "bypass_id": "0193a5b0-e3e3-7000-8000-000000000009",
                     *           "evaluated_at": "2026-11-08T12:35:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["PolicyEvaluationPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createTechDebtDraft: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "finding_ids": [
                 *         "0193a5b0-e2e2-7000-8000-000000000001"
                 *       ],
                 *       "tracker_area_id": "0193a5b0-8888-7000-8000-000000000002",
                 *       "issue_type": "Task",
                 *       "title": "Устранить SQL-инъекцию в repo.go",
                 *       "body": "Находка правила go.lang.security.audit.sqli в services/billing/repo.go:88"
                 *     }
                 */
                "application/json": components["schemas"]["TechDebtDraftInput"];
            };
        };
        responses: {
            /** @description Черновик. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e9e9-7000-8000-000000000001",
                     *       "finding_ids": [
                     *         "0193a5b0-e2e2-7000-8000-000000000001"
                     *       ],
                     *       "tracker_area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *       "issue_type": "Task",
                     *       "title": "Устранить SQL-инъекцию в repo.go",
                     *       "body": "Находка правила go.lang.security.audit.sqli в services/billing/repo.go:88",
                     *       "status": "draft",
                     *       "external_task": null,
                     *       "error_code": null,
                     *       "retry_after_seconds": null
                     *     }
                     */
                    "application/json": components["schemas"]["TechDebtDraft"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getTechDebtDraft: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                draft_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Черновик. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e9e9-7000-8000-000000000001",
                     *       "finding_ids": [
                     *         "0193a5b0-e2e2-7000-8000-000000000001"
                     *       ],
                     *       "tracker_area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *       "issue_type": "Task",
                     *       "title": "Устранить SQL-инъекцию в repo.go",
                     *       "body": "Находка правила go.lang.security.audit.sqli в services/billing/repo.go:88",
                     *       "status": "succeeded",
                     *       "external_task": {
                     *         "kind": "task",
                     *         "id": "0193a5b0-f000-7000-8000-000000000040",
                     *         "display_key": "CORE-88",
                     *         "title": "Устранить SQL-инъекцию в repo.go",
                     *         "source_url": "https://example.atlassian.net/browse/CORE-88"
                     *       },
                     *       "error_code": null,
                     *       "retry_after_seconds": null
                     *     }
                     */
                    "application/json": components["schemas"]["TechDebtDraft"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    confirmTechDebtDraft: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                draft_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Запись поставлена; исход - в черновике. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-e9e9-7000-8000-000000000001",
                     *       "finding_ids": [
                     *         "0193a5b0-e2e2-7000-8000-000000000001"
                     *       ],
                     *       "tracker_area_id": "0193a5b0-8888-7000-8000-000000000002",
                     *       "issue_type": "Task",
                     *       "title": "Устранить SQL-инъекцию в repo.go",
                     *       "body": "Находка правила go.lang.security.audit.sqli в services/billing/repo.go:88",
                     *       "status": "pending",
                     *       "external_task": null,
                     *       "error_code": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["TechDebtDraft"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listArtifacts: {
        parameters: {
            query?: {
                service_id?: string;
                affected?: "affected" | "not_affected" | "unknown";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница артефактов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-eaea-7000-8000-000000000001",
                     *           "digest": "sha256:6b2f0c1d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c",
                     *           "source_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *           "sbom_completeness": "complete",
                     *           "affected": "affected",
                     *           "db_version": "trivy-db 2026-11-08",
                     *           "last_assessed_at": "2026-11-08T04:00:00Z"
                     *         }
                     *       ],
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["ArtifactPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    getArtifactPassport: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                artifact_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Паспорт. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-eaea-7000-8000-000000000001",
                     *       "digest": "sha256:6b2f0c1d9e8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a9b8c7d6e5f4a3b2c",
                     *       "source_sha": "9f2c1e0d8b7a6f5e4d3c2b1a0f9e8d7c6b5a4f3e",
                     *       "sbom_completeness": "complete",
                     *       "deployments": [
                     *         {
                     *           "id": "0193a5b0-ebeb-7000-8000-000000000001",
                     *           "environment": "production",
                     *           "decided_at": "2026-11-04T10:00:00Z",
                     *           "deployed_at": "2026-11-05T10:00:00Z",
                     *           "outcome": "succeeded",
                     *           "exposure": "public"
                     *         }
                     *       ],
                     *       "checks_before_decision": [
                     *         {
                     *           "report_id": "0193a5b0-d4d4-7000-8000-000000000003",
                     *           "analysis_kind": "sca",
                     *           "finished_at": "2026-11-03T10:00:00Z"
                     *         }
                     *       ],
                     *       "later_checks": [
                     *         {
                     *           "report_id": "0193a5b0-d4d4-7000-8000-000000000004",
                     *           "analysis_kind": "sca",
                     *           "finished_at": "2026-11-06T10:00:00Z"
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["ArtifactPassport"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listDocumentationObligations: {
        parameters: {
            query?: {
                service_id?: string;
                status?: "candidate" | "open" | "closed" | "rejected";
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница обязательств. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-ecec-7000-8000-000000000001",
                     *           "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *           "contract_document_id": "0193a5b0-e5e5-7000-8000-000000000001",
                     *           "contract_version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                     *           "status": "candidate",
                     *           "documents": [],
                     *           "owner_person_id": "0193a5b0-2222-7000-8000-000000000002"
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["DocumentationObligationPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
        };
    };
    decideDocumentationObligation: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                obligation_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision": "confirm"
                 *     }
                 */
                "application/json": components["schemas"]["CandidateDecision"];
            };
        };
        responses: {
            /** @description Обязательство после решения. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-ecec-7000-8000-000000000001",
                     *       "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *       "contract_document_id": "0193a5b0-e5e5-7000-8000-000000000001",
                     *       "contract_version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                     *       "status": "open",
                     *       "documents": [],
                     *       "owner_person_id": "0193a5b0-2222-7000-8000-000000000002"
                     *     }
                     */
                    "application/json": components["schemas"]["DocumentationObligation"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getHandoverPackage: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Комплект. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *       "services": [
                     *         {
                     *           "service": {
                     *             "id": "0193a5b0-6666-7000-8000-000000000001",
                     *             "name": "billing",
                     *             "status": "active"
                     *           },
                     *           "owner_person_id": null,
                     *           "gaps": [
                     *             "owner_missing",
                     *             "runbook_review_missing"
                     *           ]
                     *         }
                     *       ],
                     *       "decisions": [],
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["HandoverPackage"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    decideHandover: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "decision": "request_additions",
                 *       "requested_additions": [
                 *         "Назначить владельца billing",
                 *         "Провести ревью runbook"
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["HandoverDecisionInput"];
            };
        };
        responses: {
            /** @description Решение сохранено. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-eded-7000-8000-000000000001",
                     *       "decision": "request_additions",
                     *       "requested_additions": [
                     *         "Назначить владельца billing",
                     *         "Провести ревью runbook"
                     *       ],
                     *       "decided_by": "0193a5b0-1111-7000-8000-000000000006",
                     *       "decided_at": "2026-11-08T13:00:00Z"
                     *     }
                     */
                    "application/json": components["schemas"]["HandoverDecision"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getReviewRoute: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                merge_request_id: components["parameters"]["MergeRequestId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Маршрут. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "method_run_id": "0193a5b0-f1f1-7000-8000-000000000001",
                     *       "method": {
                     *         "method_id": "review-route",
                     *         "method_version": 1
                     *       },
                     *       "places": [
                     *         {
                     *           "location": {
                     *             "path": "services/billing/repo.go",
                     *             "line_start": 80,
                     *             "line_end": 96
                     *           },
                     *           "reasons": [
                     *             "finding",
                     *             "hotspot"
                     *           ]
                     *         }
                     *       ],
                     *       "groups": [
                     *         {
                     *           "person_ids": [
                     *             "0193a5b0-2222-7000-8000-000000000001"
                     *           ],
                     *           "reasons": [
                     *             "co_change_history"
                     *           ]
                     *         }
                     *       ],
                     *       "history_window": {
                     *         "from": "2026-05-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["ReviewRoute"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createReviewRouteFeedback: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                merge_request_id: components["parameters"]["MergeRequestId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "method_run_id": "0193a5b0-f1f1-7000-8000-000000000001",
                 *       "verdict": "rejected"
                 *     }
                 */
                "application/json": components["schemas"]["RecommendationFeedbackInput"];
            };
        };
        responses: {
            /** @description Отметка сохранена. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    createForecast: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *       "question": "completion_date",
                 *       "target_count": 20,
                 *       "history": {
                 *         "from": "2026-05-01T00:00:00Z",
                 *         "to": "2026-11-01T00:00:00Z"
                 *       }
                 *     }
                 */
                "application/json": components["schemas"]["ForecastRequest"];
            };
        };
        responses: {
            /** @description Прогноз поставлен. */
            202: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f2f2-7000-8000-000000000001",
                     *       "status": "pending",
                     *       "method": {
                     *         "method_id": "monte-carlo-throughput",
                     *         "method_version": 1
                     *       },
                     *       "distribution": [],
                     *       "backtest_error": null,
                     *       "assumptions": [],
                     *       "refusal_reason": null,
                     *       "retry_after_seconds": 3
                     *     }
                     */
                    "application/json": components["schemas"]["Forecast"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getForecast: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                forecast_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Прогноз. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f2f2-7000-8000-000000000001",
                     *       "status": "succeeded",
                     *       "method": {
                     *         "method_id": "monte-carlo-throughput",
                     *         "method_version": 1
                     *       },
                     *       "distribution": [
                     *         {
                     *           "probability": 0.5,
                     *           "date": "2026-12-01T00:00:00Z",
                     *           "count": null
                     *         },
                     *         {
                     *           "probability": 0.85,
                     *           "date": "2026-12-15T00:00:00Z",
                     *           "count": null
                     *         }
                     *       ],
                     *       "backtest_error": 0.18,
                     *       "assumptions": [
                     *         "scope_stable",
                     *         "team_stable"
                     *       ],
                     *       "refusal_reason": null,
                     *       "retry_after_seconds": null
                     *     }
                     */
                    "application/json": components["schemas"]["Forecast"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createTestSelectionExperiment: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                project_id: components["parameters"]["ProjectId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "component_id": "0193a5b0-7777-7000-8000-000000000001",
                 *       "method_id": "changed-files-map",
                 *       "method_version": 1
                 *     }
                 */
                "application/json": components["schemas"]["TestSelectionExperimentCreate"];
            };
        };
        responses: {
            /** @description Эксперимент включён. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f3f3-7000-8000-000000000001",
                     *       "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *       "mode": "shadow",
                     *       "runs_compared": 0,
                     *       "missed_failures": {
                     *         "state": "NOT_EVALUATED",
                     *         "value": null,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 0,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": "no_runs_yet"
                     *       },
                     *       "estimated_savings_seconds": null,
                     *       "stopped_at": null
                     *     }
                     */
                    "application/json": components["schemas"]["TestSelectionExperiment"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getTestSelectionExperiment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                experiment_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Эксперимент. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f3f3-7000-8000-000000000001",
                     *       "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *       "mode": "shadow",
                     *       "runs_compared": 40,
                     *       "missed_failures": {
                     *         "state": "ASSESSED",
                     *         "value": 1,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 40,
                     *         "bound": null,
                     *         "low_sample": false,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "estimated_savings_seconds": 5400,
                     *       "stopped_at": null
                     *     }
                     */
                    "application/json": components["schemas"]["TestSelectionExperiment"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    stopTestSelectionExperiment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                experiment_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Эксперимент остановлен; сохранённый отчёт остаётся доступен. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f3f3-7000-8000-000000000001",
                     *       "component_id": "0193a5b0-7777-7000-8000-000000000001",
                     *       "mode": "shadow",
                     *       "stopped_at": "2026-11-09T12:00:00Z",
                     *       "runs_compared": 40,
                     *       "missed_failures": {
                     *         "state": "ASSESSED",
                     *         "value": 1,
                     *         "unit": "count",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 40,
                     *         "bound": null,
                     *         "low_sample": false,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "estimated_savings_seconds": 5400
                     *     }
                     */
                    "application/json": components["schemas"]["TestSelectionExperiment"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
    listRunbookDrills: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                document_id: components["parameters"]["DocumentId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница учений. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "id": "0193a5b0-f4f4-7000-8000-000000000001",
                     *           "version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                     *           "scenario": "Отказ базы",
                     *           "held_at": "2026-10-20T10:00:00Z",
                     *           "outcome": "failed",
                     *           "stopped_at_step": 4,
                     *           "participant_person_ids": [
                     *             "0193a5b0-2222-7000-8000-000000000002"
                     *           ]
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["RunbookDrillPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createRunbookDrill: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path: {
                document_id: components["parameters"]["DocumentId"];
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                 *       "scenario": "Отказ базы",
                 *       "held_at": "2026-10-20T10:00:00Z",
                 *       "outcome": "failed",
                 *       "stopped_at_step": 4,
                 *       "participant_person_ids": [
                 *         "0193a5b0-2222-7000-8000-000000000002"
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["RunbookDrillInput"];
            };
        };
        responses: {
            /** @description Учение записано. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f4f4-7000-8000-000000000001",
                     *       "version_ref": "7d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e",
                     *       "scenario": "Отказ базы",
                     *       "held_at": "2026-10-20T10:00:00Z",
                     *       "outcome": "failed",
                     *       "stopped_at_step": 4,
                     *       "participant_person_ids": [
                     *         "0193a5b0-2222-7000-8000-000000000002"
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["RunbookDrill"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    searchDocuments: {
        parameters: {
            query: {
                q: string;
                service_id?: string;
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Результаты поиска. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "query_id": "0193a5b0-f5f5-7000-8000-000000000001",
                     *       "data": [
                     *         {
                     *           "document": {
                     *             "kind": "document",
                     *             "id": "0193a5b0-e5e5-7000-8000-000000000002",
                     *             "display_key": null,
                     *             "title": "Восстановление базы",
                     *             "source_url": "https://gitlab.example/core/-/blob/main/docs/runbook.md"
                     *           },
                     *           "excerpt": "Восстановление базы из копии выполняется по шагам..."
                     *         }
                     *       ]
                     *     }
                     */
                    "application/json": components["schemas"]["SearchResult"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    createSearchFeedback: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                query_id: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "document_id": "0193a5b0-e5e5-7000-8000-000000000002",
                 *       "verdict": "helped"
                 *     }
                 */
                "application/json": components["schemas"]["SearchFeedbackInput"];
            };
        };
        responses: {
            /** @description Отметка сохранена. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    createSurvey: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                 *       "question_set_version": 1,
                 *       "opens_at": "2026-12-01T09:00:00Z",
                 *       "closes_at": "2026-12-05T18:00:00Z"
                 *     }
                 */
                "application/json": components["schemas"]["SurveyCreate"];
            };
        };
        responses: {
            /** @description Опрос создан. */
            201: {
                headers: {
                    Location: components["headers"]["Location"];
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f6f6-7000-8000-000000000001",
                     *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *       "question_set_version": 1,
                     *       "opens_at": "2026-12-01T09:00:00Z",
                     *       "closes_at": "2026-12-05T18:00:00Z",
                     *       "status": "scheduled",
                     *       "summary": null,
                     *       "published": false
                     *     }
                     */
                    "application/json": components["schemas"]["Survey"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getSurvey: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                survey_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Опрос. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f6f6-7000-8000-000000000001",
                     *       "team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *       "question_set_version": 1,
                     *       "opens_at": "2026-12-01T09:00:00Z",
                     *       "closes_at": "2026-12-05T18:00:00Z",
                     *       "status": "closed",
                     *       "summary": null,
                     *       "published": false
                     *     }
                     */
                    "application/json": components["schemas"]["Survey"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getSurveyForm: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Форма. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "survey_id": "0193a5b0-f6f6-7000-8000-000000000001",
                     *       "closes_at": "2026-12-05T18:00:00Z",
                     *       "questions": [
                     *         {
                     *           "question_id": "q1",
                     *           "text": "Насколько легко было завершать задачи на этой неделе?",
                     *           "scale_min": 1,
                     *           "scale_max": 5
                     *         }
                     *       ],
                     *       "already_answered": false
                     *     }
                     */
                    "application/json": components["schemas"]["SurveyForm"];
                };
            };
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    submitSurveyResponse: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "answers": [
                 *         {
                 *           "question_id": "q1",
                 *           "value": 4
                 *         }
                 *       ]
                 *     }
                 */
                "application/json": components["schemas"]["SurveyResponseInput"];
            };
        };
        responses: {
            /** @description Ответ сохранён. */
            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            409: components["responses"]["Conflict"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getDeliveryStability: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
                revision_id?: components["parameters"]["RevisionQuery"];
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                service_id: components["parameters"]["ServiceId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Показатели стабильности. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "service_id": "0193a5b0-6666-7000-8000-000000000001",
                     *       "period": {
                     *         "from": "2026-10-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       },
                     *       "change_failure_rate": {
                     *         "state": "ASSESSED",
                     *         "value": 0.5,
                     *         "unit": "ratio",
                     *         "numerator": 1,
                     *         "denominator": 2,
                     *         "n": 2,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "recovery_p50_seconds": {
                     *         "state": "ASSESSED",
                     *         "value": 86400,
                     *         "unit": "seconds",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 1,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "incident_link_candidates": [
                     *         {
                     *           "incident": {
                     *             "kind": "incident",
                     *             "id": "0193a5b0-f7f7-7000-8000-000000000001",
                     *             "display_key": "INC-12",
                     *             "title": "Ошибки оплаты",
                     *             "source_url": "https://example.atlassian.net/browse/INC-12"
                     *           },
                     *           "link_id": "0193a5b0-c0c0-7000-8000-000000000009",
                     *           "deployment": {
                     *             "kind": "deployment",
                     *             "id": "0193a5b0-f8f8-7000-8000-000000000001",
                     *             "display_key": "deploy-17",
                     *             "title": "Выпуск billing",
                     *             "source_url": "https://gitlab.example/core/-/environments/1"
                     *           },
                     *           "artifact_id": "0193a5b0-f9f9-7000-8000-000000000001",
                     *           "relation": "caused_failure",
                     *           "started_at": null,
                     *           "restored_at": null
                     *         }
                     *       ],
                     *       "access_limited": false,
                     *       "access_notes": [],
                     *       "recovery_p85_seconds": {
                     *         "state": "ASSESSED",
                     *         "value": 86400,
                     *         "unit": "seconds",
                     *         "numerator": null,
                     *         "denominator": null,
                     *         "n": 1,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "unplanned_deployment_rate": {
                     *         "state": "ASSESSED",
                     *         "value": 0.5,
                     *         "unit": "ratio",
                     *         "numerator": 1,
                     *         "denominator": 2,
                     *         "n": 2,
                     *         "bound": null,
                     *         "low_sample": true,
                     *         "excluded_count": 0,
                     *         "missing_count": 0,
                     *         "reason_code": null
                     *       },
                     *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000041",
                     *       "confirmed_links": {
                     *         "data": [
                     *           {
                     *             "deployment": {
                     *               "kind": "deployment",
                     *               "id": "0193a5b0-f8f8-7000-8000-000000000001",
                     *               "display_key": "deploy-17",
                     *               "title": "Выпуск billing",
                     *               "source_url": "https://gitlab.example/core/-/environments/1"
                     *             },
                     *             "artifact_id": "0193a5b0-f9f9-7000-8000-000000000001",
                     *             "incident": {
                     *               "kind": "incident",
                     *               "id": "0193a5b0-f7f7-7000-8000-000000000002",
                     *               "display_key": "INC-11",
                     *               "title": "Отказ после выпуска",
                     *               "source_url": "https://example.atlassian.net/browse/INC-11"
                     *             },
                     *             "link_id": "0193a5b0-c0c0-7000-8000-000000000010",
                     *             "started_at": "2026-10-10T00:00:00Z",
                     *             "restored_at": "2026-10-11T00:00:00Z",
                     *             "relation": "caused_failure"
                     *           },
                     *           {
                     *             "deployment": {
                     *               "kind": "deployment",
                     *               "id": "0193a5b0-f8f8-7000-8000-000000000002",
                     *               "display_key": "deploy-18",
                     *               "title": "Внеплановое исправление billing",
                     *               "source_url": "https://gitlab.example/core/-/environments/1"
                     *             },
                     *             "artifact_id": "0193a5b0-f9f9-7000-8000-000000000002",
                     *             "incident": {
                     *               "kind": "incident",
                     *               "id": "0193a5b0-f7f7-7000-8000-000000000002",
                     *               "display_key": "INC-11",
                     *               "title": "Отказ после выпуска",
                     *               "source_url": "https://example.atlassian.net/browse/INC-11"
                     *             },
                     *             "link_id": "0193a5b0-c0c0-7000-8000-000000000011",
                     *             "started_at": "2026-10-10T00:00:00Z",
                     *             "restored_at": "2026-10-11T00:00:00Z",
                     *             "relation": "response_to_incident"
                     *           }
                     *         ],
                     *         "next_cursor": null
                     *       }
                     *     }
                     */
                    "application/json": components["schemas"]["DeliveryStability"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    getCiCost: {
        parameters: {
            query: {
                project_id: string;
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                /** @description Конкретная ревизия расчёта. Без параметра выдаётся актуальная ревизия ряда (FR-009). */
                revision_id?: components["parameters"]["RevisionQuery"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Расход и стоимость. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "project_id": "0193a5b0-4444-7000-8000-000000000001",
                     *       "period": {
                     *         "from": "2026-10-01T00:00:00Z",
                     *         "to": "2026-11-01T00:00:00Z"
                     *       },
                     *       "usage": [
                     *         {
                     *           "runner_class": "linux-medium",
                     *           "rate": {
                     *             "version_no": 1,
                     *             "runner_class": "linux-medium",
                     *             "unit": "minute",
                     *             "amount": "2.0000",
                     *             "currency": "RUB",
                     *             "valid_from": "2026-10-01T00:00:00Z",
                     *             "valid_to": null
                     *           },
                     *           "job_seconds": {
                     *             "state": "ASSESSED",
                     *             "value": 600,
                     *             "unit": "seconds",
                     *             "numerator": null,
                     *             "denominator": null,
                     *             "n": 12,
                     *             "bound": null,
                     *             "low_sample": true,
                     *             "excluded_count": 0,
                     *             "missing_count": 0,
                     *             "reason_code": null
                     *           },
                     *           "estimate": {
                     *             "amount": "20.0000",
                     *             "currency": "RUB",
                     *             "rate_version_no": 1
                     *           },
                     *           "basis_revision_id": "0193a5b0-a1a1-7000-8000-000000000043",
                     *           "point_start": "2026-10-01T00:00:00Z"
                     *         }
                     *       ],
                     *       "fixed_fees": [
                     *         {
                     *           "amount": "5.0000",
                     *           "currency": "RUB"
                     *         }
                     *       ],
                     *       "invoices": [
                     *         {
                     *           "invoice_ref": "INV-2026-10",
                     *           "amount": "30.0000",
                     *           "currency": "RUB"
                     *         }
                     *       ],
                     *       "revision_id": "0193a5b0-a1a1-7000-8000-000000000042",
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["CiCost"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    listCiRates: {
        parameters: {
            query?: {
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница ставок. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "version_no": 1,
                     *           "runner_class": "linux-medium",
                     *           "unit": "minute",
                     *           "amount": "2.0000",
                     *           "currency": "RUB",
                     *           "valid_from": "2026-01-01T00:00:00Z",
                     *           "valid_to": null
                     *         }
                     *       ],
                     *       "next_cursor": null
                     *     }
                     */
                    "application/json": components["schemas"]["CiRatePage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
        };
    };
    createCiRateVersion: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "runner_class": "linux-medium",
                 *       "unit": "minute",
                 *       "amount": "2.5000",
                 *       "currency": "RUB",
                 *       "valid_from": "2027-01-01T00:00:00Z",
                 *       "valid_to": null
                 *     }
                 */
                "application/json": components["schemas"]["CiRateInput"];
            };
        };
        responses: {
            /** @description Ставка сохранена. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "version_no": 2,
                     *       "runner_class": "linux-medium",
                     *       "unit": "minute",
                     *       "amount": "2.5000",
                     *       "currency": "RUB",
                     *       "valid_from": "2027-01-01T00:00:00Z",
                     *       "valid_to": null
                     *     }
                     */
                    "application/json": components["schemas"]["CiRate"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    listTeamTransfers: {
        parameters: {
            query: {
                /** @description Начало периода включительно, RFC 3339. */
                from: components["parameters"]["From"];
                /** @description Конец периода исключительно, RFC 3339 (BR-006). */
                to: components["parameters"]["To"];
                /** @description Размер страницы. */
                limit?: components["parameters"]["Limit"];
                /** @description Непрозрачный курсор из `next_cursor` предыдущей страницы. Содержимое курсора не разбирается клиентом; курсор действует 24 ч. */
                cursor?: components["parameters"]["Cursor"];
            };
            header?: never;
            path: {
                team_id: components["parameters"]["TeamId"];
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Страница эпизодов. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "data": [
                     *         {
                     *           "task": {
                     *             "kind": "task",
                     *             "id": "0193a5b0-f000-7000-8000-000000000050",
                     *             "display_key": "CORE-60",
                     *             "title": "Миграция платежей",
                     *             "source_url": "https://example.atlassian.net/browse/CORE-60"
                     *           },
                     *           "from_team_id": "0193a5b0-3333-7000-8000-000000000001",
                     *           "to_team_id": "0193a5b0-3333-7000-8000-000000000002",
                     *           "effective_at": "2026-10-11T00:00:00Z",
                     *           "wait_seconds": 172800
                     *         }
                     *       ],
                     *       "history_from": "2026-01-01T00:00:00Z",
                     *       "next_cursor": null,
                     *       "access_limited": false,
                     *       "access_notes": []
                     *     }
                     */
                    "application/json": components["schemas"]["TransferPage"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            404: components["responses"]["NotFound"];
        };
    };
    createCiInvoice: {
        parameters: {
            query?: never;
            header: {
                /** @description UUID одной попытки подтверждения. Повтор с тем же ключом возвращает сохранённый результат. */
                "Idempotency-Key": components["parameters"]["IdempotencyKey"];
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                /**
                 * @example {
                 *       "invoice_ref": "INV-2026-10",
                 *       "period_from": "2026-10-01T00:00:00Z",
                 *       "period_to": "2026-11-01T00:00:00Z",
                 *       "amount": "30.0000",
                 *       "currency": "RUB"
                 *     }
                 */
                "application/json": components["schemas"]["CiInvoiceInput"];
            };
        };
        responses: {
            /** @description Счёт сохранён. */
            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    /**
                     * @example {
                     *       "id": "0193a5b0-f8f8-7000-8000-000000000001",
                     *       "invoice_ref": "INV-2026-10",
                     *       "period_from": "2026-10-01T00:00:00Z",
                     *       "period_to": "2026-11-01T00:00:00Z",
                     *       "amount": "30.0000",
                     *       "currency": "RUB"
                     *     }
                     */
                    "application/json": components["schemas"]["CiInvoice"];
                };
            };
            400: components["responses"]["BadRequest"];
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            422: components["responses"]["ValidationFailed"];
        };
    };
    getBackgroundOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                operation_id: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {
            /** @description Текущее состояние именно запрошенного запуска. */
            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BackgroundOperation"];
                };
            };
            401: components["responses"]["Unauthorized"];
            403: components["responses"]["Forbidden"];
            404: components["responses"]["NotFound"];
        };
    };
}
