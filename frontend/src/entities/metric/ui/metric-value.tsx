import { Flex, Text } from '@gravity-ui/uikit'
import type { components } from '@/shared/api'
import { DataStateBadge } from '@/shared/ui'

type Value = components['schemas']['MetricValue']

export interface MetricValueProps {
  value: Value
}

const number = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 2 })

const reasons: Readonly<Record<string, string>> = {
  history_incomplete: 'История получена не полностью',
  mapping_missing: 'Соответствие статусов не настроено',
  collection_incomplete: 'Данные собраны не полностью',
  scan_missing: 'Отчёт проверки отсутствует',
  target_missing: 'Целевой срок не задан',
}

function formatValue(
  value: number,
  unit: string,
  boundKind?: 'lower' | 'upper',
) {
  if (unit === 'seconds') {
    const rawHours = value / 3600
    const rounded =
      boundKind === 'lower'
        ? Math.floor(rawHours)
        : boundKind === 'upper'
          ? Math.ceil(rawHours)
          : Math.sign(rawHours) * Math.round(Math.abs(rawHours))
    const hours = Math.abs(rounded)
    const days = Math.floor(hours / 24)
    const sign = rounded < 0 ? '−' : ''
    return days
      ? `${sign}${number.format(days)} д ${number.format(hours % 24)} ч`
      : `${sign}${number.format(hours)} ч`
  }
  const display = new Intl.NumberFormat('ru-RU', {
    style: unit === 'ratio' ? 'percent' : 'decimal',
    maximumFractionDigits: 2,
    roundingMode:
      boundKind === 'lower'
        ? 'floor'
        : boundKind === 'upper'
          ? 'ceil'
          : 'halfExpand',
  }).format(value)
  const label = unit === 'count' ? 'шт.' : unit === 'percent' ? '%' : unit
  return unit !== 'ratio' && label ? `${display} ${label}` : display
}

export function MetricValue({ value }: MetricValueProps) {
  const { state, unit, denominator, n, bound, low_sample, reason_code } = value
  const showCoverage = state === 'PROVISIONAL' || state === 'BOUNDED'
  const missingDenominator =
    (unit === 'ratio' || unit === 'percent' || unit === '%') &&
    (denominator === null || denominator <= 0)
  let display: string | undefined
  if (!missingDenominator) {
    if (state === 'BOUNDED' && bound) {
      const direction = bound.kind === 'lower' ? 'Не менее' : 'Не более'
      display = `${direction} ${formatValue(bound.value, unit, bound.kind)}`
    }
    if (
      (state === 'ASSESSED' || state === 'PROVISIONAL') &&
      value.value !== null
    ) {
      display = formatValue(value.value, unit)
    }
  }

  const reason = reason_code
    ? Object.hasOwn(reasons, reason_code)
      ? reasons[reason_code]
      : reason_code
    : undefined
  return (
    <Flex direction="column" alignItems="flex-start" gap="spacing-2">
      <Text variant="header-1">{display ?? 'Не рассчитано'}</Text>
      <DataStateBadge state={state} {...(reason ? { reason } : {})} />
      {missingDenominator && (
        <Text>Знаменатель отсутствует или равен нулю</Text>
      )}
      {!display && !reason && !missingDenominator && (
        <Text>Причина отсутствия значения не указана</Text>
      )}
      {n !== null && <Text>n = {number.format(n)}</Text>}
      {showCoverage && (
        <Text>
          Пропущено: {number.format(value.missing_count)}. Исключено:{' '}
          {number.format(value.excluded_count)}.
        </Text>
      )}
      {low_sample && <Text>Малая выборка</Text>}
    </Flex>
  )
}
