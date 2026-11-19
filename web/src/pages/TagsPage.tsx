import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Tag } from '../api/types';
import { useTagsStore } from '../store/tags';
import { useUiStore } from '../store/ui';
import { Button, EmptyState, IconButton, TextInput, inputClass } from '../components/ui/primitives';
import { Icon, iconFromString, type IconName } from '../components/ui/Icon';
import { TagChip } from '../components/ui/TagChip';
import { errorMessage } from '../api/client';

const COLORS = ['#2563eb', '#16a34a', '#dc2626', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#64748b'];
const ICONS: IconName[] = [
  'tag', 'star', 'pin', 'folder', 'file', 'image', 'video',
  'audio', 'document', 'code', 'link', 'globe', 'clock', 'alert', 'sparkles',
];

/** Small icon picker. An empty selection means "no icon" (colour dot only). */
function IconPicker({ value, onChange, size = 24 }: { value: string; onChange: (icon: string) => void; size?: number }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-1" role="group" aria-label={t('tags.icon')}>
      <button
        type="button"
        title={t('common.none')}
        aria-label={t('common.none')}
        onClick={() => onChange('')}
        className={`inline-flex items-center justify-center rounded-md border px-1.5 py-1 text-slate-400 ${!value ? 'border-blue-500 ring-2 ring-blue-500/20' : 'border-slate-200 dark:border-slate-700'}`}
        style={{ minHeight: size }}
      >
        <span className="size-2 rounded-full bg-current" />
      </button>
      {ICONS.filter((icon) => iconFromString(icon)).map((icon) => (
        <button
          key={icon}
          type="button"
          title={icon}
          aria-label={icon}
          aria-pressed={value === icon}
          onClick={() => onChange(value === icon ? '' : icon)}
          className={`inline-flex items-center justify-center rounded-md border px-1.5 py-1 ${value === icon ? 'border-blue-500 text-blue-600 ring-2 ring-blue-500/20' : 'border-slate-200 text-slate-500 hover:bg-slate-100 dark:border-slate-700 dark:hover:bg-slate-800'}`}
          style={{ minHeight: size }}
        >
          <Icon name={icon} size={size - 10} />
        </button>
      ))}
    </div>
  );
}

