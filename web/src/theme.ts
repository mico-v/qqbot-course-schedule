import { ref } from "vue";

export type ThemeMode = "auto" | "light" | "dark";

const STORAGE_KEY = "qqbot-admin-theme";
const media = window.matchMedia("(prefers-color-scheme: dark)");

function readStoredMode(): ThemeMode {
  try {
    const saved = window.localStorage.getItem(STORAGE_KEY);
    if (saved === "auto" || saved === "light" || saved === "dark") return saved;
  } catch {
    /* Storage can be unavailable in privacy-restricted contexts. */
  }
  return "auto";
}

export const themeMode = ref<ThemeMode>(readStoredMode());

function isDark(): boolean {
  if (themeMode.value === "auto") return media.matches;
  return themeMode.value === "dark";
}

export function applyTheme(): void {
  const dark = isDark();
  document.documentElement.classList.toggle("dark", dark);
  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute("content", dark ? "#101722" : "#f3f6fb");
}

export function cycleTheme(): ThemeMode {
  const order: ThemeMode[] = ["auto", "light", "dark"];
  themeMode.value = order[(order.indexOf(themeMode.value) + 1) % order.length];
  try {
    window.localStorage.setItem(STORAGE_KEY, themeMode.value);
  } catch {
    /* Keep the current session usable even if persistence fails. */
  }
  applyTheme();
  return themeMode.value;
}

export function setupTheme(): void {
  applyTheme();
  const onChange = () => {
    if (themeMode.value === "auto") applyTheme();
  };
  if (typeof media.addEventListener === "function") {
    media.addEventListener("change", onChange);
  } else {
    media.addListener(onChange);
  }
}
