// Metadata-driven academic and facility forms with client-side validation.
import { api } from './api.js';
import { state, rows, record, label, fullLabel, curriculumName, options, active, departmentOf, requiredSubjects, curriculumFor, ui, yearLabel, rememberContext, refresh, navigate, canEdit, ownDepartment } from './store.js';
import { e, clock, parseClock, hours, days, pill, button, iconButton, notice, heading, empty, dialog, confirm, dropdown, table, tabs, toast, setBusy } from './components.js';
import { icon } from './icons.js';
import {deleteWithImpact} from './linked-delete.js';
import { relationPicker, pickRecords, dayValues, subjectDays, subjectYears, programIDs, programOptions, matchesDepartment, matchesYear, yearOptions } from './pickers.js';
import {subjectPlanEditor} from './subject-plan.js';
let formSerial = 0;
export async function editRecord(kind, original = null, defaults = {}, after = () => {}) {
    const spec=state.specs[kind];if(!spec)return;
    if(original&&!canEdit(kind,original)){toast('This record is managed by its department. You may view and select it, but only its owner can edit it.','warning');return;}
    
    const context = ui.context;
    const suggested = {};
    if (!original) {
        if (kind === 'sections') { suggested.program_id = Number(context.program || 0) || ''; suggested.year_level = Number(context.year || 1); }
        if (['programs','subjects','faculty','buildings','rooms'].includes(kind)) suggested.department_id = Number(ownDepartment() || context.department || 0) || '';
    }
    const originalRevision = state.revision, values = {...suggested, ...defaults, ...original}, controls = {}, serial = ++formSerial;
    let subjectPlan=null;
    const skipped=f=>(kind==='sections'&&['curriculum_id','period_id'].includes(f.name))||(kind==='faculty'&&['name','section_policy'].includes(f.name))||(kind==='subjects'&&['units','weekly_minutes','required_room'].includes(f.name));
    const d = dialog({wide:['faculty','subjects'].includes(kind),title:(original?'Edit ':'Add ')+spec.label.toLowerCase(),description:kind==='sections'?'Reusable section bank: choose a program and year. Generation selects the correct semester curriculum automatically. Adviser is optional.':kind==='subjects'?'Choose regular meeting days and, optionally, Saturday fallback. Working hours and conflicts always apply.':'Changes are validated before being saved.',body:'<form class="record-form"><div class="form-grid"></div><p class="form-error" role="alert" hidden></p></form>',actions:[{label:'Cancel',run:m=>m.close()},{label:'Save '+spec.label.toLowerCase(),tone:'primary',id:'save',icon:'check',run:()=>save()}]});
    const grid=d.body.querySelector('.form-grid'),form=d.body.querySelector('form');if(kind==='faculty')grid.classList.add('faculty-name-grid');
    if(kind==='sections'){d.el.classList.add('section-editor');grid.classList.add('section-form-grid');}
    if(kind==='faculty'){d.el.classList.add('faculty-editor');if(original&&!original.first_name){const info=document.createElement('div');info.className='field-wide';info.innerHTML=notice('Existing name: '+original.name+'. It will stay unchanged unless you enter the first and last name below. Multi-part names are not split automatically.');grid.append(info);}}
    for (const f of spec.fields) {
        if(kind==='subjects'&&f.name==='units'){subjectPlan=subjectPlanEditor(original);grid.append(subjectPlan.el);}
        if(skipped(f))continue;
        const wrap=document.createElement('div');wrap.className='field'+(['textarea','weekdays'].includes(f.type)?' field-wide':'');
        const id='field-'+serial+'-'+f.name, value=values[f.name]??f.default??'';
        if(kind==='sections'){
            wrap.classList.add('section-field-'+f.name);
            if(f.name==='adviser_id')wrap.classList.add('field-wide');
            if(f.name==='active')wrap.classList.add('field-wide');
        }
        if(kind==='academic_periods' && f.name==='academic_year') {
            const pair=String(value).match(/^(\d{4})-(\d{4})$/);
            wrap.innerHTML=`<label>Academic year *</label><div class="year-pair"><input type="number" min="2000" max="2200" data-year-start required placeholder="2026" value="${e(pair?.[1]||'')}"><span aria-hidden="true">—</span><input type="number" min="2001" max="2201" data-year-end required placeholder="2027" value="${e(pair?.[2]||'')}"></div><small class="help">Use consecutive years, such as 2026 — 2027.</small>`;
            controls[f.name]={getValue:()=>wrap.querySelector('[data-year-start]').value+'-'+wrap.querySelector('[data-year-end]').value};
        }else if(kind==='academic_periods' && f.name==='semester') {
            const standard=['1st Semester','2nd Semester','Summer'];const custom=!!value&&!standard.includes(value);
            wrap.innerHTML=`<label>Semester *</label><select data-semester><option value="1st Semester">First</option><option value="2nd Semester">Second</option><option value="Summer">Summer</option><option value="custom">Others (custom)</option></select><input data-custom-semester type="text" placeholder="Custom semester / session" maxlength="80" ${custom?'':'hidden'} value="${e(custom?value:'')}">`;
            const select=wrap.querySelector('select'),input=wrap.querySelector('[data-custom-semester]');select.value=custom?'custom':(value||'1st Semester');select.onchange=()=>{input.hidden=select.value!=='custom';input.required=select.value==='custom'};
            controls[f.name]={getValue:()=>select.value==='custom'?input.value.trim():select.value};
        }else if(kind==='sections' && f.name==='year_level') {
            wrap.innerHTML=`<label for="${id}">Year level <span class="muted">*</span></label>`;
            const dd=dropdown({options:[1,2,3,4,5,6].map(y=>({value:String(y),label:yearLabel(y)})),value:String(value||1),label:'Section year level',search:false});
            dd.button.id=id;wrap.append(dd.el);
            controls[f.name]={getValue:()=>Number(dd.value),dd};
        }else if(kind==='faculty' && f.name==='employment') {
            const originalValue=String(value||'').trim();
            const selectedValue=originalValue==='Full-Time'?'Full Time':originalValue==='Part-Time'?'Part Time':originalValue;
            const choices=[{value:'',label:'Select employment status'}, {value:'Full Time',label:'Full Time'}, {value:'Part Time',label:'Part Time'}];
            if(selectedValue&&!['Full Time','Part Time'].includes(selectedValue))choices.push({value:selectedValue,label:selectedValue+' (existing label)'});
            wrap.innerHTML=`<label for="${id}">Employment status</label>`;
            const select=dropdown({options:choices,value:selectedValue,label:'Faculty employment status',search:false});select.button.id=id;
            wrap.append(select.el);
            if(selectedValue&&!['Full Time','Part Time'].includes(selectedValue)){
                const help=document.createElement('small');help.className='help';help.textContent='This record has a legacy employment label. It will be preserved unless you select Full Time or Part Time.';wrap.append(help);
            }
            controls[f.name]={getValue:()=>select.value,dd:select};
        }else if(f.type==='checkbox') {
            wrap.innerHTML=`<label class="check" for="${id}"><input id="${id}" type="checkbox" ${Number(value)===1?'checked':''}> ${e(kind==='sections'?'Active section':f.label)}</label>${kind==='sections'?'<small class="help">Include this section in scheduling and academic listings.</small>':''}`;
            const input=wrap.querySelector('input');controls[f.name]={getValue:()=>input.checked?1:0,input};
        } else if(f.type==='weekdays') {
            const selected=dayValues(value);wrap.innerHTML=`<fieldset class="day-choices"><legend>${e(f.label)} *</legend>${[7,1,2,3,4,5,6].map(day=>`<label class="day-choice"><input type="checkbox" value="${day}" ${selected.includes(day)?'checked':''}><span>${e(days[day].slice(0,3))}</span></label>`).join('')}</fieldset>`;
            controls[f.name]={getValue:()=>[...wrap.querySelectorAll('input:checked')].map(i=>Number(i.value)).sort((a,b)=>a-b).join(','),wrap};
        } else {
            wrap.innerHTML=`<label for="${id}">${e(kind==='sections'&&f.name==='adviser_id'?'Adviser (optional)':f.label)}${f.required?' <span class="muted">*</span>':''}</label>`;
            if(f.type==='relation' && ['faculty','subjects'].includes(f.ref)) {
                const picker=relationPicker(f.ref,{value,required:f.required,label:kind==='sections'&&f.name==='adviser_id'?'Optional section adviser':f.label});picker.button.id=id;controls[f.name]=picker;wrap.append(picker.el);
            } else if(f.type==='relation'||f.type==='select'||f.name==='day') {
                const opts=f.type==='relation'?options(f.ref,{all:true,empty:f.required?'Select '+f.label:'Not assigned / shared'}):f.name==='day'?[7,1,2,3,4,5,6].map(v=>({value:String(v),label:days[v]})):f.options.map(x=>({value:x,label:({auto:'Automatic (exact contact time)',mwf:'Monday / Wednesday / Friday',tth:'Tuesday / Thursday',sat:'Saturday',sun:'Sunday',flex:'Flexible selected days'})[x]||x.replaceAll('_',' ')}));
                const dd=dropdown({options:opts,value,placeholder:'Select '+f.label,label:f.label});dd.button.id=id;
                controls[f.name]={getValue:()=>f.type==='relation'?(dd.value?Number(dd.value):null):f.name==='day'?Number(dd.value):dd.value,dd};wrap.append(dd.el);
            } else {
                const input=document.createElement(f.type==='textarea'?'textarea':'input');input.id=id;input.name=f.name;
                if(f.type!=='textarea')input.type=['integer','number'].includes(f.type)?'number':['time','date'].includes(f.type)?f.type:'text';
                input.value=f.type==='time'?clock(value):value;input.required=!!f.required;
                if(kind==='faculty'&&['first_name','last_name'].includes(f.name))input.required=!original||!!original.first_name;
                if(kind==='faculty'&&f.name==='email')input.type='email';
                if(['number','integer'].includes(f.type)){input.min=f.min||0;if(f.max)input.max=f.max;input.step=f.type==='number'?'any':'1';}
                if(f.type==='time')input.step='900';if(f.type==='text')input.maxLength=240;
if(f.type==='textarea')input.maxLength=2000;
                wrap.append(input);controls[f.name]={getValue:()=>f.type==='time'?parseClock(input.value):['integer','number'].includes(f.type)?Number(input.value):input.value};
            }
        }
        if(kind==='sections'&&f.name==='adviser_id'){
            const helper=document.createElement('small');helper.className='help';helper.textContent='Optional. Schedule generation will remind you if this section has no adviser; it will not block scheduling.';wrap.append(helper);
        }
        if(kind==='academic_periods'&&f.name==='term'){const helper=document.createElement('small');helper.className='help';helper.textContent='Leave blank for a normal semester. Use only for a special session label (e.g., Block A).';wrap.append(helper);}
        if(f.help){const help=document.createElement('small');help.className='help';help.textContent=f.help;wrap.append(help);}if(kind==='academic_periods' && ['start_date','end_date'].includes(f.name)){let details=d.body.querySelector('#optional-dates');if(!details){details=document.createElement('details');details.id='optional-dates';details.className='field-wide';details.innerHTML='<summary>Optional calendar dates</summary><div class="form-grid"></div>';grid.append(details);}details.querySelector('.form-grid').append(wrap);}else grid.append(wrap);
    }
    if(kind==='subjects') {
        const fallback=controls.saturday_fallback.input;
        const update=()=>{const regularSaturday=dayValues(controls.allowed_days.getValue()).includes(6);if(regularSaturday)fallback.checked=false;fallback.disabled=regularSaturday;};
        controls.allowed_days.wrap.addEventListener('change',update);update();
    }
    if(kind==='sections') {
        const explanation=document.createElement('div');explanation.className='section-match-status field-wide';
        explanation.innerHTML=notice('This section can be reused in future academic years and semesters for its selected year level. It is not attached to a specific academic period. When generating, ASFWMS matches the selected term to its program, year level and curriculum automatically.','info','Reusable section bank');
        grid.append(explanation);
        const currentProgram=Number(controls.program_id.getValue());
        if(currentProgram&&!original)rememberContext({program:currentProgram,department:record('programs',currentProgram)?.department_id||'',year:controls.year_level.getValue()});
        controls.program_id.dd.el.addEventListener('change',()=>{const pid=Number(controls.program_id.getValue());rememberContext({program:pid,department:record('programs',pid)?.department_id||''});});
        controls.year_level.dd.el.addEventListener('change',()=>rememberContext({year:controls.year_level.getValue()}));
    }
    if(['academic_periods','faculty'].includes(kind)) {
        const note=document.createElement('div');note.className='field-wide';note.innerHTML=notice(kind==='academic_periods'?'Semester and term are context labels. Meeting days are controlled per subject, not by this period. Lunch defaults to 60 minutes; sections are limited to three consecutive subject meetings with a free meeting slot before the next group.':'Consideration is a scheduling priority flag. Availability and unavailable windows are hard restrictions; written notes remain informational.');grid.append(note);
    }
    form.onsubmit=ev=>{ev.preventDefault();save();};
    async function save(){
        if(d.busy||!form.reportValidity())return;
        const body={};
        for(const f of spec.fields){if(skipped(f))continue;body[f.name]=controls[f.name].getValue();if(f.required&&f.type==='relation'&&!body[f.name])return showError('Choose '+f.label+'.');if(f.type==='weekdays'&&!body[f.name])return showError('Select at least one regular meeting day.');}
        if(kind==='faculty'&&original&&!original.first_name&&!['prefix','first_name','middle_name','last_name','extension'].some(k=>body[k])){for(const k of ['prefix','first_name','middle_name','last_name','extension'])delete body[k];}
        if(subjectPlan){try{Object.assign(body,subjectPlan.getValue());}catch(err){return showError(err.message);}}
        d.setBusy(true);setBusy(d.controls.save,true,'Saving...');
        try{
            await api('/records/'+kind+(original?'/'+original.id:''),{method:original?'PUT':'POST',body,revision:originalRevision});
            if(body.program_id&&['sections','curricula'].includes(kind))rememberContext({program:body.program_id,department:record('programs',body.program_id)?.department_id||'',year:body.year_level});
            if(body.department_id&&['faculty','subjects','programs','buildings','rooms'].includes(kind))rememberContext({department:body.department_id});
            await refresh();d.setBusy(false);d.close();toast(spec.label+' saved.');after();}
        catch(err){d.setBusy(false);setBusy(d.controls.save,false);showError(err.message);}
    }
    function showError(message){const p=d.body.querySelector('.form-error');p.textContent=message;p.hidden=false;p.scrollIntoView({block:'nearest'});}
}
export async function deleteRecord(kind,r,after=()=>{}) {const link=['curriculum_subject_years','faculty_subjects','teaching_assignments'].includes(kind);return deleteWithImpact({table:kind,id:r.id,title:(link?'Unlink ':'Delete ')+state.specs[kind].label.toLowerCase()+'?',after});}
const status = r => pill(r.active ? 'Active' : 'Inactive', r.active ? '' : 'neutral');
const display = (main, sub = '') => `<span class="main-cell">${e(main)}</span>${sub ? `<span class="sub-cell">${e(sub)}</span>` : ''}`;
function actions(r, extra = '', isLink = false) { return `<div class="cell-actions">${extra}${iconButton('edit', 'Edit record', 'edit')}${iconButton(isLink?'link':'trash', isLink?'Unlink relationship':'Delete record', 'delete', 'danger')}</div>`; }
export function recordColumns(kind) {
    switch (kind) {
        case 'departments': return [{ title: 'Department', render: r => display(r.code, r.name) }, { title: 'Programs', render: r => rows('programs').filter(x => x.department_id === r.id).length }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        case 'programs': return [{ title: 'Program', render: r => display(r.code, r.name) }, { title: 'Department', render: r => pill(label('departments', r.department_id), 'subtle') }, { title: 'Curricula', render: r => rows('curricula').filter(x => x.program_id === r.id).length }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        case 'subjects': return [{ title: 'Subject', render: r => display(r.code, r.title) }, { title: 'Faculty', render: r => { const eligible=rows('faculty_subjects').filter(x=>x.subject_id===r.id); const forced=rows('teaching_assignments').filter(x=>x.subject_id===r.id); return `<button type="button" class="cell-link" data-action="faculty" aria-label="Manage instructors for ${e(r.code)}"><strong class="main-cell">${eligible.length} eligible</strong><span class="sub-cell">${forced.length} explicitly assigned · Manage</span></button>`; } }, { title: 'Sections', render:r=>{const matching=subjectSections(r.id);return `<button type="button" class="cell-link" data-action="sections" aria-label="View sections using ${e(r.code)}"><strong class="main-cell">${matching.length} sections</strong><span class="sub-cell">View curriculum-linked sections</span></button>`;} }, { title: 'Department', render: r => e(label('departments', r.department_id)) }, { title: 'Contact / units', render: r => display(hours(r.weekly_minutes) + ' / week', r.units + ' units') }, { title: 'Priority', render: r => pill(r.priority, r.priority === 'major' ? '' : 'neutral') }, { title: 'Year / meetings', render: r => display(subjectYears(r.id).map(yearLabel).join(', ')||'Unassigned', subjectDays(r)) }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        case 'curricula': return [{ title: 'Curriculum', render: r => display(curriculumName(r.name), label('programs', r.program_id) + (r.year_level ? ' · '+yearLabel(r.year_level)+' · '+r.semester : ' · Multi-year (legacy)')) }, { title: 'Subject-year links', render: r => rows('curriculum_subject_years').filter(x => x.curriculum_id === r.id).length }, { title: 'Sections', render: r => rows('sections').filter(x => x.curriculum_id === r.id).length }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r, button('Subjects', { small: true, icon: 'book', action: 'view' })) }];
        case 'sections': return [{ title: 'Section', render: r => display(r.code, label('programs', r.program_id) + ' / ' + yearLabel(r.year_level)) }, { title: 'Curriculum', render: r => pill('Matched at generation','subtle') }, { title: 'Adviser', render: r => e(r.adviser_id ? label('faculty', r.adviser_id) : 'Not assigned') }, { title: 'Instructors', render: r => { const req = requiredSubjects(r); return pill(req.filter(x => x.assignment).length + ' / ' + req.length, req.some(x => !x.assignment) ? 'warning' : ''); } }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r, iconButton('eye', 'View section subjects', 'view')) }];
        case 'academic_periods': return [{ title: 'Academic period', render: r => display(r.label, r.academic_year + ' / ' + r.semester + (r.term ? ' / ' + r.term : '')) }, { title: 'Optional dates', render: r => r.start_date && r.end_date ? display(r.start_date, r.end_date) : 'Not required' }, { title: 'Campus hours', render: r => display(clock(r.day_start) + ' - ' + clock(r.day_end), 'Meeting days set per subject') }, { title: 'Optional breaks', render: r => display(`${r.lunch_minutes || 60} min lunch`, `Max ${r.max_consecutive || 3} consecutive section subjects`) }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        case 'buildings': return [{ title: 'Building / field', render: r => display(r.code, r.name) }, { title: 'Type', render: r => pill(r.kind, 'subtle') }, { title: 'Priority department', render: r => e(r.department_id ? label('departments', r.department_id) : 'Shared') }, { title: 'Rooms / areas', render: r => rows('rooms').filter(x => x.building_id === r.id).length }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        case 'rooms': return [{ title: 'Room / area', render: r => display(r.code, r.name) }, { title: 'Building', render: r => e(label('buildings', r.building_id)) }, { title: 'Owner', render: r => pill(r.department_id ? label('departments', r.department_id) : 'Shared', 'subtle') }, { title: 'Type / capacity', render: r => display(r.kind, r.capacity ? r.capacity + ' students' : 'Unspecified capacity') }, { title: 'Availability', render: r => e(clock(r.start_minute) + ' - ' + clock(r.end_minute)) }, { title: 'Status', render: status }, { title: 'Actions', render: r => actions(r) }];
        default: return [];
    }
}
export function renderRecords(root, { page, title, description, kinds, selected, params }) {
    const requested=selected||(page==='academic'?ui.academicTab:'');
    const kind=kinds.some(x=>x.value===requested)?requested:kinds[0].value;
    if(page==='academic')ui.academicTab=kind;
    const filters=ui.academic[kind]||(ui.academic[kind]={});
    const academic=page==='academic',yearFiltered=['subjects','sections','curricula'].includes(kind),programFiltered=['subjects','sections','curricula'].includes(kind);
    root.innerHTML=heading(title,description,button(kind==='curricula'?'Create curriculum':'Add '+state.specs[kind].label.toLowerCase(),{icon:'plus',tone:'primary',action:'add'}),academic?'ACADEMIC FOUNDATION':'PHYSICAL RESOURCES')+
        `<div class="academic-nav"><div class="tabs" id="record-tabs"></div>${academic?button('Sections '+rows('sections').length,{tone:kind==='sections'?'primary':'secondary',icon:'layers',action:'sections',className:'sections-cta'}):''}${kind==='sections'&&state.user.role==='developer'?button('Batch add sections',{icon:'plus',tone:'secondary',action:'batch-sections'}):''}</div><div class="panel flush"><div class="toolbar record-filters"><div class="search">${icon('search')}<input id="record-search" type="search" placeholder="Search ${e(state.specs[kind].label.toLowerCase())} records..." aria-label="Search records"></div><div id="department-filter"></div>${programFiltered?'<div id="program-filter"></div>':''}${yearFiltered?'<div id="year-filter"></div>':''}<label class="check"><input type="checkbox" id="show-inactive"> Include inactive</label></div><div id="record-table"></div></div>`;
    tabs(root.querySelector('#record-tabs'),kinds.filter(x=>!academic||x.value!=='sections').map(x=>({...x,count:rows(x.value).length})),kind,v=>navigate(page,{tab:v}));
    root.querySelector('[data-action=batch-sections]')?.addEventListener('click',batchCreateSections);
    const sectionsCTA=root.querySelector('[data-action=sections]');if(sectionsCTA){sectionsCTA.classList.add('sections-cta');sectionsCTA.onclick=()=>navigate(page,{tab:'sections'});sectionsCTA.setAttribute('aria-current',kind==='sections'?'page':'false');}
    const scopeReadOnly=state.user.role==='scheduler'&&['departments','academic_periods','curricula'].includes(kind);
    if(scopeReadOnly){root.dataset.scopeReadonly='true';root.querySelector('[data-action=add]').hidden=true;}
    root.querySelector('[data-action=add]').onclick=()=>{if(scopeReadOnly){toast('Only an Administrator or Developer can change this global setup.','warning');return;}rememberContext({department:department.value,program:program.value,year:year.value});kind==='curricula'?createCurriculum():editRecord(kind);};
    const department=dropdown({options:options('departments',{all:true,empty:'All departments'}),value:params?.get('department')??filters.department??ui.context.department??'',label:'Filter department',onChange:()=>{program.setOptions(programOptions(department.value,true));if(department.value&&program.value!=='unassigned'&&record('programs',program.value)?.department_id!==Number(department.value))program.value='';filters.page=1;saveFilters();rememberContext({department:department.value,program:program.value});render();}});
    const program=dropdown({options:programOptions(department.value,true),value:params?.get('program')??filters.program??(programFiltered?ui.context.program:'')??'',label:'Filter program',onChange:()=>{filters.page=1;saveFilters();rememberContext({program:program.value});render();}});
    const year=dropdown({options:yearOptions(kind==='subjects'),value:params?.get('year')??filters.year??(yearFiltered?ui.context.year:'')??'',label:'Filter year level',onChange:()=>{filters.page=1;saveFilters();rememberContext({year:year.value});render();}});
    if(kind!=='academic_periods')root.querySelector('#department-filter').append(department.el);
    root.querySelector('#program-filter')?.append(program.el);root.querySelector('#year-filter')?.append(year.el);
    function saveFilters(){Object.assign(filters,{department:department.value,program:program.value,year:year.value,search:search.value,inactive:inactive.checked});}
    const search=root.querySelector('#record-search'),inactive=root.querySelector('#show-inactive');search.value=filters.search||'';inactive.checked=!!filters.inactive;search.oninput=()=>{filters.page=1;saveFilters();render();};inactive.onchange=()=>{filters.page=1;saveFilters();render();};
    function render(){
        const term=search.value.trim().toLocaleLowerCase();
        const data=rows(kind).filter(r=>{
            const programs=programIDs(kind,r);
            return (inactive.checked||active(r))&&(kind==='academic_periods'||matchesDepartment(kind,r,department.value))&&(!programFiltered||!program.value||(program.value==='unassigned'?!programs.length:programs.includes(Number(program.value))))&&(!yearFiltered||matchesYear(kind,r,year.value,program.value==='unassigned'?'':program.value))&&(!term||Object.values(r).some(v=>typeof v==='string'&&v.toLocaleLowerCase().includes(term)));
        });
        table(root.querySelector('#record-table'),{columns:recordColumns(kind),rows:data,initialPage:filters.page||1,onPageChange:p=>{filters.page=p;},emptyTitle:'No matching records',emptyText:'Change the filters, include inactive records, or add a record.',onAction:(action,r)=>{
            if(action==='edit')editRecord(kind,r);else if(action==='delete')deleteRecord(kind,r);else if(action==='faculty'&&kind==='subjects')manageSubjectFaculty(r.id);else if(action==='sections'&&kind==='subjects')subjectSectionDetail(r.id);else if(action==='view'){if(kind==='curricula')curriculumDetail(r.id,year.value);if(kind==='sections')sectionDetail(r.id);}
        }});
    }saveFilters();render();
    const focus=Number(params?.get('focus'));
    if(focus && record(kind,focus) && !(state.user.role==='scheduler' && scopeReadOnly)){
        queueMicrotask(()=>{
            const cleaned=new URLSearchParams(params);cleaned.delete('focus');
            history.replaceState(null,'','#'+page+'?'+cleaned.toString());
            if(kind==='curricula') curriculumDetail(focus);
            else if(kind==='sections') sectionDetail(focus);
            else editRecord(kind,record(kind,focus));
        });
    }
}
// Developer-only batch creation. The UI expands A-M convenience ranges;
// the server validates every code and inserts the whole batch transactionally.
export function batchCreateSections(){
    if(state.user.role!=='developer'){toast('Batch creation is for Developers only.','warning');return;}
    const savedProgram=Number(ui.context.program||0);
    const currentProgram=record('programs',savedProgram);
    const initialYear=Number(ui.context.year||1);
    const d=dialog({title:'Batch add sections',description:'Create multiple sections for one program and year. Advisers remain unassigned and may be added later.',wide:true,body:`<form id="sections-batch-form" class="stack"><div class="form-grid"><div class="field"><label>Program *</label><div id="batch-program"></div></div><div class="field"><label>Year level *</label><div id="batch-year"></div></div><div class="field field-wide"><label for="batch-codes">Section codes *</label><textarea id="batch-codes" rows="5" required maxlength="2000" placeholder="A, B, C, D, E&#10;or A-M&#10;or BSIT-1A, BSIT-1B"></textarea><small class="help">Separate codes using commas or new lines. A-M creates A through M. Program/year combinations may reuse codes used in another year.</small></div></div><p class="small muted" id="batch-preview" role="status">No sections entered yet.</p><p class="form-error" role="alert" hidden></p></form>`,actions:[{label:'Cancel',run:m=>m.close()},{label:'Create sections',id:'save',icon:'plus',tone:'primary',run:save}]});
    const program=dropdown({options:[{value:'',label:'Choose program'},...options('programs')],value:currentProgram?String(currentProgram.id):'',label:'Batch section program',onChange:preview});
    const year=dropdown({options:yearOptions().slice(1),value:String(initialYear),label:'Batch section year',onChange:preview});
    d.body.querySelector('#batch-program').append(program.el);d.body.querySelector('#batch-year').append(year.el);
    const input=d.body.querySelector('#batch-codes');input.addEventListener('input',preview);
    function codes(){
        const tokens=input.value.split(/[,\n;]+/).map(x=>x.trim()).filter(Boolean);
        const out=[];
        for(const token of tokens){const range=token.match(/^([A-Z])\s*-\s*([A-Z])$/i);
            if(range){let from=range[1].toUpperCase().charCodeAt(0),to=range[2].toUpperCase().charCodeAt(0);
                if(from>to)throw Error('Use ascending letter ranges such as A-M.');
                for(let i=from;i<=to;i++)out.push(String.fromCharCode(i));
            }else out.push(token);
        }
        if(out.length>100)throw Error('A batch can contain at most 100 sections.');
        if(new Set(out.map(c=>c.toLocaleLowerCase())).size!==out.length)throw Error('The batch contains repeated section codes.');
        return out;
    }
    function preview(){
        const target=d.body.querySelector('#batch-preview');
        try{const selected=codes();const existing=rows('sections').filter(x=>x.program_id===Number(program.value)&&x.year_level===Number(year.value));
            const duplicates=selected.filter(c=>existing.some(s=>s.code.toLowerCase()===c.toLowerCase()));
            target.textContent=selected.length?`${selected.length} section(s) in ${label('programs',program.value)} / ${yearLabel(year.value)}${duplicates.length?' — already exists: '+duplicates.join(', '):''}. Advisers: none.`:'No sections entered yet.';
            target.classList.toggle('warning-text',duplicates.length>0);d.controls.save.disabled=!selected.length||!program.value||!!duplicates.length;
        }catch(err){target.textContent=err.message;d.controls.save.disabled=true;}
    }
    async function save(m,b){
        let selected;try{selected=codes();}catch(err){toast(err.message,'warning');return;}
        if(!program.value||!selected.length){toast('Choose a program, year and section codes.','warning');return;}
        m.setBusy(true);setBusy(b,true,'Creating...');
        try{const result=await api('/developer/sections/batch',{method:'POST',body:{program_id:Number(program.value),year_level:Number(year.value),codes:selected,size:50},revision:state.revision});
            rememberContext({program:program.value,department:record('programs',program.value)?.department_id||'',year:year.value});
            await refresh();m.setBusy(false);m.close();toast(`${result.added} section(s) created. Advisers remain unassigned.`);
        }catch(err){m.setBusy(false);setBusy(b,false);const output=m.body.querySelector('.form-error');output.hidden=false;output.textContent=err.message;}
    }
    d.body.querySelector('form').onsubmit=ev=>{ev.preventDefault();save(d,d.controls.save);};
    preview();
}
export function renderAcademic(root,params){renderRecords(root,{page:'academic',title:'Academic foundation',description:'Map subjects to curriculum years. Manage sections and their advisers in one place.',selected:params.get('tab'),params,kinds:[{value:'departments',label:'Departments'},{value:'programs',label:'Programs'},{value:'subjects',label:'Subjects'},{value:'curricula',label:'Curricula'},{value:'academic_periods',label:'Academic periods'},{value:'sections',label:'Sections'}]});}
export function renderFacilities(root,params){
    root.innerHTML=heading('Buildings & learning spaces','Manage buildings and field areas first, then the classrooms, laboratories and teaching spaces belonging to each facility.',button('Add building',{icon:'plus',tone:'primary',action:'add-building'}),'FACILITIES')+`<div class="facility-intro"><div>${pill(rows('buildings').length+' buildings / field areas','subtle')} ${pill(rows('rooms').length+' rooms / laboratories','subtle')}<p class="muted small">Rooms retain their own record if a building is deleted. Unassigned rooms can be re-linked to a new building; occupancy conflicts remain protected.</p></div>${button('Add room / lab',{icon:'plus',action:'add-room'})}</div><div class="panel facility-toolbar"><div class="search">${icon('search')}<input type="search" id="facility-search" placeholder="Find building, room or laboratory..." aria-label="Search facilities"></div><div id="facility-department"></div><label class="check"><input type="checkbox" id="facility-inactive"> Include inactive</label></div><div class="facility-buildings" id="facility-buildings"></div>`;
    root.querySelector('[data-action=add-building]').onclick=()=>editRecord('buildings');
    root.querySelector('[data-action=add-room]').onclick=()=>editRecord('rooms');
    const filters=ui.facilities;
    const dept=dropdown({options:options('departments',{all:true,empty:'All departments'}),value:filters.department||'',label:'Facilities department',onChange:()=>{filters.department=dept.value;rememberContext({department:dept.value});render();}});
    root.querySelector('#facility-department').append(dept.el);
    const facilitySearch=root.querySelector('#facility-search'),facilityInactive=root.querySelector('#facility-inactive');
    facilitySearch.value=filters.search||'';facilityInactive.checked=!!filters.inactive;
    facilitySearch.oninput=()=>{filters.search=facilitySearch.value;render();};
    facilityInactive.onchange=()=>{filters.inactive=facilityInactive.checked;render();};
    // Retain each building's expansion state across data refreshes.
    const collapsed=new Set(filters.collapsed||rows('buildings').slice(1).map(b=>b.id));
    function saveExpanded(){filters.collapsed=[...collapsed];}
    const output=root.querySelector('#facility-buildings');
    output.addEventListener('click',ev=>{
        const btn=ev.target.closest('[data-action][data-id]');if(!btn)return;
        const id=Number(btn.dataset.id),action=btn.dataset.action;
        if(action==='toggle'){collapsed.has(id)?collapsed.delete(id):collapsed.add(id);saveExpanded();render();return;}
        if(action==='add-room'){editRecord('rooms',null,{building_id:id,department_id:record('buildings',id)?.department_id||null});return;}
        if(action==='edit-building')editRecord('buildings',record('buildings',id));
        if(action==='delete-building')deleteRecord('buildings',record('buildings',id));
        if(action==='edit-room')editRecord('rooms',record('rooms',id));
        if(action==='delete-room')deleteRecord('rooms',record('rooms',id));
    });
    function render(){
        const search=root.querySelector('#facility-search').value.trim().toLowerCase();
        const showInactive=root.querySelector('#facility-inactive').checked;
        const buildings=rows('buildings').filter(b=>{
            const rooms=rows('rooms').filter(r=>r.building_id===b.id&&(!dept.value||r.department_id===Number(dept.value))&& (showInactive||active(r)));
            const match=!search||[b.name,b.code,...rooms.flatMap(r=>[r.code,r.name,r.kind])].some(x=>String(x||'').toLowerCase().includes(search));
            return (showInactive||active(b))&&(!dept.value||b.department_id===Number(dept.value)||rooms.length>0)&&match;
        }).sort((a,b)=>String(a.code||a.name).localeCompare(String(b.code||b.name)));
        output.innerHTML=buildings.length?buildings.map(b=>{
            const children=rows('rooms').filter(r=>r.building_id===b.id&&(!dept.value||r.department_id===Number(dept.value))&&(showInactive||active(r))&&(!search||[r.code,r.name,r.kind].some(x=>String(x||'').toLowerCase().includes(search))||[b.code,b.name].some(x=>String(x||'').toLowerCase().includes(search)))).sort((a,c)=>String(a.code).localeCompare(String(c.code)));
            const labs=children.filter(r=>r.kind==='laboratory').length;
            const isClosed=!search&&collapsed.has(b.id);
            return `<article class="facility-building panel" data-building="${b.id}"><div class="facility-building-header"><button type="button" class="facility-expand" data-action="toggle" data-id="${b.id}" aria-expanded="${!isClosed}" aria-label="${isClosed?'Expand':'Collapse'} ${e(b.name)}">${icon('facilities')}</button><div class="facility-building-name"><span class="eyebrow">${e(b.kind==='field'?'FIELD AREA':'BUILDING / AREA')}</span><h2>${e(b.name||b.code)}</h2><p>${e(b.code||'')} · ${e(b.department_id?label('departments',b.department_id):'Shared facility')}</p></div><div class="facility-building-metrics">${pill(children.length+' rooms','subtle')} ${pill(labs+' labs',labs?'':'neutral')} ${pill(active(b)?'Active':'Inactive',active(b)?'':'neutral')}</div><div class="cell-actions"><button type="button" class="button small" data-action="add-room" data-id="${b.id}">${icon('plus')} Add room</button><button type="button" class="icon-button" data-action="edit-building" data-id="${b.id}" aria-label="Edit building">${icon('edit')}</button><button type="button" class="icon-button danger" data-action="delete-building" data-id="${b.id}" aria-label="Delete building">${icon('trash')}</button><button type="button" class="icon-button facility-chevron" data-action="toggle" data-id="${b.id}" aria-label="${isClosed?'Expand':'Collapse'} rooms">${icon('chevron')}</button></div></div>${isClosed?'':`<div class="facility-room-list">${children.length?children.map(r=>`<div class="facility-room" data-room="${r.id}"><span class="room-type-icon">${icon(r.kind==='laboratory'?'layers':'academic')}</span><div class="facility-room-title"><strong>${e(r.code)}</strong><span>${e(r.name||r.kind)}</span></div><div class="facility-room-meta">${pill(r.kind==='laboratory'?'Laboratory':r.kind==='classroom'?'Classroom':r.kind,'subtle')} <span>${e(r.department_id?label('departments',r.department_id):'Shared')}</span></div><div class="facility-room-meta">${r.capacity?e(r.capacity+' capacity'):'Capacity unspecified'}<span>${e(clock(r.start_minute))}–${e(clock(r.end_minute))}</span></div>${pill(active(r)?'Active':'Inactive',active(r)?'':'neutral')}<div class="cell-actions"><button type="button" class="icon-button" data-action="edit-room" data-id="${r.id}" aria-label="Edit ${e(r.code)}">${icon('edit')}</button><button type="button" class="icon-button danger" data-action="delete-room" data-id="${r.id}" aria-label="Delete ${e(r.code)}">${icon('trash')}</button></div></div>`).join(''):`<div class="facility-empty">${e(search?'No rooms match this search.':'No rooms assigned to this building yet.')} <button type="button" class="text-link" data-action="add-room" data-id="${b.id}">Add a room / lab →</button></div>`}</div>`}</article>`;
        }).join(''):empty('No matching buildings','Add a building or field area first, or try adjusting the search and department filters.',button('Add building',{action:'new-building',tone:'primary',icon:'plus'}));
        // Deleting a building clears room.building_id; do not hide those rooms.
        // Keep them visible and editable until a replacement building is assigned.
        const orphanRooms=rows('rooms').filter(room=>!room.building_id&&(showInactive||active(room))&&(!dept.value||room.department_id===Number(dept.value))&&(!search||[room.code,room.name,room.kind].some(x=>String(x||'').toLowerCase().includes(search))));
        if(orphanRooms.length){
            output.insertAdjacentHTML('beforeend',`<article class="facility-building panel facility-unassigned"><div class="facility-building-header"><div class="facility-building-name"><span class="eyebrow">NEEDS REASSIGNMENT</span><h2>Rooms without a building</h2><p>The original room records remain. Choose Edit to assign each room to a building.</p></div><div class="facility-building-metrics">${pill(orphanRooms.length+' unassigned','warning')}</div></div><div class="facility-room-list">${orphanRooms.map(room=>`<div class="facility-room" data-room="${room.id}"><span class="room-type-icon">${icon('facilities')}</span><div class="facility-room-title"><strong>${e(room.code)}</strong><span>${e(room.name||room.kind)}</span></div><div class="facility-room-meta">${pill(room.kind,'subtle')}<span>${e(room.department_id?label('departments',room.department_id):'Shared')}</span></div><div class="facility-room-meta">${e((room.capacity||0)+' capacity')}</div>${pill('Building unassigned','warning')}<div class="cell-actions"><button type="button" class="button small" data-action="edit-room" data-id="${room.id}">${icon('edit')} Assign building</button></div></div>`).join('')}</div></article>`);
        }
        output.querySelector('[data-action=new-building]')?.addEventListener('click',()=>editRecord('buildings'));
    }
    render();
    const focus=Number(params?.get('focus'));
    const requested=params?.get('tab')==='rooms'?'rooms':'buildings';
    if(focus && record(requested,focus))queueMicrotask(()=>{
        history.replaceState(null,'','#facilities');
        editRecord(requested,record(requested,focus));
    });
}
export function manageSubjectFaculty(subjectID, after=()=>{}) {
    const subject=record('subjects',subjectID);
    if(!subject)return;
    const links=rows('faculty_subjects').filter(x=>x.subject_id===subjectID);
    const selected=links.map(x=>x.faculty_id);
    const locked=rows('teaching_assignments').filter(x=>x.subject_id===subjectID).map(x=>x.faculty_id);
    const revision=state.revision;
    pickRecords('faculty',{
        title:'Eligible faculty / '+subject.code,
        description:'Select faculty members who can teach '+subject.code+'. Search by name, department, or program. This does not force a section assignment. To choose a specific section, use Faculty > Sections.',
        multiple:true, selected, locked,
        confirmLabel:'Save eligible instructors',
        onChoose:async facultyIDs=>{
            await api('/subjects/'+subjectID+'/faculty',{method:'POST',body:{faculty_ids:facultyIDs},revision});
            await refresh();
            toast('Eligible instructors for '+subject.code+' saved.');
            after();
        }
    });
}

export function subjectSections(subjectID){
    return rows('sections').filter(s=>active(s)&&requiredSubjects(s).some(x=>x.subject?.id===subjectID));
}
export function subjectSectionDetail(subjectID){
    const subject=record('subjects',subjectID);if(!subject)return;
    const d=dialog({title:'Sections / '+subject.code,description:'Sections whose chosen curriculum and year include this subject. Faculty eligibility alone does not reserve a section.',wide:true,body:'<div class="panel flush" data-subject-section-table></div>',actions:[{label:'Close',run:m=>m.close()}]});
    const entries=subjectSections(subjectID);
    table(d.body.querySelector('[data-subject-section-table]'),{rows:entries,columns:[
        {title:'Section',render:r=>display(r.code,yearLabel(r.year_level)+' / '+label('programs',r.program_id))},
        {title:'Curriculum (by semester)',render:r=>{const links=rows('curriculum_subject_years').filter(m=>m.subject_id===subjectID&&m.year_level===r.year_level);const curricula=rows('curricula').filter(c=>active(c)&&c.program_id===r.program_id&&links.some(m=>m.curriculum_id===c.id));return e(curricula.map(c=>c.name+(c.semester?' / '+c.semester:'')).join(' · ')||'Not mapped');}},
        {title:'Instructor',render:r=>{const link=rows('teaching_assignments').find(x=>x.section_id===r.id&&x.subject_id===subjectID);return link?pill(label('faculty',link.faculty_id)):'Eligible pool / not fixed';}},
        {title:'Details',render:r=>button('Review section',{small:true,action:'section'})}
    ],emptyTitle:'No linked sections',emptyText:'First add this subject to an existing curriculum and year.',onAction:(a,r)=>{if(a==='section'){d.close();sectionDetail(r.id);}}});
}
const curriculumSemesters = [
    {value:'1st Semester',label:'First semester'},
    {value:'2nd Semester',label:'Second semester'},
    {value:'Summer',label:'Summer'},
    {value:'Others',label:'Other / special semester'}
];

// One curriculum is created for a program + year + semester. Subjects are
// selected afterwards and stored only through curriculum_subject_years links.
export function createCurriculum(){
    const d=dialog({title:'Create curriculum',description:'Choose where this curriculum belongs. Save it first, then select its subjects.',body:`<div class="curriculum-setup">
        <div class="curriculum-setup-steps"><span class="setup-step current">1 · Curriculum details</span><span class="setup-step">2 · Add subjects</span></div>
        <div class="form-grid curriculum-setup-grid">
          <div class="field"><label id="curr-department-label">1. Department <span class="required">*</span></label><div data-create-dept></div></div>
          <div class="field"><label>2. Program <span class="required">*</span></label><div data-create-program></div></div>
          <div class="field"><label>3. Year level <span class="required">*</span></label><div data-create-year></div></div>
          <div class="field"><label>4. Semester <span class="required">*</span></label><div data-create-semester></div></div>
        </div>
        <div data-create-status aria-live="polite" class="curriculum-setup-status"></div>
        <p class="help">Subjects stay in the main Subjects list. Saving only creates the curriculum; you choose which subjects to link next.</p>
        <p class="form-error" role="alert" hidden></p>
    </div>`,actions:[{label:'Cancel',run:m=>m.close()},{label:'Save & add subjects',tone:'primary',icon:'check',id:'save',run:save}]});
    const dept=dropdown({options:options('departments',{empty:'Select department'}),label:'Department',onChange:()=>{
        program.setOptions(options('programs',{empty:'Select program',predicate:p=>p.department_id===Number(dept.value)}));
        program.value='';rememberContext({department:dept.value,program:''});update();
    }});
    const program=dropdown({options:[{value:'',label:'Select program'}],label:'Program',onChange:()=>{rememberContext({program:program.value});update();}});
    const year=dropdown({options:[{value:'',label:'Select year level'},...yearOptions().slice(1)],label:'Year level',onChange:update});
    const semester=dropdown({options:[{value:'',label:'Select semester'},...curriculumSemesters],label:'Semester',onChange:update});
    const context=ui.context;
    if(context.department){dept.value=context.department;program.setOptions(options('programs',{empty:'Select program',predicate:p=>p.department_id===Number(dept.value)}));}
    if(context.program && record('programs',context.program)?.department_id===Number(dept.value))program.value=context.program;
    if(context.year)year.value=context.year;
    d.body.querySelector('[data-create-dept]').append(dept.el);
    d.body.querySelector('[data-create-program]').append(program.el);
    d.body.querySelector('[data-create-year]').append(year.el);
    d.body.querySelector('[data-create-semester]').append(semester.el);
    const status=d.body.querySelector('[data-create-status]'),error=d.body.querySelector('.form-error');
    function update(){
        const ready=!!(dept.value && program.value && year.value && semester.value);
        const duplicate=ready?rows('curricula').find(c=>active(c)&&c.program_id===Number(program.value)&&c.year_level===Number(year.value)&&c.semester===semester.value):null;
        d.controls.save.disabled=!ready||!!duplicate;
        status.innerHTML=duplicate?notice('This program already has an active curriculum for the selected year and semester. Open its Subjects list instead of creating a duplicate.','warning'):
            ready?notice('Ready: '+label('programs',program.value)+' · '+yearLabel(year.value)+' · '+semester.value+'. Save, then choose subjects.','info'):
            '<p class="help">Complete all four fields to continue.</p>';
        return !duplicate&&ready;
    }
    update();
    async function save(){
        if(d.busy||!update())return;
        const p=record('programs',program.value),y=Number(year.value),sem=semester.value;
        if(!p||p.department_id!==Number(dept.value))return;
        const name=p.code+' / '+yearLabel(y)+' / '+sem;
        d.setBusy(true);setBusy(d.controls.save,true,'Saving curriculum...');error.hidden=true;
        try{
            const created=await api('/records/curricula',{method:'POST',revision:state.revision,body:{program_id:p.id,name,year_level:y,semester:sem,active:1}});
            await refresh();d.setBusy(false);d.close();toast('Curriculum created. Choose the subjects to link.');
            pickCurriculumSubjects(created.id,y,()=>curriculumDetail(created.id));
        }catch(err){d.setBusy(false);setBusy(d.controls.save,false);error.textContent=err.message;error.hidden=false;update();}
    }
}

// Reusable subjects are never copied. Creating the mapping does not alter the
// independent subject record or its faculty eligibility.
function pickCurriculumSubjects(curriculumID,yearLevel,after=()=>{}){
    const c=record('curricula',curriculumID);if(!c)return;
    const y=Number(yearLevel);if(c.year_level&&c.year_level!==y){toast("Choose the curriculum's saved year level.",'warning');return;}
    const mapped=rows('curriculum_subject_years').filter(m=>m.curriculum_id===c.id&&m.year_level===y).map(m=>m.subject_id);
    const elsewhere=new Map();
    for(const m of rows('curriculum_subject_years')){
        if(m.year_level===y || record('curricula',m.curriculum_id)?.program_id!==c.program_id)continue;
        const list=elsewhere.get(m.subject_id)||new Set();list.add(m.year_level);elsewhere.set(m.subject_id,list);
    }
    const revision=state.revision;
    pickRecords('subjects',{title:'Add subjects · '+curriculumName(c.name),multiple:true,selected:mapped,locked:mapped,
        department:record('programs',c.program_id)?.department_id||'',
        confirmLabel:'Save subject links',description:yearLabel(y)+(c.semester?' · '+c.semester:'')+'. Select reusable subjects. Existing mappings are locked; original subjects will not be duplicated.',
        onChoose:async ids=>{
            const overlap=ids.filter(id=>!mapped.includes(id)&&elsewhere.has(id));
            if(overlap.length){
                const proceed=await confirm({title:'Subject appears in another year',message:'These subjects also appear in another year of this program: '+overlap.map(id=>label('subjects',id)+' ('+[...elsewhere.get(id)].map(yearLabel).join(', ')+')').join('; ')+'. You may still link them here.',word:'CONTINUE',label:'Link anyway',tone:'warning',action:async()=>{}});
                if(!proceed)return false;
            }
            const result=await api('/curricula/'+c.id+'/members',{method:'POST',body:{year_level:y,subject_ids:ids},revision});
            await refresh();toast(result.added+' subject(s) linked.');after();
        }
    });
}

export function addCurriculumMembers(curriculumID=null,after=()=>{},initialYear='1'){
    const selected=record('curricula',curriculumID),p=record('programs',selected?.program_id);
    if(selected?.year_level>0){pickCurriculumSubjects(selected.id,selected.year_level,after);return;}

    const d=dialog({title:'Add curriculum subjects',description:'Select department, program, existing curriculum, year, and subjects. Year-level ownership is checked within the program.',wide:true,body:'<div class="curriculum-member-layout"><div class="form-grid curriculum-member-form-grid"><div class="field"><label>Department</label><div data-member-dept></div></div><div class="field"><label>Program</label><div data-member-program></div></div><div class="field"><label>Existing curriculum</label><div data-member-curriculum></div></div><div class="field"><label>Year level</label><div data-member-year></div></div></div><div data-year-conflict class="curriculum-member-status" role="status"></div><p class="help curriculum-member-help">Only an existing curriculum can receive subjects. Unmapped subjects remain Unassigned.</p></div>',actions:[{label:'Cancel',run:m=>m.close()},{label:'Choose subjects',tone:'primary',icon:'book',id:'choose',run:choose}]});
    const dept=dropdown({options:options('departments',{all:true,empty:'Select department'}),value:p?.department_id||'',label:'Member department',onChange:()=>{program.setOptions(options('programs',{all:true,empty:'Select program',predicate:r=>r.department_id===Number(dept.value)}));program.value='';updateCurricula();}});
    const program=dropdown({options:options('programs',{all:true,empty:'Select program',predicate:r=>r.department_id===Number(dept.value)}),value:p?.id||'',label:'Member program',onChange:updateCurricula});
    const curriculum=dropdown({options:[],value:curriculumID||'',label:'Existing curriculum',onChange:updateYears});
    const year=dropdown({options:yearOptions().slice(1),value:initialYear||'1',label:'Member year level',onChange:updateWarning});
    d.body.querySelector('[data-member-dept]').append(dept.el);d.body.querySelector('[data-member-program]').append(program.el);d.body.querySelector('[data-member-curriculum]').append(curriculum.el);d.body.querySelector('[data-member-year]').append(year.el);
    const otherOwners=(cid,y)=>{
        const c=record('curricula',cid);if(!c)return [];
        if(c.year_level>0)return [...new Set(rows('curricula').filter(other=>other.id!==cid&&active(other)&&other.program_id===c.program_id&&other.year_level>0&&other.year_level===Number(y)&&other.semester===c.semester).map(other=>other.id))];
        return [...new Set(rows('curriculum_subject_years').filter(m=>m.curriculum_id!==cid&&m.year_level===Number(y)&&record('curricula',m.curriculum_id)?.year_level===0&&record('curricula',m.curriculum_id)?.program_id===c.program_id&&active(record('curricula',m.curriculum_id))).map(m=>m.curriculum_id))];
    };
    function updateWarning(){
        const cid=Number(curriculum.value),y=Number(year.value),other=otherOwners(cid,y);
        const conflict=d.body.querySelector('[data-year-conflict]');
        if(!cid){conflict.innerHTML=notice('Choose an existing curriculum before selecting year-level subjects.','info');d.controls.choose.disabled=true;return;}
        if(other.length){conflict.innerHTML=notice(`${yearLabel(y)} is also active in ${other.map(id=>label('curricula',id)).join('; ')}. This program/year has overlapping curriculum groups. New memberships are locked until you resolve the conflicting curriculum ownership. Existing subjects and sections are preserved.`, 'warning','Year conflict / action required');d.controls.choose.disabled=true;}
        else{const count=rows('curriculum_subject_years').filter(m=>m.curriculum_id===cid&&m.year_level===y).length;conflict.innerHTML=notice(count?`${yearLabel(y)} already exists in this curriculum with ${count} subjects. Add more subjects to this same year; there is no need to create another year.`:`${yearLabel(y)} has no members in this curriculum. You can add subjects.`, 'info');d.controls.choose.disabled=false;}
    }
    function updateYears(){
        const cid=Number(curriculum.value),current=year.value,c=record('curricula',cid);
        if(c?.year_level>0){
            // Scope is fixed when this curriculum is created; never let a mapper
            // silently create a membership under another year.
            year.setOptions([{value:String(c.year_level),label:yearLabel(c.year_level)+' / '+c.semester}]);
            year.value=String(c.year_level);
        }else{
            year.setOptions(yearOptions().slice(1).map(opt=>{
               const owners=otherOwners(cid,opt.value);
               return {...opt,label:owners.length?opt.label+' / already used in '+owners.map(id=>label('curricula',id)).join(', '):opt.label,disabled:owners.length>0};
            }));year.value=current;
        }
        updateWarning();
    }
    function updateCurricula(){const vals=options('curricula',{all:true,empty:'Select existing curriculum',predicate:r=>active(r)&&r.program_id===Number(program.value)});curriculum.setOptions(vals);if(!vals.some(x=>x.value===curriculum.value))curriculum.value='';updateYears();}
    updateCurricula();if(curriculumID)curriculum.value=String(curriculumID);updateYears();
    function choose(){const cid=Number(curriculum.value),y=Number(year.value),revision=state.revision;if(!cid||otherOwners(cid,y).length)return;
        const mapped=rows('curriculum_subject_years').filter(m=>m.curriculum_id===cid&&m.year_level===y).map(m=>m.subject_id);
        const programID=record('curricula',cid)?.program_id;
        const usedInOtherYears=rows('curriculum_subject_years').filter(m=>m.year_level!==y&&record('curricula',m.curriculum_id)?.program_id===programID);
        const elsewhere=new Map();for(const m of usedInOtherYears){const v=elsewhere.get(m.subject_id)||new Set();v.add(m.year_level);elsewhere.set(m.subject_id,v);}
        pickRecords('subjects',{title:'Add subjects / '+yearLabel(y),multiple:true,selected:mapped,locked:mapped,department:dept.value,confirmLabel:'Add selected subjects',description:'Already-mapped members remain selected. A subject used in a different year of the same program gives a warning, not an automatic block.',onChoose:async ids=>{
            const overlapping=ids.filter(sid=>elsewhere.has(sid)&&!mapped.includes(sid));
            if(overlapping.length){
                const ok=await confirm({title:'Subject appears in another year',message:'These subjects already appear in a different year of this program: '+overlapping.map(id=>label('subjects',id)+' ('+[...elsewhere.get(id)].map(yearLabel).join(', ')+')').join('; ')+'. This is a warning, not a restriction.',word:'CONTINUE',label:'Add anyway',tone:'warning',action:async()=>{}});
                if(!ok)throw new Error('Selection was not saved; review the repeated subject-year mappings.');
            }
            const result=await api('/curricula/'+cid+'/members',{method:'POST',body:{year_level:y,subject_ids:ids},revision});await refresh();toast(result.added+' subject(s) added.');d.close();after();
        }});
    }
}

export function curriculumDetail(id,initialYear=''){
    const c=record('curricula',id);if(!c)return;
    const d=dialog({title:curriculumName(c.name),description:c.year_level?`${yearLabel(c.year_level)} · ${c.semester}. Subjects are linked here, not copied.`:'Legacy curriculum: grouped by year, with existing source semester labels preserved.',wide:true,body:`<div class="row between curriculum-actions"><div data-curriculum-year></div>${button('Add subjects to year',{icon:'plus',tone:'primary',action:'map'})}</div><div data-curriculum-table class="panel flush"></div>`,actions:[{label:'Close',run:m=>m.close()}]});
    const year=dropdown({options:c.year_level?[{value:String(c.year_level),label:yearLabel(c.year_level)}]:yearOptions(),value:c.year_level?String(c.year_level):(initialYear||''),label:'Curriculum year level',onChange:render});d.body.querySelector('[data-curriculum-year]').append(year.el);
    d.body.querySelector('[data-action=map]').onclick=()=>addCurriculumMembers(id,render,year.value||'1');
    function render(){table(d.body.querySelector('[data-curriculum-table]'),{rows:rows('curriculum_subject_years').filter(m=>m.curriculum_id===id&&(!year.value||m.year_level===Number(year.value))),columns:[{title:'Year',render:m=>e(yearLabel(m.year_level))},{title:'Subject',render:m=>display(record('subjects',m.subject_id)?.code,record('subjects',m.subject_id)?.title)},{title:'Weekly contact',render:m=>e(hours(record('subjects',m.subject_id)?.weekly_minutes||0))},{title:'Source semester',render:m=>e(m.source_semester||'Unassigned')},{title:'Actions',render:m=>actions(m,'',true)}],emptyTitle:'No subjects mapped',emptyText:'Add subjects to the appropriate year in this curriculum.',onAction:(action,m)=>action==='edit'?editRecord('curriculum_subject_years',m,{},render):deleteRecord('curriculum_subject_years',m,render)});}render();
}
export function sectionDetail(id){
    const sc=record('sections',id);if(!sc)return;
    const d=dialog({title:sc.code+' / '+yearLabel(sc.year_level),description:'An adviser appointment and a teaching assignment are independent.',wide:true,body:'',actions:[{label:'Close',run:m=>m.close()},{label:'Edit section',tone:'primary',icon:'edit',run:()=>editRecord('sections',record('sections',id),{},render)}]});
    function render(){const s=record('sections',id);d.body.innerHTML=`<div class="detail-grid"><div><small>Program / Year</small><strong>${e(label('programs',s.program_id))} · ${yearLabel(s.year_level)}</strong></div><div><small>Adviser</small><div class="adviser-link-actions">${button(s.adviser_id?label('faculty',s.adviser_id):'Choose adviser',{small:true,action:'adviser',icon:'search'})}${s.adviser_id?button('Unlink adviser',{small:true,action:'unlink-adviser',tone:'danger'}):''}</div></div><div><small>Section bank</small><strong>All academic years and semesters</strong><span class="sub-cell">Curriculum resolved from the period when generating</span></div></div>${notice('Select instructors from faculty eligible for the subject. An adviser may be any active faculty member, including one who teaches no subject here.')}<div data-section-subjects class="panel flush"></div>`;
        d.body.querySelector('[data-action=adviser]').onclick=()=>{const revision=state.revision;pickRecords('faculty',{title:'Choose section adviser',selected:s.adviser_id?[s.adviser_id]:[],onChoose:async fid=>{await api('/records/sections/'+id,{method:'PUT',body:{adviser_id:fid},revision});await refresh();render();toast('Adviser saved.');}});};
        d.body.querySelector('[data-action=unlink-adviser]')?.addEventListener('click',()=>confirm({title:'Unlink section adviser?',message:'Only the adviser relationship will be cleared. The section, faculty member, and all instructor assignments remain.',word:'UNLINK',label:'Unlink adviser',tone:'danger',action:async()=>{const revision=state.revision;await api('/records/sections/'+id,{method:'PUT',body:{adviser_id:null},revision});await refresh();render();toast('Section adviser unlinked.')}}));
        table(d.body.querySelector('[data-section-subjects]'),{columns:[{title:'Required subject',render:x=>display(x.subject?.code,x.subject?.title)},{title:'Weekly contact',render:x=>e(hours(x.subject?.weekly_minutes||0))},{title:'Instructor',render:x=>x.assignment?e(label('faculty',x.assignment.faculty_id)):pill('Unassigned','warning')},{title:'Actions',render:x=>x.assignment?button('Review assignment',{small:true,action:'faculty'}):button('Choose instructor',{small:true,action:'assign',icon:'search'})}],rows:requiredSubjects(s),emptyTitle:'No curriculum subjects',emptyText:'Add subjects to this curriculum and year first.',onAction:(action,x)=>{
            if(action==='faculty'){d.close();navigate('faculty',{profile:x.assignment.faculty_id});return;}
            const revision=state.revision;pickRecords('faculty',{title:'Choose instructor / '+x.subject.code,predicate:f=>rows('faculty_subjects').some(fs=>fs.faculty_id===f.id&&fs.subject_id===x.subject.id),description:'Only faculty eligible for this subject are shown. To include someone else, add the subject through Faculty first.',onChoose:async fid=>{await api('/assignments',{method:'POST',body:{faculty_id:fid,subject_id:x.subject.id,section_ids:[s.id]},revision});await refresh();render();toast('Instructor assigned.');}});
        }});
    }render();
}
