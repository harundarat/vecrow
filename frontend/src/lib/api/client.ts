import { env } from "@/env"

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
    this.code = code
  }
}

export type RequestOptions = {
  method?: "GET" | "POST" | "PATCH" | "DELETE"
  body?: unknown
  token?: string | null
  guestToken?: string | null
  signal?: AbortSignal
}

/** Calls the API as the signed-in account; see `useAuth().request`. */
export type Requester = <T>(
  path: string,
  options?: Omit<RequestOptions, "token">
) => Promise<T>

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {}
): Promise<T> {
  const res = await send(path, "application/json", options)
  if (res.status === 204) return undefined as T
  return (await res.json().catch(() => undefined)) as T
}

/** A file the API serves as an attachment, such as a CSV export. */
export async function apiDownload(
  path: string,
  options: Omit<RequestOptions, "method" | "body"> = {}
): Promise<{ blob: Blob; filename: string | null }> {
  const res = await send(path, "*/*", options)
  const disposition = res.headers.get("Content-Disposition") ?? ""
  const filename = /filename="?([^";]+)"?/i.exec(disposition)?.[1] ?? null
  return { blob: await res.blob(), filename }
}

async function send(
  path: string,
  accept: string,
  { method = "GET", body, token, guestToken, signal }: RequestOptions
): Promise<Response> {
  const headers: Record<string, string> = { Accept: accept }
  if (body !== undefined) headers["Content-Type"] = "application/json"
  if (token) headers.Authorization = `Bearer ${token}`
  if (guestToken) headers["X-Guest-Token"] = guestToken

  let res: Response
  try {
    res = await fetch(`${env.NEXT_PUBLIC_API_URL}${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if (signal?.aborted) throw err
    throw new ApiError(0, "network_error", "Could not reach the Vecrow server.")
  }

  if (!res.ok) {
    const data: unknown = await res.json().catch(() => undefined)
    const error = isErrorEnvelope(data) ? data.error : undefined
    throw new ApiError(
      res.status,
      error?.code ?? "http_error",
      error?.message ?? `Request failed with status ${res.status}.`
    )
  }
  return res
}

type ErrorEnvelope = { error: { code: string; message: string } }

function isErrorEnvelope(data: unknown): data is ErrorEnvelope {
  if (typeof data !== "object" || data === null || !("error" in data)) {
    return false
  }
  const error = (data as { error: unknown }).error
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as { code?: unknown }).code === "string" &&
    typeof (error as { message?: unknown }).message === "string"
  )
}
