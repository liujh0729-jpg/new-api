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
import { useState, useEffect, useRef } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { cn } from '@/lib/utils'
import { useNotifications } from '@/hooks/use-notifications'
import { useSystemConfig } from '@/hooks/use-system-config'
import { useTopNavLinks } from '@/hooks/use-top-nav-links'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { LanguageSwitcher } from '@/components/language-switcher'
import { NotificationButton } from '@/components/notification-button'
import { NotificationDialog } from '@/components/notification-dialog'
import { ProfileDropdown } from '@/components/profile-dropdown'
import { ThemeSwitch } from '@/components/theme-switch'
import { defaultTopNavLinks } from '../config/top-nav.config'
import type { TopNavLink } from '../types'
import { HeaderLogo } from './header-logo'

export interface PublicHeaderProps {
  navLinks?: TopNavLink[]
  mobileLinks?: TopNavLink[]
  navContent?: React.ReactNode
  showThemeSwitch?: boolean
  showLanguageSwitcher?: boolean
  logo?: React.ReactNode
  siteName?: string
  homeUrl?: string
  leftContent?: React.ReactNode
  rightContent?: React.ReactNode
  showNavigation?: boolean
  showAuthButtons?: boolean
  showNotifications?: boolean
  className?: string
}

export function PublicHeader(props: PublicHeaderProps) {
  const {
    navLinks = defaultTopNavLinks,
    showThemeSwitch = true,
    showLanguageSwitcher = true,
    logo: customLogo,
    siteName: customSiteName,
    homeUrl = '/',
    showAuthButtons = true,
    showNotifications = true,
  } = props

  const { t } = useTranslation()
  const scrollSentinelRef = useRef<HTMLDivElement>(null)
  const [scrolled, setScrolled] = useState(false)
  const [mobileOpen, setMobileOpen] = useState(false)
  const { auth } = useAuthStore()
  const {
    systemName,
    logo: systemLogo,
    loading,
    logoLoaded,
  } = useSystemConfig()
  const dynamicLinks = useTopNavLinks()
  const notifications = useNotifications()
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const isHomePage = pathname === '/'

  const user = auth.user
  const isAuthenticated = !!user
  const displaySiteName = customSiteName || systemName
  const links = dynamicLinks.length > 0 ? dynamicLinks : navLinks

  useEffect(() => {
    const sentinel = scrollSentinelRef.current
    if (!sentinel) return

    const observer = new IntersectionObserver(
      ([entry]) => setScrolled(!entry.isIntersecting),
      { rootMargin: '0px' }
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    document.body.style.overflow = mobileOpen ? 'hidden' : ''
    return () => {
      document.body.style.overflow = ''
    }
  }, [mobileOpen])

  return (
    <>
      <div
        ref={scrollSentinelRef}
        aria-hidden='true'
        className='pointer-events-none absolute top-5 left-0 h-px w-px'
      />
      <header className='pointer-events-none fixed inset-x-0 top-0 z-50'>
        <div
          className={cn(
            'pointer-events-auto mx-auto transition-all duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]',
            scrolled ? 'max-w-6xl px-3 pt-3' : 'max-w-7xl px-4 pt-0 md:px-6'
          )}
        >
          <nav
            className={cn(
              'flex items-center justify-between transition-all duration-700 ease-[cubic-bezier(0.16,1,0.3,1)]',
              !scrolled && 'h-16 px-2',
              scrolled &&
                !isHomePage &&
                'bg-background/60 ring-border/50 h-12 rounded-2xl pr-1.5 pl-4 shadow-[0_2px_16px_-6px_rgba(0,0,0,0.08),0_0_0_0.5px_rgba(0,0,0,0.02)] ring-[0.5px] backdrop-blur-2xl dark:shadow-[0_2px_16px_-6px_rgba(0,0,0,0.4)]',
              scrolled &&
                isHomePage &&
                'h-12 rounded-2xl bg-white/82 pr-1.5 pl-4 text-[#14151a] shadow-[0_8px_30px_-14px_rgba(32,35,43,0.18)] ring-[0.5px] ring-black/10 backdrop-blur-2xl dark:bg-[#111216]/82 dark:text-white dark:shadow-[0_8px_30px_-14px_rgba(0,0,0,0.7)] dark:ring-white/15',
              isHomePage && 'text-[#14151a] dark:text-white'
            )}
          >
            {/* Logo */}
            <Link
              to={homeUrl}
              className='group flex shrink-0 items-center gap-2.5'
            >
              <div
                className={cn(
                  'flex size-7 shrink-0 items-center justify-center transition-all duration-300 group-hover:scale-105',
                  isHomePage && 'rounded-lg bg-white/95 p-0.5'
                )}
              >
                {loading ? (
                  <Skeleton className='size-full rounded-lg' />
                ) : customLogo ? (
                  customLogo
                ) : (
                  <HeaderLogo
                    src={systemLogo}
                    loading={loading}
                    logoLoaded={logoLoaded}
                    className='size-full rounded-lg object-contain'
                  />
                )}
              </div>
              <span className='text-sm font-semibold tracking-tight'>
                {loading ? <Skeleton className='h-4 w-16' /> : displaySiteName}
              </span>
            </Link>

            {/* Desktop nav */}
            <div className='hidden items-center gap-0.5 sm:flex'>
              {links.map((link, i) => {
                const isActive = pathname === link.href
                if (link.external) {
                  return (
                    <a
                      key={i}
                      href={link.href}
                      target='_blank'
                      rel='noopener noreferrer'
                      className={cn(
                        'rounded-lg px-3 py-1.5 text-[13px] font-medium transition-colors duration-200',
                        isHomePage
                          ? 'text-[#50545d] hover:text-[#14151a] dark:text-white/70 dark:hover:text-white'
                          : 'text-muted-foreground hover:text-foreground'
                      )}
                    >
                      {t(link.title)}
                    </a>
                  )
                }
                return (
                  <Link
                    key={i}
                    to={link.href}
                    className={cn(
                      'rounded-lg px-3 py-1.5 text-[13px] font-medium transition-colors duration-200',
                      isHomePage &&
                        (isActive
                          ? 'text-[#14151a] dark:text-white'
                          : 'text-[#50545d] hover:text-[#14151a] dark:text-white/70 dark:hover:text-white'),
                      !isHomePage &&
                        (isActive
                          ? 'text-foreground'
                          : 'text-muted-foreground hover:text-foreground')
                    )}
                  >
                    {t(link.title)}
                  </Link>
                )
              })}

              {(showLanguageSwitcher ||
                showThemeSwitch ||
                showNotifications) && (
                <div
                  className={cn(
                    'mx-2 h-4 w-px',
                    isHomePage ? 'bg-black/15 dark:bg-white/20' : 'bg-border/40'
                  )}
                />
              )}

              {showLanguageSwitcher && <LanguageSwitcher />}
              {showThemeSwitch && <ThemeSwitch />}
              {showNotifications && (
                <NotificationButton
                  unreadCount={notifications.unreadCount}
                  onClick={() => notifications.openDialog()}
                />
              )}

              {showAuthButtons && (
                <>
                  <div
                    className={cn(
                      'mx-1 h-4 w-px',
                      isHomePage
                        ? 'bg-black/15 dark:bg-white/20'
                        : 'bg-border/40'
                    )}
                  />
                  {loading ? (
                    <Skeleton className='h-8 w-20 rounded-lg' />
                  ) : isAuthenticated ? (
                    <ProfileDropdown />
                  ) : (
                    <Button
                      size='sm'
                      className={cn(
                        'h-8 rounded-full px-3.5 text-xs font-medium',
                        isHomePage &&
                          'bg-[#15161a] text-white hover:bg-[#2a2c32] dark:bg-[#f6f6f2] dark:text-[#101114] dark:hover:bg-white'
                      )}
                      render={<Link to='/sign-in' />}
                    >
                      {t('Sign in')}
                    </Button>
                  )}
                </>
              )}
            </div>

            {/* Mobile: compact actions + hamburger */}
            <div className='flex items-center gap-2 sm:hidden'>
              {showThemeSwitch && <ThemeSwitch />}
              {showAuthButtons && !loading && isAuthenticated && (
                <ProfileDropdown />
              )}
              <Button
                type='button'
                variant='ghost'
                size='icon'
                className='size-9'
                onClick={() => setMobileOpen((v) => !v)}
                aria-label={t('Toggle navigation menu')}
              >
                <div className='relative size-4'>
                  <span
                    className={cn(
                      'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300',
                      mobileOpen ? 'top-[7px] rotate-45' : 'top-[3px]'
                    )}
                  />
                  <span
                    className={cn(
                      'absolute inset-x-0 top-[7px] block h-[1.5px] rounded-full bg-current transition-all duration-300',
                      mobileOpen ? 'scale-x-0 opacity-0' : 'opacity-100'
                    )}
                  />
                  <span
                    className={cn(
                      'absolute inset-x-0 block h-[1.5px] origin-center rounded-full bg-current transition-all duration-300',
                      mobileOpen ? 'top-[7px] -rotate-45' : 'top-[11px]'
                    )}
                  />
                </div>
              </Button>
            </div>
          </nav>
        </div>
      </header>

      {/* Mobile full-screen overlay */}
      <div
        className={cn(
          'fixed inset-0 z-40 backdrop-blur-2xl transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] sm:pointer-events-none sm:hidden',
          pathname === '/'
            ? 'bg-[#f5f6f8]/98 text-[#14151a] dark:bg-[#08090b]/98 dark:text-[#f6f6f2]'
            : 'bg-background/98',
          mobileOpen
            ? 'pointer-events-auto opacity-100'
            : 'pointer-events-none opacity-0'
        )}
      >
        <div className='flex h-full flex-col justify-between px-8 pt-20 pb-10'>
          <nav className='flex flex-col gap-1'>
            {links.map((link, i) => {
              const isActive = pathname === link.href
              const linkClassName = cn(
                'flex items-center gap-3 py-3 text-base font-medium tracking-tight transition-all duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]',
                mobileOpen
                  ? 'translate-y-0 opacity-100'
                  : 'translate-y-4 opacity-0',
                pathname === '/' &&
                  (isActive
                    ? 'text-[#14151a] dark:text-white'
                    : 'text-[#50545d] dark:text-white/65'),
                pathname !== '/' &&
                  (isActive ? 'text-foreground' : 'text-muted-foreground')
              )
              const linkStyle = {
                transitionDelay: mobileOpen ? `${100 + i * 50}ms` : '0ms',
              }

              if (link.external) {
                return (
                  <a
                    key={i}
                    href={link.href}
                    target='_blank'
                    rel='noopener noreferrer'
                    onClick={() => setMobileOpen(false)}
                    className={linkClassName}
                    style={linkStyle}
                  >
                    {t(link.title)}
                  </a>
                )
              }

              return (
                <Link
                  key={i}
                  to={link.href}
                  onClick={() => setMobileOpen(false)}
                  className={linkClassName}
                  style={linkStyle}
                >
                  {t(link.title)}
                </Link>
              )
            })}
          </nav>

          <div
            className={cn(
              'flex flex-col gap-3 transition-all duration-500',
              mobileOpen
                ? 'translate-y-0 opacity-100'
                : 'translate-y-4 opacity-0'
            )}
            style={{ transitionDelay: mobileOpen ? '250ms' : '0ms' }}
          >
            {showAuthButtons && (
              <Link
                to={isAuthenticated ? '/dashboard' : '/sign-in'}
                onClick={() => setMobileOpen(false)}
                className={cn(
                  'inline-flex h-10 items-center justify-center rounded-full text-sm font-medium transition-opacity hover:opacity-90 active:opacity-80',
                  pathname === '/'
                    ? 'bg-[#15161a] text-white dark:bg-[#f6f6f2] dark:text-[#101114]'
                    : 'bg-foreground text-background'
                )}
              >
                {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
              </Link>
            )}
          </div>
        </div>
      </div>

      {/* Notification Dialog */}
      {showNotifications && (
        <NotificationDialog
          open={notifications.dialogOpen}
          onOpenChange={notifications.setDialogOpen}
          activeTab={notifications.activeTab}
          onTabChange={notifications.setActiveTab}
          notice={notifications.notice}
          announcements={notifications.announcements}
          loading={notifications.loading}
          onCloseToday={notifications.closeToday}
        />
      )}
    </>
  )
}
