// Read-only printable schedule view using the selected run or resource filters.
import { api } from './api.js';
import { load, record, label, fullLabel, rows } from './store.js';
import { e, clock, days, notice } from './components.js';
import { meetingRows } from './generation.js';
async function init() {
    const root = document.querySelector('#print-content');
    try {
        await load();
        const p = new URLSearchParams(location.search);
        let data = [], title = '', period = null, status = 'COMMITTED SCHEDULE', run = null;
        if (p.get('run')) {
            run = await api('/runs/' + Number(p.get('run')));
            data = run.result.meetings;
            title = label('departments', run.result.request.department_id) + ' / Generation #' + run.id;
            period = record('academic_periods', run.result.request.period_id);
            status = run.status === 'preview' ? 'DRAFT PREVIEW - NOT COMMITTED' : run.status === 'committed' ? 'COMMITTED SCHEDULE' : 'HISTORICAL ' + run.status.toUpperCase() + ' - NOT ACTIVE';
        }
        else {
            const periodID = Number(p.get('period')), department = Number(p.get('department')), view = ['section', 'faculty', 'room'].includes(p.get('view')) ? p.get('view') : 'section', resource = Number(p.get('resource'));
            data = rows('scheduled_meetings').filter(m => m.period_id === periodID && (!department || m.department_id === department) && (!resource || m[view + '_id'] === resource));
            period = record('academic_periods', periodID);
            title = resource ? fullLabel(view === 'section' ? 'sections' : view === 'faculty' ? 'faculty' : 'rooms', resource) : 'All ' + (view === 'faculty' ? 'instructors' : view + 's');
            if (department)
                title += ' / ' + label('departments', department);
        }
        root.innerHTML = `<header class="print-header"><img src="/assets/bcc-seal.png" alt="Buenavista Community College seal" class="print-seal"><p>Republic of the Philippines</p><h1>BUENAVISTA COMMUNITY COLLEGE</h1><p>Cangawa, Buenavista, Bohol</p><p>Academic Scheduling and Faculty Workload Management System</p></header><div class="print-status">${e(status)}</div><h2>${e(title)}</h2><div class="print-meta"><span>${e(period?.label || 'Academic period')} / ${e(period?.academic_year || '')}</span><span>${e(period?.start_date || '')} to ${e(period?.end_date || '')}</span><span>${data.length} weekly meetings</span></div><table class="print-table"><thead><tr><th>Day</th><th>Time</th><th>Code</th><th>Description</th><th>Units</th><th>Section</th><th>Room</th><th>Instructor</th><th>Room use</th></tr></thead><tbody>${meetingRows(data).map(m => `<tr><td>${e(days[m.day])}</td><td>${clock(m.start_minute)}-${clock(m.end_minute)}</td><td>${e(m.subject || record('subjects', m.subject_id)?.code)}</td><td>${e(m.title || record('subjects', m.subject_id)?.title)}</td><td>${e(record('subjects', m.subject_id)?.units ?? '')}</td><td>${e(m.section || label('sections', m.section_id))}</td><td>${e(m.room || label('rooms', m.room_id))}</td><td>${e((m.faculty && !/^Faculty \d+$/.test(m.faculty) ? m.faculty : label('faculty', m.faculty_id)))}</td><td>${m.borrowed ? 'Borrowed' : 'Own / shared'}</td></tr>`).join('')}</tbody></table>${!data.length ? '<p>No meetings match this view.</p>' : ''}${run?.result.issues?.length ? `<p class="small">Partial result: ${run.result.issues.length} unresolved requirements. See the generation record for details.</p>` : ''}<p class="small muted">Prepared locally on ${e(new Date().toLocaleString())}. Weekly pattern within the academic period; holidays and exam exceptions are not modeled. Historical labels come from the saved result where available; units and period metadata reflect current records.</p>`;
        document.querySelector('#print').disabled = false;
        document.querySelector('#print').onclick = () => window.print();
    }
    catch (err) {
        root.innerHTML = notice(err.message, 'danger', 'Unable to prepare schedule');
    }
}
init();
