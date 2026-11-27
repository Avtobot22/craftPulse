import type { CohortRole } from '@/shared/api'

export const roleLabels: Record<CohortRole, string> = {
  numerator: 'Числитель',
  denominator: 'Знаменатель',
  excluded: 'Исключено',
  missing: 'Пропущено',
}
