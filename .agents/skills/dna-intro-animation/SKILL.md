---
name: dna-intro-animation
description: "Port a canvas/WebGL DNA helix animation (slow → fast → morphs to circle → transitions) to a GTK4/Cairo Vala DrawingArea with frame-tick driven rendering. Use when asked to add a DNA animation, intro splash animation, canvas animation, or frame-by-frame Cairo animation to a GTK4 Vala component."
---

# DNA Intro Animation — GTK4 / Cairo / Vala

A 670-frame DNA helix animation that starts slow, accelerates, stretches the helix, morphs it into a circle, then auto-transitions to the next UI step. Ported from a vanilla HTML5 canvas animation.

---

## When to Use

- Adding a splash/intro animation before a GTK4 view shows its normal content
- Porting any HTML `<canvas>` + `requestAnimationFrame` animation to Vala
- Adding `Gtk.DrawingArea` + `add_tick_callback` rendering to any GTK4 widget

---

## Architecture

```
Step.INTRO
  ↓  (tick callback fires ~670 frames, then transitions via Idle.add)
Step.PRESENTATION  (or whatever the next step is)
```

The key GTK4 primitives:
- `Gtk.DrawingArea` — the canvas equivalent
- `set_draw_func` — replaces `ctx.clearRect` + your full draw call
- `add_tick_callback` — replaces `requestAnimationFrame`; return `true` to continue, `false` to stop
- `queue_draw()` — signals GTK to call draw_func on next frame
- `Gdk.FrameClock` — passed to tick callback (can read timestamps if needed)

---

## Step 1 — Add State Fields

Add to the class body (NOT inside a method — valac forbids nested declarations):

```vala
/* DNA intro animation state */
private Gtk.DrawingArea dna_canvas;
private int    anim_frame = 0;
private double anim_phase = 0.0;
private uint   tick_id    = 0;
```

Also add `INTRO` as the **first** value of the `Step` enum and set the initial step to `Step.INTRO`.

---

## Step 2 — Create DrawingArea in `build_ui()`

```vala
dna_canvas = new Gtk.DrawingArea () {
    halign        = CENTER,
    valign        = CENTER,
    hexpand       = true,
    vexpand       = true,
    content_width  = 520,   // logical size (scales with widget)
    content_height = 240
};
dna_canvas.add_css_class ("ai-dna-canvas");
dna_canvas.set_draw_func (draw_dna);

// Append BEFORE other children so it fills the view
center_box.append (dna_canvas);
```

---

## Step 3 — Handle INTRO in `update_step()`

At the TOP of `update_step`, always stop any running tick and hide the canvas:

```vala
if (tick_id != 0) {
    dna_canvas.remove_tick_callback (tick_id);
    tick_id = 0;
}
dna_canvas.visible = false;
```

Then for the INTRO branch, hide everything else and start the tick:

```vala
if (step == Step.INTRO) {
    split_box.add_css_class ("ai-presentation");
    // ... hide all other widgets ...
    dna_canvas.visible = true;

    anim_frame = 0;
    anim_phase = 0.0;
    tick_id = dna_canvas.add_tick_callback (on_anim_tick);
}
```

---

## Step 4 — Tick Callback (replaces requestAnimationFrame)

```vala
private bool on_anim_tick (Gtk.Widget widget, Gdk.FrameClock clock) {
    int cycle = anim_frame;
    double speed = 0;

    if (cycle < 180) {                        // slow ramp-up
        double p = (double) cycle / 180.0;
        speed = 0.03 + (0.12 - 0.03) * ease_in_out (p);
    } else if (cycle < 300) {                 // medium ramp-up
        double p = (double)(cycle - 180) / 120.0;
        speed = 0.12 + (0.32 - 0.12) * ease_in_out (p);
    } else if (cycle < 400) {                 // max speed
        speed = 0.32;
    } else if (cycle < 490) {                 // decelerate during morph
        double p = (double)(cycle - 400) / 90.0;
        speed = 0.32 * (1.0 - ease_out (p));
    }
    // frames 490-670: hold circle, speed=0

    anim_phase += speed;
    anim_frame++;
    dna_canvas.queue_draw ();

    if (anim_frame >= 670) {
        tick_id = 0;
        Idle.add (() => {           // must transition on main loop, not inside tick
            update_step (Step.PRESENTATION);
            return false;
        });
        return false;               // stop tick
    }
    return true;                    // keep ticking
}
```

> **Why `Idle.add`?** Calling `update_step` (which calls `remove_tick_callback`) from *inside* the tick callback is undefined behaviour. Schedule it on the next idle iteration instead.

---

## Step 5 — Draw Function (replaces the JS `render()` body)

The draw function receives the Cairo context and current pixel dimensions. Scale all hardcoded coordinates proportionally so the scene fills any canvas size:

```vala
private void draw_dna (Gtk.DrawingArea area, Cairo.Context cr, int width, int height) {
    double W  = (double) width;
    double H  = (double) height;
    double CX = W / 2.0;
    double CY = H / 2.0;
    double sc = Math.fmin (W / 680.0, H / 320.0);   // scale factor vs. reference size

    // Derive morph_p and stretch_p from anim_frame (same logic as on_anim_tick)
    int    cycle    = anim_frame;
    double morph_p  = 0, stretch_p = 0;
    // ... (mirror the phase conditions from on_anim_tick) ...

    // Background fill
    cr.set_source_rgb (0.769, 0.298, 0.208);   // #C44C35 coral
    cr.paint ();

    // Segment generation
    int    N      = 300;
    double AMP    = (44.0 + 71.0 * stretch_p) * sc;
    double THICK  = (12.0 + 5.0  * stretch_p) * sc;
    double R      = 78.0 * sc;
    double margin = 20.0 * sc;
    int max_segs  = 2 * (N - 1);

    double[] seg_x0 = new double[max_segs]; // ... allocate all 6 arrays
    // ... fill arrays for r=0 and r=1 strands ...
    // ... z-sort with insertion sort on a parallel index array ...
    // ... render back-to-front with cr.move_to / cr.line_to / cr.fill ...
}
```

