# Copilot Instructions — elementary-claw

## Reference Documentation

When working with Vala, GTK 4, Granite 7, or elementary OS code in this repository, **always** consult the official elementary developer documentation before suggesting fixes or generating code:

- **elementary Developer Docs**: https://docs.elementary.io/develop
- **Valadoc (API reference)**: https://valadoc.org
- **GTK 4 Reference**: https://docs.gtk.org/gtk4/
- **Granite 7 Reference**: https://valadoc.org/granite-7/Granite.html

### Source-first Rule

Before writing or modifying any Vala/GTK4 code, **always look at the existing source files** in `references/initial-setup/src/` to understand how things are already done in this project. Use them as the reference pattern — do not guess or invent Vala idioms. When unsure about a GTK4 widget, CSS property, or Vala construct, search the existing `.vala` files first, then consult Valadoc or docs.gtk.org.

Key source files to check:
- `src/Views/AIConnectView.vala` — main AI onboarding view (steps, CSS classes, widgets)
- `src/Views/AbstractInstallerView.vala` — base class (split_box, content_area, action_area)
- `src/Application.vala` — CSS provider loading, font registration
- `data/styles/nothing-installer.css` — all custom CSS classes and animations

## Vala / GTK 4 / Granite 7 Rules

- `valac` rejects `private ... { get; construct; }` — construct properties MUST be `public`.
- For internal-only data, use plain private fields + `Object()` + assign in constructor body + call `build_ui()`.
- Vala does NOT support nested method declarations. If a method references widgets created in `build_ui()`, those widgets must be **private class fields**, not `var` locals.
- Nullable strings: normalize with `value ?? ""` before widget construction.
- `Granite.BackButton` has a valac codegen bug — use `Gtk.Button.with_label()` + `add_css_class("back-button")` instead.
- `Gtk.StyleContext.add_provider_for_display` is deprecated in GTK 4.10 — use a `[CCode]` extern wrapper.
- **NEVER use literal Unicode curly quotes** (`"` `"` `'` `'`) in Vala string literals — valac interprets them as string delimiters, causing cryptic parse errors. Always use `\u201c`, `\u201d`, `\u2018`, `\u2019` escape sequences instead. Run `python3 -c "import re; [print(f'{i}: {l.rstrip()}') for i,l in enumerate(open('FILE.vala'),1) if re.search(r'[\u2018\u2019\u201c\u201d]',l)]"` to detect them.

## Granite 7.7.0 CSS Class Migration

| Deprecated constant | Replacement |
|---|---|
| `Granite.STYLE_CLASS_SUGGESTED_ACTION` | `Granite.CssClass.SUGGESTED` |
| `Granite.STYLE_CLASS_ERROR` | `Granite.CssClass.ERROR` |
| `Granite.STYLE_CLASS_H2_LABEL` | `Granite.HeaderLabel.Size.H2.to_string()` |
| `Granite.STYLE_CLASS_BACK_BUTTON` | `Gtk.Button.with_label()` + CSS class |

## Build Pipeline

- Code is edited on macOS, packaged via `./vm/package-vm-bundle.sh`, and built on an elementary OS VM via meson + ninja.
- When fixing Vala errors, always verify the fix compiles with `ninja -C build` on the VM before marking as done.

### VM Deploy Workflow (CRITICAL)

The full command to deploy and test on the VM is:

```bash
cd ~ && tar xzf /mnt/utm/elementary-claw-vm-share.tar.gz && cd references/initial-setup && sudo rm -rf build && meson build && sudo ninja -C build install
```

Then run with:

```bash
xhost +local: && sudo GTK_A11Y=none NO_AT_BRIDGE=1 GTK_IM_MODULE=gtk-im-context-simple GSETTINGS_BACKEND=memory GSK_RENDERER=cairo DISPLAY="$DISPLAY" XAUTHORITY="$XAUTHORITY" HOME="$HOME" io.elementary.initial-setup
```

**Running the app on the VM (VERIFIED WORKING):**

