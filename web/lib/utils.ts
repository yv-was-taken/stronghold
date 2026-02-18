// Utility functions for the dashboard

/**
 * Format an account number with dashes
 * Input: 1234567890123456 or 1234-5678-9012-3456
 * Output: 1234-5678-9012-3456
 */
export function formatAccountNumber(input: string): string {
  // Remove all non-digit characters
  const digits = input.replace(/\D/g, '');

  // Limit to 16 digits
  const limited = digits.slice(0, 16);

  // Format with dashes
  const parts = [];
  for (let i = 0; i < limited.length; i += 4) {
    parts.push(limited.slice(i, i + 4));
  }

  return parts.join('-');
}

/**
 * Check if an account number is valid (16 digits)
 */
export function isValidAccountNumber(input: string): boolean {
  const digits = input.replace(/\D/g, '');
  return digits.length === 16;
}

/**
 * Format a microUSDC string from the API into a human-readable dollar string.
 * 1 USDC = 1,000,000 microUSDC.
 *
 * Examples:
 *   "1250000" -> "$1.25"
 *   "1000000" -> "$1.00"
 *   "1000"    -> "$0.001"
 *   "0"       -> "$0.00"
 */
export function formatUSDC(microStr: string): string {
  const normalized = String(microStr ?? '').trim();
  const input = normalized === '' ? '0' : normalized;

  let micro: bigint;
  try {
    micro = BigInt(input);
  } catch {
    return '$0.00';
  }

  const negative = micro < 0n;
  const absMicro = negative ? -micro : micro;
  const whole = absMicro / 1_000_000n;
  const frac = absMicro % 1_000_000n;
  const wholeStr = whole.toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  const raw = `${wholeStr}.${frac.toString().padStart(6, '0')}`;
  // Trim trailing zeros but keep at least 2 decimal places
  const dotIdx = raw.indexOf('.');
  let end = raw.length;
  while (end > dotIdx + 3 && raw[end - 1] === '0') end--;
  const formatted = raw.slice(0, end);
  return negative ? `-$${formatted}` : `$${formatted}`;
}

/**
 * Format date for display
 */
export function formatDate(dateString: string): string {
  const date = new Date(dateString);
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

/**
 * Format relative time (e.g., "2 hours ago")
 */
export function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffInSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (diffInSeconds < 60) {
    return 'just now';
  }

  const diffInMinutes = Math.floor(diffInSeconds / 60);
  if (diffInMinutes < 60) {
    return `${diffInMinutes} minute${diffInMinutes > 1 ? 's' : ''} ago`;
  }

  const diffInHours = Math.floor(diffInMinutes / 60);
  if (diffInHours < 24) {
    return `${diffInHours} hour${diffInHours > 1 ? 's' : ''} ago`;
  }

  const diffInDays = Math.floor(diffInHours / 24);
  if (diffInDays < 30) {
    return `${diffInDays} day${diffInDays > 1 ? 's' : ''} ago`;
  }

  return formatDate(dateString);
}

/**
 * Truncate wallet address for display
 */
export function truncateAddress(address: string, startChars = 6, endChars = 4): string {
  if (address.length <= startChars + endChars + 3) {
    return address;
  }
  return `${address.slice(0, startChars)}...${address.slice(-endChars)}`;
}

/**
 * Copy text to clipboard
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

/**
 * Download text as a file
 */
export function downloadTextFile(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}
