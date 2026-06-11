import { ParameterGroup, PropertySchema, FormatParamsValue } from "./types";

// UI components from the ui directory
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "../ui/collapsible";
import { ChevronDown, ChevronRight } from "../icons";

import { ParameterField } from "./ParameterField";

interface ParameterGroupSectionProps {
  group: ParameterGroup;
  properties: Record<string, PropertySchema>;
  values: FormatParamsValue;
  collapsed: boolean;
  onToggle: () => void;
  onChange: (field: string, value: unknown) => void;
}

export function ParameterGroupSection({
  group,
  properties,
  values,
  collapsed,
  onToggle,
  onChange,
}: ParameterGroupSectionProps) {
  return (
    <Collapsible open={!collapsed} onOpenChange={() => onToggle()}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground text-muted-foreground w-full"
        >
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          {group.label}
        </button>
      </CollapsibleTrigger>

      <CollapsibleContent className="mt-2 ml-6 space-y-3">
        {group.description && (
          <p className="text-xs text-muted-foreground mb-2">{group.description}</p>
        )}
        {group.fields.map((fieldName) => {
          const schema = properties[fieldName];
          if (!schema || schema.deprecated) return null;
          return (
            <ParameterField
              key={fieldName}
              name={fieldName}
              schema={schema}
              value={values[fieldName]}
              onChange={(v) => onChange(fieldName, v)}
              allValues={values}
            />
          );
        })}
      </CollapsibleContent>
    </Collapsible>
  );
}
