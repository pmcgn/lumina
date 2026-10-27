import de from '../i18n/de.json';
import en from '../i18n/en.json';

type Messages = Record<string, string>;
const catalogs: Record<string, Messages> = { de, en };

class I18n {
  locale = $state<'de' | 'en'>('de');

  /** Translates a key; {name} placeholders are replaced from params. */
  t = (key: string, params?: Record<string, string | number>): string => {
    let s = catalogs[this.locale]?.[key] ?? catalogs.de[key] ?? key;
    if (params) for (const [k, v] of Object.entries(params)) s = s.replace(`{${k}}`, String(v));
    return s;
  };
}

export const i18n = new I18n();
