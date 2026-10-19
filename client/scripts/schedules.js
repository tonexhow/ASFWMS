// Committed weekly/list views, filters and explicit run-release history.
import { api } from './api.js';
import { yearLabel, state, rows, record, label, fullLabel, options, active, ui, rememberContext, refresh, navigate } from './store.js';
import { e, clock, days, hours, pill, button, notice, heading, empty, confirm, dropdown, table, tabs, dialog, toast } from './components.js';
import { meetingRows, meetingColumns, exportMeetings } from './generation.js';
import { icon } from './icons.js';
import { setBusy } from './components.js';
const templateOptions=[
  {value:'split-classic',label:'Official Classic'}, {value:'split-emerald',label:'Emerald Panel'},
  {value:'split-minimal',label:'Formal Minimal'}, {value:'split-clean',label:'Contemporary Clean'},
  {value:'split-slate',label:'Slate Signature'}, {value:'split-bamboo',label:'Bamboo Ledger'},
  {value:'split-maroon',label:'Heritage Maroon'}, {value:'split-ocean',label:'Coastal Blue'},
  {value:'split-violet',label:'Academic Violet'}, {value:'split-sunrise',label:'Sunrise Ledger'},
  {value:'split-graphite',label:'Graphite Type'}, {value:'subject-list',label:'Compact Subject List'}
];
const validFacultyEmail=value=>/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(String(value||'').trim());
const filters = ui.schedules || (ui.schedules={department:'',period:'',view:'sections',program:'',year:'',search:'',selected:[],tab:'current',template:'split-classic'});
export function runTable(container, runs) { table(container, { columns: [{ title: 'Run', render: r => `<span class="main-cell">#${r.id} / ${e(label('departments', r.department_id))}</span><span class="sub-cell">${e(r.created_at)} UTC</span>` }, { title: 'Academic period', render: r => e(label('academic_periods', r.period_id)) }, { title: 'Result', render: r => `${pill(r.outcome, r.outcome === 'partial' ? 'warning' : '')}<span class="sub-cell">${r.placed} / ${r.required} requirements</span>` }, { title: 'Mode / state', render: r => `${pill(r.status, r.status === 'committed' ? '' : 'neutral')}<span class="sub-cell">${e(r.mode)}</span>` }, { title: 'Actions', render: r => `<div class="cell-actions">${button('Review', { small: true, icon: 'eye', action: 'review' })}${r.status === 'committed' ? button('Release', { small: true, action: 'release' }) : ''}</div>` }], rows: runs, emptyTitle: 'No generation history yet', emptyText: 'Generate a preview to create a reviewable history record.', onAction: (a, r) => { if (a === 'review')
        navigate('generation', { run: r.id });
    else
        releaseRun(r); } }); }
