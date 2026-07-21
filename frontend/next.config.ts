import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // PostHog reverse proxy. The browser sends analytics to same-origin /ingest/*
  // and Next rewrites those requests to PostHog's ingestion hosts server-side.
  // This keeps events first-party so ad blockers (which block *.posthog.com)
  // don't drop them. Destinations below are US Cloud — for EU Cloud swap to
  // https://eu-assets.i.posthog.com and https://eu.i.posthog.com.
  async rewrites() {
    return [
      {
        source: "/ingest/static/:path*",
        destination: "https://us-assets.i.posthog.com/static/:path*",
      },
      {
        source: "/ingest/:path*",
        destination: "https://us.i.posthog.com/:path*",
      },
    ];
  },
  // PostHog's API endpoints rely on trailing slashes; without this Next would
  // 308-redirect and break proxied requests.
  skipTrailingSlashRedirect: true,
};

export default nextConfig;
