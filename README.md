# ext-calendarius

Public contract surface for the **calendarius** extension, per the
[`ext-<id>` convention](https://github.com/sneat-co/sneat-specs/blob/main/standards/repo-naming.md)
and the [extension backend architecture standard](https://github.com/sneat-co/sneat-specs/blob/main/standards/extension-backend-architecture.md).
Depends only on the standard library — never on another extension or the
calendarius implementation.

Created 2026-07-09 to close the compliance gap recorded in the ecosystem review
(`backstage/spec/research/ecosystem-review-2026-07/`): calendarius was the only
widely-consumed extension without a definition repo, so consumers (e.g. the
eventius adapter in `sneat-go`) imported `calendarius/backend` implementation
types directly.

## Contents

- `backend/` — Go module `github.com/sneat-co/ext-calendarius/backend`
  - `calendariusmodels` — shared vocabulary: `HappeningSpec`,
    `HappeningBrief`, `RecurringHappening`, and `Occurrence`
  - `facade4calendarius` — `HappeningsFacade` and
    `RecurringHappeningsFacade`, the interfaces consumers depend on; the host
    injects implementations from Calendarius
- `frontend/` — Nx workspace whose `ext-calendarius-contract` project publishes
  `@sneat/extension-calendarius-contract`. This repository is its sole source.

## Migration plan (grow-as-touched, no big bang)

1. ✅ Seed vocabulary + facade interface (this repo, `backend/v0.x`).
2. `sneat-go/pkg/modules/eventius` adapter: implement
   `facade4calendarius.HappeningsFacade` and keep the eventius port satisfied by
   it; new calendar consumers (bookius, school-portal) depend on this interface
   instead of writing new ports.
3. Extract further shared DTOs from `calendarius/backend/dto4calendarius` here
   as second consumers appear (`dbo4calendarius` storage types stay private —
   see the standard's moves-vs-stays rule).
4. ✅ Move `@sneat/extension-calendarius-contract` from `sneat-libs` into this
   repository. The implementation packages live in `calendarius/frontend`.
5. ✅ Add recurring-happening projection, extension-data, and occurrence
   expansion contracts so Togethered no longer imports Calendarius DBOs.
