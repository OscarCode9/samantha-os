export default function Hero() {
  return (
    <section className="hero-section section">
      <div className="hero-content">
        <div>
          <div className="hero-eyebrow">Samantha OS</div>
          <h1 className="hero-title">
            Your OS finally has a mind.
          </h1>
          <p className="hero-subtitle">
            An AI assistant that lives in your computer — not in a browser tab.
          </p>
          <div className="hero-ctas">
            <a href="#preorder" className="btn-primary">Pre-order Samantha OS</a>
            <a href="#film" className="btn-ghost">Watch the film</a>
          </div>
          <div className="hero-trust">
            Built for Samantha OS. GitHub Copilot powered.
          </div>
        </div>

        <div className="device-mockup">
          <div className="mockup-title">Samantha OS.</div>
          <div className="mockup-subtitle">The first AI-native operating system.</div>
          <div className="mockup-code">ABCD-EFGH</div>
          <div className="mockup-hint">github.com/login/device</div>
        </div>
      </div>
    </section>
  );
}
