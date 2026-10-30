import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import './styles/base.css'
import './styles/draw.css'
import './styles/app.css'
import './styles/settings.css'
import { AppProvider, useApp } from './lib/app-context'
import { Loading, ToastProvider } from './components/ui'
import Layout from './components/Layout'
import { ErrorBoundary } from './components/ErrorBoundary'
import Login from './pages/Login'
import Drawings from './pages/Drawings'
import Editor from './pages/Editor'
import Settings from './pages/Settings'

// Excalidraw loads its fonts from here (copied into the build), never from a CDN.
;(window as unknown as { EXCALIDRAW_ASSET_PATH: string }).EXCALIDRAW_ASSET_PATH = '/excalidraw/'

function Gate({ children }: { children: React.ReactNode }) {
  const { status, user } = useApp()
  const loc = useLocation()
  if (!status) return <div className="boot"><img src="/favicon.svg" alt="" /><Loading /></div>
  if (status.needs_setup) return <Navigate to="/setup" replace />
  if (!user) return <Navigate to="/login" replace state={{ from: loc.pathname + loc.search }} />
  return <>{children}</>
}

function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login mode="login" />} />
      <Route path="/setup" element={<Login mode="setup" />} />
      <Route path="/settings/:tab?" element={<Gate><ErrorBoundary where="settings"><Settings /></ErrorBoundary></Gate>} />
      <Route path="/d/:id" element={<Gate><ErrorBoundary where="editor"><Editor /></ErrorBoundary></Gate>} />
      <Route element={<Gate><ErrorBoundary where="draw"><Layout /></ErrorBoundary></Gate>}>
        <Route path="/" element={<Drawings key="all" view="all" />} />
        <Route path="/recent" element={<Drawings key="recent" view="recent" />} />
        <Route path="/starred" element={<Drawings key="starred" view="starred" />} />
        <Route path="/shared" element={<Drawings key="shared" view="shared" />} />
        <Route path="/trash" element={<Drawings key="trash" view="trash" />} />
        <Route path="/search" element={<Drawings key="search" view="search" />} />
        <Route path="/kind/:kind" element={<Drawings key="kind" view="kind" />} />
        <Route path="/folder/:id" element={<Drawings key="folder" view="folder" />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary where="app"><ToastProvider><AppProvider><App /></AppProvider></ToastProvider></ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>,
)
