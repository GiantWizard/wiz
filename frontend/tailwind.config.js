/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{html,svelte,js,ts}'],
  theme: {
    extend: {
      colors: {
        // Main background: nearly black with purple undertones
        dark: '#0B0B16',
        // Muted dark tone for navbar and similar elements
        darker: '#12121E',
        primary: '#433D8B',
        accent: '#C8ACD6',
        light: '#FFFFFF'
      },
      fontFamily: {
        inter: ['Inter', 'sans-serif']
      }
    },
  },
  plugins: [],
}
