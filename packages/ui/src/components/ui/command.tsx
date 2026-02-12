import * as React from "react";
import { useState, useMemo, useCallback } from "react";
import { cn } from "../../lib/utils";

interface CommandContextValue {
  search: string;
  setSearch: (search: string) => void;
}

const CommandContext = React.createContext<CommandContextValue>({
  search: "",
  setSearch: () => {},
});

interface CommandProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
}

function Command({ className, children, ...props }: CommandProps) {
  const [search, setSearch] = useState("");

  return (
    <CommandContext value={{ search, setSearch }}>
      <div
        className={cn(
          "flex h-full w-full flex-col overflow-hidden rounded-md bg-popover text-popover-foreground",
          className,
        )}
        {...props}
      >
        {children}
      </div>
    </CommandContext>
  );
}

interface CommandInputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  onValueChange?: (value: string) => void;
}

function CommandInput({ className, onValueChange, ...props }: CommandInputProps) {
  const { search, setSearch } = React.useContext(CommandContext);

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setSearch(e.target.value);
      onValueChange?.(e.target.value);
    },
    [setSearch, onValueChange],
  );

  return (
    <div className="flex items-center border-b border-border px-3">
      <span className="mr-2 shrink-0 opacity-50">&#x1F50D;</span>
      <input
        className={cn(
          "flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        value={search}
        onChange={handleChange}
        {...props}
      />
    </div>
  );
}

function CommandList({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("max-h-[300px] overflow-y-auto overflow-x-hidden", className)}
      {...props}
    />
  );
}

function CommandEmpty({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("py-6 text-center text-sm", className)} {...props} />;
}

function CommandGroup({ className, heading, children, ...props }: React.HTMLAttributes<HTMLDivElement> & { heading?: string }) {
  return (
    <div className={cn("overflow-hidden p-1 text-foreground", className)} {...props}>
      {heading && (
        <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">{heading}</div>
      )}
      {children}
    </div>
  );
}

interface CommandItemProps extends React.HTMLAttributes<HTMLDivElement> {
  onSelect?: () => void;
  disabled?: boolean;
  value?: string;
}

function CommandItem({ className, onSelect, disabled, children, ...props }: CommandItemProps) {
  return (
    <div
      className={cn(
        "relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground",
        disabled && "pointer-events-none opacity-50",
        className,
      )}
      onClick={() => !disabled && onSelect?.()}
      role="option"
      aria-selected={false}
      aria-disabled={disabled}
      {...props}
    >
      {children}
    </div>
  );
}

function CommandSeparator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("-mx-1 h-px bg-border", className)} {...props} />;
}

export {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
};
