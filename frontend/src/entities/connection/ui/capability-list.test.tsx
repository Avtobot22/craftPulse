import { render, screen, within } from '@testing-library/react'
import { expect, it } from 'vitest'
import { CapabilityList } from './capability-list'

it('distinguishes verified, partial and missing with visible text and different icons', () => {
  const checked_at = '2026-11-08T10:01:05Z'
  render(
    <CapabilityList
      capabilities={[
        {
          capability: 'read_current',
          state: 'verified',
          reason_code: null,
          checked_at,
        },
        {
          capability: 'read_links',
          state: 'partial',
          reason_code: 'permission_missing',
          checked_at,
        },
        {
          capability: 'webhook',
          state: 'missing',
          reason_code: 'constructor',
          checked_at,
        },
      ]}
    />,
  )
  expect(screen.getByText('проверено', { exact: true })).toBeVisible()
  expect(screen.getByText('частично')).toBeVisible()
  expect(screen.getByText('недоступно')).toBeVisible()
  const icons = screen
    .getAllByRole('listitem')
    .map((row) => row.querySelector('svg[aria-hidden="true"]')?.innerHTML)
  expect(icons.every(Boolean)).toBe(true)
  expect(new Set(icons).size).toBe(3)
  const missing = screen.getByText('Приём вебхуков').closest('li')
  if (!missing) throw new Error('Missing capability row')
  expect(within(missing).getByText('Неизвестная причина')).toBeVisible()
  expect(within(missing).getByText('constructor').tagName).toBe('CODE')
  expect(within(missing).getByText(/UTC/u)).toHaveAttribute(
    'datetime',
    checked_at,
  )
})
