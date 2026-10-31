// Applies the saved theme before first paint (same key as src/app/theme.ts).
// A separate file, not inline, so the CSP can forbid inline scripts.
try {
  const theme = localStorage.getItem("julius.theme");
  if (theme === "light" || theme === "dark") document.documentElement.dataset.theme = theme;
} catch {
  // Storage unavailable: the CSS follows the OS setting.
}
