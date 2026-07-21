import { codeToHtml } from "shiki";
import { CopyButton } from "./copy-button";

// Map our loose lang labels onto shiki grammars.
function grammar(lang?: string): string {
  switch (lang) {
    case "bash":
    case "sh":
    case "shell":
      return "bash";
    case "json":
      return "json";
    case "go":
      return "go";
    case "ts":
    case "tsx":
      return "tsx";
    case "js":
    case "javascript":
      return "javascript";
    default:
      return "text";
  }
}

export async function CodeBlock({
  code,
  lang,
  title,
}: {
  code: string;
  lang?: string;
  title?: string;
}) {
  // Dual-theme highlight: shiki emits CSS variables that flip with the
  // .dark class, so the same markup works in light and dark mode.
  const html = await codeToHtml(code, {
    lang: grammar(lang),
    themes: { light: "github-light", dark: "github-dark" },
    defaultColor: false,
  });

  return (
    <div className="not-prose my-6 overflow-hidden rounded-xl border bg-muted/40">
      <div className="flex items-center justify-between border-b bg-muted/60 px-4 py-2">
        <span className="font-mono text-xs text-muted-foreground">
          {title ?? lang ?? "code"}
        </span>
        <CopyButton value={code} onCopied="Copied to clipboard" />
      </div>
      <div
        className="shiki-block overflow-x-auto px-4 py-4 text-sm leading-relaxed [&_pre]:bg-transparent!"
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  );
}
