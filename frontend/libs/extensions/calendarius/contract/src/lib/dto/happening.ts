import { IWithRelatedOnly, IWithSpaceIDs } from '@sneat/dto';
import { ActivityType, RepeatPeriod, WeekdayCode2 } from './happening-types';
import { IWithStringID } from './todo_move_funcs';

export interface ISlotParticipant {
  readonly roles?: string[];
  // readonly type: 'member' | 'contact';
  // readonly title: string;
}

export interface IHappeningParticipant {
  readonly roles?: string[];
  // readonly type: 'member' | 'contact';
  // readonly title: string;
}

/**
 * Canonical Happening price-coverage term. This is independent of the
 * Happening recurrence cadence; `quarter` does not make an event recurring.
 */
export type TermUnit =
  | 'single'
  | 'second'
  | 'minute'
  | 'hour'
  | 'day'
  | 'week'
  | 'month'
  | 'quarter'
  | 'year';

export interface ITerm {
  readonly length: number;
  readonly unit: TermUnit;
}

export type CurrencyCode = 'USD' | 'EUR' | 'RUB' | string;

export interface IAmount {
  readonly currency: CurrencyCode;
  /** Integer minor units, matching decimal.Decimal64p2's canonical wire form. */
  readonly value: number;
}

export interface IHappeningPrice {
  readonly id: string;
  readonly term: ITerm;
  readonly amount: IAmount;
  readonly expenseQuantity?: number;
}

export const happeningPriceLimits = {
  idMaxBytes: 200,
  maxItems: 100,
} as const;

const happeningPriceTermUnits: readonly TermUnit[] = [
  'single',
  'second',
  'minute',
  'hour',
  'day',
  'week',
  'month',
  'quarter',
  'year',
];

const canonicalCurrencyCodes = new Set(
  (
    Intl as typeof Intl & {
      supportedValuesOf(key: 'currency'): readonly string[];
    }
  ).supportedValuesOf('currency'),
);

/**
 * Validates the existing Happening-owned price projection. Price item IDs are
 * the stable reference for consumers; multiple items may intentionally share
 * a term. This helper does not introduce an Event-specific pricing authority.
 */
export function assertValidHappeningPrices(
  prices: readonly IHappeningPrice[] | undefined,
): void {
  if (!prices) return;
  if (prices.length > happeningPriceLimits.maxItems)
    throw new Error(
      `prices exceeds maximum item count ${happeningPriceLimits.maxItems}`,
    );
  const seen = new Set<string>();
  for (const [index, price] of prices.entries()) {
    assertHappeningPriceID(`prices[${index}].id`, price.id);
    if (seen.has(price.id))
      throw new Error(`prices[${index}].id duplicates ${price.id}`);
    seen.add(price.id);
    if (!happeningPriceTermUnits.includes(price.term.unit))
      throw new Error(`prices[${index}].term has unknown unit`);
    if (!Number.isSafeInteger(price.term.length) || price.term.length < 1)
      throw new Error(`prices[${index}].term.length must be positive`);
    if (!canonicalCurrencyCodes.has(price.amount.currency))
      throw new Error(
        `prices[${index}].amount.currency must be a canonical ISO 4217 code`,
      );
    if (!Number.isSafeInteger(price.amount.value) || price.amount.value < 0)
      throw new Error(
        `prices[${index}].amount.value must be nonnegative safe-integer minor units`,
      );
    if (
      price.expenseQuantity !== undefined &&
      (!Number.isSafeInteger(price.expenseQuantity) ||
        price.expenseQuantity < 0)
    )
      throw new Error(
        `prices[${index}].expenseQuantity must be a nonnegative safe integer`,
      );
  }
}

