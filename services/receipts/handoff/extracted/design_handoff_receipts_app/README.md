# Handoff: Receipts — household purchase & warranty archive

## Overview
A small personal web app for **two users (a couple)** to log everything they buy and find it again later — primarily so they can pull up a receipt and product details when something breaks or needs a warranty claim. Core jobs:

1. **Add** a purchase fast — snap a photo of the receipt *or* upload a PDF/image, plus title, merchant, amount, purchase date, tags, and free-text notes/context.
2. **Find** a purchase later — full-text search + tag filtering.
3. **View / Edit / Delete** a purchase, including viewing the attached receipt files.

It must work well as a **website on both mobile and desktop** (responsive, not two separate apps).

---

## About the Design Files
The files in `prototype/` are **design references created in HTML/CSS/React-via-Babel** — a working prototype that demonstrates the intended look, layout, and behavior. **They are not production code to ship as-is.**

The task is to **recreate these designs in a real codebase** using its established stack and conventions. The prototype uses React 18 loaded from a CDN with in-browser Babel and plain `localStorage` — fine for a mockup, not for production.

**Recommended production stack (no existing codebase yet):**
- **Frontend:** React + TypeScript + Vite (or Next.js if SSR/hosting convenience is wanted). The component structure below maps 1:1.
- **Styling:** the prototype is plain CSS with CSS custom properties (design tokens). Port the tokens to whatever the team uses (CSS Modules, Tailwind config, vanilla-extract…). Tokens are listed in full below.
- **Backend / sync (important — see below):** the prototype stores data only in the browser. For a two-person "access from anywhere" tool, this needs a real backend.

### Backend is required for the real product
The prototype persists to `localStorage`, so data lives only in one browser and is **not shared** between the two users or across devices. To meet the actual goal ("add receipts from anywhere, then query later"), implement:
- **Auth:** simple multi-user (the couple). Email magic-link or any low-friction auth.
- **Data store:** a `receipts` table/collection (schema below).
- **File storage:** receipt images/PDFs belong in object storage (e.g. S3/R2/Supabase Storage), **not** inline base64 like the prototype. Store a file URL/key per attachment.
- **API:** CRUD for receipts + file upload (presigned URL or multipart). Suggested: REST or tRPC.
- A managed option like **Supabase or Firebase** covers auth + DB + storage quickly and suits a project this size.

---

## Fidelity
**High-fidelity.** Colors, typography, spacing, radii, and interactions are final and intended to be reproduced precisely. Exact values are documented in **Design Tokens** below. The only thing deliberately *not* production-ready is the data layer (localStorage → real backend) and the CDN/Babel setup.

---

## Layout model & responsive behavior
There are **two shells** chosen by a single breakpoint:

- **Mobile shell** — viewport **< 960px**. A single phone-style column. On screens ≥ 540px the column is centered (max-width **440px**) with rounded corners and a drop shadow on a soft neutral backdrop; below 540px it's full-bleed.
- **Desktop shell** — viewport **≥ 960px**. A full-viewport **three-pane** layout (master–detail).

The switch is live (re-renders on resize) via a `matchMedia('(min-width: 960px)')` hook (`RC.useMedia`). Both shells read/write the **same data store**.

> Implementation note: in the prototype the two shells are separate component trees. In production you can either keep two layouts or build one responsive layout with CSS — but the desktop master–detail and the mobile push-navigation are different enough UX that keeping two layout components (sharing the same data hooks + leaf components) is the cleaner path.

---

## Screens / Views

### MOBILE SHELL (stack navigation: Add ⇄ List → Detail → Edit)

#### 1. Add (mobile home / default screen)
- **Purpose:** The app opens here. Log a new purchase quickly.
- **Layout:** vertical flex. Fixed **header** (top), **scrollable body** (middle), fixed **footer** (bottom).
  - Header: 38×38 rounded-square brand mark (accent bg, document icon) + title "New receipt" + subtitle "Add it now, find it later".
  - Body: capture zone, then the form fields (see **Receipt Form** component).
  - Footer (`.footer.split`, two buttons side by side): **"Find a receipt"** (ghost/outline accent, search icon → navigates to List) and **"Save"** (solid accent, check icon; disabled until Title is non-empty).
