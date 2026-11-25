import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import { lateTimelineEvent } from '../../../../test/fixtures'
import { ActorKindBadge } from './actor-kind-badge'
import { EventRow } from './event-row'
import { HistoryBoundary } from './history-boundary'
import { SourceLink } from './source-link'

it.each([
  ['human', 'Человек'],
  ['bot', 'Бот'],
  ['system', 'Система'],
  ['unknown', 'Вид участника неизвестен'],
])('displays the %s actor kind with its own text', (kind, label) => {
  render(<ActorKindBadge kind={kind} />)
  expect(screen.getByText(label)).toBeVisible()
})

it('shows the event and ingestion clocks with both provenance badges', () => {
  render(<EventRow event={lateTimelineEvent} sourceName="Jira Cloud" />)
  expect(screen.getByText('CORE-15 · Перенести отчёт')).toBeVisible()
  expect(screen.getByText('Изменение поля')).toBeVisible()
  expect(screen.getByText('status: Open → In Progress')).toBeVisible()
  expect(screen.getByText('Анна Смирнова')).toBeVisible()
  expect(screen.getByText(/02\.10\.2026.*09:00:00 UTC$/)).toBeVisible()
  expect(screen.getByText(/02\.10\.2026.*12:00:00 UTC\+03:00$/)).toBeVisible()
  expect(screen.getByText(/06\.10\.2026.*00:00:00 UTC$/)).toBeVisible()
  expect(screen.getByText('Загружено с историей')).toBeVisible()
  expect(screen.getByText('Получено с опозданием')).toBeVisible()
  expect(
    screen.getByRole('link', { name: 'Открыть в Jira Cloud' }),
  ).toHaveAttribute('href', lateTimelineEvent.object.source_url)
})

it('shows an event value when the source omits the field name', () => {
  render(
    <EventRow
      event={{ ...lateTimelineEvent, field: null, before_value: null }}
    />,
  )
  expect(screen.getByText('In Progress')).toBeVisible()
})

it('applies a negative source offset across the date boundary', () => {
  render(
    <EventRow event={{ ...lateTimelineEvent, event_offset_seconds: -34200 }} />,
  )
  expect(screen.getByText(/01\.10\.2026.*23:30:00 UTC-09:30$/)).toBeVisible()
})

it('names the source whose history starts at a known date', () => {
  render(
    <HistoryBoundary historyFrom="2026-01-01T00:00:00Z" sourceName="Jira DC" />,
  )
  expect(screen.getByText(/Jira DC: раньше этой даты данных нет/)).toBeVisible()
  expect(screen.getByText(/01\.01\.2026.*UTC$/)).toBeVisible()
})

it.each([
  null,
  'javascript:alert(1)',
  'https://user:token@gitlab.example/project',
])('leaves an unavailable or unsafe source URL without a link', (url) => {
  render(<SourceLink url={url} sourceName="GitLab" />)
  expect(screen.queryByRole('link')).not.toBeInTheDocument()
})
