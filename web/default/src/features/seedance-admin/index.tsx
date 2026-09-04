import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Archive,
  Boxes,
  CircleAlert,
  Cog,
  Edit3,
  Gauge,
  Layers3,
  Plus,
  RefreshCw,
  Save,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
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
import { Skeleton } from '@/components/ui/skeleton'
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
import { Textarea } from '@/components/ui/textarea'
import { SectionPageLayout } from '@/components/layout'
import { USER_ROLE } from '@/features/users/constants'
import {
  archiveSeedanceBaseModel,
  archiveSeedanceEnhancementModel,
  archiveSeedanceOffering,
  createSeedanceCredential,
  getSeedanceOverview,
  listSeedanceBaseModels,
  listSeedanceChannels,
  listSeedanceEnhancementModels,
  listSeedanceOrders,
  saveSeedanceBaseModel,
  saveSeedanceConfig,
  saveSeedanceEnhancementModel,
  saveSeedanceOffering,
  saveSeedanceProvider,
  validateSeedanceCredential,
} from './api'
import {
  buildMediaKitSpecification,
  MEDIAKIT_BASE_URL,
  MEDIAKIT_SERVICE_CODE,
  seedanceProviderOptionLabel,
  type MediaKitResolution,
  type MediaKitToolVersion,
} from './lib/enhancement-config'
import type {
  SeedanceBaseCost,
  SeedanceBaseModel,
  SeedanceEnhancementCost,
  SeedanceEnhancementModel,
  SeedanceOffering,
  SeedanceOrder,
  SeedanceOverview,
  SeedanceProvider,
  SeedanceResolution,
} from './types'

const RESOLUTIONS: SeedanceResolution[] = ['480p', '720p', '1080p', '2k', '4k']
const SEEDANCE_MODEL_OPTIONS = [
  {
    value: 'doubao-seedance-2-5',
    label: 'Seedance 2.5',
  },
  {
    value: 'doubao-seedance-2-0',
    label: 'Seedance 2.0',
  },
  {
    value: 'doubao-seedance-2-0-mini',
    label: 'Seedance 2.0 Mini',
  },
  {
    value: 'doubao-seedance-2-0-fast',
    label: 'Seedance 2.0 Fast',
  },
]
const OUTPUT_FPS_OPTIONS = [24, 25, 30, 50, 60].map((fps) => ({
  value: String(fps),
  label: `${fps} FPS`,
}))
const AIPDD_BILLING_URL_OPTIONS = [
  { value: 'https://api.aipdd.work', label: 'AIPDD 正式服务 · api.aipdd.work' },
]
const BASE_COST_TEMPLATE = '[]'
const ENHANCEMENT_COST_TEMPLATE = '[]'
const value = (data: FormData, key: string) =>
  String(data.get(key) ?? '').trim()
const intValue = (data: FormData, key: string) =>
  Number.parseInt(value(data, key) || '0', 10)
const optionalInt = (data: FormData, key: string) =>
  value(data, key) === 'none' || !value(data, key)
    ? undefined
    : intValue(data, key)
const money = (amount?: number) =>
  amount == null ? '-' : `¥${(amount / 1_000_000).toFixed(6)}`
const seconds = (millis?: number) =>
  millis ? `${(millis / 1000).toFixed(3)}s` : '-'
const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error)
const matrixRange = (raw: string) => {
  try {
    const costs = (
      JSON.parse(raw) as Array<{ cost_micro_rmb_per_second: number }>
    ).map((row) => row.cost_micro_rmb_per_second)
    return costs.length
      ? `${money(Math.min(...costs))}–${money(Math.max(...costs))}/s`
      : '-'
  } catch {
    return '-'
  }
}

type Editor = 'base' | 'enhancement' | 'offering' | 'runtime' | null

