import { Text } from '@gravity-ui/uikit'
import { useRef } from 'react'
import { useSearchParams } from 'react-router'
import type { ApiClient } from '@/shared/api'
import { readFindingSearch } from '../model/finding-search'
import { FindingFilters } from './finding-filters'
import { FindingResults } from './finding-results'
import './findings-page.css'

export function FindingsPage({ api }: { api: ApiClient }) {
  const [search, setSearch] = useSearchParams()
  const heading = useRef<HTMLHeadingElement>(null)
  const params = readFindingSearch(search)
  const formKey = JSON.stringify([
    params.component_id,
    params.project_id,
    params.service_id,
    params.finding_kind,
    params.state,
    params.severity,
    params.sort,
  ])

  function apply(values: URLSearchParams) {
    const next = new URLSearchParams()
    for (const key of ['org_slug', 'team_id', 'limit']) {
      const value = search.get(key)
      if (value) next.set(key, value)
    }
    for (const [key, value] of values) next.set(key, value)
    heading.current?.focus()
    setSearch(next)
  }

  return (
    <section className="cp-findings">
      <Text
        as="h1"
        id="findings-heading"
        variant="header-1"
        ref={heading}
        tabIndex={-1}
      >
        Находки кода и безопасности
      </Text>
      <FindingFilters key={formKey} params={params} apply={apply} />
      <FindingResults api={api} params={params} />
    </section>
  )
}
