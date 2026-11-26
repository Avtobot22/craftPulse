import { Text } from '@gravity-ui/uikit'
import { useQuery } from '@tanstack/react-query'
import { Link, Navigate, useOutletContext, useSearchParams } from 'react-router'
import { ApiError } from '@/shared/api'
import type { ApiClient } from '@/shared/api'
import { routePaths } from '@/shared/config'
import type { TeamReportScope } from '@/shared/config'
import { EmptyState, LoadingState } from '@/shared/ui'
import { baselineQuery, mappingQuery } from '../api/target-api'
import type { TargetGroup } from '../api/target-api'
import {
  isAccessError,
  readPeriodInputs,
  toTargetPeriod,
} from '../model/target-settings'
import { TargetBaselines } from './target-baselines'
import {
  readTargetGroup,
  TargetGroupForm,
  TargetPeriodForm,
} from './target-controls'
import { TargetEditor } from './target-editor'
import { TargetReadError } from './target-read-error'
import './target-settings.css'

export function TargetSettingsPage({ api }: { api: ApiClient }) {
  const { team, project, canManageProcess } =
    useOutletContext<TeamReportScope>()
  return (
    <section className="cp-targets">
      <Text as="h1" variant="header-1">
        Целевые сроки
      </Text>
      {team && canManageProcess ? (
        <TargetSettings
          key={team.id}
          api={api}
          team={team}
          projectId={project?.id}
        />
      ) : (
        <p>Выберите команду, в которой вы управляете процессом.</p>
      )}
    </section>
  )
}

function TargetSettings({
  api,
  team,
  projectId,
}: {
  api: ApiClient
  team: NonNullable<TeamReportScope['team']>
  projectId: string | undefined
}) {
  const [search, setSearch] = useSearchParams()
  const dates = readPeriodInputs(search)
  const period = toTargetPeriod(dates.from, dates.to)
  const group = readTargetGroup(
    search.get('task_type'),
    search.get('size_label'),
  )
  const mapping = useQuery(mappingQuery(api, team.id))
  const baselines = useQuery({
    ...baselineQuery(api, team.id, period ?? { from: '', to: '' }),
    enabled: mapping.data === true && period !== null,
  })
  if (isAccessError(mapping.error) || isAccessError(baselines.error)) {
    return <Navigate to={routePaths.unavailable} replace />
  }

  const row =
    !baselines.isError && group
      ? baselines.data?.data.find(
          ({ task_type, size_label }) =>
            task_type === group.task_type && size_label === group.size_label,
        )
      : undefined
  const seconds = Math.round(row?.p85.value ?? 0)
  const historical =
    baselines.data &&
    period &&
    row &&
    (row.p85.state === 'ASSESSED' || row.p85.state === 'PROVISIONAL') &&
    row.p85.unit === 'seconds' &&
    Number.isSafeInteger(seconds) &&
    seconds > 0
      ? {
          seconds,
          revision: baselines.data.revision_id,
          period,
          lowSample: row.p85.low_sample,
        }
      : null

  const scope = {
    team_id: team.id,
    ...(projectId && { project_id: projectId }),
  }
  const mappingLink =
    routePaths.workflowSettings + '?' + new URLSearchParams(scope).toString()
  const reportLink =
    routePaths.teamReport +
    '?' +
    new URLSearchParams({
      ...scope,
      ...period,
      ...group,
      step: 'week',
    }).toString()

  function groupParams(nextGroup: TargetGroup) {
    const params = new URLSearchParams(search)
    for (const [key, value] of Object.entries({
      ...scope,
      ...period,
      ...nextGroup,
    }))
      params.set(key, value)
    return params
  }

  const groupKey = JSON.stringify(group)
  const baselineMissing =
    baselines.error instanceof ApiError && baselines.error.kind === 'not-found'
  return (
    <>
      <p>Команда {team.name}</p>
      {mapping.isPending && <LoadingState />}
      {mapping.isError && (
        <TargetReadError
          error={mapping.error}
          reason="Не удалось проверить сопоставление статусов команды."
          retry={() => {
            void mapping.refetch()
          }}
          retrying={mapping.isFetching}
          updatedAt={mapping.dataUpdatedAt}
        />
      )}
      {mapping.isSuccess && !mapping.data && (
        <div role="status">
          <p>Сначала подтвердите сопоставление статусов команды.</p>
          <Link to={mappingLink}>Настроить сопоставление</Link>
        </div>
      )}
      {mapping.data === true && (
        <div className="cp-targets__columns">
          <section
            className="cp-targets__panel"
            aria-labelledby="target-baselines-heading"
          >
            <Text as="h2" variant="subheader-3" id="target-baselines-heading">
              Исторический ориентир
            </Text>
            <TargetPeriodForm
              key={dates.from + '/' + dates.to}
              {...dates}
              loading={baselines.isFetching}
              apply={(next) => {
                if (period?.from === next.from && period.to === next.to) {
                  void baselines.refetch()
                  return
                }
                const params = new URLSearchParams(search)
                params.set('from', next.from)
                params.set('to', next.to)
                setSearch(params)
              }}
            />
            {!period && <p role="status">Проверьте период.</p>}
            {baselines.isFetching && <LoadingState />}
            {baselineMissing ? (
              <EmptyState
                reason="history"
                description="Опубликованной истории для ориентира пока нет. Можно задать срок вручную."
                action={{
                  label: 'Повторить',
                  onClick: () => {
                    void baselines.refetch()
                  },
                }}
              />
            ) : baselines.isError ? (
              <TargetReadError
                error={baselines.error}
                reason="Не удалось загрузить ориентир. Срок можно задать вручную."
                retry={() => {
                  void baselines.refetch()
                }}
                retrying={baselines.isFetching}
                updatedAt={baselines.dataUpdatedAt}
              />
            ) : (
              baselines.data &&
              (baselines.data.data.length === 0 ? (
                <EmptyState
                  reason="filters"
                  description="Нет групп с задачами за выбранный период."
                />
              ) : (
                <TargetBaselines
                  rows={baselines.data.data}
                  selected={group}
                  groupLink={(next) => '?' + groupParams(next).toString()}
                />
              ))
            )}
          </section>
          <div className="cp-targets__group">
            <TargetGroupForm
              key={groupKey}
              group={group}
              select={(next) => {
                if (
                  next.task_type !== group?.task_type ||
                  next.size_label !== group.size_label
                ) {
                  setSearch(groupParams(next))
                }
              }}
            />
            {group && (
              <TargetEditor
                key={groupKey}
                api={api}
                teamId={team.id}
                group={group}
                historical={historical}
                reportLink={reportLink}
                refreshBaseline={() => {
                  if (period) void baselines.refetch()
                }}
              />
            )}
          </div>
        </div>
      )}
    </>
  )
}
