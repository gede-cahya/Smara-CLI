import React, { useState, useEffect } from 'react'

const SimpleApp: React.FC = () => {
  const [message, setMessage] = useState('React Loaded!')
  const [workspaces, setWorkspaces] = useState<string[]>([])
  
  useEffect(() => {
    console.log('[Smara] SimpleApp mounted')
    // Test if Wails bindings exist
    if (typeof window !== 'undefined' && (window as any).go) {
      setMessage('Wails bindings found!')
      // Try to load workspaces
      try {
        const GetWorkspaces = (window as any).go.main.App.GetWorkspaces
        if (GetWorkspaces) {
          GetWorkspaces().then((ws: any) => {
            setWorkspaces(ws?.map((w: any) => w.name) || [])
            setMessage('Loaded ' + ws?.length + ' workspaces')
          }).catch((e: any) => {
            setMessage('Error: ' + e)
          })
        }
      } catch(e) {
        setMessage('Error calling Go: ' + e)
      }
    } else {
      setMessage('Wails bindings NOT found. window.go = ' + typeof (window as any).go)
    }
  }, [])

  return React.createElement('div', {
    style: {
      color: '#e2e2e2',
      padding: '40px',
      fontFamily: 'Inter, sans-serif',
      background: '#131313',
      height: '100vh',
      overflow: 'auto'
    }
  }, [
    React.createElement('h1', { key: 'title', style: { color: '#818cf8' } }, 'Smara Desktop'),
    React.createElement('p', { key: 'msg' }, message),
    React.createElement('div', { key: 'ws', style: { marginTop: '20px' } },
      workspaces.length > 0 
        ? React.createElement('div', null, 
            React.createElement('h3', null, 'Workspaces:'),
            ...workspaces.map((w, i) => 
              React.createElement('div', { key: i }, w)
            )
          )
        : null
    ),
    React.createElement('button', {
      key: 'test',
      onClick: () => setMessage('Clicked at ' + new Date().toLocaleTimeString()),
      style: {
        marginTop: '20px',
        padding: '10px 20px',
        background: '#818cf8',
        border: 'none',
        borderRadius: '8px',
        color: 'white',
        cursor: 'pointer'
      }
    }, 'Test Button')
  ])
}

export default SimpleApp
