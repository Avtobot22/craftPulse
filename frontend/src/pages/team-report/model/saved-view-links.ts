import { generatePath } from 'react-router'
import { routePaths } from '@/shared/config'

export function savedViewPath(id: string) {
  return generatePath(routePaths.savedView, {
    savedViewId: encodeURIComponent(id),
  })
}
