import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { syncStatusExample } from '../../../../test/admin-fixtures'
import { SyncDataKind } from './sync-data-kind'

const data = syncStatusExample.data_kinds[0]
if (!data) throw new Error('Expected one data kind in the OpenAPI example')

it('keeps the timeout, last success and partial coverage in separate fields', () => {
  render(<SyncDataKind data={data} />)
  expect(
    screen.getByText('Ошибка', { selector: 'dt' }).nextElementSibling,
  ).toHaveTextContent('Источник не ответил вовремя')
  expect(screen.getByText('timeout').tagName).toBe('CODE')
  expect(
    screen
      .getByText('Последний успешный сбор')
      .nextElementSibling?.querySelector('time'),
  ).toHaveAttribute('datetime', '2026-11-07T03:12:00Z')
  expect(screen.getByText('Полнота').nextElementSibling).toHaveTextContent(
    'частично',
  )
  expect(screen.getByText('Отставание').nextElementSibling).toHaveTextContent(
    '1 дн. 7 ч',
  )
  expect(screen.getByText('customfield_10031')).toBeVisible()
  expect(screen.getByText('T08, T12')).toBeVisible()
})

it('shows unknown coverage separately from the absence of a successful collection', () => {
  render(
    <SyncDataKind
      data={{ ...data, completeness: 'unknown', last_success_at: null }}
    />,
  )
  expect(screen.getByText('Полнота').nextElementSibling).toHaveTextContent(
    'неизвестно',
  )
  expect(
    screen.getByText('Последний успешный сбор').nextElementSibling,
  ).toHaveTextContent('Нет данных')
})
