let lastKey = 0

/** A React key for a row the user adds; rows can be removed in any order. */
export function newRowKey(): number {
  lastKey += 1
  return lastKey
}
