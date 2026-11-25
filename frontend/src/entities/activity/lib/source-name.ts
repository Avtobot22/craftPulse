export function activitySourceName(kind: string): string | undefined {
  switch (kind) {
    case 'task':
      return 'Jira DC'
    case 'merge_request':
      return 'GitLab'
    case 'pipeline':
    case 'attempt':
    case 'finding':
      return 'GitLab CI'
    default:
      return undefined
  }
}
