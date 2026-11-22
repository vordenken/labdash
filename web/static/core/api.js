// Thin wrappers around the labdash JSON API and the SSE stream.

async function request(method, url, body) {
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  let data = null;
  try { data = await res.json(); } catch { /* empty body */ }
  if (!res.ok) {
    const err = new Error((data && data.error) || `${res.status} ${res.statusText}`);
    err.status = res.status;
    throw err;
  }
  return data;
}

export const api = {
  session: () => request('GET', '/api/session'),
  login: (password) => request('POST', '/api/auth/login', { password }),
  logout: () => request('POST', '/api/auth/logout'),
  layout: () => request('GET', '/api/layout'),
  getConfig: () => request('GET', '/api/config'),
  putConfig: (cfg) => request('PUT', '/api/config', cfg),
  completeSetup: (cfg) => request('PUT', '/api/setup', cfg),
  containers: () => request('GET', '/api/docker/containers'),
  containerAction: (name, action) =>
    request('POST', `/api/docker/${encodeURIComponent(name)}/${action}`),
};

// Opens the SSE stream and dispatches named events to handlers.
// Reconnects automatically (EventSource does this natively).
export function stream(handlers) {
  const es = new EventSource('/api/stream');
  for (const [name, fn] of Object.entries(handlers)) {
    es.addEventListener(name, (ev) => {
      try { fn(JSON.parse(ev.data)); } catch (e) { console.error('bad event', name, e); }
    });
  }
  es.onerror = () => handlers.__error && handlers.__error();
  es.onopen = () => handlers.__open && handlers.__open();
  return es;
}
