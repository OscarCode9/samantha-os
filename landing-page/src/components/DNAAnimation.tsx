import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

/* ------------------------------------------------------------------ */
/*  Exact port of AIConnectView.vala DNA intro + PRESENTATION step     */
/*  DNA animation plays → "Samantha OS." appears with slide-up-hero    */
/* ------------------------------------------------------------------ */

const PRESENTATION_START_FRAME = 490;
const IDLE_LOOP_FRAME = 670;

function easeInOut(x: number): number {
  if (x < 0.5) return 4.0 * x * x * x;
  return 1.0 - Math.pow(-2.0 * x + 2.0, 3.0) / 2.0;
}

function easeOut(x: number): number {
  return 1.0 - Math.pow(1.0 - x, 3.0);
}

function drawDNA(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  animFrame: number,
  animPhase: number
) {
  const W = width;
  const H = height;
  const CX = W / 2.0;
  const CY = H / 2.0;
  const sc = Math.min(W / 680.0, H / 320.0);

  const cycle = animFrame;
  let morph_p = 0;
  let stretch_p = 0;

  if (cycle < 180) {
    const p = cycle / 180.0;
    stretch_p = easeInOut(p) * 0.25;
  } else if (cycle < 300) {
    const p = (cycle - 180) / 120.0;
    stretch_p = 0.25 + easeInOut(p) * 0.45;
  } else if (cycle < 400) {
    const p = (cycle - 300) / 100.0;
    stretch_p = 0.70 + p * 0.30;
  } else if (cycle < 490) {
    const p = (cycle - 400) / 90.0;
    stretch_p = 1.0 - easeInOut(p);
    morph_p = p;
  } else {
    stretch_p = 0;
    morph_p = 1.0;
  }

  // Keep the transform set by the caller so high-DPI/mobile canvases
  // render across the full viewport instead of collapsing into a corner.
  ctx.fillStyle = 'rgb(196, 76, 53)';
  ctx.fillRect(0, 0, W, H);

  const N = 300;
  const AMP = (44.0 + 71.0 * stretch_p) * sc;
  const THICK = (12.0 + 5.0 * stretch_p) * sc;
  const FREQ = 2.0;
  const R = 78.0 * sc;
  const margin = 20.0 * sc;
  const maxSegs = 2 * (N - 1);
  let segCount = 0;

  const segX0 = new Float64Array(maxSegs);
  const segY0 = new Float64Array(maxSegs);
  const segX1 = new Float64Array(maxSegs);
  const segY1 = new Float64Array(maxSegs);
  const segZ = new Float64Array(maxSegs);
  const segTh = new Float64Array(maxSegs);

  for (let r = 0; r < 2; r++) {
    const po = r * Math.PI;
    for (let i = 0; i < N - 1; i++) {
      const s0 = i / (N - 1);
      const s1 = (i + 1) / (N - 1);

      const hx0 = margin + s0 * (W - 2.0 * margin);
      const hx1 = margin + s1 * (W - 2.0 * margin);
      const a0 = FREQ * Math.PI * 2.0 * s0 + animPhase + po;
      const a1 = FREQ * Math.PI * 2.0 * s1 + animPhase + po;
      const hy0 = CY + AMP * Math.sin(a0);
      const hy1 = CY + AMP * Math.sin(a1);
      const hz0 = Math.cos(a0);
      const hz1 = Math.cos(a1);

      const ca0 = s0 * Math.PI * 2.0 - Math.PI / 2.0 + po;
      const ca1 = s1 * Math.PI * 2.0 - Math.PI / 2.0 + po;
      const cx0 = CX + R * Math.cos(ca0);
      const cy0 = CY + R * Math.sin(ca0);
      const cx1 = CX + R * Math.cos(ca1);
      const cy1 = CY + R * Math.sin(ca1);
      const cz0 = Math.sin(ca0 + animPhase * 0.08);
      const cz1 = Math.sin(ca1 + animPhase * 0.08);

      const mp = easeInOut(morph_p);
      const x0 = hx0 + (cx0 - hx0) * mp;
      const y0 = hy0 + (cy0 - hy0) * mp;
      const x1 = hx1 + (cx1 - hx1) * mp;
      const y1 = hy1 + (cy1 - hy1) * mp;
      const z0 = hz0 + (cz0 - hz0) * mp;
      const z1 = hz1 + (cz1 - hz1) * mp;
      const zAvg = (z0 + z1) / 2.0;
      const th = THICK * (0.45 + 0.55 * Math.abs(zAvg));

      segX0[segCount] = x0;
      segY0[segCount] = y0;
      segX1[segCount] = x1;
      segY1[segCount] = y1;
      segZ[segCount] = zAvg;
      segTh[segCount] = th;
      segCount++;
    }
  }

  const order = new Int32Array(segCount);
  for (let i = 0; i < segCount; i++) order[i] = i;
  for (let i = 1; i < segCount; i++) {
    const key = order[i];
    const kz = segZ[key];
    let j = i - 1;
    while (j >= 0 && segZ[order[j]] > kz) {
      order[j + 1] = order[j];
      j--;
    }
    order[j + 1] = key;
  }

  for (let i = 0; i < segCount; i++) {
    const idx = order[i];
    const dx = segX1[idx] - segX0[idx];
    const dy = segY1[idx] - segY0[idx];
    let len = Math.sqrt(dx * dx + dy * dy);
    if (len < 0.001) len = 1.0;
    const nx = -dy / len;
    const ny = dx / len;
    const b = (segZ[idx] + 1.0) / 2.0;
    const alpha = 0.3 + 0.7 * b;
    const t = segTh[idx];

    const p0tx = segX0[idx] + nx * t;
    const p0ty = segY0[idx] + ny * t;
    const p0bx = segX0[idx] - nx * t;
    const p0by = segY0[idx] - ny * t;
    const p1tx = segX1[idx] + nx * t;
    const p1ty = segY1[idx] + ny * t;
    const p1bx = segX1[idx] - nx * t;
    const p1by = segY1[idx] - ny * t;

    ctx.beginPath();
    ctx.moveTo(p0tx, p0ty);
    ctx.lineTo(p1tx, p1ty);
    ctx.lineTo(p1bx, p1by);
    ctx.lineTo(p0bx, p0by);
    ctx.closePath();
    ctx.fillStyle = `rgba(${Math.round(175 + 55 * b)}, ${Math.round(130 + 65 * b)}, ${Math.round(115 + 60 * b)}, ${alpha})`;
    ctx.fill();

    if (segZ[idx] > 0.1) {
      const mx0 = (p0tx + p0bx) / 2.0;
      const my0 = (p0ty + p0by) / 2.0;
      const mx1 = (p1tx + p1bx) / 2.0;
      const my1 = (p1ty + p1by) / 2.0;
      ctx.beginPath();
      ctx.moveTo(p0tx, p0ty);
      ctx.lineTo(p1tx, p1ty);
      ctx.lineTo(mx1, my1);
      ctx.lineTo(mx0, my0);
      ctx.closePath();
      ctx.fillStyle = `rgba(255, 225, 215, ${segZ[idx] * 0.5})`;
      ctx.fill();
    }
  }
}

