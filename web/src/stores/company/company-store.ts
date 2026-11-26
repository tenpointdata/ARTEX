"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

import type { Company } from "@/lib/types";

// The sidebar's company switcher; null means "all companies". Pages pass the
// id to the server, which filters: findings (list, groups, asset tree, stat
// cards) and tasks take `company`, assets take `company_id`, traffic takes
// `company`.
//
// A client_read session always names one of its bound companies: the server
// refuses "all" when it is bound to several and 404s an unbound id. `bound` is
// that list (GET /api/me/companies), null until loaded and always null for
// other roles. setBound keeps the persisted choice only if it is bound (another
// user of this browser may have picked it) and otherwise picks the first.
type CompanyState = {
  companyId: number | null;
  companyName: string;
  bound: Company[] | null;
  setCompany: (id: number | null, name?: string) => void;
  setBound: (rows: Company[]) => void;
};

export const useCompanyStore = create<CompanyState>()(
  persist(
    (set, get) => ({
      companyId: null,
      companyName: "",
      bound: null,
      setCompany: (id, name = "") => set({ companyId: id, companyName: id === null ? "" : name }),
      setBound: (rows) => {
        const pick = rows.find((c) => c.id === get().companyId) ?? rows[0];
        set({ bound: rows, companyId: pick?.id ?? null, companyName: pick?.name ?? "" });
      },
    }),
    {
      name: "artex-active-company",
      partialize: (s) => ({ companyId: s.companyId, companyName: s.companyName }),
    },
  ),
);

// useActiveCompany returns the active company id as the `company` query value,
// or undefined when every company is in view.
export function useActiveCompany(): string | undefined {
  const companyId = useCompanyStore((s) => s.companyId);
  return companyId === null ? undefined : String(companyId);
}

// useActiveCompanyID is the numeric id, for filters that take an id.
export function useActiveCompanyID(): number | null {
  return useCompanyStore((s) => s.companyId);
}