- **On save:** persist, reset the form, show toast "Receipt saved", navigate to List.

#### 2. List / Search (mobile)
- **Purpose:** Browse, search, and filter saved receipts.
- **Layout:** header ("Your receipts" + "N items on file", plus a **+** icon button top-right → Add) → **search bar** → horizontal scrolling **tag filter chips** (`All` + one per tag) → scrollable list of **receipt cards**. Empty state when no results.
- **Search:** case-insensitive AND-match of every whitespace-separated token across title + merchant + notes + tags.
- **Tag filter:** selecting a chip filters to receipts containing that tag; tap again or `All` to clear.
- **Card tap:** → Detail.

#### 3. Detail (mobile)
- **Purpose:** See everything about one receipt.
- **Layout:** sub-header (`.shead`) with a **back** icon button (→ List), spacer, **edit** icon button (→ Edit), **delete** icon button (opens confirm sheet). Scrollable body = the **Detail Body** component.

#### 4. Edit (mobile)
- **Purpose:** Modify an existing receipt.
- **Layout:** sub-header with back/cancel icon + "Edit receipt" title. Body = **Receipt Form** prefilled. Footer split: **Cancel** (soft) + **Save changes** (accent, disabled until Title non-empty).
- **On save:** persist, toast "Changes saved", → Detail.

#### Delete confirm (mobile) — bottom sheet
- Slides up from bottom (centered as a card ≥540px). Title "Delete this receipt?", body "“{title}” and its files will be removed. This can't be undone.", actions stacked: **Delete receipt** (danger outline) + **Keep it** (soft). Tapping the backdrop dismisses.

---

### DESKTOP SHELL (three-pane master–detail, ≥960px)

CSS grid, full viewport height: **`grid-template-columns: 248px  minmax(340px, 400px)  1fr`**.

#### Pane 1 — Sidebar (`.dnav`, 248px, surface bg, right border)
- **Brand:** 40×40 accent mark (document icon) + "Receipts" + subtitle "Household archive".
- **Primary button:** full-width solid-accent **"+ New receipt"** → opens the Form **modal** in `add` mode.
- **Tag filters** (`.dfilters`, vertical, scrollable): label "FILTER BY TAG", then rows — **"All receipts"** (with total count) and one row per tag with its count. Active row uses the accent-soft background + accent-press text. Counts are right-aligned, tabular.
- **Footer:** muted line "Stored on this device · N items" (replace with real status in production).

#### Pane 2 — List column (`.dlist`, 340–400px, right border)
- **Top:** the search bar (same search logic as mobile).
- **Scroll area:** "N results" count (+ " · {tag}" when filtered) then the list of **receipt cards**. Empty state when none. The **selected** card gets an accent border + 1px accent ring (`.card.active`).
- **Card click:** sets the selected receipt (updates Pane 3). Does **not** navigate away.

#### Pane 3 — Detail pane (`.ddetail`, remaining width, scrollable)
- **No selection:** centered empty state — document icon, "Select a receipt", "Pick an item from the list to see its files, details and notes — or add a new one."
- **Selection:** a **sticky toolbar** (right-aligned) with **"Edit"** (accent text button → Form modal in `edit` mode) and **"Delete"** (danger text button → confirm modal). Below it, a centered content column (`max-width: 760px`) = the **Detail Body** component, with a larger gallery (see component).

#### Form modal (desktop add & edit)
- Centered dialog over a dim backdrop (`rgba(20,20,20,.45)`), max-width **540px**, max-height **88vh**, radius **22px**.
- **Header:** title ("New receipt" / "Edit receipt") + close (×) icon button.
- **Body (scrollable):** the **Receipt Form** component.
- **Footer split:** **Cancel** (soft) + **Save receipt / Save changes** (accent, disabled until Title non-empty).
- **On save:** persist, close modal, select the saved receipt in the list, toast.
- Backdrop click closes.

