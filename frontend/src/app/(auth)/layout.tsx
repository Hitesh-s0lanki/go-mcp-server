import Link from "next/link";

import { site } from "@/lib/site";
import { Logo } from "@/components/brand/logo";
import { AuthBrandPanel } from "./_components/auth-brand-panel";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="grid min-h-dvh lg:grid-cols-2">
      <AuthBrandPanel />

      {/* Right: Clerk form */}
      <div className="flex min-h-dvh flex-col items-center justify-center px-6 py-12">
        {/* Mobile-only logo (brand panel is hidden below lg) */}
        <Link
          href="/"
          className="mb-8 flex items-center gap-2 text-base font-semibold lg:hidden"
        >
          <Logo className="size-7" />
          {site.name}
        </Link>

        {children}
      </div>
    </div>
  );
}
