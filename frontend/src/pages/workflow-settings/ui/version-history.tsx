import { Button } from '@gravity-ui/uikit'
import { ErrorState, StatusTime } from '@/shared/ui'

export function VersionHistory({
  title,
  versions,
  loadOlder,
  loading,
  failed,
}: {
  title: string
  versions: readonly {
    version_no: number
    confirmed_by: string
    confirmed_at: string
  }[]
  loadOlder?: () => void
  loading: boolean
  failed: boolean
}) {
  return (
    <details>
      <summary>{title}</summary>
      {!versions.length && <p>Подтверждённых версий пока нет.</p>}
      <ul>
        {versions.map((version, index) => (
          <li key={version.version_no}>
            Версия {version.version_no}
            {index === 0 && ' (действующая)'} · Автор {version.confirmed_by} ·{' '}
            <StatusTime value={version.confirmed_at} />
          </li>
        ))}
      </ul>
      {loadOlder &&
        (failed ? (
          <ErrorState
            reason="Не удалось загрузить предыдущие версии"
            onRetry={loadOlder}
            retrying={loading}
          />
        ) : (
          <Button
            type="button"
            view="outlined"
            loading={loading}
            onClick={loadOlder}
          >
            Показать предыдущие версии
          </Button>
        ))}
    </details>
  )
}
