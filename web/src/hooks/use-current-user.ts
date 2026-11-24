"use client";

import { useSession } from "@/hooks/use-session";
import { type CurrentUser, currentUserOf } from "@/lib/auth";

export function useCurrentUser(): CurrentUser {
  return currentUserOf(useSession());
}
