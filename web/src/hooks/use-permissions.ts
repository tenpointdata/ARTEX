"use client";

import { useEffect, useState } from "react";

import { auth } from "@/lib/auth";
import type { Permission } from "@/lib/permissions";

// The signed-in role's permissions, or null until the session lookup answers.
export function usePermissions(): Permission[] | null {
  const [held, setHeld] = useState<Permission[] | null>(null);
  useEffect(() => {
    let live = true;
    void auth.permissions().then((p) => {
      if (live) setHeld(p);
    });
    return () => {
      live = false;
    };
  }, []);
  return held;
}
