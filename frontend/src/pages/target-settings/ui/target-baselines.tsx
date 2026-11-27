import { Text } from '@gravity-ui/uikit'
import { Link } from 'react-router'
import { MetricValue } from '@/entities/metric'
import type { BaselineGroup, TargetGroup } from '../api/target-api'

export function TargetBaselines({
  rows,
  selected,
  groupLink,
}: {
  rows: BaselineGroup[]
  selected: TargetGroup | null
  groupLink: (group: TargetGroup) => string
}) {
  const sized = rows.filter((row) => row.size_label)
  const unsized = rows.filter((row) => !row.size_label)
  return (
    <>
      {sized.length > 0 && (
        <div
          className="cp-targets__table"
          role="region"
          aria-label="Ориентиры по группам"
          tabIndex={0}
        >
          <table>
            <thead>
              <tr>
                <th scope="col">Тип</th>
                <th scope="col">Размер</th>
                <th scope="col">Исторический p85</th>
                <th scope="col">Действие</th>
              </tr>
            </thead>
            <tbody>
              {sized.map((row) => (
                <tr key={JSON.stringify([row.task_type, row.size_label])}>
                  <th scope="row">{row.task_type}</th>
                  <td>{row.size_label}</td>
                  <td>
                    <MetricValue value={row.p85} showCompleteness />
                  </td>
                  <td>
                    {row.task_type === selected?.task_type &&
                    row.size_label === selected.size_label ? (
                      <Text>Группа выбрана</Text>
                    ) : (
                      row.size_label && (
                        <Link
                          to={groupLink({
                            task_type: row.task_type,
                            size_label: row.size_label,
                          })}
                          aria-label={`Задать срок: ${row.task_type}, ${row.size_label}`}
                        >
                          Задать срок
                        </Link>
                      )
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {unsized.length > 0 && (
        <section>
          <Text as="h3" variant="subheader-2">
            Без размера
          </Text>
          <p>Для задач без размера целевой срок не применяется.</p>
          <ul className="cp-targets__unsized">
            {unsized.map((row) => (
              <li key={row.task_type}>
                <strong>{row.task_type}</strong>
                <MetricValue value={row.p85} showCompleteness />
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
