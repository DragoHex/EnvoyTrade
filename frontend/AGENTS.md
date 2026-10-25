# Frontend Guidance & Design Principles

This file provides architectural, design, and implementation guidelines for Claude Code and coding agents working on the EnvoyTrade frontend (`frontend/`).

---

## Themed Dropdown & Select Design Principles

All dropdowns and select inputs across the UI MUST use `<ThemedSelect>` ([`frontend/src/components/ThemedSelect.tsx`](file:///Users/msp/MSP/Projects/EnvoyTrade/frontend/src/components/ThemedSelect.tsx)). Never introduce raw, native `<select>` tags in user-facing components.

### 1. Dual-Layer Architecture
- **Visual Presentation Layer**:
  - Custom trigger button (`role="button"`, `aria-haspopup="listbox"`, `aria-expanded={isOpen()}`) with rounded borders, subtle hover state, and a rotating 14px chevron (`transform: rotate(180deg)` when open).
  - Custom popover menu (`role="listbox"`, `role="option"`, `aria-selected`) with smooth z-indexed positioning (`z-index: 50`), max height (`max-height: 240px`), and vertical scrolling.
  - Selected item indicator: Displays a checkmark SVG (`✓`) next to the active choice.
  - Optional badge support: Color-coded status badges via `ThemedSelectBadge`:
    - `emerald`: specialized / active / positive (`rgba(4, 180, 136, 0.15)`).
    - `slate`: neutral / unassigned / unrestricted (`rgba(100, 116, 139, 0.15)`).
    - `amber`: warning / suspended / error (`rgba(245, 158, 11, 0.15)`).
    - `blue`: admin / system (`rgba(59, 130, 246, 0.15)`).
  - Prefix / Icon support: Custom leading icon or `<BrokerLogo>` prefix passed via `prefix` prop or option `icon` renderer.
  - Placeholder / Helper text styling: When no option is selected (or an empty/disabled placeholder is shown), the trigger label is greyed out via `var(--color-text-secondary, #9da3ae)`. Upon selecting an active option, the label switches to standard text color (`var(--color-text, #f0f0f2)`).
- **Accessibility & Test Compatibility Layer**:
  - Hidden, zero-opacity, pointer-events-disabled `<select>` synchronized with current value and options.
  - Carries `id`, `name`, and `aria-label` directly.
  - Compatible with testing-library DOM interactions: responds to `userEvent.selectOptions(screen.getByLabelText(...), value)` and `fireEvent.change`.
  - Maintains `selected={opt.value === props.value}` on options and tracks `props.options` reactively in `createEffect` so dynamically inserted options immediately synchronize the DOM value.

### 2. Interaction & Dismissal
- **Click-outside dismissal**: Listens to `mousedown` on `document` and closes when click lands outside `containerRef`.
- **Keyboard navigation**: Closes on `Escape` keydown.
- **Disabled state**: Trigger has reduced opacity (`0.5`), `cursor: not-allowed`, and ignores clicks.

---

## General Frontend Design Principles

- **Never reload/remount a whole component for a small state change.** Only the piece that actually changed should update. A list refreshing after one row's action shouldn't blank or remount the other rows.
- **Loading state gets a skeleton of the real layout, not a "Loading…" text line.** Cap it with a timeout — stop showing the skeleton and say something's wrong if data never arrives.
- **Icon-only buttons get a hover tooltip with the action's name.** Never ship an icon-only control with no text anywhere.
- **Async actions (anything that hits the network on click) give immediate feedback**: disable the control and show a busy/spin state while pending, then confirm the outcome (success or failure) after — never leave a clicked button looking inert while work happens.
- **Toggles that represent on/off state look like switches, not checkboxes.**
- **Buttons and icons must stay visible and legible in both light and dark mode** — check contrast in both themes, not just one.
- **Destructive/irreversible actions are gated behind a confirmation step.**
- **A modal's dimmed backdrop and its content panel are visually and structurally distinct** — the backdrop dims the page, the panel stands out from it.
- **Verify UI changes by actually looking at the rendered result**, not just by reading the code — a change that looks right in source can still render wrong.
- **Show broker logo instead of plain text.** Use `<BrokerLogo broker={...} />` ([`frontend/src/components/BrokerLogo.tsx`](file:///Users/msp/MSP/Projects/EnvoyTrade/frontend/src/components/BrokerLogo.tsx)). An extensible `BROKER_REGISTRY` maps broker IDs (`kite`, `zerodha`, `dhan`) to official vector logos with accessible hover tooltip (`data-tooltip`). Unknown brokers fall back gracefully to a text badge.
- **Never display UUIDs on the UI.** Display human-readable `name` when present, falling back to `brokerAccountId`, never raw database UUID.
- **Distinct iconography for actions.** Use dedicated SVG icons with tooltips: pencil (`EditIcon`) for edit, person-with-minus (`UserMinusIcon`) for remove from group, trash bin (`TrashIcon`) for delete.
- **Fixed table layout prevents shifts on state toggle.** Use `table-layout: fixed` and explicit column widths/classes so buttons toggling text (e.g. "Enable"/"Disable") or badge status never cause table column jitter.

---

## UI Color Palette

Source of truth for `docs/plans/UI-PLAN.md` — reuse for any future UI work, don't re-derive.

| Role | Light | Dark | Hex meaning |
|---|---|---|---|
| Primary/Brand | `#44475b` | `#1a1c24` | Dark slate |
| Accent (Action) | `#04b488` | `#06d896` | Teal/Green |
| Border/Divider | `#f0f0f2` | `#2a2c34` | Light gray / Charcoal |
| Text Primary | `#44475b` | `#f0f0f2` | Slate / Off-white |
| Background | `#ffffff` | `#0f1117` | White / Near-black |
| Surface | `#f5f5f7` | `#1a1c24` | Light gray / Dark gray |
