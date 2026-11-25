import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { useEffect, useRef, useState } from 'react'
import { createMemoryRouter, Link, useNavigate } from 'react-router'
import { RouterProvider } from 'react-router/dom'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import {
  ApiClient,
  InvalidResponse,
  isStrongETag,
  readRecord,
  readString,
} from '@/shared/api'
import type { ApiResponse } from '@/shared/api'
import { routePaths } from '@/shared/config'
import {
  jsonResponse,
  problem,
  problemResponse,
  structureInput,
} from '../../../../test/fixtures'
import { ChangeConfirmationDialog, UnsavedChangesDialog } from '../index'
import type { ChangeAttempt, ChangeProposal, ChangeValidation } from '../index'

// jsdom has no modal implementation. Browser-generated Escape is sent as a cancel event.
beforeAll(() => {
  const prototype = HTMLDialogElement.prototype
  if (!Object.hasOwn(prototype, 'showModal')) {
    Object.defineProperty(prototype, 'showModal', {
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
      configurable: true,
    })
  }
  if (!Object.hasOwn(prototype, 'close')) {
    Object.defineProperty(prototype, 'close', {
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
      configurable: true,
    })
  }
})

const path = '/organization/structure'
const field = '/teams/0/name'

interface Settings {
  version: number
  name: string
  project: string
}

function readSettings(value: unknown): Settings {
  const data = readRecord(value)
  if (typeof data.version !== 'number') throw new InvalidResponse()
  return {
    version: data.version,
    name: readString(data.name),
    project: readString(data.project),
  }
}

function snapshot(version: number, name = 'Платформа', project = 'Core') {
  return jsonResponse({ version, name, project }, 200, {
    ETag: `"${String(version)}"`,
  })
}

function draftBody(name: string, project: string) {
  return {
    ...structureInput,
    teams: structureInput.teams.map((team) => ({ ...team, name })),
    projects: structureInput.projects.map((item) => ({
      ...item,
      name: project,
    })),
  }
}

function prepare(
  resource: ApiResponse<Settings>,
  name: string,
): ChangeProposal<ReturnType<typeof draftBody>> {
  return {
    etag: resource.etag ?? '',
    changes: [
      { label: 'Название команды', before: resource.data.name, after: name },
    ],
    draft: draftBody(name, resource.data.project),
  }
}

function TestForm({
  api,
  initial,
}: {
  api: ApiClient
  initial: ApiResponse<Settings>
}) {
  const [saved, setSaved] = useState(initial)
  const [name, setName] = useState(initial.data.name)
  const [proposal, setProposal] = useState<ReturnType<typeof prepare> | null>(
    null,
  )
  const [validation, setValidation] = useState<ChangeValidation | null>(null)
  const form = useRef<HTMLFormElement>(null)
  const summary = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  useEffect(() => {
    return api.onAuthRequired(() => {
      void navigate(routePaths.sessionExpired, { replace: true })
    })
  }, [api, navigate])

  useEffect(() => {
    if (validation) summary.current?.focus()
  }, [validation])

  const errors = validation?.fields.get(field)
  return (
    <main>
      <h1>Настройки</h1>
      <p>Действующая версия: {saved.data.version}</p>
      {validation && (
        <div role="alert" tabIndex={-1} ref={summary}>
          Исправьте ошибки
          {errors && <a href="#team-name">Название команды</a>}
        </div>
      )}
      <form
        ref={form}
        onSubmit={(event) => {
          event.preventDefault()
          setValidation(null)
          setProposal(prepare(saved, name))
        }}
      >
        <label htmlFor="team-name">Название команды</label>
        <input
          id="team-name"
          value={name}
          aria-invalid={errors ? true : undefined}
          aria-describedby={errors ? 'team-error' : undefined}
          onChange={(event) => {
            setName(event.currentTarget.value)
          }}
        />
        {errors && <p id="team-error">Введите допустимое название</p>}
        <button type="submit" disabled={name === saved.data.name}>
          Сохранить
        </button>
      </form>
      <Link to="/overview">Перейти к обзору</Link>
      {proposal && (
        <ChangeConfirmationDialog
          title="Подтверждение настроек"
          scope="Организация"
          consequences="Название команды изменится."
          proposal={proposal}
          dangerous
          submit={(attempt, signal) =>
            api.update('PUT', path, attempt.draft, readSettings, {
              signal,
              ifMatch: attempt.etag,
              idempotencyKey: attempt.idempotencyKey,
            })
          }
          recheck={async (attempt, reason, signal) => {
            const latest = await api.read(path, readSettings, { signal })
            const desiredName = attempt.draft.teams[0]?.name ?? name
            if (reason === 'uncertain') {
              return isStrongETag(latest.etag) &&
                latest.etag !== attempt.etag &&
                latest.data.name === desiredName
                ? { kind: 'applied', result: latest }
                : { kind: 'unknown' }
            }
            return {
              kind: 'not-applied',
              proposal: prepare(latest, desiredName),
            }
          }}
          onSaved={(result) => {
            setSaved(result)
            setName(result.data.name)
          }}
          onValidation={setValidation}
          onClose={() => {
            setProposal(null)
          }}
        />
      )}
      <UnsavedChangesDialog
        formRef={form}
        formName="Настройки"
        dirty={name !== saved.data.name}
        confirmationOpen={proposal !== null}
        onDiscard={() => {
          setName(saved.data.name)
        }}
      />
    </main>
  )
}

