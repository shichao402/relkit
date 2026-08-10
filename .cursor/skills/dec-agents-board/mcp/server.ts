#!/usr/bin/env node
/**
 * Agents Board MCP — 密码写死在本文件（私有仓库）。
 * 不通过 MCP JSON / mise 注入 token。
 *
 * Workflow for agents:
 * 1. board_list_rooms → pick roomId
 * 2. board_describe_room → structured layout + plain text (default read)
 * 3. board_screenshot → PNG for handwriting / visual marks (multimodal)
 * 4. board_annotate / board_upsert_shapes → mark or draw (prefer markers for feedback)
 * 5. board_clear_markers → remove agent-only marks
 */
import { basename } from 'node:path'
import { McpServer } from '@modelcontextprotocol/sdk/server/mcp.js'
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js'
import { z } from 'zod'

// === 写死配置（与 common/config.env 保持一致）===
const BASE = 'http://board.firoyang.com'
const TOKEN = 'Xx123456'
const AGENT_NAME = 'Cursor Agent'

function projectName() {
  try {
    return basename(process.cwd()) || 'project'
  } catch {
    return 'project'
  }
}

function agentActor() {
  return `${projectName()} / ${AGENT_NAME}`
}

async function api(path: string, init: RequestInit = {}) {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-Board-Actor': agentActor(),
    Authorization: `Bearer ${TOKEN}`,
    ...(init.headers as Record<string, string>),
  }
  if (init.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json'
  const res = await fetch(`${BASE}${path}`, { ...init, headers })
  const text = await res.text()
  let data: any
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { raw: text }
  }
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}: ${typeof data === 'string' ? data : JSON.stringify(data)}`)
  }
  return data
}

function ok(data: unknown) {
  return {
    content: [{ type: 'text' as const, text: JSON.stringify(data, null, 2) }],
  }
}

function okMarkdownAndJson(markdown: string, data: unknown) {
  return {
    content: [
      { type: 'text' as const, text: markdown },
      { type: 'text' as const, text: JSON.stringify(data, null, 2) },
    ],
  }
}

const bboxSchema = z
  .object({
    x: z.number(),
    y: z.number(),
    w: z.number(),
    h: z.number(),
  })
  .describe('Page-coordinate axis-aligned box')

const shapeInputSchema = z.object({
  id: z.string().optional(),
  type: z.enum(['geo', 'text', 'note', 'arrow']),
  x: z.number().optional(),
  y: z.number().optional(),
  rotation: z.number().optional(),
  text: z.string().optional(),
  w: z.number().optional(),
  h: z.number().optional(),
  color: z.string().optional(),
  fill: z.string().optional(),
  size: z.enum(['s', 'm', 'l', 'xl']).optional(),
  geo: z
    .enum([
      'rectangle',
      'ellipse',
      'triangle',
      'diamond',
      'pentagon',
      'hexagon',
      'cloud',
      'star',
      'check-box',
      'x-box',
    ])
    .optional(),
  meta: z.record(z.unknown()).optional(),
})

const server = new McpServer({
  name: 'agents-board',
  version: '0.3.0',
})

server.tool(
  'board_list_rooms',
  'List shared whiteboard rooms (name, timestamps, last editor, permanent/alias URLs). Call this first to pick a roomId.',
  {},
  async () => ok(await api('/api/rooms')),
)

server.tool(
  'board_create_room',
  'Create a new whiteboard room. Returns permanent roomId and alias slug.',
  { name: z.string().describe('Display name, e.g. login sketch') },
  async ({ name }) => ok(await api('/api/rooms', { method: 'POST', body: JSON.stringify({ name }) })),
)

server.tool(
  'board_rename_room',
  'Rename room display name (permanent roomId unchanged; alias slug follows the new name).',
  {
    roomId: z.string().describe('Permanent room id'),
    name: z.string().describe('New display name'),
  },
  async ({ roomId, name }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}`, {
        method: 'PATCH',
        body: JSON.stringify({ name }),
      }),
    ),
)

server.tool(
  'board_describe_room',
  'PRIMARY READ TOOL. Return an agent-friendly board summary: plain-text labels, bounds, type counts, spatial ordering, and tips. Handwriting/draw strokes are listed but not OCR\'d — follow up with board_screenshot + vision. Prefer this over board_get_room.',
  { roomId: z.string().describe('Permanent room id') },
  async ({ roomId }) => {
    const data = await api(`/api/rooms/${encodeURIComponent(roomId)}/describe`)
    const markdown = typeof data.markdown === 'string' ? data.markdown : JSON.stringify(data, null, 2)
    const { markdown: _m, ...rest } = data
    return okMarkdownAndJson(markdown, rest)
  },
)

server.tool(
  'board_get_room',
  'Read room metadata + describe summary (same as board_describe_room). For raw tldraw props dump use format="raw" (debug only — hard for agents to read).',
  {
    roomId: z.string().describe('Permanent room id'),
    format: z.enum(['describe', 'raw']).optional().describe('Default describe; raw = full props dump'),
  },
  async ({ roomId, format }) => {
    const q = format === 'raw' ? '?format=raw' : ''
    const data = await api(`/api/rooms/${encodeURIComponent(roomId)}${q}`)
    if (format === 'raw') return ok(data)
    const markdown = typeof data.markdown === 'string' ? data.markdown : JSON.stringify(data, null, 2)
    const { markdown: _m, ...rest } = data
    return okMarkdownAndJson(markdown, rest)
  },
)

