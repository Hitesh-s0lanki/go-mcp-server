import { redirect } from "next/navigation";
import { SignUp } from "@clerk/nextjs";
import { auth } from "@clerk/nextjs/server";

export default async function SignUpPage() {
  const { userId } = await auth();
  // Already signed in? Skip the form and land on the API-keys dashboard.
  if (userId) redirect("/doc/keys");
  // New accounts land on /doc/keys so they can mint their first key right away.
  return <SignUp forceRedirectUrl="/doc/keys" />;
}
