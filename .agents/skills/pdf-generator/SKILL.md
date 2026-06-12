---
name: pdf-generator
description: "Generate beautiful, professionally styled PDF documents from HTML/CSS using WeasyPrint. Includes 5 pre-configured premium styles: Professional, Samantha, Cyberpunk, Acid Gradient, and Brutalist."
---

# PDF Generator Skill

Use this skill when you need to generate a PDF document for the user. It leverages the installed `weasyprint` utility to convert HTML and CSS files into high-quality PDFs.

> [!IMPORTANT]
> **STYLE CONFIRMATION**: Always ask the user which design style they prefer before generating the PDF, unless they explicitly requested one in their query. Present the 5 style choices with a brief, attractive description.

---

## 1. Quick Start

To generate a PDF, write your content in a clean HTML file, choose one of the pre-configured stylesheets from the `templates/` directory, and run `weasyprint`:

```bash
# On macOS Host:
weasyprint input.html output.pdf -s /Users/oscarcode/elementary-claw/.agents/skills/pdf-generator/templates/samantha.css

# On elementary OS VM:
weasyprint input.html output.pdf -s /home/oscar12345678/.openclaw/skills/bundled/pdf-generator/templates/samantha.css
```

---

## 2. Default Styles

The CSS template stylesheets are located at:
- **macOS Host**: `/Users/oscarcode/elementary-claw/.agents/skills/pdf-generator/templates/`
- **elementary OS VM**: `/home/oscar12345678/.openclaw/skills/bundled/pdf-generator/templates/`

Available Styles:
1.  **Professional (`professional.css`)**: Clean, corporate, elegant layout with deep navy highlights, serif body, and sans-serif headings. Ideal for reports, invoices, and resumes.
2.  **Samantha (`samantha.css`)**: Cozy, futuristic brand theme using warm coral (`#C44C35`), soft dot-grids, rounded card containers, and clean white typography.
3.  **Minimalist Cyberpunk (`cyberpunk.css`)**: High-contrast dark mode with neon cyan and green accents, monospace text, scanning lines, and technical borders.
4.  **Acid Gradient (`acid.css`)**: Bold, modern fluid layout with vibrant hot pink, purple, and neon yellow gradients, large type, and creative brutalist spacing.
5.  **Brutalist / Nothing (`brutalist.css`)**: Stark, raw monochromatic look with thick black borders, dotted grids, monospace typography, and pure red accents.

---

## 3. How to write compatible HTML

Since WeasyPrint parses HTML and CSS directly, you can write standard HTML elements. To get the best layout:

-   Wrap the main content in `<main>` or `<article>`.
-   Use `class="card"` to wrap distinct blocks (especially for Samantha, Cyberpunk, and Brutalist styles).
-   Use standard page break helper classes in your CSS if needed (e.g. `page-break-before: always`).
