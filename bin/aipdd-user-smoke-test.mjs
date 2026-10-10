#!/usr/bin/env node

// Smoke-test the eight AIPDD categories from a normal NewAPI user's perspective.
// The upstream AIPDD API key must be configured on the AIPDD channel by an admin;
// this script only sends the user's NewAPI token to NewAPI.
//
// Every selected category is called through the gateway, and the change in
// /v1/dashboard/billing/usage between the two reads is reported as the billed
// amount, so a model that is not billed at all (or billed by the wrong rule)
// shows up here instead of in production.

const DEFAULT_TIMEOUT_SECONDS = 15 * 60;
const DEFAULT_POLL_INTERVAL_SECONDS = 10;
const DEFAULT_BASE_URL = 'http://localhost:6070/';
const QUOTA_PER_UNIT = 500 * 1000;

const LEGACY_AIPDD_MODELS = [
  'aipdd-flux-gguf',
  'aipdd-flux-gguf-t2i',
  'aipdd-wan2.2-wanx',
  'aipdd-wan2.2-animater',
  'aipdd-mimic-motion',
  'aipdd-latentsync-1.5',
  'aipdd-indextts',
  'aipdd_ltx_2.3',
  'aipdd_ltx_2.3 (首尾帧)',
];

const args = parseArgs(process.argv.slice(2));

const config = {
  baseUrl: stripTrailingSlash(args['base-url'] || env('NEW_API_BASE_URL') || DEFAULT_BASE_URL),
  token: args.token || env('NEW_API_TOKEN') || env('NEWAPI_TOKEN') || env('NEWAPI_API_KEY'),
  timeoutSeconds: toInt(args['timeout-seconds'] || env('AIPDD_TEST_TIMEOUT_SECONDS'), DEFAULT_TIMEOUT_SECONDS),
  pollIntervalSeconds: toInt(
    args['poll-interval-seconds'] || env('AIPDD_TEST_POLL_INTERVAL_SECONDS'),
    DEFAULT_POLL_INTERVAL_SECONDS,
  ),
  duration: toInt(args.duration || env('AIPDD_TEST_DURATION'), 1),
  // The gateway validates video durations against the model's execution
  // protocol, so the field names differ: token_market_video reads
  // duration_seconds + video_resolution, shared_task reads duration.
  videoProtocol: String(args['video-protocol'] || env('AIPDD_TEST_VIDEO_PROTOCOL') || 'token-market').toLowerCase(),
  videoResolution: String(args['video-resolution'] || env('AIPDD_TEST_VIDEO_RESOLUTION') || '480p').trim(),
  noPoll: Boolean(args['no-poll']),
  skipMissingAssets: Boolean(args['skip-missing-assets']),
  only: csv(args.only || env('AIPDD_TEST_ONLY')),
  imageUrl: args['image-url'] || env('AIPDD_TEST_IMAGE_URL'),
  imageFile: args['image-file'] || env('AIPDD_TEST_IMAGE_FILE'),
  audioUrl: args['audio-url'] || env('AIPDD_TEST_AUDIO_URL'),
  audioFile: args['audio-file'] || env('AIPDD_TEST_AUDIO_FILE'),
  prompt: args.prompt || env('AIPDD_TEST_PROMPT') || 'A clean product shot with soft studio lighting',
  text: args.text || env('AIPDD_TEST_TEXT') || 'This is an AIPDD IndexTTS smoke test.',
  systemOneModel: String(args['systemone-model'] || env('AIPDD_TEST_SYSTEMONE_MODEL') || '').trim(),
  usageUnit: (args['usage-unit'] || env('AIPDD_USAGE_UNIT') || 'usd-cents').toLowerCase(),
  usdExchangeRate: toFloat(args['usd-exchange-rate'] || env('AIPDD_USD_EXCHANGE_RATE'), 7.3),
  tolerancePercent: toFloat(args['billing-tolerance'] || env('AIPDD_BILLING_TOLERANCE_PERCENT'), 2),
  models: {},
};

const results = [];

main()
  .then(() => checkExitCode())
  .catch((error) => {
    console.error(error?.stack || error?.message || String(error));
    process.exitCode = 1;
  });

