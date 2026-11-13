import { spawn } from "node:child_process";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const rootDir = resolve(__dirname, "..");
const backendDir = resolve(rootDir, "backend");
const frontendDir = resolve(rootDir, "frontend");

console.log("\x1b[36m%s\x1b[0m", "Starting Universal DeFi Intelligence & Fernhollow stack...");

// Start Go Backend API (:8081)
const backend = spawn("go", ["run", "-mod=vendor", "./cmd/api"], {
  cwd: backendDir,
  stdio: "inherit",
  shell: true,
  env: {
    ...process.env,
    HTTP_ADDR: process.env.HTTP_ADDR || ":8081",
    APP_ENV: process.env.APP_ENV || "local",
  },
});

backend.on("error", (err) => {
  console.error("\x1b[31m[backend error]\x1b[0m", err);
});

// Start Next.js Frontend (:3000)
const frontend = spawn("npm", ["run", "dev"], {
  cwd: frontendDir,
  stdio: "inherit",
  shell: true,
  env: {
    ...process.env,
    UDI_API_ORIGIN: process.env.UDI_API_ORIGIN || "http://127.0.0.1:8081",
  },
});

frontend.on("error", (err) => {
  console.error("\x1b[31m[frontend error]\x1b[0m", err);
});

const cleanup = () => {
  console.log("\n\x1b[33m%s\x1b[0m", "Shutting down services...");
  try {
    backend.kill("SIGINT");
  } catch {}
  try {
    frontend.kill("SIGINT");
  } catch {}
  process.exit(0);
};

process.on("SIGINT", cleanup);
process.on("SIGTERM", cleanup);

