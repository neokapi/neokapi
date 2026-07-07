/** A single row in the Add-tool dialog's "compose" section (e.g. add a route,
 *  wrap in protection). Presentational — the parent owns the action. */
export function ComposeAction({
  icon,
  title,
  description,
  onClick,
}: {
  icon: React.ReactNode;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className="flex w-full cursor-pointer items-start gap-2.5 px-4 py-2.5 text-left transition-colors hover:bg-muted"
      onClick={onClick}
    >
      {icon}
      <span className="flex flex-col gap-0.5">
        <span className="text-xs font-medium text-foreground">{title}</span>
        <span className="text-[11px] leading-snug text-muted-foreground">{description}</span>
      </span>
    </button>
  );
}
