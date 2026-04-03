import * as React from "react";

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  actions?: React.ReactNode;
  backButton?: React.ReactNode;
  className?: string;
}

export function PageHeader({ title, subtitle, actions, backButton, className }: PageHeaderProps) {
  return (
    <div className={`mb-6 flex items-center justify-between ${className ?? ""}`}>
      <div className="flex items-center gap-3">
        {backButton}
        <div>
          <h1 className="text-xl font-semibold">{title}</h1>
          {subtitle && <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  );
}
