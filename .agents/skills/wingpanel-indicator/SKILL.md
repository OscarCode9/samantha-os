---
name: wingpanel-indicator
description: 'Build, style, and deploy GTK3 Wingpanel indicators for elementary OS. Use when: creating a panel indicator, styling a GTK3 popover, fixing white borders/shadow in Wingpanel, deploying .so to the VM, restarting Wingpanel, applying css via CssProvider, troubleshooting entry frames or popover borders.'
argument-hint: 'What do you want to do? e.g. "new indicator", "fix popover style", "deploy to VM"'
---

# Wingpanel Indicator — Build, Style & Deploy

## When to Use
- Creating a new `Wingpanel.Indicator` subclass in Vala + GTK3
- Styling popover, entry, button, separator, shadow, border
- Running the deploy cycle: macOS → VM → ninja → restart Wingpanel
- Debugging CSS issues (white borders, focus ring, box-shadow clipping)

---

## Project Structure

```
panel-sam/
├── meson.build
└── src/
    └── Indicator.vala
```

- **Build output**: `/usr/lib/aarch64-linux-gnu/wingpanel/libsam.so`
- **VM**: `oscarcode91@192.168.64.5` (password `12345`), graphical session as `oscar`
- **Bundle script**: `./vm/package-vm-bundle.sh` — packs everything into `vm/dist/elementary-claw-vm-share.tar.gz`
- **Bundle path on VM**: `/tmp/elementary-claw-vm/panel-sam/`

---

## Deploy Cycle (macOS → VM → live)

```bash
# 1. Pack
./vm/package-vm-bundle.sh

# 2. Copy to VM
sshpass -p '12345' scp -o StrictHostKeyChecking=no \
  vm/dist/elementary-claw-vm-share.tar.gz oscarcode91@192.168.64.5:/tmp/

# 3. Build + install on VM
sshpass -p '12345' ssh -o StrictHostKeyChecking=no oscarcode91@192.168.64.5 '
  cd /tmp && rm -rf elementary-claw-vm && \
  tar -xzf /tmp/elementary-claw-vm-share.tar.gz 2>/dev/null && \
  cd elementary-claw-vm/panel-sam && rm -rf build && \
  meson setup build --prefix=/usr 2>&1 | tail -2 && \
  ninja -C build 2>&1 | grep -E "error:|Compiling|Linking|FAILED" && \
  echo "12345" | sudo -S ninja -C build install 2>&1 | grep -v "^sudo" | tail -2
'

# 4. Restart Wingpanel (must run as user oscar)
sshpass -p '12345' ssh -o StrictHostKeyChecking=no -o PasswordAuthentication=yes \
  oscar@192.168.64.5 'killall io.elementary.wingpanel 2>/dev/null; sleep 3; pgrep -f wingpanel | head -1'
```

> **Note**: `meson` on the VM does NOT support `-q` flag. Never use `meson setup build -q`.

---

## Wingpanel Architecture (GTK3)

| Method | Wrapping | Rules |
|---|---|---|
| `get_display_widget()` | `ToggleButton` by Wingpanel | Passive only — clicks toggle the popover, not your widget |
| `get_widget()` | Popover content | All interactive widgets go here |
| `opened()` | Called on popover open | Use to `grab_focus()` on entry |
| `closed()` | Called on popover close | Use to cancel pending async tasks |

**Key rule**: Never put `Gtk.Entry` or interactive widgets in `get_display_widget()` — Wingpanel intercepts all clicks there. All input must be in `get_widget()` (the popover).

---

## GTK3 CSS — Proven Techniques

### Load CSS in `construct`
```vala
construct {
    visible = true;
    load_css ();
}

private void load_css () {
    var provider = new Gtk.CssProvider ();
    try {
        provider.load_from_data (MY_CSS, -1);
    } catch (Error e) {
        GLib.warning ("CSS error: %s", e.message);
    }
    unowned Gdk.Screen? screen = Gdk.Screen.get_default ();
    if (screen != null) {
        Gtk.StyleContext.add_provider_for_screen (
            screen, provider, Gtk.STYLE_PROVIDER_PRIORITY_APPLICATION
        );
    }
}
```

### Kill white borders on `Gtk.Entry`
The entry border comes from the GTK3 *widget frame*, not CSS. Fix both levels:

**Vala** — disable the frame widget-side:
```vala
query_entry = new Gtk.Entry () {
    has_frame = false,   // ← KEY: kills frame before CSS
    ...
};
```

**CSS** — nuke every state:
```css
entry.my-entry,
entry.my-entry:focus,
entry.my-entry:active,
entry.my-entry:hover {
    background-color: transparent;
    border: none;
    border-image: none;
    border-radius: 0;
    box-shadow: none;
    outline: none;
    -gtk-outline-radius: 0;
}
entry.my-entry text,
entry.my-entry:focus text {
    background-color: transparent;
    box-shadow: none;
}
```

### Separator line color
In GTK3, separators use `color` (not `background-color`) for the line:
```css
separator.my-divider {
    color: rgba(255, 255, 255, 0.18);
    background-color: rgba(255, 255, 255, 0.18);  /* belt + suspenders */
    min-height: 1px;
}
```

### Popover elevation shadow (elementary OS technique)
The `margin: 6px` is required — it gives the shadow render space so it doesn't clip:
```css
popover.background,
popover.background > contents {
    background-color: #C44C35;
    background-clip: padding-box;
    border: 1px solid rgba(0, 0, 0, 0.22);
    border-radius: 6px;
    box-shadow: 0 3px 4px rgba(0, 0, 0, 0.15),
                0 3px 3px -3px rgba(0, 0, 0, 0.35);
    margin: 6px;   /* ← KEY: shadow needs this space or it clips */
    padding: 0;
}
```

> This is the exact technique from `io.elementary.stylesheet` — two subtle shadow layers + margin breathing room.

---

## `meson.build` Template

```meson
project('panel-sam', 'vala', 'c', meson_version: '>= 1.0.0')

wingpanel_dep = dependency('wingpanel')
indicatorsdir = wingpanel_dep.get_variable(pkgconfig: 'indicatorsdir')

deps = [
    dependency('glib-2.0'),
    dependency('gobject-2.0'),
    dependency('gtk+-3.0'),
    dependency('libsoup-3.0'),
    dependency('json-glib-1.0'),
    wingpanel_dep,
]

shared_module('sam',
    'src/Indicator.vala',
    dependencies: deps,
    install: true,
    install_dir: indicatorsdir,
)
```

> Use `pkgconfig: 'indicatorsdir'` (NOT `pkgconfig_define`) to get the correct install path.

---

## Common Errors & Fixes

| Error | Cause | Fix |
|---|---|---|
| `meson: error: unrecognized arguments: -q` | VM meson doesn't support `-q` | Remove the flag |
| White border on `Gtk.Entry` | GTK3 widget frame | `has_frame = false` in Vala + CSS override |
| Shadow clips at edges | No margin on popover | `margin: 6px` on `popover.background` |
| Entry text invisible | Theme color override | Set `color: #FFFFFF` on both `entry` and `entry text` nodes |
| Popover shows theme background | `.sam-popover` class not applied | Call `popover_box.get_style_context().add_class("sam-popover")` |
| Can't type in display widget | Wingpanel ToggleButton intercepts | Move all input to `get_widget()` popover |
| `killall io.elementary.wingpanel` fails | Wingpanel runs as user `oscar`, not `oscarcode91` | SSH as `oscar` (passwordless) to kill it |
