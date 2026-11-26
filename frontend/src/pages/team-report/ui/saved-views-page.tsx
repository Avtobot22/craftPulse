import { Button, Text } from '@gravity-ui/uikit'
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { InfiniteData } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import type { RefObject } from 'react'
import { Link } from 'react-router'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import { DialogFrame, ErrorState, LoadingState } from '@/shared/ui'
import { listSavedViews, savedViewKey, savedViewsKey } from '../api/saved-views'
import type { SavedView } from '../api/saved-views'
import { formatTime } from '../model/metrics'
import { savedViewPath } from '../model/saved-view-links'
import { CopySavedView } from './copy-saved-view'
import './team-report.css'

export function SavedViewsPage({ api }: { api: ApiClient }) {
  const heading = useRef<HTMLHeadingElement>(null)
  const [announcement, setAnnouncement] = useState('')
  const [selection, setSelection] = useState<{
    view: SavedView
    opener: HTMLElement
  }>()
  const list = useInfiniteQuery({
    queryKey: savedViewsKey,
    queryFn: ({ pageParam, signal }) =>
      listSavedViews(api, pageParam || undefined, signal),
    initialPageParam: '',
    getNextPageParam: (last, _pages, _cursor, visited) =>
      last.next_cursor && !visited.includes(last.next_cursor)
        ? last.next_cursor
        : undefined,
    refetchOnMount: 'always',
  })
  const rows = [
    ...new Map(
      list.data?.pages.flatMap((page) =>
        page.data.map((view) => [view.id, view] as const),
      ),
    ).values(),
  ]

  return (
    <section className="cp-team-report">
      <h1 ref={heading} tabIndex={-1}>
        Сохранённые срезы
      </h1>
      <Link to={routePaths.teamReport}>К отчёту команды</Link>
      {announcement && <p role="status">{announcement}</p>}
      {list.isPending && <LoadingState />}
      {(list.isRefetchError || list.isLoadingError) && (
        <ErrorState
          reason="Не удалось прочитать список срезов"
          onRetry={() => void list.refetch()}
        />
      )}
      {list.isSuccess && rows.length === 0 && (
        <p>Сохраните срез из отчёта команды, чтобы он появился в списке.</p>
      )}
      {rows.length > 0 && (
        <ul className="cp-team-report__saved-list">
          {rows.map((view) => (
            <li className="cp-team-report__panel" key={view.id}>
              <Text as="h2" variant="subheader-2">
                <Link to={savedViewPath(view.id)}>{view.name}</Link>
              </Text>
              <p>
                {view.pinned_revision_id
                  ? 'Закреплённая ревизия'
                  : 'Актуальная ревизия'}
                {' · Создан '}
                <time dateTime={view.created_at}>
                  {formatTime(view.created_at, 'UTC')} UTC
                </time>
              </p>
              <div className="cp-team-report__actions">
                <CopySavedView id={view.id} name={view.name} />
                <Button
                  size="xl"
                  aria-label={`Удалить срез «${view.name}»`}
                  onClick={(event) => {
                    setAnnouncement('')
                    setSelection({ view, opener: event.currentTarget })
                  }}
                >
                  Удалить
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {list.isFetchNextPageError && (
        <p role="alert">Не удалось загрузить следующую страницу.</p>
      )}
      {list.hasNextPage && (
        <div>
          <Button
            size="xl"
            loading={list.isFetchingNextPage}
            disabled={list.isFetching}
            onClick={() => void list.fetchNextPage()}
          >
            {list.isFetchNextPageError ? 'Повторить загрузку' : 'Показать ещё'}
          </Button>
        </div>
      )}
      {selection && (
        <DeleteViewDialog
          api={api}
          selection={selection}
          heading={heading}
          onClose={() => {
            setSelection(undefined)
          }}
          onDeleted={() => {
            setAnnouncement('Срез удалён')
          }}
        />
      )}
    </section>
  )
}

function DeleteViewDialog({
  api,
  selection,
  heading,
  onClose,
  onDeleted,
}: {
  api: ApiClient
  selection: { view: SavedView; opener: HTMLElement }
  heading: RefObject<HTMLHeadingElement | null>
  onClose: () => void
  onDeleted: () => void
}) {
  const deleted = useRef(false)
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => api.deleteSavedView(selection.view.id),
    onSuccess: () => {
      queryClient.setQueryData<
        InfiniteData<Awaited<ReturnType<typeof listSavedViews>>>
      >(
        savedViewsKey,
        (data) =>
          data && {
            ...data,
            pages: data.pages.map((page) => ({
              ...page,
              data: page.data.filter(({ id }) => id !== selection.view.id),
            })),
          },
      )
      queryClient.removeQueries({ queryKey: savedViewKey(selection.view.id) })
      void queryClient.invalidateQueries({ queryKey: savedViewsKey })
      deleted.current = true
      onDeleted()
      onClose()
    },
  })
  return (
    <DialogFrame
      title="Удалить сохранённый срез?"
      description={`Срез «${selection.view.name}» исчезнет из списка, его ссылка перестанет открываться.`}
      focusSafeAction
      onCancel={() => {
        if (!mutation.isPending) onClose()
      }}
      returnFocus={() => (deleted.current ? heading.current : selection.opener)}
      actions={
        <>
          <Button size="xl" disabled={mutation.isPending} onClick={onClose}>
            Отмена
          </Button>
          <Button
            size="xl"
            view="outlined-danger"
            loading={mutation.isPending}
            disabled={mutation.isPending}
            onClick={() => {
              mutation.mutate()
            }}
          >
            Удалить
          </Button>
        </>
      }
    >
      {mutation.isError && (
        <p role="alert">Не удалось удалить срез. Повторите попытку.</p>
      )}
    </DialogFrame>
  )
}
