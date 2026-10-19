// Shared loaded records, reference labels and hash-route state.
import { api, setCSRF } from './api.js';
export const state = { data: {}, specs: {}, runs: [], revision: 0, dataset: '', user: {}, loaded: false };
export const events = new EventTarget();
// UI state survives soft data refreshes and route changes, never a full browser
// reload. This contains no academic data or credentials.
export const ui = { academic: {}, faculty: {}, facilities: {}, context: {} };
export function rememberContext(next = {}) {
    for (const [key, value] of Object.entries(next)) if (value !== undefined) ui.context[key] = String(value || '');
}
export function currentContext() { return ui.context; }

export async function load() { const first=!state.loaded; const data=await api('/bootstrap'); Object.assign(state,data,{loaded:true}); setCSRF(state.user.csrf_token); if(first&&state.user.role==='scheduler'){ui.context.department=String(state.user.department_id);ui.faculty.department=String(state.user.department_id);} return state; }
export function canEdit(kind,item){if(state.user.role!=='scheduler')return true;if(['departments','academic_periods','curricula','curriculum_subject_years'].includes(kind))return false;if(item&&['faculty_considerations','faculty_work_times','faculty_preferences','faculty_subjects'].includes(kind))return record('faculty',item.faculty_id)?.department_id===state.user.department_id;return item?departmentOf(kind,item)===state.user.department_id:true;}
export function ownDepartment(){return state.user.role==='scheduler'?String(state.user.department_id||''):'';}
export async function refresh() { await load(); events.dispatchEvent(new Event('refresh')); }
export const rows = k => state.data[k] || [];
export const record = (k, id) => rows(k).find(r => r.id === Number(id));
// Existing curriculum names may contain a legacy 'Year 1' label. Keep the
// stored name unchanged while using the same ordinal style in every screen.
function displayYearLevels(text) {
    return String(text||'').replace(/\bYear\s+([1-6])\b/gi,(_,n)=>yearLabel(Number(n)));
}
export function curriculumName(name='') {
    const match=String(name).match(/^(.+?)\s*\/\s*Scanned loading\s+(\d{4})\s*-\s*(.+)$/i);
    if (!match) return displayYearLevels(name);
    const group=match[3].replace(new RegExp('^'+match[1].replace(/[.*+?^${}()|[\]\\]/g,'\\$&')+'\\s*','i'),'').trim() || match[3];
    return displayYearLevels(match[1]+' / '+group+' ('+match[2]+')');
}
export function label(k, id) { const r = record(k, id); if (!r)
    return id ? 'Removed record' : 'Not assigned'; if (k === 'subjects')
    return `${r.code} - ${r.title}`; if (k === 'curricula')
    return curriculumName(r.name); if (k === 'academic_periods')
    return r.label; return r.code || r.name || String(r.id); }
export function fullLabel(k, id) { const r = record(k, id); if (!r)
    return id ? 'Removed record' : 'Not assigned'; if (r.code && r.name && r.name !== r.code)
    return `${r.code} - ${r.name}`; return label(k, id); }
export function departmentOf(k, r) { if (k === 'departments')
    return r.id; if (r.department_id !== undefined)
    return r.department_id || 0; if (k === 'sections' || k === 'curricula')
    return record('programs', r.program_id)?.department_id || 0; return 0; }
export const active = r => r.active === undefined || r.active === 1;
export function yearLabel(year) {
    const n = Number(year);
    if (!Number.isInteger(n) || n < 1) return 'Unknown year';
    const suffix = n % 100 >= 11 && n % 100 <= 13 ? 'th' : ({1:'st',2:'nd',3:'rd'}[n % 10] || 'th');
    return n + suffix + ' year';
}
export function options(k, { all = false, predicate = () => true, empty = null } = {}) { let out = rows(k).filter(r => (all || active(r)) && predicate(r)).map(r => ({ value: String(r.id), label: fullLabel(k, r.id) + (r.active === 0 ? ' [inactive]' : '') })); if (empty !== null)
    out.unshift({ value: '', label: empty }); return out; }
// A Section is a reusable master record. Without a selected period show the
// union of linked year-level subjects; with a period show only the curriculum
// that belongs to its semester (scoped curriculum first, legacy as fallback).
export function curriculumFor(section, periodID = '') {
    const period = record('academic_periods', periodID);
    if (!period) return null;
    const normalize = v => String(v || '').toLowerCase().replace(/\s+semester/g, '').replace(/^(first|1st|1)$/, '1').replace(/^(second|2nd|2)$/, '2').replace(/\s+/g, '').trim();
    const matches = rows('curricula').filter(c => active(c) && c.program_id === section.program_id && c.year_level === section.year_level && normalize(c.semester) === normalize(period.semester));
    if (matches.length === 1) return matches[0];
    if (matches.length > 1) return null;
    const legacy = rows('curricula').filter(c => active(c) && c.program_id === section.program_id && !c.year_level && !c.semester && rows('curriculum_subject_years').some(m => m.curriculum_id === c.id && m.year_level === section.year_level));
    return legacy.length === 1 ? legacy[0] : null;
}
export function requiredSubjects(section, periodID = '') {
    const curr = periodID ? [curriculumFor(section, periodID)].filter(Boolean) : rows('curricula').filter(c => active(c) && c.program_id === section.program_id && (!c.year_level || c.year_level === section.year_level));
    const ids = new Set(curr.map(c => c.id));
    const seen = new Set();
    return rows('curriculum_subject_years').filter(m => ids.has(m.curriculum_id) && m.year_level === section.year_level && !seen.has(m.subject_id) && seen.add(m.subject_id)).map(m => ({ mapping:m, subject:record('subjects',m.subject_id), assignment:rows('teaching_assignments').find(a=>a.section_id===section.id&&a.subject_id===m.subject_id) }));
}
export function navigate(page, params = {}) { const query = new URLSearchParams(params); location.hash = page + (query.size ? '?' + query : ''); }
export function route() { const [page = 'overview', query = ''] = location.hash.slice(1).split('?'); return { page: page || 'overview', params: new URLSearchParams(query) }; }
