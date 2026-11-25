import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import type { components } from '@/shared/api'
import { FindingRow } from '..'

type FindingState = components['schemas']['FindingState']
const states = {
  open: 'Открыта',
  resolved: 'Исправлена',
  accepted_risk: 'Принята (принят риск)',
  false_positive: 'Принята (ложное срабатывание)',
  not_applicable: 'Принята (не применимо)',
  newly_observed: 'Впервые замечена',
} satisfies Record<FindingState, string>

it.each(Object.entries(states))(
  'shows %s as a distinct text state',
  (state, text) => {
    render(
      <FindingRow
        finding={{
          finding_kind: 'security',
          rule_id: 'example-rule',
          location: { path: 'src/example.go', line_start: 12, line_end: 15 },
          severity: { native: 'ERROR', mapped: 'high', mapping_version: 1 },
          confidence: null,
          state,
        }}
      />,
    )

    expect(screen.getByText(`Состояние: ${text}`)).toBeVisible()
    expect(screen.getByText('example-rule')).toBeVisible()
    expect(screen.getByText('src/example.go:12-15')).toBeVisible()
    expect(screen.getByText('Исходная серьёзность: ERROR')).toBeVisible()
    expect(
      screen.getByText('Сопоставленная серьёзность: Высокая'),
    ).toBeVisible()
    expect(screen.getByText('Уверенность: неизвестно')).toBeVisible()
  },
)
