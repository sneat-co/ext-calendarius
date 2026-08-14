import {
  assertValidHappeningPrices,
  type IHappeningPrice,
} from './happening';

export const eventHappeningLimits = {
  idMaxBytes: 200,
  principalMaxBytes: 200,
  spaceIdMaxBytes: 200,
  titleMaxBytes: 100,
  locationMaxBytes: 200,
  descriptionMaxBytes: 5000,
  requestIdMaxBytes: 200,
  timeZoneMaxBytes: 255,
  listMax: 100,
  childrenMax: 100,
  durationMaxMinutes: 7 * 24 * 60,
  maxSafeInteger: Number.MAX_SAFE_INTEGER,
} as const;

/**
 * Stable Eventius-facing projection of the canonical Calendarius
 * `type=single`, `kind=event` Happening.
 */
export interface EventHappeningDto extends EventHappeningSpecDto {
  readonly id: string;
  readonly type: 'single';
  readonly kind: 'event';
  readonly version: number;
  readonly status: EventHappeningStatus;
  readonly createdBy: string;
  /** RFC 3339 UTC instant. */
  readonly createdAt: string;
  /** Canonical Happening-owned prices, identified by stable price item ID. */
  readonly prices?: readonly IHappeningPrice[];
  /** Non-recursive view derived exclusively from standard Sneat Linkage. */
  readonly hierarchy: EventHappeningHierarchyDto;
}

export interface EventHappeningHierarchyDto {
  readonly parentHappeningId?: string;
  readonly childHappeningIds: readonly string[];
}

export type EventHappeningStatus =
  | 'active'
  | 'archived'
  | 'canceled'
  | 'deleted';

/**
 * Date and time are independently optional planning fields. When both exist,
 * timeZone (IANA TZDB) and utcOffset (±HH:MM) select one real instant, including
 * one side of a DST fold. A DST-gap local time is invalid.
 *
 * If endDate is omitted, endTime is on the same local date as date. An explicit
 * end requires endUtcOffset and must resolve after start. It is mutually
 * exclusive with durationMinutes.
 */
export interface EventHappeningSpecDto {
  readonly title: string;
  readonly date?: string;
  readonly time?: string;
  readonly timeZone?: string;
  readonly utcOffset?: string;
  readonly endDate?: string;
  readonly endTime?: string;
  readonly endUtcOffset?: string;
  /** Physical location text; URLs/virtual meeting data are not implied. */
  readonly location?: string;
  readonly description?: string;
  readonly durationMinutes?: number;
}

export interface CreateEventHappeningRequestDto {
  readonly requestId: string;
  readonly spec: EventHappeningSpecDto;
  /** Optional initial canonical Happening prices, persisted with creation. */
  readonly prices?: readonly IHappeningPrice[];
  /** Immutable first-release parent attachment through reciprocal Linkage. */
  readonly parentHappeningId?: string;
  readonly expectedParentVersion?: number;
}

