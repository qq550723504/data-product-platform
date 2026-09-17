#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { realpathSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";

export function parseCommand(args) {
  const command = args[0];
  if (command === "reset" && args.length === 2 && args[1] === "--confirm=DELETE_DEMO_DATA") return command;
  if (args.length === 1 && ["up", "down", "advance", "status", "verify", "logs", "doctor"].includes(command)) return command;
  throw new Error("Usage: node deploy/demo/demo.mjs up|down|advance|status|verify|logs|doctor\nTo delete only this demo's volumes: reset --confirm=DELETE_DEMO_DATA");
}
export function projectName(root) {
  return `dpp-demo-${createHash("sha256").update(root).digest("hex").slice(0, 12)}`;
}
export function localEndpoint(endpoint) {
  return typeof endpoint === "string" && /^(unix:\/\/\/|npipe:\/\/)/.test(endpoint);
}

const root = realpathSync(fileURLToPath(new URL("../../", import.meta.url)));
const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith("COMPOSE_")));
function docker(args, capture = false) {
  const result = spawnSync("docker", args, {
    cwd: root, env: environment, shell: false, encoding: "utf8", timeout: 20 * 60 * 1000,
    maxBuffer: 32 * 1024 * 1024, stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit",
  });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`Docker command failed (${result.status}): ${capture ? result.stderr : "see output above"}`);
  return capture ? result.stdout.trim() : "";
}
function doctor() {
  const context = docker(["context", "show"], true);
  const details = JSON.parse(docker(["context", "inspect", context], true));
  const endpoint = process.env.DOCKER_HOST && !process.env.DOCKER_CONTEXT ? process.env.DOCKER_HOST : details[0]?.Endpoints?.docker?.Host;
  if (!localEndpoint(endpoint)) throw new Error("Only a local Unix/named-pipe Docker daemon is allowed; remote/TCP/SSH contexts are refused.");
  const version = docker(["compose", "version", "--short"], true);
  if (Number(version.replace(/^v/, "").split(".")[0]) < 2) throw new Error("Docker Compose v2 or newer is required.");
  docker(["info", "--format", "{{.OSType}}"], true).split(/\s+/).forEach((os) => {
    if (os !== "linux") throw new Error("Use Linux containers for this demo.");
  });
}
function compose(args, capture = false) {
  return docker(["compose", "--project-name", projectName(root), "--project-directory", root,
    "--env-file", "deploy/demo/empty.env", "-f", "deploy/demo/compose.yml", ...args], capture);
}
function tool(command) {
  return compose(["run", "--rm", "--no-deps", "-T", "seed", command], true);
}
export function main(args) {
  const command = parseCommand(args); // Reject destructive/unknown input before touching Docker.
  doctor();
  switch (command) {
    case "doctor":
      compose(["config", "--quiet"]);
      console.log(`Local Docker ready. Project: ${projectName(root)}. Console: http://127.0.0.1:3180`);
      break;
    case "up":
      compose(["build", "api", "web"]);
      compose(["up", "-d", "--wait", "--wait-timeout", "120", "postgres", "redis", "minio"]);
      compose(["run", "--rm", "--no-deps", "-T", "migrate"]);
      compose(["up", "-d", "--wait", "--wait-timeout", "120", "api", "worker"]);
      console.log(tool("prepare"));
      compose(["up", "-d", "--wait", "--wait-timeout", "120", "web"]);
      console.log("Synthetic local demo: http://127.0.0.1:3180/reviews\nReview in the browser, run advance, then publish in the browser. down preserves data.");
      break;
    case "down":
      compose(["down", "--remove-orphans"]); // Deliberately no --volumes.
      break;
    case "reset":
      compose(["down", "--volumes", "--remove-orphans"]); // Only this path-derived Compose project.
      console.log("Deleted only this synthetic demo's containers and named volumes. No system prune was used.");
      break;
    case "logs":
      compose(["logs", "--no-color", "--tail=120", "api", "worker", "web"]);
      break;
    default:
      console.log(tool(command));
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { main(process.argv.slice(2)); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
