import * as React from "react";
import { ResponsiveContainer } from "recharts";
import { cn } from "../../lib/utils";

// ---------------------------------------------------------------------------
// Chart config
// ---------------------------------------------------------------------------

export type ChartConfig = Record<
  string,
  {
    label?: React.ReactNode;
    icon?: React.ComponentType;
  } & ({ color?: string; theme?: never } | { color?: never; theme: Record<string, string> })
>;

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

type ChartContextProps = { config: ChartConfig };

const ChartContext = React.createContext<ChartContextProps | null>(null);

export function useChart() {
  const ctx = React.useContext(ChartContext);
  if (!ctx) throw new Error("useChart must be used within <ChartContainer />");
  return ctx;
}

// ---------------------------------------------------------------------------
// ChartContainer
// ---------------------------------------------------------------------------

export interface ChartContainerProps extends React.ComponentPropsWithoutRef<"div"> {
  id?: string;
  className?: string;
  config: ChartConfig;
  children: React.ComponentProps<typeof ResponsiveContainer>["children"];
}

export function ChartContainer({ id, className, children, config, ...props }: ChartContainerProps) {
  const uniqueId = React.useId();
  const chartId = `chart-${id || uniqueId.replace(/:/g, "")}`;

  return (
    <ChartContext.Provider value={{ config }}>
      <div
        data-chart={chartId}
        className={cn(
          "[&_.recharts-cartesian-axis-tick_text]:fill-muted-foreground flex aspect-video justify-center text-xs [&_.recharts-cartesian-grid_line[stroke='#ccc']]:stroke-border/50 [&_.recharts-curve.recharts-tooltip-cursor]:stroke-border [&_.recharts-dot[stroke='#fff']]:stroke-transparent [&_.recharts-layer]:outline-hidden [&_.recharts-radial-bar-background-sector]:fill-muted [&_.recharts-rectangle.recharts-tooltip-cursor]:fill-muted [&_.recharts-reference-line_[stroke='#ccc']]:stroke-border [&_.recharts-sector[stroke='#fff']]:stroke-transparent [&_.recharts-sector]:outline-hidden [&_.recharts-surface]:outline-hidden",
          className,
        )}
        {...props}
      >
        <ChartStyle id={chartId} config={config} />
        <ResponsiveContainer>{children}</ResponsiveContainer>
      </div>
    </ChartContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// ChartStyle (CSS custom properties)
// ---------------------------------------------------------------------------

function ChartStyle({ id, config }: { id: string; config: ChartConfig }) {
  const colorConfig = Object.entries(config).filter(([, cfg]) => cfg.color || cfg.theme);
  if (!colorConfig.length) return null;

  const themes = { light: "", dark: ".dark" } as const;

  return (
    <style
      dangerouslySetInnerHTML={{
        __html: Object.entries(themes)
          .map(
            ([theme, prefix]) => `
${prefix} [data-chart=${id}] {
${colorConfig
  .map(([key, itemConfig]) => {
    const color = itemConfig.theme?.[theme] || itemConfig.color;
    return color ? `  --color-${key}: ${color};` : null;
  })
  .filter(Boolean)
  .join("\n")}
}`,
          )
          .join("\n"),
      }}
    />
  );
}

// ---------------------------------------------------------------------------
// Re-export Recharts Tooltip and Legend (pass-through)
// ---------------------------------------------------------------------------

export { Tooltip as ChartTooltip, Legend as ChartLegend } from "recharts";

// ---------------------------------------------------------------------------
// ChartTooltipContent
// ---------------------------------------------------------------------------

export interface ChartTooltipContentProps {
  active?: boolean;
  payload?: Array<{
    name?: string;
    value?: number;
    dataKey?: string | number;
    color?: string;
    fill?: string;
    payload?: Record<string, unknown>;
  }>;
  label?: string;
  hideLabel?: boolean;
  hideIndicator?: boolean;
  indicator?: "dot" | "line" | "dashed";
  labelFormatter?: (label: string, payload: unknown[]) => React.ReactNode;
  formatter?: (
    value: number | undefined,
    name: string | undefined,
    item: Record<string, unknown>,
    index: number,
  ) => React.ReactNode;
  className?: string;
  nameKey?: string;
  labelKey?: string;
}

export function ChartTooltipContent({
  active,
  payload,
  className,
  indicator = "dot",
  hideLabel = false,
  hideIndicator = false,
  label,
  labelFormatter,
  formatter,
  nameKey,
}: ChartTooltipContentProps) {
  const { config } = useChart();

  if (!active || !payload?.length) return null;

  const tooltipLabel = (() => {
    if (hideLabel || !payload.length) return null;
    const value = label && config[label]?.label ? config[label].label : label;
    if (labelFormatter && value != null) {
      return <div className="font-medium">{labelFormatter(String(value), payload)}</div>;
    }
    return value ? <div className="font-medium">{String(value)}</div> : null;
  })();

  const nestLabel = payload.length === 1 && indicator !== "dot";

  return (
    <div
      className={cn(
        "grid min-w-[8rem] items-start gap-1.5 rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl",
        className,
      )}
    >
      {!nestLabel ? tooltipLabel : null}
      <div className="grid gap-1.5">
        {payload.map((item, index) => {
          const key = `${nameKey || item.name || item.dataKey || "value"}`;
          const itemConfig = config[key];
          const indicatorColor =
            (item.payload as Record<string, string> | undefined)?.fill || item.color;

          return (
            <div
              key={String(item.dataKey ?? index)}
              className={cn(
                "flex w-full flex-wrap items-stretch gap-2 [&>svg]:h-2.5 [&>svg]:w-2.5 [&>svg]:text-muted-foreground",
                indicator === "dot" && "items-center",
              )}
            >
              {formatter && item.value !== undefined && item.name ? (
                formatter(item.value, item.name, item as Record<string, unknown>, index)
              ) : (
                <>
                  {itemConfig?.icon ? (
                    <itemConfig.icon />
                  ) : (
                    !hideIndicator && (
                      <div
                        className={cn("shrink-0 rounded-[2px]", {
                          "h-2.5 w-2.5": indicator === "dot",
                          "w-1": indicator === "line",
                          "w-0 border-[1.5px] border-dashed bg-transparent": indicator === "dashed",
                        })}
                        style={{ backgroundColor: indicatorColor, borderColor: indicatorColor }}
                      />
                    )
                  )}
                  <div className="flex flex-1 justify-between items-center leading-none">
                    <span className="text-muted-foreground">{itemConfig?.label || item.name}</span>
                    {item.value !== undefined && (
                      <span className="ml-2 font-mono font-medium tabular-nums text-foreground">
                        {item.value.toLocaleString()}
                      </span>
                    )}
                  </div>
                </>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ChartLegendContent
// ---------------------------------------------------------------------------

export interface ChartLegendContentProps {
  payload?: Array<{
    value?: string;
    dataKey?: string;
    color?: string;
  }>;
  verticalAlign?: "top" | "bottom";
  className?: string;
  hideIcon?: boolean;
  nameKey?: string;
}

export function ChartLegendContent({
  className,
  hideIcon = false,
  payload,
  verticalAlign = "bottom",
  nameKey,
}: ChartLegendContentProps) {
  const { config } = useChart();

  if (!payload?.length) return null;

  return (
    <div
      className={cn(
        "flex items-center justify-center gap-4",
        verticalAlign === "top" ? "pb-3" : "pt-3",
        className,
      )}
    >
      {payload.map((item) => {
        const key = `${nameKey || item.dataKey || "value"}`;
        const itemConfig = config[key];

        return (
          <div
            key={item.value}
            className="flex items-center gap-1.5 [&>svg]:h-3 [&>svg]:w-3 [&>svg]:text-muted-foreground"
          >
            {itemConfig?.icon && !hideIcon ? (
              <itemConfig.icon />
            ) : (
              <div
                className="h-2 w-2 shrink-0 rounded-[2px]"
                style={{ backgroundColor: item.color }}
              />
            )}
            {itemConfig?.label}
          </div>
        );
      })}
    </div>
  );
}
