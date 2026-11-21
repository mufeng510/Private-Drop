import { useEffect } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useLocation } from 'react-router-dom';
import { authApi } from './api/auth';
import { useAuthStore } from './store/auth';
import { useNotificationsStore } from './store/notifications';
import { useTagsStore } from './store/tags';
import { useUploadsStore } from './store/uploads';
import { connectRealtime } from './realtime';
import { AppShell } from './components/layout/AppShell';
import { InitPage } from './pages/InitPage';
import { LoginPage } from './pages/LoginPage';
import { MessagesPage } from './pages/MessagesPage';
import { FilesPage } from './pages/FilesPage';
import { SearchPage } from './pages/SearchPage';
import { FavoritesPage } from './pages/FavoritesPage';
import { TagsPage } from './pages/TagsPage';
import { RecyclePage } from './pages/RecyclePage';
import { NotificationsPage } from './pages/NotificationsPage';
import { ExpiringPage } from './pages/ExpiringPage';
import { AdminPage } from './pages/AdminPage';

function Bootstrap({ children }: { children: React.ReactNode }) {
  const phase = useAuthStore((s) => s.phase);
  const setStatus = useAuthStore((s) => s.setStatus);
  const token = useAuthStore((s) => s.token);
  const setLogin = useAuthStore((s) => s.setLogin);
  const setMe = useAuthStore((s) => s.setMe);
  const setGuest = useAuthStore((s) => s.setGuest);

  useEffect(() => {
    let alive = true;
    const run = async () => {
      try {
        const status = await authApi.status();
        if (!alive) return;
        setStatus(status);
      } catch {
        /* backend may not be ready yet */
      }
      if (!token) {
        if (alive) useAuthStore.setState({ phase: 'guest' });
        return;
      }
      try {
        const me = await authApi.me();
        if (!alive) return;
        setLogin(token, me.user, me.device);
        setMe(me.user, me.device, me.settings, me.mfaValid);
        connectRealtime(token);
        // Contract: upload concurrency is a stored setting. Apply it at bootstrap
        // so a page refresh does not silently reset the manager to the default.
        const configuredConcurrency = Number(me.settings.uploadConcurrency) || 3;
        await Promise.allSettled([
          useUploadsStore.getState().init(configuredConcurrency),
          useNotificationsStore.getState().load(),
          useTagsStore.getState().load(),
        ]);
      } catch {
        if (alive) setGuest();
      }
    };
    void run();
    return () => { alive = false; };
  }, [token, setStatus, setLogin, setMe, setGuest]);

  if (phase === 'loading') {
    return <div className="flex h-dvh items-center justify-center text-sm text-slate-400">Private Drop</div>;
  }
  return <>{children}</>;
}

function AuthRoutes() {
  const phase = useAuthStore((s) => s.phase);
  const location = useLocation();
  if (phase === 'authenticated') {
    if (location.pathname === '/login' || location.pathname === '/init') return <Navigate to="/" replace />;
    return (
      <Routes>
        <Route element={<AppShell />}>
          <Route index element={<MessagesPage />} />
          <Route path="files" element={<FilesPage />} />
          <Route path="search" element={<SearchPage />} />
          <Route path="favorites" element={<FavoritesPage />} />
          <Route path="tags" element={<TagsPage />} />
          <Route path="expiring" element={<ExpiringPage />} />
          <Route path="recycle" element={<RecyclePage />} />
          <Route path="notifications" element={<NotificationsPage />} />
          <Route path="settings" element={<AdminPage />} />
          <Route path="admin" element={<AdminPage />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    );
  }
  return (
    <Routes>
      <Route path="/init" element={<InitPage />} />
      <Route path="*" element={<LoginPage />} />
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Bootstrap>
        <AuthRoutes />
      </Bootstrap>
    </BrowserRouter>
  );
}
