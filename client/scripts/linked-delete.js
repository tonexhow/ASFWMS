import {api} from './api.js';
import {state,rows,record,refresh} from './store.js';
import {e,dialog,notice,toast,dropdown} from './components.js';

// Association rows are links, not extra copies of subjects or people.
const relationshipTables=new Set(['curriculum_subject_years','faculty_subjects','teaching_assignments']);
const titles={curriculum_subject_years:'Curriculum subject membership',faculty_subjects:'Faculty subject eligibility',teaching_assignments:'Instructor / section assignment',academic_periods:'Academic period',curricula:'Curriculum',generation_runs:'Generation run',scheduled_meetings:'Committed meeting'};
const titleOf=t=>titles[t]||t.replaceAll('_',' ');
function linkedList(x){
 const groups=(x.impacts||[]).filter(g=>g.table!==x.table||g.count>1);
 const output=[...groups.map(g=>`<section class="dependency-group"><div class="row between"><strong>${e(titleOf(g.table))}</strong><span class="muted small">${g.count} record(s)</span></div><div class="dependency-links">${g.samples.map(s=>s.path?`<a href="${e(s.path)}" class="dependency-link" data-go-linked="${e(s.path)}">${e(s.label)} <span aria-hidden="true">↗</span></a>`:`<span class="dependency-link muted">${e(s.label)}</span>`).join('')}</div>${g.count>g.samples.length?`<p class="muted small">+${g.count-g.samples.length} more records</p>`:''}</section>`),...(x.detaches||[]).map(g=>`<section class="dependency-group"><div class="row between"><strong>${e(titleOf(g.table))} · clear ${e(g.field)}</strong><span class="muted small">${g.count} reference(s)</span></div><div class="dependency-links">${g.samples.map(s=>s.path?`<a href="${e(s.path)}" class="dependency-link" data-go-linked="${e(s.path)}">${e(s.label)} ↗</a>`:`<span class="dependency-link">${e(s.label)}</span>`).join('')}</div></section>`)];
 return `<div class="dependency-groups">${output.join('')}</div>`;
}

/** Remove a relationship, delete an independent record, or delete a parent
 *  while clearing references and association-only rows. Server transactions
 *  recheck the impact and protect independently reusable records. */