export async function releaseRun(run) { const rev = state.revision; await confirm({ title: 'Release committed reservations?', message: `Remove the active reservations from run #${run.id} for ${label('departments', run.department_id)}. Teaching assignments and historical results remain. Other runs are untouched. Existing previews become stale.`, word: 'RELEASE', label: 'Release reservations', action: async () => { await api('/runs/' + run.id + '/release', { method: 'POST', body: { confirm: 'RELEASE' }, revision: rev }); await refresh(); toast('Reservations released. The historical run is still available.'); } }); }
// Printable Section / Room / Faculty directory, based on the existing PDF
// layouts. The Directory is a view of *committed* schedules only.
function resourceKind(view){return view==='rooms'?'rooms':view==='faculty'?'faculty':'sections';}
function resourceId(m,view){return m[view==='rooms'?'room_id':view==='faculty'?'faculty_id':'section_id'];}
function resourceMeetings(meetings,view,id){return meetings.filter(m=>resourceId(m,view)===Number(id));}
function pdfUrl({period,department='',view,resource=0,template='split-classic',inline=false}){
    const q=new URLSearchParams({period:String(period),view,template});
    if(department)q.set('department',String(department));
    if(resource)q.set('resource',String(resource));
    if(inline)q.set('inline','1');
    return '/api/timetable/pdf?'+q;
}
function zipUrl({period,department='',view,ids=[],template='split-classic'}){
    const q=new URLSearchParams({period:String(period),view,template});
    if(department)q.set('department',String(department));
    if(ids.length)q.set('ids',ids.join(','));
    return '/api/timetable/zip?'+q;
}
function groupingKey(m,view){return [resourceId(m,view),m.subject_id,m.section_id,m.faculty_id,m.room_id,m.component_id||0,m.component_kind||'',m.start_minute,m.end_minute].join('-');}
const daysLabel=[null,'Monday','Tuesday','Wednesday','Thursday','Friday','Saturday','Sunday'];
function timetableTable(meetings,view){
    const grouped=new Map();
    for(const m of meetingRows(meetings)){
        const k=groupingKey(m,view);
        if(!grouped.has(k))grouped.set(k,{...m,days:[]});
        if(!grouped.get(k).days.includes(m.day))grouped.get(k).days.push(m.day);
    }
    const entries=[...grouped.values()].sort((a,b)=>a.start_minute-b.start_minute||a.subject_id-b.subject_id);
    const buckets=[
        {title:'Monday, Wednesday, Friday',items:entries.filter(m=>m.days.every(d=>[1,3,5].includes(d)))},
        {title:'Tuesday, Thursday',items:entries.filter(m=>m.days.every(d=>[2,4].includes(d)))},
        {title:'Saturday',items:entries.filter(m=>m.days.every(d=>d===6))},
        {title:'Sunday',items:entries.filter(m=>m.days.every(d=>d===7))},
        {title:'Other combinations',items:entries.filter(m=>!m.days.every(d=>[1,3,5].includes(d))&&!m.days.every(d=>[2,4].includes(d))&&!m.days.every(d=>d===6)&&!m.days.every(d=>d===7))}
    ];
    return buckets.filter(b=>b.items.length).map(b=>`<section class="timetable-group"><h3>${e(b.title)}</h3><div class="table-scroll"><table class="timetable-preview-table"><thead><tr><th>Time</th><th>Days</th><th>Subject</th><th>Units</th><th>${view==='sections'?'Room':'Section'}</th><th>${view==='faculty'?'Room':'Instructor'}</th></tr></thead><tbody>${b.items.map(m=>{const sub=record('subjects',m.subject_id),component=record('subject_components',m.component_id);return `<tr><td class="time-cell">${e(clock(m.start_minute))} – ${e(clock(m.end_minute))}</td><td>${e(m.days.sort((a,b)=>a-b).map(d=>daysLabel[d].slice(0,2)).join(', '))}</td><td><strong>${e(sub?.code||'Subject')}</strong><span>${e(sub?.title||'')}${m.component_kind?' / '+e(m.component_kind):''}</span></td><td>${e(component?.units??sub?.units??'–')}</td><td>${e(label(view==='sections'?'rooms':'sections',view==='sections'?m.room_id:m.section_id))}</td><td>${e(label(view==='faculty'?'rooms':'faculty',view==='faculty'?m.room_id:m.faculty_id))}</td></tr>`;}).join('')}</tbody></table></div></section>`).join('');
}
function sendFacultyTimetableEmail(faculty, period, department, template){
    // Never allow the operator to substitute the recipient: the backend reads
    // the current address from the faculty record again immediately before SMTP.
    const email=String(faculty.email||'').trim();
    if(!validFacultyEmail(email)){toast('Add a valid email address to this faculty profile first.','warning');return;}
    const confirmModal=dialog({title:'Email faculty timetable',description:'Send the currently committed faculty schedule as a PDF attachment.',
      body:`<div class="stack schedule-mail-review"><div class="schedule-mail-recip"><span>Faculty member</span><strong>${e(faculty.name)}</strong></div><div class="schedule-mail-recip"><span>Recorded recipient</span><strong>${e(email)}</strong></div><div class="schedule-mail-recip"><span>Academic period</span><strong>${e(label('academic_periods',period))}</strong></div><div class="schedule-mail-recip"><span>PDF design</span><strong>${e(templateOptions.find(t=>t.value===template)?.label||'Official Classic')}</strong></div><label class="check"><input type="checkbox" data-mail-approval> I verified the saved recipient and approve emailing this faculty schedule.</label><p class="form-error" role="alert" hidden></p></div>`,
      actions:[{label:'Cancel',run:m=>m.close()},{label:'Send schedule',icon:'mail',tone:'primary',id:'send-mail',run:async(m,b)=>{
         if(!m.body.querySelector('[data-mail-approval]').checked)return;
         m.setBusy(true);setBusy(b,true,'Sending...');
         try{await api('/timetable/faculty/email',{method:'POST',body:{faculty_id:faculty.id,period_id:Number(period),department_id:Number(department)||0,template,confirm:'SEND'}});m.setBusy(false);m.close();toast('SMTP accepted the faculty schedule. Inbox delivery is not guaranteed.');}
         catch(err){m.setBusy(false);setBusy(b,false);const error=m.body.querySelector('.form-error');error.textContent=err.message;error.hidden=false;}
      }}]});
    const approval=confirmModal.body.querySelector('[data-mail-approval]');
    confirmModal.controls['send-mail'].disabled=true;
    approval.onchange=()=>{confirmModal.controls['send-mail'].disabled=!approval.checked;};
}
function schoolLetterhead(){
    return `<header class="timetable-letterhead" aria-label="Buenavista Community College official header"><div class="timetable-letterhead-main">
      <div class="timetable-brand-left"><img class="timetable-campus" src="/assets/timetable/bcc-building.jpg" alt="Buenavista Community College building"><img class="timetable-college-seal" src="/assets/timetable/bcc-seal.png" alt="BCC seal"></div>
      <div class="timetable-school-title"><span>Republic of the Philippines</span><span>Commission on Higher Education</span><strong>BUENAVISTA COMMUNITY COLLEGE</strong><span>Cangawa, Buenavista, Bohol</span><em>“Caring Your Future”</em><small>(038) 500-0137; registrarofbcc@yahoo.com</small></div>
      <div class="timetable-official-logos"><img src="/assets/timetable/buenavista-municipality.png" alt="Municipality of Buenavista"><img src="/assets/timetable/ched.png" alt="Commission on Higher Education"><img src="/assets/timetable/bagong-pilipinas.png" alt="Bagong Pilipinas"></div>
    </div><div class="timetable-vision-mission"><div><strong>VISION:</strong><span>A leading community college committed to transformative education that produces globally competent professionals.</span></div><div><strong>MISSION:</strong><span>BCC is committed to providing transformative education, advancing relevant research, and engaging meaningful extension services for sustainable community development.</span></div></div></header>`;
}
function showTimetable(meetings,view,resource,period,department){
    const key=resourceKind(view),item=record(key,resource);
    if(!item)return;
    const actual=resourceMeetings(meetings,view,resource);
    if(!actual.length){toast('No committed timetable for this record in the selected period.','warning');return;}
    const periodInfo=record('academic_periods',period);
    const title=fullLabel(key,resource);
    const scheduleTitle=view==='sections'?'Section Schedule':view==='rooms'?'Room Schedule':'Faculty Schedule';
    const uniqueSections=[...new Set(actual.map(m=>m.section_id))];
    const programs=[...new Set(uniqueSections.map(id=>record('sections',id)?.program_id).filter(Boolean))];
    const departments=[...new Set(programs.map(id=>record('programs',id)?.department_id).filter(Boolean))];
    const weeklyMinutes=actual.reduce((sum,m)=>sum+m.end_minute-m.start_minute,0);
    const emailAvailable=view==='faculty'&&validFacultyEmail(item.email)&&(state.user?.role!=='scheduler'||item.department_id===state.user.department_id);
    const storedTemplate=templateOptions.find(x=>x.value===filters.template)?.value||'split-classic';
    const body=`<div class="timetable-modal-toolbar"><div class="timetable-toolbar-label"><strong>Official schedule preview</strong><span>${e(periodInfo?.label||'Academic period')} · Landscape / Letter</span></div><div class="timetable-toolbar-actions"><div class="field timetable-template-field"><label>Design template</label><div data-template-picker></div></div><div class="timetable-toolbar-buttons">${button('Print',{action:'print',icon:'print'})}${button('Download PDF',{action:'pdf',icon:'download',tone:'primary'})}${emailAvailable?button('Email faculty',{action:'email',icon:'mail'}):''}</div></div></div>
      ${view==='faculty'&&!emailAvailable?`<p class="timetable-email-help">${validFacultyEmail(item.email)?'Email sending is limited to authorized faculty records.':'An email button will appear when a valid email is saved on this faculty record.'}</p>`:''}
      <div class="timetable-paper-viewport" tabindex="0" aria-label="Scrollable schedule page"><article class="timetable-sheet">
       ${schoolLetterhead()}<h2>${e(scheduleTitle)}</h2><div class="timetable-metadata"><p><strong>${view==='sections'?'Section':view==='rooms'?'Room / Laboratory':'Instructor'}:</strong> ${e(title)}</p><p><strong>Semester:</strong> ${e(periodInfo?.semester||'')}</p><p><strong>Department:</strong> ${e(departments.map(id=>label('departments',id)).join(' / ')||'Not assigned')}</p><p><strong>Academic Year:</strong> ${e(periodInfo?.academic_year||'')}</p></div>
       ${timetableTable(actual,view)}<footer class="timetable-signature"><span>Conforme: _____________________</span><span>Weekly contact: ${e(hours(weeklyMinutes))}</span><span>Designed and Developed by CITE and ICT Department</span></footer></article></div>`;
    const d=dialog({title,description:scheduleTitle+' · '+(periodInfo?.label||''),wide:true,body,actions:[{label:'Close',run:m=>m.close()}]});
    d.el.classList.add('timetable-dialog');
    d.el.dataset.template=storedTemplate;
    const template=dropdown({options:templateOptions,value:storedTemplate,label:'Schedule design template',onChange:v=>{filters.template=v;d.el.dataset.template=v;}});
    d.body.querySelector('[data-template-picker]').append(template.el);
    const open=(inline=false)=>window.open(pdfUrl({view,period,department,resource,template:template.value,inline}),'_blank','noopener');
    d.body.querySelector('[data-action=print]').onclick=()=>open(true);
    d.body.querySelector('[data-action=pdf]').onclick=()=>open(false);
    d.body.querySelector('[data-action=email]')?.addEventListener('click',()=>sendFacultyTimetableEmail(item,period,department,template.value));
}
function scopedForPeriod(){return rows('scheduled_meetings').filter(m=>m.period_id===Number(filters.period)&&(!filters.department||m.department_id===Number(filters.department)));}
function matchingDirectoryRows(){
    const view=filters.view,records=rows(resourceKind(view)),ms=scopedForPeriod();
    const byId=new Map();for(const m of ms){const id=resourceId(m,view);if(!byId.has(id))byId.set(id,[]);byId.get(id).push(m);}
    const text=filters.search.toLocaleLowerCase();
    return records.filter(r=>{
      const groups=byId.get(r.id)||[];if(!groups.length)return false;
      const sections=groups.map(m=>record('sections',m.section_id)).filter(Boolean);
      if(filters.program&&!sections.some(s=>s.program_id===Number(filters.program)))return false;
      if(view!=='faculty'&&filters.year&&!sections.some(s=>s.year_level===Number(filters.year)))return false;
      if(text&&!fullLabel(resourceKind(view),r.id).toLocaleLowerCase().includes(text))return false;
      return true;
    }).map(r=>({record:r,meetings:byId.get(r.id)})).sort((a,b)=>fullLabel(resourceKind(view),a.record.id).localeCompare(fullLabel(resourceKind(view),b.record.id)));
}
export function renderSchedules(root,params){
    if(params.has('department'))filters.department=params.get('department');
    if(params.has('period'))filters.period=params.get('period');
    if(!filters.period)filters.period=String(rows('academic_periods').find(active)?.id||'');
    const routeTab=params.get('tab');if(routeTab==='history')filters.tab='history';else if(routeTab==='current')filters.tab='current';
    root.innerHTML=heading('Official schedules','Section, Room and Faculty schedules use the same committed meetings. Open the official preview, print or download PDFs.',button('Generate a department',{icon:'generation',tone:'primary',action:'generate'}),'TIMETABLE DIRECTORY')+`<div id="schedule-tabs" class="tabs"></div><div id="schedules-content"></div>`;
    root.querySelector('[data-action=generate]').onclick=()=>navigate('generation');
    function page(){
        tabs(root.querySelector('#schedule-tabs'),[{value:'current',label:'Schedule directory',count:rows('scheduled_meetings').length},{value:'history',label:'Generation history',count:state.runs.length}],filters.tab,v=>{filters.tab=v;page();});
        const body=root.querySelector('#schedules-content');
        if(filters.tab==='history'){
            body.innerHTML=`${notice('Released and replaced previews do not reserve rooms; only committed meetings appear in the directory.')}<div class="panel flush" id="schedule-history"></div>`;
            runTable(body.querySelector('#schedule-history'),state.runs);return;
        }
        body.innerHTML=`<section class="panel directory-panel"><div class="directory-heading"><div class="directory-resource-tabs tabs" id="resource-tabs"></div><label class="field directory-search"><span>Search ${e(filters.view)}</span><input id="schedule-search" type="search" placeholder="Search ${e(filters.view)}..." value="${e(filters.search)}"></label></div>
          <div class="directory-filters"><div class="field"><label>Academic period</label><div id="schedule-period"></div></div><div class="field"><label>Department</label><div id="schedule-department"></div></div><div class="field"><label>Program</label><div id="schedule-program"></div></div>${filters.view==='faculty'?'':'<div class="field"><label>Year level</label><div id="schedule-year"></div></div>'}</div>
          <div class="directory-actions"><span id="directory-count" class="muted small"></span><div class="row">${button('Select all results',{action:'select-all',small:true})}${button('Clear selection',{action:'clear',small:true})}${button('Download all ZIP',{action:'zip',small:true,icon:'download'})}</div></div><div class="panel flush" id="directory-table"></div></section>`;
        const dropdowns={
          period:dropdown({options:options('academic_periods',{all:true}),value:filters.period,label:'Academic period',onChange:v=>{filters.period=v;selected.clear();draw();}}),
          department:dropdown({options:options('departments',{all:true,empty:'All departments'}),value:filters.department,label:'Department',onChange:v=>{filters.department=v;rememberContext({department:v});selected.clear();draw();}}),
          program:dropdown({options:options('programs',{all:true,empty:'All programs',predicate:p=>!filters.department||p.department_id===Number(filters.department)}),value:filters.program,label:'Program',onChange:v=>{filters.program=v;rememberContext({program:v});selected.clear();draw();}}),
          year:dropdown({options:[{value:'',label:'All year levels'},...[1,2,3,4,5,6].map(n=>({value:String(n),label:yearLabel(n)}))],value:filters.year,label:'Year level',onChange:v=>{filters.year=v;rememberContext({year:v});selected.clear();draw();}})
        };
        for(const [key,dd]of Object.entries(dropdowns))body.querySelector('#schedule-'+key)?.append(dd.el);
        let selected=new Set(filters.selected.map(Number));
        function setView(view){filters.view=view;filters.search='';selected.clear();filters.selected=[];page();}
        tabs(body.querySelector('#resource-tabs'),[{value:'sections',label:'Sections'},{value:'rooms',label:'Rooms'},{value:'faculty',label:'Faculty'}],filters.view,setView);
        const search=body.querySelector('#schedule-search');search.oninput=()=>{filters.search=search.value;draw();};
        function draw(){
            if(filters.department && filters.program && record('programs',filters.program)?.department_id!==Number(filters.department))filters.program='';
            dropdowns.program.setOptions(options('programs',{all:true,empty:'All programs',predicate:p=>!filters.department||p.department_id===Number(filters.department)}));
            dropdowns.program.value=filters.program;
            const list=matchingDirectoryRows(),view=filters.view;
            const currentSet=new Set(scopedForPeriod().map(m=>resourceId(m,view)));
            selected=new Set([...selected].filter(id=>currentSet.has(id)));
            filters.selected=[...selected];
            body.querySelector('#directory-count').textContent=list.length+' timetables · '+selected.size+' selected';
            const columns=[
                {title:'Select',render:x=>`<input type="checkbox" data-select="${x.record.id}" aria-label="Select ${e(fullLabel(resourceKind(view),x.record.id))}" ${selected.has(x.record.id)?'checked':''}>`},
                {title:view==='sections'?'Section':view==='rooms'?'Room / Laboratory':'Instructor',render:x=>`<strong class="main-cell">${e(fullLabel(resourceKind(view),x.record.id))}</strong>`},
                {title:'Department / Program',render:x=>{const departments=[...new Set(x.meetings.map(m=>label('departments',m.department_id)))];const programs=[...new Set(x.meetings.map(m=>record('sections',m.section_id)?.program_id).filter(Boolean).map(id=>label('programs',id)))];return `<span class="main-cell">${e(departments.join(' / '))}</span><span class="sub-cell">${e(programs.join(' / '))}</span>`;}},
                {title:'Days',render:x=>e([...new Set(x.meetings.map(m=>m.day))].sort((a,b)=>a-b).map(n=>days[n].slice(0,3)).join(', '))},
                {title:'Weekly hours',render:x=>e(hours(x.meetings.reduce((n,m)=>n+m.end_minute-m.start_minute,0)))},
                {title:'Actions',render:x=>`<div class="cell-actions">${button('View',{action:'view',small:true,icon:'eye'})}${view==='faculty'&&validFacultyEmail(x.record.email)&&(state.user?.role!=='scheduler'||x.record.department_id===state.user.department_id)?button('Email',{action:'email',small:true,icon:'mail'}):''}</div>`}
            ];
            const tableArea=body.querySelector('#directory-table');
            table(tableArea,{columns,rows:list,emptyTitle:'No committed timetables',emptyText:'Generate and commit a department schedule, or change filters.',pageSize:20,onAction:(a,x)=>{if(a==='view'&&x)showTimetable(scopedForPeriod(),view,x.record.id,filters.period,filters.department);if(a==='email'&&x&&view==='faculty')sendFacultyTimetableEmail(x.record,filters.period,filters.department,filters.template);}});
            tableArea.querySelectorAll('input[data-select]').forEach(input=>input.addEventListener('change',()=>{const id=Number(input.dataset.select);input.checked?selected.add(id):selected.delete(id);filters.selected=[...selected];body.querySelector('#directory-count').textContent=list.length+' timetables · '+selected.size+' selected';}));
        }
        body.querySelector('[data-action=select-all]').onclick=()=>{matchingDirectoryRows().forEach(x=>selected.add(x.record.id));draw();};
        body.querySelector('[data-action=clear]').onclick=()=>{selected.clear();draw();};
        body.querySelector('[data-action=zip]').onclick=()=>{
            const ids=selected.size?[...selected]:matchingDirectoryRows().map(x=>x.record.id);
            if(!ids.length){toast('No timetables available to download.','warning');return;}
            window.open(zipUrl({view:filters.view,period:filters.period,department:filters.department,ids,template:filters.template}),'_blank','noopener');
        };
        draw();
    }
    page();
}
