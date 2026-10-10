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
import { useEffect, useMemo, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertTriangle, ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useSystemConfigStore } from '@/stores/system-config-store'
import {
  billingCurrencyFromUSD,
  billingCurrencyToUSD,
  formatBillingCurrencyFromUSD,
  getBillingCurrencyLabel,
  getBillingCurrencySymbol,
} from '@/lib/currency'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { combineBillingExpr } from '@/features/pricing/lib/billing-expr'
import { normalizeTaskPricingResolution } from '@/features/pricing/lib/model-helpers'
import type {
  ReferenceVideoPolicy,
  TaskPricing,
  TaskPricingTier,
} from '@/features/pricing/types'
import { isValidTaskPricing } from './task-pricing-utils'
import { TieredPricingEditor } from './tiered-pricing-editor'

const createModelPricingSchema = (t: (key: string) => string) =>
  z.object({
    name: z.string().min(1, t('Model name is required')),
    price: z.string().optional(),
    ratio: z.string().optional(),
    cacheRatio: z.string().optional(),
    createCacheRatio: z.string().optional(),
    completionRatio: z.string().optional(),
    imageRatio: z.string().optional(),
    audioRatio: z.string().optional(),
    audioCompletionRatio: z.string().optional(),
  })

type ModelPricingFormValues = z.infer<
  ReturnType<typeof createModelPricingSchema>
>

type PricingMode = 'per-token' | 'per-request' | 'task_pricing' | 'tiered_expr'
type LaneKey =
  | 'completion'
  | 'cache'
  | 'createCache'
  | 'image'
  | 'audioInput'
  | 'audioOutput'

export type ModelRatioData = {
  name: string
  price?: string
  ratio?: string
  cacheRatio?: string
  createCacheRatio?: string
  completionRatio?: string
  imageRatio?: string
  audioRatio?: string
  audioCompletionRatio?: string
  billingMode?: PricingMode
  billingExpr?: string
  requestRuleExpr?: string
  taskPricing?: TaskPricing
}

type ModelPricingSheetProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: ModelRatioData) => void
  onCancel?: () => void
  editData?: ModelRatioData | null
  selectedTargetCount?: number
  taskPricingResolutionOptions?: Record<string, string[]>
}

type ModelPricingEditorPanelProps = Omit<
  ModelPricingSheetProps,
  'open' | 'onOpenChange'
> & {
  className?: string
}

type PreviewRow = {
  key: string
  label: string
  value: string
  multiline?: boolean
}

type TaskPricingVariant = 'legacy' | 'matrix'

type TaskPricingTierDraft = {
  id: number
  resolution: string
  noReferencePrice: string
  referenceVideoPolicy: ReferenceVideoPolicy
  referencePrice: string
  original: Record<string, unknown>
}

let nextTaskPricingTierID = 1

function createTaskPricingTierDraft(
  resolution = '',
  tier?: TaskPricingTier
): TaskPricingTierDraft {
  return {
    id: nextTaskPricingTierID++,
    resolution,
    noReferencePrice: formatNumber(tier?.no_reference_video_unit_price),
    referenceVideoPolicy: tier?.reference_video_policy || 'same',
    referencePrice: formatNumber(tier?.reference_video_unit_price),
    original: tier
      ? (structuredClone(tier) as unknown as Record<string, unknown>)
      : {},
  }
}

const numericDraftRegex = /^(\d+(\.\d*)?|\.\d*)?$/

const EMPTY_LANE_PRICES: Record<LaneKey, string> = {
  completion: '',
  cache: '',
  createCache: '',
  image: '',
  audioInput: '',
  audioOutput: '',
}

const EMPTY_LANE_ENABLED: Record<LaneKey, boolean> = {
  completion: false,
  cache: false,
  createCache: false,
  image: false,
  audioInput: false,
  audioOutput: false,
}

const ratioFieldByLane: Record<LaneKey, keyof ModelPricingFormValues> = {
  completion: 'completionRatio',
  cache: 'cacheRatio',
  createCache: 'createCacheRatio',
  image: 'imageRatio',
  audioInput: 'audioRatio',
  audioOutput: 'audioCompletionRatio',
}

const laneConfigs: Array<{
  key: LaneKey
  titleKey: string
  descriptionKey: string
  placeholder: string
}> = [
  {
    key: 'completion',
    titleKey: 'Completion price',
    descriptionKey: 'Output token price for generated tokens.',
    placeholder: '15',
  },
  {
    key: 'cache',
    titleKey: 'Cache read price',
    descriptionKey: 'Token price for cache reads.',
    placeholder: '0.3',
  },
  {
    key: 'createCache',
    titleKey: 'Cache write price',
    descriptionKey: 'Token price for creating cache entries.',
    placeholder: '3.75',
  },
  {
    key: 'image',
    titleKey: 'Image input price',
    descriptionKey: 'Token price for image input.',
    placeholder: '2.5',
  },
  {
    key: 'audioInput',
    titleKey: 'Audio input price',
    descriptionKey: 'Token price for audio input.',
    placeholder: '3.81',
  },
  {
    key: 'audioOutput',
    titleKey: 'Audio output price',
    descriptionKey: 'Token price for audio output.',
    placeholder: '15.11',
  },
]

function hasValue(value: unknown): boolean {
  return (
    value !== '' && value !== null && value !== undefined && value !== false
  )
}

function toNumberOrNull(value: unknown): number | null {
  if (!hasValue(value) && value !== 0) return null
  const num = Number(value)
  return Number.isFinite(num) ? num : null
}

function formatNumber(value: unknown): string {
  const num = toNumberOrNull(value)
  if (num === null) return ''
  return Number.parseFloat(num.toFixed(12)).toString()
}

function formatDisplayBillingDraft(value: unknown): string {
  const num = toNumberOrNull(value)
  if (num === null) return ''
  return formatNumber(billingCurrencyFromUSD(num))
}

function formatStoredBillingDraft(value: unknown): string {
  const num = toNumberOrNull(value)
  if (num === null) return ''
  return formatNumber(billingCurrencyToUSD(num))
}

