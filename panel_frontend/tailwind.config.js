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
        panel: "0 20px 60px rgba(12, 18, 34, 0.12)"
      },
      fontFamily: {
        sans: ["'IBM Plex Sans'", "sans-serif"],
        display: ["'Space Grotesk'", "sans-serif"]
      }
    }
  },
  plugins: []
};
