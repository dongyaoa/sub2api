#!/usr/bin/env node
// Node.js 20+，仅使用内置模块。POST 只提交一次，GET 最多尝试三次。
// node video_client.mjs --payload payload.json --output video.mp4
// node video_client.mjs --task-id 已有任务ID --output video.mp4
import { readFile, mkdir, open, rename, unlink } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { randomUUID } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';

const COMPLETE = new Set(['done', 'completed', 'succeeded', 'success']);
const FAILED = new Set(['failed', 'cancelled', 'canceled', 'expired', 'error']);
const PENDING = new Set(['', 'pending', 'queued', 'processing', 'in_progress', 'running', 'in_queue']);
const RETRYABLE = new Set([429, 500, 502, 503, 504]);
const MAX_VIDEO_BYTES = 512 * 1024 * 1024;
const key = (process.env.SUB2API_API_KEY || '').trim();

function parseArgs() {
  const args = { output: 'video.mp4', timeout: '900' };
  for (let i = 2; i < process.argv.length; i += 2) {
    const name = process.argv[i];
    if (!['--payload', '--task-id', '--output', '--timeout'].includes(name) || !process.argv[i + 1]) {
      throw new Error('用法：node video_client.mjs (--payload payload.json | --task-id ID) [--output video.mp4] [--timeout 900]');
    }
    args[name.slice(2)] = process.argv[i + 1];
  }
  if (Boolean(args.payload) === Boolean(args['task-id'])) throw new Error('--payload 和 --task-id 必须且只能指定一个');
  args.timeout = Number(args.timeout);
  if (!Number.isInteger(args.timeout) || args.timeout <= 0) throw new Error('--timeout 必须是正整数');
  return args;
}

function apiRoot(value) {
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error('SUB2API_BASE_URL 必须是没有用户信息、查询参数或片段的 HTTP(S) 站点地址');
  }
  let path = url.pathname.replace(/\/+$/, '');
  if (path.endsWith('/v1')) path = path.slice(0, -3);
  url.pathname = path + '/v1';
  return url.toString().replace(/\/$/, '');
}

function errorText(raw) {
  try {
    const data = JSON.parse(raw);
    const error = data.error ?? data;
    return typeof error === 'object' ? String(error.message || error.code || JSON.stringify(error)) : String(error);
  } catch { return raw.slice(0, 500); }
}

