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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  CheckCircle2,
  Clock3,
  Copy,
  Eraser,
  ExternalLink,
  FileJson,
  FlaskConical,
  History,
  Loader2,
  Play,
  RefreshCw,
  Save,
  Search,
  Settings2,
  WandSparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getUserModels } from '@/lib/api'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { SectionPageLayout } from '@/components/layout'
import { fetchTokenKey, getApiKeys } from '@/features/keys/api'
import { API_KEY_STATUS } from '@/features/keys/constants'
import type { ApiKey } from '@/features/keys/types'
import {
  DEFAULT_TEST_MODEL,
  MODEL_TEST_HISTORY_KEY,
  MODEL_TEST_STORAGE_KEY,
  TEST_MODELS,
  type TestField,
  type TestModel,
  type TestModelKind,
} from './model-test-platform-data'

declare const __NEW_API_SERVER_URL__: string | undefined

type FormValue = string | boolean
type FormValues = Record<string, FormValue>
type FileValues = Record<string, File>
type JsonRecord = Record<string, unknown>

type StoredConfig = {
  baseUrl?: string
  token?: string
  selectedApiKeyId?: number | string
  selectedModel?: string
  pollMaxAttempts?: number | string
}

type HistoryItem = {
  model?: string
  label?: string
  labelKey?: string
  status?: string
  taskId?: string
  time?: string
}

type PreviewState =
  | { type: 'empty'; messageKey: string }
  | { type: 'text'; text: string }
  | { type: 'media'; kind: Exclude<TestModelKind, 'text'>; url: string }

type StatusTone = 'default' | 'success' | 'warning' | 'danger'

type RunStatus = {
  code: string
  title: string
  text: string
  tone: StatusTone
}

const EMPTY_PREVIEW_KEY =
  'Images, video, audio, and text appear here. Raw responses stay below.'

const DEFAULT_STATUS_KEY =
  'Choose a model and parameters to start. Sync models return directly; async models can poll automatically.'

const selectClassName =
  'border-input focus-visible:border-ring focus-visible:ring-ring/50 dark:bg-input/30 h-8 w-full rounded-lg border bg-transparent px-2.5 py-1 text-sm outline-none transition-colors focus-visible:ring-3'

const MANUAL_API_KEY_VALUE = '__manual__'
const LOCALHOST_DEFAULT_BASE_URL = 'http://localhost:3000'
const DEFAULT_POLL_MAX_ATTEMPTS = 1000
const POSITIVE_INTEGER_REGEX = /^\d+$/

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function getRecord(value: unknown, key: string): JsonRecord | null {
  if (!isRecord(value)) return null
  const child = value[key]
  return isRecord(child) ? child : null
}

function getString(value: unknown, key: string): string {
  if (!isRecord(value)) return ''
  const raw = value[key]
  if (typeof raw === 'string') return raw
  if (typeof raw === 'number') return String(raw)
  return ''
}

