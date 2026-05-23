#!/usr/bin/env node

// Smoke-test AIPDD task models from a normal NewAPI user's perspective.
// The upstream AIPDD API key must be configured on the AIPDD channel by an admin.
// This script only sends the user's NewAPI token to NewAPI.

const DEFAULT_TIMEOUT_SECONDS = 15 * 60;
const DEFAULT_POLL_INTERVAL_SECONDS = 10;
const DEFAULT_BASE_URL = 'https://newapi.jumcp.com/';

const args = parseArgs(process.argv.slice(2));

const config = {
  baseUrl: stripTrailingSlash(args['base-url'] || env('NEW_API_BASE_URL') || DEFAULT_BASE_URL),
  token: args.token || env('NEW_API_TOKEN') || env('NEWAPI_TOKEN') || env('NEWAPI_API_KEY'),
  timeoutSeconds: toInt(args['timeout-seconds'] || env('AIPDD_TEST_TIMEOUT_SECONDS'), DEFAULT_TIMEOUT_SECONDS),
  pollIntervalSeconds: toInt(
    args['poll-interval-seconds'] || env('AIPDD_TEST_POLL_INTERVAL_SECONDS'),
    DEFAULT_POLL_INTERVAL_SECONDS,
  ),
  duration: toInt(args.duration || env('AIPDD_TEST_DURATION'), 5),
  noPoll: Boolean(args['no-poll']),
  skipMissingAssets: Boolean(args['skip-missing-assets']),
  only: csv(args.only || env('AIPDD_TEST_ONLY')),
  imageUrl: args['image-url'] || env('AIPDD_TEST_IMAGE_URL'),
  videoUrl: args['video-url'] || env('AIPDD_TEST_VIDEO_URL'),
  motionVideoUrl: args['motion-video-url'] || env('AIPDD_TEST_MOTION_VIDEO_URL'),
  appearanceImageUrl: args['appearance-image-url'] || env('AIPDD_TEST_APPEARANCE_IMAGE_URL'),
  audioUrl: args['audio-url'] || env('AIPDD_TEST_AUDIO_URL'),
  prompt: args.prompt || env('AIPDD_TEST_PROMPT') || 'A clean product shot with soft studio lighting',
  text: args.text || env('AIPDD_TEST_TEXT') || 'This is an AIPDD IndexTTS smoke test.',
};

config.motionVideoUrl ||= config.videoUrl;
config.appearanceImageUrl ||= config.imageUrl;

main().catch((error) => {
  console.error(error?.stack || error?.message || String(error));
  process.exitCode = 1;
});

async function main() {
  if (args.help || args.h) {
    printUsage();
    return;
  }

  validateBaseConfig();

  const tests = buildTests();
  const selectedTests = selectTests(tests);
  if (selectedTests.length === 0) {
    throw new Error('No tests selected.');
  }

  console.log(`Base URL: ${config.baseUrl}`);
  console.log(`Selected: ${selectedTests.map((test) => test.name).join(', ')}`);
  console.log(config.noPoll ? 'Mode: create only' : `Mode: create and poll (${config.pollIntervalSeconds}s interval)`);
  console.log('');

  const results = [];
  for (const test of selectedTests) {
    results.push(await runTest(test));
  }

  console.log('');
  console.log('Summary');
  for (const result of results) {
    const suffix = result.output?.length ? ` output=${result.output.join(',')}` : '';
    console.log(`${result.ok ? 'PASS' : 'FAIL'} ${result.name} task_id=${result.taskId || '-'} status=${result.status || '-'}${suffix}`);
  }

  if (results.some((result) => !result.ok)) {
    process.exitCode = 1;
  }
}

function buildTests() {
  return [
    {
      name: 'flux-i2i',
      model: 'aipdd-flux-gguf',
      createPath: '/v1/images/generations',
      fetchPath: (taskId) => `/v1/images/generations/${encodeURIComponent(taskId)}`,
      requiredAssets: ['imageUrl'],
      body: {
        model: 'aipdd-flux-gguf',
        image: config.imageUrl,
        prompt: config.prompt,
      },
    },
    {
      name: 'flux-t2i',
      model: 'aipdd-flux-gguf-t2i',
      createPath: '/v1/images/generations',
      fetchPath: (taskId) => `/v1/images/generations/${encodeURIComponent(taskId)}`,
      requiredAssets: [],
      body: {
        model: 'aipdd-flux-gguf-t2i',
        prompt: config.prompt,
      },
    },
    {
      name: 'wanx',
      model: 'aipdd-wan2.2-wanx',
      createPath: '/v1/videos',
      fetchPath: (taskId) => `/v1/videos/${encodeURIComponent(taskId)}`,
      requiredAssets: ['imageUrl'],
      body: {
        model: 'aipdd-wan2.2-wanx',
        prompt: 'Slow camera push in, cinematic movement',
        image: config.imageUrl,
        duration: config.duration,
      },
    },
    {
      name: 'animater',
      model: 'aipdd-wan2.2-animater',
      createPath: '/v1/videos',
      fetchPath: (taskId) => `/v1/videos/${encodeURIComponent(taskId)}`,
      requiredAssets: ['videoUrl'],
      body: {
        model: 'aipdd-wan2.2-animater',
        load_video: config.videoUrl,
        filename: filenameFromUrl(config.videoUrl),
        WanVideoTextEncodeCached_positive_prompt: 'natural motion, stable subject',
        WanVideoTextEncodeCached_negative_prompt: 'low quality, distorted, flicker',
      },
    },
    {
      name: 'mimic-motion',
      model: 'aipdd-mimic-motion',
      createPath: '/v1/videos',
      fetchPath: (taskId) => `/v1/videos/${encodeURIComponent(taskId)}`,
      requiredAssets: ['motionVideoUrl', 'appearanceImageUrl'],
      body: {
        model: 'aipdd-mimic-motion',
        motion_video: config.motionVideoUrl,
        appearance_image: config.appearanceImageUrl,
      },
    },
    {
      name: 'latentsync',
      model: 'aipdd-latentsync-1.5',
      createPath: '/v1/videos',
      fetchPath: (taskId) => `/v1/videos/${encodeURIComponent(taskId)}`,
      requiredAssets: ['videoUrl', 'audioUrl'],
      body: {
        model: 'aipdd-latentsync-1.5',
        video: config.videoUrl,
        filename: filenameFromUrl(config.videoUrl),
        LoadAudio: config.audioUrl,
      },
    },
    {
      name: 'indextts',
      model: 'aipdd-indextts',
      createPath: '/v1/audio/speech',
      fetchPath: (taskId) => `/v1/audio/speech/${encodeURIComponent(taskId)}`,
      requiredAssets: ['audioUrl'],
      body: {
        model: 'aipdd-indextts',
        input: config.text,
        metadata: {
          audio: config.audioUrl,
        },
      },
    },
  ];
}

