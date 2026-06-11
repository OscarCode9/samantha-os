function IconZap() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  );
}

function IconPanel() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
      <line x1="3" y1="9" x2="21" y2="9" />
      <line x1="9" y1="21" x2="9" y2="9" />
    </svg>
  );
}

function IconShield() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  );
}

function IconTool() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />
    </svg>
  );
}

function IconMemory() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2 12h6" />
      <path d="M22 12h-6" />
      <path d="M12 2v6" />
      <path d="M12 22v-6" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function IconProvider() {
  return (
    <svg className="feature-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
      <line x1="8" y1="21" x2="16" y2="21" />
      <line x1="12" y1="17" x2="12" y2="21" />
    </svg>
  );
}

import { useTranslation } from 'react-i18next';

const features = [
  {
    icon: <IconZap />,
    titleKey: 'features.zeroSetupTitle',
    bodyKey: 'features.zeroSetupBody',
  },
  {
    icon: <IconPanel />,
    titleKey: 'features.livesPanelTitle',
    bodyKey: 'features.livesPanelBody',
  },
  {
    icon: <IconShield />,
    titleKey: 'features.filesRulesTitle',
    bodyKey: 'features.filesRulesBody',
  },
  {
    icon: <IconTool />,
    titleKey: 'features.talksToolsTitle',
    bodyKey: 'features.talksToolsBody',
  },
  {
    icon: <IconMemory />,
    titleKey: 'features.remembersTitle',
    bodyKey: 'features.remembersBody',
  },
  {
    icon: <IconProvider />,
    titleKey: 'features.openModelTitle',
    bodyKey: 'features.openModelBody',
  },
];

export default function Features() {
  const { t } = useTranslation();

  return (
    <section className="features-section section">
      <div className="features-grid">
        <h2 className="features-title">{t('features.sectionTitle')}</h2>
        <div className="features-cards">
          {features.map((f) => (
            <div className="feature-card" key={f.titleKey}>
              {f.icon}
              <h3 className="feature-card-title">{t(f.titleKey)}</h3>
              <p className="feature-card-body">{t(f.bodyKey)}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
