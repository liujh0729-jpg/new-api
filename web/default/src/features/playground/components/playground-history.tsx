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
import { useMemo, useState } from 'react'
import {
  HistoryIcon,
  MessageSquareIcon,
  MoreHorizontalIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { ConfirmDialog } from '@/components/confirm-dialog'
import type { PlaygroundConversation, PlaygroundMode } from '../types'

interface PlaygroundHistoryProps {
  conversations: PlaygroundConversation[]
  activeConversationId: string
  disabled?: boolean
  onNewConversation: () => void
  onSelectConversation: (conversationId: string) => void
  onDeleteConversation: (conversationId: string) => void
}

function formatConversationDate(updatedAt: number): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(updatedAt))
}

function getDisplayTitle(
  conversation: PlaygroundConversation,
  t: (key: string) => string
): string {
  if (
    conversation.title === 'New conversation' ||
    conversation.title === 'Image conversation' ||
    conversation.title === 'Video conversation'
  ) {
    return t(conversation.title)
  }
  return conversation.title
}

function getModeLabel(
  mode: PlaygroundMode,
  t: (key: string) => string
): string {
  if (mode === 'image') return t('Image')
  if (mode === 'video') return t('Video')
  return t('Chat')
}

function HistoryContent({
  conversations,
  activeConversationId,
  disabled = false,
  onNewConversation,
  onSelectConversation,
  onDeleteConversation,
  onAfterSelect,
  showHeader = true,
}: PlaygroundHistoryProps & {
  onAfterSelect?: () => void
  showHeader?: boolean
}) {
  const { t } = useTranslation()
  const [deleteTarget, setDeleteTarget] =
    useState<PlaygroundConversation | null>(null)
  const sortedConversations = useMemo(
    () => [...conversations].sort((a, b) => b.updatedAt - a.updatedAt),
    [conversations]
  )

  const handleSelect = (conversationId: string) => {
    if (disabled || conversationId === activeConversationId) return
    onSelectConversation(conversationId)
    onAfterSelect?.()
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    onDeleteConversation(deleteTarget.id)
    setDeleteTarget(null)
  }

  return (
    <>
      {showHeader && (
        <div className='flex items-center justify-between gap-2 border-b px-3 py-3'>
          <div className='min-w-0'>
            <h2 className='truncate text-sm font-semibold'>
              {t('Chat history')}
            </h2>
          </div>
          <Button
            aria-label={t('New chat')}
            disabled={disabled}
            onClick={onNewConversation}
            size='sm'
            variant='secondary'
          >
            <PlusIcon size={15} />
            <span>{t('New chat')}</span>
          </Button>
        </div>
      )}

      <ScrollArea className='min-h-0 flex-1'>
        <div className='space-y-1 p-2'>
          {sortedConversations.length === 0 ? (
            <div className='text-muted-foreground px-3 py-8 text-center text-sm'>
              {t('No conversations yet')}
            </div>
          ) : (
            sortedConversations.map((conversation) => {
              const active = conversation.id === activeConversationId
              const title = getDisplayTitle(conversation, t)

              return (
                <div
                  className={cn(
                    'group flex items-center gap-1 rounded-lg',
                    active
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:bg-muted/60'
                  )}
                  key={conversation.id}
                >
                  <button
                    aria-current={active ? 'true' : undefined}
                    aria-label={`${t('Open conversation')}: ${title}`}
                    className='flex min-w-0 flex-1 items-start gap-2 rounded-lg px-2.5 py-2 text-left disabled:cursor-not-allowed'
                    disabled={disabled}
                    onClick={() => handleSelect(conversation.id)}
                    type='button'
                  >
                    <MessageSquareIcon className='mt-0.5 size-4 shrink-0' />
                    <span className='min-w-0 flex-1'>
                      <span className='block truncate text-sm font-medium'>
                        {title}
                      </span>
                      <span className='text-muted-foreground mt-0.5 flex items-center gap-1.5 text-xs'>
                        <span>{getModeLabel(conversation.config.mode, t)}</span>
                        <span aria-hidden='true'>·</span>
                        <span>
                          {formatConversationDate(conversation.updatedAt)}
                        </span>
                      </span>
                    </span>
                  </button>

                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          aria-label={t('Conversation actions')}
                          className='mr-1 opacity-70 hover:opacity-100'
                          disabled={disabled}
                          size='icon-sm'
                          variant='ghost'
                        />
                      }
                    >
                      <MoreHorizontalIcon size={15} />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align='end'>
                      <DropdownMenuItem
                        className='text-destructive focus:text-destructive'
                        onClick={() => setDeleteTarget(conversation)}
                      >
                        <Trash2Icon className='mr-2 size-4' />
                        {t('Delete conversation')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            })
          )}
        </div>
      </ScrollArea>

      <ConfirmDialog
        destructive
        confirmText={t('Delete')}
        desc={t('This conversation will be removed from this browser.')}
        handleConfirm={handleDelete}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        open={!!deleteTarget}
        title={t('Delete conversation')}
      />
    </>
  )
}

export function PlaygroundHistorySidebar(props: PlaygroundHistoryProps) {
  return (
    <aside className='bg-background hidden w-[280px] shrink-0 flex-col border-r lg:flex'>
      <HistoryContent {...props} />
    </aside>
  )
}

export function PlaygroundHistoryMobileHeader(props: PlaygroundHistoryProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <div className='bg-background flex items-center justify-between gap-2 border-b px-3 py-2 lg:hidden'>
      <Sheet open={open} onOpenChange={setOpen}>
        <Button onClick={() => setOpen(true)} size='sm' variant='outline'>
          <HistoryIcon size={15} />
          <span>{t('History')}</span>
        </Button>
        <SheetContent className='w-[320px] max-w-[85vw] gap-0 p-0' side='left'>
          <SheetHeader className='border-b px-4 py-3 text-start'>
            <SheetTitle>{t('Chat history')}</SheetTitle>
            <SheetDescription className='sr-only'>
              {t('Local playground conversations')}
            </SheetDescription>
          </SheetHeader>
          <HistoryContent
            {...props}
            onAfterSelect={() => setOpen(false)}
            showHeader={false}
          />
        </SheetContent>
      </Sheet>

      <Button
        aria-label={t('New chat')}
        disabled={props.disabled}
        onClick={props.onNewConversation}
        size='sm'
        variant='secondary'
      >
        <PlusIcon size={15} />
        <span>{t('New chat')}</span>
      </Button>
    </div>
  )
}
