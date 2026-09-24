import { differenceInDays, differenceInHours, isSameDay } from 'date-fns';
import type { FormatStyleName } from 'javascript-time-ago';
import TimeAgo from 'javascript-time-ago';
import en from 'javascript-time-ago/locale/en';

function toDate(date: Date | string | number | null | undefined): Date | null {
  if (!date) return null;
  const d = date instanceof Date ? date : new Date(date);
  return isNaN(d.getTime()) ? null : d;
}

export function dateDifferanceInDays(
  date1: Date | string | number,
  date2: Date | string | number
) {
  const d1 = toDate(date1);
  const d2 = toDate(date2);
  if (!d1 || !d2) return 0;
  const diffTime = Math.abs(d2.getTime() - d1.getTime());
  return Math.ceil(diffTime / (1000 * 60 * 60 * 24));
}

export function getLocale() {
  if (typeof navigator === 'undefined') {
    return 'en-US';
  }

  return navigator.language ?? 'en-US';
}

export function formatDate(date: Date | string | number | null | undefined) {
  const d = toDate(date);
  if (!d) {
    return '-';
  }
  const day = d.getDate();
  const month = new Intl.DateTimeFormat(getLocale(), { month: 'short' })
    .format(d)
    .replace('.', '')
    .toLowerCase();

  return `${day} ${month}`;
}

export function formatDateTime(date: Date | string | number | null | undefined) {
  const d = toDate(date);
  if (!d) {
    return '-';
  }
  const datePart = formatDate(d);
  const timePart = new Intl.DateTimeFormat(getLocale(), {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    year:
      d.getFullYear() === new Date().getFullYear() ? undefined : 'numeric',
  }).format(d);

  return `${datePart}, ${timePart}`;
}

export function formatTime(date: Date | string | number | null | undefined) {
  const d = toDate(date);
  if (!d) {
    return '-';
  }
  return new Intl.DateTimeFormat(getLocale(), {
    hour: 'numeric',
    minute: 'numeric',
    second: 'numeric',
  }).format(d);
}

TimeAgo.addDefaultLocale(en);
const ta = new TimeAgo(getLocale());

export function timeAgo(
  date: Date | string | number | null | undefined,
  style?: FormatStyleName
) {
  const d = toDate(date);
  if (!d) return '-';
  return ta.format(d, style);
}

export function formatTimeAgoOrDateTime(
  date: Date | string | number | null | undefined
) {
  const d = toDate(date);
  if (!d) return '-';
  if (Math.abs(differenceInHours(d, new Date())) < 3) {
    return timeAgo(d);
  }

  return isSameDay(d, new Date()) ? formatTime(d) : formatDateTime(d);
}

export function utc(date: string) {
  if (date.match(/^\d{4}-\d{2}-\d{2}\s\d{2}:\d{2}:\d{2}.\d{3}$/)) {
    return new Date(`${date}Z`);
  }
  return new Date(date).toISOString();
}
