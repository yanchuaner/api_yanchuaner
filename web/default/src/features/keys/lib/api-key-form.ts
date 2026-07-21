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
import type { TFunction } from 'i18next'
import { z } from 'zod'

import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

import { DEFAULT_GROUP } from '../constants'
import type { ApiKey, ApiKeyFormData, VirtualKeyPolicy } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export function getApiKeyFormSchema(
  t: TFunction,
  finiteBudgetRequired = false,
  policyManaged = false,
  policyReasonRequired = false
) {
  return z
    .object({
      name: z.string().min(1, t('Please enter a name')),
      remain_quota_dollars: z.number().optional(),
      expired_time: z.date().optional(),
      unlimited_quota: z.boolean(),
      model_limits: z.array(z.string()),
      allow_ips: z.string().optional(),
      group: z.string().optional(),
      cross_group_retry: z.boolean().optional(),
      tokenCount: z.number().min(1).optional(),
      max_rpm: z.number().int().min(1),
      max_tpm: z.number().int().min(1),
      max_concurrency: z.number().int().min(1),
      policy_reason: z.string().max(255).optional(),
    })
    .superRefine((data, ctx) => {
      if (finiteBudgetRequired && data.unlimited_quota) {
        ctx.addIssue({
          code: 'custom',
          path: ['unlimited_quota'],
          message: t('Virtual keys require a finite budget'),
        })
        return
      }

      if (data.unlimited_quota) {
        return
      }

      if (policyManaged && data.model_limits.length === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['model_limits'],
          message: t('Select at least one supported model'),
        })
      }

      if (
        policyReasonRequired &&
        (!data.policy_reason || data.policy_reason.trim().length < 3)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['policy_reason'],
          message: t('Enter a change reason'),
        })
      }

      if (
        data.remain_quota_dollars === undefined ||
        (finiteBudgetRequired
          ? parseQuotaFromDollars(data.remain_quota_dollars) <= 0
          : data.remain_quota_dollars < 0)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['remain_quota_dollars'],
          message: finiteBudgetRequired
            ? t('Quota must be greater than zero')
            : t('Quota must be zero or greater'),
        })
      }
    })
}

export type ApiKeyFormValues = z.infer<ReturnType<typeof getApiKeyFormSchema>>

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  remain_quota_dollars: 10,
  expired_time: undefined,
  unlimited_quota: true,
  model_limits: [],
  allow_ips: '',
  group: DEFAULT_GROUP,
  cross_group_retry: true,
  tokenCount: 1,
  max_rpm: 60,
  max_tpm: 100000,
  max_concurrency: 2,
  policy_reason: '',
}

type PolicyDefaults = Pick<
  ApiKeyFormValues,
  'max_rpm' | 'max_tpm' | 'max_concurrency'
>

export function getApiKeyFormDefaultValues(
  defaultUseAutoGroup: boolean,
  finiteBudgetRequired = false,
  policyDefaults?: PolicyDefaults
): ApiKeyFormValues {
  return {
    ...API_KEY_FORM_DEFAULT_VALUES,
    group: defaultUseAutoGroup ? 'auto' : DEFAULT_GROUP,
    cross_group_retry: defaultUseAutoGroup,
    unlimited_quota: finiteBudgetRequired
      ? false
      : API_KEY_FORM_DEFAULT_VALUES.unlimited_quota,
    ...policyDefaults,
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  return {
    name: data.name,
    remain_quota: data.unlimited_quota
      ? 0
      : parseQuotaFromDollars(data.remain_quota_dollars || 0),
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    unlimited_quota: data.unlimited_quota,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    allow_ips: data.allow_ips || '',
    group: data.group || '',
    cross_group_retry: data.group === 'auto' ? !!data.cross_group_retry : false,
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey,
  policy?: VirtualKeyPolicy
): ApiKeyFormValues {
  return {
    name: apiKey.name,
    remain_quota_dollars: apiKey.unlimited_quota
      ? 0
      : quotaUnitsToDollars(apiKey.remain_quota),
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    allow_ips: apiKey.allow_ips || '',
    group: apiKey.group || DEFAULT_GROUP,
    cross_group_retry: !!apiKey.cross_group_retry,
    tokenCount: 1,
    max_rpm: policy?.max_rpm ?? API_KEY_FORM_DEFAULT_VALUES.max_rpm,
    max_tpm: policy?.max_tpm ?? API_KEY_FORM_DEFAULT_VALUES.max_tpm,
    max_concurrency:
      policy?.max_concurrency ?? API_KEY_FORM_DEFAULT_VALUES.max_concurrency,
    policy_reason: '',
  }
}
