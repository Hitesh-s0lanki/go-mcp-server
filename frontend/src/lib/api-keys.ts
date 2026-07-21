import "server-only";

import { auth } from "@clerk/nextjs/server";

import { MCP_BASE_URL } from "./mcp";

// Client for the Go MCP server's Clerk-authenticated key API. The Go server is
// the authority for key ownership and the per-user cap; this module is a thin
// proxy that forwards the caller's Clerk session token so the Go server can
// verify identity itself. The frontend no longer touches Postgres directly.

/** A key as shown in the list: the secret itself is never included. */
export type ApiKeySummary = {
  id: string;
  label: string;
  /** Masked preview, e.g. "mcp_••••••••6715" — enough to recognise, not to use. */
  masked: string;
  createdAt: string;
};

/**
 * Forward a request to the Go server's key API with the caller's Clerk session
 * token attached as a Bearer credential. Runs server-side only (route handlers
 * and server components), so the token never reaches the browser.
 */
export async function keysProxy(
  path: string,
  init?: { method?: string; body?: string },
): Promise<Response> {
  const { getToken } = await auth();
  const token = await getToken();
  return fetch(`${MCP_BASE_URL}${path}`, {
    method: init?.method ?? "GET",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: init?.body,
    // Keys are per-request user data; never cache the dashboard's view of them.
    cache: "no-store",
  });
}

/**
 * The signed-in user's keys plus their cap, for the dashboard page. Throws on a
 * non-2xx response so the page surfaces the failure rather than rendering an
 * empty list as if the user had no keys.
 */
export async function listKeys(): Promise<{ keys: ApiKeySummary[]; max: number }> {
  const res = await keysProxy("/api/keys");
  if (!res.ok) {
    throw new Error(`Failed to load API keys (${res.status})`);
  }
  return res.json();
}
