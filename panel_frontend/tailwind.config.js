/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#0c1222",
        mist: "#f4f7fb",
        tide: "#1f6feb",
        ember: "#ef7d57",
        pine: "#0d5c4f"
      },
      boxShadow: {
        panel: "0 28px 80px rgba(3, 9, 20, 0.42)",
        glow: "0 0 0 1px rgba(148, 163, 184, 0.08), 0 24px 64px rgba(8, 18, 35, 0.38)"
      },
      fontFamily: {
        sans: ["'IBM Plex Sans'", "sans-serif"],
        display: ["'Space Grotesk'", "sans-serif"]
      }
    }
  },
  plugins: []
};
