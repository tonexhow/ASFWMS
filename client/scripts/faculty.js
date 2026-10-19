// Separate subject eligibility, teaching section assignments and adviser views.
import { api } from './api.js';
import { state, rows, record, label, options, active, departmentOf, requiredSubjects, ui, yearLabel, rememberContext, refresh, navigate, canEdit, ownDepartment } from './store.js';
import { e, clock, parseClock, days, hours, pill, button, iconButton, notice, heading, empty, dialog, confirm, dropdown, table, tabs, toast, setBusy } from './components.js';
import { icon } from './icons.js';
import { editRecord, deleteRecord } from './records.js';
import { pickRecords, facultyPrograms, programOptions, yearOptions } from './pickers.js';
export function renderFaculty(root,params){
    const targetSubject=Number(params.get('subject'));const filters=ui.faculty;
    root.innerHTML=heading('People behind every class','Click Subjects, Sections, Availability, Consideration or Status to manage that part directly.',button('Add faculty',{icon:'plus',tone:'primary',action:'add'}),'FACULTY & TEACHING')+(targetSubject?notice('Choose an eligible instructor for '+label('subjects',targetSubject)+'. Explicit section assignments remain separate from advisers.'):'')+`<div class="panel flush"><div class="toolbar record-filters"><div class="search">${icon('search')}<input id="faculty-search" type="search" placeholder="Search faculty by name or ID..." aria-label="Search faculty"></div><div id="faculty-department"></div><div id="faculty-program"></div><label class="check"><input type="checkbox" id="faculty-inactive"> Include inactive</label></div><div id="faculty-table"></div></div>`;
    root.querySelector('[data-action=add]').onclick=()=>{rememberContext({department:dd.value});editRecord('faculty');};
    const dd=dropdown({options:options('departments',{all:true,empty:'All departments'}),value:filters.department||'',label:'Filter faculty department',onChange:()=>{pd.setOptions(programOptions(dd.value,true));pd.value='';filters.page=1;saveFilters();render();}});
    const pd=dropdown({options:programOptions(filters.department||'',true),value:filters.program||'',label:'Filter faculty program',onChange:()=>{filters.page=1;saveFilters();render();}});root.querySelector('#faculty-department').append(dd.el);root.querySelector('#faculty-program').append(pd.el);
    const search=root.querySelector('#faculty-search'),inactive=root.querySelector('#faculty-inactive');search.value=filters.search||'';inactive.checked=!!filters.inactive;
    function saveFilters(){Object.assign(filters,{department:dd.value,program:pd.value,search:search.value,inactive:inactive.checked});rememberContext({department:dd.value,program:pd.value});}
    search.oninput=()=>{filters.page=1;saveFilters();render();};inactive.onchange=()=>{filters.page=1;saveFilters();render();};
    const cell=(action,content,name)=>`<button class="cell-link" type="button" data-action="${action}" aria-label="${e(name)}">${content}</button>`;
    function render(){const term=root.querySelector('#faculty-search').value.toLocaleLowerCase();const data=rows('faculty').filter(f=>{
        const programs=facultyPrograms(f.id);return(root.querySelector('#faculty-inactive').checked||active(f))&&(!dd.value||f.department_id===Number(dd.value)||!f.department_id||record('departments',f.department_id)?.code?.toUpperCase()==='GENED')&&(!pd.value||(pd.value==='unassigned'?!programs.length:programs.includes(Number(pd.value))))&&(f.name+' '+(f.faculty_id_number||'')).toLocaleLowerCase().includes(term)&&(!targetSubject||rows('faculty_subjects').some(x=>x.faculty_id===f.id&&x.subject_id===targetSubject));
    });table(root.querySelector('#faculty-table'),{initialPage:filters.page||1,onPageChange:p=>{filters.page=p;},columns:[
        {title:'Faculty member',render:f=>`<div class="row"><div><span class="main-cell">${e(f.name)}</span><span class="sub-cell">${e([f.faculty_id_number?'ID: '+f.faculty_id_number:'',f.employment||'Employment not specified'].filter(Boolean).join(' · '))}</span>${/^NEW (VALUES )?INSTRUCTOR/.test(f.name)?pill('Source placeholder / review name','warning'):''}</div></div>`},
        {title:'Department',render:f=>pill(f.department_id?label('departments',f.department_id):'Unassigned','subtle')},
        {title:'Subjects',render:f=>cell('subjects',`<strong class="main-cell">${rows('faculty_subjects').filter(x=>x.faculty_id===f.id).length} eligible</strong><span class="sub-cell">Manage subjects</span>`,'Manage subjects for '+f.name)},
        {title:'Sections',render:f=>{const explicit=rows('teaching_assignments').filter(x=>x.faculty_id===f.id);const committed=rows('scheduled_meetings').filter(x=>x.faculty_id===f.id);const preview=(state.preview_assignments||[]).filter(x=>x.faculty_id===f.id);return cell('sections',`<strong class="main-cell">${new Set([...explicit,...committed,...preview].map(x=>x.section_id)).size} sections</strong><span class="sub-cell">${f.section_policy==='exclusive'?'Strict':'Priority'} &middot; ${explicit.length} explicit · ${new Set(committed.map(x=>x.section_id+'-'+x.subject_id)).size} scheduled${preview.length?' · '+preview.length+' preview':''}</span>`,'Manage sections for '+f.name);}},
        {title:'Working hours / facility',render:f=>`<div class="stack compact">${cell('availability',availabilityLabel(f),'Edit working hours for '+f.name)}${button('Facility',{small:true,action:'preferences'})}</div>`},
        {title:'Consideration',render:f=>cell('priority',pill(f.priority_scheduling?'Priority':'Standard',f.priority_scheduling?'warning':'neutral'),'Change consideration for '+f.name)},
        {title:'Status',render:f=>cell('status',pill(f.active?'Active':'Inactive',f.active?'':'neutral'),'Change status for '+f.name)},
        {title:'Actions',render:f=>`<div class="cell-actions">${iconButton('edit','Edit faculty','edit')}${iconButton('trash','Delete faculty','delete','danger')}</div>`}
    ],rows:data,emptyTitle:'No matching faculty',emptyText:targetSubject?'Add this subject to faculty eligibility first, or clear the subject filter.':'Change the filters or add a faculty member.',emptyAction:targetSubject?button('Show all faculty',{action:'clear',small:true}):'',onAction:(a,f)=>{
        if(a==='clear')navigate('faculty');if(a==='subjects')facultySubjects(f.id);if(a==='sections')targetSubject?assignSections(f.id,targetSubject,()=>{},Number(params.get('section'))):facultySections(f.id);if(a==='availability')availability(f.id);if(a==='preferences')preferredScheduling(f.id);if(a==='edit')editRecord('faculty',f);if(a==='delete')deleteRecord('faculty',f);if(a==='priority')toggleFaculty(f,'priority_scheduling');if(a==='status')toggleFaculty(f,'active');
    }});}render();
    if(params.has('profile')){const id=Number(params.get('profile'));queueMicrotask(()=>{history.replaceState(null,'','#faculty');facultyProfile(id);});}
}
function availabilityLabel(f){
    const work=rows('faculty_work_times').filter(w=>w.faculty_id===f.id).sort((a,b)=>a.day-b.day);
    if(!work.length)return pill('Mon-Fri','neutral')+'<span class="sub-cell">07:00-19:00</span>';
    const same=work.every(w=>w.start_minute===work[0].start_minute&&w.end_minute===work[0].end_minute);
    const weekdays=work.length===5&&work.every((w,i)=>w.day===i+1);
    return pill(weekdays?'Mon-Fri':work.map(w=>days[w.day].slice(0,3)).join(', '),'neutral')+`<span class="sub-cell">${same?clock(work[0].start_minute)+'-'+clock(work[0].end_minute):'Different hours / day'}</span>`;
}
export function facultySubjects(id,after=()=>{}){
    const f=record('faculty',id);if(!f)return;const revision=state.revision;const readOnly=!canEdit('faculty',f);
    const selected=rows('faculty_subjects').filter(x=>x.faculty_id===id).map(x=>x.subject_id),locked=rows('teaching_assignments').filter(x=>x.faculty_id===id).map(x=>x.subject_id);
    pickRecords('subjects',{title:'Subjects / '+f.name,multiple:true,selected,locked,memoryKey:'faculty-subjects',readOnly,confirmLabel:'Save eligible subjects',description:'Choose subjects this faculty member can teach. Subjects in existing teaching assignments remain selected until those assignments are removed.',onChoose:async ids=>{await api('/faculty/'+id+'/subjects',{method:'POST',body:{subject_ids:ids},revision});await refresh();toast('Eligible subjects saved.');after();}});
}
// Explicit section selections take priority during generation; other eligible
// section-subject pairs can be assigned automatically without altering this list.
// Draft all subject-section choices in one parent dialog. Each subject opens
// a curriculum-linked section chooser; only the parent Save persists changes.
export function facultySections(id, initialSubjectID=0, after=()=>{}, preselect=0) {
    const faculty=record('faculty',id);if(!faculty)return;
    const editable=canEdit('faculty',faculty),revision=state.revision;
    const subjects=rows('faculty_subjects').filter(x=>x.faculty_id===id).map(x=>record('subjects',x.subject_id)).filter(Boolean).sort((a,b)=>a.code.localeCompare(b.code));
    const draft=new Map(subjects.map(s=>[s.id,new Set(rows('teaching_assignments').filter(a=>a.faculty_id===id&&a.subject_id===s.id).map(a=>a.section_id))]));
    if(preselect&&draft.has(initialSubjectID))draft.get(initialSubjectID).add(Number(preselect));
    const m=dialog({title:'Sections / '+faculty.name,description:editable?'Choose a policy, then select sections for each eligible subject. Nothing changes until you save.':'Read-only faculty from another department. Your section assignments can be managed from Academic / Sections.',wide:true,
      body:`<div class="stack assignment-editor"><fieldset class="assignment-policy compact-policy"><legend>1. Assignment handling</legend><div class="policy-grid">${[['priority','Priority','Selected sections first; other eligible sections allowed.'],['exclusive','Strict','Only these selected sections.']].map(([v,t,h])=>`<label class="policy-choice"><input type="radio" name="section-policy-${id}" value="${v}" ${faculty.section_policy===v||(v==='priority'&&!faculty.section_policy)?'checked':''} ${!editable?'disabled':''}><span><strong>${t}</strong><small>${h}</small></span></label>`).join('')}</div></fieldset><div class="row between"><h3>2. Subjects and sections</h3>${editable?button('Eligible subjects',{small:true,action:'eligibility'}):''}</div><div class="assignment-subjects"></div><p class="form-error" hidden role="alert"></p></div>`,
      actions:[{label:editable?'Cancel':'Close',run:d=>d.close()},...(editable?[{label:'Save sections',tone:'primary',icon:'check',id:'save',run:save}]:[])]});
    m.body.querySelector('[data-action=eligibility]')?.addEventListener('click',()=>{confirm({title:'Manage eligible subjects?',message:'Unsaved section changes in this dialog will be discarded. Saved assignments remain unchanged.',label:'Continue',action:()=>{m.close();facultySubjects(id,()=>facultySections(id));}});});
    function draw(){const host=m.body.querySelector('.assignment-subjects');host.innerHTML=subjects.length?subjects.map(s=>{const ids=[...(draft.get(s.id)||[])];return `<div class="assignment-subject-row"><div class="grow"><strong>${e(s.code)}</strong><span class="sub-cell">${e(s.title)}</span><small class="muted">${ids.length?ids.map(n=>{const sc=record('sections',n);return e(sc?label('programs',sc.program_id)+' / '+yearLabel(sc.year_level)+' / '+sc.code:'Section '+n);}).join(', '):'No sections selected'}</small></div>${button((editable?'Select':'View')+' sections'+(ids.length?' ('+ids.length+')':''),{small:true,action:'sections-'+s.id})}</div>`;}).join(''):notice('Add eligible subjects first. Only curriculum-linked sections appear.');for(const s of subjects)host.querySelector('[data-action=sections-'+s.id+']').onclick=()=>chooseSections(s);}
    function chooseSections(subject){const selected=new Set(draft.get(subject.id)||[]);const available=rows('sections').filter(s=>(active(s)||selected.has(s.id))&&requiredSubjects(s).some(x=>x.subject?.id===subject.id)).sort((a,b)=>a.program_id-b.program_id||a.year_level-b.year_level||a.code.localeCompare(b.code));
      const d=dialog({title:subject.code+' / Select sections',description:'Sections are connected by program, year level and curriculum. Your selection is kept as a draft until the parent Save.',wide:true,body:`<div class="search">${icon('search')}<input type="search" placeholder="Search program, year or section" aria-label="Search connected sections"></div><div class="section-choice-list"></div>`,actions:[{label:'Cancel',run:x=>x.close()},...(editable?[{label:'Use selected sections',tone:'primary',run:x=>{draft.set(subject.id,selected);draw();x.close();}}]:[])]});
      const search=d.body.querySelector('input[type=search]');function paint(){const list=d.body.querySelector('.section-choice-list');const term=search.value.toLocaleLowerCase();list.innerHTML=available.filter(s=>(label('programs',s.program_id)+' '+yearLabel(s.year_level)+' '+s.code).toLocaleLowerCase().includes(term)).map(s=>{const assignment=rows('teaching_assignments').find(a=>a.section_id===s.id&&a.subject_id===subject.id&&a.faculty_id!==id),locked=!editable||!canEdit('sections',s)||!!assignment;return `<label class="section-choice"><input type="checkbox" value="${s.id}" ${selected.has(s.id)?'checked':''} ${locked?'disabled':''}><span><strong>${e(s.code)}</strong><small>${e(label('programs',s.program_id))} / ${e(yearLabel(s.year_level))}${assignment?' / Assigned to '+e(label('faculty',assignment.faculty_id)):!canEdit('sections',s)?' / Other department (read-only)':''}</small></span></label>`;}).join('')||notice('No connected sections match. Check the curriculum links and section year.');for(const box of list.querySelectorAll('input'))box.onchange=()=>box.checked?selected.add(Number(box.value)):selected.delete(Number(box.value));}search.oninput=paint;paint();}
    async function save(d,b){const policy=d.body.querySelector('input[type=radio]:checked')?.value||'priority';const error=d.body.querySelector('.form-error');d.setBusy(true);setBusy(b,true,'Saving...');try{await api('/faculty/'+id+'/sections',{method:'POST',revision,body:{policy,subjects:subjects.map(s=>({subject_id:s.id,section_ids:[...(draft.get(s.id)||[])]}))}});await refresh();d.setBusy(false);d.close();toast('Section policy and selections saved.');after();}catch(err){d.setBusy(false);setBusy(b,false);error.textContent=err.message;error.hidden=false;}}
    draw();
}

