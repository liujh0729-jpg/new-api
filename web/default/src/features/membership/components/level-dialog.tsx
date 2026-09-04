import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type {
  MembershipLevel,
  MembershipLevelDraft,
  MembershipLevelInput,
} from '../types'

type LevelDialogProps = {
  open: boolean
  level: MembershipLevel | null
  draft: MembershipLevelDraft
  submitting: boolean
  onOpenChange: (open: boolean) => void
  onDraftChange: (draft: MembershipLevelDraft) => void
  onSubmit: (input: MembershipLevelInput) => void
}

export function LevelDialog(props: LevelDialogProps) {
  const { t } = useTranslation()

  const numericMultiplier = Number(props.draft.multiplier)
  const numericRank = Number(props.draft.rank)
  const numericSortOrder = Number(props.draft.sort_order)
  const isValid =
    /^[A-Z0-9][A-Z0-9_-]{0,63}$/.test(props.draft.code.trim()) &&
    props.draft.display_name.trim().length > 0 &&
    props.draft.display_name.trim().length <= 128 &&
    numericMultiplier > 0 &&
    numericMultiplier <= 1 &&
    Number.isInteger(numericRank) &&
    Number.isInteger(numericSortOrder)

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {props.level
              ? t('Edit membership level')
              : t('Add membership level')}
          </DialogTitle>
          <DialogDescription>
            {t('Multiplier 0.8 means the user pays 80% of the grouped price.')}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-4 py-2'>
          <div className='grid gap-2'>
            <Label htmlFor='membership-code'>{t('Code')}</Label>
            <Input
              id='membership-code'
              value={props.draft.code}
              disabled={Boolean(props.level)}
              placeholder='VIP1'
              onChange={(event) =>
                props.onDraftChange({
                  ...props.draft,
                  code: event.target.value.toUpperCase(),
                })
              }
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='membership-name'>{t('Display name')}</Label>
            <Input
              id='membership-name'
              value={props.draft.display_name}
              onChange={(event) =>
                props.onDraftChange({
                  ...props.draft,
                  display_name: event.target.value,
                })
              }
            />
          </div>
          <div className='grid gap-2 sm:grid-cols-3'>
            <div className='grid gap-2'>
              <Label htmlFor='membership-multiplier'>{t('Multiplier')}</Label>
              <Input
                id='membership-multiplier'
                type='number'
                min='0.000001'
                max='1'
                step='0.000001'
                value={props.draft.multiplier}
                disabled={props.level?.is_default}
                onChange={(event) =>
                  props.onDraftChange({
                    ...props.draft,
                    multiplier: event.target.value,
                  })
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='membership-rank'>{t('Rank')}</Label>
              <Input
                id='membership-rank'
                type='number'
                step='1'
                value={props.draft.rank}
                disabled={props.level?.is_default}
                onChange={(event) =>
                  props.onDraftChange({
                    ...props.draft,
                    rank: event.target.value,
                  })
                }
              />
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='membership-sort'>{t('Sort order')}</Label>
              <Input
                id='membership-sort'
                type='number'
                step='1'
                value={props.draft.sort_order}
                onChange={(event) =>
                  props.onDraftChange({
                    ...props.draft,
                    sort_order: event.target.value,
                  })
                }
              />
            </div>
          </div>
          <div className='flex items-center justify-between rounded-lg border p-3'>
            <div>
              <Label>{t('Enabled')}</Label>
              <p className='text-muted-foreground text-xs'>
                {t('Disabled levels no longer participate in resolution.')}
              </p>
            </div>
            <Switch
              checked={props.draft.enabled}
              disabled={props.level?.is_default}
              onCheckedChange={(enabled) =>
                props.onDraftChange({ ...props.draft, enabled })
              }
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            disabled={!isValid || props.submitting}
            onClick={() =>
              props.onSubmit({
                code: props.draft.code.trim(),
                display_name: props.draft.display_name.trim(),
                multiplier_ppm: Math.round(numericMultiplier * 1_000_000),
                rank: numericRank,
                sort_order: numericSortOrder,
                enabled: props.draft.enabled,
              })
            }
          >
            {t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
