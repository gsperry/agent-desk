import { Component, type ErrorInfo, type ReactNode } from 'react'

interface State {
  error: Error | null
}

// ErrorBoundary catches an error thrown while the page draws, so a bug in
// one view shows what went wrong and a way back, not a blank window.
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Agent Desk could not draw the page', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    // The stack starts with the message in most browsers; Safari's does not.
    const stack = error.stack ?? ''
    const detail = stack.includes(error.message) ? stack : `${error.name}: ${error.message}\n${stack}`.trim()
    return (
      <main className="crash" role="alert">
        <h1>Something went wrong</h1>
        <p>Agent Desk hit an error drawing this page. Your sessions keep running and nothing was sent. Reload to carry on, and please report the details below.</p>
        <div className="crash-actions">
          <button
            type="button"
            className="send"
            onClick={() => {
              window.location.reload()
            }}
          >
            Reload
          </button>
          <button
            type="button"
            className="secondary"
            onClick={() => {
              this.setState({ error: null })
            }}
          >
            Try again
          </button>
        </div>
        <pre className="crash-detail">{detail}</pre>
      </main>
    )
  }
}
