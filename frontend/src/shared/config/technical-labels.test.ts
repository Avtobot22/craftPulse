import { describe, expect, it } from 'vitest'
import { contractLabel, technicalLabels } from './index'

describe('technical contract labels', () => {
  it.each(
    Object.entries(technicalLabels).flatMap(([group, labels]) =>
      Object.keys(labels).map((code) => ({ group, labels, code })),
    ),
  )('provides Russian text for $group.$code', ({ labels, code }) => {
    const text = contractLabel(labels, code)
    expect(text).toMatch(/[а-яё]/iu)
    expect(text).not.toContain('Неизвестное значение')
  })

  it.each(['future_status', 'constructor', '__proto__'])(
    'preserves unknown code %s',
    (code) => {
      expect(contractLabel(technicalLabels.findingState, code)).toBe(
        `Неизвестное значение (${code})`,
      )
    },
  )
})