interface DNAAnimationProps {
  onComplete?: () => void;
}

export default function DNAAnimation({ onComplete }: DNAAnimationProps) {
  const { t } = useTranslation();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const frameRef = useRef(0);
  const phaseRef = useRef(0);
  const rafRef = useRef(0);
  const [showPresentation, setShowPresentation] = useState(false);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const resize = () => {
      const parent = canvas.parentElement;
      if (!parent) return;
      const dpr = window.devicePixelRatio || 1;
      const w = parent.clientWidth;
      const h = parent.clientHeight;
      canvas.width = w * dpr;
      canvas.height = h * dpr;
      canvas.style.width = w + 'px';
      canvas.style.height = h + 'px';
    };

    resize();
    window.addEventListener('resize', resize);

    const animate = () => {
      const parent = canvas.parentElement;
      if (!parent) return;
      const w = parent.clientWidth;
      const h = parent.clientHeight;
      const dpr = window.devicePixelRatio || 1;

      const cycle = frameRef.current;
      let speed = 0;

      if (cycle < 180) {
        const p = cycle / 180.0;
        speed = 0.03 + (0.12 - 0.03) * easeInOut(p);
      } else if (cycle < 300) {
        const p = (cycle - 180) / 120.0;
        speed = 0.12 + (0.32 - 0.12) * easeInOut(p);
      } else if (cycle < 400) {
        speed = 0.32;
      } else if (cycle < 490) {
        const p = (cycle - 400) / 90.0;
        speed = 0.32 * (1.0 - easeOut(p));
      }

      phaseRef.current += speed;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      drawDNA(ctx, w, h, cycle, phaseRef.current);
      frameRef.current++;

      if (frameRef.current >= PRESENTATION_START_FRAME && !showPresentation) {
        setShowPresentation(true);
        if (onComplete) {
          onComplete();
        }
        return;
      }

      if (!showPresentation) {
        rafRef.current = requestAnimationFrame(animate);
      } else {
        const idle = () => {
          const p = canvas.parentElement;
          if (!p) return;
          const w2 = p.clientWidth;
          const h2 = p.clientHeight;
          const dpr2 = window.devicePixelRatio || 1;
          phaseRef.current += 0.012;
          ctx.setTransform(dpr2, 0, 0, dpr2, 0, 0);
          drawDNA(ctx, w2, h2, IDLE_LOOP_FRAME, phaseRef.current);
          rafRef.current = requestAnimationFrame(idle);
        };
        rafRef.current = requestAnimationFrame(idle);
      }
    };

    rafRef.current = requestAnimationFrame(animate);

    return () => {
      cancelAnimationFrame(rafRef.current);
      window.removeEventListener('resize', resize);
    };
  }, [showPresentation]);

  return (
    <section
      style={{
        width: '100%',
        minHeight: '100vh',
        height: '100svh',
        background: '#C44C35',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      <canvas
        ref={canvasRef}
        style={{
          position: 'absolute',
          inset: 0,
          width: '100%',
          height: '100%',
        }}
      />

      {showPresentation && (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '0 24px',
          }}
        >
          <h1
            style={{
              fontFamily: '"Space Grotesk", sans-serif',
              fontSize: 72,
              fontWeight: 700,
              letterSpacing: '-0.03em',
              color: '#FFFFFF',
              margin: 0,
              animation: 'slide-up-hero 1000ms cubic-bezier(0.16, 1, 0.3, 1) 0ms both',
            }}
          >
            {t('hero.mockupTitle')}
          </h1>
          <h2
            style={{
              fontFamily: '"Space Grotesk", sans-serif',
              fontSize: 18,
              fontWeight: 300,
              letterSpacing: '0.01em',
              color: 'rgba(255, 255, 255, 0.65)',
              margin: '12px 0 0 0',
              animation: 'slide-up-hero 1000ms cubic-bezier(0.16, 1, 0.3, 1) 150ms both',
            }}
          >
            {t('hero.mockupSubtitle')}
          </h2>
          <a
            href="#hero"
            style={{
              fontFamily: '"Space Mono", monospace',
              fontSize: 11,
              fontWeight: 700,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'rgba(255, 255, 255, 0.70)',
              background: 'transparent',
              border: '1px solid rgba(255, 255, 255, 0.18)',
              borderRadius: 999,
              minHeight: 32,
              padding: '6px 24px',
              marginTop: 32,
              textDecoration: 'none',
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              animation: 'slide-up-hero 1000ms cubic-bezier(0.16, 1, 0.3, 1) 300ms both',
              transition: 'background-color 250ms cubic-bezier(0.25, 0.1, 0.25, 1), border-color 250ms cubic-bezier(0.25, 0.1, 0.25, 1)',
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.backgroundColor = 'rgba(255,255,255,0.08)';
              e.currentTarget.style.borderColor = 'rgba(255,255,255,0.35)';
              e.currentTarget.style.color = '#FFFFFF';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = 'transparent';
              e.currentTarget.style.borderColor = 'rgba(255,255,255,0.18)';
              e.currentTarget.style.color = 'rgba(255,255,255,0.70)';
            }}
          >
            {t('hero.getStarted')}
          </a>
        </div>
      )}
    </section>
  );
}