### Z-sorting without GLib.List overhead

Use a plain `int[]` index array and insertion sort — fast for 600 small segments:

```vala
int[] order = new int[seg_count];
for (int i = 0; i < seg_count; i++) order[i] = i;
for (int i = 1; i < seg_count; i++) {
    int key = order[i]; double kz = seg_z[key]; int j = i - 1;
    while (j >= 0 && seg_z[order[j]] > kz) { order[j+1] = order[j]; j--; }
    order[j+1] = key;
}
```

### Segment rendering (depth shading + specular highlight)

```vala
double b     = (seg_z[idx] + 1.0) / 2.0;       // 0..1 brightness
double alpha = 0.3 + 0.7 * b;
double t     = seg_th[idx];                     // half-thickness

// Compute four corners of the thick segment
double p0tx = seg_x0[idx] + nx*t,  p0ty = seg_y0[idx] + ny*t;
double p0bx = seg_x0[idx] - nx*t,  p0by = seg_y0[idx] - ny*t;
double p1tx = seg_x1[idx] + nx*t,  p1ty = seg_y1[idx] + ny*t;
double p1bx = seg_x1[idx] - nx*t,  p1by = seg_y1[idx] - ny*t;

cr.move_to (p0tx, p0ty); cr.line_to (p1tx, p1ty);
cr.line_to (p1bx, p1by); cr.line_to (p0bx, p0by);
cr.close_path ();
cr.set_source_rgba (
    (175.0 + 55.0*b) / 255.0,
    (130.0 + 65.0*b) / 255.0,
    (115.0 + 60.0*b) / 255.0,
    alpha
);
cr.fill ();

// Specular highlight on front-facing segments
if (seg_z[idx] > 0.1) {
    double mx0 = (p0tx+p0bx)/2.0, my0 = (p0ty+p0by)/2.0;
    double mx1 = (p1tx+p1bx)/2.0, my1 = (p1ty+p1by)/2.0;
    cr.move_to (p0tx, p0ty); cr.line_to (p1tx, p1ty);
    cr.line_to (mx1, my1);   cr.line_to (mx0, my0);
    cr.close_path ();
    cr.set_source_rgba (1.0, 0.882, 0.843, seg_z[idx] * 0.5);
    cr.fill ();
}
```

---

## Step 6 — Easing Helpers

Pure static methods, no GTK dependency:

```vala
private static double ease_in_out (double x) {
    if (x < 0.5) return 4.0 * x * x * x;
    return 1.0 - Math.pow (-2.0 * x + 2.0, 3.0) / 2.0;
}

private static double ease_out (double x) {
    return 1.0 - Math.pow (1.0 - x, 3.0);
}
```

---

## Step 7 — CSS

GTK4 CSS constraints apply — no `box-shadow`, no `%` keyframes, use `from`/`to` for animations:

```css
.ai-dna-canvas {
    background-color: #C44C35;
    border-radius: 12px;
}
```

The canvas draws its own background via `cr.paint()`, so no additional GTK theme interference occurs.

---

## Porting Checklist (JS canvas → Vala Cairo)

| JS concept | Vala / GTK4 equivalent |
|---|---|
| `requestAnimationFrame(render)` | `add_tick_callback(on_anim_tick)` — return `true` to continue |
| `ctx.clearRect(0,0,W,H)` + fill | `cr.paint()` after `cr.set_source_rgb(...)` |
| `ctx.beginPath()` + draw + `ctx.fill()` | `cr.move_to/line_to/close_path` + `cr.fill()` |
| `ctx.fillStyle = 'rgba(...)'` | `cr.set_source_rgba(r,g,b,a)` with 0.0–1.0 range |
| `canvas.width`, `canvas.height` | `width`, `height` params passed to `draw_func` |
| `Math.sqrt/sin/cos/pow/abs` | `Math.sqrt/sin/cos/pow/fabs` (Vala: `Math.fabs` not `Math.abs` for doubles) |
| Module-level `frame`, `phase` vars | Private class fields (Vala has no module scope) |
| `frame = 0` on loop reset | Call `update_step(Step.INTRO)` to re-enter — fields reset there |

---

## Common Pitfalls

- **`Math.abs` vs `Math.fabs`** — `Math.abs` on a `double` in Vala is ambiguous; use `Math.fabs`.
- **Tick callback called from itself** — never call `remove_tick_callback` inside the callback. Use `Idle.add` to schedule the transition.
- **`add_tick_callback` returns a `uint` handle** — store it in a class field so you can cancel it in `update_step` when the user skips or navigates away.
- **`content_width`/`content_height` vs `width_request`** — for `DrawingArea`, use `content_width`/`content_height` to set the logical size; `width_request` sets the minimum allocated size.
- **`sc` scaling** — always multiply pixel coordinates by `Math.fmin(W/REF_W, H/REF_H)` so the scene renders identically regardless of widget size.
- **Background transparency** — if you don't call `cr.paint()` with a solid color first, GTK may render the previous frame or theme background underneath.
