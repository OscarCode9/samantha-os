import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface NavbarProps {
  currentView: string;
  onViewChange: (view: string) => void;
}

export default function Navbar({ currentView, onViewChange }: NavbarProps) {
  const [isScrolled, setIsScrolled] = useState(false);
  const { t, i18n } = useTranslation();

  useEffect(() => {
    const handleScroll = () => {
      setIsScrolled(window.scrollY > 20);
    };
    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  const handleLinkClick = (e: React.MouseEvent<HTMLAnchorElement>, targetView: string, hash: string) => {
    e.preventDefault();
    onViewChange(targetView);
    window.location.hash = hash;

    // If switching views or scrolling to an anchor
    if (hash && targetView === 'home') {
      setTimeout(() => {
        const element = document.getElementById(hash.substring(1));
        if (element) {
          element.scrollIntoView({ behavior: 'smooth' });
        }
      }, 50);
    } else if (!hash) {
      window.scrollTo({ top: 0, behavior: 'smooth' });
    }
  };

  const toggleLanguage = () => {
    const nextLang = i18n.language === 'es' ? 'en' : 'es';
    i18n.changeLanguage(nextLang);
  };

  return (
    <header className={`navbar-header ${isScrolled ? 'scrolled' : ''}`}>
      <div className="navbar-container">
        <a 
          href="#" 
          className="navbar-brand font-display"
          onClick={(e) => handleLinkClick(e, 'home', '')}
        >
          <span className="navbar-brand-dot"></span>
          Samantha OS
        </a>

        <nav className="navbar-menu">
          <a 
            href="#" 
            className={`navbar-link ${currentView === 'home' && !window.location.hash ? 'active' : ''}`}
            onClick={(e) => handleLinkClick(e, 'home', '')}
          >
            {t('nav.home')}
          </a>
          <a 
            href="#features" 
            className={`navbar-link ${currentView === 'home' && window.location.hash === '#features' ? 'active' : ''}`}
            onClick={(e) => handleLinkClick(e, 'home', '#features')}
          >
            {t('nav.features')}
          </a>
          <a 
            href="#how-it-works" 
            className={`navbar-link ${currentView === 'home' && window.location.hash === '#how-it-works' ? 'active' : ''}`}
            onClick={(e) => handleLinkClick(e, 'home', '#how-it-works')}
          >
            {t('nav.howItWorks')}
          </a>
          <a 
            href="#docs" 
            className={`navbar-link ${currentView === 'docs' ? 'active' : ''}`}
            onClick={(e) => handleLinkClick(e, 'docs', '#docs')}
          >
            {t('nav.docs')}
          </a>

          <button 
            onClick={toggleLanguage} 
            className="navbar-lang-btn font-mono"
            aria-label="Toggle language"
          >
            {i18n.language === 'es' ? 'EN' : 'ES'}
          </button>
        </nav>
      </div>
    </header>
  );
}
