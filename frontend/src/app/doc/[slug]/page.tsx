import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { pages, pageMarkdown, tocOf } from "@/lib/docs";
import { MCP_BASE_URL } from "@/lib/mcp";
import { BlockRenderer } from "@/components/docs/blocks";
import { CopyPageButton } from "@/components/docs/copy-page-button";
import { ComingSoon } from "@/components/docs/coming-soon";
import { Toc } from "@/components/docs/toc";

export function generateStaticParams() {
  return Object.keys(pages).map((slug) => ({ slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const page = pages[slug];
  if (!page) return { title: "Not found · Relivo docs" };
  return {
    title: `${page.title} · Relivo docs`,
    description: typeof page.description === "string" ? page.description : undefined,
  };
}

export default async function DocPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const page = pages[slug];
  if (!page) notFound();

  // Under-development pages render a dedicated "coming soon" state (no TOC).
  if (page.wip) {
    return (
      <div className="py-10">
        <article className="min-w-0 max-w-3xl">
          <ComingSoon page={page} />
        </article>
      </div>
    );
  }

  const toc = tocOf(page);
  const markdown = pageMarkdown(page, MCP_BASE_URL);

  return (
    <div className="flex gap-12 py-10">
      <article className="min-w-0 max-w-3xl flex-1">
        <div className="mb-2 text-sm font-medium text-muted-foreground">
          {page.eyebrow}
        </div>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <h1 className="text-4xl font-semibold tracking-tight text-foreground">
            {page.title}
          </h1>
          <CopyPageButton text={markdown} />
        </div>
        <p className="mt-4 text-base leading-relaxed text-muted-foreground">
          {page.description}
        </p>

        <div className="mt-8">
          <BlockRenderer blocks={page.blocks} />
        </div>
      </article>

      <Toc items={toc} />
    </div>
  );
}
