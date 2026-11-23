/**
 * Copy text to the clipboard with a fallback for browsers where the async
 * Clipboard API is unavailable or blocked (e.g. non-secure contexts, some
 * in-app browsers / standalone PWAs).
 *
 * The async API is preferred because it does not steal focus. When it is not
 * usable we fall back to a hidden textarea + execCommand('copy'), which still
 * works inside a user gesture on older browsers.
 */
export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard?.writeText && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch {
      // Fall through to the legacy path below.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  // iOS needs contentEditable in addition to a real selection for copy to work.
  textarea.contentEditable = 'true';
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  textarea.style.left = '-9999px';
  textarea.style.fontSize = '16px';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  try {
    textarea.select();
    textarea.setSelectionRange(0, textarea.value.length);
    const ok = document.execCommand('copy');
    if (!ok) throw new Error('execCommand("copy") was rejected by the browser');
  } finally {
    textarea.remove();
  }
}
