"use client";

import { ClerkProvider as BaseClerkProvider } from "@clerk/nextjs";
import { dark } from "@clerk/themes";
import { useTheme } from "next-themes";

/**
 * Wraps Clerk's provider so its widgets (SignIn/SignUp/UserButton) follow the
 * site's light/dark theme. next-themes owns the `.dark` class on <html>; here we
 * read the resolved theme and hand Clerk the matching baseTheme. `colorPrimary`
 * is pinned to the site's foreground token so Clerk's accents match our buttons.
 */
export function ClerkProvider({ children }: { children: React.ReactNode }) {
  const { resolvedTheme } = useTheme();

  return (
    <BaseClerkProvider
      afterSignOutUrl="/"
      appearance={{
        theme: resolvedTheme === "dark" ? dark : undefined,
        variables: { colorPrimary: "oklch(0.205 0 0)" },
        elements: {
          cardBox: { boxShadow: "none", border: "none" },
          card: { boxShadow: "none", border: "none" },
        },
      }}
    >
      {children}
    </BaseClerkProvider>
  );
}