server.tool(
  'board_screenshot',
  'Capture a content-fit PNG of the board (or a bbox / shapeIds subset) for multimodal reading. Use when handwriting, red boxes, sketches, or layout cannot be understood from board_describe_room alone. Returns an image content block. Defaults: padding=24, maxWidth/maxHeight=1280, markers hidden.',
  {
    roomId: z.string(),
    bbox: bboxSchema.optional().describe('Optional page-coordinate crop; omit = union of shapes + padding'),
    shapeIds: z.array(z.string()).optional().describe('Optional: only these shapes (+ padding)'),
    padding: z.number().optional().describe('Padding around content/bbox. Default 24'),
    maxWidth: z.number().optional().describe('Max PNG width. Default 1280'),
    maxHeight: z.number().optional().describe('Max PNG height. Default 1280'),
    includeMarkers: z.boolean().optional().describe('Include agent-marker shapes. Default false'),
  },
  async ({ roomId, bbox, shapeIds, padding, maxWidth, maxHeight, includeMarkers }) => {
    const data = await api(`/api/rooms/${encodeURIComponent(roomId)}/screenshot`, {
      method: 'POST',
      body: JSON.stringify({
        bbox,
        shapeIds,
        padding,
        maxWidth,
        maxHeight,
        includeMarkers: includeMarkers ?? false,
        format: 'json',
      }),
    })
    const caption = [
      `screenshot room=${roomId}`,
      `png=${data.width}x${data.height}`,
      `shapes=${data.shapeCount}`,
      `bounds=(${data.bounds?.x},${data.bounds?.y}) ${data.bounds?.w}x${data.bounds?.h}`,
      'Read the image with vision for handwriting / visual marks. Then annotate with board_annotate if needed.',
    ].join(' | ')
    return {
      content: [
        { type: 'text' as const, text: caption },
        { type: 'image' as const, data: data.base64, mimeType: 'image/png' as const },
      ],
    }
  },
)

server.tool(
  'board_annotate',
  'Draw agent-owned markers WITHOUT modifying user shapes. Creates shapes with meta.role=agent-marker. kind=highlight (dashed rect), callout (rect + sticky), arrow-point (arrow into region). Prefer this for feedback / pointing; use board_upsert_shapes only to create real content.',
  {
    roomId: z.string(),
    kind: z.enum(['highlight', 'callout', 'arrow-point']).optional().describe('Default highlight'),
    targetShapeIds: z.array(z.string()).optional().describe('Wrap these shapes'),
    bbox: bboxSchema.optional().describe('Or mark an explicit region'),
    label: z.string().optional().describe('Optional label / callout text'),
    color: z.string().optional().describe('tldraw color name, default red/orange'),
  },
  async ({ roomId, kind, targetShapeIds, bbox, label, color }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}/annotate`, {
        method: 'POST',
        body: JSON.stringify({ kind, targetShapeIds, bbox, label, color }),
      }),
    ),
)

server.tool(
  'board_clear_markers',
  'Delete all agent-marker shapes (meta.role=agent-marker) in a room. Does not delete user content.',
  { roomId: z.string() },
  async ({ roomId }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}/markers`, {
        method: 'DELETE',
      }),
    ),
)

server.tool(
  'board_upsert_shapes',
  'Create or overwrite content shapes (geo/text/note/arrow). Tablet clients see changes live. For temporary feedback prefer board_annotate. Arrow w/h mean end-point offset from start, not a geo box size.',
  {
    roomId: z.string(),
    shapes: z.array(shapeInputSchema).min(1),
  },
  async ({ roomId, shapes }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}/shapes`, {
        method: 'POST',
        body: JSON.stringify({ shapes }),
      }),
    ),
)

server.tool(
  'board_patch_shapes',
  'Patch existing shapes (position/text/props). Prefer small patches over full upsert.',
  {
    roomId: z.string(),
    patches: z
      .array(
        z.object({
          id: z.string(),
          x: z.number().optional(),
          y: z.number().optional(),
          text: z.string().optional(),
          meta: z.record(z.unknown()).optional(),
          props: z.record(z.unknown()).optional(),
        }),
      )
      .min(1),
  },
  async ({ roomId, patches }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}/shapes`, {
        method: 'PATCH',
        body: JSON.stringify({ patches }),
      }),
    ),
)

server.tool(
  'board_delete_shapes',
  'Delete shapes by id. Do not delete user content unless asked; use board_clear_markers for agent marks.',
  {
    roomId: z.string(),
    ids: z.array(z.string()).min(1),
  },
  async ({ roomId, ids }) =>
    ok(
      await api(`/api/rooms/${encodeURIComponent(roomId)}/shapes`, {
        method: 'DELETE',
        body: JSON.stringify({ ids }),
      }),
    ),
)

const transport = new StdioServerTransport()
await server.connect(transport)
