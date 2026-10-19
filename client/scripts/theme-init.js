// Apply the saved appearance before the stylesheet paints to avoid a theme flash.
(() => {
    let preference = 'system';
    try { preference = localStorage.getItem('asfwms-theme') || 'system'; } catch {}
    const systemDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
    const theme = preference === 'light' || preference === 'dark' ? preference : (systemDark ? 'dark' : 'light');
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#0b1429' : '#f2f6fd');
})();