async function main() {
  if (args.help || args.h) {
    printUsage();
    return;
  }

  validateBaseConfig();

  await loadVisibleModels();
  reportLegacyModels();

  const tests = buildTests().filter((test) => selectTests([test]).length > 0);
  if (tests.length === 0) {
    throw new Error('No tests selected.');
  }

  console.log(`Base URL: ${config.baseUrl}`);
  console.log(`Selected: ${tests.map((test) => test.name).join(', ')}`);
  console.log(config.noPoll ? 'Mode: create only' : `Mode: create and poll (${config.pollIntervalSeconds}s interval)`);
  console.log(`Billing reconciliation: ${config.usageUnit}, tolerance ${config.tolerancePercent}%`);
  console.log('');

  for (const test of tests) {
    results.push(await runTest(test));
  }

  console.log('');
  console.log('Summary');
  for (const result of results) {
    const suffix = result.output?.length ? ` output=${result.output.join(',')}` : '';
    const billed = result.billed === null ? '-' : result.billed.toFixed(6);
    console.log(
      `${result.ok ? 'PASS' : 'FAIL'} ${result.name} task_id=${result.taskId || '-'} status=${result.status || '-'} billed=${billed}${suffix}`,
    );
  }
}

function checkExitCode() {
  if (results.some((result) => !result.ok)) {
    process.exitCode = 1;
  }
}

function buildTests() {
  return [
    {
      name: 'text',
      category: '文字模型',
      model: modelOverride('TEXT', 'ap-chat-3.8-flash'),
      kind: 'sync',
      path: '/v1/chat/completions',
      billing: { kind: 'tokens' },
      body: () => ({
        model: modelOverride('TEXT', 'ap-chat-3.8-flash'),
        messages: [{ role: 'user', content: '用三句话解释 AIPDD 网关的作用' }],
        max_tokens: 128,
      }),
    },
    {
      name: 'embeddings',
      category: '向量模型',
      model: modelOverride('EMBEDDINGS', 'ap-qwen3-embedding:0.6b'),
      kind: 'sync',
      path: '/v1/embeddings',
      billing: { kind: 'tokens' },
      body: () => ({
        model: modelOverride('EMBEDDINGS', 'ap-qwen3-embedding:0.6b'),
        input: 'AIPDD embedding smoke test',
      }),
    },
    {
      name: 'image-generation',
      category: '图片生成',
      model: modelOverride('IMAGE_GENERATION', 'ap-image-2.0'),
      kind: 'sync',
      path: '/v1/images/generations',
      billing: { kind: 'per_call' },
      body: () => ({
        model: modelOverride('IMAGE_GENERATION', 'ap-image-2.0'),
        prompt: config.prompt,
        size: '1024x1024',
        n: 1,
      }),
    },
    {
      name: 'image-edits',
      category: '图生图',
      // The generators accept reference images on /v1/images/edits, and
      // ap-qwen-image-edit currently answers 503 upstream, so a generator is
      // the default and the dedicated edit model stays available as an override.
      model: modelOverride('IMAGE_EDITS', 'ap-image-2.0'),
      kind: 'sync-multipart',
      path: '/v1/images/edits',
      requiredAssets: ['image'],
      billing: { kind: 'per_call' },
      form: async () => {
        const image = await loadAsset(config.imageFile, config.imageUrl, 'image/png');
        return {
          fields: {
            model: modelOverride('IMAGE_EDITS', 'ap-image-2.0'),
            prompt: `${config.prompt}, edited background`,
          },
          files: { image: { name: 'input.png', type: 'image/png', data: image } },
        };
      },
    },
    {
      name: 'video',
      category: '视频生成',
      model: modelOverride('VIDEO', 'ap-minimax-h3'),
      kind: 'async',
      path: '/v1/videos',
      fetchPath: (taskId) => `/v1/videos/${encodeURIComponent(taskId)}`,
      billing: { kind: 'per_second', units: config.duration },
      body: () => videoBody(),
    },
    {
      name: 'audio-speech',
      category: '音频模型',
      model: modelOverride('AUDIO_SPEECH', 'ap-indextts-2'),
      kind: 'async',
      path: '/v1/audio/speech',
      fetchPath: (taskId) => `/v1/audio/speech/${encodeURIComponent(taskId)}`,
      requiredAssets: ['audioUrl'],
      billing: { kind: 'per_call' },
      body: () => ({
        model: modelOverride('AUDIO_SPEECH', 'ap-indextts-2'),
        input: config.text,
        metadata: { audio: config.audioUrl },
      }),
    },
    {
      name: 'transcriptions',
      category: '语音识别',
      model: modelOverride('TRANSCRIPTIONS', 'ap-funasr'),
      kind: 'sync-multipart',
      path: '/v1/audio/transcriptions',
      requiredAssets: ['audio'],
      billing: { kind: 'per_second', units: null },
      form: async () => {
        const audio = await loadAsset(config.audioFile, config.audioUrl, 'audio/wav');
        return {
          fields: { model: modelOverride('TRANSCRIPTIONS', 'ap-funasr') },
          files: { file: { name: 'audio.wav', type: 'audio/wav', data: audio } },
        };
      },
    },
    {
      name: 'systemone',
      category: '决策模型',
      model: systemOneModel(),
      kind: 'sync',
      path: '/v1/systemone',
      billing: { kind: 'tokens' },
      // Upstream has not published the decision model in the catalog yet, so
      // this case only runs when an operator names the model explicitly.
      skip: () => (systemOneModel() ? '' : 'AIPDD_TEST_SYSTEMONE_MODEL/--systemone-model is not set (upstream catalog does not publish a decision model yet)'),
      body: () => ({
        // /v1/systemone is not OpenAI-shaped: dto.SystemOneRequest requires
        // model + state + questions and forwards both opaque objects upstream.
        model: systemOneModel(),
        state: { campaign: 'autumn-launch', current_price: 39 },
        questions: { price_up: 'Should we raise the price for this campaign?' },
      }),
    },
  ];
}

