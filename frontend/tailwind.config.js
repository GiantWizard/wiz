/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{html,svelte,js,ts}'],
  theme: {
    extend: {
      colors: {
        // Original brand colors
        dark: '#0B0B16',      // Main background: nearly black with purple undertones
        darker: '#12121E',    // Muted dark tone for navbar and similar elements
        primary: '#433D8B',    // A deep, rich purple
        accent: '#C8ACD6',     // A light, airy purple/lavender accent
        light: '#FFFFFF',

        // Five-step green scale (muted to blend with purples)
        green: {
          100: '#e6f5e9',      // Very light muted green (almost pastel)
          300: '#a7d4b0',      // Soft medium green
          500: '#68b17a',      // Base green; balanced and not too saturated
          700: '#3d8c57',      // Darker, more reserved green
          900: '#215b38',      // Deep, muted green for emphasis when needed
        },

        // Five-step red scale (again, intentionally muted)
        red: {
          100: '#f8e7e7',      // Very light, almost pinkish red
          300: '#e8b8b8',      // Soft red with a gentle tone
          500: '#d77a7a',      // Base red that’s warm but not jarring
          700: '#b05252',      // Darker red for subtle emphasis
          900: '#8a3636',      // Deep, reserved red for critical accents
        },

        // (Optional) Additional scales can be defined similarly:
        // For example, a five-step purple scale to expand your palette
        purple: {
          100: '#eae4f0',
          300: '#c8b3d8',
          500: '#a186c2',   // Could be close to accent if desired
          700: '#7a4f9d',
          900: '#4d2f70',
        },
      },
      fontFamily: {
        inter: ['Inter', 'sans-serif']
      }
    },
  },
  plugins: [],
}
