import { usePreferencesContext } from "@/contexts/PreferencesContext";
import { translations, type TranslationKey, type Locale } from "./translations";

export type { TranslationKey, Locale };

type TranslationFn = (key: TranslationKey, vars?: Record<string, string | number>) => string;

function translate(locale: Locale, key: TranslationKey, vars?: Record<string, string | number>): string {
  const dict = translations[locale] || translations.en;
  let text: string = dict[key] ?? key;
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      text = text.replaceAll(`{${k}}`, String(v));
    }
  }
  return text;
}

export function useT(): TranslationFn {
  const { preferences } = usePreferencesContext();
  const locale = preferences.locale || "en";
  return (key, vars) => translate(locale, key, vars);
}

/** Non-hook version for use outside React components (e.g. export-pdf) */
export function getT(locale: Locale): TranslationFn {
  return (key, vars) => translate(locale, key, vars);
}
