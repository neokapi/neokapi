// The empty flow list, pointing at the next step rather than a blank space.
//
// A project with no flows of its own still converges: it runs the default flow.
// So the empty state says that, and offers to add a flow, rather than reading
// as something missing. In ad-hoc mode the built-in flows are always present,
// so this shows only for a genuinely empty user set. Shared across kapi desktop
// and the platform's flow list.

import { Workflow } from "lucide-react";
import { Button } from "../ui/button";
import { EmptyState } from "../EmptyState";

export function FlowsEmptyState({
  projectMode,
  onCreate,
}: {
  projectMode: boolean;
  onCreate: () => void;
}) {
  return (
    <EmptyState
      icon={<Workflow size={24} className="text-muted-foreground/50" />}
      title={projectMode ? "This project runs the default flow" : "No flows yet"}
      description={
        projectMode
          ? "Add a flow here to give a collection its own sequence of steps."
          : "Create a flow, or open a flow file to run it."
      }
      action={
        <Button size="sm" onClick={onCreate}>
          Create Flow
        </Button>
      }
    />
  );
}
