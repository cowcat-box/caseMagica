import { fetchAPI, jsonHeaders, parseSSEStream, readErrorMessage, requestJSON } from '@/lib/api-client/client'
import type { SSEEvent } from '@/lib/api-client/types'

export type ContinuationDepth = 'short' | 'medium' | 'long'
export type ContinuationMode = 'direction' | 'excerpt'
export type ContinuationTarget = 'new_chapter' | 'append'
export type ContinuationTaskStatus = 'pending' | 'running' | 'done' | 'partially_done' | 'cancelled' | 'failed'

export interface ContinuationExploreRequest {
  workspace: string
  anchor_chapter_path?: string
  depth: ContinuationDepth
  mode: ContinuationMode
  style_ref_names?: string[]
  custom_style?: string
  preference?: string
}

export interface ContinuationCandidate {
  index: number
  title?: string
  direction?: string
  excerpt?: string
  plan?: string
  status: 'generating' | 'done' | 'failed'
  error?: string
}

export interface ContinuationTaskMeta {
  id: string
  workspace: string
  request: ContinuationExploreRequest
  status: ContinuationTaskStatus
  created_at?: string
  updated_at?: string
  finished_at?: string
}

export type ContinuationEvent =
  | { event: 'explore_start'; task_id: string }
  | { event: 'candidate_start'; task_id: string; index: number }
  | { event: 'candidate_delta'; task_id: string; index: number; content: string }
  | { event: 'candidate_done'; task_id: string; index: number; candidate: ContinuationCandidate }
  | { event: 'candidate_failed'; task_id: string; index: number; error: string }
  | { event: 'explore_done'; task_id: string; status: string }

export async function exploreContinuation(input: ContinuationExploreRequest): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI('/api/continuation/explore', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function listContinuationTasks(workspace: string): Promise<ContinuationTaskMeta[]> {
  const data = await requestJSON<{ tasks?: ContinuationTaskMeta[] }>(`/api/continuation/tasks?workspace=${encodeURIComponent(workspace)}`)
  return data.tasks || []
}

export async function replayContinuationTask(taskId: string): Promise<ReadableStream<SSEEvent>> {
  const res = await fetchAPI(`/api/continuation/tasks/${encodeURIComponent(taskId)}/stream`)
  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) throw new Error('No response body')
  return parseSSEStream(res.body)
}

export async function abortContinuationTask(taskId: string): Promise<void> {
  await requestJSON(`/api/continuation/tasks/${encodeURIComponent(taskId)}/abort`, { method: 'POST' })
}

export async function deleteContinuationTask(taskId: string): Promise<{ backup_path: string }> {
  return requestJSON(`/api/continuation/tasks/${encodeURIComponent(taskId)}`, { method: 'DELETE' })
}

export async function commitContinuation(input: {
  taskId: string
  index: number
  title: string
  content: string
  target: ContinuationTarget
}): Promise<{ path: string }> {
  return requestJSON('/api/continuation/commit', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      task_id: input.taskId,
      index: input.index,
      title: input.title,
      content: input.content,
      target: input.target,
    }),
  })
}
