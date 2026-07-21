// Client-side instrumentation (Next.js 15.3+ convention). This file runs in the
// browser after the HTML document loads but *before* React hydration, so
// analytics is initialized before the first user interaction. See
// node_modules/next/dist/docs/01-app/03-api-reference/03-file-conventions/instrumentation-client.md
import posthog from "posthog-js";

const key = process.env.NEXT_PUBLIC_POSTHOG_KEY;

// Only initialize when a project key is configured — keeps local dev quiet and
// avoids errors for anyone running the docs site without a PostHog project.
if (key) {
  posthog.init(key, {
    // Events go through our own /ingest path (rewritten to PostHog in
    // next.config.ts) so ad blockers that block *.posthog.com don't silently
    // drop analytics. ui_host keeps "view in PostHog" deep-links pointing at
    // the real dashboard.
    api_host: "/ingest",
    ui_host: process.env.NEXT_PUBLIC_POSTHOG_HOST ?? "https://us.posthog.com",
    // The dated defaults bundle enables the modern, recommended behavior:
    // pageviews on client-side route changes (capture_pageview: 'history_change'),
    // pageleave events, and dead-click autocapture — so App Router SPA
    // navigations are tracked automatically with no manual usePathname listener.
    defaults: "2025-05-24",
  });
}
