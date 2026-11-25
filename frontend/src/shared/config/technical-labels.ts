import type { components } from '@/shared/api'

type Schema = components['schemas']
type Summary = Schema['MergeRequestSummary']
type Comparison = Schema['TechnicalComparison']

export const technicalLabels = {
  summaryItem: {
    new: 'Новая находка',
    resolved: 'Исправлена',
    newly_observed: 'Впервые замечена',
    unchanged: 'Без изменений',
    accepted: 'Принята',
    not_evaluated: 'Не проверено',
    ci_attempts: 'Попытки CI',
    coverage: 'Покрытие',
    tests: 'Тесты',
  } satisfies Record<Schema['SummaryItem']['item_kind'], string>,
  summaryStatus: {
    building: 'Формируется',
    ready: 'Готова',
    stale: 'Устарела',
    superseded: 'Заменена новой ревизией',
  } satisfies Record<Summary['status'], string>,
  pipelineState: {
    running: 'Выполняется',
    complete: 'Завершён',
    reports_missing: 'Отчёты отсутствуют',
  } satisfies Record<Summary['pipeline_state'], string>,
  publicationStatus: {
    not_requested: 'Публикация не запрошена',
    pending: 'Ожидает публикации',
    succeeded: 'Комментарий опубликован',
    failed: 'Комментарий не опубликован',
    unknown: 'Исход публикации неизвестен',
    stale: 'Публикация устарела',
  } satisfies Record<Summary['publication']['status'], string>,
  // These codes are documented in OpenAPI as an open string.
  publicationError: {
    write_permission_missing: 'У бота нет права записи в MR.',
    head_changed: 'В MR появился новый коммит.',
    provider_error: 'Ошибка Git-хостинга.',
  },
  incomparableReason: {
    profile_changed: 'Изменился профиль проверки',
    rules_changed: 'Изменились правила или база уязвимостей',
    scope_changed: 'Изменилась область проверки',
    base_scan_missing: 'Отсутствует отчёт для базового SHA',
    head_scan_missing: 'Отсутствует отчёт для текущего SHA',
    scan_incomplete: 'Получен неполный отчёт',
  } satisfies Record<Comparison['incomparable_reasons'][number], string>,
  comparisonFactor: {
    code_changed: 'Изменился код',
    dependency_changed: 'Изменились зависимости',
    rules_changed: 'Изменились правила проверки',
    db_changed: 'Изменилась база уязвимостей',
    scope_changed: 'Изменилась область проверки',
    source_recovered: 'Доступ к источнику восстановлен',
    source_lost: 'Доступ к источнику потерян',
    identity_uncertain: 'Соответствие находок не подтверждено',
  } satisfies Record<Comparison['factors'][number]['factor'], string>,
  findingKind: {
    code: 'Код',
    security: 'Безопасность',
    secret: 'Секрет',
    dependency: 'Зависимость',
    iac: 'Инфраструктура как код',
    docs: 'Документация',
    openapi: 'Контракт OpenAPI',
  } satisfies Record<Schema['FindingKind'], string>,
  findingState: {
    open: 'Открыта',
    resolved: 'Исправлена',
    accepted_risk: 'Принята (принят риск)',
    false_positive: 'Принята (ложное срабатывание)',
    not_applicable: 'Принята (не применимо)',
    newly_observed: 'Впервые замечена',
  } satisfies Record<Schema['FindingState'], string>,
  severity: {
    critical: 'Критическая',
    high: 'Высокая',
    medium: 'Средняя',
    low: 'Низкая',
    info: 'Информационная',
    unknown: 'Неизвестно',
  } satisfies Record<NonNullable<Schema['Severity']['mapped']>, string>,
  analysisKind: {
    code: 'Анализ кода',
    sast: 'Статический анализ безопасности',
    sca: 'Анализ зависимостей',
    secrets: 'Поиск секретов',
    iac: 'Проверка инфраструктуры как кода',
    image: 'Проверка образов',
    junit: 'Отчёт о тестах JUnit',
    coverage: 'Покрытие тестами',
    docs: 'Проверка документации',
    openapi: 'Проверка контракта OpenAPI',
    sonar: 'Анализ Sonar',
  } satisfies Record<Schema['AnalysisKind'], string>,
  exceptionState: {
    pending: 'Ожидает подтверждения',
    active: 'Действует',
    rejected: 'Отклонено',
    withdrawn: 'Отозвано',
    expired: 'Срок действия истёк',
  } satisfies Record<
    Schema['ExceptionSummary' | 'ExceptionDetail']['state'],
    string
  >,
  exceptionVersionStatus: {
    pending: 'Ожидает подтверждения',
    approved: 'Подтверждена',
    rejected: 'Отклонена',
    withdrawn: 'Отозвана',
  } satisfies Record<
    Schema['ExceptionVersion' | 'ExceptionTransition']['status'],
    string
  >,
  exceptionDecisionType: {
    accepted_risk: 'Принятый риск',
    false_positive: 'Ложное срабатывание',
    not_applicable: 'Не применимо',
    bypass: 'Обход проверки',
  } satisfies Record<
    Schema['ExceptionVersion' | 'ExceptionDraft']['decision_type'],
    string
  >,
  reviewerDecision: {
    approve: 'Подтвердить',
    reject: 'Отклонить',
    withdraw: 'Отозвать',
  } satisfies Record<Schema['ExceptionDecision']['decision'], string>,
  baselineState: {
    ready: 'Готова',
    pending: 'Ожидает отчёта',
    failed: 'Не удалось подготовить',
  } satisfies Record<Schema['BaselineList']['data'][number]['state'], string>,
  reportState: {
    received: 'Получен',
    missing: 'Не получен',
    partial: 'Частичный',
    malformed: 'Отклонён',
  } satisfies Record<Schema['Pipeline']['reports'][number]['state'], string>,
  reportReason: {
    report_missing: 'Отчёт отсутствует',
    scan_missing: 'Отчёт отсутствует',
    pipeline_unknown: 'Нет данных о запуске CI',
    expected_report_profile_unavailable: 'Профиль ожидаемых отчётов недоступен',
  },
}