export async function deleteWithImpact({table,id,title,after=()=>{},executor=null}){
 let plan;
 try{plan=await api('/records/'+table+'/'+id+'/dependencies');}
 catch(err){toast(err.message,'danger');return;}
 const relationship=relationshipTables.has(table), linked=plan.related>0;
 const canCascade=['developer','administrator'].includes(state.user.role)&&plan.can_delete;
 const transferChoices=table==='academic_periods'?rows('academic_periods').filter(x=>x.id!==id):table==='curricula'?rows('curricula').filter(x=>x.id!==id&&x.program_id===record('curricula',id)?.program_id):[];
 const canMove=canCascade&&linked&&transferChoices.length>0&&[...(plan.impacts||[]),...(plan.detaches||[])].some(g=>g.table==='sections');
 const summary=linked?`${plan.related} connected association(s) or references need review. Delete and unlink keeps independent records, but releases invalid teaching/schedule links.`:relationship?'Only this relationship will be removed. The original subject, faculty, curriculum and section records stay.':'No other records depend on this item. You can delete it without removing linked data.';
 const warnings=(plan.warnings||[]), hasEffects=(plan.unlink_effects||[]).length>0;
 const option=(value,name,detail,{disabled=false,checked=false,danger=false}={})=>`<label class="dependency-choice ${danger?'is-danger':''} ${disabled?'is-disabled':''}"><input type="radio" name="delete-policy" value="${value}" ${disabled?'disabled':''} ${checked?'checked':''}><span class="dependency-choice-text"><strong>${e(name)}</strong><small>${e(detail)}</small></span></label>`;
 const html=`<div class="dependency-dialog">
 <div class="dependency-intro">${notice(summary,linked?'warning':'info')}<p class="muted small">${e(plan.label)} · ${e(titleOf(table))} #${id}</p></div>
 ${linked?`<details class="dependency-detail"><summary>Inspect ${plan.related} linked records and where to resolve them</summary>${linkedList(plan)}</details>`:''}
 ${hasEffects?`<details class="dependency-detail"><summary>Sections affected by unlinking (not deleted)</summary><div class="dependency-groups">${plan.unlink_effects.map(g=>`<section class="dependency-group"><strong>${g.count} section(s) will no longer require this subject</strong><div class="dependency-links">${g.samples.map(s=>`<a class="dependency-link" data-go-linked="${e(s.path)}" href="${e(s.path)}">${e(s.label)} ↗</a>`).join('')}</div></section>`).join('')}</div></details>`:''}
 ${warnings.length?`<div class="dependency-warning-list" id="dependency-cascade-warnings" role="note" hidden><strong>Associations and schedules affected by Delete and unlink</strong><ul>${warnings.map(w=>`<li>${e(w)}</li>`).join('')}</ul></div>`:''}
 ${plan.blocked?notice('Protected user accounts are never automatically deleted. Reassign their department links first.','danger'):''}
 <fieldset class="dependency-options"><legend>Choose exactly one operation</legend>
 ${relationship?option('unlink','Unlink relationship only','Removes this mapping, not its subject, faculty, curriculum or section. Linked schedules or assignments must be resolved first.',{disabled:linked||plan.blocked,checked:!linked}):option('single','Delete this record only','Keeps related records intact; available after its required relationships are resolved.',{disabled:linked||plan.blocked,checked:!linked})}
 ${canMove?option('move','Transfer dependent sections and delete','Move sections to another compatible '+titleOf(table)+' and preserve their instructor assignments.'):''}
 ${linked?option('preserve','Delete record and clear its links','Keep independent records (subjects, sections, faculty, rooms). Clear nullable references, remove relationship-only rows and release schedules that depend on removed links. A backup is created.',{disabled:!canCascade,danger:true}):''}
 </fieldset>
 ${canMove?`<div id="dependency-move-target" class="dependency-transfer" hidden><div class="field"><span class="field-label">Destination ${e(titleOf(table))}</span><div id="dependency-move-picker"></div></div><p class="muted small">The target curriculum must already contain the required year-level subjects. Affected generation runs are released.</p></div>`:''}
 ${linked?`<p class="dependency-guidance">To keep the original records, <strong>open a related record above</strong> and unlink or reassign it first. Deleting a curriculum will not delete its subjects or sections. Sections without a curriculum will need a new assignment before generation.</p>`:''}
 <div class="dependency-confirm"><label class="check"><input id="dependency-ack" type="checkbox"> I have reviewed the affected records and schedules.</label><label class="field" for="dependency-word"><span>Type <strong id="dependency-word-label">${relationship?'UNLINK':'DELETE'}</strong> to confirm</span></label><input id="dependency-word" spellcheck="false" autocomplete="off" placeholder="${relationship?'UNLINK':'DELETE'}"></div>
 <p class="form-error" hidden role="alert"></p></div>`;
 const m=dialog({title:title||'Review record operation',description:'Unlinking a relationship is different from deleting a record and its dependent data.',body:html,wide:true,actions:[{label:'Cancel',run:x=>x.close()},{label:relationship?'Unlink':'Delete record',tone:'danger',id:'delete',run:perform}]});
 const radio=()=>m.body.querySelector('input[name="delete-policy"]:checked')?.value||'';
 let dest=null;
 if(canMove){dest=dropdown({options:[{value:'',label:'Select a destination'},...transferChoices.map(x=>({value:String(x.id),label:String(x.label||x.name||x.code||x.academic_year)}))],label:'Destination',onChange:()=>refreshControls()});m.body.querySelector('#dependency-move-picker').append(dest.el);}
 const checkbox=m.body.querySelector('#dependency-ack'),typed=m.body.querySelector('#dependency-word'),label=m.body.querySelector('#dependency-word-label');
 const words={unlink:'UNLINK',single:'DELETE',preserve:'DELETE AND UNLINK',linked:'DELETE LINKED',move:'MOVE AND DELETE'};
 function refreshControls(){const strategy=radio(),word=words[strategy]||'';
  label.textContent=word||'CHOOSE AN OPERATION';typed.placeholder=word||'Choose an operation first';
  const target=m.body.querySelector('#dependency-move-target');if(target)target.hidden=strategy!=='move';
  const cascadeNotes=m.body.querySelector('#dependency-cascade-warnings');if(cascadeNotes)cascadeNotes.hidden=strategy!=='preserve';
  const ready=!plan.blocked&&!!word&&checkbox.checked&&typed.value.trim()===word&&(strategy!=='move'||!!dest?.value);
  m.controls.delete.disabled=!ready;
  m.controls.delete.textContent=({unlink:'Unlink only',single:'Delete record',preserve:'Delete and unlink',linked:'Delete linked data',move:'Move sections & delete'})[strategy]||'Choose operation';
  m.controls.delete.classList.toggle('danger',['preserve','single','unlink'].includes(strategy));
  m.controls.delete.classList.toggle('primary',strategy==='move');
 }
 m.body.querySelectorAll('input[name="delete-policy"]').forEach(x=>x.onchange=()=>{typed.value='';refreshControls();});
 checkbox.onchange=refreshControls;typed.oninput=refreshControls;refreshControls();
 m.body.querySelectorAll('[data-go-linked]').forEach(a=>a.addEventListener('click',ev=>{ev.preventDefault();const target=a.getAttribute('href');m.close();if(location.hash===target)window.dispatchEvent(new HashChangeEvent('hashchange'));else location.hash=target;}));
 async function perform(){
  const strategy=radio(),phrase=words[strategy];if(!phrase||typed.value.trim()!==phrase||!checkbox.checked)return;
  m.setBusy(true);const error=m.body.querySelector('.form-error');error.hidden=true;
  try{
   const body={confirm:phrase,strategy,fingerprint:plan.fingerprint,revision:plan.revision};
   const result=strategy==='move'?await api('/records/'+table+'/'+id+'/move-dependencies',{method:'POST',body:{target_id:Number(dest.value),...body}}):executor?await executor(body):await api('/records/'+table+'/'+id,{method:'DELETE',body,revision:plan.revision});
   m.setBusy(false);m.close();await refresh();
   toast(result.backup?`Change completed. Safety backup: ${result.backup}`:strategy==='unlink'?'Relationship removed. Original records were preserved.':strategy==='move'?'Sections transferred and old record removed.':'Record deletion completed.');
   after(result);
  }catch(err){m.setBusy(false);error.textContent=err.message;error.hidden=false;error.scrollIntoView({block:'nearest'});refreshControls();}
 }
}
