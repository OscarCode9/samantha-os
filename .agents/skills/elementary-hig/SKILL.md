---
name: elementary-hig
description: "Apply the elementary OS Human Interface Guidelines (HIG) when designing or reviewing GTK4/Vala apps for elementary OS. Use when asked to design, review, or audit a UI for HIG compliance, elementary style, or when adding any widget, dialog, notification, layout, icon, or user workflow. Source: https://docs.elementary.io/hig"
---

# elementary OS Human Interface Guidelines

Full reference for designing apps that feel native to elementary OS. Always apply these rules when building or reviewing any GTK4/Vala UI in this project.

**Full HIG:** https://docs.elementary.io/hig

---

## 1. Design Philosophy

### Four Core Principles

1. **Concision** — Make the app instantly understandable. The main function should be immediately apparent. Avoid feature bloat; every new feature costs performance, clarity, maintenance, and user attention. Think in modules: small, focused apps that communicate through Launcher Actions and Portals.

2. **Accessible Configuration** — Design for the out-of-the-box experience. Require zero configuration before first use when possible. Ask the OS for information (name, location) instead of asking the user. If configuration is unavoidable, present it inline (Welcome Screen), not in a separate dialog.

3. **Minimal Documentation** — The app should be self-documenting. Avoid technical jargon. Provide plain-language error messages with clear next steps. Users should not need to read a manual.

4. **Design is not veneer** — Design is how it works, not just how it looks. Every decision—button label, placement, order—is a design decision. Design is testable and objective.

---

## 2. User Workflow

### First Launch
- Show a Welcome Screen if there is no content (use `Granite.Placeholder` / welcome screen widget).
- Do not require configuration before use.
- Make the window appear fast; do background tasks after the window is visible.
- If a background task is blocking, show a progress indicator and disable blocked UI items (make them insensitive).

### Normal Launch
- Restore all state: open documents, scroll position, undo history.
- Closing and reopening should be indistinguishable from "minimize then restore."

### Always Saved
- Changes are applied instantly. No "Save" buttons for preferences or inline edits.
- Dialogs like "Song Info" should update instantly as the user types.

### Always Provide an Undo
- **Never show a warning dialog when you can provide undo instead.**
- Show a toast or info bar with an "Undo" action.
- Use a buffer period: the app shows the result immediately, then performs the irreversible action after a short delay or on window close.
- Support `Ctrl+Z` for undo.

### Closing
- Windows should close (not minimize). Save all state on close.
- Background tasks should complete shortly after close, then the process should quit.
- Do **not** add a minimize button; elementary OS does not surface minimize.

---

## 3. Layout & Spacing

### Spacing Rules
- Window border → widget minimum gap: **12px**
- Label → widget minimum gap: **12px**
- Horizontal spacing between buttons: **6px**
- If section headers are present, indent labels beneath them.

### Alignment
- Widgets should be justified (align both left and right edges).
- Labels should be **right-aligned** relative to each other in forms (easier to scan).
- Section headers should be left-aligned.

### Sidebars
- Place sidebar on the **left** (right for RTL languages).
- Organize sections from most important (top) to least important (bottom).
- Sections may be collapsible. Avoid nesting expandable sections inside each other.

### Toolbar / HeaderBar Item Order
- Most commonly used items go at the **beginning (start)**.
- Least used items go at the **end**.
- Popovers and settings menus go at the very end.
- Keep RTL in mind: layout will be mirrored.

---

## 4. Feedback Widgets — When to Use What

| Situation | Widget |
|---|---|
| Confirm user action, optional undo, auto-dismisses | **Toast** |
| Persistent contextual info, may offer action, doesn't obscure content | **Info Bar** |
| Requires immediate action, multiple options, critical info | **Message Dialog** (`Granite.MessageDialog`) |
| Info while app may not be focused, shown in Notification Center | **Notification** |
| Long-running background task progress | **Dock Progress Bar** |
| Count of new, actionable unread items | **Dock Badge** |

### Info Bar Severity
- **Information** (white): Supplemental info, optional action.
- **Question** (blue): Non-critical question, answer expected.
- **Warning** (yellow): Unexpected/bad thing may happen, action to resolve.
- **Error** (red): Error occurred, user action required. Reserve for critical situations only.

### Message Dialog Rules
- Primary text: brief summary + suggested action. Sentence case, **no terminal punctuation** (unless a question).
- Secondary text: detailed description, side effects. Sentence case, **with terminal punctuation**.
- Button order (left → right): Incidental actions | Alternative actions | Cancel | **Affirmative action** (far right, end-aligned).
- **Never use "OK"** — use explicit action names: "Save", "Delete", "Shut Down".
- Affirmative (non-destructive) action → `.suggested-action` CSS class. Focus by default.
- Destructive action → `.destructive-action` CSS class. **Do NOT focus by default.**
- Never have both `.suggested-action` and `.destructive-action` in the same dialog.
- Use `Granite.MessageDialog` — it gives the correct layout automatically.

