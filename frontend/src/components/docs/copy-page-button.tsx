"use client";

import { useState } from "react";
import { Check, ChevronDown, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";

export function CopyPageButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success("Page copied as Markdown");
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error("Couldn't copy the page");
    }
  }

  return (
    <div className="inline-flex overflow-hidden rounded-lg border">
      <Button variant="ghost" size="sm" onClick={copy} className="gap-2 rounded-none">
        {copied ? (
          <Check className="size-4 text-emerald-500" />
        ) : (
          <Copy className="size-4" />
        )}
        Copy page
      </Button>
      <div className="w-px bg-border" />
      <Button variant="ghost" size="icon" className="rounded-none" onClick={copy}>
        <ChevronDown className="size-4" />
      </Button>
    </div>
  );
}