async function main() {
  const args = parseArgs();
  if (!key || !process.env.SUB2API_BASE_URL) throw new Error('请设置 SUB2API_BASE_URL 和 SUB2API_API_KEY');
  const root = apiRoot(process.env.SUB2API_BASE_URL);
  const deadline = Date.now() + args.timeout * 1000;
  function remaining() {
    const ms = deadline - Date.now();
    if (ms <= 0) throw new Error('等待超时；可用已打印的任务 ID 和 --task-id 恢复查询，不要重新提交');
    return ms;
  }
  async function pause(ms) { await delay(Math.min(ms, remaining())); remaining(); }
  async function request(method, path, payload, accept = 'application/json') {
    const attempts = method === 'GET' ? 3 : 1;
    const requestTimeout = path.endsWith('/content') ? 600000 : 120000;
    for (let attempt = 0; attempt < attempts; attempt++) {
      let response;
      try {
        response = await fetch(root + path, {
          method, redirect: 'error', signal: AbortSignal.timeout(Math.ceil(Math.min(requestTimeout, remaining()))),
          headers: { Authorization: `Bearer ${key}`, Accept: accept, ...(payload === undefined ? {} : { 'Content-Type': 'application/json' }) },
          ...(payload === undefined ? {} : { body: JSON.stringify(payload) }),
        });
      } catch (error) {
        if (attempt + 1 < attempts) { await pause(1000 * 2 ** attempt); continue; }
        throw new Error(`请求失败：${error.message}${method === 'POST' ? '；创建未自动重试，请先检查任务历史' : ''}`);
      }
      if (response.ok) return response;
      const raw = await response.text();
      if (RETRYABLE.has(response.status) && attempt + 1 < attempts) {
        const header = response.headers.get('retry-after') || '';
        const seconds = /^\d+$/.test(header) ? Math.min(30, Math.max(1, Number(header))) : 2 ** attempt;
        await pause(seconds * 1000);
        continue;
      }
      throw new Error(`HTTP ${response.status}: ${errorText(raw)}`);
    }
  }
  async function json(method, path, payload) {
    const data = await (await request(method, path, payload)).json();
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error('接口返回了非对象 JSON');
    return data;
  }
  async function download(identifier) {
    const output = resolve(args.output);
    await mkdir(dirname(output), { recursive: true });
    const temporary = output + '.' + randomUUID() + '.part';
    let handle;
    let reader;
    try {
      const response = await request('GET', '/videos/' + encodeURIComponent(identifier) + '/content', undefined, 'video/*,application/octet-stream');
      const type = (response.headers.get('content-type') || '').split(';')[0].toLowerCase();
      const contentLength = response.headers.get('content-length') || '';
      if (!(type.startsWith('video/') || type === 'application/octet-stream')) {
        throw new Error('内容接口未返回视频：' + errorText(await response.text()));
      }
      reader = response.body.getReader();
      let first = Buffer.alloc(0);
      let count = 0;
      while (true) {
        remaining();
        const { done, value } = await reader.read();
        if (done) break;
        count += value.length;
        if (count > MAX_VIDEO_BYTES) throw new Error('视频超过示例客户端的 512 MiB 下载限制');
        if (!handle) {
          first = Buffer.concat([first, Buffer.from(value)]);
          if (first.length < 12) continue;
          if (!(first.subarray(4, 8).equals(Buffer.from('ftyp')) || first.subarray(0, 4).equals(Buffer.from([0x1a, 0x45, 0xdf, 0xa3])))) {
            throw new Error('内容不是预期的 MP4/WebM，未写入输出文件');
          }
          handle = await open(temporary, 'wx');
          await handle.writeFile(first);
          first = Buffer.alloc(0);
        } else { await handle.writeFile(value); }
      }
      if (!handle) throw new Error('视频内容为空或不完整');
      if (/^\d+$/.test(contentLength) && count !== Number(contentLength)) throw new Error('下载内容不完整，未替换输出文件；可用原任务 ID 重试');
      remaining();
      await handle.close(); handle = undefined;
      await rename(temporary, output);
      console.log(`已保存：${output}（${count} 字节）`);
    } finally {
      if (reader) await reader.cancel().catch(() => {});
      if (handle) await handle.close().catch(() => {});
      await unlink(temporary).catch(error => { if (error.code !== 'ENOENT') throw error; });
    }
  }

  let current;
  let identifier = args['task-id'];
  if (args.payload) {
    const payload = JSON.parse((await readFile(args.payload, 'utf8')).replace(/^\uFEFF/, ''));
    if (!payload || Array.isArray(payload) || typeof payload !== 'object' || !payload.model || !payload.prompt) {
      throw new Error('请求体必须是包含 model 和 prompt 的 JSON 对象');
    }
    current = await json('POST', '/videos', payload);
    identifier = String(current.request_id || current.id || current.task_id || '').trim();
    if (!identifier) throw new Error('创建响应没有任务 ID；请先检查历史，勿立即重新创建');
  }
  identifier = identifier.trim();
  if (!identifier) throw new Error('任务 ID 不能为空');
  console.log('任务 ID：' + identifier);
  let lastStatus;
  while (true) {
    remaining();
    current ??= await json('GET', '/videos/' + encodeURIComponent(identifier));
    const status = String(current.status || '').trim().toLowerCase();
    if (status !== lastStatus) { console.log('状态：' + (status || '已受理，等待查询')); lastStatus = status; }
    if (FAILED.has(status)) throw new Error('任务失败：' + JSON.stringify(current.error || status));
    if (COMPLETE.has(status)) { await download(identifier); return; }
    if (!PENDING.has(status)) throw new Error('未知任务状态：' + status + '；保留任务 ID 后联系服务方');
    await pause(5000);
    current = undefined;
  }
}

main().catch(error => {
  console.error(key ? String(error.message).split(key).join('[REDACTED]') : error.message);
  process.exitCode = 1;
});