### Notifications
- Title: sentence case, no terminal punctuation (except questions).
- Do not add a global notification toggle in the app — direct users to System Settings.
- Use `LibNotify` to set sounds (respects user settings).

### Dock Badges
- Show count of **new, unseen** items since last app open — not historical totals.
- Easy to dismiss when the user opens the app.

---

## 5. Text & Capitalization

### Capitalization Rules

| Context | Case |
|---|---|
| Titles, buttons, menus, most widgets | **Title Case** (AP Style) |
| Labels, descriptive text, secondary copy | **Sentence case** |
| Notification titles | Sentence case, no terminal punctuation |
| Dialog primary text | Sentence case, no terminal punctuation |
| Dialog secondary text | Sentence case, **with** terminal punctuation |

**Title Case (AP Style):** Capitalize first and last words; all nouns, pronouns, adjectives, verbs, adverbs, subordinate conjunctions; all words > 3 letters. Do NOT capitalize articles/prepositions/conjunctions ≤ 3 letters.

**Exception:** "elementary" is always lowercase, even at the start of a sentence.

### Writing Style
- Be brief. Short, scannable sentences.
- Think simple — no technical jargon, assume intelligent but non-technical user.
- Get to the bottom line first — most important info at the beginning.
- Don't repeat yourself.
- Use visual hierarchy: headings separate primary from secondary text.
- Make all strings translatable. Avoid culture-specific references.

### Punctuation Quick Reference
- Ellipsis: use `…` (`\u2026`), not `...`. Use for actions requiring further input or a new window.
- Left/right double quotes: `\u201C` / `\u201D`
- Apostrophe/right single quote: `\u2019`
- Em dash (—): `\u2014`, no spaces around it.
- En dash (–): `\u2013`, no spaces, used for number ranges.
- One space after a period.

### Ellipsis in Button/Menu Labels
Use `…` when the action requires additional input in a new window or dialog:
- Good: **Open…**, **Find…**, **Print…**, **Preferences…**
- Bad: **Delete** (instant, irreversible) — offer undo instead.
- Never in placeholder text. Never in submenu items (arrow implies submenu).

### Menu Item Naming
- Names should be actions or locations, never descriptions.
- Good: "Find in Page…", "Export as PDF…"
- Bad: "Software Up to Date" (unclear what clicking does)

---

## 6. App Launcher (.desktop file)

Formula: **Name is a(n) GenericName that helps you Comment.**

```ini
Name=Eddy
GenericName=Package Installer
Comment=Install Debian packages
```

- **Name**: Short, no generic descriptor. "Dexter" not "Dexter Address Book."
- **GenericName**: "My app is a(n) ___." Title Case. No articles, "app", "application", "program".
- **Comment**: Verb phrase, sentence case, no terminal punctuation. "Listen to music."
- **Categories**: Semicolon-separated, terminated with semicolon.
- **Keywords**: Title Case, semicolon-separated, single words. Help discovery without cluttering the name.

---

## 7. Iconography

### Sizes
Design each size individually (pixel-fitting). Six required sizes: **16, 24, 32, 48, 64, 128px**.

### Shape
- Use a distinctive silhouette. Not always a rounded rectangle.
- Use the object's real-world shape for hardware/file-type icons.
- Do not slap a pictogram onto a generic base shape — use a unique outline.

### Color
- Use vibrant colors — reserve grays/muted tones for boring system icons.
- Follow the [elementary Brand Palette](https://elementary.io/brand#color).
- Red = error/danger, Orange = warning, Green = go — use connotations intentionally.

### Full-Color vs Symbolic
- **Full-color**: App icons, files, devices, places.
- **Symbolic**: Buttons, lists, text fields, status indicators, dynamic/semi-transparent backgrounds.

### Composition
- Maintain baseline alignment so icons of the same size sit on the same line.
- 1px outline stroke on all icons; color = 30% darker than primary icon color. Semi-transparent stroke.
- Edge highlight: subtle 1px inner stroke, brighter at top/bottom.
- Drop shadow: linear gradient perpendicular to bottom margin, 60% opacity.

---

## 8. Quick Decision Checklist

Before shipping any screen or widget, verify:

- [ ] No configuration required before first use (or uses Welcome Screen).
- [ ] Destructive actions use undo/toast, not warning dialogs.
- [ ] State is saved on close; restores on reopen.
- [ ] No "OK" buttons — all dialog actions have specific names.
- [ ] Spacing: 12px window margin, 6px between buttons.
- [ ] Suggested action button gets `.suggested-action`. Destructive gets `.destructive-action`. They don't coexist.
- [ ] All copy in correct case (Title Case for buttons/menus, Sentence case for labels/descriptions).
- [ ] No literal curly quotes in Vala string literals — use `\u201c`, `\u201d`, `\u2019`.
- [ ] Strings are translatable with `_("...")`.
- [ ] Toolbar/sidebar items ordered most-used first, least-used last.
- [ ] Notifications go through `LibNotify`, not custom UI banners.