```bash
xhost +local: && sudo GTK_A11Y=none NO_AT_BRIDGE=1 GTK_IM_MODULE=gtk-im-context-simple GSETTINGS_BACKEND=memory GSK_RENDERER=cairo DISPLAY="$DISPLAY" XAUTHORITY="$XAUTHORITY" HOME="$HOME" io.elementary.initial-setup
```

Why each variable is needed:
- `xhost +local:` — allows root to connect to the X11 display
- `GTK_A11Y=none` — disables accessibility bridge (crashes under root)
- `NO_AT_BRIDGE=1` — disables AT-SPI DBus bridge
- `GTK_IM_MODULE=gtk-im-context-simple` — avoids ibus crash (owner mismatch under root)
- `GSETTINGS_BACKEND=memory` — avoids dconf crash (can't write to user's dconf as root)
- `GSK_RENDERER=cairo` — forces Cairo rendering instead of GL (fixes black window in VM)
- `DISPLAY`, `XAUTHORITY`, `HOME` — explicit X11 forwarding without `-E` (which leaks `DBUS_SESSION_BUS_ADDRESS` and causes ibus/dconf crashes)

> **NEVER use `sudo -E`** — it forwards `DBUS_SESSION_BUS_ADDRESS` which causes ibus and dconf to connect to the user's session bus before `main()` runs, causing segfaults.

**Common pitfalls:**

1. **Must run `sudo ninja -C build install`** — `ninja -C build` alone only compiles to `build/src/`. Without `install`, running `io.elementary.initial-setup` executes the **old system binary** at `/usr/bin/`, not the newly compiled one. Alternatively, run the binary directly: `sudo ./build/src/io.elementary.initial-setup`.
2. **`ninja: no work to do`** — The tar preserves macOS timestamps which may be older than the VM binary. Fix with: `touch src/Views/AIConnectView.vala && ninja -C build`.
3. **Mount the shared folder first** (if `/mnt/utm` is empty): `sudo mount -t 9p -o trans=virtio,version=9p2000.L share /mnt/utm`.
4. **`MESA: error: Failed to attach to x11 shm`** — Harmless noise from software renderer. Suppress with `2>&1 | grep -v "MESA: error"` if desired.
5. **Black window** — Missing `GSK_RENDERER=cairo`. GTK4's GL renderer fails under root in the VM.
6. **Segfault at password/account step** — Missing `GTK_A11Y=none` or `GTK_IM_MODULE=gtk-im-context-simple`. Accessibility and ibus crash under root.

### GTK 4 CSS Rules

- GTK 4 does **NOT** support percentage keyframe selectors (`0%, 100%`). Use only `from` and `to`. For bounce/pulse effects, use `from`/`to` with `animation-direction: alternate`.
- GTK 4 does **NOT** support `box-shadow` — use `outline` or `border`.
- GTK 4 does **NOT** support `gap` — use `border-spacing` on parent.
- GTK 4 does **NOT** support 2-value padding shorthand (`10px 20px`) — use `padding-top`/`right`/`bottom`/`left` individually.
- Button label color: GTK 4 themes override `color` on button. Always target **both** `button.class` and `button.class label` selectors, and add `background-image: none` to prevent theme gradients.
- **Overriding elementary/Adwaita white backgrounds**: Targeted selectors like `frame navigation-view-page > scrolledwindow` lose to the theme's class-qualified selectors. Use **`frame *` wildcard** to force transparent on everything inside a frame. Always include `background-image: none` alongside `background-color`.
- `Adw.NavigationPage` CSS node = `navigation-view-page` (NOT `navigation-page`). `Adw.NavigationView` = `navigation-view`.
- `frame > navigation-view` won't match — GTK4 frame has a `border` child node between frame and its content. Use `frame navigation-view` (descendant) instead.

## Go Backend (elementary-claw)

- Go module at root: `cmd/claw/main.go` → `internal/app/` → `internal/runtime/` (HTTP gateway).
- Skills system: `internal/skills/` with YAML frontmatter manifests, multi-source loading, hot-reload.
- Run tests with `go test ./...` from the repo root.
