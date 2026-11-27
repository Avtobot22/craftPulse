import { Label } from '@gravity-ui/uikit'
import type { ComponentProps, ReactNode } from 'react'
import { generatePath } from 'react-router'
import { CommitSha } from '@/entities/check-report'
import type { components, OpenCodes } from '@/shared/api'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink, StatusTime } from '@/shared/ui'

type Baseline = OpenCodes<components['schemas']['BaselineList']['data'][number]>

const reasons: Record<string, string> = {
  full_run_missing: 'Нет полного прогона основной ветки',
  scan_incomplete: 'Проверка выполнена частично',
  scan_failed: 'Проверка завершилась ошибкой',
}

const themes: Record<string, ComponentProps<typeof Label>['theme']> = {
  ready: 'success',
  pending: 'warning',
  failed: 'danger',
}

function BaselineStatus({ state, reason_code }: Baseline) {
  return (
    <>
      <Label
        theme={
          (Object.hasOwn(themes, state) ? themes[state] : undefined) ??
          'unknown'
        }
      >
        {contractLabel(technicalLabels.baselineState, state)}
      </Label>
      {reason_code ? (
        <p>{contractLabel(reasons, reason_code)}</p>
      ) : (
        state !== 'ready' && <p>Причина не указана</p>
      )}
    </>
  )
}

function rowKey({ component_id, language, analysis_kind }: Baseline) {
  return [component_id, language, analysis_kind]
    .map(encodeURIComponent)
    .join('/')
}

function MatrixCell({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <td role="cell">
      <span aria-hidden="true" className="cp-baselines__mobile-label">
        {label}
      </span>
      {children}
    </td>
  )
}

export function BaselineMatrix({
  rows,
  projectId,
  context,
  canReadFindings,
}: {
  rows: Baseline[]
  projectId: string
  context: URLSearchParams
  canReadFindings: boolean
}) {
  if (!rows.length)
    return <p role="status">В проекте нет настроенных строк базовой линии.</p>
  return (
    <table
      className="cp-baselines__matrix"
      role="table"
      aria-label="Матрица базовых линий"
    >
      <thead role="rowgroup">
        <tr role="row">
          {[
            'Компонент',
            'Язык',
            'Вид анализа',
            'Состояние',
            'Ветка',
            'SHA',
            'Время установления',
            'Отчёт',
          ].map((label) => (
            <th key={label} scope="col" role="columnheader">
              {label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody role="rowgroup">
        {rows.map((row) => {
          const key = rowKey(row)
          const component = <code>{row.component_id}</code>
          return (
            <tr key={key} role="row">
              <th scope="row" role="rowheader">
                <span aria-hidden="true" className="cp-baselines__mobile-label">
                  Компонент
                </span>
                {canReadFindings ? (
                  <InspectionLink
                    id={`baseline-component-${key}`}
                    to={{
                      pathname: generatePath(routePaths.componentOverview, {
                        componentId: encodeURIComponent(row.component_id),
                      }),
                      search: context.toString(),
                    }}
                  >
                    {component}
                  </InspectionLink>
                ) : (
                  component
                )}
              </th>
              <MatrixCell label="Язык">{row.language || 'Любой'}</MatrixCell>
              <MatrixCell label="Вид анализа">
                {contractLabel(technicalLabels.analysisKind, row.analysis_kind)}
              </MatrixCell>
              <MatrixCell label="Состояние">
                <BaselineStatus {...row} />
              </MatrixCell>
              <MatrixCell label="Ветка">
                {row.branch ? <code>{row.branch}</code> : 'Не определена'}
              </MatrixCell>
              <MatrixCell label="SHA">
                {row.sha ? <CommitSha sha={row.sha} /> : 'Нет данных'}
              </MatrixCell>
              <MatrixCell label="Время установления">
                <StatusTime value={row.established_at} />
              </MatrixCell>
              <MatrixCell label="Отчёт">
                {row.report_id ? (
                  <InspectionLink
                    id={`baseline-report-${key}`}
                    to={{
                      pathname: generatePath(routePaths.baselineReport, {
                        projectId,
                        reportId: encodeURIComponent(row.report_id),
                      }),
                      search: context.toString(),
                    }}
                  >
                    Открыть отчёт{' '}
                    <span className="cp-baselines__sr-only">
                      {row.report_id}
                    </span>
                  </InspectionLink>
                ) : (
                  'Нет отчёта'
                )}
              </MatrixCell>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

export function BaselineReport({
  reportId,
  rows,
}: {
  reportId: string
  rows: Baseline[]
}) {
  return (
    <section aria-label="Сведения об отчёте">
      <p>
        ID отчёта <code>{reportId}</code>
      </p>
      {rows.map((row) => (
        <dl className="cp-baselines__report" key={rowKey(row)}>
          <dt>Компонент</dt>
          <dd>
            <code>{row.component_id}</code>
          </dd>
          <dt>Язык</dt>
          <dd>{row.language || 'Любой'}</dd>
          <dt>Вид анализа</dt>
          <dd>
            {contractLabel(technicalLabels.analysisKind, row.analysis_kind)}
          </dd>
          <dt>Состояние</dt>
          <dd>
            <BaselineStatus {...row} />
          </dd>
          <dt>Ветка</dt>
          <dd>{row.branch ? <code>{row.branch}</code> : 'Не определена'}</dd>
          <dt>SHA</dt>
          <dd>{row.sha ? <CommitSha sha={row.sha} /> : 'Нет данных'}</dd>
          <dt>Время установления</dt>
          <dd>
            <StatusTime value={row.established_at} />
          </dd>
        </dl>
      ))}
    </section>
  )
}
