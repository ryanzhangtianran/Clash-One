import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { System } from '@wailsio/runtime'

// Linux windows have the system's title bar and no blur behind them (see .linux in style.css)
if (System.IsLinux()) document.documentElement.classList.add('linux')

// a failed Go call is already listed in the events (MarshalError in app.go): no page catches those
window.addEventListener('unhandledrejection', (e) => e.reason?.name === 'RuntimeError' && e.preventDefault())

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
