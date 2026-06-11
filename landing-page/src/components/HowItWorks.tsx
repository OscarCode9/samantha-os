import { useTranslation } from 'react-i18next';

const steps = [
  {
    number: '01',
    titleKey: 'how.step1Title',
    descKey: 'how.step1Desc',
  },
  {
    number: '02',
    titleKey: 'how.step2Title',
    descKey: 'how.step2Desc',
  },
  {
    number: '03',
    titleKey: 'how.step3Title',
    descKey: 'how.step3Desc',
  },
  {
    number: '04',
    titleKey: 'how.step4Title',
    descKey: 'how.step4Desc',
  },
];

export default function HowItWorks() {
  const { t } = useTranslation();

  return (
    <section className="how-section section">
      <div className="how-grid">
        <h2 className="how-title">{t('how.title')}</h2>
        <div className="steps">
          {steps.map((s) => (
            <div className="step" key={s.number}>
              <div className="step-number">{s.number}</div>
              <div className="step-title">{t(s.titleKey)}</div>
              <div className="step-desc">{t(s.descKey)}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
