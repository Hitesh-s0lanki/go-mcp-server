/**
 * Single source of truth for where the MCP server lives.
 *
 * Every connection URL rendered across the docs is derived from this value, so
 * pointing the docs at a tunnel or production host is a one-line change in
 * `.env.local` (NEXT_PUBLIC_MCP_BASE_URL). The fallback keeps local dev working
 * with no env file at all.
 */
export const MCP_BASE_URL = (
  process.env.NEXT_PUBLIC_MCP_BASE_URL ?? "http://localhost:8080"
).replace(/\/+$/, "");

/** Join the base URL with a namespace path (e.g. "/memory/mcp"). */
export function mcpUrl(path: string): string {
  return `${MCP_BASE_URL}${path.startsWith("/") ? path : `/${path}`}`;
}
