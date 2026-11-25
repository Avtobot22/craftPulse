import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MetricValue } from './metric-value'
import type { MetricValueProps } from './metric-value'

const base: MetricValueProps['value'] = {
  state: 'ASSESSED',
  value: 25,
  unit: 'count',
  numerator: null,
  denominator: null,
  n: 25,
  bound: null,
  low_sample: false,
  excluded_count: 0,
  missing_count: 0,
  reason_code: null,
}

describe('MetricValue', () => {
  it.each([
    ['ASSESSED', 'оценено', '25 шт.'],
    ['PROVISIONAL', 'частично', '25 шт.'],
    ['BOUNDED', 'известна граница', 'Не менее 10 шт.'],
    ['UNKNOWN', 'неизвестно', 'Не рассчитано'],
    ['NOT_EVALUATED', 'не проверено', 'Не рассчитано'],
    ['NOT_APPLICABLE', 'неприменимо', 'Не рассчитано'],
  ] as const)('renders %s with a text status', (state, badge, display) => {
    render(
      <MetricValue
        value={{
          ...base,
          state,
          bound: state === 'BOUNDED' ? { kind: 'lower', value: 10 } : null,
        }}
      />,
    )
    expect(screen.getByText(badge)).toBeInTheDocument()
    expect(screen.getByText(display)).toBeInTheDocument()
    if (display !== '25 шт.') expect(screen.queryByText('25 шт.')).toBeNull()
  })

  it('shows n and the low sample note separately', () => {
    render(<MetricValue value={{ ...base, n: 4, low_sample: true }} />)
    expect(screen.getByText('n = 4')).toBeInTheDocument()
    expect(screen.getByText('Малая выборка')).toBeInTheDocument()
  })

  it.each(['PROVISIONAL', 'BOUNDED'] as const)(
    'shows coverage counts for %s',
    (state) => {
      render(
        <MetricValue
          value={{
            ...base,
            state,
            value: state === 'PROVISIONAL' ? 259_200 : null,
            unit: 'seconds',
            n: 6,
            bound:
              state === 'BOUNDED' ? { kind: 'lower', value: 259_200 } : null,
            low_sample: true,
            missing_count: 1,
            excluded_count: 2,
            reason_code: 'history_incomplete',
          }}
        />,
      )
      expect(screen.getByText('n = 6')).toBeInTheDocument()
      expect(
        screen.getByText('Пропущено: 1. Исключено: 2.'),
      ).toBeInTheDocument()
    },
  )

  it('formats a duration as days and hours', () => {
    render(<MetricValue value={{ ...base, unit: 'seconds', value: 90_061 }} />)
    expect(screen.getByText('1 д 1 ч')).toBeInTheDocument()
  })

  it('shows the reason for a missing value', () => {
    render(
      <MetricValue
        value={{ ...base, value: null, reason_code: 'history_incomplete' }}
      />,
    )
    expect(screen.getByText('Не рассчитано')).toBeInTheDocument()
    expect(
      screen.getByText('История получена не полностью'),
    ).toBeInTheDocument()
  })

  it.each([
    ['lower', 'Не менее 1 д 1 ч'],
    ['upper', 'Не более 1 д 2 ч'],
  ] as const)(
    'preserves the direction of a rounded duration bound (%s)',
    (kind, label) => {
      render(
        <MetricValue
          value={{
            ...base,
            state: 'BOUNDED',
            unit: 'seconds',
            value: null,
            bound: { kind, value: 91_861 },
          }}
        />,
      )
      expect(screen.getByText(label)).toBeInTheDocument()
    },
  )

  it.each([0, null])(
    'omits a percentage when denominator is %s',
    (denominator) => {
      render(
        <MetricValue
          value={{ ...base, unit: 'ratio', value: 0.25, denominator }}
        />,
      )
      expect(screen.getByText('Не рассчитано')).toBeInTheDocument()
      expect(
        screen.getByText('Знаменатель отсутствует или равен нулю'),
      ).toBeInTheDocument()
    },
  )

  it.each([
    ['lower', /^Не менее 12,34\s%$/],
    ['upper', /^Не более 12,35\s%$/],
  ] as const)(
    'preserves the direction of a percentage bound (%s)',
    (kind, label) => {
      render(
        <MetricValue
          value={{
            ...base,
            state: 'BOUNDED',
            unit: 'ratio',
            denominator: 100,
            bound: { kind, value: 0.123456 },
          }}
        />,
      )
      expect(screen.getByText(label)).toBeInTheDocument()
    },
  )
})
