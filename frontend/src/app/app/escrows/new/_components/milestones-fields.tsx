"use client"

import { Plus, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from "@/components/ui/input-group"
import { MAX_MILESTONES, type MilestonesCheck } from "@/lib/escrow-rules"
import { FormSection } from "@/components/form-section"
import { newRowKey } from "@/lib/row-key"

export type MilestoneRow = { key: number; amount: string }

export function MilestonesFields({
  rows,
  onChange,
  check,
  showErrors,
}: {
  rows: MilestoneRow[]
  onChange: (rows: MilestoneRow[]) => void
  check: MilestonesCheck
  showErrors: boolean
}) {
  function update(key: number, amount: string) {
    onChange(rows.map((row) => (row.key === key ? { ...row, amount } : row)))
  }

  return (
    <FormSection
      title="Milestones"
      description="Split the work into parts. Each part is paid out when the client approves it."
    >
      <ol className="flex flex-col gap-4">
        {rows.map((row, i) => {
          const error = showErrors ? check.errors[i] : null
          const inputId = `milestone-${row.key}`
          return (
            <li key={row.key}>
              <Field data-invalid={error ? true : undefined}>
                <FieldLabel htmlFor={inputId}>Milestone {i + 1}</FieldLabel>
                <div className="flex gap-2">
                  <InputGroup>
                    <InputGroupInput
                      id={inputId}
                      inputMode="decimal"
                      autoComplete="off"
                      placeholder="0.00"
                      value={row.amount}
                      onChange={(event) => update(row.key, event.target.value)}
                      aria-invalid={error ? true : undefined}
                      aria-describedby={error ? `${inputId}-error` : undefined}
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupText>USDC</InputGroupText>
                    </InputGroupAddon>
                  </InputGroup>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Remove milestone ${i + 1}`}
                    disabled={rows.length === 1}
                    onClick={() =>
                      onChange(rows.filter((other) => other.key !== row.key))
                    }
                  >
                    <Trash2 aria-hidden />
                  </Button>
                </div>
                <FieldError id={`${inputId}-error`}>{error}</FieldError>
              </Field>
            </li>
          )
        })}
      </ol>
      {showErrors && check.listError && (
        <FieldError>{check.listError}</FieldError>
      )}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="self-start"
        disabled={rows.length >= MAX_MILESTONES}
        onClick={() => onChange([...rows, { key: newRowKey(), amount: "" }])}
      >
        <Plus aria-hidden />
        Add milestone
      </Button>
    </FormSection>
  )
}
