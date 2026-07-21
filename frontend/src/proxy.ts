import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";

// Next 16 renamed the `middleware` file convention to `proxy`. This runs Clerk
// on every matched request so `auth()` is available in server components and
// route handlers, and the session is kept fresh.
//
// The docs site is public. The API-keys dashboard and its route handlers are
// not: signed-out users hitting these get redirected to NEXT_PUBLIC_CLERK_SIGN_IN_URL
// (pages) or a 404/redirect (handlers). The handlers also re-check auth() with
// the userId for scoping — this matcher is the first gate, not the only one.
const isProtectedRoute = createRouteMatcher(["/doc/keys(.*)", "/api/keys(.*)"]);

export default clerkMiddleware(async (auth, req) => {
  if (isProtectedRoute(req)) await auth.protect();
});

export const config = {
  matcher: [
    // Run on everything except Next internals and static files...
    "/((?!_next|[^?]*\\.(?:html?|css|js(?!on)|jpe?g|webp|png|gif|svg|ttf|woff2?|ico|csv|docx?|xlsx?|zip|webmanifest)).*)",
    // ...and always run on API/tRPC routes.
    "/(api|trpc)(.*)",
  ],
};
