// First-use account setup and existing-account sign-in.
import { api, setCSRF } from './api.js';
import { setBusy } from './components.js';
const form = document.querySelector('#access-form'), err = document.querySelector('#access-error');
let setup = false;
const resetToken = new URLSearchParams(location.search).get('reset');
const resetSuccess = new URLSearchParams(location.search).has('recovered');
async function init() { try {
    if(resetToken){
        if(!/^[a-fA-F0-9]{64}$/.test(resetToken))throw Error('This recovery link is invalid. Request another reset link from your administrator.');
        document.querySelector('#access-title').textContent='Set a new password';
        document.querySelector('#access-description').textContent='Choose a new password of at least 12 characters. This one-time recovery link expires after 30 minutes.';
        document.querySelector('#access-submit').textContent='Reset password';
        document.querySelector('#username').required=false;
        document.querySelector('#username').closest('.field').hidden=true;
        document.querySelector('label[for=password]').textContent='New password';
        document.querySelector('#password').autocomplete='new-password';
        document.querySelector('#password').minLength=12;
        document.querySelector('#repeat-field').hidden=false;
        document.querySelector('#repeat').required=true;
        form.hidden=false;
        document.querySelector('#access-loading').hidden=true;
        return;
    }
    const status = await api('/status', { redirect: false });
    setup = status.setup_required;
    document.querySelector('#access-title').textContent = setup ? 'Create your workspace account' : 'Welcome back.';
    document.querySelector('#access-description').textContent = setup ? 'Set up the local administrator. Choose a unique password of at least 12 characters.' : 'Sign in to plan, generate, and review department schedules.';
    document.querySelector('#access-submit').textContent = setup ? 'Create account & open workspace' : 'Open workspace';
    document.querySelector('#repeat-field').hidden = !setup;
    document.querySelector('#repeat').required = setup;
    document.querySelector('#password').autocomplete = setup ? 'new-password' : 'current-password';
    document.querySelector('#password').minLength = setup ? 12 : 1;
    form.hidden = false;
    document.querySelector('#access-loading').hidden = true;
    if(resetSuccess){document.querySelector('#access-description').textContent='Password updated. Sign in with your new password.';}
}
catch (e) {
    document.querySelector('#access-description').textContent = e.message;
    document.querySelector('#access-loading').hidden = true;
} }
form.onsubmit = async (ev) => { ev.preventDefault(); err.hidden = true; const password = document.querySelector('#password').value; if (setup && password !== document.querySelector('#repeat').value) {
    err.textContent = 'The passwords do not match.';
    err.hidden = false;
    return;
} const b = document.querySelector('#access-submit'); setBusy(b, true, resetToken ? 'Updating password...' : setup ? 'Creating account...' : 'Signing in...'); try {
    if(resetToken){await api('/reset/confirm',{method:'POST',body:{token:resetToken,password},redirect:false});location.replace('/access?recovered=1');return;}
    const user = await api(setup ? '/setup' : '/login', { method: 'POST', body: { username: document.querySelector('#username').value.trim(), password }, redirect: false });
    setCSRF(user.csrf_token);
    location.assign('/');
}
catch (e) {
    err.textContent = e.message;
    err.hidden = false;
    setBusy(b, false);
} };
init();
