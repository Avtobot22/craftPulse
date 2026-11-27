import { generatePath, Link } from 'react-router'
import { ActivityTime, ActorKindBadge, SourceLink } from '@/entities/activity'
import { ReviewWaitDetails } from '@/entities/merge-request'
import type {
  components,
  MergeRequestListParams,
  OpenCodes,
} from '@/shared/api'
import { routePaths } from '@/shared/config'

type MergeRequest = OpenCodes<components['schemas']['MergeRequest']>

export function QueueRows({
  requests,
  labelledBy,
  sort,
}: {
  requests: MergeRequest[]
  labelledBy: string
  sort: MergeRequestListParams['sort']
}) {
  return (
    <div className="cp-mr-queue__table-wrap">
      <table
        className="cp-mr-queue__table"
        role="table"
        aria-labelledby={labelledBy}
      >
        <thead role="rowgroup">
          <tr role="row">
            <th scope="col">MR</th>
            <th scope="col">Репозиторий</th>
            <th scope="col">Автор</th>
            <th scope="col">Готовность</th>
            <th
              scope="col"
              aria-sort={
                sort === '-wait'
                  ? 'descending'
                  : sort === 'wait'
                    ? 'ascending'
                    : undefined
              }
            >
              Ожидание
            </th>
            <th scope="col">Действие</th>
          </tr>
        </thead>
        <tbody role="rowgroup">
          {requests.map((mr) => {
            const key = mr.iid === null ? '' : `!${String(mr.iid)} · `
            const readyAt =
              mr.ready_at ??
              (mr.review_wait?.fallback_open === false
                ? mr.review_wait.since
                : null)
            const summary =
              generatePath(routePaths.mergeRequestSummary, {
                mr: encodeURIComponent(mr.id),
              }) +
              '?' +
              new URLSearchParams({ head_sha: mr.head_sha }).toString()
            return (
              <tr key={mr.id} role="row">
                <th scope="row" role="rowheader">
                  <Link to={summary}>
                    {key}
                    {mr.title ?? 'Запрос на слияние'}
                  </Link>
                </th>
                <td role="cell">
                  <span
                    className="cp-mr-queue__mobile-label"
                    aria-hidden="true"
                  >
                    Репозиторий
                  </span>
                  {mr.area.display_name}
                </td>
                <td role="cell">
                  <span
                    className="cp-mr-queue__mobile-label"
                    aria-hidden="true"
                  >
                    Автор
                  </span>
                  {mr.author.display_name ?? 'Неизвестный автор'}{' '}
                  <ActorKindBadge kind={mr.author.kind} />
                </td>
                <td role="cell">
                  <span
                    className="cp-mr-queue__mobile-label"
                    aria-hidden="true"
                  >
                    Готовность
                  </span>
                  {readyAt ? (
                    <ActivityTime instant={readyAt} />
                  ) : (
                    'Дата готовности неизвестна'
                  )}
                </td>
                <td role="cell">
                  <span
                    className="cp-mr-queue__mobile-label"
                    aria-hidden="true"
                  >
                    Ожидание
                  </span>
                  <ReviewWaitDetails wait={mr.review_wait} showKind={false} />
                </td>
                <td role="cell">
                  {mr.source_url ? (
                    <SourceLink url={mr.source_url} sourceName="GitLab" />
                  ) : (
                    'Ссылка на GitLab недоступна'
                  )}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
