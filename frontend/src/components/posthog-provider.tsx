"use client";

import posthog from "posthog-js";
import { PostHogProvider as PHProvider } from "posthog-js/react";
import type { ReactNode } from "react";

// The PostHog singleton is initialized in src/instrumentation-client.ts (before
// hydration). This provider just hands that already-configured client to React
// context, so components can call hooks like usePostHog(), useFeatureFlagEnabled(),
// and posthog.capture() without re-initializing. Rendering it does NOT create a
// second client.
export function PostHogProvider({ children }: { children: ReactNode }) {
  return <PHProvider client={posthog}>{children}</PHProvider>;
}
