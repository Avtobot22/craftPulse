import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'
import { savedViewPath } from '../model/saved-view-links'

export function CopySavedView({ id, name }: { id: string; name: string }) {
  const [status, setStatus] = useState<'copied' | 'failed'>()
  const url = new URL(savedViewPath(id), window.location.origin).href

  async function copy() {
    try {
      await navigator.clipboard.writeText(url)
      setStatus('copied')
    } catch {
      setStatus('failed')
    }
  }

  return (
    <>
      <Button
        size="xl"
        aria-label={`Скопировать ссылку на срез «${name}»`}
        onClick={() => void copy()}
      >
        Скопировать ссылку
      </Button>
      {status === 'copied' && <span role="status">Ссылка скопирована</span>}
      {status === 'failed' && (
        <label>
          <span role="status">Скопируйте ссылку вручную</span>
          <input
            className="cp-team-report__saved-link"
            readOnly
            value={url}
            onFocus={(event) => {
              event.currentTarget.select()
            }}
          />
        </label>
      )}
    </>
  )
}
