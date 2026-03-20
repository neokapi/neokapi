import { useState, useRef, useEffect, useMemo } from 'react';
import { Search, X, ExternalLink, Filter, Clock, AlertTriangle, Zap, Activity } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Separator } from '@/components/ui/separator';
import { useFilter, type FilterToken } from '@/context/FilterContext';
import { useApi } from '@/context/ApiContext';

// -- Filter definitions --

const filterKeys = [
  { key: 'workspace', description: 'Filter by workspace', icon: '{}' },
  { key: 'agent', description: 'Filter by agent name', icon: '@' },
  { key: 'status', description: 'succeeded, failed, running', icon: '!' },
  { key: 'time', description: 'today, yesterday, this-week, 7d, 14d, 30d', icon: '#' },
  { key: 'tool', description: 'event type filter', icon: '>' },
] as const;

const presets: { id: string; label: string; icon: React.ReactNode; tokens: FilterToken[] }[] = [
  {
    id: 'today',
    label: "Today's activity",
    icon: <Clock className="h-3.5 w-3.5" />,
    tokens: [{ key: 'time', value: 'today', label: 'today' }],
  },
  {
    id: 'this-week',
    label: 'This week',
    icon: <Activity className="h-3.5 w-3.5" />,
    tokens: [{ key: 'time', value: 'this-week', label: 'this-week' }],
  },
  {
    id: 'failed',
    label: 'Failed jobs',
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
    tokens: [{ key: 'status', value: 'failed', label: 'failed' }],
  },
  {
    id: 'active',
    label: 'Active sessions',
    icon: <Zap className="h-3.5 w-3.5" />,
    tokens: [{ key: 'status', value: 'running', label: 'running' }],
  },
];

const statusValues = ['succeeded', 'failed', 'running'];
const timeValues = ['today', 'yesterday', 'this-week', 'this-month', '7d', '14d', '30d'];

