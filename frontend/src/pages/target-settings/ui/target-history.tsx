import { Button, Text } from '@gravity-ui/uikit'
import { StatusTime } from '@/shared/ui'
import type { TargetVersion } from '../api/target-api'
import { basisLabel, formatDuration } from '../model/target-settings'

export function TargetHistory({
  versions,
  loadMore,
  loading,
}: {
  versions: TargetVersion[]
  loadMore?: () => void
  loading: boolean
}) {
  return (
    <section aria-labelledby="target-history-heading">
      <Text as="h3" variant="subheader-2" id="target-history-heading">
        История версий
      </Text>
      {versions.length === 0 ? (
        <p>У группы пока нет подтверждённых сроков.</p>
      ) : (
        <div
          className="cp-targets__table"
          role="region"
          aria-label="История версий срока"
          tabIndex={0}
        >
          <table>
            <thead>
              <tr>
                <th scope="col">Версия</th>
                <th scope="col">Срок</th>
                <th scope="col">Основание</th>
                <th scope="col">n основания</th>
                <th scope="col">Автор</th>
                <th scope="col">Подтверждено</th>
              </tr>
            </thead>
            <tbody>
              {versions.map((version, index) => (
                <tr key={version.version_no}>
                  <th scope="row">
                    v{version.version_no}
                    {index === 0 && ' (действует)'}
                  </th>
                  <td>{formatDuration(version.duration_seconds)}</td>
                  <td>{basisLabel(version.basis)}</td>
                  <td>{version.basis_n ?? 'Нет данных'}</td>
                  <td>{version.confirmed_by}</td>
                  <td>
                    <StatusTime value={version.confirmed_at} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {loadMore && (
        <Button size="xl" onClick={loadMore} disabled={loading}>
          {loading ? 'Загрузка версий' : 'Показать предыдущие версии'}
        </Button>
      )}
    </section>
  )
}
