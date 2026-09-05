/*
 * Copyright (C) 2026 ResultV
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      // --- Figma "App UI-kit" (node 0:1) ------------------------------
      // Имена совпадают с именами переменных в Figma. Значения живут в
      // src/design/tokens.css — здесь только проброс, чтобы у Tailwind был
      // один источник правды с CSS.
      colors: {
        white: 'var(--rv-white)',
        black: 'var(--rv-black)',
        'dark-grey': 'var(--rv-dark-grey)',
        grey: 'var(--rv-grey)',
        'light-gray': 'var(--rv-light-gray)',
        'main-color': 'var(--rv-main-color)',
        'second-color': 'var(--rv-second-color)',
        warning: 'var(--rv-warning)',
        errors: 'var(--rv-errors)',
        'shadow-main': 'var(--rv-shadow-main)',
        'shadow-warning': 'var(--rv-shadow-warning)',
        'shadow-error': 'var(--rv-shadow-error)',
      },
      fontFamily: {
        ui: 'var(--rv-font-family)',
      },
      // Текстовые стили макета целиком: размер + интерлиньяж + начертание.
      fontSize: {
        h1: ['var(--rv-h1-size)', { lineHeight: 'var(--rv-h1-leading)', fontWeight: 'var(--rv-h1-weight)' }],
        title: ['var(--rv-title-size)', { lineHeight: 'var(--rv-title-leading)', fontWeight: 'var(--rv-title-weight)' }],
        'title-sm': ['var(--rv-title-sm-size)', { lineHeight: 'var(--rv-title-sm-leading)', fontWeight: 'var(--rv-title-sm-weight)' }],
        btn: ['var(--rv-btn-size)', { lineHeight: 'var(--rv-btn-leading)', fontWeight: 'var(--rv-btn-weight)' }],
        regular: ['var(--rv-regular-size)', { lineHeight: 'var(--rv-regular-leading)', fontWeight: 'var(--rv-regular-weight)' }],
        'regular-up': ['var(--rv-regular-up-size)', { lineHeight: 'var(--rv-regular-up-leading)', fontWeight: 'var(--rv-regular-up-weight)' }],
        chip: ['var(--rv-chip-size)', { lineHeight: 'var(--rv-chip-leading)', fontWeight: 'var(--rv-chip-weight)' }],
      },
      keyframes: {
        marquee: {
          '0%, 15%': { transform: 'translateX(0)' },
          '85%, 100%': { transform: 'translateX(var(--scroll-amount))' },
        },
        // The modal entrance every dialog in the app already asks for by class
        // name (AppDialogModal, UpdaterModal, UpdateNotificationModal). It was
        // never defined, so Tailwind emitted nothing and those modals simply
        // appeared. Defining it here makes the three existing call sites behave
        // the way they read.
        'fade-in-up': {
          from: { opacity: '0', transform: 'translateY(8px) scale(0.98)' },
          to: { opacity: '1', transform: 'translateY(0) scale(1)' },
        },
      },
      animation: {
        marquee: 'marquee 1s linear infinite alternate',
        'fade-in-up': 'fade-in-up 220ms ease-out both',
      }
    }
  },
  plugins: [],
}
