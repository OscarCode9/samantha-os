const steps = [
  {
    number: '01',
    title: 'Install Samantha OS.',
    desc: 'Samantha comes preloaded.',
  },
  {
    number: '02',
    title: 'Create your account.',
    desc: 'The familiar Initial Setup now includes one new screen.',
  },
  {
    number: '03',
    title: 'Connect in under a minute.',
    desc: 'GitHub device flow. One code. One browser tab. Done.',
  },
  {
    number: '04',
    title: 'Your OS thinks with you.',
    desc: 'Open the panel, start a chat, and never look back.',
  },
];

export default function HowItWorks() {
  return (
    <section className="how-section section">
      <div className="how-grid">
        <h2 className="how-title">How it works</h2>
        <div className="steps">
          {steps.map((s) => (
            <div className="step" key={s.number}>
              <div className="step-number">{s.number}</div>
              <div className="step-title">{s.title}</div>
              <div className="step-desc">{s.desc}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
