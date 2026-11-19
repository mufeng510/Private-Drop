import type { Tag } from '../../api/types';
import { Icon, iconFromString } from './Icon';

/** A tag chip that renders the optional icon chosen on the Tags page. */
export function TagChip({ tag, className = '' }: { tag: Tag; className?: string }) {
  const color = tag.color || '#64748b';
  const icon = iconFromString(tag.icon);
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium ${className}`}
      style={{ background: `${color}22`, color }}
    >
      {icon ? <Icon name={icon} size={11} /> : <span className="size-1.5 rounded-full" style={{ background: color }} />}
      {tag.name}
    </span>
  );
}
