import type { Config } from 'tailwindcss';

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: '#0f172a',
        mist: '#f8fafc',
        signal: '#0f766e',
        glow: '#d1fae5',
        ember: '#f97316',
      },
      boxShadow: {
        panel: '0 18px 60px rgba(15, 23, 42, 0.14)',
      },
      fontFamily: {
        sans: ['"Manrope"', '"Avenir Next"', 'ui-sans-serif', 'system-ui'],
      },
    },
  },
  plugins: [],
} satisfies Config;
