import { useQueries } from '@tanstack/react-query'
import { Link } from 'react-router'
import { SourceLink } from '@/entities/activity'
import { ReviewWaitDetails } from '@/entities/merge-request'
import { RestrictedResults } from '@/entities/result-access'
import { listMergeRequests } from '@/shared/api'
import type { ApiClient, TeamReport } from '@/shared/api'
import { taskAnalysisLinks } from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { ErrorState, LoadingState } from '@/shared/ui'

const groups = [
  { kind: 'no_reviewer', label: 'Нет ревьюера' },
  { kind: 'awaiting_first_response', label: 'Нет первого ответа человека' },
] as const

type Project = TeamReportScope['projects'][number]

export function TeamMRQueue({
  api,
  report,
  projects,
}: {
  api: ApiClient
  report: TeamReport
  projects: readonly Project[]
}) {
  if (projects.length === 0) return null
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-queue"
    >
      <h2 id="team-report-queue">Очередь MR команды</h2>
      <p>Текущее ожидание. Первая страница по каждому проекту.</p>
      {groups.map((group) => (
        <QueueGroup
          key={group.kind}
          api={api}
          report={report}
          projects={projects}
          group={group}
        />
      ))}
    </section>
  )
}

function QueueGroup({
  api,
  report,
  projects,
  group,
}: {
  api: ApiClient
  report: TeamReport
  projects: readonly Project[]
  group: (typeof groups)[number]
}) {
  const queries = useQueries({
    queries: projects.map(({ id }) => ({
      ...listMergeRequests(api, {
        project_id: id,
        state: 'open',
        review_wait: group.kind,
        sort: '-wait',
        limit: 25,
      }),
      refetchOnMount: 'always' as const,
    })),
  })
  const entries = projects.flatMap((project, index) => {
    const query = queries[index]
    const page =
      query?.isFetchedAfterMount && !query.isError ? query.data : undefined
    return (page?.data ?? []).flatMap((mr) =>
      mr.state === 'open' && !mr.draft && mr.review_wait?.kind === group.kind
        ? [{ project, mr, wait: mr.review_wait }]
        : [],
    )
  })
  entries.sort((a, b) => b.wait.wait_seconds - a.wait.wait_seconds)
  const seen = new Set<string>()
  const uniqueEntries = entries.filter(({ mr }) => {
    if (seen.has(mr.id)) return false
    seen.add(mr.id)
    return true
  })
  const failed = queries.filter((query) => query.isError)
  const pending = queries.some(
    (query) =>
      query.isPending || (query.isFetching && !query.isFetchedAfterMount),
  )
  const limited = queries.some(
    ({ data, isFetchedAfterMount, isError }) =>
      isFetchedAfterMount &&
      !isError &&
      (data?.access_limited === true || (data?.access_notes.length ?? 0) > 0),
  )
  const heading = 'team-report-queue-' + group.kind

  return (
    <section
      className="cp-team-report__panel cp-team-report__detail"
      aria-labelledby={heading}
    >
      <h3 id={heading}>{group.label}</h3>
      <RestrictedResults access_limited={limited} />
      {failed.length > 0 && (
        <ErrorState
          reason="Не удалось прочитать очередь одного или нескольких проектов"
          onRetry={() =>
            void Promise.all(failed.map((query) => query.refetch()))
          }
          retrying={failed.some((query) => query.isFetching)}
        />
      )}
      {pending ? (
        <LoadingState />
      ) : uniqueEntries.length > 0 ? (
        <div className="cp-team-report__table-scroll">
          <table aria-labelledby={heading}>
            <thead>
              <tr>
                <th scope="col">MR</th>
                <th scope="col" aria-sort="descending">
                  Ожидание
                </th>
                <th scope="col">Проект</th>
                <th scope="col">Источник</th>
              </tr>
            </thead>
            <tbody>
              {uniqueEntries.map(({ mr, project, wait }) => (
                <tr key={mr.id}>
                  <th scope="row">
                    {mr.iid === null ? '' : `!${String(mr.iid)} · `}
                    {mr.title ?? 'Запрос на слияние'}
                  </th>
                  <td>
                    <ReviewWaitDetails wait={wait} showKind={false} />
                  </td>
                  <td>
                    <Link
                      to={taskAnalysisLinks.mergeRequestQueue({
                        team_id: report.team_id,
                        project_id: project.id,
                        review_wait: group.kind,
                        sort: '-wait',
                        ...report.period,
                      })}
                    >
                      {project.name}
                    </Link>
                  </td>
                  <td>
                    {mr.source_url ? (
                      <SourceLink url={mr.source_url} sourceName="GitLab" />
                    ) : (
                      'Ссылка на GitLab недоступна'
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        failed.length === 0 && <p>В доступной очереди нет MR.</p>
      )}
    </section>
  )
}
