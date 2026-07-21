import type { ReactNode } from "react";
import { TopNav } from "@/components/docs/top-nav";
import { SidebarNav } from "@/components/docs/sidebar";

export default function DocLayout({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-screen bg-background">
      <TopNav />
      <div className="mx-auto flex max-w-[1400px] px-4 sm:px-6">
        {/* Left sidebar */}
        <aside className="sticky top-14 hidden h-[calc(100vh-3.5rem)] w-64 shrink-0 overflow-y-auto py-8 pr-6 lg:block">
          <SidebarNav />
        </aside>
        {/* Content + right rail */}
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  );
}