#### Delete confirm (desktop) — centered modal
- Same backdrop, narrower modal (`max-width: 420px`). Title + warning copy (same as mobile). Footer split: **Keep it** (soft) + **Delete** (danger). If the deleted item was selected, clear the selection.

---

## Shared leaf components (used by both shells)

### Receipt Form (`form.jsx` → `ReceiptForm`)
Controlled form; calls `onChange(updatedRecord)` on every edit. Fields, in order:
1. **Capture / upload zone** (`.cap`): dashed accent-soft panel. Camera icon, bold prompt ("Snap a photo or add a PDF" / "Add another file" once files exist), sub "Receipt, warranty card, manual…", then two buttons:
   - **Take photo** — opens `<input type="file" accept="image/*" capture="environment">` (camera on mobile).
   - **Upload file** — opens `<input type="file" accept="image/*,application/pdf" multiple>`.
   - Selected files render as 66×66 **thumbnails** (image preview, or a document-icon + "PDF" tile) each with a circular **×** remove button.
2. **Title** — text, placeholder "e.g. Robot lawnmower". **Required** (only validated field; gates Save).
3. **Merchant** + **Amount** on one row. Amount has a trailing "kr" suffix and `inputMode="decimal"`.
4. **Date of purchase** — native date input, defaults to today.
5. **Tags** — chip editor: type + Enter or comma to add; Backspace on empty input removes last; each chip has an × ; tags lowercased & de-duped.
6. **Notes & context** — textarea, placeholder "Serial number, where it's installed, warranty length, anything useful later…".

### Receipt Card (`shared.jsx` → `RC.CardRow`)
Row: 50×50 **thumbnail** (image, else document icon, else first letter of title on a tinted tile) · **title** (1 line, ellipsis) · **"{merchant or 'No merchant'} · {formatted date}"** · optional file-count badge ("N files") · right-aligned **amount** (`b` = grouped number, `small` = "SEK"). `active` prop adds the selected styling.

### Detail Body (`shared.jsx` → `RC.DetailBody`)
- **Gallery:** horizontal on mobile (cards 180×230) / wrapping & larger on desktop (210×270). Images shown cover-fit; PDFs show a document icon + "Open PDF" link (`target=_blank`, `download`). If no files: a dashed-less placeholder "No file attached".
- **Hero:** large title; price line = grouped amount in accent color + "kr".
- **Facts:** rows with leading accent icon — **Merchant** (store icon), **Purchased** (calendar, formatted date), **Tags** (tag icon, chips). Each row separated by a 1px top border.
- **Notes:** label "NOTES & CONTEXT" + preserved-whitespace paragraph (only if notes exist).

---

## Interactions & Behavior
- **Navigation (mobile):** in-memory screen state `{ name: 'add'|'list'|'detail'|'edit', id? }`. No URL routing in the prototype — **add real routes in production** (e.g. `/add`, `/`, `/r/:id`, `/r/:id/edit`) so links/back button work.
- **Navigation (desktop):** selection state only; list and detail are always both visible. Consider reflecting the selected id in the URL (`/?r=:id`) in production.
- **Search:** live, client-side in the prototype. With a backend, either keep client-side filtering (dataset is small) or move to a query param.
- **Validation:** Title required; Save button disabled (40% opacity) until non-empty. Nothing else is required.
- **Toast:** pill at bottom-center, slides/fades in, auto-dismisses after **2200ms**. Messages: "Receipt saved", "Changes saved", "Receipt deleted".
- **Animations / transitions:**
  - Screen/detail entrance: `translateY(7px) → 0` over **0.28s** ease (transform only — see note).
  - Bottom sheet: `translateY(40px) → 0` over **0.26s** `cubic-bezier(.2,.8,.2,1)`.
  - Modal: `translateY(10px) → 0` over **0.2s** ease.
  - Toast: opacity + `translateY(10px)` over **0.25s**.
  - Buttons: background 0.15s; `:active` nudges `translateY(1px)`.
  - **Important pattern:** entrance animations animate **transform only**, never `opacity: 0 → 1`, and the element's **base (non-animated) state is the final visible state**. This guarantees content is visible even if the animation never runs. Keep this in production (or use enter-transitions tied to a mounted class) — do not gate visibility behind an animation's start keyframe.
