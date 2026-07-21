import { ArrowUpRight, Construction } from "lucide-react";
import type { DocPage } from "@/lib/docs";
import { site } from "@/lib/site";
import { Button } from "@/components/ui/button";

/** The "under development" state rendered for Relivo Platform (wip) pages. */
export function ComingSoon({ page }: { page: DocPage }) {
  const Icon = page.wipIcon ?? Construction;

  return (
    <div className="py-4">
      <div className="mb-3 text-sm font-medium text-muted-foreground">
        {page.eyebrow}
      </div>

      <span className="inline-flex items-center gap-1.5 rounded-full border border-amber-500/30 bg-amber-500/10 px-3 py-1 text-xs font-medium text-amber-600 dark:text-amber-400">
        <span className="relative flex size-2">
          <span className="absolute inline-flex size-full animate-ping rounded-full bg-amber-500/60" />
          <span className="relative inline-flex size-2 rounded-full bg-amber-500" />
        </span>
        Under development
      </span>

      <div className="mt-5 flex items-start gap-4">
        <div className="flex size-12 shrink-0 items-center justify-center rounded-2xl border bg-card">
          <Icon className="size-6 text-foreground/80" />
        </div>
        <div>
          <h1 className="text-4xl font-semibold tracking-tight text-foreground">
            {page.title}
          </h1>
          <p className="mt-3 max-w-2xl text-base leading-relaxed text-muted-foreground">
            {page.description}
          </p>
        </div>
      </div>

      {page.wipFeatures && page.wipFeatures.length > 0 && (
        <div className="mt-10">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
            What&rsquo;s coming
          </h2>
          <div className="mt-4 grid gap-px overflow-hidden rounded-2xl border bg-border sm:grid-cols-2 lg:grid-cols-3">
            {page.wipFeatures.map((f) => (
              <div key={f.title} className="bg-card p-5">
                <h3 className="font-semibold">{f.title}</h3>
                <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">
                  {f.body}
                </p>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="mt-10 flex flex-col gap-4 rounded-2xl border bg-muted/30 p-6 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="font-medium">Preview the work in progress</div>
          <p className="mt-1 text-sm text-muted-foreground">
            This surface is still being built. Follow along in the live preview;
            it changes often and isn&rsquo;t final.
          </p>
        </div>
        <Button asChild className="shrink-0">
          <a href={site.console} target="_blank" rel="noopener noreferrer">
            Open preview <ArrowUpRight className="size-4" />
          </a>
        </Button>
      </div>
    </div>
  );
}
