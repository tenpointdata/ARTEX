"use client";

import { ApprovalRecords } from "@/components/approval-records";
import { AuditChain } from "@/components/audit-chain";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";

export default function ApprovalsPage() {
  return (
    <div className="flex min-w-0 flex-1 flex-col p-4 sm:p-6">
      <Tabs defaultValue="approvals" className="min-w-0 flex-1">
        <TabsList>
          <TabsTrigger value="approvals">{t("Approvals")}</TabsTrigger>
          <TabsTrigger value="chain">{t("Chain")}</TabsTrigger>
        </TabsList>
        <TabsContent value="approvals" className="flex min-w-0 flex-col">
          <ApprovalRecords />
        </TabsContent>
        <TabsContent value="chain" className="min-w-0">
          <AuditChain />
        </TabsContent>
      </Tabs>
    </div>
  );
}
