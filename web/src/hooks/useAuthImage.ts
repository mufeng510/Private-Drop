import { useEffect, useState } from 'react';
import { getAuthBlobUrl } from '../lib/blob';

/** Turn an authenticated binary endpoint into a blob URL for <img>/<video>. */
export function useAuthImage(path?: string): { url?: string; error?: string } {
  const [state, setState] = useState<{ url?: string; error?: string }>({});
  useEffect(() => {
    let alive = true;
    setState({});
    if (!path) return;
    getAuthBlobUrl(path)
      .then((url) => alive && setState({ url }))
      .catch((e) => alive && setState({ error: e instanceof Error ? e.message : String(e) }));
    return () => {
      alive = false;
    };
  }, [path]);
  return state;
}
