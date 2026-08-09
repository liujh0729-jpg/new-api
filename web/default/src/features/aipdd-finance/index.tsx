import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Alert02Icon,
  Loading03Icon,
  SearchIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
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
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import {
  createFinanceExport,
  downloadFinanceExport,
  getFinanceExport,
  getFinanceOrderDetail,
  getFinanceOrders,
  getFinanceSummary,
  getFinanceSyncStatus,
  retryFinanceSync,
} from './api'
import type { FinanceExportJob, FinanceFilter, FinanceOrder } from './types'

const PAGE_SIZE = 50

type DraftFilter = {
  start: string
  end: string
  user: string
  token: string
  channel: string
  instance: string
  model: string
  order: string
  costStatus: string
  issueView: string
}

function initialFilter(): DraftFilter {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), 1)
  const query = new URLSearchParams(window.location.search)
  return {
    start: toDateTimeInput(start),
    end: toDateTimeInput(new Date(now.getTime() + 60_000)),
    user: '',
    token: '',
    channel: query.get('channel_id') ?? '',
    instance: '',
    model: '',
    order: query.get('order') ?? '',
    costStatus: '',
    issueView: '',
  }
}

export function AIPDDFinanceReport() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<DraftFilter>(initialFilter)
  const [filter, setFilter] = useState<FinanceFilter>(() => toApiFilter(draft))
  const [page, setPage] = useState(1)
  const [selectedOrderId, setSelectedOrderId] = useState<string | null>(null)
  const [exportJobId, setExportJobId] = useState<string | null>(null)

  const summaryQuery = useQuery({
    queryKey: ['aipdd-finance-summary', filter],
    queryFn: () => getFinanceSummary(filter),
  })
  const ordersQuery = useQuery({
    queryKey: ['aipdd-finance-orders', filter, page],
    queryFn: () => getFinanceOrders(filter, page, PAGE_SIZE),
    placeholderData: (previousData) => previousData,
  })
  const syncQuery = useQuery({
    queryKey: ['aipdd-finance-sync', filter.instance_id],
    queryFn: () => getFinanceSyncStatus(filter.instance_id),
  })
  const detailQuery = useQuery({
    queryKey: ['aipdd-finance-order-detail', selectedOrderId],
    queryFn: () => getFinanceOrderDetail(selectedOrderId!),
    enabled: Boolean(selectedOrderId),
  })
  const exportQuery = useQuery({
    queryKey: ['aipdd-finance-export', exportJobId],
    queryFn: () => getFinanceExport(exportJobId!),
    enabled: Boolean(exportJobId),
    refetchInterval: (query) => {
      const status = (
        query.state.data as { data?: FinanceExportJob } | undefined
      )?.data?.status
      return status === 'PENDING' || status === 'RUNNING' ? 1500 : false
    },
  })
  const exportMutation = useMutation({
    mutationFn: () => createFinanceExport(filter),
    onSuccess: (response) => {
      setExportJobId(response.data.id)
      toast.success(t('The full XLSX export has been queued'))
    },
  })
  const retryMutation = useMutation({
    mutationFn: retryFinanceSync,
    onSuccess: async (response) => {
      toast.success(
        t('{{count}} orders have been queued for safe refresh', {
          count: response.data.queued,
        })
      )
      await queryClient.invalidateQueries({ queryKey: ['aipdd-finance-sync'] })
    },
  })

  const summary = summaryQuery.data?.data
  const orders = ordersQuery.data?.data ?? []
  const total = ordersQuery.data?.total ?? 0
  const pageCount = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const currentExport = exportQuery.data?.data

  function applyFilters(nextDraft = draft) {
    setFilter(toApiFilter(nextDraft))
    setPage(1)
  }

  function setIssueView(issueView: string) {
    const next = { ...draft, issueView }
    setDraft(next)
    applyFilters(next)
  }

  async function handleExportAction() {
    if (currentExport?.status === 'READY') {
      await downloadFinanceExport(currentExport)
      return
    }
    if (
      currentExport?.status === 'PENDING' ||
      currentExport?.status === 'RUNNING'
    ) {
      return
    }
    exportMutation.mutate()
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('AIPDD Profit Report')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          onClick={() => void handleExportAction()}
          disabled={
            exportMutation.isPending ||
            currentExport?.status === 'PENDING' ||
            currentExport?.status === 'RUNNING'
          }
        >
          {(exportMutation.isPending ||
            currentExport?.status === 'PENDING' ||
            currentExport?.status === 'RUNNING') && (
            <HugeiconsIcon
              icon={Loading03Icon}
              className='animate-spin'
              strokeWidth={2}
            />
          )}
          {currentExport?.status === 'READY'
            ? t('Download full XLSX')
            : t('Export full XLSX')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4'>
          <FinanceFilters
            draft={draft}
            onChange={setDraft}
            onApply={() => applyFilters()}
            onIssueView={setIssueView}
            t={t}
          />
          <SummaryCards
            summary={summary}
            loading={summaryQuery.isLoading}
            t={t}
          />
          <Tabs defaultValue='orders'>
            <TabsList>
              <TabsTrigger value='orders'>{t('Order details')}</TabsTrigger>
              <TabsTrigger value='sync'>{t('Exceptions and sync')}</TabsTrigger>
            </TabsList>
            <TabsContent value='orders'>
              <OrdersTable
                orders={orders}
                loading={ordersQuery.isLoading}
                fetching={ordersQuery.isFetching}
                page={page}
                pageCount={pageCount}
                total={total}
                onPageChange={setPage}
                onSelect={setSelectedOrderId}
                t={t}
              />
            </TabsContent>
            <TabsContent value='sync'>
              <SyncTable
                statuses={syncQuery.data?.data ?? []}
                loading={syncQuery.isLoading}
                onRetry={(channelId, instanceId) =>
                  retryMutation.mutate({
                    channel_id: channelId,
                    instance_id: instanceId,
                  })
                }
                retrying={retryMutation.isPending}
                t={t}
              />
            </TabsContent>
          </Tabs>
          {currentExport?.status === 'FAILED' && (
            <Alert variant='destructive'>
              <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
              <AlertTitle>{t('Export failed')}</AlertTitle>
              <AlertDescription>{currentExport.failure_cause}</AlertDescription>
            </Alert>
          )}
        </div>
        <OrderDetailSheet
          open={Boolean(selectedOrderId)}
          onOpenChange={(open) => !open && setSelectedOrderId(null)}
          detail={detailQuery.data?.data}
          loading={detailQuery.isLoading}
          t={t}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function FinanceFilters({
  draft,
  onChange,
  onApply,
  onIssueView,
  t,
}: {
  draft: DraftFilter
  onChange: (value: DraftFilter) => void
  onApply: () => void
  onIssueView: (value: string) => void
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  const costOptions = useMemo(
    () => [
      { value: 'all', label: t('All cost statuses') },
      { value: 'CONFIRMED', label: t('Confirmed') },
      { value: 'PARTIAL', label: t('Partially calculated') },
      { value: 'PENDING', label: t('Pending confirmation') },
      { value: 'UNVERIFIABLE', label: t('Unverifiable') },
    ],
    [t]
  )
  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('Financial filters')}</CardTitle>
        <CardDescription>
          {t('API keys are shown only by ID, name, and masked prefix')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <FieldGroup className='gap-3'>
          <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
            <FilterInput
              label={t('Start time')}
              type='datetime-local'
              value={draft.start}
              onChange={(start) => onChange({ ...draft, start })}
            />
            <FilterInput
              label={t('End time')}
              type='datetime-local'
              value={draft.end}
              onChange={(end) => onChange({ ...draft, end })}
            />
            <FilterInput
              label={t('NewAPI user ID or username')}
              value={draft.user}
              onChange={(user) => onChange({ ...draft, user })}
            />
            <FilterInput
              label={t('Token ID or name')}
              value={draft.token}
              onChange={(token) => onChange({ ...draft, token })}
            />
            <FilterInput
              label={t('AIPDD channel ID')}
              value={draft.channel}
              onChange={(channel) => onChange({ ...draft, channel })}
            />
            <FilterInput
              label={t('AIPDD instance ID')}
              value={draft.instance}
              onChange={(instance) => onChange({ ...draft, instance })}
            />
            <FilterInput
              label={t('Model')}
              value={draft.model}
              onChange={(model) => onChange({ ...draft, model })}
            />
            <FilterInput
              label={t('Unified financial order ID')}
              value={draft.order}
              onChange={(order) => onChange({ ...draft, order })}
            />
          </div>
          <div className='flex flex-wrap items-end gap-2'>
            <Field className='w-48'>
              <FieldLabel>{t('Cost status')}</FieldLabel>
              <Select
                items={costOptions}
                value={draft.costStatus || 'all'}
                onValueChange={(value) =>
                  onChange({
                    ...draft,
                    costStatus: value === 'all' || value == null ? '' : value,
                  })
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {costOptions.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Button onClick={onApply}>
              <HugeiconsIcon icon={SearchIcon} strokeWidth={2} />
              {t('Search')}
            </Button>
            <div className='flex flex-wrap gap-1'>
              {[
                ['', t('All')],
                ['LOSS', t('Loss orders')],
                ['PENDING', t('Pending invoices')],
                ['REVIEW', t('Manual review')],
                ['UNVERIFIABLE', t('Unverifiable')],
              ].map(([value, label]) => (
                <Button
                  key={value || 'all'}
                  size='sm'
                  variant={draft.issueView === value ? 'secondary' : 'ghost'}
                  onClick={() => onIssueView(value)}
                >
                  {label}
                </Button>
              ))}
            </div>
          </div>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

function FilterInput({
  label,
  value,
  onChange,
  type = 'text',
}: {
  label: string
  value: string
  onChange: (value: string) => void
  type?: string
}) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      <Input
        type={type}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </Field>
  )
}

function SummaryCards({
  summary,
  loading,
  t,
}: {
  summary?: ReturnType<typeof emptySummary>
  loading: boolean
  t: (key: string) => string
}) {
  const values = [
    [
      t('Customer net consumption'),
      formatMoney(
        summary?.customer_net_consumption_rmb_mic,
        t('Pending confirmation')
      ),
    ],
    [
      t('Confirmed source cost'),
      formatMoney(
        summary?.confirmed_source_cost_rmb_mic,
        t('Pending confirmation')
      ),
    ],
    [
      t('Confirmed profit'),
      formatMoney(summary?.confirmed_profit_rmb_mic, t('Pending confirmation')),
    ],
    [
      t('Estimated profit'),
      formatMoney(summary?.estimated_profit_rmb_mic, t('Pending confirmation')),
    ],
    [t('Loss orders'), String(summary?.loss_order_count ?? 0)],
    [
      t('Pending confirmation'),
      String(summary?.pending_confirmation_count ?? 0),
    ],
  ]
  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-6'>
      {values.map(([label, value]) => (
        <Card key={label} size='sm'>
          <CardHeader>
            <CardDescription>{label}</CardDescription>
            <CardTitle className='font-mono text-lg'>
              {loading ? <Skeleton className='h-6 w-20' /> : value}
            </CardTitle>
          </CardHeader>
        </Card>
      ))}
    </div>
  )
}

function OrdersTable({
  orders,
  loading,
  fetching,
  page,
  pageCount,
  total,
  onPageChange,
  onSelect,
  t,
}: {
  orders: FinanceOrder[]
  loading: boolean
  fetching: boolean
  page: number
  pageCount: number
  total: number
  onPageChange: (page: number) => void
  onSelect: (id: string) => void
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('AIPDD orders')}</CardTitle>
        <CardDescription>
          {t('{{count}} matching orders', { count: total })}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-0'>
        <div className='overflow-x-auto border-y'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Time')}</TableHead>
                <TableHead>{t('Unified order ID')}</TableHead>
                <TableHead>{t('User / Token')}</TableHead>
                <TableHead>{t('Model')}</TableHead>
                <TableHead className='text-right'>
                  {t('Customer charge')}
                </TableHead>
                <TableHead className='text-right'>
                  {t('AIPDD source charge')}
                </TableHead>
                <TableHead className='text-right'>{t('Profit')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 6 }).map((_, index) => (
                  <TableRow key={index}>
                    <TableCell colSpan={9}>
                      <Skeleton className='h-8 w-full' />
                    </TableCell>
                  </TableRow>
                ))
              ) : orders.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={9}
                    className='text-muted-foreground h-28 text-center'
                  >
                    {t('No matching financial orders')}
                  </TableCell>
                </TableRow>
              ) : (
                orders.map((order) => (
                  <TableRow
                    key={order.id}
                    className='cursor-pointer'
                    onClick={() => onSelect(order.id)}
                  >
                    <TableCell className='whitespace-nowrap'>
                      {formatTime(order.occurred_at)}
                    </TableCell>
                    <TableCell className='max-w-48 font-mono text-xs'>
                      <span
                        className='block truncate'
                        title={order.platform_order_id}
                      >
                        {order.platform_order_id}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div>{order.username || `#${order.user_id}`}</div>
                      <div className='text-muted-foreground text-xs'>
                        #{order.token_id} {order.token_name} ·{' '}
                        {order.token_masked_key}
                      </div>
                    </TableCell>
                    <TableCell>{order.model || '—'}</TableCell>
                    <TableCell className='text-right font-mono'>
                      {formatMoney(
                        order.customer_charge_rmb_mic,
                        t('Pending confirmation')
                      )}
                    </TableCell>
                    <TableCell className='text-right font-mono'>
                      {formatMoney(
                        order.aipdd_charge_rmb_mic,
                        t('Pending confirmation')
                      )}
                    </TableCell>
                    <TableCell
                      className={`text-right font-mono ${Number(order.estimated_profit_rmb_mic) < 0 ? 'text-destructive' : ''}`}
                    >
                      {formatMoney(
                        order.source_cost_confirmed
                          ? order.confirmed_profit_rmb_mic
                          : order.estimated_profit_rmb_mic,
                        t('Pending confirmation')
                      )}
                    </TableCell>
                    <TableCell>
                      <div className='flex flex-col items-start gap-1'>
                        <StatusBadge value={order.cost_status} />
                        <StatusBadge value={order.local_billing_status} />
                      </div>
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        size='xs'
                        variant='outline'
                        onClick={(event) => {
                          event.stopPropagation()
                          onSelect(order.id)
                        }}
                      >
                        {t('Details')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        <div className='flex items-center justify-between gap-3 px-3 pt-3'>
          <span className='text-muted-foreground text-xs'>
            {fetching
              ? t('Refreshing…')
              : t('Page {{page}} of {{pageCount}}', { page, pageCount })}
          </span>
          <div className='flex gap-2'>
            <Button
              size='sm'
              variant='outline'
              disabled={page <= 1}
              onClick={() => onPageChange(page - 1)}
            >
              {t('Previous')}
            </Button>
            <Button
              size='sm'
              variant='outline'
              disabled={page >= pageCount}
              onClick={() => onPageChange(page + 1)}
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function SyncTable({
  statuses,
  loading,
  onRetry,
  retrying,
  t,
}: {
  statuses: Awaited<ReturnType<typeof getFinanceSyncStatus>>['data']
  loading: boolean
  onRetry: (channelId: number, instanceId: string) => void
  retrying: boolean
  t: (key: string) => string
}) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('Settlement synchronization')}</CardTitle>
        <CardDescription>
          {t(
            'Manual retry only refreshes settlement snapshots and never guesses a charge or refund'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='px-0'>
        <div className='overflow-x-auto border-y'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Channel')}</TableHead>
                <TableHead>{t('Instance ID')}</TableHead>
                <TableHead>{t('Cursor')}</TableHead>
                <TableHead>{t('Last success')}</TableHead>
                <TableHead>{t('Backlog')}</TableHead>
                <TableHead>{t('Last error')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={7}>
                    <Skeleton className='h-10 w-full' />
                  </TableCell>
                </TableRow>
              ) : statuses.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className='text-muted-foreground h-28 text-center'
                  >
                    {t('No AIPDD synchronization instance found')}
                  </TableCell>
                </TableRow>
              ) : (
                statuses.map((status) => (
                  <TableRow key={`${status.channel_id}:${status.instance_id}`}>
                    <TableCell>
                      <div>
                        {status.channel_name || `#${status.channel_id}`}
                      </div>
                      {!status.single_key_valid && (
                        <Badge variant='destructive'>
                          {t('Multi-key is not allowed')}
                        </Badge>
                      )}
                    </TableCell>
                    <TableCell className='font-mono text-xs'>
                      {status.instance_id}
                    </TableCell>
                    <TableCell>{status.last_sequence}</TableCell>
                    <TableCell>{formatTime(status.last_success_at)}</TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          status.backlog_count > 0 ? 'destructive' : 'secondary'
                        }
                      >
                        {status.backlog_count}
                      </Badge>
                    </TableCell>
                    <TableCell className='text-destructive max-w-72 text-xs'>
                      {status.last_error || '—'}
                    </TableCell>
                    <TableCell className='text-right'>
                      <Button
                        size='sm'
                        variant='outline'
                        disabled={retrying || !status.single_key_valid}
                        onClick={() =>
                          onRetry(status.channel_id, status.instance_id)
                        }
                      >
                        {t('Safe refresh')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}

function OrderDetailSheet({
  open,
  onOpenChange,
  detail,
  loading,
  t,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  detail?: Awaited<ReturnType<typeof getFinanceOrderDetail>>['data']
  loading: boolean
  t: (key: string, options?: Record<string, unknown>) => string
}) {
  const order = detail?.order
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full overflow-y-auto sm:max-w-3xl'>
        <SheetHeader>
          <SheetTitle>{t('Financial order details')}</SheetTitle>
          <SheetDescription>
            {order?.platform_order_id || t('Loading order evidence…')}
          </SheetDescription>
        </SheetHeader>
        {loading || !order ? (
          <div className='grid gap-3 p-4'>
            <Skeleton className='h-24 w-full' />
            <Skeleton className='h-64 w-full' />
          </div>
        ) : (
          <div className='flex flex-col gap-4 px-4 pb-6'>
            <Alert>
              <AlertTitle>{t('NewAPI profit formula')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Customer charge RMB − AIPDD actual charge to NewAPI RMB. AIPDD internal spend is evidence only and is not used in this profit.'
                )}
              </AlertDescription>
            </Alert>
            <div className='grid gap-2 sm:grid-cols-3'>
              <AmountTile
                label={t('Customer charge AWCoin / quota')}
                value={String(order.customer_charge_quota)}
              />
              <AmountTile
                label={t('Customer charge RMB')}
                value={formatMoney(
                  order.customer_charge_rmb_mic,
                  t('Pending confirmation')
                )}
              />
              <AmountTile
                label={t('AIPDD source charge RMB')}
                value={formatMoney(
                  order.aipdd_charge_rmb_mic,
                  t('Pending confirmation')
                )}
              />
              <AmountTile
                label={t('Base model cost RMB')}
                value={formatMoney(
                  order.base_model_cost_rmb_mic,
                  t('Pending confirmation')
                )}
              />
              <AmountTile
                label={t('AIPDD model cost RMB')}
                value={formatMoney(
                  order.aipdd_model_cost_rmb_mic,
                  t('Pending confirmation')
                )}
              />
              <AmountTile
                label={
                  order.source_cost_confirmed
                    ? t('Confirmed profit RMB')
                    : t('Estimated profit RMB')
                }
                value={formatMoney(
                  order.source_cost_confirmed
                    ? order.confirmed_profit_rmb_mic
                    : order.estimated_profit_rmb_mic,
                  t('Pending confirmation')
                )}
              />
            </div>
            <DetailSection
              title={t('Cross-system mapping')}
              rows={[
                [t('NewAPI user'), `#${order.user_id} ${order.username}`],
                [
                  t('NewAPI token'),
                  `#${order.token_id} ${order.token_name} · ${order.token_masked_key}`,
                ],
                [
                  t('AIPDD channel / instance'),
                  `#${order.channel_id} ${order.channel_name} / ${order.instance_id}`,
                ],
                [t('Model'), order.model],
                [
                  t('Platform order / attempt'),
                  `${order.platform_order_id} / ${order.latest_attempt_id}`,
                ],
                [t('Upstream order'), order.upstream_reference || '—'],
                [t('Confidence'), order.financial_trace_completeness],
              ]}
            />
            <Card size='sm'>
              <CardHeader>
                <CardTitle>{t('Charge, refund, and cost revisions')}</CardTitle>
              </CardHeader>
              <CardContent className='px-0'>
                <div className='overflow-x-auto border-y'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('Time')}</TableHead>
                        <TableHead>{t('Component')}</TableHead>
                        <TableHead className='text-right'>
                          {t('AWCoin / quota delta')}
                        </TableHead>
                        <TableHead className='text-right'>
                          {t('RMB delta')}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {detail.movements.length === 0 ? (
                        <TableRow>
                          <TableCell
                            colSpan={4}
                            className='text-muted-foreground text-center'
                          >
                            {t('No movement evidence')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        detail.movements.map((movement) => (
                          <TableRow key={movement.id}>
                            <TableCell>
                              {formatTime(movement.occurred_at)}
                            </TableCell>
                            <TableCell>{movement.component}</TableCell>
                            <TableCell className='text-right font-mono'>
                              {movement.quota_delta}
                            </TableCell>
                            <TableCell className='text-right font-mono'>
                              {formatMoney(
                                movement.rmb_delta_mic,
                                t('Pending confirmation')
                              )}
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
            <Card size='sm'>
              <CardHeader>
                <CardTitle>{t('Settlement revision events')}</CardTitle>
                <CardDescription>
                  {t('{{count}} synchronized revisions', {
                    count: detail.settlement_events.length,
                  })}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='flex flex-col gap-2'>
                  {detail.settlement_events.map((event) => (
                    <div key={event.event_id} className='rounded-lg border p-3'>
                      <div className='flex justify-between gap-3'>
                        <span className='font-mono text-xs'>
                          {event.event_id}
                        </span>
                        <Badge variant='outline'>
                          v{event.settlement_revision}
                        </Badge>
                      </div>
                      <div className='text-muted-foreground mt-1 text-xs'>
                        {t('Sequence')} {event.source_sequence} ·{' '}
                        {formatTime(event.processed_at)}
                      </div>
                      {event.error_message && (
                        <p className='text-destructive mt-2 text-xs'>
                          {event.error_message}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        )}
      </SheetContent>
    </Sheet>
  )
}

function AmountTile({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 font-mono text-base font-semibold'>{value}</div>
    </div>
  )
}
function DetailSection({ title, rows }: { title: string; rows: string[][] }) {
  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className='grid gap-3 sm:grid-cols-2'>
          {rows.map(([label, value]) => (
            <div key={label}>
              <dt className='text-muted-foreground text-xs'>{label}</dt>
              <dd className='mt-1 text-sm break-all'>{value}</dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  )
}
function StatusBadge({ value }: { value: string }) {
  const destructive = value.includes('REVIEW') || value === 'UNVERIFIABLE'
  return (
    <Badge
      variant={
        destructive
          ? 'destructive'
          : value === 'CONFIRMED' || value === 'CHARGED'
            ? 'secondary'
            : 'outline'
      }
    >
      {value || 'UNKNOWN'}
    </Badge>
  )
}
function emptySummary() {
  return {
    order_count: 0,
    customer_net_consumption_rmb_mic: 0,
    confirmed_source_cost_rmb_mic: 0,
    confirmed_profit_rmb_mic: 0,
    estimated_source_cost_rmb_mic: 0,
    estimated_profit_rmb_mic: 0,
    loss_order_count: 0,
    pending_confirmation_count: 0,
    manual_review_count: 0,
  }
}
function formatMoney(
  value: number | null | undefined,
  pendingLabel = 'Pending confirmation'
) {
  if (value == null) return pendingLabel
  return `¥${(value / 1_000_000).toFixed(6)}`
}
function formatTime(value: number | null | undefined) {
  if (!value) return '—'
  return new Intl.DateTimeFormat(undefined, {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value * 1000))
}
function toDateTimeInput(value: Date) {
  const offset = value.getTimezoneOffset() * 60_000
  return new Date(value.getTime() - offset).toISOString().slice(0, 16)
}
function toApiFilter(value: DraftFilter): FinanceFilter {
  return {
    start_time: value.start
      ? Math.floor(new Date(value.start).getTime() / 1000)
      : undefined,
    end_time: value.end
      ? Math.floor(new Date(value.end).getTime() / 1000)
      : undefined,
    user: value.user.trim() || undefined,
    token: value.token.trim() || undefined,
    channel_id: Number(value.channel) > 0 ? Number(value.channel) : undefined,
    instance_id: value.instance.trim() || undefined,
    model: value.model.trim() || undefined,
    platform_order_id: value.order.trim() || undefined,
    cost_status: value.costStatus || undefined,
    issue_view: value.issueView || undefined,
  }
}