export function SeedanceAdminPage() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const canEdit = useAuthStore(
    (state) => state.auth.user?.role === USER_ROLE.ROOT
  )
  const [channelId, setChannelId] = useState(0)
  const [editor, setEditor] = useState<Editor>(null)
  const [base, setBase] = useState<SeedanceBaseModel | null>(null)
  const [enhancement, setEnhancement] =
    useState<SeedanceEnhancementModel | null>(null)
  const [offering, setOffering] = useState<SeedanceOffering | null>(null)

  const channels = useQuery({
    queryKey: ['seedance-admin', 'channels'],
    queryFn: listSeedanceChannels,
  })
  const selectedChannelId = channelId || channels.data?.[0]?.id || 0
  const overview = useQuery({
    queryKey: ['seedance-admin', 'overview', selectedChannelId],
    queryFn: () => getSeedanceOverview(selectedChannelId),
    enabled: selectedChannelId > 0,
  })
  const bases = useQuery({
    queryKey: ['seedance-admin', 'base-models'],
    queryFn: listSeedanceBaseModels,
  })
  const enhancements = useQuery({
    queryKey: ['seedance-admin', 'enhancement-models'],
    queryFn: listSeedanceEnhancementModels,
  })
  const orders = useQuery({
    queryKey: ['seedance-admin', 'orders', selectedChannelId],
    queryFn: () => listSeedanceOrders(selectedChannelId),
    enabled: selectedChannelId > 0,
    refetchInterval: 15_000,
  })
  const refresh = () =>
    client.invalidateQueries({ queryKey: ['seedance-admin'] })
  const close = () => setEditor(null)
  const save = useMutation({
    mutationFn: (work: () => Promise<unknown>) => work(),
    onSuccess: () => {
      toast.success(t('已保存'))
      close()
      refresh()
    },
    onError: (error) => toast.error(message(error)),
  })
  const archive = useMutation({
    mutationFn: (work: () => Promise<unknown>) => work(),
    onSuccess: () => {
      toast.success(t('已归档'))
      refresh()
    },
    onError: (error) => toast.error(message(error)),
  })
  const channelOptions = useMemo(
    () =>
      channels.data?.map((item) => ({
        value: String(item.id),
        label: `#${item.id} ${item.name}`,
      })) ?? [],
    [channels.data]
  )
  const loading =
    overview.isLoading || bases.isLoading || enhancements.isLoading
  const failed = overview.isError || bases.isError || enhancements.isError
  const openBase = (item: SeedanceBaseModel | null = null) => {
    setBase(item)
    setEditor('base')
  }
  const openEnhancement = (item: SeedanceEnhancementModel | null = null) => {
    setEnhancement(item)
    setEditor('enhancement')
  }
  const openOffering = (item: SeedanceOffering | null = null) => {
    setOffering(item)
    setEditor('offering')
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Seedance 管理')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Select
          value={selectedChannelId ? String(selectedChannelId) : ''}
          onValueChange={(next) => setChannelId(Number(next))}
          disabled={!channelOptions.length}
        >
          <SelectTrigger className='w-60'>
            <SelectValue placeholder={t('选择 Seedance 渠道')}>
              {(current) =>
                channelOptions.find(
                  (item) => item.value === String(current ?? '')
                )?.label ?? t('选择 Seedance 渠道')
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {channelOptions.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {canEdit && (
          <Button
            variant='outline'
            size='sm'
            onClick={() => setEditor('runtime')}
          >
            <Cog />
            {t('运行设置')}
          </Button>
        )}
        <Button variant='outline' size='sm' onClick={refresh}>
          <RefreshCw />
          {t('刷新')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          {!canEdit && (
            <Alert>
              <ShieldCheck />
              <AlertTitle>{t('管理员只读')}</AlertTitle>
              <AlertDescription>
                {t('仅 Root 可新增、编辑或归档 Seedance 配置。')}
              </AlertDescription>
            </Alert>
          )}
          {!channels.data?.length ? (
            <Alert>
              <CircleAlert />
              <AlertTitle>{t('尚未创建 Seedance 渠道')}</AlertTitle>
              <AlertDescription>
                {t('请先在渠道管理中创建 Seedance 渠道。')}
              </AlertDescription>
            </Alert>
          ) : loading ? (
            <Skeleton className='h-80 w-full' />
          ) : failed || !overview.data ? (
            <Alert variant='destructive'>
              <CircleAlert />
              <AlertTitle>{t('无法加载 Seedance 配置')}</AlertTitle>
              <AlertDescription>{t('请检查权限和后端日志。')}</AlertDescription>
            </Alert>
          ) : (
            <>
              <Tabs defaultValue='base'>
                <TabsList
                  variant='line'
                  className='h-auto max-w-full flex-wrap justify-start'
                >
                  <TabsTrigger value='base'>
                    <Boxes />
                    {t('本体模型')}
                  </TabsTrigger>
                  <TabsTrigger value='enhancement'>
                    <Sparkles />
                    {t('超分模型')}
                  </TabsTrigger>
                  <TabsTrigger value='offering'>
                    <Layers3 />
                    {t('售卖模型')}
                  </TabsTrigger>
                  <TabsTrigger value='orders'>
                    <Gauge />
                    {t('订单利润')}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value='base' className='pt-3'>
                  <Section
                    title={t('本体模型')}
                    description={t(
                      '绑定具体 Seedance 模型；预计成本可选，仅用于账单同步前的利润估算。'
                    )}
                    action={
                      canEdit && (
                        <Button size='sm' onClick={() => openBase()}>
                          <Plus />
                          {t('新增本体模型')}
                        </Button>
                      )
                    }
                  >
                    <BaseTable
                      items={bases.data ?? []}
                      canEdit={canEdit}
                      onEdit={openBase}
                      onArchive={(item) =>
                        archive.mutate(() => archiveSeedanceBaseModel(item.id))
                      }
                    />
                  </Section>
                </TabsContent>
                <TabsContent value='enhancement' className='pt-3'>
                  <Section
                    title={t('超分模型')}
                    description={t(
                      '选择外部节点或火山云直连，并冻结档位和输出规格；预计成本可选。'
                    )}
                    action={
                      canEdit && (
                        <Button size='sm' onClick={() => openEnhancement()}>
                          <Plus />
                          {t('新增超分模型')}
                        </Button>
                      )
                    }
                  >
                    <EnhancementTable
                      items={enhancements.data ?? []}
                      providers={overview.data.providers}
                      canEdit={canEdit}
                      onEdit={openEnhancement}
                      onArchive={(item) =>
                        archive.mutate(() =>
                          archiveSeedanceEnhancementModel(item.id)
                        )
                      }
                    />
                  </Section>
                </TabsContent>
                <TabsContent value='offering' className='pt-3'>
                  <Section
                    title={t('售卖模型')}
                    description={t(
                      '组合本体与超分模型，固定输入输出分辨率和输出 FPS，并配置两种售价。'
                    )}
                    action={
                      canEdit && (
                        <Button size='sm' onClick={() => openOffering()}>
                          <Plus />
                          {t('新增售卖模型')}
                        </Button>
                      )
                    }
                  >
                    <OfferingTable
                      items={overview.data.offerings}
                      bases={bases.data ?? []}
                      enhancements={enhancements.data ?? []}
                      canEdit={canEdit}
                      onEdit={openOffering}
                      onArchive={(item) =>
                        archive.mutate(() => archiveSeedanceOffering(item.id))
                      }
                    />
                  </Section>
                </TabsContent>
                <TabsContent value='orders' className='pt-3'>
                  <Section
                    title={t('订单利润')}
                    description={t(
                      '利润 = 售价 - 超分成本 - 火山云成本；账单确认后同时保留预计与实际利润。'
                    )}
                  >
                    <OrderTable items={orders.data?.items ?? []} />
                  </Section>
                </TabsContent>
              </Tabs>
            </>
          )}
        </div>
        <BaseEditor
          key={`${base?.id ?? 'new'}:${editor === 'base'}`}
          open={editor === 'base'}
          item={base}
          pending={save.isPending}
          onOpenChange={(open) => !open && close()}
          onSave={(data) => save.mutate(() => saveSeedanceBaseModel(data))}
        />
        <EnhancementEditor
          key={`${enhancement?.id ?? 'new'}:${editor === 'enhancement'}`}
          open={editor === 'enhancement'}
          item={enhancement}
          providers={overview.data?.providers ?? []}
          pending={save.isPending}
          onOpenChange={(open) => !open && close()}
          onManageProviders={() => {
            setEnhancement(null)
            setEditor('runtime')
          }}
          onSave={(data) =>
            save.mutate(() => saveSeedanceEnhancementModel(data))
          }
        />
        <OfferingEditor
          key={`${offering?.id ?? 'new'}:${editor === 'offering'}`}
          open={editor === 'offering'}
          item={offering}
          channelId={selectedChannelId}
          bases={bases.data ?? []}
          enhancements={enhancements.data ?? []}
          pending={save.isPending}
          onOpenChange={(open) => !open && close()}
          onSave={(data) => save.mutate(() => saveSeedanceOffering(data))}
        />
        {overview.data && (
          <RuntimeEditor
            key={`${selectedChannelId}:${editor === 'runtime'}`}
            open={editor === 'runtime'}
            channelId={selectedChannelId}
            overview={overview.data}
            pending={save.isPending}
            onOpenChange={(open) => !open && close()}
            onSave={(work) => save.mutate(work)}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function Section({
  title,
  description,
  action,
  children,
}: {
  title: string
  description: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className='bg-card overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-start justify-between gap-3 border-b px-4 py-3'>
        <div>
          <h3 className='text-sm font-semibold'>{title}</h3>
          <p className='text-muted-foreground mt-0.5 text-xs'>{description}</p>
        </div>
        {action}
      </div>
      <div className='overflow-x-auto'>{children}</div>
    </section>
  )
}
function Empty({ columns }: { columns: number }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell
        colSpan={columns}
        className='text-muted-foreground h-24 text-center'
      >
        {t('暂无数据')}
      </TableCell>
    </TableRow>
  )
}
function Actions<T>({
  item,
  onEdit,
  onArchive,
}: {
  item: T
  onEdit: (item: T) => void
  onArchive: (item: T) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex justify-end gap-1'>
      <Button
        size='icon-sm'
        variant='ghost'
        title={t('编辑')}
        onClick={() => onEdit(item)}
      >
        <Edit3 />
      </Button>
      <Button
        size='icon-sm'
        variant='ghost'
        title={t('归档')}
        onClick={() => onArchive(item)}
      >
        <Archive />
      </Button>
    </div>
  )
}

function BaseTable({
  items,
  canEdit,
  onEdit,
  onArchive,
}: {
  items: SeedanceBaseModel[]
  canEdit: boolean
  onEdit: (item: SeedanceBaseModel) => void
  onArchive: (item: SeedanceBaseModel) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('模型')}</TableHead>
          <TableHead>{t('火山模型 ID')}</TableHead>
          <TableHead>{t('预计成本区间')}</TableHead>
          <TableHead>{t('版本')}</TableHead>
          <TableHead>{t('状态')}</TableHead>
          {canEdit && <TableHead className='text-right'>{t('操作')}</TableHead>}
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell>
              <div className='font-medium'>{item.display_name}</div>
              <div className='text-muted-foreground font-mono text-xs'>
                {item.code}
              </div>
            </TableCell>
            <TableCell className='font-mono text-xs'>
              {item.provider_model_id}
            </TableCell>
            <TableCell>
              {matrixRange(item.cost_matrix) || t('未配置')}
            </TableCell>
            <TableCell>r{item.revision}</TableCell>
            <TableCell>
              <Badge variant={item.enabled ? 'default' : 'secondary'}>
                {item.enabled ? t('启用') : t('停用')}
              </Badge>
            </TableCell>
            {canEdit && (
              <TableCell>
                <Actions item={item} onEdit={onEdit} onArchive={onArchive} />
              </TableCell>
            )}
          </TableRow>
        ))}
        {!items.length && <Empty columns={canEdit ? 6 : 5} />}
      </TableBody>
    </Table>
  )
}

function EnhancementTable({
  items,
  providers,
  canEdit,
  onEdit,
  onArchive,
}: {
  items: SeedanceEnhancementModel[]
  providers: SeedanceProvider[]
  canEdit: boolean
  onEdit: (item: SeedanceEnhancementModel) => void
  onArchive: (item: SeedanceEnhancementModel) => void
}) {
  const { t } = useTranslation()
  const providerName = (id: number) =>
    providers.find((item) => item.id === id)?.display_name ?? `#${id}`
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('超分模型')}</TableHead>
          <TableHead>{t('执行节点')}</TableHead>
          <TableHead>{t('档位')}</TableHead>
          <TableHead>{t('预计成本区间')}</TableHead>
          <TableHead>{t('版本')}</TableHead>
          <TableHead>{t('状态')}</TableHead>
          {canEdit && <TableHead className='text-right'>{t('操作')}</TableHead>}
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell>
              <div className='font-medium'>{item.display_name}</div>
              <div className='text-muted-foreground font-mono text-xs'>
                {item.code}
              </div>
            </TableCell>
            <TableCell>{providerName(item.provider_id)}</TableCell>
            <TableCell>
              {item.quality_tier === 'professional' ? t('专业版') : t('标准版')}
            </TableCell>
            <TableCell>
              {matrixRange(item.cost_matrix) || t('未配置')}
            </TableCell>
            <TableCell>r{item.revision}</TableCell>
            <TableCell>
              <Badge variant={item.enabled ? 'default' : 'secondary'}>
                {item.enabled ? t('启用') : t('停用')}
              </Badge>
            </TableCell>
            {canEdit && (
              <TableCell>
                <Actions item={item} onEdit={onEdit} onArchive={onArchive} />
              </TableCell>
            )}
          </TableRow>
        ))}
        {!items.length && <Empty columns={canEdit ? 7 : 6} />}
      </TableBody>
    </Table>
  )
}

