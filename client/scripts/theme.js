// Same saved theme on access and workspace pages. Uses CSS theme tokens only.
import { icon } from './icons.js';
function themeNow() { return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'; }
function applyTheme(theme, persist=true) {
    const choice = theme === 'dark' ? 'dark' : 'light';
    document.documentElement.dataset.theme = choice;
    document.documentElement.style.colorScheme = choice;
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', choice === 'dark' ? '#0b1429' : '#f2f6fd');
    if (persist) { try { localStorage.setItem('asfwms-theme', choice); } catch {} }
    document.querySelectorAll('[data-theme-toggle]').forEach(button => {
        const next = choice === 'dark' ? 'light' : 'dark';
        button.innerHTML = icon(next === 'dark' ? 'moon' : 'sun') + `<span class="theme-toggle-text">${next === 'dark' ? 'Dark' : 'Light'} mode</span>`;
        button.setAttribute('aria-label', 'Switch to ' + next + ' mode');
        button.setAttribute('title', 'Switch to ' + next + ' mode');
        button.setAttribute('aria-pressed', String(choice === 'dark'));
    });
}
document.querySelectorAll('[data-theme-toggle]').forEach(button => {
    button.addEventListener('click', () => applyTheme(themeNow() === 'dark' ? 'light' : 'dark'));
});
applyTheme(themeNow(), false);
