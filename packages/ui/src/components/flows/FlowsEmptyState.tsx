// The empty flow list, pointing at the next step rather than a blank space.
//
// A project with no flows of its own still converges: it runs the default flow.
// So the empty state says that, and offers to add a flow, rather than reading
// as something missing. In ad-hoc mode the built-in flows are always present,
// so this shows only for a genuinely empty user set. Shared across kapi desktop
// and the platform's flow list; a host whose flows sit elsewhere than a recipe
// passes its own title and description.

import { Workflow } from "lucide-react";
import { Button } from "../ui/button";
import { EmptyState } from "../EmptyState";

export function FlowsEmptyState({
  projectMode,
  onCreate,
  title,
  description,
}: {
  projectMode: boolean;
  onCreate: () => void;
  /** Replaces the mode's title; pass `description` with it. */
  title?: string;
  /** Replaces the mode's description. */
  description?: string;
}) {
  const icon = <Workflow size={24} className="text-muted-foreground/50" />;
  const action = (
    <Button size="sm" onClick={onCreate}>
      Create Flow
    </Button>
  );

  if (title !== undefined) {
    return <EmptyState icon={icon} title={title} description={description} action={action} />;
  }
  if (projectMode) {
    return (
      <EmptyState
        icon={icon}
        title="This project runs the default flow"
        description="Add a flow here to give a collection its own sequence of steps."
        action={action}
      />
    );
  }
  return (
    <EmptyState
      icon={icon}
      title="No flows yet"
      description="Create a flow, or open a flow file to run it."
      action={action}
    />
  );
}
