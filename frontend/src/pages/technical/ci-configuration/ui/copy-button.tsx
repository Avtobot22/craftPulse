import { Button } from '@gravity-ui/uikit'
import { useState } from 'react'

export function CopyButton({ value, label }: { value: string; label: string }) {
  const [status, setStatus] = useState('')

  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setStatus('Скопировано')
    } catch {
      setStatus(
        'Копирование недоступно. Выделите текст и скопируйте его вручную.',
      )
    }
  }

  return (
    <div>
      <Button onClick={() => void copy()}>{label}</Button>
      <span role="status"> {status}</span>
    </div>
  )
}
