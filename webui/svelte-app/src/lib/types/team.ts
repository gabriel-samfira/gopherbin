import type { LabelInfo } from './paste';

export type TeamRole = 'owner' | 'admin' | 'member' | 'viewer' | 'pending';
export type MemberRole = 'admin' | 'member' | 'viewer';

export interface TeamMember {
	id: number;
	username: string;
	email: string;
	full_name: string;
	status?: 'active' | 'pending';
	role?: MemberRole;
	added_by_id?: number;
	added_by?: string;
}

export interface TeamStats {
	members: number;
	pending: number;
	pastes: number;
	contributors: number;
}

export interface Team {
	id: number;
	name: string;
	description?: string;
	owner: TeamMember;
	members?: TeamMember[];
	/** Relation of the current user to this team. */
	my_role?: TeamRole;
	labels?: string[];
	label_details?: LabelInfo[];
	/** Set while an ownership transfer awaits the target's decision. */
	transfer_to?: TeamMember;
	my_pending_transfer?: boolean;
	stats?: TeamStats;
}

export interface TeamList {
	teams: Team[];
	page: number;
	total_pages: number;
}

export interface TeamTransferInfo {
	team_id: number;
	team_name: string;
	from_user?: string;
}

export interface TeamInviteInfo {
	team_id: number;
	team_name: string;
	invited_by?: string;
}