function assertHappeningPriceID(field: string, value: string): void {
  if (!value) throw new Error(`${field} is required`);
  if (value.trim() !== value)
    throw new Error(`${field} must not have leading or trailing whitespace`);
  if (!isWellFormedUnicode(value))
    throw new Error(`${field} must encode as valid UTF-8`);
  if (new TextEncoder().encode(value).byteLength > happeningPriceLimits.idMaxBytes)
    throw new Error(
      `${field} exceeds maximum UTF-8 byte length ${happeningPriceLimits.idMaxBytes}`,
    );
  if (value === '*') throw new Error(`${field} must not be '*'`);
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

export interface IHappeningBase extends IWithRelatedOnly {
  readonly type: HappeningType;
  readonly status: HappeningStatus;
  readonly kind: HappeningKind;
  readonly activityType?: ActivityType; // TODO: Is it same as HappeningKind?
  readonly title: string;
  readonly summary?: string;
  readonly levels?: Level[];
  // readonly contactIDs?: readonly string[]; // obsolete
  readonly slots?: Readonly<Record<string, IHappeningSlot>>;
  readonly prices?: readonly IHappeningPrice[];
  // readonly participants?: Record<string, Readonly<IHappeningParticipant>>;
  /**
   * Per-extension data embedded on the happening, keyed by extension id (e.g.
   * `eventus`). Lets a hosting module store its own fields on the happening
   * instead of a separate overlay document; calendarius stays agnostic to the
   * blob shapes. Mirrors the backend `HappeningBase.Ext`.
   */
  readonly ext?: Readonly<Record<string, unknown>>;
}

export type IHappeningBrief = IHappeningBase;

export interface IWithDates {
  readonly dates?: string[];
}

export interface IWithSpaceDates extends IWithSpaceIDs, IWithDates {
  readonly spaceDates?: string[]; // ISO date strings prefixed with spaceID e.g. [`abc123:2019-12-01`, `abc123:2019-12-02`]
}

export interface IHappeningDbo extends IHappeningBrief, IWithSpaceDates {
  readonly description?: string;
}

export function validateHappeningDto(dto: IHappeningDbo): void {
  if (!dto.title) {
    throw new Error('happening has no title');
  }
  if (dto.title !== dto.title.trim()) {
    throw new Error(
      'happening title has leading or closing whitespace characters',
    );
  }
  switch (dto.type) {
    case 'single':
      break;
    case 'recurring':
      break;
    default:
      if (!dto.type) {
        throw new Error('happening has no type');
      }
      throw new Error('happening has unknown type: ' + dto.type);
  }
  if (!dto.type) {
    throw new Error('happening has no type');
  }
  const slots = Object.entries(dto.slots || {});
  const isPlannedSingleEvent = dto.type === 'single' && dto.kind === 'event';
  if (!slots.length && !isPlannedSingleEvent) {
    throw new Error('!dto.slots?.length');
  }
  switch (dto.type) {
    case 'single':
      slots.forEach(([slotID, slot]) => {
        if (isPlannedSingleEvent) {
          validatePlannedEventSlot(slotID, slot);
        } else {
          validateSingleHappeningSlot(slotID, slot);
        }
      });
      break;
    case 'recurring':
      slots.forEach(([slotID, slot]) =>
        validateRecurringHappeningSlot(slotID, slot),
      );
      break;
  }
}

export function validateRecurringHappeningSlot(
  slotID: string,
  slot: IHappeningSlot,
): void {
  if (slot.repeats === 'once' || slot.repeats === 'UNKNOWN') {
    throw new Error(
      `slots[${slotID}]: slot.repeats is not valid for recurring happening: ${slot.repeats}`,
    );
  }
  validateHappeningSlot(slotID, slot);
}

export function validateSingleHappeningSlot(
  slotID: string,
  slot: IHappeningSlot,
): void {
  if (slot.repeats != 'once') {
    throw new Error(
      `slots[${slotID}]: slot repeats is not 'once': ${slot.repeats}`,
    );
  }
  validateHappeningSlot(slotID, slot);
}

// validatePlannedEventSlot accepts the independently known parts of a
// one-time event plan. A title-only event has no slot; when a slot exists it
// must contribute at least a date, time, location or duration.
export function validatePlannedEventSlot(
  slotID: string,
  slot: IHappeningSlot,
): void {
  if (slot.repeats !== 'once') {
    throw new Error(
      `slots[${slotID}]: planned event slot repeats is not 'once': ${slot.repeats}`,
    );
  }
  if (
    !slot.start?.date &&
    !slot.start?.time &&
    !slot.end?.date &&
    !slot.end?.time &&
    !slot.durationMinutes &&
    !slot.location?.title &&
    !slot.location?.address
  ) {
    throw new Error(`slots[${slotID}]: planned event slot has no planning data`);
  }
  if ((slot.end?.time || slot.durationMinutes) && !slot.start?.time) {
    throw new Error(
      `slots[${slotID}]: planned event end or duration requires a start time`,
    );
  }
}

function validateHappeningSlot(slotID: string, slot: IHappeningSlot): void {
  if (
    !slot.start?.time &&
    !(slot.repeats.startsWith('monthly') || slot.repeats.startsWith('yearly'))
  ) {
    throw new Error(`slots[${slotID}]: slot has no start time: ${slot}`);
  }
}

export type HappeningType = 'recurring' | 'single';

export type HappeningStatus = 'draft' | 'active' | 'canceled' | 'archived';

export type HappeningKind = 'appointment' | 'activity' | 'task' | 'event';

export interface SlotLocation {
  readonly title?: string;
  readonly address?: string;
}

interface IFortnightly {
  readonly title: string;
}

/*
// tslint:disable-next-line:no-magic-numbers
type MonthlyDay = -5 | -4 | -3 | -2 | -1
// tslint:disable-next-line:no-magic-numbers
	| 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10
// tslint:disable-next-line:no-magic-numbers
	| 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19
// tslint:disable-next-line:no-magic-numbers
	| 20 | 21 | 22 | 23 | 24 | 25 | 26 | 27 | 28;
*/

export interface IDateTime {
  readonly date?: string;
  readonly time?: string;
}

export interface ITiming {
  readonly start?: IDateTime;
  readonly end?: IDateTime;
  readonly durationMinutes?: number;
}

export interface IHappeningSlotSingleRef {
  readonly repeats: RepeatPeriod;
  readonly weekday?: WeekdayCode2;
  readonly week?: number;
}

export type Month =
  | 'January'
  | 'February'
  | 'March'
  | 'April'
  | 'May'
  | 'June'
  | 'July'
  | 'August'
  | 'September'
  | 'October'
  | 'November'
  | 'December';

export interface IHappeningSlotTiming extends ITiming {
  readonly repeats: RepeatPeriod;
  readonly weekdays?: readonly WeekdayCode2[];
  readonly day?: number;
  readonly month?: Month;
  readonly weeks?: readonly number[];
  readonly fortnightly?: Readonly<{
    readonly odd: IFortnightly;
    readonly even: IFortnightly;
  }>;
}

export type Level = 'beginners' | 'intermediate' | 'advanced';

export interface IHappeningTask {
  readonly serviceProvider?: {
    readonly id: string;
    readonly title: string;
  };
}

export interface IHappeningSlot extends IHappeningSlotTiming {
  readonly location?: SlotLocation;
  readonly groupIds?: string[]; // TODO: What is this?
}

export type IHappeningSlotWithID = IWithStringID<IHappeningSlot>;

export const emptyTiming: ITiming = {
  // durationMinutes: 0,
};

export const emptyHappeningSlot: IHappeningSlotWithID = {
  id: '',
  repeats: 'UNKNOWN',
  ...emptyTiming,
};

export interface ISingleHappeningDbo extends IHappeningDbo {
  readonly dtStarts?: number; // UTC
  readonly dtEnds?: number; // UTC
  readonly weekdays?: WeekdayCode2[];
}

export interface DtoSingleTask extends ISingleHappeningDbo {
  readonly isCompleted: boolean;
  readonly completion?: number; // In percents, max value is 100.
}