export function TagsPage() {
  const { t } = useTranslation();
  const tags = useTagsStore((s) => s.items);
  const load = useTagsStore((s) => s.load);
  const create = useTagsStore((s) => s.create);
  const update = useTagsStore((s) => s.update);
  const remove = useTagsStore((s) => s.remove);
  const confirm = useUiStore((s) => s.requestConfirm);
  const toast = useUiStore((s) => s.toast);
  const [name, setName] = useState('');
  const [color, setColor] = useState(COLORS[0]);
  const [icon, setIcon] = useState('');
  const [sort, setSort] = useState(0);
  const [editing, setEditing] = useState<Tag | null>(null);
  const [draftName, setDraftName] = useState('');
  const [draftColor, setDraftColor] = useState(COLORS[0]);
  const [draftIcon, setDraftIcon] = useState('');
  const [draftSort, setDraftSort] = useState(0);

  useEffect(() => { void load(); }, [load]);

  const systemTags = tags.filter((tag) => tag.isSystem);
  const userTags = tags.filter((tag) => !tag.isSystem).sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0) || a.name.localeCompare(b.name));

  const doCreate = async () => {
    if (!name.trim()) return;
    try {
      await create({ name: name.trim(), color, icon: icon || undefined, sort });
      setName('');
      setIcon('');
      setSort(0);
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const doUpdate = async () => {
    if (!editing || !draftName.trim()) return;
    try {
      await update(editing.id, { name: draftName.trim(), color: draftColor, icon: draftIcon || undefined, sort: draftSort });
      setEditing(null);
      toast(t('common.saved'), 'success');
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  const doDelete = async (tag: Tag) => {
    const ok = await confirm({ title: t('tags.deleteTag'), message: tag.name, danger: true, confirmLabel: t('common.delete') });
    if (!ok) return;
    try {
      await remove(tag.id);
    } catch (e) {
      toast(errorMessage(e), 'error');
    }
  };

  return (
    <div className="mx-auto w-full max-w-3xl px-3 py-4 sm:px-5">
      <section className="mb-6 rounded-2xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
        <h2 className="mb-3 text-sm font-semibold">{t('tags.newTag')}</h2>
        <div className="flex flex-wrap items-end gap-2">
          <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder={t('tags.tagName')} className="min-w-40 flex-1" />
          <label className="flex items-center gap-1.5 text-xs text-slate-500">
            {t('tags.sortOrder')}
            <input type="number" value={sort} onChange={(e) => setSort(Number(e.target.value) || 0)} className={inputClass + ' w-20 !py-1.5 !text-xs'} aria-label={t('tags.sortOrder')} />
          </label>
          <Button variant="primary" icon="plus" onClick={() => void doCreate()}>{t('tags.create')}</Button>
        </div>
        <div className="mt-3 flex flex-col gap-2">
          <span className="text-xs font-medium text-slate-500">{t('tags.color')}</span>
          <div className="flex flex-wrap gap-1.5">
            {COLORS.map((c) => (
              <button key={c} type="button" aria-label={c} onClick={() => setColor(c)} className={`size-6 rounded-full ${color === c ? 'ring-2 ring-slate-800 dark:ring-white' : ''}`} style={{ background: c }} />
            ))}
          </div>
          <span className="mt-1 text-xs font-medium text-slate-500">{t('tags.icon')}</span>
          <IconPicker value={icon} onChange={setIcon} />
        </div>
      </section>

      {systemTags.length > 0 && (
        <section className="mb-6">
          <h2 className="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-slate-400">{t('tags.systemTag')}</h2>
          <div className="flex flex-wrap gap-2">
            {systemTags.map((tag) => <TagChip key={tag.id} tag={tag} className="border border-slate-200 px-3 py-1.5 text-sm dark:border-slate-700" />)}
          </div>
        </section>
      )}

      <section>
        <h2 className="mb-2 px-1 text-xs font-semibold uppercase tracking-wide text-slate-400">{t('tags.userTag')}</h2>
        {userTags.length === 0 ? (
          <EmptyState icon="tag" title={t('tags.noTags')} />
        ) : (
          <ul className="flex flex-col gap-2">
            {userTags.map((tag) => (
              <li key={tag.id} className="flex flex-wrap items-center gap-3 rounded-xl border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
                {editing?.id === tag.id ? (
                  <>
                    <TextInput value={draftName} onChange={(e) => setDraftName(e.target.value)} className="min-w-32 flex-1" autoFocus />
                    <input type="number" value={draftSort} onChange={(e) => setDraftSort(Number(e.target.value) || 0)} className={inputClass + ' w-20 !py-1.5 !text-xs'} aria-label={t('tags.sortOrder')} />
                    <div className="flex w-full flex-wrap gap-1.5">
                      {COLORS.map((c) => (
                        <button key={c} type="button" aria-label={c} onClick={() => setDraftColor(c)} className={`size-5 rounded-full ${draftColor === c ? 'ring-2 ring-slate-800 dark:ring-white' : ''}`} style={{ background: c }} />
                      ))}
                    </div>
                    <div className="w-full"><IconPicker value={draftIcon} onChange={setDraftIcon} size={20} /></div>
                    <div className="flex gap-2">
                      <Button variant="primary" onClick={() => void doUpdate()}>{t('common.save')}</Button>
                      <Button variant="ghost" onClick={() => setEditing(null)}>{t('common.cancel')}</Button>
                    </div>
                  </>
                ) : (
                  <>
                    <TagChip tag={tag} className="text-sm" />
                    <span className="text-[10px] text-slate-400">#{tag.sort ?? 0}</span>
                    <div className="flex-1" />
                    <IconButton icon="edit" label={t('common.edit')} onClick={() => { setEditing(tag); setDraftName(tag.name); setDraftColor(tag.color || COLORS[0]); setDraftIcon(tag.icon || ''); setDraftSort(tag.sort ?? 0); }} />
                    <IconButton icon="trash" label={t('tags.deleteTag')} className="hover:text-red-600" onClick={() => void doDelete(tag)} />
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
      <div className="h-16 md:hidden" />
    </div>
  );
}
