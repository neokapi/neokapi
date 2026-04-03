import * as React from "react";
import { Card, CardContent } from "./ui/card";

interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}

export function EmptyState({ icon, title, description, action, className }: EmptyStateProps) {
  return (
    <Card className={`border-dashed ${className ?? ""}`}>
      <CardContent className="py-8 text-center">
        {icon && <div className="mx-auto mb-3 text-muted-foreground">{icon}</div>}
        <p className="text-sm font-medium text-muted-foreground">{title}</p>
        {description && <p className="mt-1 text-xs text-muted-foreground">{description}</p>}
        {action && <div className="mt-3">{action}</div>}
      </CardContent>
    </Card>
  );
}
