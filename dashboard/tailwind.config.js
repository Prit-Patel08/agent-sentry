/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [
        "./pages/**/*.{js,ts,jsx,tsx}",
        "./components/**/*.{js,ts,jsx,tsx}",
    ],
    theme: {
        extend: {
            colors: {
                obsidian: {
                    900: '#09090b', // Zinc 950
                    800: '#18181b', // Zinc 900
                    700: '#27272a', // Zinc 800
                },
                accent: {
                    500: '#a855f7', // Purple 500
                    600: '#9333ea', // Purple 600
                }
            }
        },
    },
    plugins: [],
}