function OfferingTable({
  items,
  bases,
  enhancements,
  canEdit,
  onEdit,
  onArchive,
}: {
  items: SeedanceOffering[]
  bases: SeedanceBaseModel[]
  enhancements: SeedanceEnhancementModel[]
  canEdit: boolean
  onEdit: (item: SeedanceOffering) => void
  onArchive: (item: SeedanceOffering) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('售卖模型')}</TableHead>
          <TableHead>{t('模型组合')}</TableHead>
          <TableHead>{t('规格')}</TableHead>
          <TableHead>{t('无参考视频售价')}</TableHead>
          <TableHead>{t('有参考视频售价')}</TableHead>
          <TableHead>{t('状态')}</TableHead>
          {canEdit && <TableHead className='text-right'>{t('操作')}</TableHead>}
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => {
          const base = bases.find((row) => row.id === item.base_model_id)
          const enhancement = enhancements.find(
            (row) => row.id === item.enhancement_model_id
          )
          return (
            <TableRow
              key={item.id}
              className={item.archived_at ? 'opacity-60' : ''}
            >
              <TableCell>
                <div className='font-medium'>{item.display_name}</div>
                <div className='text-muted-foreground text-xs'>
                  {item.pricing_version}
                </div>
              </TableCell>
              <TableCell>
                <div>{base?.display_name ?? `#${item.base_model_id}`}</div>
                <div className='text-muted-foreground text-xs'>
                  {enhancement?.display_name ?? t('不超分')}
                </div>
              </TableCell>
              <TableCell className='whitespace-nowrap'>
                {item.source_resolution} → {item.target_resolution} ·{' '}
                {item.output_fps} FPS
              </TableCell>
              <TableCell>
                {money(item.no_reference_unit_price_micro_rmb)}/s
              </TableCell>
              <TableCell>
                {money(item.reference_unit_price_micro_rmb)}/s
              </TableCell>
              <TableCell>
                <Badge
                  variant={
                    item.enabled && !item.archived_at ? 'default' : 'secondary'
                  }
                >
                  {item.archived_at
                    ? t('已归档')
                    : item.enabled
                      ? t('已发布')
                      : t('草稿')}
                </Badge>
                {item.migration_needs_review && (
                  <Badge variant='destructive' className='ml-1'>
                    {t('待复核')}
                  </Badge>
                )}
              </TableCell>
              {canEdit && (
                <TableCell>
                  <Actions item={item} onEdit={onEdit} onArchive={onArchive} />
                </TableCell>
              )}
            </TableRow>
          )
        })}
        {!items.length && <Empty columns={canEdit ? 7 : 6} />}
      </TableBody>
    </Table>
  )
}

