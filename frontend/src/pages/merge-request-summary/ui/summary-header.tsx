import { useQuery } from '@tanstack/react-query'
import type { SubmitEvent } from 'react'
import { Link, useLocation, useNavigate } from 'react-router'
import { CommitSha, LastGoodResult } from '@/entities/check-report'
import { getMergeRequest } from '@/shared/api'
import type { ApiClient, components, OpenCodes } from '@/shared/api'
import { contractLabel, technicalLabels } from '@/shared/config'

type Summary = OpenCodes<components['schemas']['MergeRequestSummary']>

function sourceUrl(raw: string | null | undefined) {
  const url = raw ? URL.parse(raw) : null
  return url && ['https:', 'http:'].includes(url.protocol)
    ? url.href
    : undefined
}

export function SummaryHeader({
  api,
  summary,
  refresh,
}: {
  api: ApiClient
  summary: Summary
  refresh: () => void
}) {
  const { search } = useLocation()
  const navigate = useNavigate()
  const query = useQuery({
    ...getMergeRequest(api, summary.merge_request_id),
    retry: false,
  })
  const mr =
    query.data?.id === summary.merge_request_id ? query.data : undefined
  const source = sourceUrl(mr?.source_url)
  const params = new URLSearchParams(search)
  const pinned = params.has('revision_no') || params.has('head_sha')
  const currentHead = mr?.head_sha
  const newHead = currentHead !== undefined && currentHead !== summary.head_sha
  const headChanged = newHead || summary.stale_reason === 'new_head'
  const historical =
    summary.status === 'stale' || summary.status === 'superseded' || headChanged
  const currentParams = new URLSearchParams(params)
  currentParams.delete('head_sha')
  currentParams.delete('revision_no')
  if (newHead) currentParams.set('head_sha', currentHead)

  function openRevision(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    const revision = new FormData(event.currentTarget).get('revision_no')
    if (typeof revision !== 'string') return
    const next = new URLSearchParams(params)
    next.delete('head_sha')
    next.set('revision_no', revision)
    void navigate({ search: next.toString() })
  }

  const lastGood = summary.last_good
  const lastGoodParams = new URLSearchParams(params)
  if (lastGood) {
    lastGoodParams.set('head_sha', lastGood.head_sha)
    lastGoodParams.set('revision_no', String(lastGood.revision_no))
  }

  return (
    <header className="cp-summary-header">
      <h1>Сводка MR, ревизия {summary.revision_no}</h1>
      <p>
        MR{' '}
        {mr?.iid == null ? (
          <code>{summary.merge_request_id}</code>
        ) : (
          `!${String(mr.iid)}`
        )}
        {mr?.title && ` · ${mr.title}`}
      </p>
      {source && <a href={source}>Открыть MR в GitLab</a>}
      <dl>
        <dt>Head</dt>
        <dd>
          <CommitSha sha={summary.head_sha} />
        </dd>
        <dt>База</dt>
        <dd>
          {summary.base_sha ? (
            <CommitSha sha={summary.base_sha} />
          ) : (
            'Сопоставимой базы нет'
          )}
        </dd>
        <dt>Режим</dt>
        <dd>Только предупреждения</dd>
        <dt>Состояние сводки</dt>
        <dd>{contractLabel(technicalLabels.summaryStatus, summary.status)}</dd>
        <dt>Pipeline</dt>
        <dd>
          {contractLabel(technicalLabels.pipelineState, summary.pipeline_state)}
        </dd>
        <dt>Время построения</dt>
        <dd>
          {summary.built_at ? (
            <time dateTime={summary.built_at}>{summary.built_at}</time>
          ) : (
            'Сводка ещё не построена'
          )}
        </dd>
      </dl>
      {historical && (
        <div className="cp-summary-notice" role="status">
          <strong>Историческая сводка</strong>
          {headChanged && (
            <p>В MR появился новый коммит. Сводка относится к прежнему head.</p>
          )}
          {summary.stale_reason && summary.stale_reason !== 'new_head' && (
            <p>Причина: {summary.stale_reason}</p>
          )}
        </div>
      )}
      {(pinned || newHead) && (
        <Link to={{ search: currentParams.toString(), hash: '' }}>
          Открыть актуальную сводку
        </Link>
      )}
      {historical && !pinned && !newHead && (
        <button
          type="button"
          onClick={() => {
            refresh()
            void query.refetch()
          }}
        >
          Обновить сводку
        </button>
      )}
      {lastGood && (
        <aside aria-label="Последний хороший результат">
          <LastGoodResult result={lastGood} />
          {lastGood.revision_no !== summary.revision_no && (
            <Link to={{ search: lastGoodParams.toString(), hash: '' }}>
              Открыть ревизию {lastGood.revision_no}
            </Link>
          )}
        </aside>
      )}
      <p>
        Комментарий в MR:{' '}
        {contractLabel(
          technicalLabels.publicationStatus,
          summary.publication.status,
        )}
      </p>
      {summary.publication.error_code && (
        <p>
          {contractLabel(
            technicalLabels.publicationError,
            summary.publication.error_code,
          )}
        </p>
      )}
      {summary.publication.published_at && (
        <p>
          Опубликован{' '}
          <time dateTime={summary.publication.published_at}>
            {summary.publication.published_at}
          </time>
        </p>
      )}
      {summary.publication.external_id && (
        <p>
          ID комментария <code>{summary.publication.external_id}</code>
        </p>
      )}
      <form className="cp-summary-revision" onSubmit={openRevision}>
        <label>
          Номер ревизии{' '}
          <input
            key={summary.revision_no}
            name="revision_no"
            required
            inputMode="numeric"
          pattern="[1-9][0-9]{0,9}"
          onChange={(event) => {
            const input = event.currentTarget
            input.setCustomValidity(
              Number(input.value) > 2147483647
                ? 'Номер ревизии выходит за допустимый диапазон.'
                : '',
            )
          }}
            defaultValue={summary.revision_no}
          />
        </label>
        <button type="submit">Открыть ревизию</button>
      </form>
    </header>
  )
}
