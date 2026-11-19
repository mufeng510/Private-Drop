import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useUiStore } from '../store/ui';

export const SEND_EVENT = 'pd:send-message';
/** Emitted by Escape when no modal/lightbox is open so pages can exit selection. */
export const ESC_EVENT = 'pd:escape';

/** Global shortcuts. The composer handles Ctrl+V itself while mounted. */
export function useKeyboardShortcuts(): void {
  const navigate = useNavigate();
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        navigate('/search');
        return;
      }
      if (mod && e.key === 'Enter') {
        e.preventDefault();
        window.dispatchEvent(new CustomEvent(SEND_EVENT));
        return;
      }
      if (e.key === 'Escape') {
        const ui = useUiStore.getState();
        // Close the topmost layer first: lightbox, then confirmation dialog.
        if (ui.lightbox) {
          ui.closeLightbox();
          return;
        }
        if (ui.confirm) {
          ui.resolveConfirm(false);
          return;
        }
        ui.setSidebarOpen(false);
        ui.setUploadPanelOpen(false);
        // Pages with their own transient UI (select mode, popovers) opt in.
        window.dispatchEvent(new Event(ESC_EVENT));
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [navigate]);
}
