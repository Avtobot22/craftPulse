import { Text } from '@gravity-ui/uikit'
import { useRef, useState } from 'react'
import {
  Navigate,
  useLocation,
  useOutletContext,
  useSearchParams,
} from 'react-router'
import type { ApiClient, Team } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { EmptyState } from '@/shared/ui'
import { readTimelineSearch } from '../model/timeline-search'
import { TimelineForm } from './timeline-form'
import { TimelineResults } from './timeline-results'
import './project-timeline-page.css'

export function ProjectTimelinePage({ api }: { api: ApiClient }) {
  const heading = useRef<HTMLHeadingElement>(null)
  const location = useLocation()
  const { pathname, hash } = location
  const state: unknown = location.state
  const [search, setSearch] = useSearchParams()
  const scope = useOutletContext<{
    project?: Team
    actions: CurrentUser['actions']
  }>()
  const [today] = useState(() => new Date().toISOString().slice(0, 10))
  const { params, draft, issue } = readTimelineSearch(search, today)
  const project = scope.project

  if (project && !['date', 'from', 'to'].some((key) => search.has(key))) {
    const next = new URLSearchParams(search)
    next.set('date', draft.date)
    return (
      <Navigate
        to={{
          pathname,
          search: next.toString(),
          hash,
        }}
        replace
        state={state}
      />
    )
  }

  function update(values: Record<string, string>) {
    const next = new URLSearchParams(search)
    next.delete('cursor')
    for (const [key, value] of Object.entries(values)) {
      if (value) next.set(key, value)
      else next.delete(key)
    }
    heading.current?.focus()
    setSearch(next)
  }

  return (
    <section className="cp-timeline" aria-labelledby="project-timeline-heading">
      <Text
        as="h1"
        variant="header-1"
        id="project-timeline-heading"
        ref={heading}
        tabIndex={-1}
      >
        Таймлайн проекта
      </Text>
      {project ? (
        <>
          <TimelineForm
            key={search.toString()}
            draft={draft}
            issue={issue}
            apply={update}
          />
          {params && (
            <TimelineResults
              key={JSON.stringify([project.id, params])}
              api={api}
              projectId={project.id}
              params={params}
              canReadRevision={scope.actions.includes('report.read')}
            />
          )}
        </>
      ) : (
        <EmptyState
          reason="filters"
          title="Выберите проект"
          description="Проект можно выбрать в верхней панели."
        />
      )}
    </section>
  )
}