/** Undefined leaves a field unchanged; an empty string clears an optional field. */
export interface UpdateEventHappeningRequestDto {
  readonly requestId: string;
  readonly expectedVersion: number;
  readonly title?: string;
  readonly date?: string;
  readonly time?: string;
  readonly timeZone?: string;
  readonly utcOffset?: string;
  readonly endDate?: string;
  readonly endTime?: string;
  readonly endUtcOffset?: string;
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

/** Mirrors the Go provider-boundary validation and UTF-8 byte limits. */
export function assertValidEventHappeningSpec(
  value: EventHappeningSpecDto,
): void {
  assertText('title', value.title, eventHappeningLimits.titleMaxBytes, true);
  assertDate('date', value.date);
  assertClock('time', value.time);
  assertTimeZone(value.timeZone);
  assertOffset('utcOffset', value.utcOffset);
  assertDate('endDate', value.endDate);
  assertClock('endTime', value.endTime);
  assertOffset('endUtcOffset', value.endUtcOffset);
  assertText(
    'location',
    value.location,
    eventHappeningLimits.locationMaxBytes,
    false,
  );
  assertText(
    'description',
    value.description,
    eventHappeningLimits.descriptionMaxBytes,
    false,
  );
  const duration = value.durationMinutes ?? 0;
  if (
    !Number.isSafeInteger(duration) ||
    duration < 0 ||
    duration > eventHappeningLimits.durationMaxMinutes
  ) {
    throw new Error(
      `durationMinutes must be a safe integer between 0 and ${eventHappeningLimits.durationMaxMinutes}`,
    );
  }

  const completeStart = !!value.date && !!value.time;
  if (completeStart) {
    if (!value.timeZone) throw new Error('timeZone is required with date and time');
    if (!value.utcOffset) throw new Error('utcOffset is required with date and time');
  } else if (value.utcOffset) {
    throw new Error('utcOffset requires date and time');
  }
  if (value.endDate && !value.endTime) throw new Error('endDate requires endTime');
  if (value.endUtcOffset && !value.endTime)
    throw new Error('endUtcOffset requires endTime');
  if (value.endTime) {
    if (!completeStart) throw new Error('endTime requires date and time');
    if (!value.endUtcOffset)
      throw new Error('endUtcOffset is required with endTime');
  }
  if (duration && !completeStart)
    throw new Error('durationMinutes requires date and time');
  if (value.endTime && duration)
    throw new Error('endTime and durationMinutes are mutually exclusive');

  const startDate = value.date;
  const startTime = value.time;
  if (completeStart && startDate && startTime && value.timeZone && value.utcOffset) {
    const start = localInstant(
      startDate,
      startTime,
      value.timeZone,
      value.utcOffset,
    );
    if (value.endTime && value.endUtcOffset) {
      const end = localInstant(
        value.endDate || startDate,
        value.endTime,
        value.timeZone,
        value.endUtcOffset,
      );
      if (end <= start) throw new Error('end instant must be after start instant');
    }
  }
}

export function assertValidCreateEventHappeningRequest(
  value: CreateEventHappeningRequestDto,
): void {
  assertText(
    'requestId',
    value.requestId,
    eventHappeningLimits.requestIdMaxBytes,
    true,
  );
  assertValidEventHappeningSpec(value.spec);
  assertValidHappeningPrices(value.prices);
  if (value.parentHappeningId) {
    assertLinkHappeningId('parentHappeningId', value.parentHappeningId);
    if (
      !Number.isSafeInteger(value.expectedParentVersion) ||
      (value.expectedParentVersion ?? 0) < 1
    )
      throw new Error(
        'expectedParentVersion must be a positive safe integer with parentHappeningId',
      );
  } else if ((value.expectedParentVersion ?? 0) !== 0) {
    throw new Error('expectedParentVersion requires parentHappeningId');
  }
}

export function assertValidUpdateEventHappeningRequest(
  value: UpdateEventHappeningRequestDto,
): void {
  assertText(
    'requestId',
    value.requestId,
    eventHappeningLimits.requestIdMaxBytes,
    true,
  );
  if (!Number.isSafeInteger(value.expectedVersion) || value.expectedVersion < 1)
    throw new Error('expectedVersion must be a positive safe integer');
  if (value.title !== undefined)
    assertText('title', value.title, eventHappeningLimits.titleMaxBytes, true);
  assertDate('date', value.date);
  assertClock('time', value.time);
  assertTimeZone(value.timeZone);
  assertOffset('utcOffset', value.utcOffset);
  assertDate('endDate', value.endDate);
  assertClock('endTime', value.endTime);
  assertOffset('endUtcOffset', value.endUtcOffset);
  if (value.location !== undefined)
    assertText(
      'location',
      value.location,
      eventHappeningLimits.locationMaxBytes,
      false,
    );
  if (value.description !== undefined)
    assertText(
      'description',
      value.description,
      eventHappeningLimits.descriptionMaxBytes,
      false,
    );
  if (
    value.durationMinutes !== undefined &&
    (!Number.isSafeInteger(value.durationMinutes) ||
      value.durationMinutes < 0 ||
      value.durationMinutes > eventHappeningLimits.durationMaxMinutes)
  ) {
    throw new Error('durationMinutes is outside the finite bound');
  }
}

export function assertValidEventHappening(value: EventHappeningDto): void {
  assertLinkHappeningId('id', value.id);
  if (value.type !== 'single') throw new Error('type must be single');
  if (value.kind !== 'event') throw new Error('kind must be event');
  if (!Number.isSafeInteger(value.version) || value.version < 1)
    throw new Error('version must be a positive safe integer');
  if (!['active', 'archived', 'canceled', 'deleted'].includes(value.status))
    throw new Error(`unknown status: ${value.status}`);
  assertText(
    'createdBy',
    value.createdBy,
    eventHappeningLimits.principalMaxBytes,
    true,
  );
  if (!isRFC3339UTC(value.createdAt))
    throw new Error('createdAt must be an RFC 3339 UTC instant');
  assertValidHappeningPrices(value.prices);
  assertValidEventHappeningHierarchy(value.id, value.hierarchy);
  assertValidEventHappeningSpec(value);
}

export function assertValidEventHappeningHierarchy(
  happeningId: string,
  value: EventHappeningHierarchyDto,
): void {
  if (!value || !Array.isArray(value.childHappeningIds))
    throw new Error('hierarchy.childHappeningIds is required');
  if (value.parentHappeningId) {
    assertLinkHappeningId('hierarchy.parentHappeningId', value.parentHappeningId);
    if (value.parentHappeningId === happeningId)
      throw new Error('hierarchy parent must not reference itself');
  }
  if (value.childHappeningIds.length > eventHappeningLimits.childrenMax)
    throw new Error('hierarchy child list exceeds finite bound');
  value.childHappeningIds.forEach((childId, index) => {
    assertLinkHappeningId(`hierarchy.childHappeningIds[${index}]`, childId);
    if (childId === happeningId)
      throw new Error('hierarchy child must not reference itself');
    if (index && value.childHappeningIds[index - 1] >= childId)
      throw new Error('hierarchy child IDs must be sorted and unique');
  });
}

function assertLinkHappeningId(field: string, value: string): void {
  assertText(field, value, eventHappeningLimits.idMaxBytes, true);
  if (value.includes('@'))
    throw new Error(`${field} must be a same-Space bare Happening ID`);
}

function isRFC3339UTC(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})T([01]\d|2[0-3]):([0-5]\d):([0-5]\d)(?:\.\d{1,9})?Z$/.exec(
    value,
  );
  if (!match) return false;
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) return false;
  return (
    parsed.getUTCFullYear() === Number(match[1]) &&
    parsed.getUTCMonth() + 1 === Number(match[2]) &&
    parsed.getUTCDate() === Number(match[3]) &&
    parsed.getUTCHours() === Number(match[4]) &&
    parsed.getUTCMinutes() === Number(match[5]) &&
    parsed.getUTCSeconds() === Number(match[6])
  );
}

