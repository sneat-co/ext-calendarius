/**
 * Stable Eventius-facing projection of a Calendarius `type=single`,
 * `kind=event` happening. Its `id` is the canonical event identity; clients
 * must never create a parallel Event record.
 */
export interface EventHappeningDto {
  readonly id: string;
  readonly version: number;
  readonly title: string;
  readonly date?: string;
  readonly time?: string;
  readonly endDate?: string;
  readonly endTime?: string;
  readonly location?: string;
  readonly description?: string;
  readonly durationMinutes?: number;
  readonly status: EventHappeningStatus;
  readonly createdBy: string;
  readonly createdAt: string;
}

export type EventHappeningStatus =
  | 'active'
  | 'archived'
  | 'cancelled'
  | 'deleted';

/** Date and time are deliberately independently optional planning fields. */
export interface EventHappeningSpecDto {
  readonly title: string;
  readonly date?: string;
  readonly time?: string;
  readonly endDate?: string;
  readonly endTime?: string;
  readonly location?: string;
  readonly description?: string;
  readonly durationMinutes?: number;
}

export interface CreateEventHappeningRequestDto {
  /** Stable caller-generated operation ID; the same ID with another payload conflicts. */
  readonly requestId: string;
  readonly spec: EventHappeningSpecDto;
}

/** Undefined leaves a field unchanged; an empty string clears an optional field. */
export interface UpdateEventHappeningRequestDto {
  readonly requestId: string;
  /** Required optimistic-concurrency guard from EventHappeningDto.version. */
  readonly expectedVersion: number;
  readonly title?: string;
  readonly date?: string;
  readonly time?: string;
  readonly endDate?: string;
  readonly endTime?: string;
  readonly location?: string;
  readonly description?: string;
  readonly durationMinutes?: number;
}

export type EventHappeningMutationDisposition =
  | 'created'
  | 'changed'
  | 'unchanged'
  | 'reused';

export interface EventHappeningMutationDto {
  readonly event: EventHappeningDto;
  readonly disposition: EventHappeningMutationDisposition;
}
