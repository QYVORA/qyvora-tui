#!/usr/bin/env bun

import { chmodSync, existsSync, mkdirSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import type { BunPlugin } from "bun"

interface BuildTarget {
  bunTarget: string
  nativePackage: string
  libc: "glibc" | "musl" | undefined
}

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const packageRoot = resolve(__dirname, "..")
const repoRoot = resolve(packageRoot, "../..")
const coreRoot = join(repoRoot, "packages", "core")
const distDir = join(packageRoot, "dist")
const args = process.argv.slice(2)
const argValue = (name: string) => {
  const inline = args.find((arg) => arg.startsWith(`--${name}=`))?.slice(name.length + 3)
  if (inline !== undefined) return inline
  const index = args.indexOf(`--${name}`)
  return index >= 0 ? args[index + 1] : undefined
}
const targetArg = argValue("target")
const outArg = argValue("out")
const platform = process.platform === "win32" ? "windows" : process.platform === "darwin" ? "darwin" : "linux"
const arch = process.arch

function hostTarget(): BuildTarget {
  const libc = platform === "linux" ? (process.env.OPENTUI_LIBC === "musl" ? "musl" : "glibc") : undefined
  const npmPlatform = platform === "windows" ? "win32" : platform
  const suffix = libc === "musl" ? "-musl" : ""
  return {
    bunTarget: `bun-${platform}-${arch}${libc === "musl" ? "-musl" : ""}`,
    nativePackage: `@opentui/core-${npmPlatform}-${arch}${suffix}`,
    libc,
  }
}

function parseTarget(value: string | undefined): BuildTarget {
  if (!value) return hostTarget()
  const normalized = value.startsWith("bun-") ? value.slice(4) : value
  const parts = normalized.split("-")
  if (parts.length < 2) throw new Error(`Invalid target: ${value}`)
  const [targetPlatform, targetArch, libcPart] = parts
  if (targetPlatform !== "linux" && targetPlatform !== "darwin" && targetPlatform !== "windows") {
    throw new Error(`Invalid target platform: ${targetPlatform}`)
  }
  if (targetArch !== "x64" && targetArch !== "arm64") {
    throw new Error(`Invalid target architecture: ${targetArch}`)
  }
  const libc = targetPlatform === "linux" ? (libcPart === "musl" ? "musl" : "glibc") : undefined
  const npmPlatform = targetPlatform === "windows" ? "win32" : targetPlatform
  return {
    bunTarget: `bun-${targetPlatform}-${targetArch}${libc === "musl" ? "-musl" : ""}`,
    nativePackage: `@opentui/core-${npmPlatform}-${targetArch}${libc === "musl" ? "-musl" : ""}`,
    libc,
  }
}

const target = parseTarget(targetArg)
const outfile = outArg ? resolve(outArg) : join(distDir, `qyvora-ai-tui-${target.bunTarget}`)
mkdirSync(dirname(outfile), { recursive: true })

function hasNativePackage(name: string): boolean {
  return (
    existsSync(join(coreRoot, "node_modules", name)) ||
    existsSync(join(repoRoot, "node_modules", name)) ||
    existsSync(join(packageRoot, "node_modules", name))
  )
}

if (!hasNativePackage(target.nativePackage)) {
  throw new Error(`Missing native package ${target.nativePackage}; run bun install first`)
}

const plugin: BunPlugin = {
  name: "qyvora-chat-workspace",
  setup(build) {
    build.onResolve({ filter: /^@opentui\/core-(?:darwin|linux|win32)-/ }, (args) => {
      if (args.path === target.nativePackage) return
      return { path: args.path, external: true }
    })
    build.onResolve({ filter: /^@opentui\/core$/ }, () => ({
      path: join(coreRoot, "src", "index.ts"),
    }))
  },
}

const result = await Bun.build({
  entrypoints: [join(packageRoot, "src", "app", "main.ts")],
  sourcemap: "linked",
  plugins: [plugin],
  define: {
    "process.env.OPENTUI_LIBC": JSON.stringify(target.libc ?? "glibc"),
  },
  compile: {
    target: target.bunTarget as any,
    outfile,
    execArgv: ['--env-file=""', "--"],
  },
})

if (result.logs.length > 0) {
  for (const log of result.logs) {
    const write = log.level === "error" ? console.error : log.level === "warning" ? console.warn : console.log
    write(log.message)
  }
}

if (!result.success) {
  process.exit(1)
}

if (platform !== "windows") chmodSync(outfile, 0o755)
console.log(outfile)
