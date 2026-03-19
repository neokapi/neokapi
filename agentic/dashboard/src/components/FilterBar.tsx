import { useState, useRef, useMemo, useCallback, useEffect } from 'react';
import { Search, X, ExternalLink, ChevronDown, ListFilter } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command';
import { useFilter, type FilterToken } from '@/context/FilterContext';
import { workspaces } from '@/data/workspaces';
import { agents } from '@/data/agents';

interface FilterValue {
  value: string;
  label: string;
}

interface FilterField {
  key: string;
  label: string;
  hint: string;
  values: FilterValue[];
}

const staticStatusValues: FilterValue[] = [
  { value: 'succeeded', label: 'Succeeded' },
  { value: 'failed', label: 'Failed' },
  { value: 'running', label: 'Running' },
];

const timeValues: FilterValue[] = [
  { value: 'today', label: 'Today' },
  { value: 'yesterday', label: 'Yesterday' },
  { value: 'this-week', label: 'This week' },
  { value: 'this-month', label: 'This month' },
  { value: '7d', label: 'Last 7 days' },
  { value: '14d', label: 'Last 14 days' },
  { value: '30d', label: 'Last 30 days' },
];

const toolValues: FilterValue[] = [
  { value: 'connector_pull', label: 'connector_pull' },
  { value: 'connector_push', label: 'connector_push' },
  { value: 'update_block', label: 'update_block' },
  { value: 'list_blocks', label: 'list_blocks' },
  { value: 'tm_search', label: 'tm_search' },
  { value: 'run_flow', label: 'run_flow' },
  { value: 'check_vocabulary', label: 'check_vocabulary' },
  { value: 'term_search', label: 'term_search' },
];

const presets: { label: string; tokens: FilterToken[] }[] = [
  { label: "Today's activity", tokens: [{ key: 'time', value: 'today', label: 'Today' }] },
  { label: 'This week', tokens: [{ key: 'time', value: 'this-week', label: 'This week' }] },
  { label: 'Failed jobs', tokens: [{ key: 'status', value: 'failed', label: 'Failed' }] },
  { label: 'Active sessions', tokens: [{ key: 'status', value: 'running', label: 'Running' }] },
];

type DropdownMode = 'fields' | 'values';

