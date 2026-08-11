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
      keyframes: {
        marquee: {
          '0%, 15%': { transform: 'translateX(0)' },
          '85%, 100%': { transform: 'translateX(var(--scroll-amount))' },
        },
        // Drains left-to-right for exactly as long as a toast is on screen, so
        // the toast says when it is leaving instead of vanishing unannounced.
        'toast-drain': {
          from: { transform: 'scaleX(1)' },
          to: { transform: 'scaleX(0)' },
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
        // 4s matches DEFAULT_DURATION in ToastContext so the class is correct
        // on its own; ToastStack overrides animation-duration per toast for the
        // ones that stay longer.
        'toast-drain': 'toast-drain 4s linear forwards',
        'fade-in-up': 'fade-in-up 220ms ease-out both',
      }
    }
  },
  plugins: [],
}
