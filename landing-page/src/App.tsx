import { useState, useEffect } from 'react';
import DNAAnimation from './components/DNAAnimation';
import Navbar from './components/Navbar';
import Hero from './components/Hero';
import Features from './components/Features';
import HowItWorks from './components/HowItWorks';
import Quote from './components/Quote';
import Docs from './components/Docs';
import Footer from './components/Footer';

function App() {
  const [view, setView] = useState(() => {
    const hash = window.location.hash;
    return hash === '#docs' || hash === '#/docs' ? 'docs' : 'home';
  });

  const [isIntroComplete, setIsIntroComplete] = useState(() => {
    const hash = window.location.hash;
    return hash === '#docs' || hash === '#/docs';
  });

  useEffect(() => {
    const handleHashChange = () => {
      const hash = window.location.hash;
      if (hash === '#docs' || hash === '#/docs') {
        setView('docs');
      } else {
        setView('home');
      }
    };
    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

  const showNavbar = view === 'docs' || isIntroComplete;

  return (
    <main>
      {view === 'home' && (
        <DNAAnimation onComplete={() => setIsIntroComplete(true)} />
      )}
      
      {showNavbar && (
        <Navbar currentView={view} onViewChange={setView} />
      )}
      
      {view === 'home' ? (
        <>
          <div id="hero" style={{ paddingTop: '72px' }}>
            <Hero />
          </div>
          <div id="features">
            <Features />
          </div>
          <div id="how-it-works">
            <HowItWorks />
          </div>
          <Quote />
          <Footer />
        </>
      ) : (
        <div style={{ paddingTop: '72px' }}>
          <Docs />
          <Footer />
        </div>
      )}
    </main>
  );
}

export default App;
