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
import { ArrowRight, CheckCircle2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

const STEPS = [
  {
    title: 'Sign in with your main-site account',
    description:
      'Access is limited to verified Yanchuan students, alumni, and teachers.',
  },
  {
    title: 'Create an API key',
    description:
      'Open the console, create a key, and store it like a password.',
  },
  {
    title: 'Use the compatible endpoint',
    description:
      'Point OpenAI-compatible apps and agents to the Yanchuan API endpoint.',
  },
] as const

export function YanchuanDocs() {
  const { t } = useTranslation()

  return (
    <PublicLayout>
      <div className='mx-auto max-w-5xl py-8 sm:py-14'>
        <header className='max-w-3xl'>
          <p className='text-primary text-sm font-semibold'>{t('Quick guide')}</p>
          <h1 className='mt-3 text-3xl font-semibold sm:text-4xl'>
            {t('Connect to Yanchuan API')}
          </h1>
          <p className='text-muted-foreground mt-5 text-base leading-8'>
            {t(
              'The interface follows the OpenAI-compatible format. Most supported applications only need an endpoint, an API key, and a model name.'
            )}
          </p>
        </header>

        <section className='border-border mt-12 divide-y border-y'>
          {STEPS.map((step, index) => (
            <div
              key={step.title}
              className='grid gap-3 py-6 sm:grid-cols-[3rem_1fr] sm:gap-5'
            >
              <span className='text-primary text-sm font-semibold'>
                {String(index + 1).padStart(2, '0')}
              </span>
              <div>
                <h2 className='font-semibold'>{t(step.title)}</h2>
                <p className='text-muted-foreground mt-2 text-sm leading-6'>
                  {t(step.description)}
                </p>
              </div>
            </div>
          ))}
        </section>

        <section className='mt-12 grid gap-8 lg:grid-cols-2'>
          <div>
            <h2 className='text-xl font-semibold'>{t('Connection values')}</h2>
            <dl className='mt-5 space-y-4 text-sm'>
              <div>
                <dt className='text-muted-foreground'>{t('Endpoint')}</dt>
                <dd className='mt-1 font-mono'>https://api.yanchuaner.cn/v1</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Authorization')}</dt>
                <dd className='mt-1 font-mono'>Bearer sk-...</dd>
              </div>
              <div>
                <dt className='text-muted-foreground'>{t('Format')}</dt>
                <dd className='mt-1'>OpenAI-compatible REST API</dd>
              </div>
            </dl>
          </div>
          <div className='bg-muted/45 overflow-x-auto rounded-lg border p-5'>
            <pre className='text-xs leading-6 sm:text-sm'>
              <code>{`curl https://api.yanchuaner.cn/v1/chat/completions \\
  -H "Authorization: Bearer sk-..." \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`}</code>
            </pre>
          </div>
        </section>

        <section className='bg-muted/35 mt-12 rounded-lg border p-6'>
          <h2 className='text-xl font-semibold'>{t('Billing and privacy')}</h2>
          <div className='mt-5 grid gap-4 sm:grid-cols-2'>
            {[
              'Balances are shown in both USD and CNY at $1.00 = ¥7.00.',
              'Every request records its model, token usage, and charge.',
              'The first-version public-interest allowance is $1.00 / ¥7.00.',
              'Never paste an API key into a public repository or shared chat.',
            ].map((item) => (
              <p key={item} className='flex gap-3 text-sm leading-6'>
                <CheckCircle2 className='text-primary mt-0.5 size-4 shrink-0' />
                <span>{t(item)}</span>
              </p>
            ))}
          </div>
        </section>

        <div className='mt-10 flex flex-wrap gap-3'>
          <Button render={<Link to='/dashboard' />}>
            {t('Open console')}
            <ArrowRight data-icon='inline-end' />
          </Button>
          <Button variant='outline' render={<Link to='/' />}>
            {t('Back to home')}
          </Button>
        </div>
      </div>
    </PublicLayout>
  )
}
