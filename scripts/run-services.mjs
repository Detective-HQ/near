import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const projectDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const nextCli = resolve(projectDir, "node_modules/next/dist/bin/next");
const command = process.argv[2] === "start" ? "start" : "dev";
const isProduction = command === "start";
const apiPort = process.env.API_PORT ?? "8081";
const nextPort = process.env.NEXT_PORT ?? "3000";
const apiHost = process.env.API_HOST ?? (process.env.NODE_ENV === "production" ? "0.0.0.0" : "127.0.0.1");
const nextHost = process.env.NEXT_HOST ?? (process.env.NODE_ENV === "production" ? "0.0.0.0" : "127.0.0.1");
const sharedEnv = {
  ...process.env,
  NODE_ENV: isProduction ? "production" : "development"
};

const services = [
  {
    name: "api",
    args: [resolve(projectDir, "server.mjs")],
    env: {
      ...sharedEnv,
      HOST: apiHost,
      PORT: apiPort
    }
  },
  {
    name: "next",
    args: [
      nextCli,
      command,
      "--hostname",
      nextHost,
      "--port",
      nextPort
    ],
    env: {
      ...sharedEnv,
      UDI_API_ORIGIN: process.env.UDI_API_ORIGIN ?? `http://127.0.0.1:${apiPort}`
    }
  }
];

const children = new Map();
let stopping = false;

function stopAll(exitCode = 0) {
  if (stopping) return;
  stopping = true;
  for (const child of children.values()) {
    if (child.exitCode === null && !child.killed) child.kill();
  }
  if (children.size === 0) process.exitCode = exitCode;
}

for (const service of services) {
  const child = spawn(process.execPath, service.args, {
    cwd: projectDir,
    env: service.env,
    stdio: "inherit",
    windowsHide: true
  });
  children.set(service.name, child);

  child.once("error", (error) => {
    process.stderr.write(`[${service.name}] failed to start: ${error.message}\n`);
    stopAll(1);
  });
  child.once("exit", (code, signal) => {
    children.delete(service.name);
    if (!stopping) {
      const result = code ?? (signal ? 1 : 0);
      process.stderr.write(`[${service.name}] exited (${signal ?? result}); stopping the other service.\n`);
      stopAll(result);
    } else if (children.size === 0) {
      process.exitCode ??= 0;
    }
  });
}

process.on("SIGINT", () => stopAll(130));
process.on("SIGTERM", () => stopAll(143));
