import { useAuthImage } from '../../hooks/useAuthImage';
import { Icon } from './Icon';

/** Image whose source is an authenticated API endpoint (blob URL under the hood). */
export function AuthImage({ path, alt, className, iconName = 'image' }: { path: string; alt: string; className?: string; iconName?: 'image' | 'video' | 'file' }) {
  const { url, error } = useAuthImage(path);
  if (!url) {
    return (
      <span className="flex size-full items-center justify-center bg-slate-100 text-slate-400 dark:bg-slate-800">
        <Icon name={error ? 'alert' : iconName} size={24} />
      </span>
    );
  }
  return <img src={url} alt={alt} className={className} loading="lazy" />;
}