export default function FilterBar() {
  const {
    tokens,
    addToken,
    removeToken,
    clearTokens,
    search,
    setSearch,
    workspace,
  } = useFilter();

  const [presetsOpen, setPresetsOpen] = useState(false);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [dropdownMode, setDropdownMode] = useState<DropdownMode>('fields');
  const [activeFieldKey, setActiveFieldKey] = useState<string | null>(null);
  const [inputValue, setInputValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // Build filter fields dynamically based on current tokens
  const filterFields: FilterField[] = useMemo(() => {
    const filteredAgents = workspace
      ? agents.filter((a) => a.workspace === workspace)
      : agents;
    const uniqueAgents = Array.from(
      new Map(filteredAgents.map((a) => [a.id, a])).values()
    );

    return [
      {
        key: 'workspace',
        label: 'workspace',
        hint: 'filter by workspace',
        values: workspaces.map((w) => ({ value: w.id, label: w.name })),
      },
      {
        key: 'agent',
        label: 'agent',
        hint: 'filter by agent',
        values: uniqueAgents.map((a) => ({ value: a.id, label: a.name })),
      },
      {
        key: 'status',
        label: 'status',
        hint: 'filter by job status',
        values: staticStatusValues,
      },
      {
        key: 'time',
        label: 'time',
        hint: 'filter by time range',
        values: timeValues,
      },
      {
        key: 'tool',
        label: 'tool',
        hint: 'filter by MCP tool used',
        values: toolValues,
      },
    ];
  }, [workspace]);

  const activeField = useMemo(
    () => filterFields.find((f) => f.key === activeFieldKey) ?? null,
    [filterFields, activeFieldKey]
  );

  // Check if input text matches a filter key followed by colon
  const checkForFilterKey = useCallback(
    (value: string) => {
      const match = value.match(/^(\w+):$/);
      if (match) {
        const field = filterFields.find((f) => f.key === match[1]);
        if (field) {
          setActiveFieldKey(field.key);
          setDropdownMode('values');
          setInputValue('');
          setDropdownOpen(true);
          return true;
        }
      }
      return false;
    },
    [filterFields]
  );

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      if (!checkForFilterKey(value)) {
        setInputValue(value);
        // Show fields dropdown when user starts typing
        if (value && !dropdownOpen) {
          setDropdownMode('fields');
          setDropdownOpen(true);
        }
      }
    },
    [checkForFilterKey, dropdownOpen]
  );

  const handleInputFocus = useCallback(() => {
    if (!dropdownOpen) {
      setDropdownMode('fields');
      setActiveFieldKey(null);
      setDropdownOpen(true);
    }
  }, [dropdownOpen]);

  const handleInputKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      // Backspace on empty input removes last token
      if (e.key === 'Backspace' && !inputValue && tokens.length > 0) {
        removeToken(tokens.length - 1);
        return;
      }
      // Enter with free text sets search
      if (e.key === 'Enter' && inputValue && dropdownMode === 'fields') {
        // Check if it looks like a filter key
        const colonIdx = inputValue.indexOf(':');
        if (colonIdx === -1) {
          setSearch(inputValue);
          setInputValue('');
          setDropdownOpen(false);
        }
      }
      // Escape closes dropdown
      if (e.key === 'Escape') {
        setDropdownOpen(false);
        setActiveFieldKey(null);
        inputRef.current?.blur();
      }
    },
    [inputValue, tokens.length, removeToken, dropdownMode, setSearch]
  );

  const selectField = useCallback((key: string) => {
    setActiveFieldKey(key);
    setDropdownMode('values');
    setInputValue('');
    // Keep focus on input for keyboard navigation
    inputRef.current?.focus();
  }, []);

  const selectValue = useCallback(
    (field: FilterField, val: FilterValue) => {
      addToken({ key: field.key, value: val.value, label: val.label });
      setActiveFieldKey(null);
      setDropdownMode('fields');
      setInputValue('');
      setDropdownOpen(false);
      inputRef.current?.focus();
    },
    [addToken]
  );

  const handlePreset = useCallback(
    (preset: { label: string; tokens: FilterToken[] }) => {
      for (const t of preset.tokens) {
        addToken(t);
      }
      setPresetsOpen(false);
    },
    [addToken]
  );

  // Clear search token
  const clearSearch = useCallback(() => {
    setSearch('');
  }, [setSearch]);

  // Close dropdown on outside click
  useEffect(() => {
    if (!dropdownOpen) {
      setActiveFieldKey(null);
    }
  }, [dropdownOpen]);

  const activeWs = workspaces.find((w) => w.id === workspace);
  const bowrainUrl = activeWs
    ? `https://dev.bowrain.cloud/${activeWs.slug}`
    : null;

  return (
    <div className="px-4 py-3 sm:px-6">
      <div className="flex items-center gap-2">
        {/* Filters preset button */}
        <Popover open={presetsOpen} onOpenChange={setPresetsOpen}>
          <PopoverTrigger
            render={
              <Button variant="outline" size="sm" className="gap-1.5 shrink-0" />
            }
          >
            <ListFilter className="h-3.5 w-3.5" />
            Filters
            <ChevronDown className="h-3 w-3 text-muted-foreground" />
          </PopoverTrigger>
          <PopoverContent align="start" className="w-56 p-1">
            <Command>
              <CommandList>
                <CommandGroup heading="Quick filters">
                  {presets.map((preset) => (
                    <CommandItem
                      key={preset.label}
                      onSelect={() => handlePreset(preset)}
                      className="text-xs cursor-pointer"
                    >
                      {preset.label}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>

        {/* Unified search/filter input */}
        <div className="relative flex-1 min-w-[180px]">
          <div className="flex items-center gap-1 rounded-lg border border-input bg-transparent px-2 focus-within:border-ring focus-within:ring-1 focus-within:ring-ring/50 min-h-[32px]">
            <Search className="h-3.5 w-3.5 text-muted-foreground shrink-0" />

            {/* Active filter tokens inline */}
            {tokens.map((token, i) => (
              <Badge
                key={`${token.key}-${token.value}`}
                variant="secondary"
                className="gap-0.5 pl-1.5 pr-0.5 py-0 text-[11px] font-mono shrink-0 h-5"
              >
                <span className="text-muted-foreground">{token.key}:</span>
                <span>{token.label}</span>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    removeToken(i);
                  }}
                  className="ml-0.5 p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </Badge>
            ))}

            {/* Free text search token */}
            {search && (
              <Badge
                variant="secondary"
                className="gap-0.5 pl-1.5 pr-0.5 py-0 text-[11px] font-mono shrink-0 h-5"
              >
                <span className="text-muted-foreground">search:</span>
                <span>{search}</span>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    clearSearch();
                  }}
                  className="ml-0.5 p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
                >
                  <X className="w-3 h-3" />
                </button>
              </Badge>
            )}

            <input
              ref={inputRef}
              type="text"
              value={inputValue}
              onChange={handleInputChange}
              onFocus={handleInputFocus}
              onKeyDown={handleInputKeyDown}
              placeholder={
                tokens.length === 0 && !search
                  ? 'Filter by workspace, agent, status, time...'
                  : ''
              }
              className="flex-1 min-w-[120px] h-7 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />

            {(tokens.length > 0 || search) && (
              <button
                onClick={clearTokens}
                className="p-0.5 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors shrink-0"
                title="Clear all filters"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>

          {/* Autocomplete dropdown */}
          {dropdownOpen && (
            <div className="absolute left-0 top-full z-50 mt-1 w-full rounded-lg border bg-popover shadow-md">
              <Command
                shouldFilter={dropdownMode === 'fields'}
                value={inputValue}
              >
                <CommandList>
                  {dropdownMode === 'fields' && (
                    <>
                      <CommandEmpty className="py-3 text-xs text-center text-muted-foreground">
                        Press Enter to search for &quot;{inputValue}&quot;
                      </CommandEmpty>
                      <CommandGroup heading="Available filters">
                        {filterFields.map((field) => (
                          <CommandItem
                            key={field.key}
                            value={field.key}
                            onSelect={() => selectField(field.key)}
                            className="cursor-pointer"
                          >
                            <span className="font-mono text-xs font-medium">
                              {field.label}:
                            </span>
                            <span className="ml-2 text-xs text-muted-foreground">
                              {field.hint}
                            </span>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                    </>
                  )}

                  {dropdownMode === 'values' && activeField && (
                    <>
                      <CommandEmpty className="py-3 text-xs text-center text-muted-foreground">
                        No matching values.
                      </CommandEmpty>
                      <CommandGroup
                        heading={`${activeField.label} values`}
                      >
                        {activeField.values.map((val) => (
                          <CommandItem
                            key={val.value}
                            value={val.value}
                            keywords={[val.label]}
                            onSelect={() => selectValue(activeField, val)}
                            className="cursor-pointer"
                            data-checked={
                              tokens.some(
                                (t) =>
                                  t.key === activeField.key &&
                                  t.value === val.value
                              )
                                ? 'true'
                                : undefined
                            }
                          >
                            <span className="text-xs">{val.label}</span>
                            <span className="ml-auto text-[10px] text-muted-foreground font-mono">
                              {val.value}
                            </span>
                          </CommandItem>
                        ))}
                      </CommandGroup>
                      <CommandSeparator />
                      <CommandGroup>
                        <CommandItem
                          value="__back__"
                          onSelect={() => {
                            setActiveFieldKey(null);
                            setDropdownMode('fields');
                          }}
                          className="cursor-pointer text-xs text-muted-foreground"
                        >
                          Back to filters
                        </CommandItem>
                      </CommandGroup>
                    </>
                  )}
                </CommandList>
              </Command>
            </div>
          )}

          {/* Invisible overlay to close dropdown on outside click */}
          {dropdownOpen && (
            <div
              className="fixed inset-0 z-40"
              onClick={() => setDropdownOpen(false)}
            />
          )}
        </div>

        {/* View in Bowrain link */}
        {bowrainUrl && (
          <Button
            variant="ghost"
            size="sm"
            render={
              <a
                href={bowrainUrl}
                target="_blank"
                rel="noopener noreferrer"
              />
            }
            className="ml-auto gap-1.5 text-muted-foreground hover:text-foreground shrink-0"
          >
            View in Bowrain
            <ExternalLink className="h-3.5 w-3.5" />
          </Button>
        )}
      </div>
    </div>
  );
}