function modelOverride(category, fallback) {
  return env(`AIPDD_SMOKE_MODEL_${category}`) || fallback;
}

function systemOneModel() {
  return config.systemOneModel || modelOverride('SYSTEMONE', '');
}

function videoBody() {
  const body = {
    model: modelOverride('VIDEO', 'ap-minimax-h3'),
    prompt: 'Slow camera push in on a product, cinematic lighting',
  };
  if (config.videoProtocol === 'shared-task') {
    body.duration = config.duration;
    return body;
  }
  body.duration_seconds = config.duration;
  body.video_resolution = config.videoResolution;
  return body;
}

function selectTests(tests) {
  const selectedNames = new Set(config.only);
  return tests.filter((test) => {
    if (selectedNames.size > 0 && !selectedNames.has(test.name) && !selectedNames.has(test.model)) {
      return false;
    }
    const missing = (test.requiredAssets || []).filter((key) => !hasAsset(key));
    if (missing.length === 0) {
      return true;
    }
    const message = `${test.name} requires ${missing.map(assetEnvName).join(', ')}`;
    if (config.skipMissingAssets) {
      console.warn(`SKIP ${message}`);
      return false;
    }
    throw new Error(`${message}. Pass the files or URLs with args or environment variables.`);
  });
}

function hasAsset(key) {
  if (key === 'image') {
    return Boolean(config.imageFile || config.imageUrl);
  }
  if (key === 'audio') {
    return Boolean(config.audioFile || config.audioUrl);
  }
  return Boolean(config[key]);
}

async function runTest(test) {
  const skipReason = test.skip ? test.skip() : '';
  if (skipReason) {
    console.log(`SKIP ${test.name} (${test.category}): ${skipReason}`);
    return { name: test.name, ok: true, skipped: true, billed: null };
  }
  console.log(`${test.kind === 'async' ? 'CREATE' : 'CALL'} ${test.name} (${test.model})`);
  const usageBefore = await readUsage();
  try {
    const created = test.kind === 'sync' ? await request('POST', test.path, test.body())
      : test.kind === 'sync-multipart' ? await requestForm('POST', test.path, await test.form())
        : await request('POST', test.path, test.body());
    const taskId = getTaskId(created);
    const createdStatus = getStatus(created);
    if (taskId) {
      console.log(`  task_id=${taskId}`);
    } else if (test.kind === 'async') {
      throw new Error(`Create response does not contain task_id/id: ${JSON.stringify(created)}`);
    }

    let finalResponse = created;
    if (test.kind === 'async' && !isTerminalStatus(createdStatus) && !config.noPoll) {
      finalResponse = await pollTask(test, taskId);
    }
    const status = getStatus(finalResponse) || (test.kind === 'async' ? 'unknown' : 'completed');
    const output = getOutputUrls(finalResponse);
    const ok = test.kind === 'async' && !config.noPoll ? isSuccessStatus(status) : true;
    const billed = await settleBilling(test, usageBefore, finalResponse);
    console.log(`  final_status=${status}${output.length ? ` output=${output.join(',')}` : ''} billed=${describeBilled(billed)}`);
    return { name: test.name, ok, taskId, status, output, billed };
  } catch (error) {
    console.error(`  ERROR ${error?.message || error}`);
    return { name: test.name, ok: false, billed: null };
  }
}

