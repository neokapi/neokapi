import { useState } from 'react'
import { Check, Zap, Users, Building2 } from 'lucide-react'

type Billing = 'monthly' | 'annual'

const TIERS = [
  {
    id: 'starter',
    name: 'Starter',
    icon: Zap,
    description: 'For an individual or small team moving local work onto the platform.',
    monthly: '$XX',
    annual: '$XX',
    annualNote: '/mo, billed annually',
    cta: 'Start free trial',
    ctaStyle: 'border border-neutral-700 bg-neutral-900/50 text-neutral-200 hover:border-neutral-500 hover:text-white',
    features: [
      '1 workspace, 3 projects',
      'All formats and workflow tools',
      'AI translation (bring your own key)',
      'Translation memory',
      'Pseudo-localization',
      'Community support',
    ],
  },
  {
    id: 'team',
    name: 'Team',
    icon: Users,
    description: 'For teams that need collaboration, connectors, and automation.',
    monthly: '$XX',
    annual: '$XX',
    annualNote: '/mo, billed annually',
    cta: 'Start free trial',
    ctaStyle: 'bg-brand-500 text-white hover:bg-brand-600',
    featured: true,
    features: [
      'Everything in Starter, plus:',
      'Unlimited workspaces & projects',
      'Live connectors (CMS, Git, Figma)',
      'Automated workflows',
      'Visual translation editor',
      'Shared glossaries & translation memory',
      'Team collaboration & review',
      'Priority support',
    ],
  },
  {
    id: 'enterprise',
    name: 'Enterprise',
    icon: Building2,
    description: 'For organizations with custom integration and compliance needs.',
    monthly: 'Custom',
    annual: 'Custom',
    annualNote: '',
    cta: 'Talk to us',
    ctaStyle: 'border border-neutral-700 bg-neutral-900/50 text-neutral-200 hover:border-neutral-500 hover:text-white',
    features: [
      'Everything in Team, plus:',
      'Custom AI providers & models',
      'Per-locale cultural adaptations',
      'CMS/DAM enterprise connectors',
      'SSO & audit trails',
      'On-premise deployment option',
      'Dedicated support & SLA',
    ],
  },
]

export function Plans() {
  const [billing, setBilling] = useState<Billing>('annual')

  return (
    <section id="plans" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          Plans
        </h2>
        <p className="mt-3 text-neutral-400">
          The open-source <code className="rounded bg-neutral-800/50 px-1.5 py-0.5 text-xs text-neutral-300">kapi</code> toolchain is free and runs anywhere.
          Add Bowrain when a team needs shared governance, connectors, collaboration, and version history on the server.
        </p>

        <div className="mt-8 inline-flex items-center rounded-full border border-neutral-800 bg-neutral-900/50 p-1">
          <button
            onClick={() => setBilling('monthly')}
            className={`rounded-full px-5 py-1.5 text-sm font-medium transition ${
              billing === 'monthly'
                ? 'bg-neutral-800 text-white shadow-sm'
                : 'text-neutral-500 hover:text-neutral-300'
            }`}
          >
            Monthly
          </button>
          <button
            onClick={() => setBilling('annual')}
            className={`rounded-full px-5 py-1.5 text-sm font-medium transition ${
              billing === 'annual'
                ? 'bg-neutral-800 text-white shadow-sm'
                : 'text-neutral-500 hover:text-neutral-300'
            }`}
          >
            Annual
            <span className="ml-1.5 rounded-full bg-suggestion/10 px-2 py-0.5 text-xs text-suggestion">Save 20%</span>
          </button>
        </div>
      </div>

      <div className="mt-12 grid gap-6 lg:grid-cols-3">
        {TIERS.map(tier => {
          const Icon = tier.icon
          const price = billing === 'monthly' ? tier.monthly : tier.annual
          const isCustom = price === 'Custom'

          return (
            <div
              key={tier.id}
              className={`relative flex flex-col rounded-xl border p-8 ${
                tier.featured
                  ? 'border-brand-500/50 bg-brand-500/5 shadow-lg shadow-brand-500/5'
                  : 'border-neutral-800 bg-neutral-900/30'
              }`}
            >
              {tier.featured && (
                <div className="absolute -top-3 left-1/2 -translate-x-1/2 rounded-full bg-brand-500 px-3 py-0.5 text-xs font-medium text-white">
                  Most popular
                </div>
              )}

              <div className="mb-4 flex items-center gap-3">
                <div className={`flex h-10 w-10 items-center justify-center rounded-lg ${
                  tier.featured ? 'bg-brand-500/20' : 'bg-neutral-800'
                }`}>
                  <Icon className={`h-5 w-5 ${tier.featured ? 'text-brand-400' : 'text-neutral-400'}`} />
                </div>
                <h3 className="text-xl font-semibold text-white">{tier.name}</h3>
              </div>

              <p className="text-sm text-neutral-400">{tier.description}</p>

              <div className="mt-6 mb-6">
                <div className="flex items-baseline gap-1">
                  <span className="text-4xl font-bold text-white">{price}</span>
                  {!isCustom && (
                    <span className="text-sm text-neutral-500">
                      {billing === 'monthly' ? '/mo' : tier.annualNote}
                    </span>
                  )}
                </div>
              </div>

              <a
                href={tier.id === 'enterprise' ? 'mailto:hello@bowrain.com' : '#get-started'}
                className={`mb-8 flex items-center justify-center rounded-xl px-6 py-3 text-sm font-medium transition ${tier.ctaStyle}`}
              >
                {tier.cta}
              </a>

              <ul className="flex-1 space-y-3">
                {tier.features.map((feature, i) => {
                  const isHeader = feature.endsWith(':')
                  return (
                    <li key={i} className={`flex items-start gap-2 text-sm ${isHeader ? 'text-neutral-300 font-medium' : 'text-neutral-400'}`}>
                      {!isHeader && <Check className={`mt-0.5 h-4 w-4 shrink-0 ${tier.featured ? 'text-brand-400' : 'text-neutral-600'}`} />}
                      {feature}
                    </li>
                  )
                })}
              </ul>
            </div>
          )
        })}
      </div>

      <p className="mt-8 text-center text-sm text-neutral-600">
        The open-source <code className="rounded bg-neutral-800/50 px-1.5 py-0.5 text-xs text-neutral-400">kapi</code> toolchain is free — formats, workflow tools, and AI translation, on your own machine. No account required.
      </p>
    </section>
  )
}
