import axios from 'axios'

// The list of seeded client API keys, so the UI can switch between tenants.
export const API_KEYS = ['key-alpha', 'key-beta', 'key-gamma', 'key-delta', 'key-test']

// Which client are we acting as? Stored in the browser so it survives refreshes.
export function getApiKey(): string {
  return localStorage.getItem('apiKey') || 'key-alpha'
}
export function setApiKey(key: string) {
  localStorage.setItem('apiKey', key)
}

// One axios instance for the whole app. baseURL '/api' is forwarded to the Go backend
// by the Vite proxy (see vite.config.ts).
export const api = axios.create({ baseURL: '/api' })

// Automatically attach the x-api-key header to EVERY request — so no page has to
// remember to do it. The key is read fresh each time, so switching clients just works.
api.interceptors.request.use((config) => {
  config.headers['x-api-key'] = getApiKey()
  return config
})

// Shape of a task as returned by the API (matches models.Task in Go).
export interface Task {
  id: number
  type: string
  priority: number
  status: string
  retry_count: number
  max_retries: number
  created_at: string
  started_at: string | null
  completed_at: string | null
  error_message: string | null
}

// GET /api/tasks returns a page of tasks plus pagination metadata.
export interface TaskPage {
  data: Task[]
  total: number
  page: number
  page_size: number
  total_pages: number
}
