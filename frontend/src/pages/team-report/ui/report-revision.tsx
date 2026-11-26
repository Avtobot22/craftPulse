import { Button, Label } from '@gravity-ui/uikit'
import { useMutation } from '@tanstack/react-query'
import type { ApiClient } from '@/shared/api'
import { ErrorState } from '@/shared/ui'
import { refreshReport } from '../api/report-api'
import type { RevisionMetadata, TeamReport } from '@/shared/api'
import { formatTime } from '../model/metrics'

const irreproducible: Readonly<Record<string, string>> = {
  basis_deleted: 'основание удалено',
  basis_expired: 'срок хранения основания истёк',
  definition_retired: 'версия определения снята с использования',
}

export function ReportRevision({
  api,
  report,
  metadata,
  isHistorical,
  isReading,
  onRead,
  onLatest,
}: {
  api: ApiClient
  report: TeamReport
  metadata: RevisionMetadata | undefined
  isHistorical: boolean
  isReading: boolean
  onRead: () => void
  onLatest: () => void
}) {
  const { revision } = report
  const timeZone = metadata?.timeZone ?? 'UTC'
  const refresh = useMutation({
    mutationFn: () => refreshReport(api, revision.series_id),
  })
  const requestRefresh = () => {
    refresh.mutate()
  }
  const failed = refresh.isError || refresh.data?.data === 'failed'
  return (
    <section aria-label="Ревизия отчёта" className="cp-team-report__panel">
      <dl className="cp-team-report__revision">
        <div>
          <dt>Период</dt>
          <dd>
            <time dateTime={report.period.from}>
              {formatTime(report.period.from, timeZone)}
            </time>
            {' - '}
            <time dateTime={report.period.to}>
              {formatTime(report.period.to, timeZone)}
            </time>
            {' (конец не входит)'}
          </dd>
        </div>
        <div>
          <dt>Ревизия</dt>
          <dd>{revision.revision_no}</dd>
        </div>
        <div>
          <dt>Состояние на</dt>
          <dd>
            <time dateTime={revision.as_of}>
              {formatTime(revision.as_of, timeZone)}
            </time>
          </dd>
        </div>
        <div>
          <dt>Опубликовано</dt>
          <dd>
            {revision.published_at ? (
              <time dateTime={revision.published_at}>
                {formatTime(revision.published_at, timeZone)}
              </time>
            ) : (
              'Публикация ожидается'
            )}
          </dd>
        </div>
        <div>
          <dt>Часовой пояс</dt>
          <dd>
            {metadata
              ? timeZone
              : 'Часовой пояс ревизии недоступен, даты в UTC'}
          </dd>
        </div>
      </dl>
      <ul className="cp-team-report__chips" aria-label="Состояние ревизии">
        {isHistorical && (
          <li>
            <Label>Закреплённая ревизия</Label>
          </li>
        )}
        {report.grouping === 'current_structure' && (
          <li>
            <Label>По текущему составу команды</Label>
          </li>
        )}
        {report.grouping === 'structure_at_date' && (
          <li>
            <Label>По составу команды на дату</Label>
          </li>
        )}
        {revision.loading && (
          <li>
            <Label theme="warning">
              Загрузка истории не завершена, значения частичные
            </Label>
          </li>
        )}
        {report.access_limited && (
          <li>
            <Label theme="warning">Доступ к данным ограничен</Label>
          </li>
        )}
        {revision.irreproducible_reason && (
          <li>
            <Label theme="warning">
              Ревизия невоспроизводима (
              {irreproducible[revision.irreproducible_reason] ??
                'причина не уточнена'}
              )
            </Label>
          </li>
        )}
      </ul>
      <div className="cp-team-report__actions">
        <Button size="xl" loading={refresh.isPending} onClick={requestRefresh}>
          Обновить
        </Button>
        {isHistorical ? (
          <Button size="xl" onClick={onLatest}>
            К актуальной ревизии
          </Button>
        ) : (
          <Button size="xl" loading={isReading} onClick={onRead}>
            Перечитать отчёт
          </Button>
        )}
      </div>
      {refresh.isSuccess && !failed && (
        <p role="status">
          {refresh.data.data === 'succeeded'
            ? 'Пересчёт завершён. Перечитайте отчёт, чтобы увидеть опубликованную ревизию.'
            : 'Пересчёт поставлен в очередь. Действующая ревизия доступна до публикации новой.'}
        </p>
      )}
      {failed && (
        <ErrorState
          reason="Не удалось поставить пересчёт ряда"
          onRetry={requestRefresh}
        />
      )}
    </section>
  )
}
