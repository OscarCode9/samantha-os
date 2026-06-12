import { useTranslation } from 'react-i18next';

export default function Hero() {
  const { t } = useTranslation();

  return (
    <section className="hero-section section">
      <div className="hero-content">
        <div>
          <div className="hero-eyebrow">{t('hero.eyebrow')}</div>
          <h1 className="hero-title">
            {t('hero.title')}
          </h1>
          <p className="hero-subtitle">
            {t('hero.subtitle')}
          </p>
          <div className="hero-ctas">
            <a href="#docs" className="btn-primary">{t('hero.tryBeta')}</a>
            <a href="#film" className="btn-ghost">{t('hero.watchFilm')}</a>
          </div>
          <div className="hero-trust">
            {t('hero.trust')}
          </div>
        </div>

        <div className="device-mockup">
          <div className="mockup-title">{t('hero.mockupTitle')}</div>
          <div className="mockup-subtitle">{t('hero.mockupSubtitle')}</div>
          <div className="mockup-code">ABCD-EFGH</div>
          <div className="mockup-hint">github.com/login/device</div>
        </div>
      </div>
    </section>
  );
}