async function toggleFaculty(f,key){
    if(!canEdit('faculty',f)){toast('This faculty record is read-only.','warning');return;}
    const revision=state.revision,isPriority=key==='priority_scheduling',enable=!f[key];
    await confirm({title:isPriority?(enable?'Give scheduling consideration?':'Use standard priority?'):(enable?'Activate faculty?':'Deactivate faculty?'),message:isPriority?`${f.name}: ${enable?'prioritize this faculty within the major/minor phase. Majors are always handled first. Availability and conflict checks stay enforced.':'remove the early-placement preference. Working hours will stay unchanged.'}`:`${f.name}: ${enable?'make available for teaching assignments.':'prevent new placements. Existing committed schedules must be resolved before deactivation.'}`,label:'Save change',action:async()=>{await api('/records/faculty/'+f.id,{method:'PUT',body:{[key]:enable?1:0},revision});await refresh();toast(isPriority?'Consideration updated.':'Faculty status updated.');}});
}
export function facultyProfile(id) {
    if (!record('faculty', id))
        return;
    let tab = 'teaching';
    const d = dialog({ title: record('faculty', id).name, description: 'Faculty -> Subject -> Sections. Adviser appointments do not create teaching assignments.', wide: true, body: '', actions: [{ label: 'Close', run: m => m.close() }, { label: 'Facility', icon: 'rooms', run: () => preferredScheduling(id, render) }, { label: 'Availability', icon: 'clock', run: () => availability(id, render) }, { label: 'Edit profile', tone: 'primary', icon: 'edit', run: () => editRecord('faculty', record('faculty', id), {}, render) }] });
    function render() {
        const f = record('faculty', id);
        if (!f)
            return;
        const eligible = rows('faculty_subjects').filter(x => x.faculty_id === id), teaching = rows('teaching_assignments').filter(x => x.faculty_id === id);
        d.body.innerHTML = `<div class="detail-grid"><div><small>Department</small><strong>${e(f.department_id ? label('departments', f.department_id) : 'Shared')}</strong></div><div><small>Teaching workload</small><strong>${teaching.length} subject-section assignments</strong></div><div><small>Weekly teaching cap</small><strong>${f.max_weekly_minutes ? e(hours(f.max_weekly_minutes)) : 'Not capped'}</strong></div></div><div class="tabs" id="profile-tabs"></div><div id="profile-content"></div>`;
        tabs(d.body.querySelector('#profile-tabs'), [{ value: 'teaching', label: 'Subjects & sections', count: eligible.length }, { value: 'blocks', label: 'Unavailable windows', count: rows('faculty_considerations').filter(x => x.faculty_id === id).length }, { value: 'advising', label: 'Advisory sections', count: rows('sections').filter(s => s.adviser_id === id).length }], tab, v => { tab = v; render(); });
        const content = d.body.querySelector('#profile-content');
        if (tab === 'teaching') {
            content.innerHTML = `<div class="row between"><p class="muted small">Subject eligibility alone does not assign a class.</p>${button('Add eligible subject', { small: true, icon: 'plus', action: 'eligible' })}</div><div class="panel flush" id="eligible-table"></div><h3>Current teaching assignments</h3><div class="panel flush" id="teaching-table"></div>`;
            content.querySelector('[data-action=eligible]').onclick = () => facultySubjects(id, render);
            table(content.querySelector('#eligible-table'), { columns: [{ title: 'Eligible subject', render: x => `<span class="main-cell">${e(record('subjects', x.subject_id)?.code)}</span><span class="sub-cell">${e(record('subjects', x.subject_id)?.title)}</span>` }, { title: 'Handled sections', render: x => teaching.filter(a => a.subject_id === x.subject_id).length }, { title: 'Actions', render: x => `<div class="cell-actions">${button('Select sections', { small: true, icon: 'link', action: 'sections' })}${iconButton('link', 'Unlink eligible subject', 'remove')}</div>` }], rows: eligible, emptyTitle: 'No eligible subjects', emptyText: 'Add a subject first, then select which sections this faculty member teaches.', onAction: (a, x) => a === 'sections' ? assignSections(id, x.subject_id, render) : deleteRecord('faculty_subjects', x, render) });
            table(content.querySelector('#teaching-table'), { columns: [{ title: 'Subject', render: a => e(record('subjects', a.subject_id)?.code) }, { title: 'Section', render: a => e(label('sections', a.section_id)) }, { title: 'Coverage', render: a => e('Reusable section bank') }, { title: 'Actions', render: a => `<div class="cell-actions">${iconButton('link', 'Unlink teaching assignment', 'remove')}</div>` }], rows: teaching, emptyTitle: 'No sections assigned', emptyText: 'Choose Select sections beside an eligible subject.', onAction: (a, x) => deleteRecord('teaching_assignments', x, render) });
        }
        else if (tab === 'blocks') {
            content.innerHTML = `${notice('These time windows are hard restrictions, applied each week. Profile notes are informational and are not interpreted as restrictions.')}<div class="row between"><p class="muted small">Leave this empty when there are no additional restrictions.</p>${button('Add unavailable window', { small: true, icon: 'plus', action: 'block' })}</div><div class="panel flush" id="block-table"></div>${f.consideration ? `<br>${notice(f.consideration, 'info', 'Profile notes (informational)')}` : ''}`;
            content.querySelector('[data-action=block]').onclick = () => editRecord('faculty_considerations', null, { faculty_id: id }, render);
            table(content.querySelector('#block-table'), { columns: [{ title: 'Day', render: b => e(days[b.day]) }, { title: 'Unavailable', render: b => clock(b.start_minute) + ' - ' + clock(b.end_minute) }, { title: 'Reason', render: b => e(b.note || 'Not specified') }, { title: 'Actions', render: b => `<div class="cell-actions">${iconButton('edit', 'Edit window', 'edit')}${iconButton('trash', 'Remove window', 'delete')}</div>` }], rows: rows('faculty_considerations').filter(x => x.faculty_id === id), emptyTitle: 'No unavailable windows', emptyText: 'Scheduling follows this faculty member\'s working days and hours.', onAction: (a, b) => a === 'edit' ? editRecord('faculty_considerations', b, {}, render) : deleteRecord('faculty_considerations', b, render) });
        }
        else {
            content.innerHTML = notice('These are adviser relationships only. They do not add classes, contact minutes, or room reservations.') + '<div id="adviser-table" class="panel flush"></div>';
            table(content.querySelector('#adviser-table'), { columns: [{ title: 'Advisory section', render: s => e(s.code) }, { title: 'Year level', render: s => e(yearLabel(s.year_level)) }, { title: 'Section bank', render: s => e('Every term (curriculum resolved at generation)') }, { title: 'Actions', render: s => button('Edit adviser', { small: true, action: 'edit' }) }], rows: rows('sections').filter(s => s.adviser_id === id), emptyTitle: 'No advisory sections', emptyText: 'Set an adviser from Academic -> Sections. Teaching the section is not required.', onAction: (a, s) => editRecord('sections', s, {}, render) });
        }
    }
    render();
}
export function assignSections(facultyID,subjectID,after=()=>{},preselect=0){
    return facultySections(facultyID,subjectID,after,preselect);
}
export function availability(id, after = () => { }) {
    const f = record('faculty', id);
    if (!f)
        return;
    if(!canEdit('faculty',f)){toast('Working hours for another department are read-only.','warning');return;}
    const existing = rows('faculty_work_times').filter(x => x.faculty_id === id);
    const revision = state.revision;
    const d = dialog({ title: 'Working days & hours', description: f.name + ' / Default: Monday-Friday, 07:00-19:00.', body: `${notice('Custom availability replaces the default working week. Sunday or Saturday classes require matching subject days and faculty working hours. Unavailable windows apply within these working hours.')}<form id="availability-form">${[7,1,2,3,4,5,6].map(day => { const name = days[day], w = existing.find(x => x.day === day), enabled = existing.length ? !!w : day <= 5; return `<div class="day-row" data-day="${day}"><label class="check"><input type="checkbox" ${enabled ? 'checked' : ''}>${e(name.slice(0, 3))}</label><input type="time" aria-label="${e(name)} starts" value="${clock(w?.start_minute ?? 420)}" step="900"><span class="muted">-</span><input type="time" aria-label="${e(name)} ends" value="${clock(w?.end_minute ?? 1140)}" step="900"></div>`; }).join('')}</form><p class="form-error" role="alert" hidden></p>`, actions: [{ label: 'Facility', icon: 'rooms', run: () => preferredScheduling(id, after) }, { label: 'Use M-F default', run: (m, b) => save(m, b, true) }, { label: 'Cancel', run: m => m.close() }, { label: 'Save custom hours', tone: 'primary', icon: 'check', run: (m, b) => save(m, b, false) }] });
    const form = d.body.querySelector('form');
    function update() { for (const row of form.querySelectorAll('.day-row')) {
        const checkbox = row.querySelector('input[type=checkbox]');
        row.querySelectorAll('input[type=time]').forEach(i => i.disabled = !checkbox.checked);
    } }
    form.onchange = update;
    form.onsubmit = ev => ev.preventDefault();
    update();
    async function save(m, b, useDefault) { if (!useDefault && !form.reportValidity())
        return; const days = [...form.querySelectorAll('.day-row')].filter(row => row.querySelector('input[type=checkbox]').checked).map(row => { const times = row.querySelectorAll('input[type=time]'); return { day: Number(row.dataset.day), start_minute: parseClock(times[0].value), end_minute: parseClock(times[1].value) }; }); m.setBusy(true); setBusy(b, true, 'Saving...'); try {
        await api('/faculty/' + id + '/work-times', { method: 'POST', body: { use_default: useDefault, days: useDefault ? [] : days }, revision });
        await refresh();
        m.setBusy(false);
        m.close();
        toast('Faculty working hours saved.');
        after();
    }
    catch (err) {
        m.setBusy(false);
        setBusy(b, false);
        const p = m.body.querySelector('.form-error');
        p.textContent = err.message;
        p.hidden = false;
    } }
}

