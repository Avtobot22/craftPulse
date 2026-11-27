import { Card, Label, Text } from '@gravity-ui/uikit'
import {
  generatePath,
  Link,
  useOutletContext,
  useSearchParams,
} from 'react-router'
import { ActivityTime, SourceLink } from '@/entities/activity'
import { CheckReportState } from '@/entities/check-report'
import { MetricValue } from '@/entities/metric'
import type { CurrentUser } from '@/shared/auth'
import {
  contractLabel,
  contractLabels,
  routePaths,
  technicalLabels,
} from '@/shared/config'
import { DataStateBadge } from '@/shared/ui'
import type { Pipeline, Usage } from '../model/pipeline'
import { secondsText } from '../model/pipeline'

export function AttemptHistory({ state }: { state: string }) {
  if (state === 'complete') return 'Полная'
  if (state === 'truncated') return 'Известны не все повторы'
  return 'Полнота истории неизвестна'
}

export function PipelineOverview({ pipeline }: { pipeline: Pipeline }) {
  const user = useOutletContext<CurrentUser | undefined>()
  const [params] = useSearchParams()
  const mr = pipeline.merge_request_id
  const mrParams = new URLSearchParams({ head_sha: pipeline.sha })
  for (const key of ['org_slug', 'team_id', 'project_id']) {
    const value = params.get(key)
    if (value) mrParams.set(key, value)
  }
  return (
    <Card className="cp-ci-panel">
      <Text as="h2" variant="subheader-2">
        Запуск CI
      </Text>
      <dl className="cp-ci-facts">
        <div>
          <dt>Исход</dt>
          <dd>
            <Label
              theme={pipeline.outcome === 'success' ? 'success' : 'normal'}
            >
              {contractLabel(contractLabels.ciOutcome, pipeline.outcome)}
            </Label>
          </dd>
        </div>
        <div>
          <dt>Статус в источнике</dt>
          <dd>{pipeline.native_status || 'Неизвестен'}</dd>
        </div>
        <div>
          <dt>Ветка или тег</dt>
          <dd>{pipeline.ref_name || 'Нет данных'}</dd>
        </div>
        <div>
          <dt>SHA</dt>
          <dd>
            <code>{pipeline.sha}</code>
          </dd>
        </div>
        <div>
          <dt>Merge request</dt>
          <dd>
            {mr && user?.actions.includes('mr.read') ? (
              <Link
                to={{
                  pathname: generatePath(routePaths.mergeRequestSummary, {
                    mr,
                  }),
                  search: mrParams.toString(),
                }}
              >
                <code>{mr}</code>
              </Link>
            ) : mr ? (
              <code>{mr}</code>
            ) : (
              'Нет связи'
            )}
          </dd>
        </div>
        <div>
          <dt>Начало</dt>
          <dd>
            {pipeline.started_at ? (
              <ActivityTime instant={pipeline.started_at} />
            ) : (
              'Неизвестно'
            )}
          </dd>
        </div>
        <div>
          <dt>Завершение</dt>
          <dd>
            {pipeline.finished_at ? (
              <ActivityTime instant={pipeline.finished_at} />
            ) : (
              'Неизвестно'
            )}
          </dd>
        </div>
      </dl>
      <SourceLink url={pipeline.source_url}>
        {pipeline.source_url ? 'Открыть CI в GitLab' : null}
      </SourceLink>
    </Card>
  )
}

export function AttemptTotals({
  pipeline,
  usage,
  retryUsage,
}: {
  pipeline: Pipeline
  usage: Usage | undefined
  retryUsage?: string
}) {
  const { attempts_total, retried_jobs, allowed_failures, attempt_history } =
    pipeline
  const total =
    usage?.state === 'BOUNDED' && usage.bound?.kind === 'lower'
      ? secondsText(usage.bound.value, true)
      : usage?.value != null &&
          (usage.state === 'ASSESSED' || usage.state === 'PROVISIONAL')
        ? secondsText(
            usage.value,
            attempt_history !== 'complete' || usage.state === 'PROVISIONAL',
          )
        : 'Неизвестно'
  return (
    <dl className="cp-ci-facts">
      <div>
        <dt>Известные попытки</dt>
        <dd>{attempts_total}</dd>
      </div>
      <div>
        <dt>Задания с повтором</dt>
        <dd>{retried_jobs}</dd>
      </div>
      <div>
        <dt>Разрешённые падения</dt>
        <dd>{allowed_failures}</dd>
      </div>
      <div>
        <dt>История попыток</dt>
        <dd>
          <AttemptHistory state={attempt_history} />
        </dd>
      </div>
      <div>
        <dt>Общий расход CI</dt>
        <dd>{total}</dd>
      </div>
      <div>
        <dt>Время повторов</dt>
        <dd>{retryUsage ?? 'Неизвестно'}</dd>
      </div>
    </dl>
  )
}

export function PipelineReports({ pipeline }: { pipeline: Pipeline }) {
  return (
    <Card className="cp-ci-panel">
      <Text as="h2" variant="subheader-2">
        Отчёты проверок
      </Text>
      {pipeline.reports.length === 0 ? (
        <p>Нет сведений об отчётах.</p>
      ) : (
        <dl className="cp-ci-facts">
          {pipeline.reports.map(({ analysis_kind, state }) => (
            <div key={analysis_kind}>
              <dt>
                {contractLabel(technicalLabels.analysisKind, analysis_kind)}
              </dt>
              <dd>
                <CheckReportState state={state} />
                {state === 'malformed' && ' · повреждён'}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </Card>
  )
}

function CoverageValue({
  value,
}: {
  value: NonNullable<Pipeline['coverage']>['line'] | undefined
}) {
  if (!value || value.state === 'NOT_EVALUATED') return 'Не измерено'
  if (value.state === 'NOT_APPLICABLE')
    return <DataStateBadge state="NOT_APPLICABLE" />
  if (value.state === 'UNKNOWN') return <DataStateBadge state="UNKNOWN" />
  return <MetricValue value={value} />
}

export function PipelineCoverage({
  coverage,
}: {
  coverage: Pipeline['coverage']
}) {
  return (
    <Card className="cp-ci-panel">
      <Text as="h2" variant="subheader-2">
        Покрытие
      </Text>
      <dl className="cp-ci-facts">
        <div>
          <dt>Покрытие строк</dt>
          <dd>
            <CoverageValue value={coverage?.line} />
          </dd>
        </div>
        <div>
          <dt>Покрытие ветвей</dt>
          <dd>
            <CoverageValue value={coverage?.branch} />
          </dd>
        </div>
      </dl>
    </Card>
  )
}