// The usage endpoint is token-scoped or user-scoped depending on server config,
// so it is read as an opaque number: the delta is the amount this call cost in
// the server's own display unit.
async function settleBilling(test, usageBefore, finalResponse) {
  const usageAfter = await readUsage();
  const delta = usageAfter === null || usageBefore === null ? null : usageAfter - usageBefore;
  const expectedRaw = expectedUsage(test, finalResponse);
  if (delta === null) {
    console.warn(`  billing: usage endpoint unavailable, delta not measured`);
    return null;
  }
  if (delta <= 0) {
    throw new Error(`${test.name} was not billed (usage delta ${delta}). Check the model price imported from the AIPDD catalog.`);
  }
  if (expectedRaw === null) {
    return delta;
  }
  const tolerance = Math.max(expectedRaw * (config.tolerancePercent / 100), 1e-9);
  if (Math.abs(delta - expectedRaw) > tolerance) {
    throw new Error(
      `${test.name} billed ${delta} but the catalog price expects ${expectedRaw} ` +
        `(±${config.tolerancePercent}%). Check the unit and the imported price.`,
    );
  }
  return delta;
}

function expectedUsage(test, finalResponse) {
  const expectedUSD = expectedUnitUSD(test);
  if (expectedUSD === null) {
    return null;
  }
  let units = 1;
  if (test.billing.kind === 'per_second') {
    units = billedSeconds(test, finalResponse);
    if (units === null) {
      return null;
    }
  }
  return convertUSD(expectedUSD * units);
}

function expectedUnitUSD(test) {
  const raw = env(`AIPDD_SMOKE_EXPECTED_USD_${test.name.toUpperCase().replace(/-/g, '_')}`);
  if (!raw) {
    return null;
  }
  const value = toFloat(raw, Number.NaN);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`Expected price for ${test.name} must be a positive USD amount, got ${raw}`);
  }
  return value;
}

function billedSeconds(test, finalResponse) {
  const reported = getNumeric(finalResponse, ['duration', 'usage.duration', 'data.duration', 'metadata.duration']);
  if (reported !== undefined && reported > 0) {
    return reported;
  }
  return test.billing.units ?? null;
}

function convertUSD(amountUSD) {
  if (config.usageUnit === 'raw-quota') {
    return amountUSD * QUOTA_PER_UNIT;
  }
  if (config.usageUnit === 'cny-fen') {
    return amountUSD * 100 * config.usdExchangeRate;
  }
  return amountUSD * 100;
}

function describeBilled(billed) {
  if (billed === null || billed === undefined) {
    return 'not measured';
  }
  return `${billed} ${config.usageUnit}`;
}

async function readUsage() {
  try {
    const response = await request('GET', '/v1/dashboard/billing/usage');
    const value = response?.total_usage;
    return typeof value === 'number' && Number.isFinite(value) ? value : null;
  } catch (error) {
    console.warn(`  usage read failed: ${error?.message || error}`);
    return null;
  }
}

async function loadVisibleModels() {
  try {
    const response = await request('GET', '/v1/models');
    config.models = Object.fromEntries(
      (response?.data || []).filter((item) => typeof item?.id === 'string').map((item) => [item.id, item]),
    );
    config.visibleModelIds = Object.keys(config.models);
  } catch (error) {
    console.warn(`GET /v1/models failed: ${error?.message || error}`);
    config.visibleModelIds = [];
  }
}

function reportLegacyModels() {
  const stillVisible = LEGACY_AIPDD_MODELS.filter((name) => config.visibleModelIds.includes(name));
  if (stillVisible.length > 0) {
    throw new Error(`Retired ComfyUI models are still listed by /v1/models: ${stillVisible.join(', ')}`);
  }
  console.log(`Legacy check: none of the ${LEGACY_AIPDD_MODELS.length} retired ComfyUI models are listed by /v1/models.`);
  if (config.visibleModelIds.length > 0) {
    console.log(`Visible models: ${config.visibleModelIds.length}`);
  }
}

