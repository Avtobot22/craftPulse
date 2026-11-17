import type { ApiProblem } from '@/shared/api'

const describeProblem = (problem: ApiProblem): string => problem.title

export function HomePage({ problem }: { problem?: ApiProblem }) {
  return (
    <main>
      <h1>craftPulse</h1>
      <p>Платформа аналитики разработки</p>
      {problem && <p role="alert">{describeProblem(problem)}</p>}
    </main>
  )
}
