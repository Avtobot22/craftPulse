import { Button, Text } from '@gravity-ui/uikit'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useOutletContext } from 'react-router'
import { ApiError, getPersons } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { LoadingState, UnavailableRedirect } from '@/shared/ui'
import { collectChoices, getAreas, getStructure } from '../api/structure'
import { StructureEditor } from './structure-editor'
import './organization.css'

export function OrganizationPage({ api }: { api: ApiClient }) {
  const current = useOutletContext<CurrentUser>()
  if (!current.actions.includes('org.manage')) return <UnavailableRedirect />
  return (
    <OrganizationContent
      key={current.organization.id + ':' + current.user.id}
      api={api}
      current={current}
    />
  )
}

function OrganizationContent({
  api,
  current,
}: {
  api: ApiClient
  current: CurrentUser
}) {
  const organizationId = current.organization.id
  const client = useQueryClient()
  const structure = useQuery({
    queryKey: ['organization-structure', organizationId],
    queryFn: ({ signal }) => getStructure(api, signal),
    refetchOnWindowFocus: false,
    refetchOnMount: 'always',
  })
  const areas = useQuery({
    queryKey: ['organization-structure', organizationId, 'areas'],
    queryFn: ({ signal }) => getAreas(api, signal),
    enabled: structure.isSuccess,
  })
  const people = useQuery({
    queryKey: ['organization-structure', organizationId, 'people'],
    queryFn: ({ signal }) =>
      collectChoices(async (cursor) => {
        const page = await getPersons(
          api,
          { kind: 'human', ...(cursor === undefined ? {} : { cursor }) },
          signal,
        )
        return {
          ...page,
          data: page.data.map(({ id, display_name }) => ({
            id,
            name: display_name ?? id,
          })),
        }
      }),
    enabled: structure.isSuccess,
  })

  if (
    structure.error instanceof ApiError &&
    structure.error.kind === 'forbidden'
  )
    return <UnavailableRedirect />
  return (
    <section className="cp-org">
      <Text as="h1" variant="header-1">
        Структура организации
      </Text>
      {structure.data && structure.isFetchedAfterMount ? (
        <StructureEditor
          api={api}
          initial={structure.data}
          catalog={{
            areas: areas.data ?? [],
            people: people.data ?? [],
            areaProblem: areas.isPending
              ? 'Загружаем области источников'
              : areas.isError
                ? 'Не удалось загрузить области источников'
                : undefined,
            peopleProblem: people.isPending
              ? 'Загружаем участников'
              : people.isError
                ? 'Не удалось загрузить участников'
                : undefined,
          }}
          onSaved={(result) => {
            client.setQueryData(
              ['organization-structure', organizationId],
              result,
            )
            client.setQueryData(
              ['organization-structure-version', organizationId],
              result.data.version,
            )
          }}
        />
      ) : structure.isFetching || structure.isPending ? (
        <LoadingState />
      ) : (
        <div role="alert">
          <p>Не удалось загрузить структуру организации.</p>
          <Button onClick={() => void structure.refetch()}>
            Повторить загрузку
          </Button>
        </div>
      )}
      {(areas.isError || people.isError) && (
        <Button
          onClick={() => {
            if (areas.isError) void areas.refetch()
            if (people.isError) void people.refetch()
          }}
        >
          Повторить загрузку справочников
        </Button>
      )}
    </section>
  )
}
