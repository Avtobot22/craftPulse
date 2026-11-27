import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ReceiptStatusBadge } from './receipt-status-badge'

it.each([
  ['quarantined', 'Пакет проверяется'],
  ['accepted', 'Принят'],
  ['partial', 'Принят частично'],
  ['rejected', 'Отклонён'],
  ['duplicate', 'Повтор доставки'],
  ['future', 'Неизвестное значение (future)'],
])('labels receipt status %s with text and an icon', (status, text) => {
  const { container } = render(<ReceiptStatusBadge status={status} />)
  expect(screen.getByText(text)).toBeVisible()
  expect(container.querySelector('svg[aria-hidden="true"]')).toBeInTheDocument()
})
