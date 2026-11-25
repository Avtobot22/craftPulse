import type { ReportBlockProps } from '../model/metrics'
import { FlowChart } from './metric-block'

export function FlowBalanceBlock({ block }: { block: ReportBlockProps }) {
  return (
    <section
      className="cp-team-report__block"
      aria-labelledby="team-report-balance"
    >
      <h2 id="team-report-balance">Баланс потока и незавершённая работа</h2>
      <div className="cp-team-report__charts">
        <FlowChart
          block={block}
          chartId="balance"
          title="Баланс потока"
          summary="Начатые и впервые завершённые задачи на одной шкале. Баланс показан по T18."
          metricIds={['T18', 'T04', 'T05']}
        />
        <FlowChart
          block={block}
          chartId="wip"
          title="Задачи в работе и блокировки"
          summary="Заблокированные задачи входят в общее число задач в работе (WIP)."
          metricIds={['T12', 'T13']}
        />
      </div>
    </section>
  )
}
