import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { AuditEntry, DiagnoseResult, LogEntry, StorageStats, SystemInfo } from '../../api/types';
import { logsApi, type LogType } from '../../api/logs';
import { systemApi } from '../../api/system';
import { errorMessage } from '../../api/client';
import { useUiStore } from '../../store/ui';
import { Button, EmptyState, ProgressBar, Spinner, inputClass } from '../../components/ui/primitives';
import { Icon } from '../../components/ui/Icon';
import { formatBytes, formatDateTime } from '../../lib/format';

function Card({ title, description, children }: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <section className="rounded-2xl border border-slate-200 bg-white p-5 dark:border-slate-800 dark:bg-slate-900">
      <h2 className="text-base font-semibold">{title}</h2>
      {description && <p className="mt-1 text-xs text-slate-400">{description}</p>}
      <div className="mt-4">{children}</div>
    </section>
  );
}

export function LogsAndAudit() {
  const { t, i18n } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const [tab, setTab] = useState<'logs' | 'audit'>('logs');
  const [type, setType] = useState<LogType | 'all'>('all');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(false);

  const load = async () => {
    setLoading(true);
    try {
      const [l, a] = await Promise.all([
        logsApi.list(type === 'all' ? undefined : type, { limit: 200 }),
        logsApi.audit({ limit: 200 }),
      ]);
      setLogs(l.items);
      setAudit(a.items);
    } catch (e) {
      toast(errorMessage(e), 'error');
    } finally {
      setLoading(false);
    }
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps -- load is recreated each render; run on mount/filter change only
  useEffect(() => { void load(); }, [type]);

  return (
    <Card title={t('admin.logs')}>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="inline-flex rounded-lg border border-slate-200 p-0.5 dark:border-slate-700">
          {(['logs', 'audit'] as const).map((item) => (
            <button key={item} type="button" onClick={() => setTab(item)} aria-pressed={tab === item} className={`rounded-md px-3 py-1.5 text-xs font-medium ${tab === item ? 'bg-blue-600 text-white' : 'text-slate-500'}`}>
              {t(item === 'logs' ? 'admin.logs' : 'admin.audit')}
            </button>
          ))}
        </div>
        {tab === 'logs' && (
          <select value={type} onChange={(e) => setType(e.target.value as LogType | 'all')} className={inputClass + ' w-auto !py-1.5 !text-xs'} aria-label={t('admin.logType')}>
            {(['all', 'app', 'upload', 'clean', 'error', 'security'] as const).map((v) => <option key={v} value={v}>{v === 'all' ? t('common.all') : v}</option>)}
          </select>
        )}
        <div className="flex-1" />
        <Button variant="ghost" icon="refresh" onClick={() => void load()}>{t('common.refresh')}</Button>
      </div>
      {loading ? <div className="flex justify-center py-8"><Spinner /></div> : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="text-slate-400">
              <tr>
                <th scope="col" className="py-2 pr-3">{t('admin.time')}</th>
                {tab === 'logs' ? (
                  <>
                    <th scope="col" className="py-2 pr-3">{t('admin.level')}</th>
                    <th scope="col" className="py-2 pr-3">{t('admin.message')}</th>
                  </>
                ) : (
                  <>
                    <th scope="col" className="py-2 pr-3">{t('admin.action')}</th>
                    <th scope="col" className="py-2 pr-3">{t('admin.detail')}</th>
                    <th scope="col" className="py-2 pr-3">{t('admin.ip')}</th>
                  </>
                )}
              </tr>
            </thead>
            <tbody>
              {tab === 'logs' ? logs.map((log) => (
                <tr key={log.id} className="border-t border-slate-100 dark:border-slate-800">
                  <td className="py-2 pr-3 whitespace-nowrap text-slate-400">{formatDateTime(log.createdAt, i18n.language)}</td>
                  <td className="py-2 pr-3"><span className={`rounded px-1.5 py-0.5 ${log.level === 'error' ? 'bg-red-100 text-red-700' : log.level === 'warn' ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-600'}`}>{log.level}</span></td>
                  <td className="py-2 pr-3">{log.message}</td>
                </tr>
              )) : audit.map((entry) => (
                <tr key={entry.id} className="border-t border-slate-100 dark:border-slate-800">
                  <td className="py-2 pr-3 whitespace-nowrap text-slate-400">{formatDateTime(entry.createdAt, i18n.language)}</td>
                  <td className="py-2 pr-3 font-medium">{entry.action}</td>
                  <td className="py-2 pr-3">{entry.detail || '—'}</td>
                  <td className="py-2 pr-3">{entry.ip || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {(tab === 'logs' ? logs : audit).length === 0 && <EmptyState title={t('admin.noLogs')} />}
        </div>
      )}
    </Card>
  );
}

export function SystemStatusView() {
  const { t } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const [info, setInfo] = useState<SystemInfo | null>(null);
  useEffect(() => {
    systemApi.info().then(setInfo).catch((e) => toast(errorMessage(e), 'error'));
  }, [toast]);
  const rows: Array<[string, string]> = info ? [
    [t('admin.version'), info.version],
    [t('admin.port'), String(info.port)],
    [t('admin.dataDir'), info.dataDir],
    [t('admin.os'), info.os || t('common.unknown')],
    [t('admin.uptime'), info.uptime || info.startedAt || '—'],
    [t('admin.deployInfo'), info.deployment || 'Docker'],
  ] : [];
  return (
    <Card title={t('admin.system')}>
      {!info ? <Spinner /> : (
        <dl className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {rows.map(([label, value]) => (
            <div key={label} className="rounded-lg bg-slate-50 p-3 dark:bg-slate-800">
              <dt className="text-xs text-slate-400">{label}</dt>
              <dd className="mt-0.5 break-all text-sm font-medium">{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </Card>
  );
}

export function StorageStatsView() {
  const { t } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const [stats, setStats] = useState<StorageStats | null>(null);
  useEffect(() => {
    systemApi.storage().then(setStats).catch((e) => toast(errorMessage(e), 'error'));
  }, [toast]);
  if (!stats) return <Card title={t('admin.storage')}><Spinner /></Card>;
  const cards: Array<[string, string]> = [
    [t('storage.total'), formatBytes(stats.totalBytes)],
    [t('storage.files'), String(stats.fileCount)],
    [t('storage.messages'), String(stats.messageCount)],
    [t('storage.recycleBytes'), formatBytes(stats.recycleBytes)],
    [t('storage.favoriteBytes'), formatBytes(stats.favoriteBytes)],
    [t('storage.uploadTemp'), formatBytes(stats.uploadTempBytes)],
    ...(stats.duplicateBytes !== undefined ? [[t('storage.duplicates'), formatBytes(stats.duplicateBytes)]] as Array<[string, string]> : []),
  ];
  return (
    <div className="flex flex-col gap-5">
      <Card title={t('admin.storage')}>
        <dl className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          {cards.map(([label, value]) => (
            <div key={label} className="rounded-lg bg-slate-50 p-3 dark:bg-slate-800">
              <dt className="text-xs text-slate-400">{label}</dt>
              <dd className="mt-0.5 text-sm font-semibold tabular-nums">{value}</dd>
            </div>
          ))}
        </dl>
      </Card>
      <Card title={t('storage.byType')}>
        <div className="flex flex-col gap-3">
          {stats.byType.map((item) => (
            <div key={item.type}>
              <div className="mb-1 flex justify-between text-xs"><span>{t(`files.${item.type}`)}</span><span className="tabular-nums text-slate-400">{formatBytes(item.bytes)} · {item.count}</span></div>
              <ProgressBar value={stats.totalBytes > 0 ? (item.bytes / stats.totalBytes) * 100 : 0} />
            </div>
          ))}
        </div>
      </Card>
      <Card title={t('storage.largest')}>
        {stats.largestFiles.length === 0 ? <p className="text-sm text-slate-400">{t('common.noData')}</p> : (
          <ul className="flex flex-col gap-1.5">
            {stats.largestFiles.slice(0, 10).map((file) => (
              <li key={file.id} className="flex justify-between rounded-lg bg-slate-50 px-3 py-2 text-sm dark:bg-slate-800">
                <span className="truncate">{file.name}</span><span className="tabular-nums text-slate-400">{formatBytes(file.size)}</span>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card title={t('storage.warnings')}>
        {stats.warnings && stats.warnings.length > 0 ? (
          <ul className="flex flex-col gap-1.5">{stats.warnings.map((w, i) => <li key={i} className="rounded-lg bg-amber-50 p-2 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">{w}</li>)}</ul>
        ) : <p className="text-sm text-slate-400">{t('storage.noWarnings')}</p>}
      </Card>
    </div>
  );
}

export function DiagnosticsView() {
  const { t, i18n } = useTranslation();
  const toast = useUiStore((s) => s.toast);
  const [result, setResult] = useState<DiagnoseResult | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async () => {
    setBusy(true);
    try { setResult(await systemApi.diagnose()); } catch (e) { toast(errorMessage(e), 'error'); }
    finally { setBusy(false); }
  };
  return (
    <Card title={t('admin.diagnostics')}>
      <Button variant="primary" icon="activity" loading={busy} onClick={() => void run()} className="self-start">{t('admin.runDiagnose')}</Button>
      {result && (
        <div className="mt-4">
          <p className={`mb-3 text-sm font-medium ${result.ok ? 'text-emerald-600' : 'text-red-600'}`}>
            {result.ok ? t('admin.diagnoseOk') : t('admin.diagnoseIssues', { count: result.checks.filter((c) => !c.ok).length })}
          </p>
          <ul className="flex flex-col gap-2">
            {result.checks.map((check) => (
              <li key={check.name} className="flex items-center gap-2 rounded-lg border border-slate-200 p-2.5 text-sm dark:border-slate-700">
                <Icon name={check.ok ? 'success' : 'alert'} size={16} className={check.ok ? 'text-emerald-600' : 'text-red-600'} />
                <span className="flex-1">{check.name}</span>
                <span className="text-xs text-slate-400">{check.message}</span>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-slate-400">{t('admin.checkedAt')}: {formatDateTime(result.checkedAt, i18n.language)}</p>
        </div>
      )}
    </Card>
  );
}
