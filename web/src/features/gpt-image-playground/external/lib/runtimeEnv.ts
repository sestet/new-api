export function readRuntimeEnv(value: unknown): string {
  return String.prototype.trim.call(value ?? '')
}