async function pollTask(test, taskId) {
  const startedAt = Date.now();
  let lastResponse;
  while (Date.now() - startedAt < config.timeoutSeconds * 1000) {
    await sleep(config.pollIntervalSeconds * 1000);
    lastResponse = await request('GET', test.fetchPath(taskId));
    const status = getStatus(lastResponse);
    const output = getOutputUrls(lastResponse);
    console.log(`  poll status=${status || 'unknown'}${output.length ? ` output=${output.join(',')}` : ''}`);
    if (isTerminalStatus(status)) {
      return lastResponse;
    }
  }
  throw new Error(`Timed out after ${config.timeoutSeconds}s. Last response: ${JSON.stringify(lastResponse)}`);
}

async function request(method, path, body) {
  const response = await fetch(`${config.baseUrl}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${config.token}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  const parsed = text ? parseJson(text) : null;
  if (!response.ok) {
    throw new Error(`${method} ${path} failed with HTTP ${response.status}: ${text}`);
  }
  return parsed;
}

async function requestForm(method, path, { fields = {}, files = {} }) {
  const form = new FormData();
  for (const [key, value] of Object.entries(fields)) {
    form.append(key, String(value));
  }
  for (const [key, file] of Object.entries(files)) {
    form.append(key, new Blob([file.data], { type: file.type }), file.name);
  }
  const response = await fetch(`${config.baseUrl}${path}`, {
    method,
    headers: { Authorization: `Bearer ${config.token}` },
    body: form,
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`${method} ${path} failed with HTTP ${response.status}: ${text}`);
  }
  return text ? parseJson(text) : null;
}

async function loadAsset(filePath, url, fallbackType) {
  if (filePath) {
    const fs = await import('node:fs/promises');
    return new Uint8Array(await fs.readFile(filePath));
  }
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Asset download failed with HTTP ${response.status}: ${url}`);
  }
  return new Uint8Array(await response.arrayBuffer());
}

function getTaskId(value) {
  return pick(value, ['task_id', 'id']) || pick(value?.data, ['task_id', 'id']);
}

function getStatus(value) {
  return pick(value, ['status']) || pick(value?.data, ['status']) || pick(value?.data?.data, ['status']);
}

function getOutputUrls(value) {
  const candidates = [
    pick(value, ['result_url', 'url']),
    ...(Array.isArray(value?.output) ? value.output : []),
    ...(Array.isArray(value?.metadata?.urls) ? value.metadata.urls : []),
    ...(Array.isArray(value?.data?.url) ? value.data.url : []),
    ...(Array.isArray(value?.data) ? value.data.map((item) => item?.url || item?.b64_json) : []),
    pick(value?.data, ['result_url', 'url']),
    ...(Array.isArray(value?.data?.output) ? value.data.output : []),
    ...(Array.isArray(value?.data?.metadata?.urls) ? value.data.metadata.urls : []),
  ];
  return [...new Set(candidates.filter((item) => typeof item === 'string' && item.trim() !== ''))];
}

function pick(value, keys) {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  for (const key of keys) {
    if (typeof value[key] === 'string' && value[key].trim() !== '') {
      return value[key];
    }
  }
  return undefined;
}

function getNumeric(value, paths) {
  for (const path of paths) {
    let current = value;
    for (const part of path.split('.')) {
      current = current?.[part];
    }
    if (typeof current === 'number' && Number.isFinite(current)) {
      return current;
    }
  }
  return undefined;
}

function isTerminalStatus(status) {
  if (!status) {
    return false;
  }
  const normalized = status.toLowerCase();
  return ['success', 'succeeded', 'completed', 'failure', 'failed', 'cancelled', 'canceled', 'error'].includes(normalized);
}

function isSuccessStatus(status) {
  if (!status) {
    return false;
  }
  const normalized = status.toLowerCase();
  return ['success', 'succeeded', 'completed'].includes(normalized);
}

function validateBaseConfig() {
  if (!config.baseUrl) {
    throw new Error('Missing NewAPI base URL. Set NEW_API_BASE_URL or pass --base-url.');
  }
  if (!config.token) {
    throw new Error('Missing NewAPI token. Set NEW_API_TOKEN or pass --token.');
  }
  if (!['token-market', 'shared-task'].includes(config.videoProtocol)) {
    throw new Error('AIPDD_TEST_VIDEO_PROTOCOL/--video-protocol must be token-market or shared-task.');
  }
  const maxDuration = config.videoProtocol === 'shared-task' ? 20 : 15;
  if (config.duration < 1 || config.duration > maxDuration) {
    throw new Error(`AIPDD_TEST_DURATION/--duration must be between 1 and ${maxDuration} seconds.`);
  }
  if (!config.videoResolution) {
    throw new Error('AIPDD_TEST_VIDEO_RESOLUTION/--video-resolution must not be empty.');
  }
  if (!['usd-cents', 'cny-fen', 'raw-quota'].includes(config.usageUnit)) {
    throw new Error('AIPDD_USAGE_UNIT/--usage-unit must be usd-cents, cny-fen or raw-quota.');
  }
}

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith('--')) {
      continue;
    }
    const eq = arg.indexOf('=');
    if (eq > 0) {
      out[arg.slice(2, eq)] = arg.slice(eq + 1);
      continue;
    }
    const key = arg.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith('--')) {
      out[key] = true;
    } else {
      out[key] = next;
      i += 1;
    }
  }
  return out;
}

