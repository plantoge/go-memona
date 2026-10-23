export type Theme = 'light' | 'dark';
const preferenceKey = 'memona-theme';
export function initialTheme(storage: Pick<Storage, 'getItem'> | undefined, prefersDark: boolean): Theme {
  try {
    const saved = storage?.getItem(preferenceKey);
    if (saved === 'light' || saved === 'dark') return saved;
  } catch { /* Storage can be blocked by browser policy. */ }
  return prefersDark ? 'dark' : 'light';
}
export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  try { localStorage.setItem(preferenceKey, theme); } catch { /* In-memory toggle still works. */ }
}
export function startTheme(): Theme {
  let storage: Storage | undefined;
  try { storage = localStorage; } catch { /* Private mode / restricted storage. */ }
  const theme = initialTheme(storage, window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = theme;
  return theme;
}
