import type { ReportSeries } from '@/shared/api'
import { seriesGroupLabel } from './metrics'

export const technicalMetrics = {
  X03: {
    title: 'Задачи с актуальными техническими предупреждениями',
    first: 'warning',
  },
  X04: { title: 'Успешные pipeline с оговорками', first: 'caveat' },
  X05: {
    title: 'Полнота цепочки от задачи до проверок',
    first: 'confirmed_mr',
  },
  X06: {
    title: 'Подтверждённый результат исправления в MR',
    first: 'code:resolved',
  },
}

export const chainStages = [
  'confirmed_mr',
  'current_head_ci',
  'complete_reports',
]

const signals: Readonly<Record<string, string>> = {
  warning: 'Технические предупреждения',
  caveat: 'Оговорки результата',
  new_code: 'Новые находки в коде',
  new_security: 'Новые находки безопасности',
  docs_violation: 'Нарушения документации',
  missing_report: 'Пропущен ожидаемый отчёт',
  fail_to_pass: 'Fail → pass',
  ci_passed_after_retry: 'Fail → pass',
  allowed_failure: 'Разрешённое падение',
  code_exception: 'Действующие исключения для кода',
  security_exception: 'Действующие исключения для безопасности',
  confirmed_mr: 'Подтверждённый MR',
  current_head_ci: 'CI на текущем head',
  complete_reports: 'Полные отчёты профиля',
  checked_code: 'Код проверен',
  checked_security: 'Безопасность проверена',
  checked_tests: 'Результаты тестов получены',
  checked_coverage: 'Покрытие получено',
  checked_docs: 'Документация проверена',
}

const analysisKinds: Readonly<Record<string, string>> = {
  code: 'Код',
  sast: 'SAST',
  sca: 'SCA',
  secrets: 'Секреты',
  iac: 'IaC',
  image: 'Образы',
  docs: 'Документация',
}

const dispositions: Readonly<Record<string, string>> = {
  new: 'Новые находки',
  persisting: 'Сохранившиеся находки',
  resolved: 'Подтверждённо устранённые находки',
  accepted: 'Принятые исключения',
  unverified: 'Непроверенные области',
}

export function technicalSignalLabel(signal: string) {
  if (Object.hasOwn(signals, signal)) return signals[signal]
  const [kind = '', disposition = ''] = signal.split(':', 2)
  const kindLabel = Object.hasOwn(analysisKinds, kind)
    ? analysisKinds[kind]
    : undefined
  const dispositionLabel = Object.hasOwn(dispositions, disposition)
    ? dispositions[disposition]
    : undefined
  return kindLabel && dispositionLabel
    ? `${kindLabel} · ${dispositionLabel}`
    : undefined
}

export function technicalSeriesLabel(series: ReportSeries, groupBy: string) {
  const signal = series.group?.signal ?? series.estimator
  const label = technicalSignalLabel(signal ?? '') ?? 'Результат проверки'
  return [label, seriesGroupLabel(series.group, groupBy)]
    .filter(Boolean)
    .join(' · ')
}

export function technicalGapLabel(reason: string | null) {
  switch (reason) {
    case 'no_merge_request':
      return 'Нет подтверждённого MR'
    case 'current_head_ci_missing':
      return 'Нет CI на текущем head'
    case 'scope_complete_reports_missing':
      return 'Профиль отчётов заполнен не полностью'
    case 'task_state_unknown':
      return 'Состояние задачи неизвестно'
    case null:
    default:
      return 'Недостаточно данных для подтверждения'
  }
}
