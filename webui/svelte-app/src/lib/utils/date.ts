import { formatDistanceToNow, format } from 'date-fns';

export function timeAgo(date: string | Date): string {
	return formatDistanceToNow(new Date(date), { addSuffix: true });
}

export function formatDate(date: string | Date, formatStr: string = 'PPpp'): string {
	return format(new Date(date), formatStr);
}
