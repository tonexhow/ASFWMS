// Searchable selection dialogs shared by every faculty/adviser/subject input.
import { rows, record, label, fullLabel, active, departmentOf, ui, yearLabel, ownDepartment, state } from './store.js';
import { e, days, dialog, dropdown, table, pill, notice, setBusy } from './components.js';
import { icon } from './icons.js';
export const yearOptions = (unassigned = false) => [{ value: '', label: 'All year levels' }, ...[1,2,3,4,5,6].map(n=>({value:String(n),label:yearLabel(n)})), ...(unassigned ? [{value:'unassigned',label:'Unassigned'}] : [])];
export function subjectPrograms(id) {
    return [...new Set(rows('curriculum_subject_years').filter(m=>m.subject_id===id).map(m=>record('curricula',m.curriculum_id)?.program_id).filter(Boolean))];
}
export function subjectYears(id, program = '') {
    return [...new Set(rows('curriculum_subject_years').filter(m=>m.subject_id===id && (!program || record('curricula',m.curriculum_id)?.program_id===Number(program))).map(m=>m.year_level))].sort((a,b)=>a-b);
}
export function facultyPrograms(id) {
    return [...new Set([
        ...rows('teaching_assignments').filter(a=>a.faculty_id===id).map(a=>record('sections',a.section_id)?.program_id),
        ...rows('faculty_subjects').filter(a=>a.faculty_id===id).flatMap(a=>subjectPrograms(a.subject_id))
    ].filter(Boolean))];
}
export const programIDs = (kind,r) => kind==='subjects' ? subjectPrograms(r.id) : kind==='faculty' ? facultyPrograms(r.id) : r.program_id ? [r.program_id] : [];
export function matchesDepartment(kind,r,id) {
    if (!id) return true;
    if (departmentOf(kind,r)===Number(id)) return true;
    if(['subjects','faculty'].includes(kind)&&(!r.department_id||record('departments',r.department_id)?.code?.toUpperCase()==='GENED'))return true;
    return kind==='subjects' && subjectPrograms(r.id).some(pid=>record('programs',pid)?.department_id===Number(id));
}
export function matchesYear(kind,r,year,program='') {
    if (!year) return true;
    if (kind==='sections') return r.year_level===Number(year);
    const levels = kind==='subjects' ? subjectYears(r.id,program) : [...(r.year_level?[r.year_level]:[]),...rows('curriculum_subject_years').filter(m=>m.curriculum_id===r.id).map(m=>m.year_level)];
    return year==='unassigned' ? !levels.length : levels.includes(Number(year));
}
export function programOptions(department='', unassigned=false) {
    return [{value:'',label:'All programs'},...rows('programs').filter(p=>active(p)&&(!department||p.department_id===Number(department))).map(p=>({value:String(p.id),label:p.code})),...(unassigned?[{value:'unassigned',label:'Unassigned'}]:[])];
}
export function dayValues(value='1,2,3,4,5') {
    const translated={weekdays:'1,2,3,4,5',saturday:'6',sunday:'7',weekdays_saturday:'1,2,3,4,5,6'}[value] || value;
    return String(translated).split(',').filter(Boolean).map(Number);
}
export function subjectDays(s) {
    return dayValues(s.allowed_days).map(n=>days[n]?.slice(0,3)||'?').join(', ')+(s.saturday_fallback?' / Sat fallback':'');
}
export function pickRecords(kind, { title='', description='', multiple=false, selected=[], predicate=()=>true, onChoose=()=>{}, confirmLabel='Use selection', department, program, year, locked=[], memoryKey='default', readOnly=false }={}) {
    const plural=kind==='faculty'?'faculty':'subjects';
    const yearEnabled=kind==='subjects'&&memoryKey!=='faculty-subjects';
    const saved=(ui.pickers||(ui.pickers={}))[memoryKey]||{};
    department=department??saved.department??ownDepartment()??'';
    program=program??saved.program??'';
    year=yearEnabled?(year??saved.year??''):'';
    let currentPage=1;
    const choices=new Set(selected.map(Number)),lockedIDs=new Set(locked.map(Number));
    const base=()=>rows(kind).filter(r=>(active(r)||choices.has(r.id))&&predicate(r)).sort((a,b)=>fullLabel(kind,a.id).localeCompare(fullLabel(kind,b.id)));
    const d=dialog({title:title || (multiple?'Select ':'Choose ')+plural,description:description || (kind==='faculty'?'Search by name, department or program. Program links come from eligible subjects and teaching assignments.':'Search by code or name, department, program or curriculum year. Unmapped subjects remain Unassigned.'),wide:true,
        body:`<div class="picker-filters"><div class="search">${icon('search')}<input type="search" data-picker-search placeholder="Search ${plural} by name${kind==='subjects'?' or code':''}..." aria-label="Search ${plural}"></div><div data-picker-department></div><div data-picker-program></div>${yearEnabled?'<div data-picker-year></div>':''}<div data-picker-status></div></div><p class="small muted" data-picker-summary role="status"></p><div class="panel flush picker-results"></div><p class="form-error" role="alert" hidden></p>`,
        actions:[{label:'Cancel',run:m=>m.close()},{label:confirmLabel,tone:'primary',icon:'check',id:'choose',run:async(m,b)=>{
            m.setBusy(true);setBusy(b,true,'Applying...');
            try {const result=await onChoose(multiple?[...choices]:[...choices][0]);m.setBusy(false);setBusy(b,false);if(result===false)return;m.close();}
            catch(err){m.setBusy(false);setBusy(b,false);const p=m.body.querySelector('.form-error');p.textContent=err.message;p.hidden=false;updateCount();}
        }}]});
    const dd=dropdown({options:[{value:'',label:'All departments'},...rows('departments').filter(active).map(r=>({value:String(r.id),label:r.code}))],value:department,label:'Picker department',onChange:()=>{pd.setOptions(programOptions(dd.value,true));if(pd.value!=='unassigned'&&pd.value&&record('programs',pd.value)?.department_id!==Number(dd.value)&&dd.value)pd.value='';draw();}});
    const pd=dropdown({options:programOptions(department,true),value:program,label:'Picker program',onChange:draw});
    const yd=yearEnabled?dropdown({options:yearOptions(true),value:year,label:'Picker year level',onChange:draw}):null;
    d.body.querySelector('[data-picker-department]').append(dd.el);d.body.querySelector('[data-picker-program]').append(pd.el);d.body.querySelector('[data-picker-year]')?.append(yd?.el);
    const search=d.body.querySelector('[data-picker-search]');search.oninput=draw;
    const selection=dropdown({options:[{value:'all',label:'All available records'},{value:'selected',label:'Selected only'},{value:'unselected',label:'Not selected yet'}],value:'all',label:'Selection status',search:false,onChange:draw});
    d.body.querySelector('[data-picker-status]').append(selection.el);
    function updateCount(){d.controls.choose.disabled=readOnly||(!multiple&&!choices.size);}
    function draw(reset=true){
        ui.pickers[memoryKey]={department:dd.value,program:pd.value,year:yd?.value||''};
        if(reset!==false)currentPage=1;
        const query=search.value.trim().toLocaleLowerCase();
        const data=base().filter(r=>{
            const programs=programIDs(kind,r);
            return (selection.value==='all'||(selection.value==='selected' ? choices.has(r.id) : !choices.has(r.id)))&&matchesDepartment(kind,r,dd.value)&&(!pd.value||(pd.value==='unassigned'?!programs.length:programs.includes(Number(pd.value))))&&(!yearEnabled||matchesYear(kind,r,yd.value,pd.value==='unassigned'?'':pd.value))&&(!query||fullLabel(kind,r.id).toLocaleLowerCase().includes(query));
        });
        d.body.querySelector('[data-picker-summary]').textContent=`${data.length} matching ${plural} / ${choices.size} selected${multiple?' (selections stay when filters change)':''}`;
        table(d.body.querySelector('.picker-results'),{pageSize:12,initialPage:currentPage,onPageChange:p=>currentPage=p,rows:data,emptyTitle:'No matching '+plural,emptyText:'Change a filter or search. Unassigned records have no curriculum/program mapping yet.',columns:[
            {title:'Select',render:r=>`<button type="button" class="pick-check ${choices.has(r.id)?'selected':''}" role="${multiple?'checkbox':'radio'}" aria-checked="${choices.has(r.id)}" aria-label="Select ${e(fullLabel(kind,r.id))}" data-action="pick" data-pick-id="${r.id}" ${readOnly||lockedIDs.has(r.id)?'disabled':''}>${icon(choices.has(r.id)?'check':'plus')}</button>`},
            {title:kind==='subjects'?'Subject':'Faculty member',render:r=>`<strong class="main-cell">${e(kind==='subjects'?r.code:r.name)}</strong><span class="sub-cell">${e(kind==='subjects'?r.title:r.employment||'Employment not specified')}</span>${choices.has(r.id)?pill('Selected','subtle'):''} ${lockedIDs.has(r.id)?pill('Required by an explicit assignment','warning'):''}`},
            {title:'Department / program',render:r=>`<span class="main-cell">${e(r.department_id?label('departments',r.department_id):'Unassigned')}</span><span class="sub-cell">${e(programIDs(kind,r).map(id=>record('programs',id)?.code).join(', ')||'Unassigned')}</span>`},
            ...(yearEnabled?[{title:'Year',render:r=>e(subjectYears(r.id).map(y=>yearLabel(y)).join(', ')||'Unassigned')}]:[])
        ],onAction:(a,r)=>{if(readOnly||a!=='pick'||lockedIDs.has(r.id))return;if(multiple){choices.has(r.id)?choices.delete(r.id):choices.add(r.id);}else{choices.clear();choices.add(r.id);}draw(false);d.body.querySelector(`[data-pick-id="${r.id}"]`)?.focus();}});
        updateCount();
    }
    draw();
    return d;
}
export function relationPicker(kind,{value=null,required=false,label:fieldLabel='',predicate=()=>true}={}) {
    let current=value?Number(value):null;
    const el=document.createElement('div');el.className='relation-picker';
    const b=document.createElement('button');b.type='button';b.className='select-button';b.setAttribute('aria-label',fieldLabel||'Choose '+kind);
    b.onclick=()=>pickRecords(kind,{selected:current?[current]:[],predicate,onChoose:id=>{current=id;paint();el.dispatchEvent(new Event('change'));}});
    el.append(b);
    const clear=document.createElement('button');clear.type='button';clear.className='icon-button';clear.innerHTML=icon('close');clear.setAttribute('aria-label','Clear '+(fieldLabel||kind));clear.onclick=()=>{current=null;paint();el.dispatchEvent(new Event('change'));};if(!required)el.append(clear);
    function paint(){b.innerHTML=`<span class="select-label">${e(current?fullLabel(kind,current):'Choose '+(kind==='faculty'?'faculty member':'subject'))}</span>${icon('search')}`;clear.hidden=!current;}
    paint();return{el,button:b,getValue:()=>current,setValue:id=>{current=id;paint();}};
}
