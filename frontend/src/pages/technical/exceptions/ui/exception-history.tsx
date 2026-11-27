import { Text } from '@gravity-ui/uikit'
import { useRef } from 'react'
import { generatePath, useOutletContext, useSearchParams } from 'react-router'
import {
  describeExceptionScope,
  ExceptionPeriod,
  exceptionStatus,
  formatExceptionDate,
} from '@/entities/exception'
import { isStrongETag } from '@/shared/api'
import type {
  ApiClient,
  ApiResponse,
  ExceptionDetail,
  ExceptionVersion,
} from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { contractLabel, routePaths, technicalLabels } from '@/shared/config'
import { InspectionLink } from '@/shared/ui'
import { ExceptionReview } from './exception-review'

export function ExceptionHistory({
  api,
  resource,
  refreshing,
  asOf,
}: {
  api: ApiClient
  resource: ApiResponse<ExceptionDetail>
  refreshing: boolean
  asOf: number
}) {
  const currentUser = useOutletContext<CurrentUser>()
  const [search] = useSearchParams()
  const statusHeading = useRef<HTMLHeadingElement>(null)
  const { data, etag } = resource
  const current = data.current_version
  const versions = [current, ...data.versions].sort(
    (a, b) => b.version_no - a.version_no,
  )
  const active =
    data.state === 'active'
      ? versions.find(
          (version) =>
            version.status === 'approved' &&
            Date.parse(version.effective_from) <= asOf &&
            Date.parse(version.expires_at) > asOf,
        )
      : undefined
  const expired =
    data.state === 'expired' ||
    (data.state === 'active' &&
      !active &&
      Date.parse(current.expires_at) <= asOf)
  const state = expired
    ? 'expired'
    : data.state === 'active' && !active
      ? current.status === 'rejected' || current.status === 'withdrawn'
        ? current.status
        : 'pending'
      : data.state
  const isAuthor = current.author_user_id === currentUser.user.id
  const local =
    !current.external_authority && current.decision_type !== 'bypass'
  const manageable = local && currentUser.actions.includes('exceptions.manage')
  const actionable = local && !expired && Date.parse(current.expires_at) > asOf
  const pending = current.status === 'pending'
  const mayReview =
    actionable && pending && currentUser.actions.includes('exceptions.review')
  const mayWithdraw =
    actionable &&
    manageable &&
    isAuthor &&
    (pending || current.status === 'approved')

  return (
    <>
      <section>
        <Text as="h2" variant="header-2" tabIndex={-1} ref={statusHeading}>
          {state === 'pending' && current.status === 'approved'
            ? 'Начало действия ещё не наступило'
            : exceptionStatus(state)}
        </Text>
        {current.external_authority && (
          <p>Источник решения: {current.external_authority}</p>
        )}
        {active && (
          <VersionCard
            title={`Действующая версия ${String(active.version_no)}`}
            version={active}
          />
        )}
        {pending && !expired && (
          <VersionCard
            title={`Ожидающая версия ${String(current.version_no)}`}
            version={current}
          />
        )}
        {(expired || !pending) && !active && (
          <VersionCard
            title={`Текущая версия ${String(current.version_no)}`}
            version={current}
            expired={expired}
          />
        )}
        {manageable && (
          <InspectionLink
            id="exception-extend"
            to={{
              pathname: generatePath(routePaths.exceptionExtend, {
                exceptionId: data.id,
              }),
              search: search.toString(),
            }}
          >
            Продлить
          </InspectionLink>
        )}
        <ExceptionReview
          api={api}
          exception={data}
          etag={isStrongETag(etag) ? etag : null}
          refreshing={refreshing}
          approve={mayReview && !(isAuthor && current.approval_required)}
          reject={mayReview}
          withdraw={mayWithdraw}
          onResult={() => {
            statusHeading.current?.focus()
          }}
        />
      </section>
      <section>
        <Text as="h2" variant="header-2">
          Версии
        </Text>
        <ol className="cp-exceptions__list">
          {versions.map((version) => (
            <li key={version.version_no}>
              <details>
                <summary>
                  Версия {version.version_no}
                  {version.version_no === current.version_no
                    ? ' · последняя'
                    : ''}
                  {' · '}
                  {contractLabel(
                    technicalLabels.exceptionVersionStatus,
                    version.status,
                  )}
                  {version.status === 'approved' &&
                    Date.parse(version.expires_at) <= asOf && (
                      <>
                        {' '}
                        · Срок истёк{' '}
                        <time dateTime={version.expires_at}>
                          {formatExceptionDate(version.expires_at)}
                        </time>
                      </>
                    )}
                </summary>
                <VersionCard
                  version={version}
                  expired={Date.parse(version.expires_at) <= asOf}
                />
              </details>
            </li>
          ))}
        </ol>
      </section>
      <section>
        <Text as="h2" variant="header-2">
          Переходы
        </Text>
        {data.transitions.length ? (
          <ol className="cp-exceptions__list">
            {[...data.transitions]
              .sort(
                (a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at),
              )
              .map((event, index) => (
                <li key={index}>
                  Версия {event.version_no} ·{' '}
                  {contractLabel(
                    technicalLabels.exceptionVersionStatus,
                    event.status,
                  )}
                  {' · '}
                  {event.actor_user_id}
                  {' · '}
                  <time dateTime={event.occurred_at}>
                    {formatExceptionDate(event.occurred_at)}
                  </time>
                </li>
              ))}
          </ol>
        ) : (
          <p>Переходов пока нет.</p>
        )}
      </section>
    </>
  )
}

function VersionCard({
  version,
  title,
  expired,
}: {
  version: ExceptionVersion
  title?: string
  expired?: boolean
}) {
  return (
    <div className="cp-exceptions__version">
      {title && (
        <Text as="h3" variant="subheader-2">
          {title}
        </Text>
      )}
      <p>
        {contractLabel(
          technicalLabels.exceptionDecisionType,
          version.decision_type,
        )}{' '}
        · {version.reason}
      </p>
      <p>{describeExceptionScope(version.scope)}</p>
      <p>Автор: {version.author_user_id}</p>
      <ExceptionPeriod
        start={version.effective_from}
        end={version.expires_at}
        expired={expired ?? false}
      />
    </div>
  )
}
