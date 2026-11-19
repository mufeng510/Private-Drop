import type { IconName } from './components/ui/Icon';

export interface NavItem {
  to: string;
  labelKey: string;
  icon: IconName;
  end?: boolean;
}

export const primaryNav: NavItem[] = [
  { to: '/', labelKey: 'nav.messages', icon: 'message', end: true },
  { to: '/files', labelKey: 'nav.files', icon: 'file' },
  { to: '/search', labelKey: 'nav.search', icon: 'search' },
  { to: '/favorites', labelKey: 'nav.favorites', icon: 'star' },
  { to: '/tags', labelKey: 'nav.tags', icon: 'tag' },
  { to: '/expiring', labelKey: 'nav.expiring', icon: 'clock' },
  { to: '/recycle', labelKey: 'nav.recycle', icon: 'trash' },
  { to: '/notifications', labelKey: 'nav.notifications', icon: 'bell' },
];

export const bottomNav: NavItem[] = [
  { to: '/', labelKey: 'nav.messages', icon: 'message', end: true },
  { to: '/files', labelKey: 'nav.files', icon: 'file' },
  { to: '/search', labelKey: 'nav.search', icon: 'search' },
  { to: '/notifications', labelKey: 'nav.notifications', icon: 'bell' },
  { to: '/settings', labelKey: 'nav.settings', icon: 'settings' },
];
