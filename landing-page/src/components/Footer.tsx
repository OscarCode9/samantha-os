export default function Footer() {
  return (
    <footer className="footer-section">
      <div className="footer-top">
        <div className="footer-wordmark">Samantha OS</div>
        <nav>
          <ul className="footer-links">
            <li><a href="#about">About</a></li>
            <li><a href="#docs">Docs</a></li>
            <li><a href="#github">GitHub</a></li>
            <li><a href="#privacy">Privacy</a></li>
          </ul>
        </nav>
      </div>
      <div className="footer-bottom">
        <p className="footer-fine">
          Samantha OS is an independent operating system. GitHub Copilot is a trademark of GitHub, Inc.
          Not affiliated with OpenAI, Anthropic, or any AI provider.
        </p>
      </div>
    </footer>
  );
}
