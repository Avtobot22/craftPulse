import type { components } from '@/shared/api'

type Schema = components['schemas']

const ciOutcome = {
  running: 'Выполняется',
  success: 'Успешно',
  failed: 'Сбой',
  canceled: 'Отменено',
  skipped: 'Пропущено',
  unknown: 'Исход неизвестен',
} satisfies Record<Schema['Pipeline' | 'Attempt']['outcome'], string>

const reviewWait = {
  no_reviewer: 'Нет ревьюера',
  awaiting_first_response: 'Ждёт первого ответа',
  awaiting_re_review: 'Ждёт повторного ревью',
  responded: 'Ответ получен',
  unknown: 'Состояние неизвестно: история MR неполная',
} satisfies Record<NonNullable<Schema['ReviewWait']>['kind'], string>

export const contractLabels = {
  taskStatus: {
    intake: 'Поступила',
    active: 'В работе',
    wait: 'Ожидает',
    blocked: 'Заблокирована',
    done: 'Завершена',
    cancelled: 'Отменена',
    duplicate: 'Дубликат',
    unknown: 'Смысл статуса неизвестен',
  } satisfies Record<Schema['TaskStatus']['meaning'], string>,
  eventKind: {
    created: 'Создание задачи',
    field_change: 'Изменение поля',
    comment: 'Комментарий',
    link_change: 'Изменение связи',
    moved: 'Перенос задачи',
    ready: 'Готов к ревью',
    reviewer_assigned: 'Ревьюер назначен',
    reviewer_removed: 'Ревьюер снят',
    review: 'Ревью',
    approval: 'Одобрение',
    head_pushed: 'Обновление head',
    merged: 'Слияние',
    closed: 'Закрытие',
    reopened: 'Повторное открытие',
    pipeline_started: 'Запуск pipeline',
    pipeline_finished: 'Завершение pipeline',
    attempt_started: 'Запуск попытки',
    attempt_finished: 'Завершение попытки',
    version_created: 'Создание версии документа',
  },
  linkMethod: {
    source_explicit: 'Явная связь в источнике',
    user_confirmed: 'Подтверждение пользователем',
    branch_key: 'Ключ в имени ветки',
    title_key: 'Ключ в заголовке',
    time_window: 'Совпадение по времени',
    path_match: 'Совпадение по пути',
  } satisfies Record<Schema['Link']['method'], string>,
  linkStatus: {
    candidate: 'Кандидат',
    confirmed: 'Подтверждена',
    rejected: 'Отклонена',
  } satisfies Record<Schema['Link']['status'], string>,
  ciOutcome,
  failureKind: {
    script_failure: 'Ошибка сценария задания',
    runner_system_failure: 'Сбой раннера',
    stuck_or_timeout_failure: 'Задание зависло или превысило лимит времени',
    job_execution_timeout: 'Превышен лимит времени задания',
    scheduler_failure: 'Сбой планировщика',
    api_failure: 'Сбой API',
    data_integrity_failure: 'Нарушена целостность данных',
    unknown_failure: 'Причина сбоя неизвестна',
  },
  reviewWait,
}

export function contractLabel(
  labels: Readonly<Record<string, string>>,
  code: string,
) {
  return (
    (Object.hasOwn(labels, code) ? labels[code] : undefined) ??
    `Неизвестное значение (${code})`
  )
}