function stableJson(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

function getDefaultBaseUrl(): string {
  const envBaseUrl =
    typeof __NEW_API_SERVER_URL__ === 'string' ? __NEW_API_SERVER_URL__ : ''
  if (typeof envBaseUrl === 'string' && envBaseUrl.trim()) {
    return envBaseUrl.trim()
  }
  if (typeof window !== 'undefined') return window.location.origin
  return ''
}

function normalizeBaseUrl(value = ''): string {
  return (value || getDefaultBaseUrl()).trim().replace(/\/+$/, '')
}

function getInitialBaseUrl(value?: string): string {
  const normalized = normalizeBaseUrl(value)
  const defaultBaseUrl = normalizeBaseUrl()
  if (
    normalized === LOCALHOST_DEFAULT_BASE_URL &&
    defaultBaseUrl !== LOCALHOST_DEFAULT_BASE_URL
  ) {
    return defaultBaseUrl
  }
  return normalized
}

function ensureApiKeyPrefix(value: string): string {
  const key = value.trim()
  if (!key) return ''
  return key.startsWith('sk-') ? key : `sk-${key}`
}

function getModelDisplayLabel(
  model: TestModel,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  return model.label || t(model.labelKey)
}

function createDynamicChatModel(modelId: string): TestModel {
  return {
    ...TEST_MODELS[0],
    id: modelId,
    label: modelId,
    labelKey: 'Dynamic chat model',
    vendor: 'Dynamic',
    noteKey:
      'Dynamically fetched model. This test uses Chat Completions by default.',
  }
}

function normalizeTemplateModelId(modelId: string): string {
  return modelId.trim().toLowerCase().replace(/[_.]/g, '-')
}

function createModelFromTemplate(
  modelId: string,
  template: TestModel
): TestModel {
  return {
    ...template,
    id: modelId,
    label: modelId,
  }
}

function findKnownModelTemplate(modelId: string): TestModel | undefined {
  const exact = TEST_MODELS.find((model) => model.id === modelId)
  if (exact) return exact

  const normalizedModelId = normalizeTemplateModelId(modelId)
  return TEST_MODELS.find(
    (model) => normalizeTemplateModelId(model.id) === normalizedModelId
  )
}

function createDynamicModel(modelId: string): TestModel {
  const template = findKnownModelTemplate(modelId)
  return template
    ? createModelFromTemplate(modelId, template)
    : createDynamicChatModel(modelId)
}

function buildModelOptions(dynamicModelIds: string[]): TestModel[] {
  const knownModels = new Map(TEST_MODELS.map((model) => [model.id, model]))
  const normalizedDynamicIds = dynamicModelIds
    .map((modelId) => modelId.trim())
    .filter(Boolean)

  if (normalizedDynamicIds.length === 0) return TEST_MODELS

  const dynamicModels = normalizedDynamicIds.map(
    (modelId) => knownModels.get(modelId) || createDynamicModel(modelId)
  )
  const seen = new Set(
    dynamicModels.map((model) => normalizeTemplateModelId(model.id))
  )
  const templateModels = TEST_MODELS.filter(
    (model) => !seen.has(normalizeTemplateModelId(model.id))
  )
  return [...dynamicModels, ...templateModels]
}

function getApiKeyOptionLabel(apiKey: ApiKey): string {
  const name = apiKey.name || `#${apiKey.id}`
  return apiKey.key ? `${name} · ${apiKey.key}` : name
}

function getDefaultFormValues(model: TestModel): FormValues {
  return Object.fromEntries(
    model.fields.map((field) => [
      field.key,
      field.type === 'checkbox'
        ? Boolean(field.example)
        : String(field.example ?? ''),
    ])
  )
}

function parseStoredConfig(): StoredConfig {
  if (typeof window === 'undefined') return {}
  try {
    const parsed = JSON.parse(
      window.localStorage.getItem(MODEL_TEST_STORAGE_KEY) || '{}'
    ) as StoredConfig
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function persistConfig(patch: StoredConfig) {
  if (typeof window === 'undefined') return
  const current = parseStoredConfig()
  window.localStorage.setItem(
    MODEL_TEST_STORAGE_KEY,
    JSON.stringify({ ...current, ...patch })
  )
}

function parsePollMaxAttempts(value: unknown): number | null {
  const text = String(value ?? '').trim()
  if (!POSITIVE_INTEGER_REGEX.test(text)) return null
  const attempts = Number(text)
  if (!Number.isSafeInteger(attempts) || attempts < 1) return null
  return attempts
}

function getInitialPollMaxAttempts(value: unknown): string {
  return String(parsePollMaxAttempts(value) || DEFAULT_POLL_MAX_ATTEMPTS)
}

function parseNumberFieldValue(value: FormValue | undefined): number | undefined {
  const text = String(value ?? '').trim()
  if (!text) return undefined
  const numberValue = Number(text)
  return Number.isFinite(numberValue) ? numberValue : undefined
}

function normalizeFieldValue(
  field: TestField,
  value: FormValue | undefined
): FormValue | number | undefined {
  if (field.type === 'number') return parseNumberFieldValue(value)
  if (value === '' || value == null) return undefined
  return value
}

function parseHistory(): HistoryItem[] {
  if (typeof window === 'undefined') return []
  try {
    const parsed = JSON.parse(
      window.localStorage.getItem(MODEL_TEST_HISTORY_KEY) || '[]'
    )
    return Array.isArray(parsed) ? (parsed as HistoryItem[]) : []
  } catch {
    return []
  }
}

function buildChatBody(model: TestModel, values: FormValues): JsonRecord {
  const messages: JsonRecord[] = []
  const system = String(values.system || '').trim()
  if (system) messages.push({ role: 'system', content: system })
  messages.push({ role: 'user', content: String(values.prompt || '') })

  const body: JsonRecord = {
    model: model.id,
    messages,
  }
  for (const field of model.fields) {
    if (field.key === 'system' || field.key === 'prompt') continue
    const value = normalizeFieldValue(field, values[field.key])
    if (value === undefined || value === false) continue
    body[field.key] = value
  }
  return body
}

function buildDefaultBody(model: TestModel, values: FormValues): JsonRecord {
  const body: JsonRecord = { model: model.id }
  const metadata: JsonRecord = {}

  for (const field of model.fields) {
    const value = normalizeFieldValue(field, values[field.key])
    if (field.type === 'checkbox') {
      if (value) body[field.key] = true
      continue
    }

    if (value === undefined) continue
    if (field.key === 'duration') {
      const duration = parseNumberFieldValue(values[field.key])
      if (duration !== undefined) body.duration = duration
    } else if (
      ['ratio', 'resolution', 'seed', 'watermark'].includes(field.key) &&
      model.vendor === 'Doubao'
    ) {
      metadata[field.key] =
        field.key === 'watermark' ? Boolean(value) : String(value)
    } else {
      body[field.key] = value
    }
  }

  if (Object.keys(metadata).length > 0) body.metadata = metadata
  return body
}

function buildRequestBody(model: TestModel, values: FormValues): JsonRecord {
  return model.builder === 'chat'
    ? buildChatBody(model, values)
    : buildDefaultBody(model, values)
}

function hasMeaningfulValue(value: unknown): boolean {
  if (value === undefined || value === null) return false
  if (typeof value === 'string') return value.trim() !== ''
  if (Array.isArray(value)) return value.length > 0
  return true
}

function validateRequiredFields(
  model: TestModel,
  body: JsonRecord,
  formValues: FormValues | null,
  files: FileValues,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  for (const field of model.fields) {
    if (!field.required) continue
    const hasFile = Boolean(files[field.key])
    const value = formValues ? formValues[field.key] : body[field.key]
    const rawModeTextPrompt =
      !formValues &&
      field.key === 'prompt' &&
      model.kind === 'text' &&
      Array.isArray(body.messages) &&
      body.messages.some((message) =>
        isRecord(message) ? String(message.content || '').trim() !== '' : false
      )

    if (!hasFile && !rawModeTextPrompt && !hasMeaningfulValue(value)) {
      throw new Error(
        t('Missing required field: {{field}}', { field: field.key })
      )
    }
  }
}

function buildFetchPayload(
  body: JsonRecord,
  files: FileValues
): { body: BodyInit; headers: Record<string, string> } {
  const fileKeys = Object.keys(files)
  if (fileKeys.length === 0) {
    return {
      body: stableJson(body),
      headers: { 'Content-Type': 'application/json' },
    }
  }

  const form = new FormData()
  for (const [key, value] of Object.entries(body)) {
    if (value === undefined || value === null || value === '') continue
    if (typeof value === 'object' && !(value instanceof File)) {
      form.append(key, JSON.stringify(value))
    } else {
      form.append(key, String(value))
    }
  }
  for (const key of fileKeys) {
    form.set(key, files[key])
  }
  return { body: form, headers: {} }
}

async function readJsonOrText(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return {}
  try {
    return JSON.parse(text)
  } catch {
    return { raw: text }
  }
}

function extractTaskId(json: unknown): string {
  return (
    getString(json, 'task_id') ||
    getString(json, 'id') ||
    getString(getRecord(json, 'data'), 'task_id') ||
    getString(getRecord(json, 'data'), 'id') ||
    getString(getRecord(getRecord(json, 'data'), 'video'), 'id')
  )
}

function extractStatus(json: unknown): string {
  return (
    getString(json, 'status') ||
    getString(getRecord(json, 'data'), 'status') ||
    getString(getRecord(json, 'output'), 'status')
  ).toLowerCase()
}

function extractText(json: unknown): string {
  const content = isRecord(json) ? json.content : undefined
  const contentText = Array.isArray(content)
    ? content
        .map((item) => (isRecord(item) ? getString(item, 'text') : ''))
        .join('')
    : ''
  const choices =
    isRecord(json) && Array.isArray(json.choices) ? json.choices : []
  const firstChoice =
    choices.length > 0 && isRecord(choices[0]) ? choices[0] : null
  const message = getRecord(firstChoice, 'message')

  return (
    getString(message, 'content') ||
    getString(message, 'reasoning_content') ||
    getString(firstChoice, 'text') ||
    contentText ||
    getString(json, 'output_text') ||
    getString(json, 'raw')
  )
}

function extractErrorMessage(json: unknown): string {
  const error = getRecord(json, 'error')
  const data = getRecord(json, 'data')
  const dataError = getRecord(data, 'error')
  return (
    getString(error, 'message') ||
    getString(json, 'message') ||
    getString(dataError, 'message') ||
    getString(data, 'error') ||
    getString(json, 'fail_reason') ||
    getString(json, 'raw')
  )
}

function extractUrls(json: unknown): string[] {
  const urls: string[] = []
  const seen = new Set<string>()

  const add = (value: unknown) => {
    if (typeof value !== 'string') return
    const trimmed = value.trim()
    if (!/^https?:\/\//i.test(trimmed)) return
    if (seen.has(trimmed)) return
    seen.add(trimmed)
    urls.push(trimmed)
  }

  const walk = (value: unknown, key = '') => {
    if (value == null) return
    if (typeof value === 'string') {
      if (
        /url|output|audio|video|image/i.test(key) ||
        /^https?:\/\//i.test(value)
      ) {
        add(value)
      }
      return
    }
    if (Array.isArray(value)) {
      value.forEach((item) => walk(item, key))
      return
    }
    if (isRecord(value)) {
      Object.entries(value).forEach(([childKey, childValue]) =>
        walk(childValue, childKey)
      )
    }
  }

  walk(json)
  return urls
}

function isComplete(status: string, urls: string[]) {
  return (
    ['succeeded', 'completed', 'success'].includes(status) ||
    (urls.length > 0 && status !== 'failed')
  )
}

function isFailed(status: string, json: unknown) {
  return (
    ['failed', 'error', 'cancelled', 'canceled'].includes(status) ||
    Boolean(
      getRecord(json, 'error') || getRecord(getRecord(json, 'data'), 'error')
    )
  )
}

function formatElapsed(seconds: number): string {
  const minutes = String(Math.floor(seconds / 60)).padStart(2, '0')
  const rest = String(seconds % 60).padStart(2, '0')
  return `${minutes}:${rest}`
}

function getBadgeVariant(tone: StatusTone) {
  if (tone === 'danger') return 'destructive'
  if (tone === 'warning' || tone === 'success') return 'secondary'
  return 'outline'
}

export function TestBench() {
  const { t } = useTranslation()
  const stored = useMemo(parseStoredConfig, [])
  const initialModel = stored.selectedModel || DEFAULT_TEST_MODEL

  const apiKeysQuery = useQuery({
    queryKey: ['test-bench-api-keys'],
    queryFn: async () => {
      const result = await getApiKeys({ p: 1, size: 100 })
      if (!result.success) {
        throw new Error(result.message || 'Failed to load API keys')
      }
      return result.data?.items ?? []
    },
    staleTime: 60 * 1000,
  })

  const userModelsQuery = useQuery({
    queryKey: ['test-bench-user-models'],
    queryFn: async () => {
      const result = await getUserModels()
      if (!result.success) {
        throw new Error(result.message || 'Failed to load models')
      }
      return result.data ?? []
    },
    staleTime: 60 * 1000,
  })

  const [baseUrl, setBaseUrl] = useState(() => getInitialBaseUrl(stored.baseUrl))
  const [apiToken, setApiToken] = useState(stored.token || '')
  const [selectedApiKeyId, setSelectedApiKeyId] = useState(
    stored.selectedApiKeyId != null
      ? String(stored.selectedApiKeyId)
      : MANUAL_API_KEY_VALUE
  )
  const [loadingApiKey, setLoadingApiKey] = useState(false)
  const [selectedModelId, setSelectedModelId] = useState(initialModel)
  const apiKeys = useMemo(() => apiKeysQuery.data ?? [], [apiKeysQuery.data])
  const enabledApiKeys = useMemo(
    () => apiKeys.filter((key) => key.status === API_KEY_STATUS.ENABLED),
    [apiKeys]
  )
  const dynamicModelIds = useMemo(
    () => userModelsQuery.data ?? [],
    [userModelsQuery.data]
  )
  const modelOptions = useMemo(
    () => buildModelOptions(dynamicModelIds),
    [dynamicModelIds]
  )
  const selectedModel = useMemo(
    () =>
      modelOptions.find((model) => model.id === selectedModelId) ||
      (selectedModelId ? createDynamicModel(selectedModelId) : null) ||
      modelOptions[0] ||
      TEST_MODELS[0],
    [modelOptions, selectedModelId]
  )

  const [formValues, setFormValues] = useState<FormValues>(() =>
    getDefaultFormValues(selectedModel)
  )
  const [files, setFiles] = useState<FileValues>({})
  const [rawMode, setRawMode] = useState(false)
  const [noPoll, setNoPoll] = useState(false)
  const [pollMaxAttempts, setPollMaxAttempts] = useState(() =>
    getInitialPollMaxAttempts(stored.pollMaxAttempts)
  )
  const [requestJson, setRequestJson] = useState(() =>
    stableJson(
      buildRequestBody(selectedModel, getDefaultFormValues(selectedModel))
    )
  )
  const [responseJson, setResponseJson] = useState('')
  const [resultUrls, setResultUrls] = useState<string[]>([])
  const [preview, setPreview] = useState<PreviewState>({
    type: 'empty',
    messageKey: EMPTY_PREVIEW_KEY,
  })
  const [history, setHistory] = useState<HistoryItem[]>(parseHistory)
  const [progress, setProgress] = useState(0)
  const [running, setRunning] = useState(false)
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [elapsed, setElapsed] = useState('00:00')
  const [lastTaskId, setLastTaskId] = useState('')
  const [lastQueryEndpoint, setLastQueryEndpoint] = useState('')
  const [connectionCode, setConnectionCode] = useState('ready')
  const [saveCode, setSaveCode] = useState('local')
  const [status, setStatus] = useState<RunStatus>({
    code: 'idle',
    title: t('Awaiting test'),
    text: t(DEFAULT_STATUS_KEY),
    tone: 'success',
  })

  useEffect(() => {
    if (selectedApiKeyId !== MANUAL_API_KEY_VALUE || apiToken) return
    const firstEnabledKey = enabledApiKeys[0]
    if (firstEnabledKey) setSelectedApiKeyId(String(firstEnabledKey.id))
  }, [apiToken, enabledApiKeys, selectedApiKeyId])

  useEffect(() => {
    if (selectedApiKeyId === MANUAL_API_KEY_VALUE) return
    const tokenId = Number(selectedApiKeyId)
    if (!Number.isFinite(tokenId)) return

    let active = true
    setLoadingApiKey(true)
    fetchTokenKey(tokenId)
      .then((result) => {
        if (!active) return
        if (!result.success || !result.data?.key) {
          throw new Error(result.message || 'Failed to load API key')
        }
        setApiToken(ensureApiKeyPrefix(result.data.key))
        persistConfig({ selectedApiKeyId: tokenId, token: '' })
      })
      .catch((error) => {
        if (!active) return
        toast.error(error instanceof Error ? error.message : String(error))
      })
      .finally(() => {
        if (active) setLoadingApiKey(false)
      })

    return () => {
      active = false
    }
  }, [selectedApiKeyId])

  useEffect(() => {
    if (modelOptions.some((model) => model.id === selectedModelId)) return
    setSelectedModelId(modelOptions[0]?.id || DEFAULT_TEST_MODEL)
  }, [modelOptions, selectedModelId])

  useEffect(() => {
    const nextValues = getDefaultFormValues(selectedModel)
    setFormValues(nextValues)
    setFiles({})
    setRequestJson(stableJson(buildRequestBody(selectedModel, nextValues)))
    persistConfig({ selectedModel: selectedModel.id })
  }, [selectedModel])

  useEffect(() => {
    if (rawMode) return
    setRequestJson(stableJson(buildRequestBody(selectedModel, formValues)))
  }, [formValues, rawMode, selectedModel])

  useEffect(() => {
    if (!running || !startedAt) return
    const update = () => {
      setElapsed(formatElapsed(Math.floor((Date.now() - startedAt) / 1000)))
    }
    update()
    const timer = window.setInterval(update, 250)
    return () => window.clearInterval(timer)
  }, [running, startedAt])

  const setDefaultStatus = useCallback(() => {
    setStatus({
      code: 'idle',
      title: t('Awaiting test'),
      text: t(DEFAULT_STATUS_KEY),
      tone: 'success',
    })
  }, [t])

  const clearPreview = useCallback((messageKey = EMPTY_PREVIEW_KEY) => {
    setPreview({ type: 'empty', messageKey })
    setResultUrls([])
  }, [])

  const renderJsonResult = useCallback((json: unknown) => {
    setResponseJson(typeof json === 'string' ? json : stableJson(json))
    setResultUrls(extractUrls(json))
  }, [])

  const renderResult = useCallback(
    (model: TestModel, json: unknown, urls = extractUrls(json)) => {
      setResultUrls(urls)
      if (model.kind === 'text') {
        setPreview({
          type: 'text',
          text: extractText(json) || t('No text content found in response.'),
        })
        return
      }

      const firstUrl = urls[0]
      if (firstUrl) {
        setPreview({ type: 'media', kind: model.kind, url: firstUrl })
      } else {
        setPreview({
          type: 'empty',
          messageKey: 'No previewable result URL found in the task response.',
        })
      }
    },
    [t]
  )

  const addHistory = useCallback(
    (model: TestModel, statusCode: string, taskId: string) => {
      const item: HistoryItem = {
        model: model.id,
        label: model.label,
        labelKey: model.label ? undefined : model.labelKey,
        status: statusCode,
        taskId,
        time: new Date().toLocaleString(undefined, { hour12: false }),
      }
      setHistory((current) => {
        const next = [item, ...current].slice(0, 12)
        window.localStorage.setItem(
          MODEL_TEST_HISTORY_KEY,
          JSON.stringify(next)
        )
        return next
      })
    },
    []
  )

  const handleApiKeySelection = (value: string) => {
    setSelectedApiKeyId(value)
    if (value === MANUAL_API_KEY_VALUE) {
      setApiToken(stored.token || '')
      persistConfig({ selectedApiKeyId: value })
    } else {
      setApiToken('')
      persistConfig({ selectedApiKeyId: Number(value), token: '' })
    }
  }

  const updateField = (field: TestField, value: FormValue) => {
    setFormValues((current) => ({
      ...current,
      [field.key]: value,
    }))
  }

  const updatePollMaxAttempts = (value: string) => {
    if (/^\d*$/.test(value)) setPollMaxAttempts(value)
  }

  const getRequestBodyFromEditor = () => {
    if (!rawMode) return buildRequestBody(selectedModel, formValues)
    const text = requestJson.trim()
    if (!text) throw new Error(t('Raw JSON is empty'))
    const parsed = JSON.parse(text)
    if (!isRecord(parsed))
      throw new Error(t('request.json must be a JSON object'))
    return parsed
  }

  const saveConfig = () => {
    const normalized = normalizeBaseUrl(baseUrl)
    const normalizedPollMaxAttempts = parsePollMaxAttempts(pollMaxAttempts)
    if (!normalizedPollMaxAttempts) {
      toast.error(t('Polling limit must be a positive integer'))
      return
    }
    setBaseUrl(normalized)
    setPollMaxAttempts(String(normalizedPollMaxAttempts))
    persistConfig({
      baseUrl: normalized,
      token:
        selectedApiKeyId === MANUAL_API_KEY_VALUE
          ? ensureApiKeyPrefix(apiToken)
          : '',
      selectedApiKeyId:
        selectedApiKeyId === MANUAL_API_KEY_VALUE
          ? selectedApiKeyId
          : Number(selectedApiKeyId),
      selectedModel: selectedModel.id,
      pollMaxAttempts: normalizedPollMaxAttempts,
    })
    setSaveCode('saved')
    toast.success(t('Configuration saved to this browser'))
  }

  const forgetConfig = () => {
    window.localStorage.removeItem(MODEL_TEST_STORAGE_KEY)
    setApiToken('')
    setSelectedApiKeyId(MANUAL_API_KEY_VALUE)
    setBaseUrl(normalizeBaseUrl())
    setPollMaxAttempts(String(DEFAULT_POLL_MAX_ATTEMPTS))
    setSaveCode('local')
    toast.success(t('Local connection configuration cleared'))
  }

  const copyText = async (text: string, messageKey: string) => {
    const ok = await copyToClipboard(text || '')
    if (ok) {
      toast.success(t(messageKey))
    } else {
      toast.error(t('Copy failed'))
    }
  }

  const handleStreamResponse = async (response: Response) => {
    if (!response.ok) {
      const json = await readJsonOrText(response)
      renderJsonResult(json)
      throw new Error(extractErrorMessage(json) || `HTTP ${response.status}`)
    }

    if (!response.body) {
      const json = await readJsonOrText(response)
      renderJsonResult(json)
      setPreview({
        type: 'text',
        text: extractText(json) || t('No text content found in response.'),
      })
      return
    }

    setStatus({
      code: 'streaming',
      title: t('Streaming response'),
      text: 'SSE stream',
      tone: 'warning',
    })
    setProgress(38)

    const reader = response.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    let text = ''
    let raw = ''

    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      raw += buffer
      const lines = buffer.split(/\r?\n/)
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        try {
          const item = JSON.parse(data)
          const choices =
            isRecord(item) && Array.isArray(item.choices) ? item.choices : []
          const firstChoice =
            choices.length > 0 && isRecord(choices[0]) ? choices[0] : null
          const delta = getRecord(firstChoice, 'delta')
          text += getString(delta, 'content') || getString(firstChoice, 'text')
          setPreview({ type: 'text', text })
        } catch {
          /* Keep malformed SSE chunks in responseJson only. */
        }
      }
      setProgress(Math.min(92, 38 + text.length / 40))
    }

    setResponseJson(raw || text)
  }

  const pollTask = useCallback(
    async (taskIdOverride?: string, modelOverride?: TestModel) => {
      const model = modelOverride || selectedModel
      let taskId = taskIdOverride || lastTaskId
      if (!taskId) {
        taskId = window.prompt(t('Enter task_id')) || ''
        if (!taskId) return
      }
      const token = ensureApiKeyPrefix(apiToken)
      if (!token) {
        toast.error(t('Please select or enter an API Key first'))
        return
      }
      const queryTemplate = model.queryEndpoint || lastQueryEndpoint
      if (!queryTemplate) {
        toast.error(t('Current model has no query endpoint'))
        return
      }
      const maxAttempts = parsePollMaxAttempts(pollMaxAttempts)
      if (!maxAttempts) {
        toast.error(t('Polling limit must be a positive integer'))
        return
      }

      setRunning(true)
      setStartedAt((current) => current || Date.now())
      const intervalMs = 5000

      try {
        for (let attempt = 1; attempt <= maxAttempts; attempt++) {
          setStatus({
            code: 'polling',
            title: t('Polling {{current}}/{{total}}', {
              current: attempt,
              total: maxAttempts,
            }),
            text: `task_id: ${taskId}`,
            tone: 'warning',
          })
          setProgress(Math.min(95, 42 + attempt * 0.55))

          const url =
            normalizeBaseUrl(baseUrl) +
            queryTemplate.replace('{task_id}', encodeURIComponent(taskId))
          const response = await fetch(url, {
            headers: { Authorization: `Bearer ${token}` },
          })
          const json = await readJsonOrText(response)
          renderJsonResult(json)
          if (!response.ok) {
            throw new Error(
              extractErrorMessage(json) || `HTTP ${response.status}`
            )
          }

          const responseStatus = extractStatus(json)
          const urls = extractUrls(json)
          if (isFailed(responseStatus, json)) {
            renderResult(model, json, urls)
            throw new Error(
              extractErrorMessage(json) ||
                t('Task failed: {{status}}', {
                  status: responseStatus || 'failed',
                })
            )
          }
          if (isComplete(responseStatus, urls)) {
            renderResult(model, json, urls)
            setStatus({
              code: 'complete',
              title: t('Task complete'),
              text: urls[0] || responseStatus || 'completed',
              tone: 'success',
            })
            setProgress(100)
            addHistory(model, 'complete', taskId)
            return
          }
          if (attempt < maxAttempts) {
            await new Promise((resolve) =>
              window.setTimeout(resolve, intervalMs)
            )
          }
        }

        setStatus({
          code: 'timeout',
          title: t('Polling stopped'),
          text: t(
            'Polled {{count}} times and task is still not complete. You can query again later.',
            { count: maxAttempts }
          ),
          tone: 'danger',
        })
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        setStatus({
          code: 'failed',
          title: t('Polling failed'),
          text: message,
          tone: 'danger',
        })
        toast.error(message)
      } finally {
        setRunning(false)
      }
    },
    [
      addHistory,
      apiToken,
      baseUrl,
      lastQueryEndpoint,
      lastTaskId,
      pollMaxAttempts,
      renderJsonResult,
      renderResult,
      selectedModel,
      t,
    ]
  )

  const runTest = async () => {
    const token = ensureApiKeyPrefix(apiToken)
    if (!token) {
      toast.error(t('Please select or enter an API Key first'))
      return
    }

    let body: JsonRecord
    try {
      body = getRequestBodyFromEditor()
      validateRequiredFields(
        selectedModel,
        body,
        rawMode ? null : formValues,
        files,
        t
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error))
      return
    }

    setRunning(true)
    setStartedAt(Date.now())
    setElapsed('00:00')
    setStatus({
      code: 'requesting',
      title: t('Sending request'),
      text: `${selectedModel.method} ${selectedModel.endpoint}`,
      tone: 'warning',
    })
    setProgress(12)
    setResponseJson('')
    clearPreview()
    setRequestJson(stableJson(body))

    try {
      const url = normalizeBaseUrl(baseUrl) + selectedModel.endpoint
      const payload = buildFetchPayload(body, files)
      const response = await fetch(url, {
        method: selectedModel.method,
        headers: {
          Authorization: `Bearer ${token}`,
          ...payload.headers,
        },
        body: payload.body,
      })

      if (body.stream === true && selectedModel.kind === 'text') {
        await handleStreamResponse(response)
        setStatus({
          code: 'complete',
          title: t('Stream response complete'),
          text: getModelDisplayLabel(selectedModel, t),
          tone: 'success',
        })
        setProgress(100)
        addHistory(selectedModel, 'stream', '')
        return
      }

      const json = await readJsonOrText(response)
      renderJsonResult(json)
      if (!response.ok) {
        throw new Error(extractErrorMessage(json) || `HTTP ${response.status}`)
      }

      if (selectedModel.mode === 'async') {
        const taskId = extractTaskId(json)
        if (!taskId)
          throw new Error(t('Created successfully but no task_id found'))
        setLastTaskId(taskId)
        setLastQueryEndpoint(selectedModel.queryEndpoint || '')
        addHistory(selectedModel, 'queued', taskId)
        setStatus({
          code: 'queued',
          title: t('Async task created'),
          text: `task_id: ${taskId}`,
          tone: 'warning',
        })
        setProgress(noPoll ? 35 : 42)
        if (!noPoll) {
          await pollTask(taskId, selectedModel)
        }
      } else {
        const urls = extractUrls(json)
        renderResult(selectedModel, json, urls)
        setStatus({
          code: 'complete',
          title: t('Sync response complete'),
          text: getModelDisplayLabel(selectedModel, t),
          tone: 'success',
        })
        setProgress(100)
        addHistory(selectedModel, 'complete', '')
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setStatus({
        code: 'failed',
        title: t('Request failed'),
        text: message,
        tone: 'danger',
      })
      setProgress(100)
      toast.error(message)
    } finally {
      setRunning(false)
    }
  }

  const testModelsEndpoint = async () => {
    const token = ensureApiKeyPrefix(apiToken)
    if (!token) {
      toast.error(t('Please select or enter an API Key first'))
      return
    }

    setConnectionCode('checking')
    try {
      const response = await fetch(`${normalizeBaseUrl(baseUrl)}/v1/models`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      const json = await readJsonOrText(response)
      renderJsonResult(json)
      if (!response.ok) {
        throw new Error(extractErrorMessage(json) || `HTTP ${response.status}`)
      }

      const data = isRecord(json) && Array.isArray(json.data) ? json.data : []
      const ids = data
        .map((item) => (isRecord(item) ? getString(item, 'id') : ''))
        .filter(Boolean)
      setConnectionCode('connected')
      setStatus({
        code: 'models',
        title: t('Model list validation complete'),
        text: t('Returned {{count}} models', { count: ids.length }),
        tone: 'success',
      })
      toast.success(
        t('Model list validated: {{count}} models', { count: ids.length })
      )
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      setConnectionCode('failed')
      toast.error(message)
    }
  }

  const clearResult = () => {
    setResponseJson('')
    setLastTaskId('')
    setStartedAt(null)
    setElapsed('00:00')
    setProgress(0)
    setDefaultStatus()
    clearPreview()
  }

  const clearHistory = () => {
    window.localStorage.removeItem(MODEL_TEST_HISTORY_KEY)
    setHistory([])
    toast.success(t('History cleared'))
  }

  const formatRequestJson = () => {
    try {
      setRequestJson(stableJson(JSON.parse(requestJson)))
    } catch {
      toast.error(t('request.json is not valid JSON'))
    }
  }

  const fillExample = () => {
    const nextValues = getDefaultFormValues(selectedModel)
    setFormValues(nextValues)
    setFiles({})
    setRequestJson(stableJson(buildRequestBody(selectedModel, nextValues)))
    toast.success(t('Example parameters filled'))
  }

  const modelLabel = getModelDisplayLabel(selectedModel, t)
  const selectedApiKey = apiKeys.find(
    (key) => String(key.id) === selectedApiKeyId
  )
  const apiKeyHint =
    selectedApiKeyId === MANUAL_API_KEY_VALUE
      ? t('Manual API Key')
      : selectedApiKey?.name || t('Loading API key...')
  const modelSelectionDescription = userModelsQuery.isLoading
    ? t('Loading models...')
    : dynamicModelIds.length > 0
      ? t('{{count}} account models loaded', { count: dynamicModelIds.length })
      : t('{{count}} built-in test templates', { count: TEST_MODELS.length })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Test Bench')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          render={
            <a
              href='/docs/aipdd-user-guide.zh_CN.md'
              target='_blank'
              rel='noreferrer'
            />
          }
        >
          <ExternalLink className='h-4 w-4' />
          {t('Request docs')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='grid gap-4 xl:grid-cols-[390px_minmax(0,1fr)]'>
          <aside className='space-y-4'>
            <TitledCard
              title={t('Connection configuration')}
              description={t('API Keys are loaded from your account.')}
              icon={<Settings2 className='h-4 w-4' />}
              action={<StateBadge code={connectionCode} />}
              contentClassName='space-y-4'
            >
              <div className='space-y-2'>
                <FieldLabel
                  label={t('API endpoint')}
                  hint={t('From environment variable')}
                />
                <Input
                  value={baseUrl}
                  onChange={(event) => setBaseUrl(event.target.value)}
                  autoComplete='off'
                  className='font-mono text-xs'
                />
              </div>
              <div className='space-y-2'>
                <FieldLabel label={t('API Key')} hint={apiKeyHint} />
                <select
                  className={selectClassName}
                  value={selectedApiKeyId}
                  onChange={(event) =>
                    handleApiKeySelection(event.target.value)
                  }
                  disabled={apiKeysQuery.isLoading}
                >
                  <option value={MANUAL_API_KEY_VALUE}>
                    {apiKeysQuery.isLoading
                      ? t('Loading API keys...')
                      : t('Manual API Key')}
                  </option>
                  {enabledApiKeys.map((apiKey) => (
                    <option key={apiKey.id} value={apiKey.id}>
                      {getApiKeyOptionLabel(apiKey)}
                    </option>
                  ))}
                </select>
                {selectedApiKeyId === MANUAL_API_KEY_VALUE ? (
                  <Input
                    type='password'
                    value={apiToken}
                    onChange={(event) => setApiToken(event.target.value)}
                    placeholder='sk-xxxx'
                    autoComplete='off'
                    className='font-mono text-xs'
                  />
                ) : (
                  <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-xs leading-relaxed'>
                    {loadingApiKey
                      ? t('Loading selected API key...')
                      : t('Selected API key will be loaded before requests.')}
                  </div>
                )}
                {!apiKeysQuery.isLoading && enabledApiKeys.length === 0 ? (
                  <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-xs leading-relaxed'>
                    {t(
                      'No enabled API keys found. Use manual mode or create one first.'
                    )}
                  </div>
                ) : null}
              </div>
              <div className='flex flex-wrap gap-2'>
                <Button type='button' size='sm' onClick={saveConfig}>
                  <Save className='h-4 w-4' />
                  {t('Save configuration')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={testModelsEndpoint}
                >
                  <Search className='h-4 w-4' />
                  {t('Validate models')}
                </Button>
                <Button
                  type='button'
                  variant='destructive'
                  size='sm'
                  onClick={forgetConfig}
                >
                  <Eraser className='h-4 w-4' />
                  {t('Clear')}
                </Button>
              </div>
              <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-2 text-xs leading-relaxed'>
                {t(
                  'Selected API Key IDs are saved locally. Manual API Keys are stored only in this browser.'
                )}
              </div>
              <StateBadge code={saveCode} className='w-fit' />
            </TitledCard>

            <TitledCard
              title={t('Model selection')}
              description={modelSelectionDescription}
              icon={<FlaskConical className='h-4 w-4' />}
              action={<ModeBadge mode={selectedModel.mode} />}
              contentClassName='space-y-4'
            >
              <div className='space-y-2'>
                <FieldLabel label={t('Select model')} hint={selectedModel.id} />
                <select
                  className={selectClassName}
                  value={selectedModel.id}
                  onChange={(event) => setSelectedModelId(event.target.value)}
                >
                  {modelOptions.map((model) => (
                    <option key={model.id} value={model.id}>
                      {getModelDisplayLabel(model, t)}
                    </option>
                  ))}
                </select>
              </div>
              <div className='bg-muted/20 rounded-lg border p-3'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0'>
                    <div className='truncate text-sm font-semibold'>
                      {modelLabel}
                    </div>
                    <div className='text-muted-foreground mt-1 truncate font-mono text-xs'>
                      {selectedModel.id} · {selectedModel.vendor} ·{' '}
                      {t(selectedModel.billingKey)}
                    </div>
                  </div>
                  <Badge variant='outline'>{selectedModel.kind}</Badge>
                </div>
              </div>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t(selectedModel.noteKey)}
              </p>
              {userModelsQuery.isError ? (
                <p className='text-muted-foreground text-xs leading-relaxed'>
                  {t(
                    'Could not load account models. Built-in templates are shown.'
                  )}
                </p>
              ) : null}
            </TitledCard>

            <TitledCard
              title={t('Request parameters')}
              description={`${selectedModel.method} ${selectedModel.endpoint}`}
              icon={<FileJson className='h-4 w-4' />}
              action={
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={fillExample}
                >
                  <WandSparkles className='h-4 w-4' />
                  {t('Fill example')}
                </Button>
              }
              contentClassName='space-y-4'
            >
              <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-1'>
                {selectedModel.fields.map((field) => (
                  <ParameterField
                    key={field.key}
                    field={field}
                    value={formValues[field.key]}
                    file={files[field.key]}
                    onChange={(value) => updateField(field, value)}
                    onFileChange={(file) =>
                      setFiles((current) => {
                        const next = { ...current }
                        if (file) next[field.key] = file
                        else delete next[field.key]
                        return next
                      })
                    }
                  />
                ))}
              </div>
              <ToggleLine
                checked={noPoll}
                onCheckedChange={setNoPoll}
                label={t('Create async task without auto polling')}
              />
              <div className='space-y-2'>
                <FieldLabel label={t('Polling limit')} hint={t('times')} />
                <Input
                  type='number'
                  min={1}
                  step={1}
                  inputMode='numeric'
                  pattern='[0-9]*'
                  value={pollMaxAttempts}
                  onChange={(event) =>
                    updatePollMaxAttempts(event.target.value)
                  }
                  onBlur={() => {
                    const normalized =
                      parsePollMaxAttempts(pollMaxAttempts) ||
                      DEFAULT_POLL_MAX_ATTEMPTS
                    setPollMaxAttempts(String(normalized))
                  }}
                  className='text-sm'
                />
                <p className='text-muted-foreground text-xs leading-relaxed'>
                  {t(
                    'Maximum automatic polling attempts. Each attempt waits 5 seconds.'
                  )}
                </p>
              </div>
              <ToggleLine
                checked={rawMode}
                onCheckedChange={setRawMode}
                label={t('Override form parameters with raw JSON below')}
              />
              <div className='flex flex-wrap gap-2'>
                <Button
                  type='button'
                  size='sm'
                  onClick={runTest}
                  disabled={running}
                >
                  {running ? (
                    <Loader2 className='h-4 w-4 animate-spin' />
                  ) : (
                    <Play className='h-4 w-4' />
                  )}
                  {t('Start test')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => void pollTask()}
                  disabled={running}
                >
                  <RefreshCw className='h-4 w-4' />
                  {t('Query task')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    void copyText(requestJson, 'request.json copied')
                  }
                >
                  <Copy className='h-4 w-4' />
                  {t('Copy request')}
                </Button>
                <Button
                  type='button'
                  variant='destructive'
                  size='sm'
                  onClick={clearResult}
                >
                  <Eraser className='h-4 w-4' />
                  {t('Clear result')}
                </Button>
              </div>
            </TitledCard>

            <TitledCard
              title={t('Recent tasks')}
              icon={<History className='h-4 w-4' />}
              action={
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={clearHistory}
                >
                  {t('Clear')}
                </Button>
              }
              contentClassName='space-y-2'
            >
              {history.length === 0 ? (
                <div className='text-muted-foreground rounded-lg border border-dashed px-3 py-4 text-sm'>
                  {t('No task records')}
                </div>
              ) : (
                history.map((item, index) => {
                  const model =
                    modelOptions.find(
                      (candidate) => candidate.id === item.model
                    ) || selectedModel
                  return (
                    <button
                      key={`${item.model}-${item.taskId}-${item.time}-${index}`}
                      type='button'
                      className='hover:bg-muted/60 bg-muted/20 w-full rounded-lg border p-3 text-left transition-colors'
                      onClick={() => {
                        setSelectedModelId(model.id)
                        if (item.taskId) void pollTask(item.taskId, model)
                      }}
                    >
                      <div className='truncate text-sm font-medium'>
                        {item.label ||
                          (item.labelKey
                            ? t(item.labelKey)
                            : getModelDisplayLabel(model, t))}
                      </div>
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {item.status || '-'} · {item.time || '-'}
                      </div>
                      {item.taskId ? (
                        <div className='text-muted-foreground mt-1 truncate font-mono text-xs'>
                          {item.taskId}
                        </div>
                      ) : null}
                    </button>
                  )
                })
              )}
            </TitledCard>
          </aside>

          <section className='min-w-0 space-y-4'>
            <div className='bg-card text-card-foreground rounded-xl border p-4 shadow-sm'>
              <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
                <div className='min-w-0'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <h3 className='text-base font-semibold'>{status.title}</h3>
                    <Badge
                      variant={getBadgeVariant(status.tone)}
                      className={cn(
                        status.tone === 'success' &&
                          'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
                        status.tone === 'warning' &&
                          'bg-amber-500/10 text-amber-700 dark:text-amber-300'
                      )}
                    >
                      {status.code}
                    </Badge>
                    <Badge variant='outline' className='gap-1 font-mono'>
                      <Clock3 className='h-3 w-3' />
                      {elapsed}
                    </Badge>
                  </div>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {status.text}
                  </p>
                </div>
                <div className='min-w-44'>
                  <div className='bg-muted h-2 overflow-hidden rounded-full'>
                    <div
                      className='bg-primary h-full rounded-full transition-all'
                      style={{
                        width: `${Math.max(0, Math.min(100, progress))}%`,
                      }}
                    />
                  </div>
                </div>
              </div>
            </div>

            <div className='bg-muted/20 grid min-h-[420px] place-items-center overflow-hidden rounded-xl border'>
              <Preview preview={preview} />
            </div>

            <div className='grid gap-4 lg:grid-cols-2'>
              <TitledCard
                title='request.json'
                icon={<FileJson className='h-4 w-4' />}
                action={
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={formatRequestJson}
                  >
                    {t('Format')}
                  </Button>
                }
                contentClassName='p-0'
              >
                <Textarea
                  value={requestJson}
                  onChange={(event) => setRequestJson(event.target.value)}
                  readOnly={!rawMode}
                  spellCheck={false}
                  className='min-h-[320px] resize-y rounded-none border-0 bg-transparent p-4 font-mono text-xs focus-visible:ring-0'
                />
              </TitledCard>

              <TitledCard
                title='response.json'
                icon={<FileJson className='h-4 w-4' />}
                action={
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() =>
                      void copyText(responseJson, 'response.json copied')
                    }
                  >
                    <Copy className='h-4 w-4' />
                    {t('Copy')}
                  </Button>
                }
                contentClassName='p-0'
              >
                <Textarea
                  value={responseJson}
                  readOnly
                  spellCheck={false}
                  className='min-h-[280px] resize-y rounded-none border-0 bg-transparent p-4 font-mono text-xs focus-visible:ring-0'
                />
                <div className='flex flex-wrap gap-2 border-t p-3'>
                  {resultUrls.length === 0 ? (
                    <span className='text-muted-foreground text-xs'>
                      {t('No result links')}
                    </span>
                  ) : (
                    resultUrls.map((url, index) => (
                      <a
                        key={url}
                        href={url}
                        target='_blank'
                        rel='noreferrer'
                        className='bg-primary/10 text-primary max-w-full truncate rounded-full border px-3 py-1 font-mono text-xs'
                      >
                        {t('Generated result {{index}}', { index: index + 1 })}:{' '}
                        {url}
                      </a>
                    ))
                  )}
                </div>
              </TitledCard>
            </div>
          </section>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function FieldLabel({ label, hint }: { label: string; hint?: string }) {
  return (
    <div className='flex items-center justify-between gap-2'>
      <Label className='text-xs font-semibold'>{label}</Label>
      {hint ? (
        <span className='text-muted-foreground truncate text-xs'>{hint}</span>
      ) : null}
    </div>
  )
}

function StateBadge({ code, className }: { code: string; className?: string }) {
  const { t } = useTranslation()
  const success = ['ready', 'connected', 'saved', 'local'].includes(code)
  const warning = code === 'checking'
  return (
    <Badge
      variant={success || warning ? 'secondary' : 'destructive'}
      className={cn(
        success && 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
        warning && 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
        className
      )}
    >
      {code === 'local' ? t('Local saved') : t(code)}
    </Badge>
  )
}

function ModeBadge({ mode }: { mode: TestModel['mode'] }) {
  return (
    <Badge
      variant='secondary'
      className={cn(
        mode === 'sync'
          ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
          : 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
      )}
    >
      {mode}
    </Badge>
  )
}

function ToggleLine({
  checked,
  onCheckedChange,
  label,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  label: string
}) {
  return (
    <label className='flex items-center gap-2 text-sm'>
      <Checkbox
        checked={checked}
        onCheckedChange={(next) => onCheckedChange(next === true)}
      />
      <span>{label}</span>
    </label>
  )
}

function ParameterField({
  field,
  value,
  file,
  onChange,
  onFileChange,
}: {
  field: TestField
  value: FormValue | undefined
  file?: File
  onChange: (value: FormValue) => void
  onFileChange: (file: File | null) => void
}) {
  const { t } = useTranslation()
  const requiredHint = field.required ? t('Required') : t('Optional')
  const wide = field.type === 'textarea' || field.file

  if (field.type === 'checkbox') {
    return (
      <div className={cn('space-y-2', wide && 'sm:col-span-2 xl:col-span-1')}>
        <ToggleLine
          checked={value === true}
          onCheckedChange={onChange}
          label={t(field.labelKey)}
        />
      </div>
    )
  }

  return (
    <div className={cn('space-y-2', wide && 'sm:col-span-2 xl:col-span-1')}>
      <FieldLabel
        label={`${t(field.labelKey)}${field.required ? ' *' : ''}`}
        hint={requiredHint}
      />
      {field.type === 'textarea' ? (
        <Textarea
          value={String(value ?? '')}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t(field.labelKey)}
          className='min-h-24 text-sm'
        />
      ) : field.type === 'select' ? (
        <select
          className={selectClassName}
          value={String(value ?? '')}
          onChange={(event) => onChange(event.target.value)}
        >
          {(field.options || []).map((option) => (
            <option key={option} value={option}>
              {option}
            </option>
          ))}
        </select>
      ) : (
        <Input
          type={field.type === 'url' ? 'url' : field.type}
          value={String(value ?? '')}
          onChange={(event) => onChange(event.target.value)}
          placeholder={t(field.labelKey)}
          className='text-sm'
          {...field.attrs}
        />
      )}
      {field.file ? (
        <div className='space-y-2 pt-1'>
          <FieldLabel
            label={t(field.fileLabelKey || 'Upload file')}
            hint={t('Uploaded file takes priority')}
          />
          <Input
            type='file'
            onChange={(event) => onFileChange(event.target.files?.[0] || null)}
          />
          {file ? (
            <div className='text-muted-foreground truncate text-xs'>
              {file.name}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}

function Preview({ preview }: { preview: PreviewState }) {
  const { t } = useTranslation()

  if (preview.type === 'text') {
    return (
      <pre className='h-full min-h-[420px] w-full overflow-auto p-6 text-sm leading-7 whitespace-pre-wrap'>
        {preview.text}
      </pre>
    )
  }

  if (preview.type === 'media') {
    if (preview.kind === 'image') {
      return (
        <img
          src={preview.url}
          alt={t('Generated image')}
          className='max-h-[68vh] w-full object-contain'
        />
      )
    }
    if (preview.kind === 'video') {
      return (
        <video
          src={preview.url}
          controls
          playsInline
          className='max-h-[68vh] w-full bg-black object-contain'
        />
      )
    }
    return <audio src={preview.url} controls className='w-4/5 max-w-2xl' />
  }

  return (
    <div className='max-w-xl px-6 py-14 text-center'>
      <div className='bg-primary/10 text-primary mx-auto mb-4 grid h-16 w-16 place-items-center rounded-full'>
        <CheckCircle2 className='h-7 w-7' />
      </div>
      <h3 className='text-lg font-semibold'>{t('Result preview')}</h3>
      <p className='text-muted-foreground mt-2 text-sm leading-relaxed'>
        {t(preview.messageKey)}
      </p>
    </div>
  )
}
