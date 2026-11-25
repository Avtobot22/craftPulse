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

  it.each([
    [130, '0,036 ч'],
    [1, '0,00028 ч'],
    [0, '0 ч'],
  ] as const)('formats a duration of %s seconds', (value, label) => {
    render(<MetricValue value={{ ...base, unit: 'seconds', value }} />)
    expect(screen.getByText(label)).toBeInTheDocument()
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
    [
      'NOT_APPLICABLE',
      'as_of_only',
      'Показатель рассчитывается только на дату среза',
    ],
    [
      'UNKNOWN',
      'watermark_missing',
      'Нет отметки о свежести данных по одному или нескольким подключениям',
    ],
    [
      'NOT_EVALUATED',
      'reconcile_missing',
      'Сверка с источником ещё не проводилась',
    ],
    [
      'NOT_EVALUATED',
      'reconcile_unfinished',
      'Нет результата завершённой сверки с источником',
    ],
    ['UNKNOWN', 'type_unknown', 'Тип задачи неизвестен'],
    ['UNKNOWN', 'no_start', 'В истории задачи нет даты начала работы'],
    [
      'NOT_EVALUATED',
      'code_required_policy_missing',
      'Не настроено, для каких задач требуется изменение кода',
    ],
    [
      'UNKNOWN',
      'mr_unknown',
      'Неизвестно, слит ли связанный запрос на слияние',
    ],
    [
      'ASSESSED',
      'first_external_response',
      'Первый ответ определён по комментарию другого участника',
    ],
    [
      'UNKNOWN',
      'reviewer_field_unavailable',
      'Нет данных о назначении ревьюера',
    ],
    ['UNKNOWN', 'pipeline_unknown', 'Нет данных о запуске CI'],
    [
      'NOT_EVALUATED',
      'expected_report_profile_unavailable',
      'Нет настроек ожидаемых отчётов CI',
    ],
  ] as const)(
    'explains the report reason %s / %s',
    (state, reason_code, label) => {
      render(
        <MetricValue
          value={{
            ...base,
            state,
            value: state === 'ASSESSED' ? base.value : null,
            reason_code,
          }}
        />,
      )
      expect(screen.getByText(label)).toBeInTheDocument()
    },
  )

  it('shows an unrecognized API state as unknown and hides its number', () => {
    // A newer API can send strings outside the currently generated union.
    const state = 'FUTURE_STATE' as MetricValueProps['value']['state']
    render(<MetricValue value={{ ...base, state }} />)
    expect(screen.getByText('неизвестно')).toBeInTheDocument()
    expect(screen.getByText('Не рассчитано')).toBeInTheDocument()
    expect(screen.queryByText('25 шт.')).not.toBeInTheDocument()
  })

  it('explains conflicting event order for an unknown result', () => {
    render(
      <MetricValue
        value={{
          ...base,
          state: 'UNKNOWN',
          value: null,
          reason_code: 'event_order_conflict',
        }}
      />,
    )
    expect(screen.getByText('Не рассчитано')).toBeInTheDocument()
    expect(
      screen.getByText('Данные о порядке событий противоречат друг другу'),
    ).toBeInTheDocument()
    expect(screen.queryByText('event_order_conflict')).not.toBeInTheDocument()
  })

  it('shows coverage and explains why C05 is provisional', () => {
    render(
      <MetricValue
        value={{
          ...base,
          state: 'PROVISIONAL',
          value: 0.07,
          unit: 'ratio',
          numerator: 812,
          denominator: 11_600,
          n: null,
          reason_code: 'history_window_short',
        }}
      />,
    )
    expect(screen.getByText(/^7\s%$/)).toBeInTheDocument()
    expect(screen.getByText('частично')).toBeInTheDocument()
    expect(screen.getByText(/^Охват: 812 из 11\s600$/)).toBeInTheDocument()
    expect(
      screen.getByText('Доступная история короче периода расчёта'),
    ).toBeInTheDocument()
  })

  it('uses readable text for an unrecognized reason code', () => {
    render(
      <MetricValue
        value={{ ...base, value: null, reason_code: 'new_reason' }}
      />,
    )
    expect(screen.getByText('Описание причины недоступно')).toBeInTheDocument()
    expect(screen.queryByText('new_reason')).not.toBeInTheDocument()
  })

  it.each([
    [720, 'lower', 'Не менее 0,2 ч'],
    [750, 'lower', 'Не менее 0,2 ч'],
    [750, 'upper', 'Не более 0,21 ч'],
    [91_861, 'lower', 'Не менее 1 д 1 ч'],
    [91_861, 'upper', 'Не более 1 д 2 ч'],
  ] as const)(
    'preserves the direction of a duration bound of %s seconds (%s)',
    (value, kind, label) => {
      render(
        <MetricValue
          value={{
            ...base,
            state: 'BOUNDED',
            unit: 'seconds',
            value: null,
            bound: { kind, value },
          }}
        />,
      )
      expect(screen.getByText(label)).toBeInTheDocument()
    },
  )

  it.each([0, null])(
    'omits percentage and coverage when denominator is %s',
    (denominator) => {
      render(
        <MetricValue
          value={{
            ...base,
            state: 'PROVISIONAL',
            unit: 'ratio',
            value: 0.25,
            numerator: 25,
            denominator,
          }}
        />,
      )
      expect(screen.getByText('Не рассчитано')).toBeInTheDocument()
      expect(
        screen.getByText('Знаменатель отсутствует или равен нулю'),
      ).toBeInTheDocument()
      expect(screen.queryByText(/Охват:/)).not.toBeInTheDocument()
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