function OrderTable({ items }: { items: SeedanceOrder[] }) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('订单')}</TableHead>
          <TableHead>{t('模型与规格')}</TableHead>
          <TableHead>{t('时长')}</TableHead>
          <TableHead>{t('售价')}</TableHead>
          <TableHead>{t('超分成本')}</TableHead>
          <TableHead>{t('火山云成本')}</TableHead>
          <TableHead>{t('利润')}</TableHead>
          <TableHead>{t('状态')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => {
          const superResolutionCost =
            item.super_resolution_cost_micro_rmb ||
            (item.enhancement_model_id == null
              ? item.service_charge_total_micro_rmb
              : 0)
          const volc =
            item.volcengine_actual_micro_rmb ??
            item.volcengine_estimated_micro_rmb
          const profit =
            item.newapi_actual_profit_micro_rmb ??
            item.newapi_estimated_profit_micro_rmb
          return (
            <TableRow key={item.id}>
              <TableCell>
                <div
                  className='max-w-44 truncate font-mono text-xs'
                  title={item.platform_order_id}
                >
                  {item.platform_order_id}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {item.has_reference_video ? t('有参考视频') : t('无参考视频')}
                </div>
              </TableCell>
              <TableCell>
                <div>{item.model}</div>
                <div className='text-muted-foreground text-xs'>
                  {item.source_resolution} → {item.target_resolution} ·{' '}
                  {item.output_fps} FPS
                </div>
              </TableCell>
              <TableCell>
                <div>{seconds(item.actual_duration_millis)}</div>
                <div className='text-muted-foreground text-xs'>
                  {t('请求')} {seconds(item.requested_duration_millis)}
                </div>
              </TableCell>
              <TableCell>{money(item.model_sale_micro_rmb)}</TableCell>
              <TableCell>{money(superResolutionCost)}</TableCell>
              <TableCell>
                <div>{money(volc)}</div>
                <div className='text-muted-foreground text-xs'>
                  {item.volcengine_actual_micro_rmb == null
                    ? t('预计成本')
                    : t('实际成本')}
                </div>
              </TableCell>
              <TableCell
                className={
                  profit < 0
                    ? 'text-destructive font-semibold'
                    : 'font-semibold text-emerald-600'
                }
              >
                <div>{money(profit)}</div>
                <div className='text-muted-foreground text-xs font-normal'>
                  {item.newapi_actual_profit_micro_rmb == null
                    ? t('预计利润')
                    : t('实际利润')}
                </div>
              </TableCell>
              <TableCell>
                <Badge
                  variant={
                    item.order_status === 'SUCCEEDED'
                      ? 'default'
                      : item.order_status === 'FAILED'
                        ? 'destructive'
                        : 'secondary'
                  }
                >
                  {item.order_status}
                </Badge>
              </TableCell>
            </TableRow>
          )
        })}
        {!items.length && <Empty columns={8} />}
      </TableBody>
    </Table>
  )
}

function EditorShell({
  open,
  title,
  description,
  pending,
  onOpenChange,
  onSubmit,
  children,
}: {
  open: boolean
  title: string
  description: string
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
  children: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full overflow-y-auto sm:max-w-xl'>
        <form className='flex min-h-full flex-col' onSubmit={onSubmit}>
          <SheetHeader>
            <SheetTitle>{title}</SheetTitle>
            <SheetDescription>{description}</SheetDescription>
          </SheetHeader>
          <div className='grid gap-4 px-4 pb-6'>{children}</div>
          <SheetFooter>
            <Button type='submit' disabled={pending}>
              <Save />
              {pending ? t('保存中') : t('保存')}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <div className='grid gap-1.5'>
      <Label>{label}</Label>
      {children}
      {hint && <p className='text-muted-foreground text-xs'>{hint}</p>}
    </div>
  )
}

type BaseCostDraft = Record<
  SeedanceResolution,
  { withoutReference: string; withReference: string }
>
type EnhancementCostDraft = Record<
  SeedanceResolution,
  { upTo30: string; over30: string }
>

const emptyBaseCostDraft = (): BaseCostDraft =>
  Object.fromEntries(
    RESOLUTIONS.map((resolution) => [
      resolution,
      { withoutReference: '', withReference: '' },
    ])
  ) as BaseCostDraft

const emptyEnhancementCostDraft = (): EnhancementCostDraft =>
  Object.fromEntries(
    RESOLUTIONS.map((resolution) => [resolution, { upTo30: '', over30: '' }])
  ) as EnhancementCostDraft

const microRmbToInput = (amount: number) => String(amount / 1_000_000)
const inputToMicroRmb = (amount: string) =>
  Math.round(Number(amount) * 1_000_000)
function parseBaseCostDraft(raw: string): BaseCostDraft {
  const draft = emptyBaseCostDraft()
  try {
    const rows = JSON.parse(raw) as SeedanceBaseCost[]
    rows.forEach((row) => {
      if (!RESOLUTIONS.includes(row.source_resolution)) return
      const key = row.has_reference_video ? 'withReference' : 'withoutReference'
      draft[row.source_resolution][key] = microRmbToInput(
        row.cost_micro_rmb_per_second
      )
    })
  } catch {
    return draft
  }
  return draft
}

function parseEnhancementCostDraft(raw: string): EnhancementCostDraft {
  const draft = emptyEnhancementCostDraft()
  try {
    const rows = JSON.parse(raw) as SeedanceEnhancementCost[]
    rows.forEach((row) => {
      if (!RESOLUTIONS.includes(row.target_resolution)) return
      const key = row.fps_bucket === 'GT_30' ? 'over30' : 'upTo30'
      draft[row.target_resolution][key] = microRmbToInput(
        row.cost_micro_rmb_per_second
      )
    })
  } catch {
    return draft
  }
  return draft
}

function BaseCostMatrixFields({ initialValue }: { initialValue: string }) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(() => parseBaseCostDraft(initialValue))
  const rows = RESOLUTIONS.flatMap<SeedanceBaseCost>((resolution) => {
    const values = draft[resolution]
    const result: SeedanceBaseCost[] = []
    if (values.withoutReference !== '') {
      result.push({
        source_resolution: resolution,
        has_reference_video: false,
        cost_micro_rmb_per_second: inputToMicroRmb(values.withoutReference),
      })
    }
    if (values.withReference !== '') {
      result.push({
        source_resolution: resolution,
        has_reference_video: true,
        cost_micro_rmb_per_second: inputToMicroRmb(values.withReference),
      })
    }
    return result
  })
  const update = (
    resolution: SeedanceResolution,
    key: keyof BaseCostDraft[SeedanceResolution],
    next: string
  ) =>
    setDraft((current) => ({
      ...current,
      [resolution]: { ...current[resolution], [key]: next },
    }))
  return (
    <div className='overflow-hidden rounded-lg border'>
      <input type='hidden' name='cost_matrix' value={JSON.stringify(rows)} />
      <div className='bg-muted/40 grid grid-cols-[5rem_1fr_1fr] gap-2 px-3 py-2 text-xs font-medium'>
        <span>{t('分辨率')}</span>
        <span>{t('无参考视频')}</span>
        <span>{t('有参考视频')}</span>
      </div>
      <div className='divide-y'>
        {RESOLUTIONS.map((resolution) => (
          <div
            key={resolution}
            className='grid grid-cols-[5rem_1fr_1fr] items-center gap-2 px-3 py-2'
          >
            <span className='text-sm font-medium'>
              {resolution.toUpperCase()}
            </span>
            <Input
              type='number'
              min={0}
              step='0.000001'
              inputMode='decimal'
              aria-label={`${resolution} ${t('无参考视频成本')}`}
              placeholder={t('未配置')}
              value={draft[resolution].withoutReference}
              onChange={(event) =>
                update(resolution, 'withoutReference', event.target.value)
              }
            />
            <Input
              type='number'
              min={0}
              step='0.000001'
              inputMode='decimal'
              aria-label={`${resolution} ${t('有参考视频成本')}`}
              placeholder={t('未配置')}
              value={draft[resolution].withReference}
              onChange={(event) =>
                update(resolution, 'withReference', event.target.value)
              }
            />
          </div>
        ))}
      </div>
    </div>
  )
}

