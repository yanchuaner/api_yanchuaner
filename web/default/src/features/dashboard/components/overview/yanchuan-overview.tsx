/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  FileText,
  KeyRound,
  RadioTower,
  Users,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatNumber, formatQuotaDualCurrency } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const USER_ACTIONS = [
  {
    title: 'Create an API key',
    description: 'Use one key in Yanchuan AI and future agent products.',
    to: '/keys',
    icon: KeyRound,
  },
  {
    title: 'Connect your application',
    description: 'Copy the OpenAI-compatible endpoint and request example.',
    to: '/docs',
    icon: BookOpen,
  },
  {
    title: 'Review usage details',
    description: 'Check each request, model, token count, and charge.',
    to: '/usage-logs',
    icon: FileText,
  },
] as const

export function YanchuanOverview() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const remainingQuota = Number(user?.quota ?? 0)
  const usedQuota = Number(user?.used_quota ?? 0)
  const requestCount = Number(user?.request_count ?? 0)
  const isAdmin = Boolean(user?.role && user.role >= ROLE.ADMIN)

  return (
    <div className='space-y-8'>
      <section>
        <p className='text-muted-foreground text-sm'>
          {t('Signed in as {{name}}', {
            name: user?.display_name || user?.username || t('Yanchuan member'),
          })}
        </p>
        <h2 className='mt-2 text-2xl font-semibold'>{t('Your API at a glance')}</h2>
      </section>

      <section className='divide-border bg-card grid overflow-hidden rounded-lg border sm:grid-cols-3 sm:divide-x'>
        <div className='border-border border-b p-5 sm:border-b-0'>
          <p className='text-muted-foreground text-sm'>{t('Credit remaining')}</p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {formatQuotaDualCurrency(remainingQuota)}
          </p>
        </div>
        <div className='border-border border-b p-5 sm:border-b-0'>
          <p className='text-muted-foreground text-sm'>{t('Credit used')}</p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {formatQuotaDualCurrency(usedQuota)}
          </p>
        </div>
        <div className='p-5'>
          <p className='text-muted-foreground text-sm'>{t('Requests')}</p>
          <p className='mt-2 text-xl font-semibold tabular-nums'>
            {formatNumber(requestCount)}
          </p>
        </div>
      </section>

      <section className='grid gap-3 lg:grid-cols-3'>
        {USER_ACTIONS.map((action) => {
          const Icon = action.icon
          return (
            <Link
              key={action.title}
              to={action.to}
              className='bg-card hover:bg-accent/50 group flex min-h-36 flex-col rounded-lg border p-5 transition-colors'
            >
              <Icon className='text-primary size-5' />
              <h3 className='mt-5 font-semibold'>{t(action.title)}</h3>
              <p className='text-muted-foreground mt-2 text-sm leading-6'>
                {t(action.description)}
              </p>
              <ArrowRight className='text-muted-foreground mt-auto size-4 self-end transition-transform group-hover:translate-x-1' />
            </Link>
          )
        })}
      </section>

      <section className='bg-muted/35 flex flex-col gap-4 rounded-lg border p-5 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <h3 className='font-semibold'>{t('Transparent billing')}</h3>
          <p className='text-muted-foreground mt-1 text-sm leading-6'>
            {t(
              'The fixed reference rate is $1.00 = ¥7.00. Model prices and every request charge remain visible in the console.'
            )}
          </p>
        </div>
        <Button variant='outline' render={<Link to='/usage-logs' />}>
          {t('View usage')}
        </Button>
      </section>

      {isAdmin && (
        <section className='border-border border-t pt-8'>
          <h3 className='font-semibold'>{t('Administration')}</h3>
          <div className='mt-4 flex flex-wrap gap-3'>
            <Button variant='outline' render={<Link to='/users' />}>
              <Users data-icon='inline-start' />
              {t('Users and allowances')}
            </Button>
            <Button variant='outline' render={<Link to='/channels' />}>
              <RadioTower data-icon='inline-start' />
              {t('Channels and routing')}
            </Button>
          </div>
        </section>
      )}
    </div>
  )
}
