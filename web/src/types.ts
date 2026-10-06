export interface ScopeMemberSummary {
  user_id: string;
  name: string;
  event_count: number;
  revision: number;
}

export interface Scope {
  scope_id: string;
  kind: string;
  target_id: string;
  label: string;
  member_count: number;
  event_count: number;
  pending_count: number;
  members: ScopeMemberSummary[];
}

export interface WebEvent {
  id: number;
  uid: string;
  course: string;
  location: string;
  description: string;
  start: string;
  end: string;
  rrule: string;
}

export interface PageSchedule {
  scope_id: string;
  user_id: string;
  name: string;
  qq?: string;
  revision: number;
  events: WebEvent[];
}

export interface DayOverrideRow {
  user_id: string;
  name: string;
  day: string;
  kind: string;
  source_day: string;
  created_by?: string;
  created_at?: string;
}

export interface CheckinTotal {
  user_id: string;
  name: string;
  points: number;
  days: number;
}

export interface CheckinRecord {
  user_id: string;
  name: string;
  day: string;
  points: number;
  created_at?: string;
}

export interface CheckinBoard {
  scope_id: string;
  totals: CheckinTotal[];
  records: CheckinRecord[];
}

export interface BotSettings {
  enabled: boolean;
  reply_plain: boolean;
  reply_slash: boolean;
  reply_mention: boolean;
  send_format: string;
  nickname: string;
}

export interface PendingMember {
  user_id: string;
  name: string;
}

export interface PendingMemberList {
  scope_id: string;
  members: PendingMember[];
  member_count: number;
  note: string;
}

export interface ImportResult {
  member_count: number;
  created_count: number;
  updated_count: number;
  event_count: number;
  day_override_count?: number;
  skipped?: string[];
}

export interface SavePageResult {
  scope_id: string;
  user_id: string;
  name: string;
  revision: number;
  event_count: number;
}
