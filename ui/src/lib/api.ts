const BASE = '';

export type ApprovalPolicy = 'always_allow' | 'ask_session' | 'ask_always';

export type RequestKind = 'execute_accept' | 'secret_request';

export interface SecretRequest {
  id: string;
  kind: RequestKind;
  alias?: string;     // set for secret_request
  aliases?: string[]; // set for execute_accept
  reason: string;
  project_path: string;
  project_name: string;
  project_slug: string;
  status: 'pending' | 'approved' | 'denied';
  created_at: string;
}

export interface BorrowMatch {
  Project: { path: string; name: string; slug: string };
  Meta: { alias: string; policy: ApprovalPolicy };
}

export async function fetchPendingRequests(projectSlug?: string): Promise<SecretRequest[]> {
  const params = projectSlug ? `?project=${projectSlug}` : '';
  const res = await fetch(`${BASE}/api/requests${params}`);
  return res.json();
}

export async function fetchRequest(id: string): Promise<SecretRequest> {
  const res = await fetch(`${BASE}/api/requests/${id}`);
  return res.json();
}

export interface RequestResponse {
  result: any;
  pending: SecretRequest[];
}

export async function approveRequest(id: string, value: string, policy: ApprovalPolicy): Promise<RequestResponse> {
  const res = await fetch(`${BASE}/api/requests/${id}/approve`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value, policy }),
  });
  return res.json();
}

export async function denyRequest(id: string): Promise<RequestResponse> {
  const res = await fetch(`${BASE}/api/requests/${id}/deny`, { method: 'POST' });
  return res.json();
}

export interface ProjectInfo {
  path: string;
  name: string;
  slug: string;
}

export interface CredentialSource {
  type: string;
  file_path?: string;
  file_key?: string;
}

export interface CopyFromRef {
  project_slug: string;
  alias: string;
}

export interface CredentialMeta {
  alias: string;
  policy: ApprovalPolicy;
  source: CredentialSource;
  context?: string;
  value_length: number;
  created_at: string;
  updated_at: string;
}

export async function fetchProjects(): Promise<ProjectInfo[]> {
  const res = await fetch(`${BASE}/api/projects`);
  return res.json();
}

export async function fetchCredentials(slug: string): Promise<Record<string, CredentialMeta>> {
  const res = await fetch(`${BASE}/api/projects/${slug}/credentials`);
  return res.json();
}

export async function updateCredential(slug: string, alias: string, payload: { value?: string; policy?: ApprovalPolicy; copy_value_from?: CopyFromRef }, noEmit = true) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (noEmit) headers['No-Emit'] = 'true';
  return fetch(`${BASE}/api/projects/${slug}/credentials/${encodeURIComponent(alias)}`, {
    method: 'PUT',
    headers,
    body: JSON.stringify(payload),
  });
}

export async function deleteCredential(slug: string, alias: string) {
  return fetch(`${BASE}/api/projects/${slug}/credentials/${encodeURIComponent(alias)}`, {
    method: 'DELETE',
    headers: { 'No-Emit': 'true' },
  });
}

export async function revealCredential(slug: string, alias: string): Promise<string> {
  const res = await fetch(`${BASE}/api/projects/${slug}/credentials/${encodeURIComponent(alias)}/reveal`);
  const data = await res.json();
  return data.value;
}

export async function copyCredential(toSlug: string, fromSlug: string, alias: string) {
  return fetch(`${BASE}/api/projects/${toSlug}/credentials/copy`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ from_project_slug: fromSlug, alias }),
  });
}

export async function createCredential(
  slug: string,
  payload: {
    alias: string;
    value?: string;
    policy: ApprovalPolicy;
    context?: string;
    copy_from?: CopyFromRef;
  }
) {
  const res = await fetch(`${BASE}/api/projects/${slug}/credentials`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  if (res.status === 409) {
    throw new Error('duplicate');
  }
  return res;
}

export async function searchCredentials(alias: string, excludeProject: string): Promise<BorrowMatch[]> {
  const params = new URLSearchParams({ alias, exclude_project: excludeProject });
  const res = await fetch(`${BASE}/api/credentials/search?${params}`);
  return res.json();
}

export async function updateCredentialContext(slug: string, alias: string, context: string) {
  return fetch(`${BASE}/api/projects/${slug}/credentials/${encodeURIComponent(alias)}/context`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ context }),
  });
}

// --- Sessions ---

export interface ActiveSession {
  project_slug: string;
  project_path: string;
  project_name: string;
  last_seen: string;
}

export async function fetchSessions(): Promise<ActiveSession[]> {
  const res = await fetch(`${BASE}/api/sessions`);
  return res.json();
}

// --- Notes ---

export interface Note {
  alias: string;
  body: string;
  created_at: string;
  updated_at: string;
}

export async function fetchNotes(slug: string): Promise<Note[]> {
  const res = await fetch(`${BASE}/api/projects/${slug}/notes`);
  return res.json();
}

export async function setNote(slug: string, alias: string, body: string) {
  return fetch(`${BASE}/api/projects/${slug}/notes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ alias, body }),
  });
}

export async function deleteNote(slug: string, alias: string) {
  return fetch(`${BASE}/api/projects/${slug}/notes/${encodeURIComponent(alias)}`, {
    method: 'DELETE',
  });
}
