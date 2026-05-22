export function formatRelativeAge(value: string): string {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return value;
  }
  const deltaMs = Date.now() - timestamp;
  const abs = Math.abs(deltaMs);
  const minutes = Math.round(abs / 60000);
  if (minutes < 60) {
    return `${minutes || 1}m ago`;
  }
  const hours = Math.round(minutes / 60);
  if (hours < 48) {
    return `${hours}h ago`;
  }
  const days = Math.round(hours / 24);
  return `${days}d ago`;
}
