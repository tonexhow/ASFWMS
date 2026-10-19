// Shared accessible interaction components; no remote UI dependencies.
import { icon } from './icons.js';
export const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
export const e = escapeHTML;
export const clock = n => `${String(Math.floor(Number(n) / 60)).padStart(2, '0')}:${String(Number(n) % 60).padStart(2, '0')}`;
export const parseClock = s => { const [h, m] = String(s).split(':').map(Number); return h * 60 + m; };
export const days = ['', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
export const hours = n => `${(Number(n) / 60).toLocaleString(undefined, { maximumFractionDigits: 2 })} h`;
export const number = n => Number(n || 0).toLocaleString();
export function pill(text, tone = '') { return `<span class="pill ${e(tone)}">${e(text)}</span>`; }
export function button(text, { icon: ico = '', tone = '', small = false, action = '', disabled = false, title = '' } = {}) { return `<button type="button" class="button ${e(tone)} ${small ? 'small' : ''}" ${action ? `data-action="${e(action)}"` : ''} ${disabled ? 'disabled' : ''} ${title ? `data-tip="${e(title)}"` : ''}>${ico ? icon(ico) : ''}${e(text)}</button>`; }
export function iconButton(ico, title, action = '', tone = '') { return `<button type="button" class="icon-button ${e(tone)}" aria-label="${e(title)}" data-tip="${e(title)}" ${action ? `data-action="${e(action)}"` : ''}>${icon(ico)}</button>`; }
export function notice(text, tone = '', title = '') { return `<div class="notice ${e(tone)}">${icon(tone === 'warning' || tone === 'danger' ? 'alert' : 'info')}<div>${title ? `<strong>${e(title)}</strong><p>${e(text)}</p>` : e(text)}</div></div>`; }
export function heading(title, description, action = '', eyebrow = 'WORKSPACE') { return `<div class="page-heading"><div><span class="eyebrow">${e(eyebrow)}</span><h1>${e(title)}</h1><p>${e(description)}</p></div>${action}</div>`; }
export function empty(title, description, action = '', ico = 'layers') { return `<div class="empty-state"><span class="empty-icon">${icon(ico)}</span><h3>${e(title)}</h3><p>${e(description)}</p>${action}</div>`; }
export function stat(title, value, note = '', ico = 'layers') { return `<div class="stat-card"><div class="stat-top"><span>${e(title)}</span>${icon(ico)}</div><div class="stat-value">${e(value)}</div><div class="stat-bottom">${e(note)}</div></div>`; }
export function skeleton() { return `<div class="stack" aria-busy="true" aria-label="Loading workspace"><div class="skeleton line"></div><div class="skeleton-grid">${Array(4).fill('<div class="skeleton card"></div>').join('')}</div><div class="panel skeleton-stack">${Array(5).fill('<div class="skeleton row"></div>').join('')}</div></div>`; }
export function toast(message, tone = '') { let region = document.querySelector('#toast-region'); if (!region) {
    region = document.createElement('div');
    region.id = 'toast-region';
    region.className = 'toast-region';
    region.setAttribute('aria-live', 'polite');
    document.body.append(region);
} const el = document.createElement('div'); el.className = `toast ${tone}`; el.innerHTML = `${icon(tone === 'danger' || tone === 'warning' ? 'alert' : 'check')}<p>${e(message)}</p>${iconButton('close', 'Dismiss message')}`; region.append(el); while (region.children.length > 3)
    region.firstElementChild.remove(); el.querySelector('button').onclick = () => el.remove(); setTimeout(() => el.remove(), tone === 'danger' ? 12000 : 6500); }
export function setBusy(btn, busy, label = 'Working...') { if (busy) {
    btn.dataset.previous = btn.innerHTML;
    btn.disabled = true;
    btn.setAttribute('aria-busy', 'true');
    btn.innerHTML = `<span class="spinner"></span>${e(label)}`;
}
else {
    btn.innerHTML = btn.dataset.previous || btn.innerHTML;
    btn.disabled = false;
    btn.removeAttribute('aria-busy');
} }
let serial = 0, activeSelect = null;
export function dropdown({ options = [], value = '', placeholder = 'Choose...', onChange = () => { }, label = 'Choose an option', search = true, disabled = false } = {}) {
    const wrap = document.createElement('div');
    wrap.className = 'dropdown';
    const id = 'select-' + (++serial);
    wrap.innerHTML = `<button type="button" class="select-button" role="combobox" aria-haspopup="listbox" aria-expanded="false" aria-controls="${id}" aria-label="${e(label)}"><span class="select-label"></span>${icon('chevron')}</button><div class="select-menu" popover="manual"><input type="search" placeholder="Search choices..." aria-label="Search ${e(label)}" autocomplete="off"><div class="select-options" id="${id}" role="listbox" aria-label="${e(label)}"></div></div>`;
    const btn = wrap.querySelector('button'), menu = wrap.querySelector('.select-menu'), input = menu.querySelector('input'), list = menu.querySelector('.select-options');
    let opts = options, current = String(value ?? ''), focused = -1, filtered = [], opened = false;
    btn.disabled = disabled;
    input.hidden = !search;
    function paintLabel() { const choice = opts.find(o => String(o.value) === current); btn.querySelector('.select-label').textContent = choice?.label || placeholder; }
    function paintList() { filtered = opts.filter(o => o.label.toLowerCase().includes(input.value.toLowerCase())); list.innerHTML = filtered.length ? filtered.map((o, i) => `<button type="button" role="option" class="select-option ${i === focused ? 'focused' : ''}" aria-selected="${String(o.value) === current}" aria-disabled="${!!o.disabled}" data-index="${i}" tabindex="-1">${e(o.label)}</button>`).join('') : '<div class="select-empty">No matching choices</div>'; }
    function close(focus = false) { if (!opened)
        return; opened = false; try {
        menu.hidePopover();
    }
    catch { } menu.hidden = true; btn.setAttribute('aria-expanded', 'false'); btn.removeAttribute('aria-activedescendant'); document.removeEventListener('pointerdown', outside, true); window.removeEventListener('resize', position); if (activeSelect === close)
        activeSelect = null; if (focus)
        btn.focus(); }
    function position() { if (!btn.isConnected)
        return close(); const r = btn.getBoundingClientRect(); const width = Math.max(r.width, 220); menu.style.width = `${Math.min(width, window.innerWidth - 24)}px`; menu.style.left = `${Math.min(r.left, window.innerWidth - width - 12) < 12 ? 12 : Math.min(r.left, window.innerWidth - width - 12)}px`; const h = Math.min(330, menu.scrollHeight || 330); menu.style.top = `${r.bottom + h + 10 > window.innerHeight ? Math.max(10, r.top - h - 5) : r.bottom + 5}px`; }
    function outside(ev) { if (!wrap.contains(ev.target))
        close(); }
    function open() { if (btn.disabled)
        return; if (opened)
        return close(true); activeSelect?.(); activeSelect = close; opened = true; input.value = ''; focused = -1; paintList(); menu.hidden = false; menu.showPopover(); btn.setAttribute('aria-expanded', 'true'); position(); document.addEventListener('pointerdown', outside, true); window.addEventListener('resize', position); if (search)
        input.focus();
    else
        btn.focus(); }
    function choose(i) { const o = filtered[i]; if (!o || o.disabled)
        return; current = String(o.value); paintLabel(); close(true); onChange(current, o); wrap.dispatchEvent(new Event('change', { bubbles: true })); }
    function keys(ev) { if (ev.key === 'Escape' && opened) {
        ev.preventDefault();
        ev.stopPropagation();
        close(true);
        return;
    } if (ev.key === 'Tab') {
        close();
        return;
    } if (!opened && ['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(ev.key)) {
        ev.preventDefault();
        open();
        return;
    } if (!opened)
        return; if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(ev.key)) {
        ev.preventDefault();
        let index = ev.key === 'Home' ? 0 : ev.key === 'End' ? filtered.length - 1 : focused + (ev.key === 'ArrowDown' ? 1 : -1);
        index = Math.max(0, Math.min(filtered.length - 1, index));
        focused = index;
        paintList();
        const item = list.children[index];
        if (item) {
            item.id = id + '-active';
            btn.setAttribute('aria-activedescendant', item.id);
            item.scrollIntoView({ block: 'nearest' });
        }
    }
    else if (ev.key === 'Enter' && focused >= 0) {
        ev.preventDefault();
        choose(focused);
    } }
    btn.onclick = open;
    btn.onkeydown = keys;
    menu.onkeydown = keys;
    input.oninput = () => { focused = -1; paintList(); position(); };
    list.onclick = ev => { const b = ev.target.closest('[data-index]'); if (b)
        choose(Number(b.dataset.index)); };
    menu.hidden = true;
    paintLabel();
    return { el: wrap, get value() { return current; }, set value(v) { current = String(v ?? ''); paintLabel(); }, setOptions(next) { opts = next; paintLabel(); if (opened)
            paintList(); }, close, button: btn };
}
export function dialog({ title, description = '', body = '', wide = false, actions = [], onClose } = {}) {
    activeSelect?.();
    const previous = document.activeElement;
    const el = document.createElement('dialog');
    el.className = 'modal' + (wide ? ' wide' : '');
    const id = 'modal-' + (++serial);
    el.setAttribute('aria-labelledby', id);
    el.innerHTML = `<div class="modal-inner"><div class="modal-header"><div><h2 id="${id}">${e(title)}</h2>${description ? `<p>${e(description)}</p>` : ''}</div>${iconButton('close', 'Close dialog')}</div><div class="modal-body">${body}</div><div class="modal-footer"></div></div>`;
    document.body.append(el);
    const footer = el.querySelector('.modal-footer'), controls = {};
    let busy = false;
    const close = () => { if (busy)
        return; activeSelect?.(); el.close(); };
    el.querySelector('.modal-header button').onclick = close;
    el.addEventListener('cancel', ev => { if (busy) {
        ev.preventDefault();
        return;
    } activeSelect?.(); });
    for (const a of actions) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'button ' + (a.tone || '');
        b.innerHTML = (a.icon ? icon(a.icon) : '') + e(a.label);
        b.onclick = () => a.run?.(d, b);
        footer.append(b);
        if (a.id)
            controls[a.id] = b;
    }
    const d = { el, body: el.querySelector('.modal-body'), footer, controls, close, setBusy(value) { busy = value; el.querySelector('.modal-header button').disabled = value; for (const b of footer.querySelectorAll('button'))
            b.disabled = value; }, get busy() { return busy; } };
    el.addEventListener('close', () => { el.remove(); onClose?.(); if (previous?.isConnected)
        previous.focus(); });
    el.showModal();
    return d;
}
export function confirm({ title = 'Confirm action', message, word = 'CONFIRM', label = 'Confirm', tone = 'danger', extra = '', action } = {}) {
    return new Promise(resolve => {
        let completed = false;
        const d = dialog({ title, body: `${notice(message, tone === 'danger' ? 'warning' : tone)}${extra}<div class="stack"><label class="check"><input type="checkbox" id="confirm-check"> I understand the effect of this action.</label><div class="field"><label for="confirm-word">Type ${e(word)} to continue</label><input id="confirm-word" autocomplete="off" spellcheck="false" placeholder="${e(word)}"></div><p class="form-error" role="alert" hidden></p></div>`, actions: [{ label: 'Cancel', run: m => m.close() }, { label, tone, id: 'confirm', run: async (m, b) => { m.setBusy(true); setBusy(b, true); try {
                        await action?.();
                        completed = true;
                        resolve(true);
                        m.setBusy(false);
                        m.close();
                    }
                    catch (err) {
                        const error = m.body.querySelector('.form-error');
                        error.textContent = err.message;
                        error.hidden = false;
                        m.setBusy(false);
                        setBusy(b, false);
                        update();
                    } } }], onClose: () => { if (!completed)
                resolve(false); } });
        const check = d.body.querySelector('#confirm-check'), wordInput = d.body.querySelector('#confirm-word');
        function update() { d.controls.confirm.disabled = !check.checked || wordInput.value !== word; }
        check.onchange = update;
        wordInput.oninput = update;
        update();
    });
}
export function table(container, { columns, rows, pageSize = 20, emptyTitle = 'No records yet', emptyText = 'Add a record to begin.', emptyAction = '', onAction = () => { }, initialPage = 1, onPageChange = () => {} }) { let page = initialPage; const draw = () => { const count = rows.length, pages = Math.max(1, Math.ceil(count / pageSize)); page = Math.min(page, pages); const list = rows.slice((page - 1) * pageSize, page * pageSize); container.innerHTML = count ? `<div class="table-scroll"><table class="data-table"><thead><tr>${columns.map(c => `<th scope="col">${e(c.title)}</th>`).join('')}</tr></thead><tbody>${list.map((r, i) => `<tr data-row="${(page - 1) * pageSize + i}">${columns.map(c => `<td>${c.render ? c.render(r) : e(r[c.key])}</td>`).join('')}</tr>`).join('')}</tbody></table></div><div class="pagination"><span>${number((page - 1) * pageSize + 1)}-${number(Math.min(page * pageSize, count))} of ${number(count)}</span><div class="row">${button('Previous', { small: true, action: 'table-prev', disabled: page === 1 })}<span>Page ${page} / ${pages}</span>${button('Next', { small: true, action: 'table-next', disabled: page === pages })}</div></div>` : empty(emptyTitle, emptyText, emptyAction); }; container.onclick = ev => { const b = ev.target.closest('[data-action]'); if (!b)
    return; if (b.dataset.action === 'table-prev') {
    page--;
    onPageChange(page);draw();
    return;
} if (b.dataset.action === 'table-next') {
    page++;
    onPageChange(page);draw();
    return;
} const tr = b.closest('[data-row]'); onAction(b.dataset.action, tr ? rows[Number(tr.dataset.row)] : null, b); }; draw(); return { redraw: draw }; }
export function tabs(container, items, value, onChange) { container.innerHTML = items.map(x => `<button type="button" class="tab ${x.value === value ? 'active' : ''}" data-tab="${e(x.value)}" aria-pressed="${x.value === value}">${e(x.label)}${x.count !== undefined ? `<span class="tab-count">${number(x.count)}</span>` : ''}</button>`).join(''); container.onclick = ev => { const b = ev.target.closest('[data-tab]'); if (b)
    onChange(b.dataset.tab); }; }
