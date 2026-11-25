import { Provider } from '@gravity-ui/uikit'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { expect, it, vi } from 'vitest'
import { CheckReportState, LastGoodResult, ReportSha } from '..'

function show(node: ReactNode) {
  return render(
    <Provider theme="light" lang="ru">
      {node}
    </Provider>,
  )
}

it('shows a missing report as not checked, with a reason and without a success state', () => {
  show(<CheckReportState state="missing" reasonCode="report_missing" />)
  expect(screen.getByText('Не получен')).toBeVisible()
  expect(screen.getByText(/не проверено/iu)).toHaveTextContent(
    'Отчёт отсутствует',
  )
  expect(screen.queryByText(/успешно|оценено/iu)).not.toBeInTheDocument()
})

it('shows the rejection and timestamp of the report', () => {
  const date = '2026-10-01T09:00:00Z'
  show(<CheckReportState state="malformed" reportedAt={date} />)
  expect(screen.getByText('Отклонён')).toBeVisible()
  expect(screen.getByText(date)).toHaveAttribute('datetime', date)
})

it('shows the report SHA mismatch and copies the full SHA', async () => {
  const clipboard = { writeText: vi.fn().mockResolvedValue(undefined) }
  const original = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: clipboard,
  })
  try {
    const sha = 'a'.repeat(40)
    show(<ReportSha sha={sha} expectedSha={'b'.repeat(40)} />)
    expect(screen.getByText(sha.slice(0, 12))).toHaveAttribute('title', sha)
    expect(screen.getByText(/отчёт другого SHA/iu)).toBeVisible()
    fireEvent.click(
      screen.getByRole('button', { name: `Скопировать SHA ${sha}` }),
    )
    expect(clipboard.writeText).toHaveBeenCalledWith(sha)
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Скопировано')
    })
  } finally {
    if (original) Object.defineProperty(navigator, 'clipboard', original)
    else Reflect.deleteProperty(navigator, 'clipboard')
  }
})

it('identifies the last good result and its date', () => {
  const date = '2026-10-01T09:00:00Z'
  show(<LastGoodResult result={{ head_sha: 'a'.repeat(40), built_at: date }} />)
  expect(screen.getByText(/последний хороший результат от/iu)).toBeVisible()
  expect(screen.getByText(date)).toHaveAttribute('datetime', date)
})