// A facility is a soft preference, never permission to borrow or ignore conflicts.
export function preferredScheduling(id,after=()=>{}){
    const f=record('faculty',id);if(!f)return;const editable=canEdit('faculty',f),revision=state.revision;
    const saved=rows('faculty_preferences').find(p=>p.faculty_id===id)||{};
    const m=dialog({title:'Facility / '+f.name,description:'Working time comes only from the weekly availability settings.',body:`<div class="stack"><div class="field"><label>Preferred facility (optional)</label><div id="preferred-facility"></div><small class="help">A suitable, available facility is preferred within its ownership tier. Borrowing still requires permission.</small></div><p class="form-error" role="alert" hidden></p></div>`,actions:[{label:editable?'Cancel':'Close',run:d=>d.close()},...(editable?[{label:'Save facility',tone:'primary',icon:'check',run:save}]:[])]});
    const dd=dropdown({options:[{value:'',label:'No preferred facility'},...rows('rooms').filter(r=>active(r)&&active(record('buildings',r.building_id))).sort((a,b)=>(b.department_id===f.department_id)-(a.department_id===f.department_id)||a.code.localeCompare(b.code)).map(r=>({value:String(r.id),label:label('buildings',r.building_id)+' / '+r.code+' / '+r.kind}))],value:String(saved.preferred_room_id||''),label:'Preferred facility',disabled:!editable});m.body.querySelector('#preferred-facility').append(dd.el);
    async function save(d,b){d.setBusy(true);setBusy(b,true,'Saving...');try{await api('/faculty/'+id+'/preferences',{method:'POST',revision,body:{preferred_room_id:Number(dd.value)||0}});await refresh();d.setBusy(false);d.close();toast('Facility preference saved.');after();}catch(err){d.setBusy(false);setBusy(b,false);const p=d.body.querySelector('.form-error');p.textContent=err.message;p.hidden=false;}}
}
