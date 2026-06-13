import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
    base: "/",
    plugins: [react()],
    server: {
        proxy: {
            "/api": "http://127.0.0.1:8080"
        }
    },
    build: {
        emptyOutDir: true,
        manifest: false,
        outDir: "static",
        rollupOptions: {
            input: "index.html",
            watch: {
                exclude: ["node_modules/**", "static/**"]
            },
            output: {
                assetFileNames: (assetInfo) => {
                    const name = assetInfo.name || assetInfo.names.find(Boolean) || assetInfo.originalFileName || assetInfo.originalFileNames.find(Boolean) || "";
                    if (name.endsWith(".css")) {
                        return "site.css";
                    }

                    return "[name][extname]";
                },
                entryFileNames: "site.js"
            }
        }
    }
});
