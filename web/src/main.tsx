import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './fonts.css'
import './index.css'
import App from './App'

const CHUNK_RELOAD_MARKER = 'ampmanager:chunk-reload-attempted'

if (typeof window !== 'undefined') {
  window.addEventListener('vite:preloadError', (event) => {
    if (window.sessionStorage.getItem(CHUNK_RELOAD_MARKER) === '1') {
      console.error('vite preload failed after reload attempt', event)
      return
    }

    event.preventDefault()
    window.sessionStorage.setItem(CHUNK_RELOAD_MARKER, '1')
    window.location.reload()
  })

  window.addEventListener(
    'load',
    () => {
      window.sessionStorage.removeItem(CHUNK_RELOAD_MARKER)
    },
    { once: true },
  )
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