- **Hover states:** icon buttons → surface bg; cards → border + soft shadow; filter rows → surface2; primary button → darker accent (`--accent-press`); danger hover → light red bg.
- **Loading / error states:** none in the prototype (synchronous localStorage). **Add in production:** list skeletons or spinner while fetching; upload progress for files; error toasts on failed save/upload/delete; optimistic update or pending state on mutations.

---

## State Management
Prototype keeps everything client-side. Logical state to reproduce:
- **`items`** — array of receipts (from the store).
- **Mobile:** `screen` (`{name, id}`), `toastMsg`, `del` (receipt pending delete or null). Each Add/Edit screen holds a local `rec` draft.
- **Desktop:** `q` (search text), `tag` (active tag or null), `selId` (selected receipt id), `modal` (`{mode:'add'|'edit', id?}` or null), `del`, `toastMsg`. Derived: `tagCounts`, sorted `tags`, `filtered` list, `sel` (selected receipt).
- **Store API to reproduce as a data layer / hooks** (`store.js` → `Store`): `all()` (newest first), `get(id)`, `add(obj)`, `update(id, patch)`, `remove(id)`, `allTags()` (sorted by frequency). In production back these with API calls (e.g. React Query / SWR) instead of localStorage.

### Receipt data model
```
Receipt {
  id: string                 // server-generated in prod
  title: string              // required
  merchant: string
  amount: string|number      // stored as entered; format for display
  date: string               // ISO 'YYYY-MM-DD'
  tags: string[]             // lowercased, unique
  notes: string
  files: Attachment[]
  created: number            // epoch ms; used for default sort (newest first)
}
Attachment {
  name: string
  type: string               // MIME, e.g. 'image/jpeg' | 'application/pdf'
  dataUrl: string            // PROTOTYPE ONLY (base64). In prod: store a URL/key to object storage instead.
}
```

### Helpers to reproduce (`store.js`)
- **`rcpFmt(n)`** — strips non-numeric, formats with `sv-SE` grouping (e.g. `18990 → "18 990"`); returns "—" if NaN.
- **`rcpDate(iso)`** — `en-GB` `{day:'numeric', month:'short', year:'numeric'}` → e.g. "2 Nov 2024".
- **`rcpReadFile(file)`** — FileReader → `{name, type, dataUrl}`. Replace with real upload in prod.

---

## Design Tokens

### Colors
| Token | Hex | Use |
|---|---|---|
| `--bg` | `#FFFFFF` | App background, cards, inputs (focus) |
| `--surface` | `#FAFAFA` | Inputs/fields default, sidebar bg |
| `--surface2` | `#F4F4F2` | Soft button bg, detail thumb tile, filter hover |
| `--text` | `#141414` | Primary text |
| `--muted` | `#8A8A8A` | Secondary text, labels |
| `--faint` | `#B5B5B5` | Placeholders, footnotes |
| `--border` | `#ECECEC` | Field borders, dividers |
| `--border2` | `#E2E2E0` | Stronger borders, button outlines, scrollbar thumb |
| `--accent` | `#EA580C` | Primary actions, brand mark, price, active states |
| `--accent-press` | `#C2470A` | Accent hover/pressed |
| `--accentText` | `#FFFFFF` | Text/icon on accent |
| `--accent-soft` | `#FFF1E8` | Capture zone bg, tag chips, active filter bg |
| Danger | `#D7263D` text · `#FEECEC` bg · `#F0C2C2` border | Delete actions |
| Toast check | `#7CF2A4` | Toast icon |
| Mobile scene backdrop | `radial-gradient(120% 80% at 50% 0%, #F3F1EC, #E5E2DC)` | Behind the phone column |

