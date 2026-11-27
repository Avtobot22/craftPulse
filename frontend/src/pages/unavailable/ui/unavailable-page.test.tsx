import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { expect, it, vi } from 'vitest'
import { ApiClient } from '@/shared/api'
import type { components } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { UnavailablePage } from './unavailable-page'

const source = {
  connection_id: '0193a5b0-5555-7000-8000-000000000003',
  provider: 'confluence_dc',
  display_name: 'Confluence',
  state: 'not_linked',
  linked_at: null,
} satisfies components['schemas']['SourceLink']

function mount(response: Response, forbidden = false) {
  const fetcher = vi.fn<typeof fetch>().mockResolvedValue(response)
  const api = new ApiClient(fetcher)
  const queries = new QueryClient()
  render(
    <QueryClientProvider client={queries}>
      <MemoryRouter initialEntries={[routePaths.unavailable]}>
        <UnavailablePage api={api} forbidden={forbidden} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { fetcher, queries }
}

it.each(['not_linked', 'expired', 'revoked'] as const)(
  'offers consent for a %s connection and keeps focus on the heading',
  async (state) => {
    mount(Response.json({ data: [{ ...source, state }] }))
    const offer = await screen.findByRole('region', { name: 'Confluence' })
    expect(
      within(offer).getByRole('button', { name: 'Разрешить доступ' }),
    ).toHaveProperty('tabIndex', 0)
    expect(
      screen.getByRole('heading', { name: 'Объект или область недоступны' }),
    ).toHaveFocus()
  },
)

it.each([
  [
    'only a linked connection',
    () => Response.json({ data: [{ ...source, state: 'linked' }] }),
  ],
  ['no user-token connections', () => Response.json({ data: [] })],
  ['a list failure', () => new Response(null, { status: 503 })],
] as const)('keeps the basic denial with %s', async (_scenario, response) => {
  const { fetcher, queries } = mount(response())
  await waitFor(() => {
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(queries.isFetching()).toBe(0)
  })
  expect(
    screen.getByRole('heading', { name: 'Объект или область недоступны' }),
  ).toBeVisible()
  expect(screen.getByRole('button', { name: 'К списку' })).toBeVisible()
  expect(screen.queryByRole('button', { name: 'Разрешить доступ' })).toBeNull()
  expect(screen.queryByRole('region', { name: 'Confluence' })).toBeNull()
})