export default function FilterBar() {
  const { tokens, addToken, removeToken, clearTokens, search, setSearch } = useFilter();
  const api = useApi();
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [inputFocused, setInputFocused] = useState(false);
  const [hintDropdownOpen, setHintDropdownOpen] = useState(false);
  const [inputValue, setInputValue] = useState('');
  const [activeKey, setActiveKey] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Derive event types from audit log
  const eventTypeValues = useMemo(() => {
    const types = new Set<string>();
    for (const entry of api.auditLog) {
      types.add(entry.event_type);
    }
    return [...types].sort();
  }, [api.auditLog]);

  // Derive bowrain URL from workspace token
  const wsToken = tokens.find((t) => t.key === 'workspace');
  const bowrainUrl = wsToken
    ? `https://dev.bowrain.cloud/${wsToken.value}`
    : 'https://dev.bowrain.cloud';

  // Close hint dropdown on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setHintDropdownOpen(false);
        setActiveKey(null);
      }
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, []);

  // Get autocomplete values for the active key
  function getValuesForKey(key: string): { value: string; label: string }[] {
    switch (key) {
      case 'workspace':
        return api.workspaces.map((w) => ({ value: w.slug, label: w.name }));
      case 'agent':
        return api.agents.map((a) => ({ value: a.id, label: a.displayName }));
      case 'status':
        return statusValues.map((v) => ({ value: v, label: v }));
      case 'time':
        return timeValues.map((v) => ({ value: v, label: v }));
      case 'tool':
        return eventTypeValues.map((v) => ({ value: v, label: v }));
      default:
        return [];
    }
  }

  function handleInputChange(val: string) {
    setInputValue(val);

    // Check if typing a filter key like "workspace:"
    const colonIndex = val.indexOf(':');
    if (colonIndex > 0) {
      const key = val.slice(0, colonIndex).trim();
      const rest = val.slice(colonIndex + 1).trim();
      const matchedKey = filterKeys.find((fk) => fk.key === key);
      if (matchedKey) {
        setActiveKey(key);
        if (rest) {
          // Check if there's an exact match to add as token
          const values = getValuesForKey(key);
          const exact = values.find(
            (v) => v.value.toLowerCase() === rest.toLowerCase() || v.label.toLowerCase() === rest.toLowerCase()
          );
          if (exact) {
            addToken({ key, value: exact.value, label: exact.label });
            setInputValue('');
            setActiveKey(null);
            setHintDropdownOpen(false);
            return;
          }
        }
        setHintDropdownOpen(true);
        return;
      }
    }

    if (!activeKey) {
      setSearch(val);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Backspace' && inputValue === '' && tokens.length > 0) {
      removeToken(tokens.length - 1);
    }
    if (e.key === 'Escape') {
      setHintDropdownOpen(false);
      setActiveKey(null);
      inputRef.current?.blur();
    }
  }

  function handleFocus() {
    setInputFocused(true);
    if (inputValue === '' && !activeKey) {
      setHintDropdownOpen(true);
    }
  }

  function handleBlur() {
    setInputFocused(false);
    // Delay to allow click events on dropdown
    setTimeout(() => {
      if (!containerRef.current?.contains(document.activeElement)) {
        setHintDropdownOpen(false);
      }
    }, 200);
  }

  function selectFilterKey(key: string) {
    setActiveKey(key);
    setInputValue(`${key}:`);
    setHintDropdownOpen(true);
    inputRef.current?.focus();
  }

  function selectValue(key: string, value: string, label: string) {
    addToken({ key, value, label });
    setInputValue('');
    setActiveKey(null);
    setHintDropdownOpen(false);
    inputRef.current?.focus();
  }

  function handlePreset(preset: { tokens: FilterToken[] }) {
    clearTokens();
    for (const t of preset.tokens) {
      addToken(t);
    }
    setPopoverOpen(false);
  }

  // Values filtered by current input
  const autocompleteValues = activeKey
    ? getValuesForKey(activeKey).filter((v) => {
        const query = inputValue.includes(':')
          ? inputValue.slice(inputValue.indexOf(':') + 1).trim().toLowerCase()
          : '';
        if (!query) return true;
        return v.value.toLowerCase().includes(query) || v.label.toLowerCase().includes(query);
      })
    : [];

  return (
    <div className="space-y-0 px-4 py-3 sm:px-6" ref={containerRef}>
      {/* Single row: Filters button + unified search input + Bowrain link */}
      <div className="flex items-center gap-2">
        {/* Filters popover */}
        <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
          <PopoverTrigger
            render={
              <Button variant="outline" size="sm" className="gap-1.5 shrink-0">
                <Filter className="h-3.5 w-3.5" />
                Filters
              </Button>
            }
          />
          <PopoverContent align="start" className="w-64 p-0">
            {/* Presets */}
            <div className="p-2">
              <p className="px-2 pb-1.5 text-xs font-medium text-muted-foreground">Quick filters</p>
              {presets.map((p) => (
                <button
                  key={p.id}
                  onClick={() => handlePreset(p)}
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm bg-transparent border-none cursor-pointer hover:bg-accent hover:text-accent-foreground transition-colors text-left"
                >
                  {p.icon}
                  {p.label}
                </button>
              ))}
            </div>
            <Separator />
            {/* Filter keys */}
            <div className="p-2">
              <p className="px-2 pb-1.5 text-xs font-medium text-muted-foreground">Available filters</p>
              {filterKeys.map((fk) => (
                <button
                  key={fk.key}
                  onClick={() => {
                    selectFilterKey(fk.key);
                    setPopoverOpen(false);
                  }}
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm bg-transparent border-none cursor-pointer hover:bg-accent hover:text-accent-foreground transition-colors text-left"
                >
                  <span className="w-5 text-center font-mono text-xs text-muted-foreground">{fk.icon}</span>
                  <span className="font-medium">{fk.key}</span>
                  <span className="ml-auto text-xs text-muted-foreground truncate max-w-[120px]">{fk.description}</span>
                </button>
              ))}
            </div>
          </PopoverContent>
        </Popover>

        {/* Unified search input with tokens */}
        <div className="relative flex-1 min-w-[200px]">
          <div
            className={`flex flex-wrap items-center gap-1 rounded-lg border bg-transparent px-2 py-1 min-h-[32px] transition-colors ${
              inputFocused ? 'border-ring ring-1 ring-ring/50' : 'border-input'
            }`}
          >
            <Search className="h-3.5 w-3.5 text-muted-foreground shrink-0" />

            {/* Active filter tokens as badges inside the input */}
            {tokens.map((token, i) => (
              <Badge
                key={`${token.key}-${i}`}
                variant="secondary"
                className="gap-1 pl-1.5 pr-1 py-0 text-[11px] font-mono shrink-0"
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

            <input
              ref={inputRef}
              type="text"
              value={inputValue}
              onChange={(e) => handleInputChange(e.target.value)}
              onKeyDown={handleKeyDown}
              onFocus={handleFocus}
              onBlur={handleBlur}
              placeholder={tokens.length > 0 ? 'Add filter...' : 'Filter by workspace, agent, status...'}
              className="flex-1 min-w-[120px] h-6 bg-transparent text-sm outline-none placeholder:text-muted-foreground border-none"
            />

            {(tokens.length > 0 || search) && (
              <button
                onClick={() => {
                  clearTokens();
                  setInputValue('');
                  setActiveKey(null);
                }}
                className="p-0.5 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors shrink-0"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>

          {/* Hint dropdown */}
          {hintDropdownOpen && (
            <div className="absolute top-full left-0 right-0 z-50 mt-1 rounded-lg border bg-popover p-1 shadow-md ring-1 ring-foreground/10">
              {!activeKey ? (
                // Show filter key hints
                <div>
                  <p className="px-2 py-1 text-xs text-muted-foreground">Filter by</p>
                  {filterKeys.map((fk) => (
                    <button
                      key={fk.key}
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => selectFilterKey(fk.key)}
                      className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm bg-transparent border-none cursor-pointer hover:bg-accent hover:text-accent-foreground transition-colors text-left"
                    >
                      <span className="w-5 text-center font-mono text-xs text-muted-foreground">{fk.icon}</span>
                      <span className="font-mono font-medium">{fk.key}:</span>
                      <span className="text-xs text-muted-foreground">{fk.description}</span>
                    </button>
                  ))}
                </div>
              ) : (
                // Show values for active key
                <div>
                  <p className="px-2 py-1 text-xs text-muted-foreground">{activeKey} values</p>
                  {autocompleteValues.length === 0 ? (
                    <p className="px-2 py-1.5 text-xs text-muted-foreground">No matches</p>
                  ) : (
                    autocompleteValues.map((v) => (
                      <button
                        key={v.value}
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={() => selectValue(activeKey, v.value, v.label)}
                        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm bg-transparent border-none cursor-pointer hover:bg-accent hover:text-accent-foreground transition-colors text-left"
                      >
                        <span className="font-mono">{v.label}</span>
                        {v.value !== v.label && (
                          <span className="text-xs text-muted-foreground">{v.value}</span>
                        )}
                      </button>
                    ))
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Bowrain link */}
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
      </div>
    </div>
  );
}
