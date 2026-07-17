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
import { ArrowRight, BadgeDollarSign, KeyRound, Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'

const CAPABILITIES = [
  { title: 'One key for supported models', icon: KeyRound },
  { title: 'Clear USD and CNY accounting', icon: BadgeDollarSign },
  { title: 'Works with apps and agents', icon: Route },
] as const

export function Home() {
  const { t } = useTranslation()
  const isAuthenticated = useAuthStore((state) => Boolean(state.auth.user))

  return (
    <PublicLayout showMainContainer={false}>
      <main className='border-border/70 flex min-h-svh flex-col border-b pt-16'>
        <section className='mx-auto flex w-full max-w-6xl flex-1 items-center px-5 py-16 sm:px-8 sm:py-20 lg:px-10'>
          <div className='grid w-full items-end gap-12 lg:grid-cols-[minmax(0,1.35fr)_minmax(20rem,0.65fr)] lg:gap-20'>
            <div className='max-w-3xl'>
              <p className='text-primary mb-5 text-sm font-semibold'>
                {t('For verified Yanchuan students, alumni, and teachers')}
              </p>
              <h1 className='text-foreground text-4xl font-semibold sm:text-5xl lg:text-6xl'>
                {t('Yanchuan API')}
              </h1>
              <p className='text-muted-foreground mt-6 max-w-2xl text-base leading-8 sm:text-lg'>
                {t(
                  'A simple, shared AI gateway maintained for the Yanchuan community. Start with a public-interest allowance, bring your own provider when needed, and keep one API key across your tools.'
                )}
              </p>
              <div className='mt-9 flex flex-wrap gap-3'>
                <Button
                  size='lg'
                  render={
                    <Link
                      to={isAuthenticated ? '/dashboard' : '/sign-in'}
                    />
                  }
                >
                  {isAuthenticated ? t('Open console') : t('Sign in with main site')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                <Button size='lg' variant='outline' render={<Link to='/docs' />}>
                  {t('Read the quick guide')}
                </Button>
              </div>
            </div>

            <div className='border-border divide-border border-y'>
              {CAPABILITIES.map((capability) => {
                const Icon = capability.icon
                return (
                  <div
                    key={capability.title}
                    className='flex min-h-16 items-center gap-4 border-b py-4 last:border-b-0'
                  >
                    <Icon className='text-primary size-5 shrink-0' />
                    <span className='text-sm font-medium sm:text-base'>
                      {t(capability.title)}
                    </span>
                  </div>
                )
              })}
            </div>
          </div>
        </section>

        <section className='bg-muted/35 border-border border-t'>
          <div className='mx-auto grid max-w-6xl gap-4 px-5 py-6 text-sm sm:grid-cols-3 sm:px-8 lg:px-10'>
            <p>
              <span className='font-semibold'>$1.00</span>
              <span className='text-muted-foreground'> = ¥7.00</span>
            </p>
            <p className='text-muted-foreground sm:text-center'>
              {t('First-version allowance: $1.00 / ¥7.00')}
            </p>
            <p className='text-muted-foreground sm:text-right'>
              {t('Usage and rates remain visible before every request')}
            </p>
          </div>
        </section>
      </main>
    </PublicLayout>
  )
}
