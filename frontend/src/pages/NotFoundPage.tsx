import { A } from '@solidjs/router'
import gandalf404Url from '../assets/404-gandalf.jpg'

export function NotFoundPage() {
  return (
    <div class="not-found-container" data-testid="not-found-page">
      <div class="not-found-card">
        <img
          src={gandalf404Url}
          alt="404 - Way Not Found (Gandalf looking lost with a map)"
          class="not-found-image"
          data-testid="not-found-image"
        />
        <h1 class="not-found-code">404</h1>
        <h2 class="not-found-title">Page Not Found</h2>
        <p class="not-found-desc">
          "I have no memory of this place." — You seem to have wandered off the marked trail. The route you are looking for does not exist or has been moved.
        </p>
        <div class="not-found-actions">
          <A href="/" class="btn-primary" data-testid="btn-return-dashboard">
            Return to Dashboard
          </A>
        </div>
      </div>
    </div>
  )
}
