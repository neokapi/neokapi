import { motion, AnimatePresence } from 'framer-motion';
import { X, CheckCircle2, XCircle, Clock } from 'lucide-react';
import type { AgentSession } from '../data/sessions';
import { agents, accentColorMap } from '../data/agents';

interface SessionDetailProps {
  session: AgentSession;
  onClose: () => void;
}

function formatDuration(secs: number): string {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-US", { weekday: "short", month: "short", day: "numeric" });
}

export default function SessionDetail({ session, onClose }: SessionDetailProps) {
  const agent = agents.find((a) => a.id === session.agentId);
  const color = agent ? accentColorMap[agent.accentColor] || "#f59e0b" : "#f59e0b";

  return (
    <AnimatePresence>
      <motion.div
        className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        onClick={onClose}
      >
        <motion.div
          className="max-h-[80vh] w-full max-w-lg overflow-y-auto rounded-xl border border-[var(--color-border)] bg-[var(--color-bg-primary)] p-6 shadow-xl"
          initial={{ opacity: 0, y: 20, scale: 0.95 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: 20, scale: 0.95 }}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div className="flex items-start justify-between">
            <div className="flex items-center gap-3">
              <span className="text-3xl">{agent?.avatar}</span>
              <div>
                <h3 className="text-lg font-semibold text-[var(--color-text-primary)]">
                  {session.agentName}
                </h3>
                <p className="text-sm text-[var(--color-text-secondary)]">
                  {agent?.role} &middot; {session.workspace}
                </p>
              </div>
            </div>
            <button
              onClick={onClose}
              className="rounded-lg p-1.5 text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-bg-elevated)] hover:text-[var(--color-text-primary)]"
            >
              <X size={18} />
            </button>
          </div>

          {/* Status + timing */}
          <div className="mt-4 flex flex-wrap items-center gap-3">
            {session.status === "succeeded" ? (
              <span className="flex items-center gap-1 rounded-full bg-green-500/15 px-2.5 py-1 text-xs font-medium text-green-400">
                <CheckCircle2 size={12} /> Succeeded
              </span>
            ) : session.status === "failed" ? (
              <span className="flex items-center gap-1 rounded-full bg-red-500/15 px-2.5 py-1 text-xs font-medium text-red-400">
                <XCircle size={12} /> Failed
              </span>
            ) : (
              <span className="flex items-center gap-1 rounded-full bg-amber-500/15 px-2.5 py-1 text-xs font-medium text-amber-400">
                <Clock size={12} /> Running
              </span>
            )}
            <span className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
              {formatDate(session.startTime)}
            </span>
            <span className="font-[family-name:var(--font-mono)] text-xs text-[var(--color-text-muted)]">
              {formatTime(session.startTime)} &mdash; {formatTime(session.endTime)}
            </span>
            <span className="font-[family-name:var(--font-mono)] text-xs font-semibold text-[var(--color-text-secondary)]">
              {formatDuration(session.durationSecs)}
            </span>
          </div>

          {/* Summary */}
          <div className="mt-4 rounded-lg bg-[var(--color-bg-elevated)] p-3">
            <p className="text-sm text-[var(--color-text-secondary)]">
              {session.summary}
            </p>
          </div>

          {/* Tool calls timeline */}
          <div className="mt-5">
            <h4 className="mb-3 text-sm font-semibold text-[var(--color-text-primary)]">
              Tool Calls ({session.toolCalls.length})
            </h4>
            <div className="space-y-0">
              {session.toolCalls.map((tc, i) => (
                <div key={i} className="flex items-start gap-3">
                  {/* Vertical line connector */}
                  <div className="flex flex-col items-center">
                    <div
                      className="h-3 w-3 rounded-full border-2"
                      style={{
                        borderColor: tc.success ? color : "#f43f5e",
                        backgroundColor: tc.success ? `${color}30` : "#f43f5e30",
                      }}
                    />
                    {i < session.toolCalls.length - 1 && (
                      <div className="h-6 w-px bg-[var(--color-border)]" />
                    )}
                  </div>

                  <div className="flex flex-1 items-center justify-between pb-2">
                    <div>
                      <span className="font-[family-name:var(--font-mono)] text-xs font-medium text-[var(--color-text-primary)]">
                        {tc.tool}
                      </span>
                      {!tc.success && (
                        <span className="ml-2 text-[10px] font-medium text-red-400">FAILED</span>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                        {tc.durationMs}ms
                      </span>
                      <span className="font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                        {formatTime(tc.timestamp)}
                      </span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  );
}
