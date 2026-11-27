"use client";

import * as React from "react";

import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";

import { TaskArchivesPanel } from "./_components/task-archives-panel";
import { TaskTable } from "./_components/task-table";
import { TaskToolbar } from "./_components/task-toolbar";
import { useTaskActions } from "./_components/use-task-actions";
import { useTaskList } from "./_components/use-task-list";

export default function TasksPage() {
  const [activeTab, setActiveTab] = React.useState("current");
  const list = useTaskList();
  const actions = useTaskActions(list, setActiveTab);

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="gap-4">
      <TabsList className="mx-4 lg:mx-6">
        <TabsTrigger value="current">{t("Current tasks")}</TabsTrigger>
        <TabsTrigger value="archived">{t("Archived")}</TabsTrigger>
      </TabsList>
      <TabsContent value="current">
        <Card>
          <CardContent className="flex flex-col gap-4 px-0 pt-6">
            <TaskToolbar list={list} actions={actions} />
            <TaskTable list={list} actions={actions} />
          </CardContent>
        </Card>
      </TabsContent>
      <TabsContent value="archived">
        <TaskArchivesPanel onChanged={list.load} />
      </TabsContent>
    </Tabs>
  );
}
