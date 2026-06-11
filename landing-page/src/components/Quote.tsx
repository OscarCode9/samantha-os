import { useTranslation } from 'react-i18next';

export default function Quote() {
  const { t } = useTranslation();

  return (
    <section className="quote-section section">
      <div className="quote-content">
        <blockquote className="quote-text">
          {t('quote.text')}
        </blockquote>
        <cite className="quote-attribution">{t('quote.author')}</cite>
      </div>
    </section>
  );
}
