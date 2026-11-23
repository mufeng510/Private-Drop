import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTagsStore } from '../../store/tags';
import { Icon, iconFromString } from './Icon';
import { Button, TextInput } from './primitives';

const COLORS = ['#2563eb', '#16a34a', '#dc2626', '#d97706', '#7c3aed', '#0891b2', '#db2777', '#64748b'];

export function TagPicker({ selectedIds, onChange, compact = false }: { selectedIds: string[]; onChange: (ids: string[]) => void; compact?: boolean }) {
  const { t } = useTranslation();
  const tags = useTagsStore((s) => s.items);
  const load = useTagsStore((s) => s.load);
  const create = useTagsStore((s) => s.create);
  const [open, setOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const [newColor, setNewColor] = useState(COLORS[0]);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  const toggle = (id: string) =>
    onChange(selectedIds.includes(id) ? selectedIds.filter((x) => x !== id) : [...selectedIds, id]);

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className={compact
          ? 'relative inline-flex size-9 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800'
          : 'relative inline-flex min-h-9 items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 text-sm dark:border-slate-600'}
      >
        <Icon name="tag" size={16} />
        {!compact && <span>{t('messages.addTag')}</span>}
        {selectedIds.length > 0 && (
          <span className="absolute -right-1 -top-1 flex min-w-4 items-center justify-center rounded-full bg-blue-600 px-1 text-[10px] font-semibold leading-4 text-white">
            {selectedIds.length}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute left-0 top-full z-50 mt-1 w-60 rounded-xl border border-slate-200 bg-white p-2 shadow-xl dark:border-slate-700 dark:bg-slate-800">
          <div className="max-h-52 overflow-y-auto">
            {tags.filter((tag) => !tag.isSystem).map((tag) => (
              <label key={tag.id} className="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm hover:bg-slate-100 dark:hover:bg-slate-700">
                <input type="checkbox" checked={selectedIds.includes(tag.id)} onChange={() => toggle(tag.id)} className="size-4 accent-blue-600" />
                {iconFromString(tag.icon)
                  ? <Icon name={iconFromString(tag.icon)!} size={12} style={{ color: tag.color || '#64748b' }} />
                  : <span className="size-2.5 rounded-full" style={{ background: tag.color || '#64748b' }} />}
                <span className="flex-1 truncate">{tag.name}</span>
              </label>
            ))}
            {tags.filter((tag) => !tag.isSystem).length === 0 && <p className="px-2 py-3 text-xs text-slate-400">{t('tags.noTags')}</p>}
          </div>
          <div className="mt-2 border-t border-slate-200 pt-2 dark:border-slate-700">
            <p className="mb-1.5 text-xs font-medium text-slate-500">{t('tags.newTag')}</p>
            <div className="flex gap-1.5">
              <TextInput value={newName} onChange={(e) => setNewName(e.target.value)} placeholder={t('tags.tagName')} className="min-w-0 flex-1" />
              <Button
                variant="primary"
                onClick={() => {
                  const name = newName.trim();
                  if (!name) return;
                  void create({ name, color: newColor }).then((tag) => {
                    onChange([...selectedIds, tag.id]);
                    setNewName('');
                    setOpen(false);
                  });
                }}
              >
                <Icon name="plus" size={16} />
              </Button>
            </div>
            <div className="mt-2 flex gap-1.5">
              {COLORS.map((color) => (
                <button
                  key={color}
                  type="button"
                  aria-label={color}
                  onClick={() => setNewColor(color)}
                  className={`size-5 rounded-full ${newColor === color ? 'ring-2 ring-slate-800 dark:ring-white' : ''}`}
                  style={{ background: color }}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