function EnhancementCostMatrixFields({
  initialValue,
}: {
  initialValue: string
}) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState(() =>
    parseEnhancementCostDraft(initialValue)
  )
  const rows = RESOLUTIONS.flatMap<SeedanceEnhancementCost>((resolution) => {
    const values = draft[resolution]
    const result: SeedanceEnhancementCost[] = []
    if (values.upTo30 !== '') {
      result.push({
        target_resolution: resolution,
        fps_bucket: 'LE_30',
        cost_micro_rmb_per_second: inputToMicroRmb(values.upTo30),
      })
    }
    if (values.over30 !== '') {
      result.push({
        target_resolution: resolution,
        fps_bucket: 'GT_30',
        cost_micro_rmb_per_second: inputToMicroRmb(values.over30),
      })
    }
    return result
  })
  const update = (
    resolution: SeedanceResolution,
    key: keyof EnhancementCostDraft[SeedanceResolution],
    next: string
  ) =>
    setDraft((current) => ({
      ...current,
      [resolution]: { ...current[resolution], [key]: next },
    }))
  return (
    <div className='overflow-hidden rounded-lg border'>
      <input type='hidden' name='cost_matrix' value={JSON.stringify(rows)} />
      <div className='bg-muted/40 grid grid-cols-[5rem_1fr_1fr] gap-2 px-3 py-2 text-xs font-medium'>
        <span>{t('分辨率')}</span>
        <span>{t('不超过 30 FPS')}</span>
        <span>{t('超过 30 FPS')}</span>
      </div>
      <div className='divide-y'>
        {RESOLUTIONS.map((resolution) => (
          <div
            key={resolution}
            className='grid grid-cols-[5rem_1fr_1fr] items-center gap-2 px-3 py-2'
          >
            <span className='text-sm font-medium'>
              {resolution.toUpperCase()}
            </span>
            <Input
              type='number'
              min={0}
              step='0.000001'
              inputMode='decimal'
              aria-label={`${resolution} ${t('不超过 30 FPS 成本')}`}
              placeholder={t('未配置')}
              value={draft[resolution].upTo30}
              onChange={(event) =>
                update(resolution, 'upTo30', event.target.value)
              }
            />
            <Input
              type='number'
              min={0}
              step='0.000001'
              inputMode='decimal'
              aria-label={`${resolution} ${t('超过 30 FPS 成本')}`}
              placeholder={t('未配置')}
              value={draft[resolution].over30}
              onChange={(event) =>
                update(resolution, 'over30', event.target.value)
              }
            />
          </div>
        ))}
      </div>
    </div>
  )
}

function parseMediaKitSpecification(raw?: string): {
  resolution: SeedanceResolution
  toolVersion: MediaKitToolVersion
} {
  try {
    const parsed = JSON.parse(raw || '{}') as {
      resolution?: SeedanceResolution
      target_resolution?: SeedanceResolution
      tool_version?: MediaKitToolVersion
    }
    const resolution = parsed.resolution ?? parsed.target_resolution
    return {
      resolution:
        resolution && RESOLUTIONS.includes(resolution) ? resolution : '1080p',
      toolVersion:
        parsed.tool_version === 'professional' ? 'professional' : 'standard',
    }
  } catch {
    return { resolution: '1080p', toolVersion: 'standard' }
  }
}

function BaseEditor({
  open,
  item,
  pending,
  onOpenChange,
  onSave,
}: {
  open: boolean
  item: SeedanceBaseModel | null
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: Partial<SeedanceBaseModel>) => void
}) {
  const { t } = useTranslation()
  const [providerModelId, setProviderModelId] = useState(
    item?.provider_model_id ?? ''
  )
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const costMatrix = value(data, 'cost_matrix')
    if (!providerModelId.trim()) {
      toast.error(t('请选择或输入火山 Seedance 模型 ID。'))
      return
    }
    onSave({
      id: item?.id,
      code: value(data, 'code'),
      display_name: value(data, 'display_name'),
      provider_model_id: value(data, 'provider_model_id'),
      cost_matrix: costMatrix,
      enabled: data.get('enabled') === 'on',
    })
  }
  return (
    <EditorShell
      open={open}
      pending={pending}
      onOpenChange={onOpenChange}
      onSubmit={submit}
      title={item ? t('编辑本体模型') : t('新增本体模型')}
      description={t(
        '预计成本仅用于账单同步前的利润估算；火山账单同步后以实际成本为准。'
      )}
    >
      <Field label={t('内部代码')}>
        <Input
          name='code'
          defaultValue={item?.code}
          required
          readOnly={Boolean(item)}
        />
      </Field>
      <Field label={t('显示名称')}>
        <Input name='display_name' defaultValue={item?.display_name} required />
      </Field>
      <Field label={t('火山 Seedance 模型 ID')}>
        <Combobox
          options={SEEDANCE_MODEL_OPTIONS}
          value={providerModelId}
          onValueChange={(next) => setProviderModelId(next ?? '')}
          placeholder={t('选择或输入火山模型 ID')}
          searchPlaceholder={t('搜索 Seedance 模型...')}
          emptyText={t('未找到模型，可直接输入完整模型 ID。')}
          allowCustomValue
        />
        <input type='hidden' name='provider_model_id' value={providerModelId} />
      </Field>
      <Field
        label={t('火山预计成本（元/秒，可选）')}
        hint={t('可以全部留空；未配置时按 0 估算，不影响模型发布和运行。')}
      >
        <BaseCostMatrixFields
          key={`${item?.id ?? 'new'}:${item?.cost_matrix ?? ''}`}
          initialValue={item?.cost_matrix ?? BASE_COST_TEMPLATE}
        />
      </Field>
      <label className='flex items-center gap-2 text-sm'>
        <Switch name='enabled' defaultChecked={item?.enabled ?? true} />
        {t('启用当前版本')}
      </label>
    </EditorShell>
  )
}

