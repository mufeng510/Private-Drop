import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import { zhCN } from './zh-CN';
import { en } from './en';

export type Language = 'zh-CN' | 'en';

function initialLanguage(): Language {
  try {
    const stored = globalThis.localStorage?.getItem('pd.lang');
    if (stored === 'zh-CN' || stored === 'en') return stored;
  } catch {
    /* ignore */
  }
  return typeof navigator !== 'undefined' && navigator.language.toLowerCase().startsWith('zh')
    ? 'zh-CN'
    : 'en';
}

void i18n.use(initReactI18next).init({
  resources: {
    'zh-CN': { translation: zhCN },
    en: { translation: en },
  },
  lng: initialLanguage(),
  fallbackLng: 'zh-CN',
  interpolation: { escapeValue: false },
  returnNull: false,
});

export function setLanguage(lang: Language): void {
  void i18n.changeLanguage(lang);
  try {
    globalThis.localStorage?.setItem('pd.lang', lang);
  } catch {
    /* ignore */
  }
  if (typeof document !== 'undefined') document.documentElement.lang = lang;
}

if (typeof document !== 'undefined') document.documentElement.lang = i18n.language;

export default i18n;
