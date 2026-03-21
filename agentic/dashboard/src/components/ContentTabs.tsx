import { useState } from "react";
import SessionTable from "./SessionTable";
import ActivityTab from "./ActivityTab";
import IssuesFeed from "./IssuesFeed";
import MemoryTab from "./MemoryTab";

const tabs = [
  { id: "jobs", label: "Job Executions" },
  { id: "activity", label: "Activity" },
  { id: "issues", label: "Issues" },
  { id: "memory", label: "Memory" },
] as const;

type TabId = (typeof tabs)[number]["id"];

export default function ContentTabs() {
  const [activeTab, setActiveTab] = useState<TabId>("jobs");

  return (
    <div>
      {/* Tab bar */}
      <div className="flex border-b">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2.5 text-sm font-medium transition-colors border-b-2 bg-transparent cursor-pointer ${
              activeTab === tab.id
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      <div className="pt-4">
        {activeTab === "jobs" && <SessionTable />}
        {activeTab === "activity" && <ActivityTab />}
        {activeTab === "issues" && <IssuesFeed />}
        {activeTab === "memory" && <MemoryTab />}
      </div>
    </div>
  );
}
