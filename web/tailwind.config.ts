import type { Config } from 'tailwindcss';

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        ink: '#0f172a',
        signal: '#0f766e',
        surface: '#f8f9fa',
      },
      fontFamily: {
        sans: ['"Manrope"', '"Avenir Next"', 'ui-sans-serif', 'system-ui'],
      },
    },
  },
  plugins: [],
} satisfies Config;
