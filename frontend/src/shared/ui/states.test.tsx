import {
  Provider,
  Button,
  Toaster,
  ToasterComponent,
  ToasterProvider,
} from '@gravity-ui/uikit'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import {
  DataStateBadge,
  EmptyState,
  ErrorState,
  LoadingState,
  useNotifications,
  StaleState,
  SuccessState,
  TechnicalValue,
} from './index'

function show(node: ReactNode) {
  return render(
    <Provider theme="light" lang="ru">
      {node}
    </Provider>,
  )
}
afterEach(() => {
  vi.useRealTimers()
})

describe('общие состояния', () => {
  it('объявляет загрузку без процента с заданной геометрией', () => {
    show(
      <LoadingState
        minHeight={320}
        blocks={[{ width: '100%', height: 240 }]}
      />,
    )
    expect(screen.getByRole('status')).toHaveTextContent('Загрузка')
    expect(screen.getByRole('region')).toHaveStyle({ minHeight: '320px' })
    expect(screen.queryByRole('progressbar')).not.toBeInTheDocument()
  })
  it.each([
    ['filters', 'Нет объектов по фильтрам'],
    ['source', 'Источник не настроен'],
    ['history', 'История ещё не получена'],
    ['inapplicable', 'Результат неприменим'],
  ] as const)('различает причину пустоты %s', (reason, title) => {
    const action = vi.fn()
    show(
      <EmptyState
        reason={reason}
        action={{ label: 'Сбросить фильтры', onClick: action }}
      />,
    )
    expect(screen.getByRole('status', { name: title })).toHaveTextContent(title)
    expect(screen.getByRole('status').querySelector('svg')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Сбросить фильтры' }))
    expect(action).toHaveBeenCalledOnce()
  })
  it.each([
    ['PROVISIONAL', 'частично'],
    ['BOUNDED', 'известна граница'],
    ['NOT_EVALUATED', 'не проверено'],
    ['UNKNOWN', 'неизвестно'],
    ['NOT_APPLICABLE', 'неприменимо'],
    ['ASSESSED', 'оценено'],
  ] as const)('подписывает %s без зависимости от цвета', (state, label) => {
    const { container } = show(
      <DataStateBadge state={state} reason="Причина" />,
    )
    expect(screen.getByText(label)).toBeInTheDocument()
    expect(screen.getByText('Причина')).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeInTheDocument()
  })
  it('показывает безопасные поля ошибки и блокирует повтор в работе', () => {
    const retry = vi.fn()
    const problem = {
      reason: 'Источник временно недоступен',
      correlationId: 'safe-id',
      detail: 'password=private',
      credentials: 'secret',
      onRetry: retry,
    }
    const { rerender } = show(<ErrorState {...problem} />)
    expect(screen.getByRole('alert', { name: 'Ошибка' })).toHaveTextContent(
      'safe-id',
    )
    expect(
      screen.queryByText(/password=private|secret/),
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Повторить' }))
    expect(retry).toHaveBeenCalledOnce()
    rerender(
      <Provider>
        <ErrorState {...problem} retrying />
      </Provider>,
    )
    expect(
      screen.getByRole('button', { name: 'Повтор выполняется' }),
    ).toBeDisabled()
  })
  it('отделяет дату снимка от текущей ошибки', () => {
    show(
      <StaleState
        snapshotAt="2026-11-21T10:00:00Z"
        currentError={<ErrorState reason="Сбой чтения" onRetry={vi.fn()} />}
      >
        <div>Разрешённый снимок</div>
      </StaleState>,
    )
    expect(
      screen.getByRole('status', { name: 'Устаревшие данные' }),
    ).toHaveTextContent('2026-11-21T10:00:00Z')
    expect(screen.getByRole('status')).not.toHaveTextContent('Сбой чтения')
    expect(screen.getByRole('alert')).toHaveTextContent('Сбой чтения')
  })
  it('объявляет успех с версией', () => {
    show(<SuccessState message="Сохранено" version="17" />)
    expect(screen.getByRole('status', { name: 'Успех' })).toHaveTextContent(
      'Версия: 17',
    )
  })
})

describe('копирование', () => {
  it('копирует полный SHA и объявляет результат', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const value = 'abcdef'.repeat(50)
    show(<TechnicalValue label="SHA" value={value} />)
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать: SHA' }))
    expect(writeText).toHaveBeenCalledWith(value)
    await act(async () => {
      await Promise.resolve()
    })
    expect(screen.getByRole('status')).toHaveTextContent('Скопировано')
  })
  it('объявляет отказ clipboard', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    show(<TechnicalValue label="SHA" value="abc" />)
    fireEvent.click(screen.getByRole('button', { name: 'Скопировать: SHA' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Не удалось скопировать',
    )
  })
  it('использует fallback UIKit и возвращает фокус на кнопку', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    const fallback = vi.fn().mockReturnValue(true)
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: fallback,
    })
    try {
      show(<TechnicalValue label="SHA" value="full-sha" />)
      const button = screen.getByRole('button', { name: 'Скопировать: SHA' })
      button.focus()
      fireEvent.click(button)
      await act(async () => {
        await Promise.resolve()
      })
      expect(screen.getByRole('status')).toHaveTextContent('Скопировано')
      expect(fallback).toHaveBeenCalledWith('copy')
      expect(button).toHaveFocus()
    } finally {
      Reflect.deleteProperty(document, 'execCommand')
    }
  })
  it('не предлагает копировать скрытое значение', () => {
    show(<TechnicalValue label="Ключ" hidden />)
    expect(screen.getByText('Значение скрыто')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})

function NotificationControls() {
  const notifications = useNotifications()
  return (
    <>
      <Button
        onClick={() => {
          notifications.success({ name: 'saved', message: 'Сохранено' })
        }}
      >
        Показать успех
      </Button>
      <Button
        onClick={() => {
          notifications.success({
            name: 'empty-actions',
            message: 'Сохранено без действий',
            actions: [],
          })
        }}
      >
        Показать успех без действий
      </Button>
      <Button
        onClick={() => {
          notifications.error({
            name: 'error',
            message: 'Не удалось сохранить',
          })
        }}
      >
        Показать ошибку
      </Button>
      <Button
        onClick={() => {
          notifications.success({
            name: 'action',
            message: 'Результат готов',
            actions: [{ label: 'Открыть', onClick: () => undefined }],
          })
        }}
      >
        Показать действие
      </Button>
    </>
  )
}
function showNotifications() {
  const toaster = new Toaster()
  show(
    <ToasterProvider toaster={toaster}>
      <NotificationControls />
      <ToasterComponent />
    </ToasterProvider>,
  )
  return toaster
}

describe('штатные уведомления UIKit', () => {
  it('закрывает пассивный успех через 8 секунд', () => {
    vi.useFakeTimers()
    const toaster = showNotifications()
    fireEvent.click(screen.getByRole('button', { name: 'Показать успех' }))
    expect(screen.getByText('Сохранено')).toBeInTheDocument()
    act(() => {
      vi.advanceTimersByTime(7999)
    })
    expect(toaster.has('saved')).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(toaster.has('saved')).toBe(false)
  })
  it('пустой массив действий не отключает таймер успеха', () => {
    vi.useFakeTimers()
    const toaster = showNotifications()
    fireEvent.click(
      screen.getByRole('button', { name: 'Показать успех без действий' }),
    )
    act(() => {
      vi.advanceTimersByTime(8000)
    })
    expect(toaster.has('empty-actions')).toBe(false)
  })
  it('сохраняет успех при наведении и использует штатный таймер после ухода', () => {
    vi.useFakeTimers()
    const toaster = showNotifications()
    fireEvent.click(screen.getByRole('button', { name: 'Показать успех' }))
    act(() => {
      vi.advanceTimersByTime(3000)
    })
    fireEvent.mouseOver(screen.getByText('Сохранено'))
    act(() => {
      vi.advanceTimersByTime(10000)
    })
    expect(toaster.has('saved')).toBe(true)
    fireEvent.mouseLeave(
      screen.getByText('Сохранено').closest('[data-toast]') ??
        screen.getByText('Сохранено'),
    )
    act(() => {
      vi.advanceTimersByTime(7999)
    })
    expect(toaster.has('saved')).toBe(true)
    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(toaster.has('saved')).toBe(false)
  })
  it.each([
    ['Показать ошибку', 'error', 'Не удалось сохранить'],
    ['Показать действие', 'action', 'Результат готов'],
  ])('сохраняет уведомление %s до закрытия', (trigger, name, message) => {
    vi.useFakeTimers()
    const toaster = showNotifications()
    fireEvent.click(screen.getByRole('button', { name: trigger }))
    expect(screen.getByText(message)).toBeInTheDocument()
    act(() => {
      vi.advanceTimersByTime(30000)
    })
    expect(toaster.has(name)).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: /Закрыть/ }))
    expect(toaster.has(name)).toBe(false)
  })
})
