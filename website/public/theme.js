// Apply a saved override before the first paint. Without one, CSS follows the OS.
try {
  const theme = localStorage.getItem("passpine-website-theme");
  if (theme === "light" || theme === "dark") {
    document.documentElement.dataset.theme = theme;
  }
} catch {
  // Storage can be unavailable in restricted browsing contexts.
}
