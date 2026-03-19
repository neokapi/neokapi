import { motion } from 'framer-motion';
import { ArrowRight, ArrowDown } from 'lucide-react';
import { useFilter } from '../context/FilterContext';

const steps = [
  { avatar: '\u{1F6E0}\u{FE0F}', role: 'L10N Engineer', action: 'pull & push', color: 'rgb(var(--agent-amber))', active: false },
  { avatar: '\u{1F1EB}\u{1F1F7}', role: 'Language Experts', action: 'translate', color: 'rgb(var(--agent-blue))', active: true },
  { avatar: '\u{1F50D}', role: 'Reviewer', action: 'validate', color: 'rgb(var(--agent-teal))', active: true },
  { avatar: '\u{1F6E0}\u{FE0F}', role: 'L10N Engineer', action: 'deploy', color: 'rgb(var(--agent-amber))', active: false },
];

export default function HandoffChain() {
  const { selectedWorkspace } = useFilter();

  if (selectedWorkspace && selectedWorkspace !== 'excalidraw') return null;

  return (
    <motion.section
      className="px-4 py-16 sm:px-6"
      initial={{ opacity: 0 }}
      whileInView={{ opacity: 1 }}
      viewport={{ once: true }}
      transition={{ duration: 0.6 }}
    >
      <div className="mx-auto max-w-5xl">
        <h2
          className="font-display mb-8 text-center text-xl font-normal sm:text-2xl"
          style={{ color: 'rgb(var(--text-primary))' }}
        >
          Agent Handoff Pipeline
        </h2>

        {/* Desktop: horizontal */}
        <div className="hidden items-center justify-center gap-4 sm:flex">
          {steps.map((step, i) => (
            <div key={i} className="flex items-center gap-4">
              <motion.div
                className="flex flex-col items-center gap-2"
                initial={{ opacity: 0, y: 20 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.4, delay: i * 0.1 }}
              >
                <div
                  className="flex h-16 w-16 items-center justify-center rounded-full text-2xl"
                  style={{
                    borderWidth: '2px',
                    borderStyle: 'solid',
                    borderColor: step.active ? step.color : 'rgb(var(--border))',
                    backgroundColor: step.active
                      ? `color-mix(in srgb, ${step.color} 15%, transparent)`
                      : 'rgb(var(--bg-card))',
                    boxShadow: step.active
                      ? `0 0 20px color-mix(in srgb, ${step.color} 30%, transparent)`
                      : 'none',
                  }}
                >
                  {step.avatar}
                </div>
                <div className="text-center">
                  <div
                    className="text-xs font-medium"
                    style={{ color: 'rgb(var(--text-primary))' }}
                  >
                    {step.role}
                  </div>
                  <div
                    className="font-mono text-[10px]"
                    style={{ color: 'rgb(var(--text-muted))' }}
                  >
                    {step.action}
                  </div>
                </div>
              </motion.div>
              {i < steps.length - 1 && (
                <div className="relative flex items-center">
                  <div
                    className="h-px w-8"
                    style={{ backgroundColor: 'rgb(var(--border))' }}
                  />
                  <ArrowRight
                    size={16}
                    style={{ color: 'rgb(var(--text-muted))' }}
                  />
                  {/* Traveling dot */}
                  <div
                    className="animate-travel-dot absolute left-0 h-1.5 w-1.5 rounded-full"
                    style={{
                      backgroundColor:
                        steps[i + 1].active || step.active
                          ? step.color
                          : 'rgb(var(--text-muted))',
                      animationDelay: `${i * 0.5}s`,
                    }}
                  />
                </div>
              )}
            </div>
          ))}
        </div>

        {/* Mobile: vertical */}
        <div className="flex flex-col items-center gap-3 sm:hidden">
          {steps.map((step, i) => (
            <div key={i} className="flex flex-col items-center">
              <motion.div
                className="flex items-center gap-3"
                initial={{ opacity: 0, y: 20 }}
                whileInView={{ opacity: 1, y: 0 }}
                viewport={{ once: true }}
                transition={{ duration: 0.4, delay: i * 0.1 }}
              >
                <div
                  className="flex h-14 w-14 items-center justify-center rounded-full text-xl"
                  style={{
                    borderWidth: '2px',
                    borderStyle: 'solid',
                    borderColor: step.active ? step.color : 'rgb(var(--border))',
                    backgroundColor: step.active
                      ? `color-mix(in srgb, ${step.color} 15%, transparent)`
                      : 'rgb(var(--bg-card))',
                  }}
                >
                  {step.avatar}
                </div>
                <div>
                  <div
                    className="text-sm font-medium"
                    style={{ color: 'rgb(var(--text-primary))' }}
                  >
                    {step.role}
                  </div>
                  <div
                    className="font-mono text-xs"
                    style={{ color: 'rgb(var(--text-muted))' }}
                  >
                    {step.action}
                  </div>
                </div>
              </motion.div>
              {i < steps.length - 1 && (
                <div className="flex h-6 items-center">
                  <ArrowDown
                    size={16}
                    style={{ color: 'rgb(var(--text-muted))' }}
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
