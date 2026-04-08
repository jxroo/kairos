import { PageHeader } from "@/components/layout/PageHeader";
import { HealthCard } from "@/components/system/HealthCard";
import { ModelList } from "@/components/system/ModelList";
import { ToolsList } from "@/components/system/ToolsList";

export function SystemStatus() {
  return (
    <div className="animate-fade-in-up">
      <PageHeader title="System Status" description="Runtime health, models, and tools" />
      <div className="space-y-4">
        <HealthCard />
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
          <ModelList />
          <ToolsList />
        </div>
      </div>
    </div>
  );
}