### Typography
- **Display / headings — `--fh`:** **Bricolage Grotesque** (weights 600/700/800). Used for h1/h2, hero title, prices, amounts, brand. Tight letter-spacing (-0.3 to -0.6px on large sizes).
- **Body / UI — `--fb`:** **Hanken Grotesk** (400/500/600/700). All inputs, labels, body text, buttons.
- Both are Google Fonts. Sizes (px): screen titles 22; sub-screen titles 18; hero title 26 (mobile) / 30 (desktop); price 22/24; card title 14.5; body/input 14–14.5; labels 11 uppercase letter-spacing .4px; meta 11–12; section labels 10.5–11 uppercase.

### Spacing & radii
- Screen padding: headers ~18–22px; form/body 16–18px horizontal.
- Field gap: 14px; row gap: 11px.
- **Radii:** inputs/cards 12–13px; large cards/panels 16–22px; pills/chips 8px (tag) / 999px (filter chip, search clear); icon buttons 11px; brand mark 12px; thumbnails 12px.
- Icon button: 38×38 (40×40 brand mark). Tap targets ≥ 44px effective on mobile where relevant.

### Shadows
- `--shadow`: `0 1px 2px rgba(20,20,20,.04), 0 8px 24px rgba(20,20,20,.06)` (cards on hover).
- `--shadow-lg`: `0 12px 40px rgba(20,20,20,.16)` (phone column ≥540px, modals, toast).

---

## Assets
- **Icons:** all inline SVG (24×24, `stroke="currentColor"`), defined in `icons.js` (`window.IC` map + `<Ico name=…>` helper). Names used: `search, camera, plus, doc, tag, cal, shop, coin, note, chev, check, back, pencil, trash, x, image, upload`. Replace with the codebase's icon library (Lucide/Feather match this stroke style closely — these icons are Feather-like) or port the SVGs directly.
- **Fonts:** Bricolage Grotesque + Hanken Grotesk (Google Fonts). Self-host in production for performance/privacy.
- **No raster images or logos** ship with the design. Receipt photos/PDFs are user-provided at runtime.

---

## Files (in `prototype/`)
- **`Receipts.html`** — entry point; loads fonts, CSS, React (CDN), and the scripts in order. Open this to run the prototype.
- **`receipts/app.css`** — all styles + design tokens (`:root`). The source of truth for visual values.
- **`icons.js`** — inline SVG icon set + `<Ico>` helper.
- **`receipts/store.js`** — data layer (localStorage CRUD + seed data) and formatting helpers. **Replace with API/backend in production.**
- **`receipts/shared.jsx`** — `RC.useMedia`, `RC.Toast`, `RC.thumbFor`, `RC.CardRow`, `RC.DetailBody` (used by both shells).
- **`receipts/form.jsx`** — `ReceiptForm` (capture zone, tag editor, fields).
- **`receipts/screens.jsx`** — mobile shell (Add/List/Detail/Edit) + the responsive root that picks mobile vs desktop.
- **`receipts/desktop.jsx`** — desktop three-pane shell + form/confirm modals.

> The prototype seeds 5 example receipts (TV, NAS, soundbar, dishwasher, mower) so search/filter feel populated. Remove seeding in production; start empty with a first-run empty state.

---

## Suggested build order
1. Tokens + base styles; port the icon set; load fonts.
2. Data layer + types (start with a mock/in-memory or Supabase client behind the same `all/get/add/update/remove/allTags` interface).
3. Leaf components: `ReceiptForm`, `CardRow`, `DetailBody`, `Toast`.
4. Desktop three-pane shell + modals.
5. Mobile shell + navigation/routing.
6. Responsive switch.
7. Backend: auth, receipts CRUD, file upload to object storage, wire to the data layer; swap base64 attachments for stored file URLs.
8. Production states: loading/skeletons, upload progress, error handling.
