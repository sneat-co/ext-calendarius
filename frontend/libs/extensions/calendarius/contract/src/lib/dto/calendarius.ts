import { IIdAndDbo } from '@sneat/core';
import { IWithRelatedOnly } from '@sneat/dto';
import { ISpaceItemWithOptionalDbo } from '@sneat/space-models';
import { IHappeningBrief } from './happening';

export interface ICalendarHappeningBrief
  extends IHappeningBrief, IWithRelatedOnly {}

export type CalendarHappeningBriefsByID = Record<
  string,
  ICalendarHappeningBrief
>;

export interface ICalendariusSpaceDbo {
  readonly recurringHappenings?: Readonly<CalendarHappeningBriefsByID>;
}

export type CalendarHappeningBriefsBySpaceID = Record<
  string,
  CalendarHappeningBriefsByID
>;

export type ICalendariusSpaceDboWithID = IIdAndDbo<ICalendariusSpaceDbo>;

export type ICalendariusSpaceContext =
  ISpaceItemWithOptionalDbo<ICalendariusSpaceDbo>;