function formatDisplayBillingPrice(value: unknown): string {
  const num = toNumberOrNull(value)
  if (num === null) return ''
  return formatBillingCurrencyFromUSD(num, {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

function ratioToBasePrice(ratio: unknown): string {
  const num = toNumberOrNull(ratio)
  if (num === null) return ''
  return formatNumber(num * 2)
}

function deriveLanePrice(
  ratio: unknown,
  denominator: unknown,
  fallback = ''
): string {
  const ratioNumber = toNumberOrNull(ratio)
  const denominatorNumber = toNumberOrNull(denominator)
  if (ratioNumber === null || denominatorNumber === null) return fallback
  return formatNumber(ratioNumber * denominatorNumber)
}

function createInitialLaneState(data?: ModelRatioData | null) {
  if (!data) {
    return {
      promptPrice: '',
      prices: { ...EMPTY_LANE_PRICES },
      enabled: { ...EMPTY_LANE_ENABLED },
    }
  }

  const promptPrice = ratioToBasePrice(data.ratio)
  const audioInputPrice = deriveLanePrice(data.audioRatio, promptPrice)
  const prices: Record<LaneKey, string> = {
    completion: deriveLanePrice(data.completionRatio, promptPrice),
    cache: deriveLanePrice(data.cacheRatio, promptPrice),
    createCache: deriveLanePrice(data.createCacheRatio, promptPrice),
    image: deriveLanePrice(data.imageRatio, promptPrice),
    audioInput: audioInputPrice,
    audioOutput: deriveLanePrice(data.audioCompletionRatio, audioInputPrice),
  }

  return {
    promptPrice,
    prices,
    enabled: {
      completion: hasValue(data.completionRatio),
      cache: hasValue(data.cacheRatio),
      createCache: hasValue(data.createCacheRatio),
      image: hasValue(data.imageRatio),
      audioInput: hasValue(data.audioRatio),
      audioOutput: hasValue(data.audioCompletionRatio),
    },
  }
}

function getModeLabel(mode: PricingMode) {
  if (mode === 'per-request') return 'Per-request'
  if (mode === 'task_pricing') return 'Per-second'
  if (mode === 'tiered_expr') return 'Expression'
  return 'Per-token'
}

function getModeBadgeVariant(
  mode: PricingMode
): 'default' | 'secondary' | 'outline' {
  if (mode === 'per-request') return 'secondary'
  if (mode === 'task_pricing') return 'secondary'
  if (mode === 'tiered_expr') return 'default'
  return 'outline'
}

function buildPreviewRows(
  values: ModelPricingFormValues,
  mode: PricingMode,
  billingExpr: string,
  requestRuleExpr: string,
  promptPrice: string,
  lanePrices: Record<LaneKey, string>,
  laneEnabled: Record<LaneKey, boolean>,
  taskNoReferencePrice: string,
  referenceVideoPolicy: ReferenceVideoPolicy,
  taskReferencePrice: string,
  taskPricingVariant: TaskPricingVariant,
  taskPricingTiers: TaskPricingTierDraft[],
  t: (key: string) => string
): PreviewRow[] {
  if (mode === 'tiered_expr') {
    const effectiveExpr = combineBillingExpr(billingExpr, requestRuleExpr)
    return [
      { key: 'mode', label: 'BillingMode', value: 'tiered_expr' },
      {
        key: 'expr',
        label: t('Expression'),
        value: effectiveExpr || t('Empty'),
        multiline: true,
      },
    ]
  }

  if (mode === 'per-request') {
    return [
      {
        key: 'price',
        label: 'ModelPrice',
        value: values.price
          ? formatDisplayBillingPrice(values.price)
          : t('Empty'),
      },
    ]
  }

  if (mode === 'task_pricing') {
    if (taskPricingVariant === 'matrix') {
      return [
        { key: 'mode', label: 'BillingMode', value: 'task_pricing' },
        ...taskPricingTiers.map((tier) => {
          const noReferencePrice = toNumberOrNull(tier.noReferencePrice)
          const referencePrice =
            tier.referenceVideoPolicy === 'same'
              ? noReferencePrice
              : toNumberOrNull(tier.referencePrice)
          const referencePreview =
            tier.referenceVideoPolicy === 'disabled'
              ? t('Not allowed')
              : referencePrice !== null
                ? formatDisplayBillingPrice(referencePrice * 5)
                : '—'
          return {
            key: `resolution-${tier.id}`,
            label:
              normalizeTaskPricingResolution(tier.resolution) ||
              t('Resolution'),
            value: `${t('Without video input')} ${
              noReferencePrice !== null
                ? formatDisplayBillingPrice(noReferencePrice * 5)
                : '—'
            } · ${t('With video input')} ${referencePreview}`,
          }
        }),
      ]
    }
    const noReferencePrice = toNumberOrNull(taskNoReferencePrice)
    const referencePrice =
      referenceVideoPolicy === 'same'
        ? noReferencePrice
        : toNumberOrNull(taskReferencePrice)
    const rows: PreviewRow[] = [
      { key: 'mode', label: 'BillingMode', value: 'task_pricing' },
      {
        key: 'noReferencePrice',
        label: t('Without video input'),
        value:
          noReferencePrice !== null
            ? `${formatDisplayBillingPrice(noReferencePrice)} / ${t('second')}`
            : t('Empty'),
      },
      {
        key: 'referencePolicy',
        label: t('Video input rule'),
        value: t(
          referenceVideoPolicy === 'same'
            ? 'Same price'
            : referenceVideoPolicy === 'custom'
              ? 'Separate price'
              : 'Not allowed'
        ),
      },
    ]

    if (referenceVideoPolicy !== 'disabled') {
      rows.push({
        key: 'fiveSecondPreview',
        label: t('5-second preview'),
        value:
          noReferencePrice !== null && referencePrice !== null
            ? `${t('Without video input')} ${formatDisplayBillingPrice(noReferencePrice * 5)} · ${t('With video input')} ${formatDisplayBillingPrice(referencePrice * 5)}`
            : t('Empty'),
      })
    }
    return rows
  }

  return [
    {
      key: 'inputPrice',
      label: t('Input price'),
      value: promptPrice ? formatDisplayBillingPrice(promptPrice) : t('Empty'),
    },
    {
      key: 'completion',
      label: t('Completion price'),
      value:
        laneEnabled.completion && lanePrices.completion
          ? formatDisplayBillingPrice(lanePrices.completion)
          : t('Empty'),
    },
    {
      key: 'cache',
      label: t('Cache read price'),
      value:
        laneEnabled.cache && lanePrices.cache
          ? formatDisplayBillingPrice(lanePrices.cache)
          : t('Empty'),
    },
    {
      key: 'createCache',
      label: t('Cache write price'),
      value:
        laneEnabled.createCache && lanePrices.createCache
          ? formatDisplayBillingPrice(lanePrices.createCache)
          : t('Empty'),
    },
    {
      key: 'image',
      label: t('Image input price'),
      value:
        laneEnabled.image && lanePrices.image
          ? formatDisplayBillingPrice(lanePrices.image)
          : t('Empty'),
    },
    {
      key: 'audio',
      label: t('Audio input price'),
      value:
        laneEnabled.audioInput && lanePrices.audioInput
          ? formatDisplayBillingPrice(lanePrices.audioInput)
          : t('Empty'),
    },
    {
      key: 'audioCompletion',
      label: t('Audio output price'),
      value:
        laneEnabled.audioOutput && lanePrices.audioOutput
          ? formatDisplayBillingPrice(lanePrices.audioOutput)
          : t('Empty'),
    },
  ]
}

export function ModelPricingSheet({
  open,
  onOpenChange,
  onSave,
  onCancel,
  editData,
  selectedTargetCount = 0,
  taskPricingResolutionOptions,
}: ModelPricingSheetProps) {
  const { t } = useTranslation()
  const title = editData ? t('Edit model pricing') : t('Add model pricing')
  const description = editData?.name || t('New model')

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='w-full gap-0 p-0 sm:max-w-2xl'>
        <SheetHeader className='sr-only'>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <ModelPricingEditorPanel
          onSave={onSave}
          editData={editData}
          selectedTargetCount={selectedTargetCount}
          taskPricingResolutionOptions={taskPricingResolutionOptions}
          onCancel={() => {
            onCancel?.()
            onOpenChange(false)
          }}
          className='h-full rounded-none border-0'
        />
      </SheetContent>
    </Sheet>
  )
}

export function ModelPricingEditorPanel({
  onSave,
  editData,
  selectedTargetCount = 0,
  taskPricingResolutionOptions = {},
  onCancel,
  className,
}: ModelPricingEditorPanelProps) {
  const { t } = useTranslation()
  const currency = useSystemConfigStore((state) => state.config.currency)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [promptPrice, setPromptPrice] = useState('')
  const [lanePrices, setLanePrices] = useState<Record<LaneKey, string>>({
    ...EMPTY_LANE_PRICES,
  })
  const [laneEnabled, setLaneEnabled] = useState<Record<LaneKey, boolean>>({
    ...EMPTY_LANE_ENABLED,
  })
  const [billingExpr, setBillingExpr] = useState('')
  const [requestRuleExpr, setRequestRuleExpr] = useState('')
  const [taskNoReferencePrice, setTaskNoReferencePrice] = useState('')
  const [referenceVideoPolicy, setReferenceVideoPolicy] =
    useState<ReferenceVideoPolicy>('same')
  const [taskReferencePrice, setTaskReferencePrice] = useState('')
  const [taskPricingVariant, setTaskPricingVariant] =
    useState<TaskPricingVariant>('matrix')
  const [taskPricingTiers, setTaskPricingTiers] = useState<
    TaskPricingTierDraft[]
  >([])
  const [taskPricingSource, setTaskPricingSource] = useState<
    Record<string, unknown>
  >({})
  const [showTaskErrors, setShowTaskErrors] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(true)
  const isEditMode = !!editData
  const editModelSupportsResolutionMatrix = editData
    ? (taskPricingResolutionOptions[editData.name]?.length ?? 0) > 0
    : false
  const priceUnitLabel = `${getBillingCurrencyLabel()}/1M ${t('tokens')}`

  const form = useForm<ModelPricingFormValues>({
    resolver: zodResolver(createModelPricingSchema(t)),
    defaultValues: {
      name: '',
      price: '',
      ratio: '',
      cacheRatio: '',
      createCacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
    },
  })

  useEffect(() => {
    const nextLaneState = createInitialLaneState(editData)

    if (editData) {
      form.reset({
        name: editData.name,
        price: editData.price || '',
        ratio: editData.ratio || '',
        cacheRatio: editData.cacheRatio || '',
        createCacheRatio: editData.createCacheRatio || '',
        completionRatio: editData.completionRatio || '',
        imageRatio: editData.imageRatio || '',
        audioRatio: editData.audioRatio || '',
        audioCompletionRatio: editData.audioCompletionRatio || '',
      })
      setPricingMode(
        editData.billingMode === 'task_pricing'
          ? 'task_pricing'
          : editData.billingMode === 'tiered_expr'
            ? 'tiered_expr'
            : editData.price
              ? 'per-request'
              : 'per-token'
      )
      setBillingExpr(editData.billingExpr || '')
      setRequestRuleExpr(editData.requestRuleExpr || '')
      const editTaskPricing = editData.taskPricing
      const usesMatrix =
        !!editTaskPricing &&
        'by_resolution' in editTaskPricing &&
        !!editTaskPricing.by_resolution
      setTaskPricingVariant(
        editTaskPricing
          ? usesMatrix
            ? 'matrix'
            : 'legacy'
          : editModelSupportsResolutionMatrix
            ? 'matrix'
            : 'legacy'
      )
      setTaskPricingSource(
        editTaskPricing
          ? (structuredClone(editTaskPricing) as unknown as Record<
              string,
              unknown
            >)
          : {}
      )
      setTaskPricingTiers(
        usesMatrix
          ? Object.entries(editTaskPricing.by_resolution).map(
              ([resolution, tier]) =>
                createTaskPricingTierDraft(resolution, tier)
            )
          : []
      )
      setTaskNoReferencePrice(
        !usesMatrix
          ? formatNumber(editTaskPricing?.no_reference_video_unit_price)
          : ''
      )
      setReferenceVideoPolicy(
        !usesMatrix ? editTaskPricing?.reference_video_policy || 'same' : 'same'
      )
      setTaskReferencePrice(
        !usesMatrix
          ? formatNumber(editTaskPricing?.reference_video_unit_price)
          : ''
      )
    } else {
      form.reset({
        name: '',
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
      })
      setPricingMode('per-token')
      setBillingExpr('')
      setRequestRuleExpr('')
      setTaskNoReferencePrice('')
      setReferenceVideoPolicy('same')
      setTaskReferencePrice('')
      setTaskPricingVariant('matrix')
      setTaskPricingTiers([])
      setTaskPricingSource({})
    }

    setPromptPrice(nextLaneState.promptPrice)
    setLanePrices(nextLaneState.prices)
    setLaneEnabled(nextLaneState.enabled)
    setPreviewOpen(true)
    setShowTaskErrors(false)
  }, [editData, editModelSupportsResolutionMatrix, form])

  const setFormValue = (field: keyof ModelPricingFormValues, value: string) => {
    form.setValue(field, value, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  const deriveLaneRatio = (
    lane: LaneKey,
    price: string,
    nextPromptPrice = promptPrice,
    nextLanePrices = lanePrices
  ) => {
    const priceNumber = toNumberOrNull(price)
    if (priceNumber === null) return ''

    if (lane === 'audioOutput') {
      const audioInputPrice = toNumberOrNull(nextLanePrices.audioInput)
      if (audioInputPrice === null || audioInputPrice === 0) return ''
      return formatNumber(priceNumber / audioInputPrice)
    }

    const inputPrice = toNumberOrNull(nextPromptPrice)
    if (inputPrice === null || inputPrice === 0) return ''
    return formatNumber(priceNumber / inputPrice)
  }

  const syncLaneRatios = (
    nextPromptPrice = promptPrice,
    nextLanePrices = lanePrices,
    nextLaneEnabled = laneEnabled
  ) => {
    const inputPrice = toNumberOrNull(nextPromptPrice)
    setFormValue(
      'ratio',
      inputPrice !== null ? formatNumber(inputPrice / 2) : ''
    )

    laneConfigs.forEach(({ key }) => {
      const ratioField = ratioFieldByLane[key]
      if (!nextLaneEnabled[key]) {
        setFormValue(ratioField, '')
        return
      }
      setFormValue(
        ratioField,
        deriveLaneRatio(
          key,
          nextLanePrices[key],
          nextPromptPrice,
          nextLanePrices
        )
      )
    })
  }

  const handlePromptPriceChange = (value: string) => {
    if (!numericDraftRegex.test(value)) return
    setPromptPrice(value)
    syncLaneRatios(value, lanePrices, laneEnabled)
  }

  const handleLanePriceChange = (lane: LaneKey, value: string) => {
    if (!numericDraftRegex.test(value)) return
    const nextLanePrices = { ...lanePrices, [lane]: value }
    setLanePrices(nextLanePrices)

    if (laneEnabled[lane]) {
      setFormValue(
        ratioFieldByLane[lane],
        deriveLaneRatio(lane, value, promptPrice, nextLanePrices)
      )
    }

    if (lane === 'audioInput' && laneEnabled.audioOutput) {
      setFormValue(
        'audioCompletionRatio',
        deriveLaneRatio(
          'audioOutput',
          nextLanePrices.audioOutput,
          promptPrice,
          nextLanePrices
        )
      )
    }
  }

  const handleLaneToggle = (lane: LaneKey, checked: boolean) => {
    const nextEnabled = { ...laneEnabled, [lane]: checked }
    let nextPrices = lanePrices

    if (!checked) {
      nextPrices = { ...nextPrices, [lane]: '' }
      setFormValue(ratioFieldByLane[lane], '')
      if (lane === 'audioInput') {
        nextEnabled.audioOutput = false
        nextPrices.audioOutput = ''
        setFormValue('audioCompletionRatio', '')
      }
    }

    setLaneEnabled(nextEnabled)
    setLanePrices(nextPrices)

    if (checked) {
      setFormValue(
        ratioFieldByLane[lane],
        deriveLaneRatio(lane, nextPrices[lane], promptPrice, nextPrices)
      )
    }
  }

  const handleModeChange = (value: string) => {
    const nextMode = value as PricingMode
    setPricingMode(nextMode)
    setShowTaskErrors(false)
    if (nextMode === 'tiered_expr' && !billingExpr) {
      setBillingExpr('tier("base", p * 0 + c * 0)')
    }
  }

  const watchedValues = form.watch()
  const previewRows = useMemo(() => {
    void currency
    return buildPreviewRows(
      watchedValues,
      pricingMode,
      billingExpr,
      requestRuleExpr,
      promptPrice,
      lanePrices,
      laneEnabled,
      taskNoReferencePrice,
      referenceVideoPolicy,
      taskReferencePrice,
      taskPricingVariant,
      taskPricingTiers,
      t
    )
  }, [
    billingExpr,
    laneEnabled,
    lanePrices,
    pricingMode,
    promptPrice,
    requestRuleExpr,
    taskNoReferencePrice,
    referenceVideoPolicy,
    taskReferencePrice,
    taskPricingVariant,
    taskPricingTiers,
    t,
    watchedValues,
    currency,
  ])

  const taskNoReferencePriceValid =
    (toNumberOrNull(taskNoReferencePrice) ?? 0) > 0
  const taskReferencePriceValid =
    referenceVideoPolicy !== 'custom' ||
    (toNumberOrNull(taskReferencePrice) ?? 0) > 0
  const showNoReferencePriceError =
    !taskNoReferencePriceValid &&
    (showTaskErrors || taskNoReferencePrice !== '')
  const showReferencePriceError =
    !taskReferencePriceValid && (showTaskErrors || taskReferencePrice !== '')
  const supportedTaskResolutions = useMemo(
    () => taskPricingResolutionOptions[watchedValues.name.trim()] || [],
    [taskPricingResolutionOptions, watchedValues.name]
  )
  const matrixValidation = useMemo(() => {
    const invalidTierIDs = new Set<number>()
    const seen = new Map<string, number>()
    for (const tier of taskPricingTiers) {
      const resolution = normalizeTaskPricingResolution(tier.resolution)
      const noReferencePrice = toNumberOrNull(tier.noReferencePrice)
      const referencePrice = toNumberOrNull(tier.referencePrice)
      const duplicateID = seen.get(resolution)
      if (
        !resolution ||
        resolution.length > 128 ||
        noReferencePrice === null ||
        noReferencePrice <= 0 ||
        (tier.referenceVideoPolicy === 'custom' &&
          (referencePrice === null || referencePrice <= 0))
      ) {
        invalidTierIDs.add(tier.id)
      }
      if (duplicateID !== undefined) {
        invalidTierIDs.add(duplicateID)
        invalidTierIDs.add(tier.id)
      }
      seen.set(resolution, tier.id)
    }
    return {
      invalidTierIDs,
      valid: taskPricingTiers.length > 0 && invalidTierIDs.size === 0,
    }
  }, [taskPricingTiers])

  const updateTaskPricingTier = (
    id: number,
    patch: Partial<TaskPricingTierDraft>
  ) => {
    setTaskPricingTiers((tiers) =>
      tiers.map((tier) => (tier.id === id ? { ...tier, ...patch } : tier))
    )
  }

  const addTaskPricingTier = () => {
    const used = new Set(
      taskPricingTiers.map((tier) =>
        normalizeTaskPricingResolution(tier.resolution)
      )
    )
    const resolution =
      supportedTaskResolutions.find((candidate) => !used.has(candidate)) || ''
    setTaskPricingTiers((tiers) => [
      ...tiers,
      createTaskPricingTierDraft(resolution),
    ])
  }

  const handleTaskPricingVariantChange = (variant: TaskPricingVariant) => {
    if (variant === taskPricingVariant) return
    if (variant === 'matrix' && taskPricingTiers.length === 0) {
      const tier = createTaskPricingTierDraft(
        supportedTaskResolutions[0] || '',
        taskNoReferencePriceValid
          ? {
              no_reference_video_unit_price: Number(taskNoReferencePrice),
              reference_video_policy: referenceVideoPolicy,
              ...(referenceVideoPolicy === 'custom' && taskReferencePriceValid
                ? {
                    reference_video_unit_price: Number(taskReferencePrice),
                  }
                : {}),
            }
          : undefined
      )
      setTaskPricingTiers([tier])
    }
    if (variant === 'legacy' && taskPricingTiers[0]) {
      const tier = taskPricingTiers[0]
      setTaskNoReferencePrice(tier.noReferencePrice)
      setReferenceVideoPolicy(tier.referenceVideoPolicy)
      setTaskReferencePrice(tier.referencePrice)
    }
    setTaskPricingVariant(variant)
    setShowTaskErrors(false)
  }

  const warnings = useMemo(() => {
    const nextWarnings: string[] = []
    const hasConflict =
      !!editData?.price &&
      [
        editData.ratio,
        editData.completionRatio,
        editData.cacheRatio,
        editData.createCacheRatio,
        editData.imageRatio,
        editData.audioRatio,
        editData.audioCompletionRatio,
      ].some(hasValue)

    if (hasConflict) {
      nextWarnings.push(
        t(
          'This model has both fixed-price and token-price settings. Saving the current mode will rewrite the conflicting fields.'
        )
      )
    }

    if (
      pricingMode === 'task_pricing' &&
      editData &&
      !isValidTaskPricing(editData.taskPricing)
    ) {
      nextWarnings.push(
        t(
          'Configure a positive per-second price before this model can be used.'
        )
      )
    }

    if (
      pricingMode === 'per-token' &&
      toNumberOrNull(promptPrice) === null &&
      laneConfigs.some(
        ({ key }) => laneEnabled[key] && hasValue(lanePrices[key])
      )
    ) {
      nextWarnings.push(
        t('Input price is required before saving dependent prices.')
      )
    }

    if (
      pricingMode === 'per-token' &&
      laneEnabled.audioOutput &&
      !hasValue(lanePrices.audioInput)
    ) {
      nextWarnings.push(t('Audio output price requires an audio input price.'))
    }

    return nextWarnings
  }, [editData, laneEnabled, lanePrices, pricingMode, promptPrice, t])

  const handleSubmit = (values: ModelPricingFormValues) => {
    if (
      pricingMode === 'task_pricing' &&
      (taskPricingVariant === 'matrix'
        ? !matrixValidation.valid
        : !taskNoReferencePriceValid || !taskReferencePriceValid)
    ) {
      setShowTaskErrors(true)
      return
    }
    if (
      pricingMode === 'per-token' &&
      toNumberOrNull(promptPrice) === null &&
      laneConfigs.some(
        ({ key }) => laneEnabled[key] && hasValue(lanePrices[key])
      )
    ) {
      form.setError('ratio', {
        message: t('Input price is required before saving dependent prices.'),
      })
      return
    }

    if (
      pricingMode === 'per-token' &&
      laneEnabled.audioOutput &&
      !hasValue(lanePrices.audioInput)
    ) {
      form.setError('audioRatio', {
        message: t('Audio output price requires an audio input price.'),
      })
      return
    }

    const data: ModelRatioData = {
      name: values.name.trim(),
      billingMode: pricingMode,
      price: values.price || '',
      ratio: values.ratio || '',
      cacheRatio: values.cacheRatio || '',
      createCacheRatio: values.createCacheRatio || '',
      completionRatio: values.completionRatio || '',
      imageRatio: values.imageRatio || '',
      audioRatio: values.audioRatio || '',
      audioCompletionRatio: values.audioCompletionRatio || '',
    }

    if (pricingMode === 'tiered_expr') {
      data.billingExpr = billingExpr
      data.requestRuleExpr = requestRuleExpr
    }

    if (pricingMode === 'task_pricing') {
      const nextPricing = structuredClone(taskPricingSource)
      nextPricing.unit = 'second'
      if (taskPricingVariant === 'matrix') {
        delete nextPricing.no_reference_video_unit_price
        delete nextPricing.reference_video_policy
        delete nextPricing.reference_video_unit_price
        nextPricing.by_resolution = Object.fromEntries(
          taskPricingTiers.map((tier) => {
            const nextTier = structuredClone(tier.original)
            nextTier.no_reference_video_unit_price = Number(
              tier.noReferencePrice
            )
            nextTier.reference_video_policy = tier.referenceVideoPolicy
            if (tier.referenceVideoPolicy === 'custom') {
              nextTier.reference_video_unit_price = Number(tier.referencePrice)
            } else {
              delete nextTier.reference_video_unit_price
            }
            return [normalizeTaskPricingResolution(tier.resolution), nextTier]
          })
        )
      } else {
        delete nextPricing.by_resolution
        nextPricing.no_reference_video_unit_price = Number(taskNoReferencePrice)
        nextPricing.reference_video_policy = referenceVideoPolicy
        if (referenceVideoPolicy === 'custom') {
          nextPricing.reference_video_unit_price = Number(taskReferencePrice)
        } else {
          delete nextPricing.reference_video_unit_price
        }
      }
      data.taskPricing = nextPricing as TaskPricing
    }

    onSave(data)
    form.reset()
    onCancel?.()
  }

  const activeName = watchedValues.name || editData?.name || t('New model')

  return (
    <div
      className={cn(
        'bg-card flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border',
        className
      )}
    >
      <div className='border-b p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0'>
            <h3 className='truncate text-base font-medium'>
              {isEditMode ? t('Edit model pricing') : t('Add model pricing')}
            </h3>
            <p className='text-muted-foreground truncate text-sm'>
              {activeName}
            </p>
          </div>
          <Badge variant={getModeBadgeVariant(pricingMode)}>
            {t(getModeLabel(pricingMode))}
          </Badge>
        </div>
      </div>

      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(handleSubmit)}
          className='flex min-h-0 flex-1 flex-col'
          autoComplete='off'
        >
          <div className='min-h-0 flex-1 overflow-y-auto p-4'>
            <FieldGroup>
              {warnings.length > 0 && (
                <Alert variant='destructive'>
                  <AlertTriangle data-icon='inline-start' />
                  <AlertDescription>
                    <div className='flex flex-col gap-1'>
                      {warnings.map((warning) => (
                        <span key={warning}>{warning}</span>
                      ))}
                    </div>
                  </AlertDescription>
                </Alert>
              )}

              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('gpt-4')}
                        {...field}
                        disabled={isEditMode}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The exact model identifier as used in API requests.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Tabs value={pricingMode} onValueChange={handleModeChange}>
                <TabsList className='grid h-auto w-full grid-cols-2 sm:grid-cols-4'>
                  <TabsTrigger value='per-token'>{t('Per-token')}</TabsTrigger>
                  <TabsTrigger value='per-request'>
                    {t('Per-request')}
                  </TabsTrigger>
                  <TabsTrigger value='task_pricing'>
                    {t('Per-second')}
                  </TabsTrigger>
                  <TabsTrigger value='tiered_expr'>
                    {t('Expression')}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value='per-token' className='flex flex-col gap-5'>
                  <FieldGroup>
                    <Field>
                      <FieldLabel>{t('Input price')}</FieldLabel>
                      <PriceInput
                        value={promptPrice}
                        placeholder='3'
                        onChange={handlePromptPriceChange}
                      />
                      <FieldDescription>
                        {t('Price')} ({priceUnitLabel})
                      </FieldDescription>
                    </Field>

                    <div className='grid gap-3 sm:grid-cols-2'>
                      {laneConfigs.map((lane) => {
                        const disabled =
                          lane.key === 'audioOutput' &&
                          (!laneEnabled.audioInput ||
                            !hasValue(lanePrices.audioInput))
                        return (
                          <PriceLane
                            key={lane.key}
                            title={t(lane.titleKey)}
                            description={t(lane.descriptionKey)}
                            placeholder={lane.placeholder}
                            value={lanePrices[lane.key]}
                            enabled={laneEnabled[lane.key]}
                            disabled={disabled}
                            onEnabledChange={(checked) =>
                              handleLaneToggle(lane.key, checked)
                            }
                            onChange={(value) =>
                              handleLanePriceChange(lane.key, value)
                            }
                          />
                        )
                      })}
                    </div>
                  </FieldGroup>
                </TabsContent>

                <TabsContent
                  value='per-request'
                  className='flex flex-col gap-5'
                >
                  <FormField
                    control={form.control}
                    name='price'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Fixed price')}</FormLabel>
                        <FormControl>
                          <PriceInput
                            value={field.value || ''}
                            placeholder='0.01'
                            suffix={t('per request')}
                            onChange={field.onChange}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Price')} ({getBillingCurrencyLabel()}/
                          {t('request')})
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </TabsContent>

                <TabsContent
                  value='task_pricing'
                  className='flex flex-col gap-5'
                >
                  <Field>
                    <FieldLabel>{t('Pricing structure')}</FieldLabel>
                    <ToggleGroup
                      value={[taskPricingVariant]}
                      onValueChange={(values) => {
                        const next = values.at(-1) as
                          | TaskPricingVariant
                          | undefined
                        if (next) handleTaskPricingVariantChange(next)
                      }}
                      variant='outline'
                      className='grid w-full grid-cols-2'
                      aria-label={t('Pricing structure')}
                    >
                      <ToggleGroupItem value='legacy'>
                        {t('One price for all resolutions')}
                      </ToggleGroupItem>
                      <ToggleGroupItem value='matrix'>
                        {t('Pricing by resolution')}
                      </ToggleGroupItem>
                    </ToggleGroup>
                    <FieldDescription>
                      {taskPricingVariant === 'matrix'
                        ? t(
                            'Only resolutions supported upstream and configured here are available to users.'
                          )
                        : t(
                            'The same price applies to every resolution supported upstream.'
                          )}
                    </FieldDescription>
                  </Field>

                  {taskPricingVariant === 'legacy' ? (
                    <FieldGroup>
                      <Field
                        data-invalid={showNoReferencePriceError || undefined}
                      >
                        <FieldLabel>{t('Without video input')}</FieldLabel>
                        <PriceInput
                          value={taskNoReferencePrice}
                          placeholder='0.12'
                          suffix={t('per second')}
                          invalid={showNoReferencePriceError}
                          onChange={setTaskNoReferencePrice}
                        />
                        <FieldDescription>
                          {t(
                            'Local selling price for each requested output second when no video is provided.'
                          )}
                        </FieldDescription>
                        {showNoReferencePriceError && (
                          <FieldError>
                            {t('The per-second price must be greater than 0.')}
                          </FieldError>
                        )}
                      </Field>

                      <Field>
                        <FieldLabel>{t('Video input rule')}</FieldLabel>
                        <ToggleGroup
                          value={[referenceVideoPolicy]}
                          onValueChange={(values) => {
                            const next = values.at(-1) as
                              | ReferenceVideoPolicy
                              | undefined
                            if (next) setReferenceVideoPolicy(next)
                          }}
                          variant='outline'
                          className='grid w-full grid-cols-3'
                          aria-label={t('Video input rule')}
                        >
                          <ToggleGroupItem value='same'>
                            {t('Same price')}
                          </ToggleGroupItem>
                          <ToggleGroupItem value='custom'>
                            {t('Separate price')}
                          </ToggleGroupItem>
                          <ToggleGroupItem value='disabled'>
                            {t('Not allowed')}
                          </ToggleGroupItem>
                        </ToggleGroup>
                        <FieldDescription>
                          {t(
                            'New API detects video input automatically and applies this local rule.'
                          )}
                        </FieldDescription>
                      </Field>

                      {referenceVideoPolicy === 'custom' && (
                        <Field
                          data-invalid={showReferencePriceError || undefined}
                        >
                          <FieldLabel>{t('With video input')}</FieldLabel>
                          <PriceInput
                            value={taskReferencePrice}
                            placeholder='0.18'
                            suffix={t('per second')}
                            invalid={showReferencePriceError}
                            onChange={setTaskReferencePrice}
                          />
                          <FieldDescription>
                            {t(
                              'Local selling price for each requested output second when video input is present.'
                            )}
                          </FieldDescription>
                          {showReferencePriceError && (
                            <FieldError>
                              {t(
                                'The per-second price must be greater than 0.'
                              )}
                            </FieldError>
                          )}
                        </Field>
                      )}

                      <Field className='rounded-lg border p-3'>
                        <FieldTitle>{t('5-second preview')}</FieldTitle>
                        <FieldDescription>
                          {taskNoReferencePriceValid
                            ? `${t('Without video input')} ${formatDisplayBillingPrice(Number(taskNoReferencePrice) * 5)}`
                            : `${t('Without video input')} —`}
                          {referenceVideoPolicy === 'disabled'
                            ? ` · ${t('With video input')} ${t('Not allowed')}`
                            : taskReferencePriceValid
                              ? ` · ${t('With video input')} ${formatDisplayBillingPrice(Number(referenceVideoPolicy === 'same' ? taskNoReferencePrice : taskReferencePrice) * 5)}`
                              : ` · ${t('With video input')} —`}
                        </FieldDescription>
                      </Field>
                    </FieldGroup>
                  ) : (
                    <FieldGroup>
                      <div className='flex items-center justify-between gap-3'>
                        <div>
                          <FieldTitle>
                            {t('Resolution price matrix')}
                          </FieldTitle>
                          <FieldDescription>
                            {t(
                              'Catalog suggestions are active now; custom IDs remain saved as inactive tiers.'
                            )}
                          </FieldDescription>
                        </div>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={addTaskPricingTier}
                        >
                          <Plus data-icon='inline-start' />
                          {t('Add resolution')}
                        </Button>
                      </div>

                      <div className='rounded-lg border'>
                        <Table>
                          <TableHeader>
                            <TableRow>
                              <TableHead className='min-w-32'>
                                {t('Resolution')}
                              </TableHead>
                              <TableHead className='min-w-52'>
                                {t('Without video input')}
                              </TableHead>
                              <TableHead className='min-w-36'>
                                {t('Video input rule')}
                              </TableHead>
                              <TableHead className='min-w-52'>
                                {t('With video input')}
                              </TableHead>
                              <TableHead>{t('5-second preview')}</TableHead>
                              <TableHead>{t('Status')}</TableHead>
                              <TableHead className='w-10' />
                            </TableRow>
                          </TableHeader>
                          <TableBody>
                            {taskPricingTiers.map((tier) => {
                              const normalizedResolution =
                                normalizeTaskPricingResolution(tier.resolution)
                              const active =
                                supportedTaskResolutions.includes(
                                  normalizedResolution
                                )
                              const noReferencePrice = toNumberOrNull(
                                tier.noReferencePrice
                              )
                              const referencePrice =
                                tier.referenceVideoPolicy === 'same'
                                  ? noReferencePrice
                                  : toNumberOrNull(tier.referencePrice)
                              const invalid =
                                showTaskErrors &&
                                matrixValidation.invalidTierIDs.has(tier.id)
                              return (
                                <TableRow key={tier.id}>
                                  <TableCell>
                                    <Combobox
                                      options={supportedTaskResolutions.map(
                                        (resolution) => ({
                                          value: resolution,
                                          label: resolution,
                                        })
                                      )}
                                      value={tier.resolution}
                                      onValueChange={(value) =>
                                        updateTaskPricingTier(tier.id, {
                                          resolution: value || '',
                                        })
                                      }
                                      placeholder={t('Resolution ID')}
                                      emptyText={t(
                                        'Enter a canonical resolution ID'
                                      )}
                                      allowCustomValue
                                      className={cn(
                                        'w-32',
                                        invalid && 'border-destructive'
                                      )}
                                    />
                                  </TableCell>
                                  <TableCell>
                                    <PriceInput
                                      value={tier.noReferencePrice}
                                      placeholder='0.08'
                                      suffix={t('per second')}
                                      invalid={invalid}
                                      onChange={(value) =>
                                        updateTaskPricingTier(tier.id, {
                                          noReferencePrice: value,
                                        })
                                      }
                                    />
                                  </TableCell>
                                  <TableCell>
                                    <Select
                                      items={[
                                        {
                                          value: 'same',
                                          label: t('Same price'),
                                        },
                                        {
                                          value: 'custom',
                                          label: t('Separate price'),
                                        },
                                        {
                                          value: 'disabled',
                                          label: t('Not allowed'),
                                        },
                                      ]}
                                      value={tier.referenceVideoPolicy}
                                      onValueChange={(value) =>
                                        updateTaskPricingTier(tier.id, {
                                          referenceVideoPolicy:
                                            value as ReferenceVideoPolicy,
                                        })
                                      }
                                    >
                                      <SelectTrigger className='w-36'>
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        <SelectGroup>
                                          <SelectItem value='same'>
                                            {t('Same price')}
                                          </SelectItem>
                                          <SelectItem value='custom'>
                                            {t('Separate price')}
                                          </SelectItem>
                                          <SelectItem value='disabled'>
                                            {t('Not allowed')}
                                          </SelectItem>
                                        </SelectGroup>
                                      </SelectContent>
                                    </Select>
                                  </TableCell>
                                  <TableCell>
                                    {tier.referenceVideoPolicy === 'custom' ? (
                                      <PriceInput
                                        value={tier.referencePrice}
                                        placeholder='0.12'
                                        suffix={t('per second')}
                                        invalid={invalid}
                                        onChange={(value) =>
                                          updateTaskPricingTier(tier.id, {
                                            referencePrice: value,
                                          })
                                        }
                                      />
                                    ) : (
                                      <span className='text-muted-foreground text-xs'>
                                        {tier.referenceVideoPolicy === 'same'
                                          ? t('Same price')
                                          : t('Not allowed')}
                                      </span>
                                    )}
                                  </TableCell>
                                  <TableCell className='text-xs'>
                                    <div>
                                      {noReferencePrice !== null
                                        ? formatDisplayBillingPrice(
                                            noReferencePrice * 5
                                          )
                                        : '—'}
                                    </div>
                                    <div className='text-muted-foreground'>
                                      {tier.referenceVideoPolicy === 'disabled'
                                        ? t('Not allowed')
                                        : referencePrice !== null
                                          ? formatDisplayBillingPrice(
                                              referencePrice * 5
                                            )
                                          : '—'}
                                    </div>
                                  </TableCell>
                                  <TableCell>
                                    <Badge
                                      variant={active ? 'secondary' : 'outline'}
                                    >
                                      {active ? t('Active') : t('Inactive')}
                                    </Badge>
                                  </TableCell>
                                  <TableCell>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='icon-sm'
                                      aria-label={t('Remove resolution')}
                                      onClick={() =>
                                        setTaskPricingTiers((tiers) =>
                                          tiers.filter(
                                            (candidate) =>
                                              candidate.id !== tier.id
                                          )
                                        )
                                      }
                                    >
                                      <Trash2 />
                                    </Button>
                                  </TableCell>
                                </TableRow>
                              )
                            })}
                          </TableBody>
                        </Table>
                        {taskPricingTiers.length === 0 && (
                          <div className='text-muted-foreground px-4 py-8 text-center text-sm'>
                            {t('No resolution tiers configured')}
                          </div>
                        )}
                      </div>
                      {showTaskErrors && !matrixValidation.valid && (
                        <FieldError>
                          {t(
                            'Add at least one unique resolution with valid positive prices.'
                          )}
                        </FieldError>
                      )}
                      {taskPricingTiers.some(
                        (tier) =>
                          !supportedTaskResolutions.includes(
                            normalizeTaskPricingResolution(tier.resolution)
                          )
                      ) && (
                        <FieldDescription>
                          {t(
                            'Inactive tiers are saved but hidden from public pricing and Playground until upstream support appears.'
                          )}
                        </FieldDescription>
                      )}
                    </FieldGroup>
                  )}
                </TabsContent>

                <TabsContent
                  value='tiered_expr'
                  className='flex flex-col gap-5'
                >
                  <TieredPricingEditor
                    modelName={watchedValues.name}
                    billingExpr={billingExpr}
                    requestRuleExpr={requestRuleExpr}
                    onBillingExprChange={setBillingExpr}
                    onRequestRuleExprChange={setRequestRuleExpr}
                  />
                </TabsContent>
              </Tabs>

              <Collapsible open={previewOpen} onOpenChange={setPreviewOpen}>
                <CollapsibleTrigger
                  render={
                    <Button
                      type='button'
                      variant='outline'
                      className='flex w-full justify-between'
                    />
                  }
                >
                  <span>{t('Save preview')}</span>
                  <ChevronDown
                    className={cn(
                      'transition-transform',
                      previewOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent className='pt-3'>
                  <div className='rounded-lg border'>
                    {previewRows.map((row) => (
                      <div
                        key={row.key}
                        className='grid grid-cols-[140px_1fr] gap-3 border-b px-3 py-2 text-sm last:border-b-0'
                      >
                        <span className='text-muted-foreground text-xs'>
                          {row.label}
                        </span>
                        <span
                          className={cn(
                            'min-w-0',
                            row.multiline
                              ? 'font-mono text-xs leading-5 break-words whitespace-pre-wrap'
                              : 'truncate'
                          )}
                        >
                          {row.value}
                        </span>
                      </div>
                    ))}
                  </div>
                </CollapsibleContent>
              </Collapsible>
            </FieldGroup>
          </div>

          <SheetFooter className='bg-background/95 border-t sm:flex-row sm:items-center sm:justify-between'>
            <div className='text-muted-foreground text-xs'>
              {selectedTargetCount > 0
                ? t('{{count}} selected targets available for bulk copy.', {
                    count: selectedTargetCount,
                  })
                : t('Changes are written to the settings draft on save.')}
            </div>
            <div className='flex justify-end gap-2'>
              <Button type='button' variant='outline' onClick={onCancel}>
                {t('Cancel')}
              </Button>
              <Button type='submit'>
                {isEditMode ? t('Update') : t('Add')}
              </Button>
            </div>
          </SheetFooter>
        </form>
      </Form>
    </div>
  )
}

function PriceInput(props: {
  value: string
  placeholder?: string
  disabled?: boolean
  invalid?: boolean
  suffix?: string
  onChange: (value: string) => void
}) {
  const currencyKey = useSystemConfigStore((state) => {
    const currency = state.config.currency
    return [
      currency.quotaDisplayType,
      currency.usdExchangeRate,
      currency.customCurrencySymbol,
      currency.customCurrencyExchangeRate,
    ].join(':')
  })
  const suffix = props.suffix ?? `${getBillingCurrencyLabel()}/1M`
  const displayValue = formatDisplayBillingDraft(props.value)
  const sourceValue = `${currencyKey}:${displayValue}`
  const [focused, setFocused] = useState(false)
  const [draftState, setDraftState] = useState(() => ({
    source: sourceValue,
    draft: displayValue,
  }))
  let value = focused ? draftState.draft : displayValue

  if (!focused && draftState.source !== sourceValue) {
    value = displayValue
    setDraftState({ source: sourceValue, draft: displayValue })
  }

  const handleChange = (value: string) => {
    if (!numericDraftRegex.test(value)) return
    setDraftState({ source: sourceValue, draft: value })
    props.onChange(formatStoredBillingDraft(value))
  }

  return (
    <InputGroup>
      <InputGroupAddon>{getBillingCurrencySymbol()}</InputGroupAddon>
      <InputGroupInput
        inputMode='decimal'
        aria-invalid={props.invalid || undefined}
        value={value}
        placeholder={props.placeholder}
        disabled={props.disabled}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onChange={(event) => handleChange(event.target.value)}
      />
      <InputGroupAddon align='inline-end'>{suffix}</InputGroupAddon>
    </InputGroup>
  )
}

function PriceLane(props: {
  title: string
  description: string
  placeholder: string
  value: string
  enabled: boolean
  disabled?: boolean
  onEnabledChange: (checked: boolean) => void
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const currencyKey = useSystemConfigStore((state) => {
    const currency = state.config.currency
    return [
      currency.quotaDisplayType,
      currency.usdExchangeRate,
      currency.customCurrencySymbol,
      currency.customCurrencyExchangeRate,
    ].join(':')
  })
  const effectiveDisabled = props.disabled || !props.enabled
  const priceUnitLabel = useMemo(() => {
    void currencyKey
    return `${getBillingCurrencyLabel()}/1M ${t('tokens')}`
  }, [currencyKey, t])

  return (
    <Field
      className={cn(
        'rounded-lg border p-3',
        effectiveDisabled && 'bg-muted/35'
      )}
      data-disabled={effectiveDisabled || undefined}
    >
      <div className='flex items-start justify-between gap-3'>
        <FieldContent>
          <FieldTitle>{props.title}</FieldTitle>
          <FieldDescription>{props.description}</FieldDescription>
        </FieldContent>
        <Switch
          checked={props.enabled}
          disabled={props.disabled}
          onCheckedChange={props.onEnabledChange}
          aria-label={props.title}
        />
      </div>
      <PriceInput
        value={props.value}
        placeholder={props.placeholder}
        disabled={effectiveDisabled}
        onChange={props.onChange}
      />
      <FieldDescription>
        {props.enabled
          ? `${t('Price')} (${priceUnitLabel})`
          : t('Disabled lanes are omitted on save.')}
      </FieldDescription>
    </Field>
  )
}
