import { useTranslation } from 'react-i18next';

export default function Footer() {
  const { t } = useTranslation();

  return (
    <footer className="footer-section">
      <div className="footer-top">
        <div className="footer-wordmark">Samantha OS</div>
        <nav>
          <ul className="footer-links">
            <li><a href="#about">{t('footer.about')}</a></li>
            <li><a href="#docs">{t('footer.docs')}</a></li>
            <li><a href="#github">{t('footer.github')}</a></li>
            <li><a href="#privacy">{t('footer.privacy')}</a></li>
          </ul>
        </nav>
      </div>
      <div className="footer-bottom">
        <p className="footer-fine">
          {t('footer.finePrint')}
        </p>
      </div>
    </footer>
  );
}
