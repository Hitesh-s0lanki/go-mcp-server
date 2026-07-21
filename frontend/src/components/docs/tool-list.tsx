import { servers } from "@/lib/docs";
import { Badge } from "@/components/ui/badge";

export function ToolList({ serverKey }: { serverKey: string }) {
  const s = servers[serverKey];
  if (!s) return null;

  return (
    <div className="not-prose my-6 space-y-4">
      {s.tools.map((tool) => (
        <div key={tool.name} className="rounded-xl border bg-card">
          <div className="flex flex-wrap items-center gap-2 border-b px-5 py-3">
            <code className="font-mono text-sm font-semibold text-foreground">
              {tool.name}
            </code>
            <Badge variant="secondary" className="font-mono text-[11px]">
              tool
            </Badge>
          </div>
          <div className="px-5 py-4">
            <p className="text-sm leading-relaxed text-muted-foreground">
              {tool.summary}
            </p>
            {tool.params.length > 0 && (
              <dl className="mt-4 space-y-2.5">
                {tool.params.map((p) => (
                  <div
                    key={p.name}
                    className="grid grid-cols-[minmax(0,10rem)_1fr] gap-x-4 gap-y-1 border-t pt-2.5 first:border-t-0 first:pt-0 sm:items-baseline"
                  >
                    <dt className="flex flex-wrap items-center gap-1.5">
                      <code className="font-mono text-xs font-medium text-foreground">
                        {p.name}
                      </code>
                      {p.required && (
                        <span className="text-[10px] font-medium uppercase tracking-wide text-rose-500">
                          required
                        </span>
                      )}
                    </dt>
                    <dd className="text-xs leading-relaxed text-muted-foreground">
                      <span className="mr-2 font-mono text-foreground/70">
                        {p.type}
                        {p.default !== undefined && ` · default ${p.default}`}
                      </span>
                      {p.description}
                    </dd>
                  </div>
                ))}
              </dl>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
