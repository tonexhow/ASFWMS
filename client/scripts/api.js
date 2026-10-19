// HTTP requests, CSRF headers, optimistic edit revisions and safe downloads.
let csrf = '';
export function setCSRF(token) { csrf = token || ''; }
export class APIError extends Error {
    constructor(message, code, status) { super(message); this.code = code; this.status = status; }
}
export async function api(path, { method = 'GET', body, revision, redirect = true, blob = false, signal } = {}) {
    const headers = { 'Accept': 'application/json' };
    if (method !== 'GET') {
        headers['Content-Type'] = 'application/json';
        headers['X-ASFWMS-Request'] = '1';
        if (csrf)
            headers['X-CSRF-Token'] = csrf;
    }
    if (revision !== undefined)
        headers['If-Match'] = String(revision);
    let response;
    try {
        response = await fetch('/api' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin', signal });
    }
    catch (e) {
        if (e.name === 'AbortError')
            throw e;
        throw new APIError('Cannot reach ASFWMS. Check that the Go server is still running.', 'NETWORK', 0);
    }
    if (!response.ok) {
        let data;
        try {
            data = await response.json();
        }
        catch {
            data = {};
        }
        const err = new APIError(data.error?.message || `Request failed (${response.status}).`, data.error?.code || 'REQUEST', response.status);
        if (response.status === 401 && redirect) {
            location.assign('/access');
        }
        throw err;
    }
    if (blob)
        return { blob: await response.blob(), filename: response.headers.get('Content-Disposition')?.match(/filename=([^;]+)/)?.[1] || 'ASFWMS_backup.db' };
    return response.json();
}
export function downloadBlob(blob, filename) { const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = filename; document.body.append(a); a.click(); a.remove(); setTimeout(() => URL.revokeObjectURL(url), 30000); }

// Read real, bounded NDJSON progress from the server. No simulated percentages.
export async function generateStream(body, onProgress=()=>{}, signal) {
    const headers={'Accept':'application/x-ndjson','Content-Type':'application/json','X-ASFWMS-Request':'1'};
    if(csrf)headers['X-CSRF-Token']=csrf;
    const response=await fetch('/api/generate/stream',{method:'POST',headers,body:JSON.stringify(body),credentials:'same-origin',signal});
    if(!response.ok){let data={};try{data=await response.json();}catch{}if(response.status===401)location.assign('/access');throw new APIError(data.error?.message||'Generation request failed.',data.error?.code||'GENERATION',response.status);}
    if(!response.body)throw new Error('Streaming is unavailable in this browser. Use a current desktop or mobile browser.');
    const reader=response.body.getReader(),decoder=new TextDecoder();let buffer='',run=null;
    function accept(line){if(!line.trim())return;const event=JSON.parse(line);if(event.type==='error')throw new APIError(event.error?.message||'Generation failed.',event.error?.code||'GENERATION',409);if(event.type==='result')run=event.run;else if(event.type==='progress')onProgress(event);}
    try{while(true){const {done,value}=await reader.read();buffer+=done?decoder.decode():decoder.decode(value,{stream:true});if(buffer.length>10*1024*1024)throw new Error('Generation response exceeds the supported size.');let index;while((index=buffer.indexOf('\n'))!==-1){accept(buffer.slice(0,index));buffer=buffer.slice(index+1);}if(done)break;}if(buffer.trim())accept(buffer);if(!run)throw new Error('Generation connection ended before a preview was saved. Check the run history before retrying.');return run;}finally{reader.releaseLock();}
}