function assertText(
  field: string,
  value: string | undefined,
  maxBytes: number,
  required: boolean,
): void {
  if (!value) {
    if (required) throw new Error(`${field} is required`);
    return;
  }
  if (!isWellFormedUnicode(value))
    throw new Error(`${field} must encode as valid UTF-8`);
  if (value.trim() !== value)
    throw new Error(`${field} must not have leading or trailing whitespace`);
  if (new TextEncoder().encode(value).byteLength > maxBytes)
    throw new Error(`${field} exceeds maximum UTF-8 byte length ${maxBytes}`);
}

function assertDate(field: string, value: string | undefined): void {
  if (!value) return;
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) throw new Error(`${field} must be an ISO date`);
  const [year, month, day] = match.slice(1).map(Number);
  const parsed = new Date(0);
  parsed.setUTCHours(0, 0, 0, 0);
  parsed.setUTCFullYear(year, month - 1, day);
  if (
    parsed.getUTCFullYear() !== year ||
    parsed.getUTCMonth() !== month - 1 ||
    parsed.getUTCDate() !== day
  )
    throw new Error(`${field} must be a real ISO date`);
}

function isWellFormedUnicode(value: string): boolean {
  for (let i = 0; i < value.length; i++) {
    const unit = value.charCodeAt(i);
    if (unit >= 0xd800 && unit <= 0xdbff) {
      if (i + 1 >= value.length) return false;
      const next = value.charCodeAt(++i);
      if (next < 0xdc00 || next > 0xdfff) return false;
    } else if (unit >= 0xdc00 && unit <= 0xdfff) {
      return false;
    }
  }
  return true;
}

function assertClock(field: string, value: string | undefined): void {
  if (value && !/^([01]\d|2[0-3]):[0-5]\d$/.test(value))
    throw new Error(`${field} must be 24-hour HH:MM time`);
}

function assertTimeZone(value: string | undefined): void {
  if (!value) return;
  assertText(
    'timeZone',
    value,
    eventHappeningLimits.timeZoneMaxBytes,
    false,
  );
  if (value === 'Local') throw new Error('timeZone must not be Local');
  try {
    new Intl.DateTimeFormat('en', { timeZone: value }).format();
  } catch {
    throw new Error('timeZone must be an IANA TZDB name');
  }
}

function assertOffset(field: string, value: string | undefined): void {
  if (!value) return;
  offsetMinutes(field, value);
}

function offsetMinutes(field: string, value: string): number {
  const match = /^([+-])(\d{2}):(\d{2})$/.exec(value);
  if (!match) throw new Error(`${field} must use ±HH:MM`);
  const hours = Number(match[2]);
  const minutes = Number(match[3]);
  if (hours > 14 || minutes > 59 || (hours === 14 && minutes !== 0))
    throw new Error(`${field} must be between -14:00 and +14:00`);
  const total = hours * 60 + minutes;
  return match[1] === '-' ? -total : total;
}

function localInstant(
  date: string,
  clock: string,
  timeZone: string,
  offset: string,
): number {
  const [year, month, day] = date.split('-').map(Number);
  const [hour, minute] = clock.split(':').map(Number);
  const candidate = Date.UTC(year, month - 1, day, hour, minute) -
    offsetMinutes('offset', offset) * 60_000;
  const formatter = new Intl.DateTimeFormat('en-CA-u-nu-latn', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  });
  const parts = Object.fromEntries(
    formatter
      .formatToParts(new Date(candidate))
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, part.value]),
  );
  if (
    Number(parts['year']) !== year ||
    Number(parts['month']) !== month ||
    Number(parts['day']) !== day ||
    Number(parts['hour']) !== hour ||
    Number(parts['minute']) !== minute
  )
    throw new Error(`local time/offset does not exist in IANA zone ${timeZone}`);
  return candidate;
}
