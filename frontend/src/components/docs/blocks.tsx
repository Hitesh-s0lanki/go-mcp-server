import Link from "next/link";
import { ArrowRight, Info, Lightbulb, TriangleAlert } from "lucide-react";
import type { Block } from "@/lib/docs";
import { cn } from "@/lib/utils";
import { CodeBlock } from "./code-block";
import { Connect } from "./connect";
import { ToolList } from "./tool-list";
import { SkillDownload } from "./skill-download";
import { ArchitectureDiagram } from "./architecture-diagram";

const calloutStyles = {
  tip: { icon: Lightbulb, wrap: "border-emerald-500/25 bg-emerald-500/[0.07]", ic: "text-emerald-500" },
  info: { icon: Info, wrap: "border-sky-500/25 bg-sky-500/[0.07]", ic: "text-sky-500" },
  warning: { icon: TriangleAlert, wrap: "border-amber-500/30 bg-amber-500/[0.08]", ic: "text-amber-500" },
} as const;

function Callout({
  variant = "info",
  title,
  content,
}: {
  variant?: "tip" | "info" | "warning";
  title?: string;
  content: React.ReactNode;
}) {
  const style = calloutStyles[variant];
  const Icon = style.icon;
  return (
    <div className={cn("not-prose my-6 rounded-xl border px-5 py-4", style.wrap)}>
      <div className="flex items-center gap-2">
        <Icon className={cn("size-4", style.ic)} />
        {title && <span className="text-sm font-semibold text-foreground">{title}</span>}
      </div>
      <div className="mt-2 space-y-2 text-sm leading-relaxed text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-xs [&_code]:text-foreground/80 [&_strong]:font-semibold [&_strong]:text-foreground">
        {content}
      </div>
    </div>
  );
}

function CardGrid({
  items,
}: {
  items: { title: string; description: string; href: string; icon: React.ComponentType<{ className?: string }> }[];
}) {
  return (
    <div className="not-prose my-6 grid gap-3 sm:grid-cols-3">
      {items.map((c) => {
        const Icon = c.icon;
        return (
          <Link
            key={c.title}
            href={c.href}
            className="group flex flex-col rounded-xl border bg-card p-5 transition-colors hover:border-foreground/20 hover:bg-accent/40"
          >
            <Icon className="size-5 text-foreground/70" />
            <div className="mt-3 flex items-center gap-1 text-sm font-semibold">
              {c.title}
              <ArrowRight className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
            </div>
            <p className="mt-1 text-sm leading-relaxed text-muted-foreground">
              {c.description}
            </p>
          </Link>
        );
      })}
    </div>
  );
}

function Steps({ items }: { items: { title: string; content: React.ReactNode }[] }) {
  return (
    <ol className="not-prose my-6 space-y-4">
      {items.map((s, i) => (
        <li key={i} className="flex gap-4">
          <span className="flex size-7 shrink-0 items-center justify-center rounded-full border bg-muted text-xs font-semibold text-foreground">
            {i + 1}
          </span>
          <div className="pt-0.5">
            <div className="text-sm font-medium text-foreground">{s.title}</div>
            <div className="mt-0.5 text-sm leading-relaxed text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-xs [&_code]:text-foreground/80">
              {s.content}
            </div>
          </div>
        </li>
      ))}
    </ol>
  );
}

export function BlockRenderer({ blocks }: { blocks: Block[] }) {
  return (
    <>
      {blocks.map((block, i) => {
        switch (block.kind) {
          case "lead":
            return (
              <p
                key={i}
                className="text-lg leading-relaxed text-foreground/80 [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_code]:text-foreground/90 [&_strong]:font-semibold [&_strong]:text-foreground"
              >
                {block.content}
              </p>
            );
          case "heading": {
            const Icon = block.icon;
            return (
              <h2
                key={i}
                id={block.id}
                className="group mt-12 flex scroll-mt-24 items-center gap-2.5 text-2xl font-semibold tracking-tight text-foreground"
              >
                {Icon && <Icon className="size-5 text-foreground/50" />}
                <a href={`#${block.id}`} className="hover:underline">
                  {block.text}
                </a>
              </h2>
            );
          }
          case "text":
            return (
              <p
                key={i}
                className="mt-4 leading-relaxed text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.85em] [&_code]:text-foreground/80"
              >
                {block.content}
              </p>
            );
          case "callout":
            return <Callout key={i} variant={block.variant} title={block.title} content={block.content} />;
          case "code":
            return <CodeBlock key={i} code={block.code} lang={block.lang} title={block.title} />;
          case "connect":
            return <Connect key={i} serverKeys={block.serverKeys} />;
          case "tools":
            return <ToolList key={i} serverKey={block.serverKey} />;
          case "skill":
            return <SkillDownload key={i} />;
          case "cards":
            return <CardGrid key={i} items={block.items} />;
          case "steps":
            return <Steps key={i} items={block.items} />;
          case "diagram":
            return <ArchitectureDiagram key={i} variant={block.variant} />;
          default:
            return null;
        }
      })}
    </>
  );
}