function selectTests(tests) {
  const selectedNames = new Set(config.only);
  return tests.filter((test) => {
    if (selectedNames.size > 0 && !selectedNames.has(test.name) && !selectedNames.has(test.model)) {
      return false;
    }
    const missing = test.requiredAssets.filter((key) => !config[key]);
    if (missing.length === 0) {
      return true;
    }
    const message = `${test.name} requires ${missing.map(assetEnvName).join(', ')}`;
    if (config.skipMissingAssets) {
      console.warn(`SKIP ${message}`);
      return false;
    }
    throw new Error(`${message}. Pass the URLs with args or environment variables.`);
  });
}

async function runTest(test) {
  console.log(`CREATE ${test.name} (${test.model})`);
  try {
    const createResponse = await request('POST', test.createPath, test.body);
    const taskId = getTaskId(createResponse);
    if (!taskId) {
      throw new Error(`Create response does not contain task_id/id: ${JSON.stringify(createResponse)}`);
    }
    console.log(`  task_id=${taskId}`);

    if (config.noPoll) {
      return { name: test.name, ok: true, taskId, status: getStatus(createResponse) || 'created' };
    }

    const finalResponse = await pollTask(test, taskId);
    const status = getStatus(finalResponse);
    const output = getOutputUrls(finalResponse);
    const ok = isSuccessStatus(status);
    console.log(`  final_status=${status || 'unknown'}${output.length ? ` output=${output.join(',')}` : ''}`);
    return { name: test.name, ok, taskId, status, output };
  } catch (error) {
    console.error(`  ERROR ${error?.message || error}`);
    return { name: test.name, ok: false };
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
  if (!config.token) {
    throw new Error('Missing NewAPI token. Set NEW_API_TOKEN or pass --token.');
  }
  if (config.duration !== 5 && config.duration !== 10) {
    throw new Error('AIPDD_TEST_DURATION/--duration must be 5 or 10.');
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

function stripTrailingSlash(value) {
  return String(value || '').replace(/\/+$/, '');
}

function filenameFromUrl(value) {
  if (!value) {
    return '';
  }
  try {
    const url = new URL(value);
    return decodeURIComponent(url.pathname.split('/').filter(Boolean).pop() || 'input.mp4');
  } catch {
    return String(value).split('/').filter(Boolean).pop() || 'input.mp4';
  }
}

function assetEnvName(key) {
  return {
    imageUrl: 'AIPDD_TEST_IMAGE_URL',
    videoUrl: 'AIPDD_TEST_VIDEO_URL',
    motionVideoUrl: 'AIPDD_TEST_MOTION_VIDEO_URL or AIPDD_TEST_VIDEO_URL',
    appearanceImageUrl: 'AIPDD_TEST_APPEARANCE_IMAGE_URL or AIPDD_TEST_IMAGE_URL',
    audioUrl: 'AIPDD_TEST_AUDIO_URL',
  }[key] || key;
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
  node bin/aipdd-user-smoke-test.mjs --base-url https://newapi.jumcp.com/ --token sk-newapi-token \\
    --image-url https://example.com/input.png \\
    --video-url https://example.com/input.mp4 \\
    --audio-url https://example.com/reference.wav

Environment variables:
  NEW_API_BASE_URL                 NewAPI base URL, default https://newapi.jumcp.com/
  NEW_API_TOKEN                    User NewAPI token
  AIPDD_TEST_IMAGE_URL             Public input image URL
  AIPDD_TEST_VIDEO_URL             Public input video URL
  AIPDD_TEST_AUDIO_URL             Public input audio URL
  AIPDD_TEST_MOTION_VIDEO_URL      Optional motion video URL, defaults to video URL
  AIPDD_TEST_APPEARANCE_IMAGE_URL  Optional appearance image URL, defaults to image URL
  AIPDD_TEST_DURATION              5 or 10, default 5
  AIPDD_TEST_ONLY                  Comma-separated test names or model names

Options:
  --no-poll                        Only create tasks, do not poll completion
  --skip-missing-assets            Skip tests whose required asset URLs are missing
  --only flux-i2i,flux-t2i,wanx    Run selected tests only
`);
}