function EnhancementEditor({
  open,
  item,
  providers,
  pending,
  onOpenChange,
  onManageProviders,
  onSave,
}: {
  open: boolean
  item: SeedanceEnhancementModel | null
  providers: SeedanceProvider[]
  pending: boolean
  onOpenChange: (open: boolean) => void
  onManageProviders: () => void
  onSave: (data: Partial<SeedanceEnhancementModel>) => void
}) {
  const { t } = useTranslation()
  const availableProviders = providers.filter(
    (provider) =>
      provider.status === 'ACTIVE' || provider.id === item?.provider_id
  )
  const [providerId, setProviderId] = useState(
    item?.provider_id ??
      availableProviders.find((row) => row.status === 'ACTIVE')?.id ??
      0
  )
  const selectedProvider = providers.find((row) => row.id === providerId)
  const isMediaKit = selectedProvider?.adapter_type === 'VOLCENGINE_MEDIAKIT'
  const currentMediaKitSpecification = parseMediaKitSpecification(
    item?.specification
  )
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const qualityTier = value(data, 'quality_tier') as MediaKitToolVersion
    const costMatrix = value(data, 'cost_matrix')
    if (!providerId) {
      toast.error(t('请选择执行节点。'))
      return
    }
    onSave({
      id: item?.id,
      code: value(data, 'code'),
      display_name: value(data, 'display_name'),
      provider_id: providerId,
      service_code: isMediaKit
        ? MEDIAKIT_SERVICE_CODE
        : value(data, 'service_code'),
      quality_tier: qualityTier,
      specification: isMediaKit
        ? buildMediaKitSpecification(
            value(data, 'mediakit_resolution') as MediaKitResolution,
            qualityTier
          )
        : value(data, 'specification'),
      specification_version: value(data, 'specification_version'),
      cost_matrix: costMatrix,
      enabled: data.get('enabled') === 'on',
    })
  }
  return (
    <EditorShell
      open={open}
      pending={pending}
      onOpenChange={onOpenChange}
      onSubmit={submit}
      title={item ? t('编辑超分模型') : t('新增超分模型')}
      description={t(
        '输出 FPS 使用售卖模型的固定值；预计成本仅用于账单同步前的利润估算。'
      )}
    >
      <Field label={t('内部代码')}>
        <Input
          name='code'
          defaultValue={item?.code}
          required
          readOnly={Boolean(item)}
        />
      </Field>
      <Field label={t('显示名称')}>
        <Input name='display_name' defaultValue={item?.display_name} required />
      </Field>
      <Field label={t('执行节点')}>
        <div className='flex gap-2'>
          <Select
            value={providerId ? String(providerId) : ''}
            onValueChange={(next) => setProviderId(Number(next))}
            disabled={!availableProviders.length}
          >
            <SelectTrigger className='min-w-0 flex-1'>
              <SelectValue
                placeholder={
                  availableProviders.length
                    ? t('选择节点')
                    : t('暂无可用执行节点')
                }
              >
                {(current) => {
                  const provider = availableProviders.find(
                    (row) => String(row.id) === String(current ?? '')
                  )
                  return provider
                    ? seedanceProviderOptionLabel(provider)
                    : availableProviders.length
                      ? t('选择节点')
                      : t('暂无可用执行节点')
                }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {availableProviders.map((provider) => (
                <SelectItem key={provider.id} value={String(provider.id)}>
                  {seedanceProviderOptionLabel(provider)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type='button' variant='outline' onClick={onManageProviders}>
            <Plus />
            {t('添加节点')}
          </Button>
        </div>
        {!availableProviders.length && (
          <p className='text-muted-foreground text-xs'>
            {t('请先添加并启用火山 AI MediaKit 或自定义远端服务节点。')}
          </p>
        )}
      </Field>
      {isMediaKit ? (
        <div className='grid gap-3 rounded-lg border p-3'>
          <div className='text-sm font-medium'>
            {t('火山 AI MediaKit 参数')}
          </div>
          <Field label={t('服务代码')} hint={t('由系统固定，无需填写。')}>
            <Input value={MEDIAKIT_SERVICE_CODE} readOnly />
          </Field>
          <div className='grid grid-cols-2 gap-3'>
            <Field label={t('目标分辨率')}>
              <ResolutionSelect
                name='mediakit_resolution'
                selected={currentMediaKitSpecification.resolution}
              />
            </Field>
            <Field label={t('处理版本（超分档位）')}>
              <Select
                name='quality_tier'
                defaultValue={
                  item?.quality_tier ?? currentMediaKitSpecification.toolVersion
                }
              >
                <SelectTrigger>
                  <SelectValue>
                    {(current) =>
                      current === 'professional' ? t('专业版') : t('标准版')
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='standard'>{t('标准版')}</SelectItem>
                  <SelectItem value='professional'>{t('专业版')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          <p className='text-muted-foreground text-xs'>
            {t('场景固定为 AIGC，系统会自动生成执行规格，无需编辑 JSON。')}
          </p>
        </div>
      ) : (
        <div className='grid gap-3 rounded-lg border p-3'>
          <div className='text-sm font-medium'>{t('自定义远端服务参数')}</div>
          <div className='grid grid-cols-2 gap-3'>
            <Field label={t('服务代码')}>
              <Input
                name='service_code'
                defaultValue={item?.service_code}
                required
              />
            </Field>
            <Field label={t('超分档位')}>
              <Select
                name='quality_tier'
                defaultValue={item?.quality_tier ?? 'standard'}
              >
                <SelectTrigger>
                  <SelectValue>
                    {(current) =>
                      current === 'professional' ? t('专业版') : t('标准版')
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='standard'>{t('标准版')}</SelectItem>
                  <SelectItem value='professional'>{t('专业版')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field
            label={t('执行规格 JSON')}
            hint={t('仅自定义远端服务需要按供应商协议填写。')}
          >
            <Textarea
              name='specification'
              rows={5}
              className='font-mono text-xs'
              defaultValue={item?.specification ?? '{}'}
              required
            />
          </Field>
        </div>
      )}
      <div className='grid grid-cols-1 gap-3'>
        <Field
          label={t('规格版本')}
          hint={t('配置发生变化时填写新版本，例如 v2。')}
        >
          <Input
            name='specification_version'
            defaultValue={item?.specification_version ?? 'v1'}
            required
          />
        </Field>
      </div>
      <Field
        label={t('超分预计成本（元/秒，可选）')}
        hint={t('可以全部留空；未配置时按 0 估算，不影响模型发布和运行。')}
      >
        <EnhancementCostMatrixFields
          key={`${item?.id ?? 'new'}:${item?.cost_matrix ?? ''}`}
          initialValue={item?.cost_matrix ?? ENHANCEMENT_COST_TEMPLATE}
        />
      </Field>
      <label className='flex items-center gap-2 text-sm'>
        <Switch name='enabled' defaultChecked={item?.enabled ?? true} />
        {t('启用当前版本')}
      </label>
    </EditorShell>
  )
}

function OfferingEditor({
  open,
  item,
  channelId,
  bases,
  enhancements,
  pending,
  onOpenChange,
  onSave,
}: {
  open: boolean
  item: SeedanceOffering | null
  channelId: number
  bases: SeedanceBaseModel[]
  enhancements: SeedanceEnhancementModel[]
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: Partial<SeedanceOffering>) => void
}) {
  const { t } = useTranslation()
  const [outputFps, setOutputFps] = useState(String(item?.output_fps ?? 24))
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    const parsedOutputFps = intValue(data, 'output_fps')
    if (
      !Number.isInteger(parsedOutputFps) ||
      parsedOutputFps < 1 ||
      parsedOutputFps > 240
    ) {
      toast.error(t('输出帧率必须是 1–240 的整数。'))
      return
    }
    onSave({
      id: item?.id,
      channel_id: channelId,
      display_name: value(data, 'display_name'),
      base_model_id: intValue(data, 'base_model_id'),
      enhancement_model_id: optionalInt(data, 'enhancement_model_id'),
      source_resolution: value(data, 'source_resolution') as SeedanceResolution,
      target_resolution: value(data, 'target_resolution') as SeedanceResolution,
      output_fps: parsedOutputFps,
      no_reference_unit_price_micro_rmb: inputToMicroRmb(
        value(data, 'no_reference_unit_price_rmb')
      ),
      reference_unit_price_micro_rmb: inputToMicroRmb(
        value(data, 'reference_unit_price_rmb')
      ),
      pricing_version: value(data, 'pricing_version'),
      enabled: data.get('enabled') === 'on',
    })
  }
  return (
    <EditorShell
      open={open}
      pending={pending}
      onOpenChange={onOpenChange}
      onSubmit={submit}
      title={item ? t('编辑售卖模型') : t('新增售卖模型')}
      description={t('售卖模型冻结模型组合、分辨率、输出 FPS 与两种每秒售价。')}
    >
      <Alert>
        <CircleAlert />
        <AlertTitle>{t('允许亏损发布')}</AlertTitle>
        <AlertDescription>
          {t('售价低于超分成本与火山成本之和时仍可保存，请在发布前核对利润。')}
        </AlertDescription>
      </Alert>
      <Field label={t('公开模型名称')}>
        <Input name='display_name' defaultValue={item?.display_name} required />
      </Field>
      <Field label={t('本体模型')}>
        <Select
          name='base_model_id'
          defaultValue={String(item?.base_model_id ?? bases[0]?.id ?? '')}
        >
          <SelectTrigger>
            <SelectValue placeholder={t('选择本体模型')}>
              {(current) => {
                const selected = bases.find(
                  (row) => String(row.id) === String(current ?? '')
                )
                return selected
                  ? `${selected.display_name} · r${selected.revision}`
                  : t('选择本体模型')
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {bases
              .filter((row) => row.enabled)
              .map((row) => (
                <SelectItem key={row.id} value={String(row.id)}>
                  {row.display_name} · r{row.revision}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
      </Field>
      <Field label={t('超分模型')}>
        <Select
          name='enhancement_model_id'
          defaultValue={
            item?.enhancement_model_id
              ? String(item.enhancement_model_id)
              : 'none'
          }
        >
          <SelectTrigger>
            <SelectValue>
              {(current) => {
                if (current === 'none' || current == null) return t('不超分')
                const selected = enhancements.find(
                  (row) => String(row.id) === String(current)
                )
                return selected
                  ? `${selected.display_name} · ${
                      selected.quality_tier === 'professional'
                        ? t('专业版')
                        : t('标准版')
                    }`
                  : t('不超分')
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='none'>{t('不超分')}</SelectItem>
            {enhancements
              .filter((row) => row.enabled)
              .map((row) => (
                <SelectItem key={row.id} value={String(row.id)}>
                  {row.display_name} ·{' '}
                  {row.quality_tier === 'professional'
                    ? t('专业版')
                    : t('标准版')}
                </SelectItem>
              ))}
          </SelectContent>
        </Select>
      </Field>
      <div className='grid grid-cols-2 gap-3'>
        <Field label={t('原始分辨率')}>
          <ResolutionSelect
            name='source_resolution'
            selected={item?.source_resolution ?? '720p'}
          />
        </Field>
        <Field label={t('目标分辨率')}>
          <ResolutionSelect
            name='target_resolution'
            selected={item?.target_resolution ?? '1080p'}
          />
        </Field>
      </div>
      <Field
        label={t('固定输出 FPS')}
        hint={t('可选择常用帧率，也可直接输入 1–240 的整数。')}
      >
        <Combobox
          options={OUTPUT_FPS_OPTIONS}
          value={outputFps}
          onValueChange={(next) => setOutputFps(next ?? '')}
          placeholder={t('选择或输入输出帧率')}
          searchPlaceholder={t('选择常用帧率或输入整数...')}
          emptyText={t('输入 1–240 的整数。')}
          allowCustomValue
        />
        <input type='hidden' name='output_fps' value={outputFps} />
      </Field>
      <div className='grid grid-cols-2 gap-3'>
        <Field label={t('无参考视频售价（元/秒）')}>
          <Input
            name='no_reference_unit_price_rmb'
            type='number'
            min={0}
            step='0.000001'
            inputMode='decimal'
            defaultValue={microRmbToInput(
              item?.no_reference_unit_price_micro_rmb ?? 0
            )}
            required
          />
        </Field>
        <Field label={t('有参考视频售价（元/秒）')}>
          <Input
            name='reference_unit_price_rmb'
            type='number'
            min={0}
            step='0.000001'
            inputMode='decimal'
            defaultValue={microRmbToInput(
              item?.reference_unit_price_micro_rmb ?? 0
            )}
            required
          />
        </Field>
      </div>
      <Field
        label={t('价格版本')}
        hint={t('模型组合或价格变化时必须填写新的版本。')}
      >
        <Input
          name='pricing_version'
          defaultValue={item?.pricing_version ?? 'v1'}
          required
        />
      </Field>
      <label className='flex items-center gap-2 text-sm'>
        <Switch name='enabled' defaultChecked={item?.enabled ?? false} />
        {t('立即发布')}
      </label>
    </EditorShell>
  )
}
function ResolutionSelect({
  name,
  selected,
}: {
  name: string
  selected: SeedanceResolution
}) {
  return (
    <Select name={name} defaultValue={selected}>
      <SelectTrigger>
        <SelectValue>
          {(current) => String(current ?? selected).toUpperCase()}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {RESOLUTIONS.map((resolution) => (
          <SelectItem key={resolution} value={resolution}>
            {resolution.toUpperCase()}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function RuntimeEditor({
  open,
  channelId,
  overview,
  pending,
  onOpenChange,
  onSave,
}: {
  open: boolean
  channelId: number
  overview: SeedanceOverview
  pending: boolean
  onOpenChange: (open: boolean) => void
  onSave: (work: () => Promise<unknown>) => void
}) {
  const { t } = useTranslation()
  const config = overview.config
  const [billingBaseUrl, setBillingBaseUrl] = useState(
    config?.aipdd_billing_base_url ?? 'https://api.aipdd.work'
  )
  const [providerAdapterType, setProviderAdapterType] = useState<
    SeedanceProvider['adapter_type']
  >('VOLCENGINE_MEDIAKIT')
  const configSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    onSave(() =>
      saveSeedanceConfig(channelId, {
        instance_id: value(data, 'instance_id'),
        aipdd_billing_base_url: billingBaseUrl,
        aipdd_billing_api_key:
          value(data, 'aipdd_billing_api_key') || undefined,
        volcengine_bill_sync_enabled:
          config?.volcengine_bill_sync_enabled ?? false,
        volcengine_bill_product_codes: parseList(
          config?.volcengine_bill_product_codes
        ),
        volcengine_bill_configuration_codes: parseList(
          config?.volcengine_bill_configuration_codes
        ),
        status: value(data, 'status') as 'ACTIVE' | 'DISABLED',
      })
    )
  }
  const credentialSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    onSave(() =>
      createSeedanceCredential(channelId, {
        ark_api_key: value(data, 'ark_api_key'),
        access_key_id: value(data, 'access_key_id') || undefined,
        secret_access_key: value(data, 'secret_access_key') || undefined,
      })
    )
  }
  const providerSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const data = new FormData(event.currentTarget)
    onSave(() =>
      saveSeedanceProvider({
        provider_type: 'DIRECT_EXTERNAL',
        adapter_type: providerAdapterType,
        display_name: value(data, 'display_name'),
        service_endpoint:
          providerAdapterType === 'VOLCENGINE_MEDIAKIT'
            ? MEDIAKIT_BASE_URL
            : value(data, 'service_endpoint'),
        service_code:
          providerAdapterType === 'VOLCENGINE_MEDIAKIT'
            ? MEDIAKIT_SERVICE_CODE
            : value(data, 'service_code'),
        credential:
          providerAdapterType === 'GENERIC_HTTP'
            ? value(data, 'credential')
            : undefined,
        mediakit_api_key:
          providerAdapterType === 'VOLCENGINE_MEDIAKIT'
            ? value(data, 'mediakit_api_key')
            : undefined,
        status: 'ACTIVE',
        capabilities: '{}',
        timeout_policy: '{"timeout_seconds":120}',
        retry_policy: '{}',
        fallback_policy: '{}',
      })
    )
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full overflow-y-auto sm:max-w-2xl'>
        <SheetHeader>
          <SheetTitle>{t('运行设置')}</SheetTitle>
          <SheetDescription>
            {t('维护 Seedance 服务连接、Ark 凭证和超分执行节点。')}
          </SheetDescription>
        </SheetHeader>
        <div className='space-y-5 px-4 pb-6'>
          <form
            className='grid gap-3 rounded-lg border p-3'
            onSubmit={configSubmit}
          >
            <h4 className='text-sm font-semibold'>{t('Seedance 服务连接')}</h4>
            <Field label={t('实例 ID')}>
              <Input
                name='instance_id'
                defaultValue={config?.instance_id || overview.site_instance_id}
                required
              />
            </Field>
            <Field label={t('AIPDD 财务地址')}>
              <Combobox
                options={AIPDD_BILLING_URL_OPTIONS}
                value={billingBaseUrl}
                onValueChange={(next) => setBillingBaseUrl(next ?? '')}
                placeholder={t('选择或输入 AIPDD 财务地址')}
                searchPlaceholder={t('选择正式服务或输入自定义地址...')}
                emptyText={t('可直接输入 HTTPS 地址。')}
                allowCustomValue
              />
            </Field>
            <Field label={t('AIPDD 财务密钥')}>
              <Input
                name='aipdd_billing_api_key'
                type='password'
                placeholder={
                  overview.billing_credential_configured
                    ? t('留空保留现有密钥')
                    : ''
                }
              />
            </Field>
            <Field label={t('状态')}>
              <Select name='status' defaultValue={config?.status ?? 'DISABLED'}>
                <SelectTrigger>
                  <SelectValue>
                    {(current) =>
                      current === 'ACTIVE' ? t('启用') : t('停用')
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='ACTIVE'>{t('启用')}</SelectItem>
                  <SelectItem value='DISABLED'>{t('停用')}</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <Button type='submit' size='sm' disabled={pending}>
              <Save />
              {t('保存连接')}
            </Button>
          </form>
          <form
            className='grid gap-3 rounded-lg border p-3'
            onSubmit={credentialSubmit}
          >
            <h4 className='text-sm font-semibold'>{t('新增 Ark 凭证')}</h4>
            <Field label={t('Ark API Key')}>
              <Input name='ark_api_key' type='password' required />
            </Field>
            <div className='grid grid-cols-2 gap-3'>
              <Field label={t('账单只读 Access Key')}>
                <Input name='access_key_id' />
              </Field>
              <Field label={t('账单只读 Secret Key')}>
                <Input name='secret_access_key' type='password' />
              </Field>
            </div>
            <Button type='submit' size='sm' disabled={pending}>
              <Save />
              {t('保存凭证')}
            </Button>
            <div className='flex flex-wrap gap-2'>
              {overview.credentials.map((credential) => (
                <Button
                  type='button'
                  key={credential.id}
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    onSave(() =>
                      validateSeedanceCredential(channelId, credential.id)
                    )
                  }
                >
                  #{credential.id} · v{credential.version} · {credential.status}
                </Button>
              ))}
            </div>
          </form>
          <form
            className='grid gap-3 rounded-lg border p-3'
            onSubmit={providerSubmit}
          >
            <h4 className='text-sm font-semibold'>{t('新增超分执行节点')}</h4>
            <Field label={t('节点名称')}>
              <Input name='display_name' required />
            </Field>
            <Field label={t('接入方式')}>
              <Select
                value={providerAdapterType}
                onValueChange={(next) =>
                  setProviderAdapterType(
                    next as SeedanceProvider['adapter_type']
                  )
                }
              >
                <SelectTrigger>
                  <SelectValue>
                    {(current) =>
                      current === 'GENERIC_HTTP'
                        ? t('自定义远端服务')
                        : t('火山 AI MediaKit')
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='VOLCENGINE_MEDIAKIT'>
                    {t('火山 AI MediaKit')}
                  </SelectItem>
                  <SelectItem value='GENERIC_HTTP'>
                    {t('自定义远端服务')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </Field>
            {providerAdapterType === 'VOLCENGINE_MEDIAKIT' ? (
              <>
                <Field
                  label={t('服务地址')}
                  hint={t('火山直连地址由系统固定。')}
                >
                  <Input value={MEDIAKIT_BASE_URL} readOnly />
                </Field>
                <Field label={t('服务代码')} hint={t('服务代码由系统固定。')}>
                  <Input value={MEDIAKIT_SERVICE_CODE} readOnly />
                </Field>
                <Field label={t('MediaKit API Key')}>
                  <Input name='mediakit_api_key' type='password' required />
                </Field>
              </>
            ) : (
              <>
                <Field label={t('任务集合地址')}>
                  <Input
                    name='service_endpoint'
                    type='url'
                    placeholder='https://supplier.example/tasks'
                    required
                  />
                </Field>
                <Field label={t('服务代码')}>
                  <Input name='service_code' required />
                </Field>
                <Field
                  label={t('Bearer Token')}
                  hint={t('供应商不需要鉴权时可留空。')}
                >
                  <Input name='credential' type='password' />
                </Field>
              </>
            )}
            <Button type='submit' size='sm' disabled={pending}>
              <Save />
              {t('保存节点')}
            </Button>
          </form>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function parseList(raw?: string) {
  try {
    const parsed = JSON.parse(raw || '[]')
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return []
  }
}
