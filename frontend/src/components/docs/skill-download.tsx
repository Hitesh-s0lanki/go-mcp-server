"use client";

import { useState, useSyncExternalStore } from "react";
import { Check, Copy, Download, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { statefulSkill, statefulSkillMarkdown } from "@/lib/stateful-skill";

const ROUTE = "/api/skills/stateful-memory";

// The site's own origin only exists in the browser. Read it hydration-safely:
// the server snapshot is empty, so the statically rendered markup matches, and
// the real origin fills in on the client after hydration. origin never changes,
// so there is nothing to subscribe to.
const subscribe = () => () => {};
function useOrigin() {
  return useSyncExternalStore(
    subscribe,
    () => window.location.origin,
    () => "",
  );
}

/**
 * Download surface for the stateful-memory Agent Skill, shown on the Memory
 * server page. The memory_* tools give an agent a place to store knowledge;
 * this skill is the protocol that makes it actually *use* it — recall before
 * acting, persist after. Offer three ways to install it: a direct download of
 * SKILL.md, a copy of its full contents, and a one-line curl that drops it
 * straight into `.claude/skills/`.
 *
 * The install command needs the site's own origin, which only exists in the
 * browser — resolve it after mount so the statically rendered markup stays
 * hydration-stable, and fall back to a relative curl until it does.
 */
export function SkillDownload() {
  const origin = useOrigin();
  const [copiedCmd, setCopiedCmd] = useState(false);

  const installCmd = `mkdir -p .claude/skills/${statefulSkill.name}\ncurl -sL ${origin}${ROUTE} -o ${statefulSkill.installPath}`;

  async function copyCmd() {
    try {
      await navigator.clipboard.writeText(installCmd);
      setCopiedCmd(true);
      toast.success("Install command copied");
      setTimeout(() => setCopiedCmd(false), 1500);
    } catch {
      toast.error("Couldn't copy to clipboard");
    }
  }

  return (
    <div className="not-prose my-6 overflow-hidden rounded-xl border bg-card">
      <div className="flex flex-col gap-4 p-5 sm:flex-row sm:items-start">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
          <Sparkles className="size-4.5 text-foreground/70" />
        </div>

        <div className="min-w-0 flex-1">
          <div className="text-sm font-medium">Stateful memory skill</div>
          <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
            {statefulSkill.tagline} Drop it into your agent&rsquo;s{" "}
            <code className="rounded bg-muted px-1 py-0.5 font-mono text-xs text-foreground/80">
              .claude/skills/
            </code>{" "}
            folder so it recalls before acting and persists after — instead of
            starting every session from a blank slate.
          </p>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button size="sm" asChild>
              {/* A real navigation to the attachment route triggers the browser
                  download; `download` names the saved file. */}
              <a href={ROUTE} download="SKILL.md">
                <Download className="size-3.5" />
                Download SKILL.md
              </a>
            </Button>
            <CopySkillButton />
          </div>
        </div>
      </div>

      <div className="border-t bg-muted/40">
        <div className="flex items-center justify-between px-5 py-2">
          <span className="font-mono text-xs text-muted-foreground">
            install into your project
          </span>
          <button
            type="button"
            onClick={copyCmd}
            aria-label="Copy install command"
            className="inline-flex items-center gap-1.5 rounded-md text-muted-foreground transition-colors hover:text-foreground"
          >
            {copiedCmd ? (
              <Check className="size-3.5 text-emerald-500" />
            ) : (
              <Copy className="size-3.5" />
            )}
          </button>
        </div>
        <pre className="overflow-x-auto px-5 pb-4 font-mono text-xs leading-relaxed text-foreground/80">
          {installCmd}
        </pre>
      </div>
    </div>
  );
}

/** Copies the full SKILL.md contents to the clipboard. */
function CopySkillButton() {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(statefulSkillMarkdown);
      setCopied(true);
      toast.success("SKILL.md copied");
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error("Couldn't copy to clipboard");
    }
  }

  return (
    <Button size="sm" variant="outline" onClick={copy}>
      {copied ? (
        <Check className="size-3.5 text-emerald-500" />
      ) : (
        <Copy className="size-3.5" />
      )}
      Copy contents
    </Button>
  );
}
