import { Text } from '@gravity-ui/uikit'
import { useLocation, useOutletContext } from 'react-router'
import type { ApiClient } from '@/shared/api'
import type { CurrentUser } from '@/shared/auth'
import { adminLinks, readTechnicalId, routePaths } from '@/shared/config'
import { EmptyState, ErrorState, LoadingState } from '@/shared/ui'
import { useImportProjects } from '../api/import-projects'
import { initialDraft } from '../model/import-draft'
import { ImportEditor } from './import-editor'
import './import-package.css'

export function ImportPackagePage({ api }: { api: ApiClient }) {
  const current = useOutletContext<CurrentUser>()
  const location = useLocation()
  const { projects, loading, error, fetching, retry } = useImportProjects(
    api,
    current,
  )
  const origin: unknown = location.state
  const receiptId =
    origin &&
    typeof origin === 'object' &&
    'returnReceiptId' in origin &&
    typeof origin.returnReceiptId === 'string'
      ? readTechnicalId(origin.returnReceiptId)
      : undefined
  const cancelTo = receiptId
    ? adminLinks.receipt(receiptId)
    : current.actions.includes('connections.manage')
      ? routePaths.connections
      : routePaths.home

  return (
    <section className="cp-import">
      <Text as="h1" variant="header-1" id="import-heading">
        Импорт
      </Text>
      {loading ? (
        <LoadingState />
      ) : error && !projects.length ? (
        <ErrorState
          reason="Не удалось загрузить проекты"
          retrying={fetching}
          onRetry={retry}
        />
      ) : projects.length ? (
        <>
          {error && (
            <ErrorState
              reason="Не удалось загрузить часть проектов"
              retrying={fetching}
              onRetry={retry}
            />
          )}
          <ImportEditor
            key={location.key}
            api={api}
            projects={projects}
            initial={initialDraft(new URLSearchParams(location.search))}
            cancelTo={cancelTo}
            journalAllowed={current.actions.includes('sync.read')}
          />
        </>
      ) : (
        <EmptyState reason="filters" title="Нет проектов для импорта" />
      )}
    </section>
  )
}