// Tooltip is a top-layer popover, but it must be positioned in viewport
// coordinates (not the browser popover's default centered position).
export function installTooltips(){
 let tip=null,target=null;
 const hide=()=>{
    if(tip){try{tip.hidePopover();}catch{}tip.remove();tip=null;}
    if(target){target.removeAttribute('aria-describedby');target=null;}
 };
 const position=()=>{
    if(!tip||!target||!target.isConnected){hide();return;}
    const r=target.getBoundingClientRect(),gap=8,pad=10;
    const w=tip.offsetWidth,h=tip.offsetHeight;
    const left=Math.max(pad,Math.min(r.left+r.width/2-w/2,innerWidth-w-pad));
    let top=r.top-h-gap;
    if(top<pad)top=r.bottom+gap;
    if(top+h>innerHeight-pad)top=Math.max(pad,innerHeight-h-pad);
    tip.style.left=left+'px';tip.style.top=top+'px';
 };
 const show=el=>{
    if(!el||!el.isConnected||!el.dataset.tip||el.disabled){hide();return;}
    if(el===target){position();return;}
    hide();target=el;
    tip=document.createElement('div');tip.className='tooltip';tip.id='asfwms-current-tooltip';
    tip.setAttribute('role','tooltip');tip.setAttribute('popover','manual');
    tip.textContent=el.dataset.tip;
    document.body.append(tip);
    tip.style.position='fixed';tip.style.inset='auto';tip.style.margin='0';
    tip.showPopover();el.setAttribute('aria-describedby',tip.id);
    position();
 };
 document.addEventListener('pointerover',ev=>{const el=ev.target.closest?.('[data-tip]');if(el)show(el);else if(!target?.contains(ev.target))hide();});
 document.addEventListener('focusin',ev=>{const el=ev.target.closest?.('[data-tip]');if(el)show(el);else hide();});
 document.addEventListener('pointerout',ev=>{if(target&&!target.contains(ev.relatedTarget))hide();});
 document.addEventListener('click',hide);
 document.addEventListener('close',hide,true);
 document.addEventListener('cancel',hide,true);
 window.addEventListener('scroll',hide,true);
 window.addEventListener('resize',hide);
}