async function mount(fetcher: typeof fetch) {
  const api = new ApiClient(fetcher)
  const initial = await api.read(path, readSettings)
  const router = createMemoryRouter(
    [
      { path, element: <TestForm api={api} initial={initial} /> },
      { path: '/overview', element: <h1 tabIndex={-1}>Обзор</h1> },
      { path: routePaths.sessionExpired, element: <h1>Сессия истекла</h1> },
    ],
    { initialEntries: ['/overview', path] },
  )
  render(<RouterProvider router={router} />)
  return { api, router }
}

function edit(name = 'Инфраструктура') {
  const input = screen.getByRole('textbox', { name: 'Название команды' })
  input.focus()
  fireEvent.change(input, { target: { value: name } })
  return input
}

function open() {
  edit()
  const opener = screen.getByRole('button', { name: 'Сохранить' })
  opener.focus()
  fireEvent.click(opener)
  return screen.getByRole('dialog', { name: 'Подтверждение настроек' })
}

function button(dialog: HTMLElement, name: string) {
  return within(dialog).getByRole('button', { name })
}

function escape(dialog: HTMLElement) {
  fireEvent(dialog, new Event('cancel', { cancelable: true }))
}

describe('UI-17', () => {
  it.each(['Отмена', 'Escape'])(
    'закрывает диалог через %s без записи и возвращает фокус',
    async (action) => {
      const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1))
      await mount(fetcher)
      const dialog = open()
      if (action === 'Escape') escape(dialog)
      else fireEvent.click(button(dialog, action))

      expect(fetcher).toHaveBeenCalledOnce()
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Сохранить' })).toHaveFocus()
      })
      expect(screen.getByRole('textbox')).toHaveValue('Инфраструктура')
    },
  )

  it('удерживает фокус внутри диалога и начинает с безопасной кнопки', async () => {
    await mount(vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1)))
    const dialog = open()
    const cancel = button(dialog, 'Отмена')
    const submit = button(dialog, 'Подтвердить')
    expect(cancel).toHaveFocus()
    expect(fireEvent.keyDown(cancel, { key: 'Tab', shiftKey: true })).toBe(
      false,
    )
    expect(submit).toHaveFocus()
    expect(fireEvent.keyDown(submit, { key: 'Tab' })).toBe(false)
    expect(cancel).toHaveFocus()
  })

  it('отправляет одно подтверждение и закрывается после ответа 200', async () => {
    let resolveRequest: (response: Response) => void = () => {
      throw new Error('Запрос не начался')
    }
    const pending = new Promise<Response>((resolve) => {
      resolveRequest = resolve
    })
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(snapshot(1))
      .mockReturnValueOnce(pending)
    await mount(fetcher)
    const dialog = open()
    const submit = button(dialog, 'Подтвердить')
    act(() => {
      submit.click()
      submit.click()
    })

    expect(button(dialog, 'Сохранение')).toBeDisabled()
    expect(button(dialog, 'Отмена')).toBeDisabled()
    escape(dialog)
    expect(dialog).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(2)
    await act(() => {
      resolveRequest(snapshot(2, 'Инфраструктура'))
      return pending.then(() => undefined)
    })

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText('Действующая версия: 2')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Настройки' })).toHaveFocus()
    })
  })

  it('сохраняет v2 первого автора среди версий и требует нового подтверждения v3', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(snapshot(1))
      .mockResolvedValueOnce(snapshot(1))
      .mockResolvedValueOnce(snapshot(2, 'Инструменты', 'Новый проект'))
      .mockResolvedValueOnce(snapshot(2, 'Инструменты', 'Новый проект'))
      .mockResolvedValueOnce(problemResponse('version_conflict', 412))
      .mockResolvedValueOnce(snapshot(2, 'Инструменты', 'Новый проект'))
      .mockResolvedValueOnce(snapshot(3, 'Инфраструктура', 'Новый проект'))
    const { api } = await mount(fetcher)
    const dialog = open()
    const firstAuthor = new ApiClient(fetcher)
    const first = await firstAuthor.read(path, readSettings)
    expect(first.etag).toBe('"1"')
    const v2 = await firstAuthor.update(
      'PUT',
      path,
      draftBody('Инструменты', 'Новый проект'),
      readSettings,
    )
    expect(v2.data.version).toBe(2)
    // A background read advances the cache while the second author still sees v1.
    await api.read(path, readSettings)
    fireEvent.click(button(dialog, 'Подтвердить'))

    expect(
      await within(dialog).findByText('Было: Инструменты'),
    ).toBeInTheDocument()
    expect(
      within(dialog).getByText('Станет: Инфраструктура'),
    ).toBeInTheDocument()
    expect(within(dialog).getByText('"2"')).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(6)
    expect(
      new Headers(fetcher.mock.calls[4]?.[1]?.headers).get('If-Match'),
    ).toBe('"1"')

    fireEvent.click(button(dialog, 'Подтвердить'))
    expect(await screen.findByText('Действующая версия: 3')).toBeInTheDocument()
    expect(
      new Headers(fetcher.mock.calls[6]?.[1]?.headers).get('If-Match'),
    ).toBe('"2"')
    expect(fetcher.mock.calls[6]?.[1]?.body).toBe(
      JSON.stringify(draftBody('Инфраструктура', 'Новый проект')),
    )
    expect(
      new Headers(fetcher.mock.calls[6]?.[1]?.headers).get('Idempotency-Key'),
    ).not.toBe(
      new Headers(fetcher.mock.calls[4]?.[1]?.headers).get('Idempotency-Key'),
    )
  })

  it('возвращает 422 в сводку формы и связывает ошибку с полем по JSON Pointer', async () => {
    const response = jsonResponse(
      {
        ...problem('validation_failed', 422),
        errors: [{ field, code: 'invalid_format', detail: 'private value' }],
      },
      422,
      { 'Content-Type': 'application/problem+json' },
    )
    await mount(
      vi
        .fn<typeof fetch>()
        .mockResolvedValueOnce(snapshot(1))
        .mockResolvedValueOnce(response),
    )
    fireEvent.click(button(open(), 'Подтвердить'))

    const summary = await screen.findByRole('alert')
    expect(summary).toHaveFocus()
    expect(within(summary).getByRole('link')).toHaveAttribute(
      'href',
      '#team-name',
    )
    const input = screen.getByRole('textbox')
    expect(input).toHaveAccessibleDescription('Введите допустимое название')
    expect(input).toHaveValue('Инфраструктура')
    expect(screen.queryByText(/private value/u)).not.toBeInTheDocument()
    expect(screen.getByText('Действующая версия: 1')).toBeInTheDocument()
  })

  it('при 401 пропускает переход к истёкшей сессии через защиту черновика', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(snapshot(1))
      .mockResolvedValueOnce(problemResponse('auth_required', 401))
    await mount(fetcher)
    fireEvent.click(button(open(), 'Подтвердить'))

    expect(
      await screen.findByRole('heading', { name: 'Сессия истекла' }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(2)
  })

  it('показывает подтверждённое после таймаута сохранение без второго PUT', async () => {
    const fetcher = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(snapshot(1))
      .mockRejectedValueOnce(new DOMException('Timeout', 'TimeoutError'))
      .mockResolvedValueOnce(snapshot(2, 'Инфраструктура'))
    await mount(fetcher)
    const dialog = open()
    fireEvent.click(button(dialog, 'Подтвердить'))

    expect(
      await within(dialog).findByText('Изменение применилось'),
    ).toBeInTheDocument()
    expect(fetcher.mock.calls[2]?.[1]?.method).toBe('GET')
    fireEvent.click(button(dialog, 'Готово'))
    expect(screen.getByText('Действующая версия: 2')).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledTimes(3)
  })

  it('разрешает повтор после таймаута, если операция отклонена и ETag не изменился', async () => {
    const proposal = prepare(
      { data: { version: 1, name: 'Платформа', project: 'Core' }, etag: '"1"' },
      'Инфраструктура',
    )
    const submit = vi
      .fn<(attempt: ChangeAttempt<typeof proposal.draft>) => Promise<string>>()
      .mockRejectedValueOnce(new DOMException('Timeout', 'TimeoutError'))
      .mockResolvedValueOnce('"2"')
    const onSaved = vi.fn()
    const onClose = vi.fn()
    render(
      <ChangeConfirmationDialog
        title="Подтверждение настроек"
        scope="Организация"
        consequences="Название команды изменится."
        proposal={proposal}
        submit={submit}
        recheck={() => Promise.resolve({ kind: 'not-applied', proposal })}
        onSaved={onSaved}
        onValidation={vi.fn()}
        onClose={onClose}
      />,
    )
    const dialog = screen.getByRole('dialog')
    fireEvent.click(button(dialog, 'Подтвердить'))

    expect(
      await within(dialog).findByText('Изменение не применилось'),
    ).toBeInTheDocument()
    expect(button(dialog, 'Отмена')).toBeEnabled()
    expect(submit).toHaveBeenCalledTimes(1)
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.click(button(dialog, 'Подтвердить'))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith('"2"'))
    expect(onClose).toHaveBeenCalledOnce()
    expect(submit.mock.calls.map(([attempt]) => attempt.etag)).toEqual([
      '"1"',
      '"1"',
    ])
    expect(submit.mock.calls[1]?.[0].idempotencyKey).not.toBe(
      submit.mock.calls[0]?.[0].idempotencyKey,
    )
  })

  it('после таймаута ждёт подтверждения отложенной записи и не отправляет второй PUT', async () => {
    let readCurrent = () => snapshot(1)
    const fetcher = vi
      .fn<typeof fetch>()
      .mockImplementation(() => Promise.resolve(readCurrent()))
      .mockResolvedValueOnce(snapshot(1))
      .mockRejectedValueOnce(new DOMException('Timeout', 'TimeoutError'))
    await mount(fetcher)
    const dialog = open()
    fireEvent.click(button(dialog, 'Подтвердить'))

    expect(
      await within(dialog).findByText('Исход сохранения пока неизвестен'),
    ).toBeInTheDocument()
    expect(button(dialog, 'Отмена')).toBeDisabled()
    expect(
      within(dialog).queryByRole('button', { name: 'Подтвердить' }),
    ).not.toBeInTheDocument()
    escape(dialog)
    expect(dialog).toBeInTheDocument()
    fireEvent.click(button(dialog, 'Перечитать ресурс'))
    expect(
      await within(dialog).findByText('Исход сохранения пока неизвестен'),
    ).toBeInTheDocument()
    expect(screen.getByText('Действующая версия: 1')).toBeInTheDocument()

    readCurrent = () => snapshot(2, 'Инфраструктура')
    fireEvent.click(button(dialog, 'Перечитать ресурс'))
    expect(
      await within(dialog).findByText('Изменение применилось'),
    ).toBeInTheDocument()
    fireEvent.click(button(dialog, 'Готово'))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(screen.getByText('Действующая версия: 2')).toBeInTheDocument()
    expect(fetcher.mock.calls.map(([, request]) => request?.method)).toEqual([
      'GET',
      'PUT',
      'GET',
      'GET',
      'GET',
    ])
  })
})

