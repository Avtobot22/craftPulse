import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { ReviewWaitDetails } from './review-wait'

const wait = {
  since: '2026-10-02T06:00:00Z',
  wait_seconds: 180000,
  fallback_open: true,
}

it('shows the review duration from opening when readiness is unknown', () => {
  render(
    <ReviewWaitDetails wait={{ ...wait, kind: 'awaiting_first_response' }} />,
  )
  expect(
    screen.getByText('Ждёт первого ответа · 2 д 2 ч (от открытия)'),
  ).toBeVisible()
})

it('shows incomplete MR history as an unknown review state', () => {
  render(
    <ReviewWaitDetails
      wait={{ ...wait, kind: 'unknown', fallback_open: false }}
    />,
  )
  expect(
    screen.getByText('Состояние неизвестно: история MR неполная · 2 д 2 ч'),
  ).toBeVisible()
  expect(screen.queryByText(/нет ревьюера/iu)).not.toBeInTheDocument()
})
