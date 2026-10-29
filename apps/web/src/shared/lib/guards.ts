export function isNonEmptyString(value: string): boolean {
  return typeof value === "string" && value.trim().length > 0
}

export function isNonNegativeInteger(value: number): boolean {
  return Number.isInteger(value) && value >= 0
}

export function isRate(value: number): boolean {
  return typeof value === "number" && value >= 0 && value <= 1
}

export function nextIndex(current: number, total: number): number {
  return (current + 1) % total
}

export function prevIndex(current: number, total: number): number {
  return (current - 1 + total) % total
}