describe('UI-18', () => {
  function tryLeave() {
    const link = screen.getByRole('link', { name: 'Перейти к обзору' })
    link.focus()
    fireEvent.click(link)
    return screen.getByRole('dialog', { name: 'Изменения не сохранены' })
  }

  it('возвращает фокус в поле с прежним вводом после продолжения редактирования', async () => {
    const { router } = await mount(
      vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1)),
    )
    const input = edit()
    const dialog = tryLeave()
    const keep = button(dialog, 'Продолжить редактирование')
    expect(keep).toHaveFocus()
    fireEvent.click(keep)

    await waitFor(() => {
      expect(input).toHaveFocus()
    })
    expect(input).toHaveValue('Инфраструктура')
    expect(router.state.location.pathname).toBe(path)
  })

  it('отменяет Browser Back по Escape и сохраняет введённое значение', async () => {
    const { router } = await mount(
      vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1)),
    )
    const input = edit()
    await act(async () => router.navigate(-1))
    escape(screen.getByRole('dialog', { name: 'Изменения не сохранены' }))

    expect(router.state.location.pathname).toBe(path)
    await waitFor(() => {
      expect(input).toHaveFocus()
    })
    expect(input).toHaveValue('Инфраструктура')
  })

  it('отбрасывает черновик и продолжает навигацию без записи', async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1))
    await mount(fetcher)
    edit()
    fireEvent.click(button(tryLeave(), 'Отбросить изменения'))

    expect(
      await screen.findByRole('heading', { name: 'Обзор' }),
    ).toBeInTheDocument()
    expect(fetcher).toHaveBeenCalledOnce()
  })

  it('не открывает второй диалог до закрытия подтверждения', async () => {
    const { router } = await mount(
      vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1)),
    )
    const dialog = open()
    await act(async () => router.navigate(-1))
    expect(screen.getAllByRole('dialog')).toEqual([dialog])
    escape(dialog)
    expect(
      screen.getByRole('dialog', { name: 'Изменения не сохранены' }),
    ).toBeInTheDocument()
  })

  it('включает beforeunload только при несохранённом вводе', async () => {
    await mount(vi.fn<typeof fetch>().mockResolvedValueOnce(snapshot(1)))
    const beforeUnload = () =>
      window.dispatchEvent(new Event('beforeunload', { cancelable: true }))
    expect(beforeUnload()).toBe(true)
    edit()
    expect(beforeUnload()).toBe(false)
    edit('Платформа')
    expect(beforeUnload()).toBe(true)
  })
})
