#!/usr/bin/env node
/**
 * MCP cold-start: ensure node_modules, then run server.ts via tsx.
 * Diagnostics go to stderr (stdout is MCP JSON-RPC).
 */
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { spawn, spawnSync } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = dirname(fileURLToPath(import.meta.url))
const lockPath = join(root, 'package-lock.json')
const markerPath = join(root, 'node_modules', '@modelcontextprotocol', 'sdk', 'package.json')
const stampPath = join(root, 'node_modules', '.agents-board-mcp-ci')
const tsxCli = join(root, 'node_modules', 'tsx', 'dist', 'cli.mjs')
const entryPath = join(root, 'server.ts')

function lockHash() {
  if (!existsSync(lockPath)) return ''
  return createHash('sha256').update(readFileSync(lockPath)).digest('hex')
}

function needsInstall() {
  if (!existsSync(entryPath)) {
    console.error(`[agents-board-mcp] missing ${entryPath}`)
    process.exit(1)
  }
  if (!existsSync(markerPath) || !existsSync(tsxCli) || !existsSync(stampPath)) return true
  const expected = lockHash()
  if (!expected) return true
  return readFileSync(stampPath, 'utf8').trim() !== expected
}

function ensureDeps() {
  if (!needsInstall()) return
  if (!existsSync(lockPath)) {
    console.error(`[agents-board-mcp] missing package-lock.json under ${root}`)
    process.exit(1)
  }
  console.error(`[agents-board-mcp] installing dependencies (npm ci --ignore-scripts)...`)
  const result = spawnSync('npm', ['ci', '--ignore-scripts'], {
    cwd: root,
    env: process.env,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  if (result.stdout) process.stderr.write(result.stdout)
  if (result.stderr) process.stderr.write(result.stderr)
  if (result.status !== 0) {
    console.error(`[agents-board-mcp] npm ci failed with exit ${result.status ?? 1}`)
    process.exit(result.status ?? 1)
  }
  writeFileSync(stampPath, `${lockHash()}\n`, 'utf8')
  console.error(`[agents-board-mcp] dependencies ready`)
}

ensureDeps()

const child = spawn(process.execPath, [tsxCli, entryPath], {
  cwd: root,
  env: process.env,
  stdio: 'inherit',
})

child.on('error', (error) => {
  console.error(`[agents-board-mcp] failed to start: ${error.message}`)
  process.exit(1)
})

child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 1)
})
