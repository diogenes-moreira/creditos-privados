import { format } from "date-fns";

/**
 * CuppoGan credit lines are Colombian, so amounts are grouped the Colombian
 * way whatever locale the browser reports (as in the quota lookup).
 */
const MONEY_LOCALE = "es-CO";

/**
 * Formats a decimal string for display. The string the backend sent is the
 * value of record: it is parsed to be printed, never to be recomputed.
 */
export const formatCreditAmount = (raw: string, currency: string): string => {
  const value = Number.parseFloat(raw);
  if (!Number.isFinite(value)) return raw;
  return new Intl.NumberFormat(MONEY_LOCALE, {
    style: "currency",
    currency,
    minimumFractionDigits: 0,
    maximumFractionDigits: 2,
  }).format(value);
};

export const formatDateTime = (iso?: string): string => {
  if (!iso) return "";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : format(date, "dd/MM/yyyy HH:mm");
};

export const formatDate = (iso?: string): string => {
  if (!iso) return "";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : format(date, "dd/MM/yyyy");
};

export const formatBytes = (size: number): string =>
  size >= 1024 * 1024
    ? `${(size / (1024 * 1024)).toFixed(1)} MB`
    : `${Math.max(1, Math.round(size / 1024))} KB`;

/** The first and last characters of a hash, enough to compare at a glance. */
export const shortHash = (hash: string): string =>
  hash.length > 20 ? `${hash.slice(0, 10)}…${hash.slice(-8)}` : hash;