function env(key) {
  return process.env[key]?.trim();
}

function csv(value) {
  if (!value) {
    return [];
  }
  return String(value)
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
}

function toInt(value, fallback) {
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  const parsed = Number.parseInt(String(value), 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function toFloat(value, fallback) {
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  const parsed = Number.parseFloat(String(value));
  return Number.isFinite(parsed) ? parsed : fallback;
}

function stripTrailingSlash(value) {
  return String(value || '').replace(/\/+$/, '');
}

function assetEnvName(key) {
  return (
    {
      image: 'AIPDD_TEST_IMAGE_FILE or AIPDD_TEST_IMAGE_URL',
      audio: 'AIPDD_TEST_AUDIO_FILE or AIPDD_TEST_AUDIO_URL',
      audioUrl: 'AIPDD_TEST_AUDIO_URL',
    }[key] || key
  );
}

function parseJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return { raw: text };
  }
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function printUsage() {
  console.log(`
Usage:
node bin/aipdd-user-smoke-test.mjs --base-url http://localhost:6070/ --token sk-newapi-token \\
    --image-file ./input.png --audio-file ./reference.wav \\
    --audio-url https://example.com/reference.wav

Categories (see --only):
  text             文字模型   POST /v1/chat/completions        per token
  embeddings       向量模型   POST /v1/embeddings              per token
  image-generation 图片生成   POST /v1/images/generations      per call
  image-edits      图生图     POST /v1/images/edits            per call (multipart)
  video            视频生成   POST /v1/videos + poll           per second
  audio-speech     音频模型   POST /v1/audio/speech + poll     per call
  transcriptions   语音识别   POST /v1/audio/transcriptions    per second (multipart)
  systemone        决策模型   POST /v1/systemone               requires a published model name

Environment variables:
  NEW_API_BASE_URL                 NewAPI base URL, default http://localhost:6070/
  NEW_API_TOKEN                    User NewAPI token
  AIPDD_TEST_IMAGE_FILE            Local image file for /v1/images/edits
  AIPDD_TEST_IMAGE_URL             Image URL fallback when no local file is given
  AIPDD_TEST_AUDIO_FILE            Local audio file for /v1/audio/transcriptions
  AIPDD_TEST_AUDIO_URL             Reference-audio URL for 音频模型 (upstream needs a URL)
  AIPDD_TEST_SYSTEMONE_MODEL       Decision model name, once upstream publishes one
  AIPDD_TEST_DURATION              Video duration in seconds, default 1
  AIPDD_TEST_VIDEO_PROTOCOL        token-market (default) or shared-task
  AIPDD_TEST_VIDEO_RESOLUTION      Token-market video resolution, default 480p
  AIPDD_USAGE_UNIT                 usd-cents (default), cny-fen or raw-quota
  AIPDD_USD_EXCHANGE_RATE          Rate used when comparing cny-fen usage, default 7.3
  AIPDD_BILLING_TOLERANCE_PERCENT  Allowed deviation from the catalog price, default 2
  AIPDD_TEST_ONLY                  Comma-separated test names or model names
  AIPDD_SMOKE_MODEL_<CATEGORY>     Override the catalog model of a category
  AIPDD_SMOKE_EXPECTED_USD_<NAME>  Expected unit price in USD for billing reconciliation

Options:
  --base-url, --token              Same meaning as the environment variables above
  --no-poll                        Only create tasks, do not poll completion
  --skip-missing-assets            Skip tests whose required assets are missing
  --only text,embeddings,image-edits
  --duration N                     视频生成 seconds, default 1
  --video-protocol token-market|shared-task
                                   Which duration field the video model accepts
  --video-resolution 480p          token-market resolution (480p, 768p or 1080p)
`);
}
