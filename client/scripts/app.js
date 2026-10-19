// Workspace bootstrap, navigation, dashboard and local account actions.
import { api, downloadBlob } from './api.js';
import { state, rows, record, label, active, departmentOf, requiredSubjects, load, refresh, events, navigate, route } from './store.js';
import { e, number, pill, button, iconButton, notice, heading, empty, dialog, confirm, toast, setBusy, stat, skeleton, installTooltips } from './components.js';
import { icon } from './icons.js';
import { renderAcademic, renderFacilities, editRecord } from './records.js';
import { renderFaculty } from './faculty.js';
import {renderAccess} from './access-management.js';
import {renderDeveloper} from './developer-tools.js';
import { renderGeneration } from './generation.js';
import { renderSchedules, runTable } from './schedules.js';
const navigation = [['overview', 'Overview'], ['academic', 'Academic'], ['faculty', 'Faculty'], ['facilities', 'Facilities'], ['generation', 'Generation'], ['schedules', 'Schedules']];
const root = document.querySelector('#workspace');
function overview() {
    const period = rows('academic_periods').find(active);
    const sections = rows('sections').filter(s => active(s) && active(record('programs',s.program_id)));
    const requirements = sections.flatMap(s => requiredSubjects(s,period?.id));
    const assigned = requirements.filter(x => x.assignment).length;
    root.innerHTML = heading('Your scheduling workspace', 'A clear view of the people, programs, and spaces behind each department\'s weekly schedule.', button('Generate a schedule', { icon: 'generation', tone: 'primary', action: 'generate' }), 'PLAN WITH PURPOSE') + `<div class="stack"><div class="grid four">${stat('Active sections', number(sections.length), period?.label || 'No active academic period', 'academic')}${stat('Active faculty', number(rows('faculty').filter(active).length), 'Instructor and adviser roles stay separate', 'faculty')}${stat('Active rooms', number(rows('rooms').filter(active).length), 'Department-owned and shared spaces', 'facilities')}${stat('Committed meetings', number(rows('scheduled_meetings').length), 'Actual reservations across all periods', 'schedules')}</div><div class="hero"><span class="eyebrow">READY WHEN YOUR DEPARTMENT IS</span><h2>Plan independently. Share resources responsibly.</h2><p>Map subjects to each section\'s curriculum and year, assign its instructors, then generate. Keep valid partial results and review every unresolved requirement.</p><div class="row">${button('Review academic setup', { icon: 'arrow', action: 'academic' })}<span class="small">${assigned} of ${requirements.length} subject-section requirements have fixed instructor links; other eligible staff may be selected automatically.</span></div></div>${notice('Preload records come from the supplied PostgreSQL dump, including real faculty names and recorded working hours. Review source semester dates, curriculum gaps, provisional major/minor labels and room requirements before committing schedules. Production accounts and schedules are not imported.', 'warning', 'Source records / review needed')}<section><div class="section-heading"><div><h2>Explicit instructor coverage</h2><p>Fixed teaching links for ${e(period?.label || 'the active period')}. Unassigned subjects can still be staffed using eligible faculty; this is not a placement guarantee.</p></div><a class="small" href="#academic?tab=sections">Manage sections &#8594;</a></div><div class="grid four" id="department-cards"></div></section><section><div class="section-heading"><div><h2>Recent generation activity</h2><p>Previews do not occupy rooms until you commit them.</p></div><a class="small" href="#schedules?tab=history">All history &#8594;</a></div><div id="overview-history" class="panel flush"></div></section></div>`;
    const departments = rows('departments').filter(d => active(d) && rows('programs').some(p => active(p) && p.department_id === d.id));
    root.querySelector('#department-cards').innerHTML = departments.length ? departments.map(d => { const ss = sections.filter(s => departmentOf('sections', s) === d.id), req = ss.flatMap(s => requiredSubjects(s,period?.id)), count = req.filter(x => x.assignment).length, percent = req.length ? Math.round(count / req.length * 100) : 0; return `<article class="panel department-card"><div class="row between"><div><h3>${e(d.code)}</h3><p>${e(d.name)}</p></div><span class="department-monogram">${e(d.code.slice(0, 2))}</span></div><div><div class="metric-caption"><span>${count} / ${req.length} fixed links</span><strong>${percent}%</strong></div><progress class="meter" max="100" value="${percent}" aria-label="${e(d.code)} explicit instructor coverage ${percent}%"></progress></div><div class="department-metrics"><div><strong>${ss.length}</strong>Sections</div><div><strong>${rows('rooms').filter(r => active(r) && r.department_id === d.id).length}</strong>Owned rooms</div><div><strong>${rows('faculty').filter(f => active(f) && f.department_id === d.id).length}</strong>Faculty</div></div>${button('Plan ' + d.code, { small: true, icon: 'arrow', action: 'department-' + d.id })}</article>`; }).join('') : empty('Start your academic foundation', 'Create a department and program first.', button('Open Academic', { action: 'academic', tone: 'primary' }));
    runTable(root.querySelector('#overview-history'), state.runs.slice(0, 5));
    root.querySelectorAll('[data-action=academic]').forEach(b => b.onclick = () => navigate('academic'));
    root.querySelector('[data-action=generate]').onclick = () => navigate('generation');
    root.querySelectorAll('[data-action^="department-"]').forEach(b => b.onclick = () => navigate('generation', { department: b.dataset.action.slice(11), period: period?.id || '' }));
}
async function render() {
    if (!state.loaded)
        return;
    root.removeAttribute('data-scope-readonly');
    const { page, params } = route();
    const chosen = [...navigation,...(state.user.role!=='scheduler'?[['access','Access']]:[]),...(state.user.role==='developer'?[['developer','Developer tools']]:[])].find(x => x[0] === page) || navigation[0];
    document.querySelector('#breadcrumb').textContent = chosen[1];
    document.title = 'ASFWMS | ' + chosen[1];
    const role=state.user.role||'developer';
    const visibleNavigation=role==='developer'?[...navigation,['access','Access'],['developer','Developer tools']]:role==='administrator'?[...navigation,['access','Access']]:navigation;
    document.querySelector('#navigation').innerHTML = visibleNavigation.map(([name, text], i) => `<a href="#${name}" class="nav-link ${name === chosen[0] ? 'active' : ''}" ${name === chosen[0] ? 'aria-current="page"' : ''}>${icon(name)}<span>${e(text)}</span><span class="nav-index">0${i + 1}</span></a>`).join('');
    document.querySelector('#account-name').textContent = state.user.username;
    document.querySelector('#account .avatar').textContent = state.user.username?.slice(0, 1).toUpperCase() || 'A';
    document.querySelector('#revision-label').textContent = 'Data revision ' + state.revision;
    try {
        if (chosen[0] === 'overview')
            overview();
        if (chosen[0] === 'academic')
            renderAcademic(root, params);
        if (chosen[0] === 'faculty')
            renderFaculty(root, params);
        if (chosen[0] === 'facilities')
            renderFacilities(root, params);
        if (chosen[0] === 'generation')
            await renderGeneration(root, params);
        if (chosen[0] === 'schedules')
            renderSchedules(root, params);
        if(chosen[0]==='access') renderAccess(root,params);
        if(chosen[0]==='developer') renderDeveloper(root,params);
    }
    catch (err) {
        root.innerHTML = notice(err.message, 'danger', 'This view could not be displayed');
        console.error(err);
    }
}
async function init() { try {
    const status = await api('/status', { redirect: false });
    if (status.setup_required) {
        location.replace('/access');
        return;
    }
    await load();
    document.querySelector('#startup').hidden = true;
    document.querySelector('#shell').hidden = false;
    render();
}
catch (err) {
    if (err.status === 401)
        return;
    document.querySelector('#startup').innerHTML = `<img src="/assets/bcc-seal.png" width="56" height="56" alt=""><h1>ASFWMS</h1><p>${e(err.message)}</p>${button('Try again', { tone: 'primary', action: 'retry' })}`;
    document.querySelector('[data-action=retry]').onclick = init;
} }
async function account() {
    const d = dialog({ title: 'Account & workspace', description: 'Signed in as ' + state.user.username + ' ('+(state.user.role||'developer')+'). Your access and department scope are enforced by the server.', body: `<div class="stack">${notice('A backup contains your local records, password hashes and sessions. Keep it private. Use the backup button while running, or stop the server before copying database.db.', 'info')}<div class="row">${state.user.role==='developer'?button('Download SQLite backup', { icon: 'database', action: 'backup', tone: 'secondary' }):''}${button('Change password', { icon: 'lock', action: 'password' })}${button('Request password reset',{icon:'lock',action:'request-reset'})}</div><div class="panel"><h3>Reference dataset</h3><p class="muted small">${e(state.dataset || 'Custom / empty workspace')}</p><p class="muted small">Preload from database/seed.sql is explicit and refuses to run when business records already exist. It never resets your data.</p>${state.user.role==='developer'?button('Load source records', { icon: 'layers', action: 'seed', small: true, disabled: rows('departments').length > 0 }):''}</div><a href="/guide" target="_blank" rel="noopener">Read delivery notes, database mapping, and known limitations &#8599;</a></div>`, actions: [{ label: 'Close', run: m => m.close() }, { label: 'Sign out', icon: 'logout', run: async (m) => { try {
                    await api('/logout', { method: 'POST', body: {} });
                    location.assign('/access');
                }
                catch (err) {
                    toast(err.message, 'danger');
                } } }] });
    d.body.querySelector('[data-action=request-reset]').onclick=async()=>{try{await api('/account/reset-request',{method:'POST',body:{message:'Requested from Account settings.'}});toast('Reset request logged. Contact the developer or administrator for a recovery email.');}catch(err){toast(err.message,'danger')}};
    const backupAction=d.body.querySelector('[data-action=backup]');
    if(backupAction) backupAction.onclick = async (ev) => { const b = ev.currentTarget; setBusy(b, true, 'Preparing backup...'); try {
        const result = await api('/backup', { method: 'POST', body: {}, blob: true });
        downloadBlob(result.blob, result.filename);
        toast('SQLite backup prepared. Keep it private.');
    }
    catch (err) {
        toast(err.message, 'danger');
    }
    finally {
        setBusy(b, false);
    } };
    const seedAction=d.body.querySelector('[data-action=seed]');
    if(seedAction) seedAction.onclick = () => confirm({ title: 'Load real source records?', message: 'This reads database/seed.sql and inserts source academic, faculty and facility records into an empty workspace. It refuses to overwrite existing business data. The seed contains real faculty names; keep it private.', word: 'LOAD REFERENCE', label: 'Load source records', tone: 'primary', action: async () => { await api('/seed', { method: 'POST', body: { confirm: 'LOAD REFERENCE' } }); await refresh(); d.close(); toast('Source records loaded.'); } });
    d.body.querySelector('[data-action=password]').onclick = () => { const m = dialog({ title: 'Change local password', description: 'All sessions will be signed out after the password is changed.', body: '<form class="stack" id="password-form"><div class="field"><label for="password-current">Current password</label><input type="password" id="password-current" required autocomplete="current-password"></div><div class="field"><label for="password-new">New password (at least 12 characters)</label><input type="password" id="password-new" minlength="12" maxlength="128" required autocomplete="new-password"></div><div class="field"><label for="password-confirm">Confirm new password</label><input type="password" id="password-confirm" required autocomplete="new-password"></div><p class="form-error" role="alert" hidden></p></form>', actions: [{ label: 'Cancel', run: x => x.close() }, { label: 'Change password', tone: 'primary', id: 'save', run: save }] }); const form = m.body.querySelector('form'); form.onsubmit = ev => { ev.preventDefault(); save(m, m.controls.save); }; async function save(x, b) { if (!form.reportValidity())
        return; const old = form.querySelector('#password-current').value, pw = form.querySelector('#password-new').value, repeat = form.querySelector('#password-confirm').value, error = form.querySelector('.form-error'); if (pw !== repeat) {
        error.textContent = 'The new passwords do not match.';
        error.hidden = false;
        return;
    } x.setBusy(true); setBusy(b, true, 'Changing password...'); try {
        await api('/account/password', { method: 'POST', body: { current: old, new: pw } });
        location.assign('/access');
    }
    catch (err) {
        x.setBusy(false);
        setBusy(b, false);
        error.textContent = err.message;
        error.hidden = false;
    } } };
}
const toggle = document.querySelector('#menu-toggle'), sidebar = document.querySelector('#sidebar'), scrim = document.querySelector('#sidebar-scrim');
let menuOpen = false;
toggle.innerHTML = icon('menu');
function menu(open) { menuOpen = open; sidebar.classList.toggle('open', open); scrim.hidden = !open; toggle.setAttribute('aria-expanded', String(open)); sidebar.inert = window.innerWidth <= 850 && !open; }
toggle.onclick = () => menu(!menuOpen);
scrim.onclick = () => menu(false);
window.addEventListener('resize', () => menu(window.innerWidth <= 850 ? menuOpen : false));
document.addEventListener('keydown', ev => { if (ev.key === 'Escape' && menuOpen) {
    menu(false);
    toggle.focus();
} });
menu(false);
document.querySelector('#refresh').innerHTML = icon('refresh');
document.querySelector('#refresh').onclick = async (ev) => { const b = ev.currentTarget; setBusy(b, true, ''); try {
    await refresh();
    toast('Workspace refreshed.');
}
catch (err) {
    toast(err.message, 'danger');
}
finally {
    setBusy(b, false);
} };
document.querySelector('#account').onclick = account;
window.addEventListener('hashchange', () => { menu(false); render(); root.focus({ preventScroll: true }); window.scrollTo({ top: 0 }); });
// A data save updates SPA records without a browser reload. Restore scroll
// position after the current route is re-rendered, retaining filter context.
events.addEventListener('refresh', () => {
    const x=window.scrollX, y=window.scrollY, hash=location.hash;
    Promise.resolve(render()).finally(() => {
        if(location.hash===hash)requestAnimationFrame(()=>window.scrollTo(x,y));
    });
});
installTooltips();
init();
