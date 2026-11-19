import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { registerSW } from 'virtual:pwa-register';
import './i18n';
import './index.css';
import App from './App';

// vite-plugin-pwa: registering with autoUpdate swaps in the new worker when a
// new version is available. No custom prompt UI is needed for a personal app.
registerSW({ immediate: true });

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
