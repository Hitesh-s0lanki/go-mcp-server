import { servers } from "@/lib/docs";
import { mcpUrl } from "@/lib/mcp";
import { CopyButton } from "./copy-button";
import { ApiKeyCallout } from "./api-key-callout";

/**
 * Renders connection URL cards for one or more namespaces. Each URL is built
 * from NEXT_PUBLIC_MCP_BASE_URL via mcpUrl(), so the base host is defined in
 * exactly one place.
 */
export function Connect({ serverKeys }: { serverKeys: string[] }) {
  return (
    <div className="not-prose my-6 grid gap-3">
      <ApiKeyCallout />
      {serverKeys.map((key) => {
        const s = servers[key];
        if (!s) return null;
        const url = mcpUrl(s.path);
        const Icon = s.icon;
        return (
          <div
            key={key}
            className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3"
          >
            <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
              <Icon className="size-4.5 text-foreground/70" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="text-sm font-medium">{s.name}</div>
              <code className="block truncate font-mono text-xs text-muted-foreground">
                {url}
              </code>
            </div>
            <CopyButton value={url} onCopied={`${s.name} URL copied`} />
          </div>
        );
      })}
    </div>
  );
}
