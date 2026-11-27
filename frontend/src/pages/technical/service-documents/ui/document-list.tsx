import { Button } from '@gravity-ui/uikit'
import { ActivityTime, SourceLink } from '@/entities/activity'
import type { ServiceDocument } from '@/shared/api'

const formats: Readonly<Record<string, string>> = {
  markdown: 'Markdown',
  openapi: 'OpenAPI',
  confluence: 'Confluence',
}

export function DocumentList({
  documents,
  hasMore,
  loading,
  loadMore,
}: {
  documents: ServiceDocument[]
  hasMore: boolean
  loading: boolean
  loadMore: () => void
}) {
  return (
    <section aria-labelledby="service-document-list">
      <h2 id="service-document-list">Документы текущей версии</h2>
      {documents.length > 0 ? (
        <ul className="cp-service-docs__cards">
          {documents.map((document) => (
            <li key={document.id}>
              <h3>{document.title ?? document.path ?? 'Документ'}</h3>
              <dl>
                <dt>Формат</dt>
                <dd>{formats[document.doc_format] ?? 'Неизвестно'}</dd>
                <dt>Путь</dt>
                <dd>{document.path ?? 'Неизвестно'}</dd>
                <dt>Версия</dt>
                <dd>
                  <code>{document.version_ref}</code>
                </dd>
                <dt>Последняя проверка</dt>
                <dd>
                  {document.last_checked_at ? (
                    <ActivityTime instant={document.last_checked_at} />
                  ) : (
                    'Неизвестно'
                  )}
                </dd>
                <dt>Открытые нарушения</dt>
                <dd>{document.open_violations ?? 'Неизвестно'}</dd>
              </dl>
              {document.source_url && (
                <SourceLink url={document.source_url}>
                  {document.doc_format === 'confluence'
                    ? 'Открыть источник'
                    : 'Открыть документ в GitLab'}
                </SourceLink>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p>Нет доступных документов.</p>
      )}
      {hasMore && (
        <Button size="l" disabled={loading} onClick={loadMore}>
          Ещё документы
        </Button>
      )}
    </section>
  )
}
