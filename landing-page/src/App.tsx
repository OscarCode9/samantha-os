import DNAAnimation from './components/DNAAnimation';
import Hero from './components/Hero';
import Features from './components/Features';
import HowItWorks from './components/HowItWorks';
import Quote from './components/Quote';
import Footer from './components/Footer';

function App() {
  return (
    <main>
      <DNAAnimation />
      <div id="hero">
        <Hero />
      </div>
      <Features />
      <HowItWorks />
      <Quote />
      <Footer />
    </main>
  );
}

export default App;
