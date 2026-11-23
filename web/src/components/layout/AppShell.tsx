import { useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { useUiStore } from '../../store/ui';
import { Sidebar } from './Sidebar';
import { TopBar } from './TopBar';
import { ConfirmDialog, Toasts } from '../ui/Overlays';
import { Lightbox } from '../Lightbox';
import { UploadPanel } from '../upload/UploadPanel';
import { ShareHandler } from '../ShareHandler';
import { useKeyboardShortcuts } from '../../hooks/useKeyboardShortcuts';
import { IconButton } from '../ui/primitives';

function useThemeEffect(): void {
  const theme = useUiStore((s) => s.theme);
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const apply = () => {
      const dark = theme === 'dark' || (theme === 'system' && media.matches);
      document.documentElement.classList.toggle('dark', dark);
    };
    apply();
    media.addEventListener('change', apply);
    return () => media.removeEventListener('change', apply);
  }, [theme]);
}

export function AppShell() {
  const sidebarOpen = useUiStore((s) => s.sidebarOpen);
  const setSidebarOpen = useUiStore((s) => s.setSidebarOpen);
  const wsStatus = useUiStore((s) => s.wsStatus);
  useThemeEffect();
  useKeyboardShortcuts();

  return (
    <div className="flex h-dvh overflow-hidden bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
      <a href="#main-content" className="skip-link visually-hidden">Skip to content</a>
      <div className="hidden md:flex"><Sidebar /></div>

      {sidebarOpen && (
        <div className="fixed inset-0 z-50 flex md:hidden">
          <div className="absolute inset-0 bg-black/40" onClick={() => setSidebarOpen(false)} />
          <div className="relative z-10 h-full"><Sidebar /></div>
          <div className="absolute right-3 top-3 z-10"><IconButton icon="x" label="Close" onClick={() => setSidebarOpen(false)} className="bg-white text-slate-800 dark:bg-slate-800 dark:text-white" /></div>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar />
        <main id="main-content" tabIndex={-1} className="min-h-0 flex-1 overflow-hidden focus:outline-none">
          <div className="h-full overflow-y-auto">
            <Outlet />
          </div>
        </main>
      </div>

      <div className="pointer-events-none fixed left-1/2 top-16 z-20 -translate-x-1/2">
        {wsStatus === 'reconnecting' && (
          <span className="pointer-events-auto rounded-full bg-amber-500/90 px-3 py-1 text-[11px] font-medium text-white shadow">Reconnecting…</span>
        )}
      </div>

      <ShareHandler />
      <ConfirmDialog />
      <Toasts />
      <Lightbox />
      <UploadPanel />
    </div>
  );
}
