import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'

// a failed Go call is already listed in the events (MarshalError in app.go): no page catches those
window.addEventListener('unhandledrejection', (e) => e.reason?.name === 'RuntimeError' && e.preventDefault())

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
