import {api} from './api.js';
import {rows,state,refresh} from './store.js';
import {e,button,heading,pill,notice,dialog,confirm,toast,dropdown} from './components.js';
import {deleteWithImpact} from './linked-delete.js';
import {icon} from './icons.js';

const view={table:'generation_runs',offset:0,selected:0,selectedIDs:new Set(),all:false};
function clearBulkSelection(){view.selectedIDs.clear();view.all=false;}

const readonly=new Set(['id','created_at','updated_at','committed_at','created_by']);
const bytes=(v)=>v>0?(v/1024/1024/1024>=1?(v/1024/1024/1024).toFixed(1)+' GB':(v/1024/1024).toFixed(1)+' MB'):'Unavailable';
const fmt=(v)=>v===null||v===undefined?'—':typeof v==='string'&&v.length>110?v.slice(0,110)+'…':String(v);
function devEdit(recordData,action,data){
 const edit=action==='update';
 const allowed=(data.editable_fields||[]).filter(f=>!readonly.has(f));
 const fields=allowed.map(f=>({name:f,column:data.columns.find(c=>c.name===f),value:recordData?.[f]}));
 const form=`<form id="db-editor" class="stack"><div class="dev-edit-grid">${fields.map(f=>{
   const isNum=/INT|REAL|NUM|FLOAT|DOUBLE/i.test(f.column?.type||'');
   const max=String(f.value||'').length>200;
   return `<label class="field"><span>${e(f.name)} ${f.column?.notnull?'*':''}</span>${max?`<textarea data-field="${e(f.name)}" rows="3">${e(String(f.value??''))}</textarea>`:`<input data-field="${e(f.name)}" ${isNum?'inputmode="decimal"':'type="text"'} value="${e(String(f.value??''))}" placeholder="${f.column?.notnull?'Required':'Blank = empty string'}">`}</label>`;
 }).join('')}</div><details><summary>Advanced JSON editor</summary><p class="muted small">Editable fields only. Cannot change record ID or generated timestamps.</p><textarea id="dev-advanced" class="dev-json" rows="8" spellcheck="false">${e(JSON.stringify(Object.fromEntries(fields.map(f=>[f.name,f.value??null])),null,2))}</textarea><label class="check"><input id="use-dev-advanced" type="checkbox"> Use JSON instead of the fields above</label></details><p class="form-error" hidden role="alert"></p></form>`;
 const modal=dialog({title:`${edit?'Edit':'Add'} ${data.table}${edit?' #'+recordData.id:''}`,wide:true,description:'Direct database edits are audited and validated. IDs, creation dates and generation timestamps cannot be edited.',body:form,actions:[{label:'Cancel',run:x=>x.close()},{label:edit?'Save record':'Insert record',tone:'primary',id:'save',run:save}]});
 modal.body.querySelector('#db-editor').onsubmit=event=>{event.preventDefault();save()};
 function fieldValues(){
  if(modal.body.querySelector('#use-dev-advanced').checked) {const x=JSON.parse(modal.body.querySelector('#dev-advanced').value);if(!x||Array.isArray(x)||typeof x!=='object')throw Error('Enter a JSON object');if(Object.keys(x).some(k=>!allowed.includes(k)))throw Error('JSON contains a protected column');return x;}
  const values={};for(const input of modal.body.querySelectorAll('[data-field]')){
   const name=input.dataset.field,column=data.columns.find(c=>c.name===name),raw=input.value;
   if(!edit&&raw==='')continue;
   if(/INT|REAL|NUM|FLOAT|DOUBLE/i.test(column?.type||'')&&raw!==''&&!isNaN(Number(raw)))values[name]=Number(raw);
   else if(raw==='NULL'&&!column?.notnull)values[name]=null;
   else values[name]=raw;
  }return values;
 }
 async function save(){let values;const error=modal.body.querySelector('.form-error');error.hidden=true;
  try {values=fieldValues()}catch(err){error.textContent=err.message;error.hidden=false;return}
  await confirm({title:`Apply ${action} to ${data.table}?`,message:`This modifies SQLite ${data.table} ${edit?'#'+recordData.id:'as a new record'}. Dependent records remain protected by the database and business rules.`,word:'APPLY',label:'Apply change',tone:'warning',action:async()=>{
   modal.setBusy(true);try{await api('/developer/database',{method:'POST',body:{table:data.table,operation:action,id:recordData?.id||0,values,confirm:'APPLY'}});
    modal.setBusy(false);modal.close();clearBulkSelection();await refresh();toast('Database record saved.');await loadDB();
   }catch(err){modal.setBusy(false);error.textContent=err.message;error.hidden=false;throw err}
  }});
 }
}
async function reviewBulkDelete(x){
 const scope=view.all?'all':'selected';
 const ids=view.all?[]:[...view.selectedIDs].sort((a,b)=>a-b);
 if(scope==='selected'&&!ids.length){toast('Select at least one row first.','warning');return;}
 let preview;
 try{preview=await api('/developer/database/bulk-preview',{method:'POST',body:{table:x.table,scope,ids}});}
 catch(err){toast(err.message,'danger');return;}
 const plan=preview.impact;
 const total=preview.selected_count;
 const intro=`Delete ${total} selected ${x.table.replaceAll('_',' ')} record(s)?`;
 const list=(plan.impacts||[]).map(g=>`<li><strong>${e(g.table)}</strong>: ${g.count} row(s) removed${g.table===x.table?' (selected)':''}</li>`).join('');
 const detaches=(plan.detaches||[]).map(g=>`<li><strong>${e(g.table)}.${e(g.field)}</strong>: ${g.count} reference(s) cleared to Unassigned/NULL</li>`).join('');
 const warnings=(plan.warnings||[]).map(t=>`<li>${e(t)}</li>`).join('');
 const extra=`<div class="dev-bulk-impact">
  <p class="muted small">Selected from <strong>${e(x.table)}</strong>. ${scope==='all'?'This includes EVERY row in this table, across all pages.':'Only the selected IDs will be deleted, including selections on other pages.'}</p>
  <div class="dev-bulk-impact-stat"><strong>${plan.total}</strong><span>total rows deleted, including necessary dependent links and affected schedule runs</span></div>
  <details open><summary>Rows that will be deleted</summary><ul>${list||'<li>None</li>'}</ul></details>
  ${detaches?`<details open><summary>Surviving records with references cleared</summary><ul>${detaches}</ul></details>`:''}
  ${warnings?`<details><summary>Other consequences to review</summary><ul>${warnings}</ul></details>`:''}
  ${plan.blocked?'<p class="dev-bulk-blocked">Blocked: a protected account has a relationship that must be resolved first.</p>':''}
  <p class="muted small">Independent subjects, faculty, curricula, programs and sections outside this deletion plan remain. A complete SQLite backup is created before the transaction.</p>
 </div>`;
 if(plan.blocked){toast('Bulk deletion is blocked by protected account relationships. Review the dependency impact.','danger');
  dialog({title:'Bulk deletion blocked',description:intro,wide:true,body:extra,actions:[{label:'Close',run:m=>m.close()}]});return;
 }
 let outcome;
 const approved=await confirm({title:'Bulk delete · '+x.table,word:preview.confirm_phrase,label:scope==='all'?'Delete entire table selection':'Delete selected rows',tone:'danger',message:intro+' This is destructive. The operation will unlink surviving records where safe and release affected schedules.',extra,action:async()=>{
   outcome=await api('/developer/database/bulk-delete',{method:'POST',body:{table:x.table,scope,ids,expected_count:total,fingerprint:plan.fingerprint,revision:plan.revision,confirm:preview.confirm_phrase}});
 }});
 if(approved){clearBulkSelection();view.selected=0;view.offset=0;await refresh();await loadDB();toast(`${outcome.selected_deleted} row(s) deleted. Backup: ${outcome.backup}`);}
}
async function loadDB(){const root=document.querySelector('#developer-db');if(!root)return;
 try{
  const x=await api('/developer/database?'+new URLSearchParams({table:view.table,offset:view.offset}));
  const headers=x.columns.map(c=>c.name),columns=headers.slice(0,7),selected=x.rows.find(r=>r.id===view.selected);
  const allActive=view.all, selectedCount=allActive?x.count:view.selectedIDs.size;
  const pageIDs=x.rows.map(r=>Number(r.id));
  root.innerHTML=`<section class="panel dev-bar"><div class="row between wrap"><div><h3>SQLite records</h3><p class="muted small">Inspect, edit or delete a row. Bulk deletion is available only in this Developer workspace. Account credentials and tokens are never exposed.</p></div><div class="row">${pill(x.count+' rows','subtle')}${x.read_only?pill('Generated schedules: edit disabled','warning'):button('Insert record',{icon:'plus',action:'add'})}</div></div><div id="developer-table-picker" class="dev-table-picker"></div>
  <details class="dev-schema"><summary>Table structure and outgoing relationships (${x.columns.length} columns)</summary><div class="db-relationships"><strong>${e(x.table)}</strong>${(x.foreign_keys||[]).map(f=>`<div class="db-fk-edge"><span>${e(f.from)} → ${e(f.to)}</span><button type="button" class="button small" data-related="${e(f.table)}">Open ${e(f.table)}</button></div>`).join('')||'<p class="muted small">No outgoing keys.</p>'}</div><div class="dev-columns">${x.columns.map(c=>`<div>${e(c.name)} <code>${e(c.type)}</code>${readonly.has(c.name)?' · Read only':''}</div>`).join('')}</div><pre class="schema-code">${e(x.schema)}</pre></details></section>
  <section class="panel dev-bulk-bar"><div class="dev-bulk-bar-intro"><div><strong>Bulk selection · ${e(x.table.replaceAll('_',' '))}</strong><p class="muted small">Select rows on this page, keep selections across pages, or select all ${x.count} rows in this table.</p></div><strong class="dev-bulk-count" id="dev-bulk-count" aria-live="polite">${selectedCount} selected${allActive?' · Entire table':''}</strong></div>
   <div class="dev-bulk-actions">${button('Select page',{small:true,action:'bulk-page',disabled:x.rows.length===0})}${button('Select all in table ('+x.count+')',{small:true,action:'bulk-all',disabled:x.count===0})}${button('Clear selection',{small:true,action:'bulk-clear',disabled:selectedCount===0})}${button('Review bulk delete',{small:true,icon:'trash',tone:'danger',action:'bulk-delete',disabled:selectedCount===0})}</div>
   <p class="muted small" id="dev-bulk-note">${allActive?'Every row in this table is selected, including pages not shown. Clear selection to return to individual checkboxes.':'No changes are made by selecting rows; the delete action requires an impact review and confirmation.'}</p></section>
  <div class="panel flush"><div class="responsive-table"><table class="data-table compact" aria-label="Database table rows"><thead><tr><th scope="col" class="dev-checkbox-cell"><input type="checkbox" id="dev-select-page" aria-label="Select or deselect all rows on this page" ${allActive?'checked disabled':''}></th>${columns.map(k=>`<th>${e(k)}</th>`).join('')}<th>Manage</th></tr></thead><tbody>${x.rows.map((r,index)=>`<tr class="dev-record-row ${selected?.id===r.id?'is-selected':''} ${allActive||view.selectedIDs.has(Number(r.id))?'is-bulk-selected':''}" data-db-row="${index}"><td class="dev-checkbox-cell"><input type="checkbox" data-bulk-row="${index}" aria-label="Include record #${r.id} in bulk selection" ${allActive||view.selectedIDs.has(Number(r.id))?'checked':''} ${allActive?'disabled':''}></td>${columns.map(k=>`<td><span class="db-value" title="${e(String(r[k]??''))}">${e(fmt(r[k]))}</span></td>`).join('')}<td><div class="cell-actions">${!x.read_only?`<button type="button" class="button small" data-edit="${index}">Edit</button>`:''}<button type="button" class="button small danger" data-remove="${index}">Delete</button></div></td></tr>`).join('')||`<tr><td colspan="${columns.length+2}">This table has no records.</td></tr>`}</tbody></table></div><div class="row between toolbar wrap"><p class="muted small">Showing ${x.rows.length} of ${x.count}. Selections persist when changing pages within this table.</p><div class="row">${button('Previous',{action:'prev',small:true,disabled:view.offset===0})}${button('Next',{action:'next',small:true,disabled:view.offset+75>=x.count})}</div></div></div><div id="developer-selection"></div>`;
  const picker=dropdown({options:x.tables.map(name=>({value:name,label:name.replaceAll('_',' ')})),value:x.table,label:'Choose SQLite table',onChange:table=>{view.table=table;view.offset=0;view.selected=0;clearBulkSelection();loadDB()}});
  root.querySelector('#developer-table-picker').append(picker.el);
  root.querySelector('[data-action=add]')?.addEventListener('click',()=>devEdit({},'insert',x));
  root.querySelector('[data-action=prev]').onclick=()=>{view.offset=Math.max(0,view.offset-75);view.selected=0;loadDB()};
  root.querySelector('[data-action=next]').onclick=()=>{view.offset+=75;view.selected=0;loadDB()};
  for(const btn of root.querySelectorAll('[data-related]'))btn.onclick=()=>{if(x.tables.includes(btn.dataset.related)){view.table=btn.dataset.related;view.offset=0;view.selected=0;clearBulkSelection();loadDB()}};
  function updateBulkControls(){const count=view.all?x.count:view.selectedIDs.size;
    root.querySelector('#dev-bulk-count').textContent=count+' selected'+(view.all?' · Entire table':'');
    root.querySelector('[data-action=bulk-clear]').disabled=count===0;
    root.querySelector('[data-action=bulk-delete]').disabled=count===0;
    const page=root.querySelector('#dev-select-page');
    page.checked=view.all||pageIDs.every(id=>view.selectedIDs.has(id));
    page.indeterminate=!view.all&&pageIDs.some(id=>view.selectedIDs.has(id))&&!page.checked;
    page.disabled=view.all||pageIDs.length===0;
    root.querySelector('#dev-bulk-note').textContent=view.all?'Every row in this table is selected, across all pages. Clear selection to select individual records.':'No changes are made by selecting rows. Delete requires a preview, typed confirmation and database backup.';
    for(const tr of root.querySelectorAll('[data-db-row]')){const id=Number(x.rows[Number(tr.dataset.dbRow)].id);tr.classList.toggle('is-bulk-selected',view.all||view.selectedIDs.has(id));const ck=tr.querySelector('[data-bulk-row]');ck.checked=view.all||view.selectedIDs.has(id);ck.disabled=view.all;}
  }
  root.querySelector('[data-action=bulk-page]').onclick=()=>{view.all=false;pageIDs.forEach(id=>view.selectedIDs.add(id));updateBulkControls()};
  root.querySelector('[data-action=bulk-all]').onclick=()=>{view.all=true;view.selectedIDs.clear();updateBulkControls()};
  root.querySelector('[data-action=bulk-clear]').onclick=()=>{clearBulkSelection();updateBulkControls()};
  root.querySelector('[data-action=bulk-delete]').onclick=()=>reviewBulkDelete(x);
  root.querySelector('#dev-select-page').onchange=event=>{if(view.all)return;pageIDs.forEach(id=>event.target.checked?view.selectedIDs.add(id):view.selectedIDs.delete(id));updateBulkControls()};
  for(const checkbox of root.querySelectorAll('[data-bulk-row]'))checkbox.onchange=()=>{if(view.all)return;const id=Number(x.rows[Number(checkbox.dataset.bulkRow)].id);if(checkbox.checked)view.selectedIDs.add(id);else view.selectedIDs.delete(id);updateBulkControls()};
  for(const tr of root.querySelectorAll('[data-db-row]')){
   tr.onclick=event=>{if(event.target.closest('button,input,label'))return;const r=x.rows[Number(tr.dataset.dbRow)];view.selected=r.id;root.querySelectorAll('.dev-record-row').forEach(row=>row.classList.toggle('is-selected',row===tr));drawSelection(r)};
  }
  const remove=r=>deleteWithImpact({table:x.table,id:r.id,title:(['faculty_subjects','curriculum_subject_years','teaching_assignments'].includes(x.table)?'Unlink / review':'Delete / review')+' · '+x.table+' #'+r.id,executor:body=>api('/developer/database',{method:'POST',body:{table:x.table,operation:'delete',id:r.id,values:{},...body}}),after:()=>{clearBulkSelection();loadDB()}});
  for(const btn of root.querySelectorAll('[data-edit]'))btn.onclick=()=>devEdit(x.rows[Number(btn.dataset.edit)],'update',x);
  for(const btn of root.querySelectorAll('[data-remove]'))btn.onclick=()=>remove(x.rows[Number(btn.dataset.remove)]);
  function drawSelection(r){const target=root.querySelector('#developer-selection');if(!target)return;target.innerHTML=`<section class="panel dev-selected"><div class="row between wrap"><div><span class="eyebrow">INSPECTED RECORD</span><h3>${e(x.table)} / #${r.id}</h3></div><div class="row">${!x.read_only?button('Edit this row',{action:'selected-edit',icon:'edit'}):''}${button(['faculty_subjects','curriculum_subject_years','teaching_assignments'].includes(x.table)?'Unlink / review':'Review links & delete',{action:'selected-remove',icon:['faculty_subjects','curriculum_subject_years','teaching_assignments'].includes(x.table)?'link':'trash',tone:'danger'})}</div></div><div class="dev-columns">${headers.map(key=>`<div><strong>${e(key)}</strong><span class="db-value">${e(fmt(r[key]))}</span>${readonly.has(key)?'<span class="muted small">Generated / protected</span>':''}</div>`).join('')}</div></section>`;target.querySelector('[data-action=selected-remove]').onclick=()=>remove(r);target.querySelector('[data-action=selected-edit]')?.addEventListener('click',()=>devEdit(r,'update',x));}
  if(selected)drawSelection(selected);
  updateBulkControls();
 }catch(err){root.innerHTML=notice(err.message,'danger')}
}
function metric(label,value,subtitle,percent=undefined){return `<div class="panel dev-metric"><span class="eyebrow">${e(label)}</span><strong>${e(String(value))}</strong><p class="muted small">${e(subtitle||'')}</p>${Number.isFinite(percent)?`<div class="dev-meter" role="progressbar" aria-valuenow="${Math.min(100,Math.max(0,percent))}" aria-valuemin="0" aria-valuemax="100"><span style="width:${Math.max(0,Math.min(100,percent))}%"></span></div>`:''}</div>`}
async function loadSystem(){const root=document.querySelector('#developer-system');if(!root)return;
 try{const x=await api('/developer/system'),m=x.machine||{},net=x.network||{},used=m.ram_total_bytes-m.ram_available_bytes,diskUsed=m.storage_total_bytes-m.storage_free_bytes;
  const online=!!net.reachable;
  root.innerHTML=`<div class="panel"><div class="row between wrap"><div><h3>Host diagnostics</h3><p class="muted small">Live read-only metrics from this computer, not a cloud server. Values refresh on demand.</p></div>${button('Refresh metrics',{icon:'refresh',action:'system-refresh'})}</div></div>
   <div class="dev-metric-grid">${metric('CPU / PROCESSOR',m.cpu_model||'Unavailable',`${m.cores||'?'} logical processor(s)`,Number(m.cpu_usage_percent))}${metric('CPU UTILIZATION',Number(m.cpu_usage_percent||0).toFixed(1)+'%','Approximate CPU load at last sample',Number(m.cpu_usage_percent||0))}${metric('MEMORY / RAM',m.ram_total_bytes?bytes(used)+' / '+bytes(m.ram_total_bytes):'Unavailable',m.ram_total_bytes?bytes(m.ram_available_bytes)+' available':'Hardware query unavailable',m.ram_total_bytes?100*used/m.ram_total_bytes:undefined)}${metric('STORAGE',m.storage_total_bytes?bytes(diskUsed)+' / '+bytes(m.storage_total_bytes):'Unavailable',m.storage_total_bytes?bytes(m.storage_free_bytes)+' available · '+(m.storage_label||'local drive'):'Disk usage unavailable',m.storage_total_bytes?100*diskUsed/m.storage_total_bytes:undefined)}${metric('GRAPHICS / GPU',(m.gpu||[]).length?(m.gpu||[]).join(' / '):'Not reported','GPU model names only; utilization depends on OS drivers')}${metric('INTERNET',net.status||'Not tested',`${(net.interfaces||[]).length} active non-loopback interface(s) · Test ${net.probe||'unavailable'}`)}</div>
   <div class="grid three">${metric('DATABASE',x.database+' '+x.sqlite_version,`Integrity: ${x.database_integrity} · ${bytes(x.db_bytes)}`)}${metric('GO PROCESS',x.go_version,`${x.go_routines} goroutines · ${x.heap_mb} MB heap`)}${metric('EMAIL / SMTP',x.smtp_configured?'Configured':'Not configured',x.smtp_configured?'Recovery email is configured':'Set SMTP_EMAIL / SMTP_PASS to send recovery emails')}</div>
   ${m.note?notice(m.note,'info'):''}
   <section class="panel"><div class="row between wrap"><div><h3>System & activity logs</h3><p class="muted small">${e(x.log_source||'Audit log')} · Most recent 40 entries · No passwords or reset tokens displayed</p></div><div class="search">${icon('search')}<input type="search" id="dev-log-search" placeholder="Find action, entity, or time..." aria-label="Search system activity logs"></div></div><div class="responsive-table"><table class="data-table compact"><thead><tr><th>Time</th><th>Action</th><th>Entity</th><th>Details</th></tr></thead><tbody id="dev-logs"></tbody></table></div></section>
   <details class="panel dev-schema"><summary>Service configuration and infrastructure</summary><p class="muted">Client HTML/CSS/JS → Go HTTP API → services and scheduler → local SQLite. Access checks are enforced on every API request.</p><ul><li>OS: ${e(x.os)} / ${e(x.arch)}</li><li>Local database path: <code>${e(x.db_path)}</code></li><li>Server time: ${e(x.server_time)}</li></ul><div class="dev-columns">${Object.entries(x.counts||{}).map(([name,count])=>`<div>${e(name)} · ${count}</div>`).join('')}</div></details>`;
  const logEntries=x.logs||[];const search=root.querySelector('#dev-log-search'),body=root.querySelector('#dev-logs');
  function draw(){const q=search.value.toLowerCase();const list=logEntries.filter(it=>[it.action,it.entity,it.created_at,it.detail].some(v=>String(v??'').toLowerCase().includes(q)));body.innerHTML=list.map(it=>`<tr><td>${e(it.created_at)}</td><td>${pill(it.action,'subtle')}</td><td>${e(it.entity)} #${e(it.entity_id)}</td><td>${e(it.detail)}</td></tr>`).join('')||'<tr><td colspan="4">No matching activity logs.</td></tr>'}draw();search.oninput=draw;
  root.querySelector('[data-action=system-refresh]').onclick=()=>loadSystem();
 }catch(err){root.innerHTML=notice(err.message,'danger')}
}
function cleanup(){return `<div class="panel dev-cleanup"><div class="row between"><div><h3>Release test schedules</h3><p class="muted small">Remove reservations for one department and period without deleting curricula, instructors, or academic records.</p></div>${pill('Developer only','warning')}</div><div class="form-grid"><label class="field">Department<div id="cleanup-department-select"></div></label><label class="field">Academic period<div id="cleanup-period-select"></div></label></div><label class="check"><input type="checkbox" id="cleanup-history"> Also delete generation run history</label><div class="row">${button('Clear selected schedule',{icon:'trash',action:'clear',tone:'danger'})}</div></div>`}
export function renderDeveloper(root,params){
 if(state.user.role!=='developer'){root.innerHTML=notice('Developer access is required.','danger');return}
 const tab=params.get('tab')==='system'?'system':'database';
 if(params.get('table')&&tab==='database'){if(view.table!==params.get('table'))clearBulkSelection();view.table=params.get('table');view.offset=0;view.selected=Number(params.get('focus'))||0}
 root.innerHTML=heading('Developer workspace','Inspect data relationships, manage test records with safeguards, and see local system diagnostics.','','SYSTEM ADMINISTRATION')+
 `<div class="stack"><div class="tabs"><a class="tab ${tab==='database'?'active':''}" href="#developer?tab=database">${icon('database')} Database</a><a class="tab ${tab==='system'?'active':''}" href="#developer?tab=system">${icon('developer')} System</a></div><div id="developer-db" ${tab==='database'?'':'hidden'}></div><div id="developer-system" ${tab==='system'?'':'hidden'}></div>${cleanup()}</div>`;
 if(tab==='database')loadDB();else loadSystem();
 const department=dropdown({options:[{value:'',label:'Select department'},...rows('departments').map(d=>({value:String(d.id),label:d.code+' · '+d.name}))],label:'Schedule department'});
 const period=dropdown({options:[{value:'',label:'Select period'},...rows('academic_periods').map(p=>({value:String(p.id),label:p.label}))],label:'Schedule academic period'});
 root.querySelector('#cleanup-department-select').append(department.el);root.querySelector('#cleanup-period-select').append(period.el);
 root.querySelector('[data-action=clear]').onclick=()=>{
  const department_id=Number(department.value),period_id=Number(period.value),delete_history=root.querySelector('#cleanup-history').checked;
  if(!department_id||!period_id){toast('Select both department and academic period.','warning');return}
  confirm({title:'Release committed test schedules?',message:'This clears class reservations for the selected department and period. '+(delete_history?'Generation-run history will also be deleted.':'Runs remain marked as released.'),word:'CLEAR SCHEDULE',label:'Release reservations',tone:'danger',action:async()=>{const result=await api('/developer/clear-schedule',{method:'POST',body:{department_id,period_id,delete_history,confirm:'CLEAR SCHEDULE'}});toast(result.deleted_meetings+' meeting(s) cleared.');await refresh();if(tab==='database')loadDB()}});
 };
}
