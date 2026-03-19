import { motion } from 'framer-motion';
import { ArrowRight } from 'lucide-react';
import { useFilter } from '../context/FilterContext';

const steps = [
  { avatar: "\u{1F6E0}\u{FE0F}", role: "L10N Engineer", action: "pull & push", color: "#f59e0b", active: false },
  { avatar: "\u{1F1EB}\u{1F1F7}", role: "Language Experts", action: "translate", color: "#3b82f6", active: true },
  { avatar: "\u{1F50D}", role: "Reviewer", action: "validate", color: "#14b8a6", active: true },
  { avatar: "\u{1F6E0}\u{FE0F}", role: "L10N Engineer", action: "deploy", color: "#f59e0b", active: false },
];

export default function HandoffChain() {
  const { selectedWorkspace } = useFilter();

  // Only show for Excalidraw workspace or "all"
  if (selectedWorkspace && selectedWorkspace !== "excalidraw") return null;

  return (
    <motion.section
      className="px-6 py-12"
      initial={{ opacity: 0 }}
      whileInView={{ opacity: 1 }}
      viewport={{ once: true }}
      transition={{ duration: 0.6 }}
    >
      <div className="mx-auto max-w-5xl">
        <h2 className="mb-8 text-center font-[family-name:var(--font-display)] text-2xl font-bold text-[var(--color-text-primary)]">
          Agent Handoff Pipeline
        </h2>
        <div className="flex items-center justify-center gap-2 overflow-x-auto pb-4 md:gap-4">
          {steps.map((step, i) => (
            <div key={i} className="flex items-center gap-2 md:gap-4">
              <motion.div
                className="flex flex-col items-center gap-2"
                initial={{ opacity: 0, y: 20 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.4, delay: i * 0.1 }}
              >
                <div
                  className="flex h-14 w-14 items-center justify-center rounded-xl border text-2xl md:h-16 md:w-16"
                  style={{
                    borderColor: step.active ? step.color : 'var(--color-border)',
                    backgroundColor: step.active ? `${step.color}15` : 'var(--color-bg-card)',
                    boxShadow: step.active ? `0 0 20px ${step.color}30` : 'none',
                  }}
                >
                  {step.avatar}
                </div>
                <div className="text-center">
                  <div className="text-xs font-medium text-[var(--color-text-primary)]">
                    {step.role}
                  </div>
                  <div className="font-[family-name:var(--font-mono)] text-[10px] text-[var(--color-text-muted)]">
                    {step.action}
                  </div>
                </div>
              </motion.div>
              {i < steps.length - 1 && (
                <div className="relative flex items-center">
                  <ArrowRight size={16} className="text-[var(--color-text-muted)]" />
                  <motion.div
                    className="absolute left-0 h-1 w-1 rounded-full"
                    style={{ backgroundColor: steps[i + 1].active || step.active ? step.color : 'var(--color-text-muted)' }}
                    animate={{ x: [0, 16, 0] }}
                    transition={{ duration: 1.5, repeat: Infinity, delay: i * 0.3 }}
                  />
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </motion.section>
  );
}
