import type { components } from '@/shared/api'

type Schema = components['schemas']

export const adminLabels = {
  provider: {
    jira_cloud: 'Jira Cloud',
    gitlab: 'GitLab',
    github: 'GitHub',
    jenkins: 'Jenkins',
    sonarqube: 'SonarQube',
    confluence_cloud: 'Confluence Cloud',
    confluence_dc: 'Confluence DC',
    git: 'Git',
    universal: 'Универсальный импорт',
    client: 'Адаптер клиента',
  } satisfies Record<Schema['Provider'], string>,
  dataKind: {
    tasks: 'Задачи',
    merge_requests: 'Запросы на слияние',
    ci: 'Запуски CI',
    reports: 'Отчёты проверок',
    documents: 'Документы',
    accounts: 'Учётные записи',
    permissions: 'Права доступа',
  } satisfies Record<Schema['DataKind'], string>,
  capability: {
    read_current: 'Чтение текущего состояния',
    read_status_history: 'Чтение истории статусов',
    read_field_history: 'Чтение истории полей',
    read_resolution_history: 'Чтение истории исходов задач',
    read_sprint_history: 'Чтение истории спринтов',
    read_links: 'Чтение связей',
    read_permissions: 'Чтение прав доступа',
    read_deletions: 'Чтение сведений об удалении',
    write_mr_comment: 'Публикация комментариев в MR',
    write_tracker_issue: 'Создание задач в трекере',
    webhook: 'Приём вебхуков',
    read_branch_analysis: 'Чтение анализов веток',
    read_mr_analysis: 'Чтение анализов MR в SonarQube',
  } satisfies Record<Schema['Capability']['capability'], string>,
  capabilityState: {
    verified: 'проверено',
    partial: 'частично',
    missing: 'недоступно',
  } satisfies Record<Schema['Capability']['state'], string>,
  probeStatus: {
    pending: 'Ожидает проверки',
    running: 'Проверяется',
    succeeded: 'Проверка завершена',
    failed: 'Проверка не пройдена',
  } satisfies Record<Schema['ConnectionProbe']['status'], string>,
  probeReason: {
    edition_mismatch: 'Подключена неподдерживаемая редакция источника',
    auth_failed: 'Источник отклонил учётные данные',
    outside_network_scope: 'Адрес вне разрешённой сетевой области',
    certificate_untrusted: 'Сертификат источника не входит в список доверенных',
    timeout: 'Источник не ответил вовремя',
    adapter_unavailable: 'Адаптер недоступен',
    contract_unsupported: 'Версия контракта адаптера не поддерживается',
    configuration_changed: 'Настройки подключения изменились во время проверки',
  },
  capabilityReason: {
    edition_unsupported: 'Редакция SonarQube не поддерживает анализ MR',
    repository_binding_missing:
      'Проект SonarQube не связан с собранным репозиторием',
    not_checked: 'Адаптер не проверял эту возможность',
    permission_missing: 'Недостаточно прав в источнике',
    no_repository: 'Нет доступного репозитория для проверки',
    invalid_repository: 'Указан неверный идентификатор репозитория',
    no_project: 'Нет доступного проекта для проверки',
    invalid_project: 'Указан неверный идентификатор проекта',
    project_not_visible: 'Проект недоступен учётной записи сбора',
    browse_projects_missing: 'Нет разрешения на просмотр проектов',
    issues_unavailable: 'Задачи проекта недоступны',
    no_readable_project: 'Нет доступного проекта SonarQube',
    project_unavailable: 'Проект SonarQube недоступен учётной записи сбора',
    edition_does_not_support_branch_analysis:
      'Редакция SonarQube не поддерживает анализ веток',
    edition_does_not_support_mr_analysis:
      'Редакция SonarQube не поддерживает анализ MR',
    edition_capability_unverified:
      'Возможность редакции SonarQube не проверена',
    no_issue_to_verify: 'Нет задачи для проверки',
    issue_links_unavailable: 'Связи задач недоступны',
    changelog_unavailable: 'История изменений задачи недоступна',
    create_issues_missing: 'Нет разрешения на создание задач',
    jira_access_revocation_unverified:
      'Проверка отзыва доступа в Jira недоступна',
    project_permissions_unavailable: 'Не удалось прочитать права проекта',
    merge_requests_unavailable: 'Запросы на слияние недоступны',
    no_merge_request_to_verify: 'Нет запроса на слияние для проверки',
    merge_request_history_unavailable: 'История запроса на слияние недоступна',
    write_bot_token_unavailable: 'Нет доступного токена бота для публикации',
    write_permission_missing: 'Недостаточно прав для публикации',
    webhook_secret_missing_nightly_reconciliation:
      'Секрет подписи вебхука не задан. Выполняется ночная сверка',
  },
  syncPhase: {
    backfill: 'Загрузка истории',
    incremental: 'Сбор изменений',
    paused: 'Приостановлен',
    stopped: 'Остановлен',
  } satisfies Record<Schema['SyncDataKindStatus']['phase'], string>,
  syncOutcome: {
    success: 'Успешно',
    partial: 'Частично',
    error: 'Ошибка',
  } satisfies Record<
    NonNullable<Schema['SyncDataKindStatus']['last_outcome']>,
    string
  >,
  syncError: {
    unauthorized: 'Источник отклонил учётные данные',
    forbidden: 'Недостаточно прав для чтения',
    rate_limited: 'Источник ограничил частоту запросов',
    unavailable: 'Источник недоступен',
    timeout: 'Источник не ответил вовремя',
    adapter_unavailable: 'Адаптер недоступен',
    invalid_response: 'Источник вернул некорректный ответ',
  },
  syncCompleteness: {
    complete: 'Полные данные',
    partial: 'частично',
    unknown: 'неизвестно',
  } satisfies Record<Schema['SyncDataKindStatus']['completeness'], string>,
  receiptStatus: {
    quarantined: 'Обрабатывается',
    accepted: 'Принят',
    partial: 'Принят частично',
    rejected: 'Отклонён',
    duplicate: 'Дубликат',
  } satisfies Record<Schema['Receipt']['status'], string>,
  receiptReason: {
    schema_unsupported: 'Схема или её версия не поддерживается',
    envelope_invalid: 'Пакет не соответствует формату',
    path_unsafe: 'Пакет содержит недопустимый путь',
    limit_exceeded: 'Превышен лимит пакета',
    outside_scope: 'Пакет вне разрешённой области',
  },
  recordOutcome: {
    accepted: 'Принята',
    replay: 'Уже принята ранее',
    conflict: 'Конфликт с принятой записью',
    rejected: 'Отклонена',
    suppressed: 'Не восстановлена по правилам удаления',
  } satisfies Record<Schema['ReceiptRecord']['outcome'], string>,
  recordReason: {
    missing_source_id: 'Нет идентификатора записи в источнике',
    outside_scope: 'Запись вне разрешённой области',
    outside_channel: 'Запись не поддерживается для этого канала приёма',
    invalid_record: 'Некорректный формат записи',
    missing_event_time: 'Не указано время события',
    kind_not_allowed: 'Вид записи не допускается схемой',
    unsupported_payload_schema: 'Схема содержимого записи не поддерживается',
    class_not_allowed: 'Класс записи не допускается схемой',
    not_snapshot: 'Запись должна быть снимком',
    missing_read_id: 'Не указан идентификатор чтения',
    schema_violation: 'Содержимое записи не соответствует схеме',
    missing_file: 'Файл записи отсутствует в пакете',
    file_mismatch: 'Размер или контрольная сумма файла не совпадает',
  },
}

export function providerLabel(code: string): string {
  const providers: Readonly<Record<string, string>> = adminLabels.provider
  return Object.hasOwn(providers, code)
    ? (providers[code] ?? 'другой источник')
    : 'другой источник'
}
