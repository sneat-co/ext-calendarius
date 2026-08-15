import { describe, expect, it } from 'vitest';
import {
  assertValidEventHappening,
  assertValidEventHappeningSpec,
  assertValidCreateEventHappeningRequest,
  eventHappeningLimits,
  EventHappeningDto,
} from './event-happening';

const scheduled = {
  title: 'Picnic',
  date: '2026-08-01',
  time: '12:30',
  timeZone: 'Europe/Dublin',
  utcOffset: '+01:00',
  endTime: '14:00',
  endUtcOffset: '+01:00',
};

describe('Event Happening contract', () => {
  it('accepts title-only and unambiguous scheduled plans', () => {
    expect(() => assertValidEventHappeningSpec({ title: 'Plan' })).not.toThrow();
    expect(() => assertValidEventHappeningSpec(scheduled)).not.toThrow();
  });

  it('uses UTF-8 bytes for finite string bounds', () => {
    expect(() =>
      assertValidEventHappeningSpec({
        title: 'é'.repeat(eventHappeningLimits.titleMaxBytes / 2 + 1),
      }),
    ).toThrow('UTF-8 byte length');
    expect(() =>
      assertValidEventHappeningSpec({ title: String.fromCharCode(0xd800) }),
    ).toThrow('valid UTF-8');
    expect(() =>
      assertValidEventHappeningSpec({
        title: 'Plan',
        location: 'é'.repeat(eventHappeningLimits.locationMaxBytes / 2 + 1),
      }),
    ).toThrow('UTF-8 byte length');
  });

  it('rejects unsafe end ordering and zone-offset mismatches', () => {
    expect(() =>
      assertValidEventHappeningSpec({ ...scheduled, endTime: '11:00' }),
    ).toThrow('after start');
    expect(() =>
      assertValidEventHappeningSpec({ ...scheduled, utcOffset: '+00:00' }),
    ).toThrow('does not exist');
  });

  it('validates the complete projection', () => {
    const event: EventHappeningDto = {
      ...scheduled,
      id: 'event1',
      type: 'single',
      kind: 'event',
      version: 1,
      status: 'active',
      createdBy: 'user1',
      createdAt: '2026-08-01T10:00:00Z',
      hierarchy: { childHappeningIds: [] },
      prices: [
        {
          id: 'single1',
          term: { unit: 'single', length: 1 },
          amount: { currency: 'EUR', value: 25 },
          expenseQuantity: 1,
        },
        {
          id: 'single1-team',
          term: { unit: 'single', length: 1 },
          amount: { currency: 'EUR', value: 50 },
          expenseQuantity: 2,
        },
      ],
    };
    expect(() => assertValidEventHappening(event)).not.toThrow();
    expect(() => assertValidEventHappening({ ...event, version: 0 })).toThrow(
      'version',
    );
    expect(() =>
      assertValidEventHappening({ ...event, version: Number.MAX_SAFE_INTEGER + 1 }),
    ).toThrow('version');
    expect(() =>
      assertValidEventHappening({ ...event, createdAt: '2026-02-30T10:00:00Z' }),
    ).toThrow('RFC 3339 UTC');
    expect(() =>
      assertValidEventHappening({
        ...event,
        prices: event.prices?.map((price) => ({ ...price, id: 'same' })),
      }),
    ).toThrow('duplicates');
  });

  it('validates canonical initial Happening prices on create', () => {
    const prices = [
      {
        id: 'single1',
        term: { unit: 'single' as const, length: 1 },
        amount: { currency: 'EUR', value: 25 },
      },
      {
        id: 'quarter1',
        term: { unit: 'quarter' as const, length: 1 },
        amount: { currency: 'EUR', value: 12_000 },
      },
    ];
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'priced-create',
        spec: { title: 'Priced game night' },
        prices,
      }),
    ).not.toThrow();
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'priced-create-unknown-term',
        spec: { title: 'Priced game night' },
        prices: [
          {
            ...prices[0],
            term: { unit: 'fortnight' as never, length: 1 },
          },
        ],
      }),
    ).toThrow('unknown unit');
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'priced-create',
        spec: { title: 'Priced game night' },
        prices: [{ ...prices[0], id: '' }],
      }),
    ).toThrow('id is required');
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'priced-create-lowercase',
        spec: { title: 'Priced game night' },
        prices: [
          {
            ...prices[0],
            amount: { currency: 'eur', value: 25 },
          },
        ],
      }),
    ).toThrow('ISO 4217');
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'priced-create-fraction',
        spec: { title: 'Priced game night' },
        prices: [
          {
            ...prices[0],
            amount: { currency: 'EUR', value: 25.5 },
          },
        ],
      }),
    ).toThrow('safe-integer minor units');
  });

  it('projects an annual recurring Series root and rejects a concrete single schedule on it', () => {
    const series: EventHappeningDto = {
      id: 'annual-cup', type: 'recurring', recurrence: { repeats: 'yearly' }, kind: 'event',
      title: 'Annual cup', version: 1, status: 'active', createdBy: 'user1',
      createdAt: '2026-08-01T10:00:00Z', hierarchy: { childHappeningIds: ['cup-2026'] },
    };
    expect(() => assertValidEventHappening(series)).not.toThrow();
    expect(() => assertValidCreateEventHappeningRequest({
      requestId: 'annual-cup-create', type: 'recurring', recurrence: { repeats: 'yearly' },
      spec: { title: 'Annual cup' },
    })).not.toThrow();
    expect(() => assertValidEventHappening({ ...series, time: '12:00' })).toThrow('recurrence');
    expect(() => assertValidCreateEventHappeningRequest({
      requestId: 'bad-recurring', type: 'recurring', recurrence: { repeats: 'yearly' },
      spec: { title: 'Annual cup', date: '2026-08-01', time: '12:00', timeZone: 'Europe/Dublin', utcOffset: '+01:00' },
    })).toThrow('recurrence');
  });

  it('validates non-recursive same-Space hierarchy conveniences', () => {
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'root-create',
        expectedParentVersion: 0,
        spec: { title: 'Root' },
      }),
    ).not.toThrow();
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'child-create',
        parentHappeningId: 'parent1',
        expectedParentVersion: 1,
        spec: { title: 'Child' },
      }),
    ).not.toThrow();
    expect(() =>
      assertValidCreateEventHappeningRequest({
        requestId: 'cross-space-child',
        parentHappeningId: 'parent@space2',
        expectedParentVersion: 1,
        spec: { title: 'Child' },
      }),
    ).toThrow('same-Space bare');
    const event: EventHappeningDto = {
      ...scheduled,
      id: 'event1',
      type: 'single',
      kind: 'event',
      version: 1,
      status: 'active',
      createdBy: 'user1',
      createdAt: '2026-08-01T10:00:00Z',
      hierarchy: {
        parentHappeningId: 'parent1',
        childHappeningIds: ['child-a', 'child-b'],
      },
    };
    expect(() => assertValidEventHappening(event)).not.toThrow();
    expect(() =>
      assertValidEventHappening({
        ...event,
        hierarchy: { childHappeningIds: ['child-b', 'child-a'] },
      }),
    ).toThrow('sorted and unique');
  });
});
