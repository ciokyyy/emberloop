// Install-prompt helpers (pure, tested in install.test.js). InstallApp.svelte
// owns the beforeinstallprompt event wiring around them.
export function isIOSDevice(ua = '') {
  return /iphone|ipad|ipod/i.test(ua || '');
}

export function isStandalone({ matchMedia, standalone } = {}) {
  if (standalone === true) return true;
  try {
    if (typeof matchMedia === 'function' && matchMedia('(display-mode: standalone)').matches) return true;
  } catch {
    // matchMedia unavailable (tests, old browsers): not standalone.
  }
  return false;
}

// One state for the settings card: installed hides the card's button, prompt
// shows "Install app", ios-hint shows the Add-to-Home-Screen steps, and
// unavailable (desktop Chrome without a captured event, old browsers) hides
// the button but keeps the hint line.
export function installState({ standalone = false, hasPrompt = false, ios = false } = {}) {
  if (standalone) return 'installed';
  if (hasPrompt) return 'prompt';
  if (ios) return 'ios-hint';
  return 'unavailable';
}
