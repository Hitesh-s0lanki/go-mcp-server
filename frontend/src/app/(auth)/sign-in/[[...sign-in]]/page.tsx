import { redirect } from "next/navigation";
import { SignIn } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";

export default async function SignInPage() {
  const { userId } = await auth();
  // Already signed in? Skip the form and land on the API-keys dashboard.
  if (userId) redirect("/doc/keys");
  // forceRedirectUrl wins over the env fallback so a fresh sign-in always ends
  // up on /doc/keys regardless of where the flow was entered from.
  return <SignIn forceRedirectUrl="/doc/keys" />;
}
