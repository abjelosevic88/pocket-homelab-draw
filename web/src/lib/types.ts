export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'

export type Kind = 'sketch' | 'mermaid' | 'drawio'

export interface Settings {
  core_configured: boolean
  core_registered?: boolean
  title: string; drawio_url: string; date_format: string; time_format: string; week_start: number
  default_kind: 'ask' | Kind; sketch_grid: boolean; sketch_font: 'hand' | 'normal' | 'code'; sketch_snap: boolean
  mermaid_theme: 'auto' | 'default' | 'neutral' | 'forest' | 'dark' | 'base'; mermaid_look: 'classic' | 'handDrawn'
  list_style: 'grid' | 'list'; sort_by: 'edited' | 'created' | 'title'; trash_days: number; keep_versions: number
  notify_url: string; notify_format: string; notify_configured: boolean; notify_link_adds: boolean; notify_files_errors: boolean
  files_mode: ConnMode; files_via: ConnVia; files_detail: string; files_open_url: string; files_configured: boolean
  files_url: string; files_token: string; files_public_url: string; files_folder: string; files_auto: boolean
  notes_mode: ConnMode; notes_via: ConnVia; notes_detail: string; notes_open_url: string; notes_configured: boolean
  notes_url: string; notes_token: string; notes_public_url: string
  projects_mode: ConnMode; projects_via: ConnVia; projects_detail: string; projects_open_url: string; projects_configured: boolean
  projects_url: string; projects_token: string; projects_public_url: string
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  ai_language: string
  setup_complete: boolean; base_url: string; max_body_bytes: number
}

export interface Folder { id: number; name: string; sort: number; count: number; created_at: string }

export interface Drawing {
  id: number; uid: string; folder_id: number | null; kind: Kind; title: string; data?: string
  preview_rev: number; starred: boolean; rev: number; bytes: number; source: string
  files_path: string; files_rev: number; files_error: string
  opened_at: string; edited_at: string; trashed_at: string; created_at: string; updated_at: string
  folder_name: string; has_preview: boolean; versions: number
  /** how I may use it: mine, or shared with me to edit or to view */
  access?: Access; owner_name?: string; shares?: number
}

export type Access = 'owner' | 'edit' | 'view'

export interface Share { user_id: number; name: string; email: string; role: 'edit' | 'view'; created_at: string; here?: boolean }
export interface Person { id: number; name: string; email: string }

export interface Version { id: number; drawing_id: number; rev: number; title: string; data?: string; bytes: number; reason: 'edit' | 'restore' | 'import'; created_at: string; has_preview: boolean }

export interface Counts { all: number; starred: number; recent: number; trash: number; no_folder: number; bytes: number; kinds: Record<Kind, number>; shared: number }

export interface Dashboard { counts: Counts; folders: Folder[]; recent: Drawing[] }
