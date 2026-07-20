#!/usr/bin/env node

import { accessSync, constants } from "node:fs";
import { spawn } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const binaryName = process.platform === "win32" ? "casemagica.exe" : "casemagica";
const binaryPath = join(packageRoot, "vendor", platformKey(), binaryName);

try {
  accessSync(binaryPath, constants.F_OK);
} catch {
  console.error(`当前平台暂不支持或二进制缺失: ${platformKey()}`);
  console.error(`缺失文件: ${binaryPath}`);
  process.exit(1);
}

const args = process.argv.slice(2);
const backendPort = process.env.CASEMAGICA_BACKEND_PORT || process.env.CASEMAGICA_BACKEND_PORT;
if (backendPort && !hasFlag(args, "port")) {
  args.push("--port", backendPort);
}

const child = spawn(binaryPath, args, {
  stdio: "inherit",
  env: {
    ...process.env,
    CASEMAGICA_DIR: process.env.CASEMAGICA_DIR || process.env.CASEMAGICA_DIR || resolve(process.cwd(), ".casemagica"),
    CASEMAGICA_WEB_DIR: process.env.CASEMAGICA_WEB_DIR || process.env.NOVA_WEB_DIR || join(packageRoot, "web"),
    CASEMAGICA_SKILLS_DIR: process.env.CASEMAGICA_SKILLS_DIR || process.env.CASEMAGICA_SKILLS_DIR || join(packageRoot, "skills"),
  },
});

child.on("exit", (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 0);
});

child.on("error", (err) => {
  console.error(`启动 CaseMagica 失败: ${err.message}`);
  process.exit(1);
});

function platformKey() {
  const arch = process.arch === "x64" ? "x64" : process.arch;
  return `${process.platform}-${arch}`;
}

function hasFlag(args, name) {
  return args.some((arg) => arg === `--${name}` || arg.startsWith(`--${name}=`));
}
