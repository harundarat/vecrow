"use client"

import Link from "next/link"
import { useEffect, useState } from "react"
import {
  ChevronRight,
  CircleAlert,
  FileText,
  LoaderCircle,
  Mail,
  SearchX,
} from "lucide-react"
import {
  ESCROW_STATUS_HINT,
  ESCROW_STATUS_LABEL,
  EscrowStatusBadge,
} from "@/components/escrow-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useAuth } from "@/context/auth"
import { describeApiError } from "@/lib/api/errors"
import {
  ESCROW_STATUSES,
  listMyEscrows,
  type EscrowRole,
  type EscrowStatus,
  type MyEscrow,
} from "@/lib/api/escrow"
import { ESCROW_ROLE_LABEL, formatDate } from "@/lib/format"
import { formatUsdc } from "@/lib/usdc"
import { EmptyState } from "./empty-state"

type RoleTab = "all" | EscrowRole
type StatusFilter = "all" | EscrowStatus

const ROLE_TABS: Array<{ value: RoleTab; label: string }> = [
  { value: "all", label: "All" },
  { value: "freelancer", label: "Freelancer" },
  { value: "client", label: "Client" },
  { value: "member", label: "Member" },
]

const STATUS_ITEMS: Array<{ value: StatusFilter; label: string }> = [
  { value: "all", label: "All statuses" },
  ...ESCROW_STATUSES.map((status) => ({
    value: status,
    label: ESCROW_STATUS_LABEL[status],
  })),
]

type Result = { key: string; escrows?: MyEscrow[]; error?: string }

export function EscrowList() {
  const { request } = useAuth()
  const [role, setRole] = useState<RoleTab>("all")
  const [status, setStatus] = useState<StatusFilter>("all")
  const [reloads, setReloads] = useState(0)
  const [result, setResult] = useState<Result | null>(null)

  // Results are tagged with the query they answer, so a slow answer to an
  // earlier filter never shows under a newer one.
  const key = `${role}|${status}|${reloads}`

  useEffect(() => {
    const controller = new AbortController()
    listMyEscrows(
      request,
      {
        role: role === "all" ? undefined : role,
        status: status === "all" ? undefined : status,
      },
      controller.signal
    ).then(
      (escrows) => setResult({ key, escrows }),
      (err: unknown) => {
        if (!controller.signal.aborted) {
          setResult({ key, error: describeApiError(err) })
        }
      }
    )
    return () => controller.abort()
  }, [request, role, status, key])

  const current = result?.key === key ? result : null
  const filtered = role !== "all" || status !== "all"

  return (
    <Tabs value={role} onValueChange={(value) => setRole(value as RoleTab)}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <TabsList aria-label="Your role" className="w-full sm:w-fit">
          {ROLE_TABS.map((tab) => (
            <TabsTrigger key={tab.value} value={tab.value}>
              {tab.label}
            </TabsTrigger>
          ))}
        </TabsList>
        <Select
          items={STATUS_ITEMS}
          value={status}
          onValueChange={(value) => setStatus((value ?? "all") as StatusFilter)}
        >
          <SelectTrigger aria-label="Status" className="w-full sm:w-48">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_ITEMS.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <TabsContent value={role} className="mt-4">
        {!current ? (
          <div
            className="flex justify-center py-12"
            aria-busy="true"
            aria-live="polite"
          >
            <LoaderCircle
              aria-label="Loading escrows"
              className="size-6 animate-spin text-muted-foreground"
            />
          </div>
        ) : current.error ? (
          <EmptyState
            icon={<CircleAlert aria-hidden className="size-6" />}
            title="Could not load your escrows"
            description={current.error}
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => setReloads((n) => n + 1)}
              >
                Try again
              </Button>
            }
          />
        ) : current.escrows?.length ? (
          <ul className="flex flex-col gap-3">
            {current.escrows.map((escrow) => (
              <li key={escrow.id}>
                <EscrowItem escrow={escrow} />
              </li>
            ))}
          </ul>
        ) : filtered ? (
          <EmptyState
            icon={<SearchX aria-hidden className="size-6" />}
            title="No escrows here"
            description="None of your escrows match this filter."
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setRole("all")
                  setStatus("all")
                }}
              >
                Show all escrows
              </Button>
            }
          />
        ) : (
          <EmptyState
            icon={<FileText aria-hidden className="size-6" />}
            title="No escrows yet"
            description="Create an escrow, then send its link to the other party."
            action={
              <Link
                href="/app/escrows/new"
                className={buttonVariants({ size: "sm" })}
              >
                Create an escrow
              </Link>
            }
          />
        )}
      </TabsContent>
    </Tabs>
  )
}

function EscrowItem({ escrow }: { escrow: MyEscrow }) {
  const roles = escrow.roles.map((r) => ESCROW_ROLE_LABEL[r] ?? r).join(", ")
  const milestones = escrow.milestones.length

  return (
    <Link
      href={`/escrows/${escrow.id}`}
      className="flex items-center gap-3 rounded-2xl bg-card p-4 transition-colors outline-none hover:bg-muted/60 focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <EscrowStatusBadge status={escrow.status} />
          {escrow.invited && (
            <Badge variant="outline" title="Someone invited you to this escrow">
              <Mail aria-hidden />
              Invited
            </Badge>
          )}
        </div>
        <div>
          <p className="text-lg font-medium">
            {escrow.total ? formatUsdc(escrow.total) : "—"}{" "}
            <span className="text-sm text-muted-foreground">USDC</span>
          </p>
          <p className="text-sm text-muted-foreground">
            {ESCROW_STATUS_HINT[escrow.status]}
          </p>
        </div>
        <p className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span>
            You: <span className="text-foreground">{roles}</span>
          </span>
          <span>
            {milestones} {milestones === 1 ? "milestone" : "milestones"}
          </span>
          <span>Created {formatDate(escrow.created_at)}</span>
          <span className="font-mono" title={escrow.id}>
            #{escrow.id.slice(0, 8)}
          </span>
        </p>
      </div>
      <ChevronRight
        aria-hidden
        className="size-5 shrink-0 text-muted-foreground"
      />
    </Link>
  )
}
