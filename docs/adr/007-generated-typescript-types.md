# ADR-007: TypeScript types generated from the Go data models

Status: accepted · Date: 2026-10-18

## Context
Every widget's data shape existed twice: as Go structs and as hand-written TypeScript. Nothing connected them, so renaming a JSON field on one side compiled fine and failed silently on the tablet (ADR-003 had planned generated types, they were not built).

## Decision
- Generate the TypeScript with **tygo**, used as a **CLI tool** pinned in `backend/go.mod` (`go tool tygo generate`, config in `backend/tygo.yaml`). It is a build-time tool only and is not part of the server binary or the container image.
- Only the **wire models** are converted: `model.go` in each widget package, `config/client.go` (the config sent to the browser) and `core/protocol.go` (envelope and widget state). Provider internals stay out. Output goes to `frontend/**/*.gen.ts` and is committed.
- String enums become TypeScript unions (`enum_style: union`); this needs Go consts prefixed with the type name (`ConditionRain`, `RowKindSwitch`).
- A Go test (`internal/typegen`) regenerates into a temp folder and fails if the committed files differ. It is the safety net until there is CI.
- The frontend re-exports the generated types under its own names in each widget's `types.ts`; a rename on the Go side now fails `svelte-check`.

## Settings (added 2026-10-18)
Widget settings are typed Go structs (`Settings` in each `model.go`) as well, so they are generated like the data models. The loosely typed YAML map is decoded strictly (`widget.DecodeSettings`: unknown keys and wrong types are errors that name the setting and list the valid ones), and every widget implements `widget.Validator`, which the config loader calls: a bad setting is rejected when the file is loaded and the previous configuration stays active. `widget.StringList` and `widget.FlexString` cover values that YAML users may write as one value or a list, or as text or a number. A test checks that every registered widget type validates its settings.

## Not covered
- Wrong values with the right type (e.g. seconds vs. milliseconds) still need tests.
- The generated TypeScript of a flexible setting shows its canonical form (e.g. `calendars` as objects although a plain connection name is accepted in YAML).

## Alternatives considered
- Go to JSON Schema to TypeScript: more moving parts; the strict struct decoding already gives validation.
- tygo as a library: no benefit for a build-time step.
