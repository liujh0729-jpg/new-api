import crypto from 'node:crypto';
import fs from 'node:fs/promises';
import path from 'node:path';
import { spawn } from 'node:child_process';

export const DEFAULT_BASE_URL = 'http://localhost:6070';
export const DEFAULT_MAX_COST_USD = 12;
export const DEFAULT_TIMEOUT_SECONDS = 30 * 60;
export const DEFAULT_POLL_INTERVAL_SECONDS = 10;
export const REFERENCE_VIDEO_PLACEHOLDER = '{{REFERENCE_VIDEO}}';

export const SEEDANCE_MODELS = Object.freeze({
  vip: 'AP Seedance-2.0 VIP',
  standard: 'AP Seedance-2.0 标准版',
  light: 'AP Seedance-2.0 轻量版',
  value: 'AP Seedance-2.0 高性价比版',
});

const PROMPTS = Object.freeze({
  kineticSculpture:
    'An entirely original non-character kinetic sculpture made of interlocking translucent ceramic rings moves through a brutalist rooftop observatory at sunset. In one continuous macro-to-wide camera move, the rings separate, orbit through three nested axes, pass behind concrete fins, reconnect in the identical topology, catch a wind-blown crimson ribbon, and cast physically consistent caustics across wet stone. Preserve ring count, material identity, ribbon attachment, collision-free motion, occlusion recovery, skyline parallax, reflections and a gradual sunset-to-blue-hour exposure transition. No people, animals, copyrighted characters, brands, text, logos, object duplication, topology mutation, flicker, teleportation, frame tearing, or cuts.',
  rooftop:
    'Preserve the exact identity, facial proportions, short dark hair, bow and school-uniform silhouette from the manga reference. On a high city rooftop at sunset, execute a continuous 180-degree camera arc while she turns from the railing, catches a wind-blown paper charm with her right hand, and looks into camera. Maintain causal hand-object contact, hair and fabric wind response, layered skyline parallax, consistent railing geometry, and a physically plausible sunset-to-blue-hour lighting transition. End on a stable three-quarter close-up. No captions, logos, face drift, extra limbs or fingers, clothing mutation, flicker, teleportation, frame tearing, or sudden camera cuts.',
  station:
    'A single uninterrupted tracking shot through a crowded underground station. Keep the heroine identity and outfit absolutely stable as commuters repeatedly occlude her, then recover the same face and body geometry after every occlusion. She transfers a red ticket from left hand to right hand, passes behind a glass advertising wall, and steps onto a moving escalator. Preserve ticket permanence, hand continuity, reflection timing, foreground-to-background parallax, crowd collision avoidance, and coherent fluorescent light changes. No text, logos, face drift, duplicate people, extra fingers, object popping, flicker, teleportation, or cuts.',
  rain:
    'Night rain chase in a narrow neon street, filmed with a low stabilized pursuit camera that rises into an overhead crane move. The heroine runs with a transparent umbrella, splashes through three puddles, briefly slips, regains balance against a wall, then turns toward camera. Synchronize footfalls, rain impacts and breathing with the supplied audio while preserving umbrella shape, wet coat continuity, facial identity, puddle reflections, splash causality, motion blur direction and neon light transport. No captions, logos, face drift, extra limbs, warped hands, clothing mutation, flicker, teleportation, or broken reflections.',
  mirror:
    'Inside a mirrored elevator, stage one continuous shot with the heroine and her reflection visible simultaneously. She raises her left hand, adjusts the bow with her right hand, watches floor lights change from warm amber to cold white, and exits as the doors open onto a bright atrium. The real and reflected actions must be temporally exact with correct left-right inversion, stable facial identity, consistent depth, door occlusion and exposure adaptation. No written floor numbers, logos, duplicated reflections, extra limbs, face drift, flicker, teleportation, or discontinuous doors.',
  transform:
    'Begin as the supplied black-and-white manga line art and transform gradually into cinematic live-action color without changing the heroine face, hairstyle, bow, uniform silhouette, pose or camera framing. Ink contours become real fabric seams, screentone becomes volumetric sunset haze, and the rooftop gains realistic depth while the original composition remains locked. Add a slow dolly-in and subtle wind motion only after identity is stable. No captions, speech bubbles, logos, face drift, extra limbs, clothing redesign, texture crawling, flicker, teleportation, or cuts.',
  continuation:
    'Continue seamlessly from the reference video first frame and motion state. Preserve the heroine identity, clothing, camera velocity, light direction and environmental geometry. Complete the interrupted movement with causal momentum: she pivots around the railing, catches the same paper charm, crosses behind a foreground pillar, reappears with identical proportions, then returns to a final pose that can loop into the reference opening. Maintain object permanence, occlusion recovery, hair dynamics, parallax and exposure continuity. No captions, logos, face drift, extra limbs, clothing mutation, flicker, teleportation, frame tearing, or cuts.',
});

export function parseArgs(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index += 1) {
    const raw = argv[index];
    if (!raw.startsWith('--')) {
      continue;
    }
    const equalIndex = raw.indexOf('=');
    if (equalIndex > 2) {
      result[raw.slice(2, equalIndex)] = raw.slice(equalIndex + 1);
      continue;
    }
    const key = raw.slice(2);
    const next = argv[index + 1];
    if (!next || next.startsWith('--')) {
      result[key] = true;
      continue;
    }
    result[key] = next;
    index += 1;
  }
  return result;
}

export function csv(value) {
  if (!value) {
    return [];
  }
  return String(value).split(',').map((item) => item.trim()).filter(Boolean);
}

export function toNumber(value, fallback) {
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function stripTrailingSlash(value) {
  return String(value || '').replace(/\/+$/, '');
}

export function redactUrl(value) {
  if (typeof value !== 'string') {
    return value;
  }
  try {
    const parsed = new URL(value);
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return value;
    }
    parsed.search = '';
    parsed.hash = '';
    return parsed.toString();
  } catch {
    return value;
  }
}

export function sanitizeDeep(value) {
  if (Array.isArray(value)) {
    return value.map(sanitizeDeep);
  }
  if (value && typeof value === 'object') {
    const result = {};
    for (const [key, nested] of Object.entries(value)) {
      if (/^(tokenId|tokenName|tokenDeleted|tokenDeleteError)$/i.test(key)) {
        result[key] = sanitizeDeep(nested);
      } else if (/password|token|authorization|cookie|api[_-]?key|signature/i.test(key)) {
        result[key] = '[REDACTED]';
      } else {
        result[key] = sanitizeDeep(nested);
      }
    }
    return result;
  }
  if (typeof value === 'string' && /^https?:\/\//i.test(value)) {
    return redactUrl(value);
  }
  if (typeof value === 'string' && /^(sk-|Bearer\s+)/i.test(value)) {
    return '[REDACTED]';
  }
  return value;
}

export function makeRunId(date = new Date(), randomBytes = crypto.randomBytes(2)) {
  const stamp = date.toISOString().replace(/\D/g, '').slice(2, 14);
  return 'sd' + stamp + Buffer.from(randomBytes).toString('hex');
}

export function accountUsername(runId, group) {
  return runId + (group === 'VIP1' ? 'v' : 'd');
}

export function deriveAccountPassword(adminPassword, username) {
  const digest = crypto.createHmac('sha256', String(adminPassword)).update('seedance-test:' + username).digest('base64url');
  return 'T!' + digest.slice(0, 16);
}

function textContent(text) {
  return { type: 'text', text };
}

function imageContent(imageUrl) {
  return { type: 'image_url', role: 'reference_image', image_url: { url: imageUrl } };
}

function videoContent(videoUrl) {
  return { type: 'video_url', role: 'reference_video', video_url: { url: videoUrl } };
}

function audioContent(audioUrl) {
  return { type: 'audio_url', role: 'reference_audio', audio_url: { url: audioUrl } };
}

export function buildPositiveCases(options = {}) {
  const imageUrl = options.imageUrl || 'https://assets.invalid/character.jpg';
  const audioUrl = options.audioUrl || 'https://assets.invalid/reference.mp3';
  const referenceVideo = options.referenceVideoUrl || REFERENCE_VIDEO_PLACEHOLDER;
  const callbackUrl = options.callbackUrl || '';
  const serviceTier = options.serviceTier === undefined ? 'default' : options.serviceTier;

  const c07Body = {
    model: SEEDANCE_MODELS.standard,
    resolution: '720p',
    ratio: '9:16',
    content: [textContent(PROMPTS.station), imageContent(imageUrl)],
    duration: 5,
    seed: 20260717,
    metadata: {
      resolution: '1080p',
      ratio: '1:1',
      watermark: true,
      seed: 99,
    },
  };
  const c10Body = {
    model: SEEDANCE_MODELS.light,
    resolution: '720p',
    ratio: '3:4',
    content: [textContent(PROMPTS.mirror)],
    duration: 5,
    generate_audio: false,
  };

  const cases = [
    {
      id: 'C01',
      group: 'default',
      model: SEEDANCE_MODELS.light,
      resolution: '480p',
      ratio: '16:9',
      modalities: ['text'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.light,
        prompt: PROMPTS.kineticSculpture,
        resolution: '480p',
        ratio: '16:9',
        duration: 5,
        generate_audio: false,
        seed: 0,
        priority: 0,
        return_last_frame: true,
      },
      description: 'Bootstrap text-only video; prompt fallback and explicit zero values.',
    },
    {
      id: 'C02',
      group: 'VIP1',
      model: SEEDANCE_MODELS.vip,
      resolution: '480p',
      ratio: '9:16',
      modalities: ['text', 'image'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.vip,
        resolution: '480p',
        ratio: '9:16',
        content: [textContent(PROMPTS.transform), imageContent(imageUrl)],
        generate_audio: true,
        return_last_frame: true,
      },
      description: 'Image-to-video with catalog default duration and 480p no-discount assertion.',
    },
    {
      id: 'C03',
      group: 'default',
      model: SEEDANCE_MODELS.vip,
      resolution: '720p',
      ratio: '1:1',
      modalities: ['text', 'image', 'audio'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.vip,
        resolution: '720p',
        ratio: '1:1',
        content: [textContent(PROMPTS.rain), imageContent(imageUrl), audioContent(audioUrl)],
        duration: 5,
        seed: 314159,
        generate_audio: true,
      },
      description: 'Image and audio composition with explicit duration.',
    },
    {
      id: 'C04',
      group: 'VIP1',
      model: SEEDANCE_MODELS.vip,
      resolution: '1080p',
      ratio: '4:3',
      modalities: ['text', 'video'],
      referenceVideo: true,
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.vip,
        resolution: '1080p',
        ratio: '4:3',
        content: [textContent(PROMPTS.continuation), videoContent(referenceVideo)],
        duration: 5,
        seed: 271828,
      },
      description: 'Reference-video continuation and VIP1 pricing.',
    },
    {
      id: 'C05',
      group: 'default',
      model: SEEDANCE_MODELS.vip,
      resolution: '4k',
      ratio: '3:4',
      modalities: ['text'],
      expectedSeconds: 5,
      serviceTierProbe: Boolean(serviceTier),
      body: {
        model: SEEDANCE_MODELS.vip,
        prompt: PROMPTS.mirror,
        width: 2160,
        height: 2880,
        duration: 5,
        priority: 1,
        ...(serviceTier ? { service_tier: serviceTier } : {}),
      },
      description: '4K resolution and 3:4 ratio inferred from dimensions; service-tier probe.',
    },
    {
      id: 'C06',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '480p',
      ratio: '16:9',
      modalities: ['text'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.standard,
        prompt: PROMPTS.station,
        resolution: '480p',
        ratio: '16:9',
        duration: 5,
      },
      description: 'Prompt fallback with explicit duration.',
    },
    {
      id: 'C07',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      ratio: '9:16',
      modalities: ['text', 'image'],
      expectedSeconds: 5,
      body: structuredClone(c07Body),
      description: 'Root-field precedence over metadata and watermark fallback.',
    },
    {
      id: 'C08',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '1080p',
      ratio: '1:1',
      modalities: ['text', 'image', 'video', 'audio'],
      referenceVideo: true,
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.standard,
        resolution: '1080p',
        ratio: '1:1',
        content: [
          textContent(PROMPTS.rain),
          imageContent(imageUrl),
          videoContent(referenceVideo),
          audioContent(audioUrl),
        ],
        duration: 5,
        generate_audio: true,
        priority: 0,
      },
      description: 'Four-modality official content with audio generation.',
    },
    {
      id: 'C09',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '4k',
      ratio: '4:3',
      modalities: ['text', 'image'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.standard,
        resolution: '4k',
        ratio: '4:3',
        content: [textContent(PROMPTS.transform), imageContent(imageUrl)],
        duration: 5,
        return_last_frame: false,
        ...(callbackUrl ? { callback_url: callbackUrl } : {}),
      },
      description: '4K image-to-video with optional callback.',
    },
    {
      id: 'C10',
      group: 'VIP1',
      model: SEEDANCE_MODELS.light,
      resolution: '720p',
      ratio: '3:4',
      modalities: ['text'],
      expectedSeconds: 5,
      body: structuredClone(c10Body),
      description: 'Text-only generate_audio=false side of the audio toggle pair.',
    },
    {
      id: 'C11',
      group: 'default',
      model: SEEDANCE_MODELS.light,
      resolution: '1080p',
      ratio: '16:9',
      modalities: ['text', 'video'],
      referenceVideo: true,
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.light,
        resolution: '1080p',
        ratio: '16:9',
        content: [textContent(PROMPTS.continuation), videoContent(referenceVideo)],
        duration: 5,
      },
      description: 'Light 1080p reference-video continuation.',
    },
    {
      id: 'C12',
      group: 'VIP1',
      model: SEEDANCE_MODELS.value,
      resolution: '1080p',
      ratio: '9:16',
      modalities: ['text', 'image'],
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.value,
        resolution: '1080p',
        ratio: '9:16',
        content: [textContent(PROMPTS.rooftop), imageContent(imageUrl)],
        duration: 5,
        seed: 161803,
      },
      description: 'High-value 1080p identity and motion consistency.',
    },
    {
      id: 'C13',
      group: 'default',
      model: SEEDANCE_MODELS.value,
      resolution: '4k',
      ratio: '1:1',
      modalities: ['text', 'video', 'audio'],
      referenceVideo: true,
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.value,
        resolution: '4k',
        ratio: '1:1',
        content: [textContent(PROMPTS.rain), videoContent(referenceVideo), audioContent(audioUrl)],
        duration: 5,
        generate_audio: true,
      },
      description: 'High-value 4K reference video plus synchronized audio.',
    },
    {
      id: 'C14',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      ratio: '9:16',
      modalities: ['text', 'image'],
      expectedSeconds: 5,
      body: structuredClone(c07Body),
      description: 'Exact C07 payload under VIP1 for group-price comparison.',
    },
    {
      id: 'C15',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      ratio: '9:16',
      modalities: ['text', 'image', 'video'],
      referenceVideo: true,
      expectedSeconds: 5,
      body: {
        ...structuredClone(c07Body),
        content: [...c07Body.content, videoContent(referenceVideo)],
      },
      description: 'C07 reference-video counterpart for pricing-variant comparison.',
    },
    {
      id: 'C16',
      group: 'VIP1',
      model: SEEDANCE_MODELS.light,
      resolution: '720p',
      ratio: '3:4',
      modalities: ['text'],
      expectedSeconds: 5,
      body: {
        ...structuredClone(c10Body),
        generate_audio: true,
      },
      description: 'C10 duplicate with generate_audio=true.',
    },
  ];
  return cases;
}

export function buildNegativeCases(options = {}) {
  const audioUrl = options.audioUrl || 'https://assets.invalid/reference.mp3';
  return [
    {
      id: 'N01',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: { model: SEEDANCE_MODELS.standard, resolution: '720p' },
      description: 'Missing content.',
    },
    {
      id: 'N02',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: { model: SEEDANCE_MODELS.standard, resolution: '720p', content: 'invalid-content' },
      description: 'Malformed content type.',
    },
    {
      id: 'N03',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: { model: SEEDANCE_MODELS.standard, prompt: PROMPTS.rooftop, width: 1280 },
      description: 'Incomplete explicit dimensions.',
    },
    {
      id: 'N04',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: {
        model: SEEDANCE_MODELS.standard,
        prompt: PROMPTS.station,
        resolution: '720p',
        ratio: '2:1',
      },
      description: 'Unsupported ratio.',
    },
    {
      id: 'N05',
      group: 'default',
      model: SEEDANCE_MODELS.light,
      resolution: '4k',
      expectedNoCharge: true,
      body: {
        model: SEEDANCE_MODELS.light,
        prompt: PROMPTS.mirror,
        resolution: '4k',
        ratio: '16:9',
        duration: 5,
      },
      description: 'Unsupported Light 4K model-resolution combination.',
    },
    {
      id: 'N06',
      group: 'VIP1',
      model: SEEDANCE_MODELS.light,
      resolution: '720p',
      expectedNoCharge: true,
      providerProbe: true,
      expectedSeconds: 5,
      body: {
        model: SEEDANCE_MODELS.light,
        resolution: '720p',
        ratio: '16:9',
        content: [audioContent(audioUrl)],
        duration: 5,
      },
      description: 'Audio-only provider capability probe; acceptance requires rejection and zero net charge.',
    },
    {
      id: 'N07',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: {
        model: SEEDANCE_MODELS.standard,
        prompt: PROMPTS.rooftop,
        resolution: '720p',
        ratio: '16:9',
        frames: 120,
        fps: 24,
      },
      description: 'Official frames/fps capability probe; must be rejected before task creation and billing.',
    },
    {
      id: 'N08',
      group: 'VIP1',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: {
        model: SEEDANCE_MODELS.standard,
        prompt: PROMPTS.station,
        resolution: '720p',
        ratio: '16:9',
        frames: 120,
        frames_per_second: 24,
      },
      description: 'Official frames_per_second capability probe; must be rejected before task creation and billing.',
    },
    {
      id: 'N09',
      group: 'default',
      model: SEEDANCE_MODELS.standard,
      resolution: '720p',
      expectedNoCharge: true,
      body: {
        model: SEEDANCE_MODELS.standard,
        prompt: PROMPTS.mirror,
        resolution: '720p',
        ratio: '1:1',
        frames: 120,
        framespersecond: 24,
      },
      description: 'Official framespersecond capability probe; must be rejected before task creation and billing.',
    },
  ];
}

export function materializeCase(testCase, referenceVideoUrl) {
  const replace = (value) => {
    if (Array.isArray(value)) {
      return value.map(replace);
    }
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value).map(([key, nested]) => [key, replace(nested)]));
    }
    return value === REFERENCE_VIDEO_PLACEHOLDER ? referenceVideoUrl : value;
  };
  return { ...testCase, body: replace(testCase.body) };
}

export function validateCoverage(cases) {
  const expectedSlots = new Set([
    SEEDANCE_MODELS.vip + '|480p',
    SEEDANCE_MODELS.vip + '|720p',
    SEEDANCE_MODELS.vip + '|1080p',
    SEEDANCE_MODELS.vip + '|4k',
    SEEDANCE_MODELS.standard + '|480p',
    SEEDANCE_MODELS.standard + '|720p',
    SEEDANCE_MODELS.standard + '|1080p',
    SEEDANCE_MODELS.standard + '|4k',
    SEEDANCE_MODELS.light + '|480p',
    SEEDANCE_MODELS.light + '|720p',
    SEEDANCE_MODELS.light + '|1080p',
    SEEDANCE_MODELS.value + '|1080p',
    SEEDANCE_MODELS.value + '|4k',
  ]);
  const actualSlots = new Set(cases.slice(0, 13).map((item) => item.model + '|' + item.resolution));
  const missing = [...expectedSlots].filter((slot) => !actualSlots.has(slot));
  return {
    ok: cases.length === 16 && missing.length === 0,
    expectedSlots: [...expectedSlots],
    actualSlots: [...actualSlots],
    missing,
  };
}

function effectiveTier(config, resolution) {
  if (!config || typeof config !== 'object') {
    throw new Error('Task pricing is missing.');
  }
  if (config.by_resolution && config.by_resolution[resolution]) {
    return config.by_resolution[resolution];
  }
  return config;
}

export function computeQuote(pricingEntry, groupRatio, testCase, quotaPerUnit) {
  if (!pricingEntry || pricingEntry.billing_mode !== 'task_pricing') {
    throw new Error('Model is not configured for task_pricing: ' + testCase.model);
  }
  const config = pricingEntry.task_pricing;
  const tier = effectiveTier(config, testCase.resolution);
  const hasReferenceVideo = Boolean(testCase.referenceVideo);
  let unitPrice = Number(tier.no_reference_video_unit_price);
  let variant = 'no_reference_video';
  if (hasReferenceVideo) {
    variant = 'reference_video';
    const policy = tier.reference_video_policy || config.reference_video_policy;
    if (policy === 'custom') {
      unitPrice = Number(tier.reference_video_unit_price || config.reference_video_unit_price);
    } else if (policy === 'disabled') {
      throw new Error('Reference video is disabled for ' + testCase.model + ' ' + testCase.resolution);
    }
  }
  if (!(unitPrice > 0)) {
    throw new Error('Invalid unit price for ' + testCase.model + ' ' + testCase.resolution);
  }
  const configuredPolicy = tier.group_ratio_policy || config.group_ratio_policy || '';
  const ratioPolicy = configuredPolicy || (testCase.resolution === '480p' ? 'none' : 'global');
  const appliedRatio = ratioPolicy === 'none' ? 1 : Number(groupRatio || 1);
  const quantity = Number(testCase.expectedSeconds || 5);
  const baseUsd = unitPrice * quantity;
  const saleUsd = baseUsd * appliedRatio;
  return {
    unit: config.unit || 'second',
    variant,
    unitPriceUsd: unitPrice,
    quantity,
    groupRatio: appliedRatio,
    groupRatioPolicy: ratioPolicy,
    baseUsd,
    saleUsd,
    quota: Math.round(saleUsd * quotaPerUnit),
    hasReferenceVideo,
    resolution: testCase.resolution,
  };
}

export function findPricingEntry(pricingResponse, modelName) {
  const entries = Array.isArray(pricingResponse?.data) ? pricingResponse.data : [];
  return entries.find((item) => item.model_name === modelName);
}

export function groupRatioFor(pricingResponse, group) {
  const value = pricingResponse?.group_ratio?.[group];
  return Number.isFinite(Number(value)) ? Number(value) : 1;
}

export function buildCostPlan(cases, pricingByGroup, quotaPerUnit, maxCostUsd, allGroups = []) {
  const planned = cases.map((testCase) => {
    const pricing = pricingByGroup[testCase.group];
    const entry = findPricingEntry(pricing, testCase.model);
    const quote = computeQuote(entry, groupRatioFor(pricing, testCase.group), testCase, quotaPerUnit);
    return { ...testCase, quote };
  });
  const totalUsd = planned.reduce((sum, item) => sum + item.quote.saleUsd, 0);
  if (totalUsd > maxCostUsd + 1e-9) {
    throw new Error(
      'Projected positive-case cost $' + totalUsd.toFixed(6) +
      ' exceeds hard cap $' + Number(maxCostUsd).toFixed(2) + '.',
    );
  }
  const totalQuota = Math.round(maxCostUsd * quotaPerUnit);
  const byGroupUsd = {};
  for (const item of planned) {
    byGroupUsd[item.group] = (byGroupUsd[item.group] || 0) + item.quote.saleUsd;
  }
  const groups = Object.keys(byGroupUsd);
  const quotaCaps = {};
  let assigned = 0;
  groups.forEach((group, index) => {
    if (index === groups.length - 1) {
      quotaCaps[group] = totalQuota - assigned;
      return;
    }
    const value = Math.floor(totalQuota * (byGroupUsd[group] / totalUsd));
    quotaCaps[group] = value;
    assigned += value;
  });
  for (const group of allGroups) {
    quotaCaps[group] ??= 0;
    byGroupUsd[group] ??= 0;
  }
  return { planned, totalUsd, totalQuota, byGroupUsd, quotaCaps, maxCostUsd };
}

export function dedupeAccounts(accounts) {
  const byUsername = new Map();
  for (const account of accounts || []) {
    const key = account?.username || account?.id;
    if (key === undefined || key === null) {
      continue;
    }
    const existing = byUsername.get(key);
    const hasVerifiedCleanup = typeof account.tokenDeleted === 'boolean';
    const existingHasVerifiedCleanup = typeof existing?.tokenDeleted === 'boolean';
    if (!existing || (hasVerifiedCleanup && !existingHasVerifiedCleanup)) {
      byUsername.set(key, account);
    }
  }
  return [...byUsername.values()];
}

export class HttpSession {
  constructor(baseUrl, options = {}) {
    this.baseUrl = stripTrailingSlash(baseUrl);
    this.userId = options.userId || null;
    this.cookie = options.cookie || '';
    this.fetchImpl = options.fetchImpl || fetch;
  }

  async request(method, requestPath, options = {}) {
    const headers = new Headers(options.headers || {});
    if (this.cookie) {
      headers.set('Cookie', this.cookie);
    }
    if (this.userId) {
      headers.set('New-Api-User', String(this.userId));
    }
    if (options.token) {
      headers.set('Authorization', 'Bearer ' + options.token);
    }
    let body;
    if (options.body !== undefined) {
      headers.set('Content-Type', 'application/json');
      body = JSON.stringify(options.body);
    }
    const started = Date.now();
    const response = await this.fetchImpl(this.baseUrl + requestPath, {
      method,
      headers,
      body,
      signal: options.signal,
    });
    const setCookies = typeof response.headers.getSetCookie === 'function'
      ? response.headers.getSetCookie()
      : [response.headers.get('set-cookie')].filter(Boolean);
    if (setCookies.length > 0) {
      this.cookie = setCookies.map((item) => item.split(';', 1)[0]).join('; ');
    }
    const raw = await response.text();
    let data = null;
    if (raw) {
      try {
        data = JSON.parse(raw);
      } catch {
        data = { raw };
      }
    }
    return {
      ok: response.ok && data?.success !== false,
      httpOk: response.ok,
      status: response.status,
      headers: Object.fromEntries(response.headers.entries()),
      requestId: response.headers.get('x-oneapi-request-id') || '',
      data,
      raw,
      elapsedMs: Date.now() - started,
    };
  }
}

export async function apiSuccess(session, method, requestPath, options = {}) {
  const response = await session.request(method, requestPath, options);
  if (!response.ok) {
    const message = response.data?.message || response.data?.error?.message || response.raw || 'unknown error';
    throw new Error(method + ' ' + requestPath + ' failed (HTTP ' + response.status + '): ' + message);
  }
  return response;
}

export async function login(baseUrl, username, password, fetchImpl = fetch) {
  const session = new HttpSession(baseUrl, { fetchImpl });
  const response = await apiSuccess(session, 'POST', '/api/user/login', {
    body: { username, password },
  });
  const user = response.data?.data;
  if (!user?.id) {
    throw new Error('Login succeeded but no user id was returned.');
  }
  session.userId = user.id;
  return { session, user };
}

export async function preflightAsset(url, expectedPrefix, fetchImpl = fetch) {
  if (!url) {
    throw new Error('Missing required ' + expectedPrefix + ' asset URL.');
  }
  const response = await fetchImpl(url, {
    method: 'GET',
    headers: { Range: 'bytes=0-1023' },
  });
  if (!response.ok) {
    throw new Error('Asset preflight failed for ' + redactUrl(url) + ' with HTTP ' + response.status + '.');
  }
  const contentType = String(response.headers.get('content-type') || '').toLowerCase();
  if (!contentType.startsWith(expectedPrefix + '/')) {
    throw new Error(
      'Asset ' + redactUrl(url) + ' returned ' + (contentType || 'unknown MIME') +
      ', expected ' + expectedPrefix + '/*.',
    );
  }
  const reader = response.body?.getReader();
  let received = 0;
  if (reader) {
    while (received < 1024) {
      const chunk = await reader.read();
      if (chunk.done) {
        break;
      }
      received += chunk.value.byteLength;
    }
    await reader.cancel().catch(() => {});
  } else {
    received = (await response.arrayBuffer()).byteLength;
  }
  return {
    url: redactUrl(url),
    status: response.status,
    contentType,
    contentLength: Number(response.headers.get('content-length') || 0),
    contentRange: response.headers.get('content-range') || '',
    sampleBytes: received,
  };
}

export function getTaskId(value) {
  return value?.id || value?.task_id || value?.data?.id || value?.data?.task_id || '';
}

export function getTaskStatus(value) {
  return value?.status || value?.data?.status || value?.data?.data?.status || '';
}

export function getOutputUrls(value) {
  const values = [
    value?.result_url,
    value?.url,
    ...(Array.isArray(value?.output) ? value.output : []),
    ...(Array.isArray(value?.metadata?.urls) ? value.metadata.urls : []),
    value?.data?.result_url,
    value?.data?.url,
    ...(Array.isArray(value?.data?.output) ? value.data.output : []),
    ...(Array.isArray(value?.data?.metadata?.urls) ? value.data.metadata.urls : []),
    value?.data?.content?.video_url,
    value?.data?.content?.last_frame_url,
  ];
  return [...new Set(values.filter((item) => typeof item === 'string' && item.trim()))];
}

export function isTerminalStatus(status) {
  return ['success', 'succeeded', 'completed', 'failure', 'failed', 'cancelled', 'canceled', 'error']
    .includes(String(status || '').toLowerCase());
}

export function isSuccessStatus(status) {
  return ['success', 'succeeded', 'completed'].includes(String(status || '').toLowerCase());
}

export async function pollVideoTask(options) {
  const started = Date.now();
  let lastResponse = null;
  while (Date.now() - started < options.timeoutSeconds * 1000) {
    if (options.shouldStop?.()) {
      throw new Error('Interrupted while polling task ' + options.taskId + '.');
    }
    await sleep(options.pollIntervalSeconds * 1000);
    const response = await options.session.request(
      'GET',
      '/v1/videos/' + encodeURIComponent(options.taskId),
      { token: options.token },
    );
    lastResponse = response;
    if (!response.httpOk) {
      if (response.status >= 500 || response.status === 429) {
        continue;
      }
      throw new Error(
        'Task poll failed for ' + options.taskId + ' with HTTP ' + response.status + ': ' +
        (response.data?.error?.message || response.raw),
      );
    }
    const status = getTaskStatus(response.data);
    options.onPoll?.({ status, response });
    if (isTerminalStatus(status)) {
      return response;
    }
  }
  throw new Error(
    'Task ' + options.taskId + ' timed out after ' + options.timeoutSeconds +
    ' seconds. Last response: ' + JSON.stringify(sanitizeDeep(lastResponse?.data)),
  );
}

export function pageItems(response) {
  const data = response?.data?.data ?? response?.data;
  if (Array.isArray(data)) {
    return data;
  }
  if (Array.isArray(data?.items)) {
    return data.items;
  }
  return [];
}

export function parseLogOther(log) {
  if (!log) {
    return {};
  }
  if (log.other && typeof log.other === 'object') {
    return log.other;
  }
  try {
    return JSON.parse(log.other || '{}');
  } catch {
    return {};
  }
}

export function sumNetLogQuota(logs) {
  return logs.reduce((sum, log) => {
    const quota = Number(log.quota || 0);
    return sum + (Number(log.type) === 6 ? -quota : Number(log.type) === 2 ? quota : 0);
  }, 0);
}

export async function downloadArtifact(url, outputPath, fetchImpl = fetch) {
  const response = await fetchImpl(url);
  if (!response.ok || !response.body) {
    throw new Error('Download failed for ' + redactUrl(url) + ' with HTTP ' + response.status + '.');
  }
  await fs.mkdir(path.dirname(outputPath), { recursive: true });
  const handle = await fs.open(outputPath, 'w');
  const hash = crypto.createHash('sha256');
  const reader = response.body.getReader();
  let size = 0;
  try {
    while (true) {
      const chunk = await reader.read();
      if (chunk.done) {
        break;
      }
      hash.update(chunk.value);
      size += chunk.value.byteLength;
      await handle.write(chunk.value);
    }
  } finally {
    await handle.close();
  }
  return {
    path: outputPath,
    size,
    sha256: hash.digest('hex'),
    contentType: response.headers.get('content-type') || '',
  };
}

async function runProgram(command, args) {
  return await new Promise((resolve) => {
    const child = spawn(command, args, { windowsHide: true });
    const stdout = [];
    const stderr = [];
    child.stdout.on('data', (chunk) => stdout.push(chunk));
    child.stderr.on('data', (chunk) => stderr.push(chunk));
    child.on('error', (error) => resolve({ ok: false, error: error.message, stdout: '', stderr: '' }));
    child.on('close', (code) => resolve({
      ok: code === 0,
      code,
      stdout: Buffer.concat(stdout).toString('utf8'),
      stderr: Buffer.concat(stderr).toString('utf8'),
    }));
  });
}

export async function inspectMedia(filePath, contactSheetPath) {
  const probe = await runProgram('ffprobe', [
    '-v', 'error',
    '-show_entries', 'format=duration,size,format_name:stream=index,codec_type,codec_name,width,height,r_frame_rate',
    '-of', 'json',
    filePath,
  ]);
  let metadata = { available: false, error: probe.error || probe.stderr.trim() || 'ffprobe unavailable' };
  if (probe.ok) {
    try {
      metadata = { available: true, ...JSON.parse(probe.stdout) };
    } catch (error) {
      metadata = { available: false, error: error.message };
    }
  }
  let contactSheet = null;
  if (contactSheetPath && metadata.available) {
    const render = await runProgram('ffmpeg', [
      '-y', '-i', filePath,
      '-vf', 'fps=1/2,scale=320:-1,tile=3x2',
      '-frames:v', '1',
      contactSheetPath,
    ]);
    if (render.ok) {
      contactSheet = contactSheetPath;
    }
  }
  return { metadata, contactSheet };
}

function csvCell(value) {
  const text = value === undefined || value === null
    ? ''
    : typeof value === 'object'
      ? JSON.stringify(value)
      : String(value);
  return '"' + text.replace(/"/g, '""') + '"';
}

export function renderBillingCsv(run) {
  const money = reportMoney(run);
  const headers = [
    'case_id', 'group', 'model', 'resolution', 'status', 'request_id', 'task_id',
    'pricing_variant', 'unit_price_usd', 'quantity', 'expected_group_ratio',
    'expected_sale_usd', 'log_sale_usd', 'task_quota', 'task_quota_usd',
    'balance_delta_quota', 'balance_delta_usd', 'net_log_quota',
    'net_log_usd', 'discrepancy_usd', 'usd_cny_rate', 'unit_price_cny',
    'expected_sale_cny', 'log_sale_cny', 'task_quota_cny', 'balance_delta_cny',
    'net_log_cny', 'discrepancy_cny', 'pass',
  ];
  const rows = [headers.map(csvCell).join(',')];
  for (const result of run.results || []) {
    const billing = result.billing || {};
    rows.push([
      result.id,
      result.group,
      result.model,
      result.resolution,
      result.finalStatus || result.outcome || '',
      result.requestId || '',
      result.taskId || '',
      billing.expected?.variant || '',
      billing.expected?.unitPriceUsd ?? '',
      billing.expected?.quantity ?? '',
      billing.expected?.groupRatio ?? '',
      billing.expected?.saleUsd ?? '',
      billing.log?.saleUsd ?? '',
      billing.taskQuota ?? '',
      billing.taskQuotaUsd ?? '',
      billing.balanceDeltaQuota ?? '',
      billing.balanceDeltaUsd ?? '',
      billing.netLogQuota ?? '',
      billing.netLogUsd ?? '',
      billing.discrepancyUsd ?? '',
      money.code === 'CNY' ? money.rate : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.expected?.unitPriceUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.expected?.saleUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.log?.saleUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.taskQuotaUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.balanceDeltaUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.netLogUsd, money) : '',
      money.code === 'CNY' ? usdToReportCurrency(billing.discrepancyUsd, money) : '',
      result.pass,
    ].map(csvCell).join(','));
  }
  return rows.join('\n') + '\n';
}

export function renderExternalCostCsv(run) {
  const rows = [[
    'cost_id', 'scope', 'task_id', 'currency', 'amount', 'awcoin', 'description',
  ].map(csvCell).join(',')];
  for (const cost of run.externalCosts || []) {
    rows.push([
      cost.id,
      cost.scope,
      cost.taskId,
      cost.currency,
      cost.amount,
      cost.awcoin,
      cost.description,
    ].map(csvCell).join(','));
  }
  return rows.join('\n') + '\n';
}

function escapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function reportMoney(run) {
  const rate = Number(run.reporting?.usdCnyRate);
  if (run.reporting?.currency === 'CNY' && Number.isFinite(rate) && rate > 0) {
    return {
      code: 'CNY',
      label: '人民币',
      symbol: '¥',
      rate,
      rateDate: run.reporting?.usdCnyRateDate || '',
      rateSource: run.reporting?.usdCnyRateSource || '',
    };
  }
  return { code: 'USD', label: '美元', symbol: '$', rate: 1, rateDate: '', rateSource: '' };
}

function usdToReportCurrency(value, money) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  const number = Number(value);
  return Number.isFinite(number) ? number * money.rate : '';
}

function fmtReportMoney(value, money) {
  const converted = usdToReportCurrency(value, money);
  return converted === '' ? '-' : money.symbol + converted.toFixed(6);
}

function convertUsdText(value, money) {
  if (money.code !== 'CNY') {
    return String(value ?? '');
  }
  return String(value ?? '').replace(/\$(-?\d+(?:\.\d+)?)/g, (_, amount) =>
    fmtReportMoney(Number(amount), money));
}

function externalCostInReportCurrency(run, money) {
  return (run.externalCosts || []).reduce((sum, cost) => {
    if (cost.currency === money.code && Number.isFinite(Number(cost.amount))) {
      return sum + Number(cost.amount);
    }
    if (cost.currency === 'USD' && money.code === 'CNY' && Number.isFinite(Number(cost.amount))) {
      return sum + Number(cost.amount) * money.rate;
    }
    return sum;
  }, 0);
}

function zhStatus(value) {
  const normalized = String(value || '').toLowerCase();
  const labels = {
    success: '成功',
    succeeded: '成功',
    completed: '完成',
    failure: '失败',
    failed: '失败',
    cancelled: '已取消',
    canceled: '已取消',
    error: '错误',
    queued: '排队中',
    running: '生成中',
    in_progress: '处理中',
    create_failed: '创建失败',
    completed_with_failures: '完成但检查未通过',
    rejected_as_expected: '按预期拒绝',
  };
  return labels[normalized] || value || '-';
}

function zhModalities(values) {
  const labels = {
    text: '文本',
    image: '图片',
    video: '视频',
    audio: '音频',
    'validation-only': '仅参数校验',
  };
  const parts = Array.isArray(values) ? values : String(values || '').split('+');
  return parts.filter(Boolean).map((item) => labels[item] || item).join('+') || '-';
}

function zhSummaryKey(value) {
  return String(value || '').includes('+') || value === 'text' || value === 'validation-only'
    ? zhModalities(value)
    : value;
}

function zhVariant(value) {
  return {
    no_reference_video: '无参考视频',
    reference_video: '有参考视频',
  }[value] || value || '-';
}

export function renderMarkdownReport(run) {
  const lines = [];
  const money = reportMoney(run);
  const externalCost = externalCostInReportCurrency(run, money);
  const combinedActual = usdToReportCurrency(run.summary?.actualBalanceUsd, money);
  lines.push('# Seedance 双分组全量测试与费用审计');
  lines.push('');
  lines.push('- 运行编号：' + run.runId);
  lines.push('- 接口地址：' + redactUrl(run.baseUrl));
  lines.push('- 开始时间：' + run.startedAt);
  lines.push('- 结束时间：' + (run.finishedAt || '-'));
  lines.push('- 价格版本：' + (run.environment?.pricingVersion || '-'));
  lines.push('- 目录版本：' + (run.environment?.catalogRevision || '未提供'));
  lines.push('- 费用硬上限：' + fmtReportMoney(run.costPlan?.maxCostUsd, money));
  lines.push('- 正向用例预计费用：' + fmtReportMoney(run.costPlan?.totalUsd, money));
  lines.push('- 平台余额实际支出：' + fmtReportMoney(run.summary?.actualBalanceUsd, money));
  if (money.code === 'CNY') {
    lines.push('- 人民币换算：1 美元 = ' + money.rate + ' 元人民币' +
      (money.rateDate ? '（' + money.rateDate + '）' : '') +
      (money.rateSource ? '，来源：' + money.rateSource : ''));
    lines.push('- 含上游辅助费用的实际总支出：' + money.symbol +
      (Number(combinedActual || 0) + externalCost).toFixed(6));
  }
  lines.push('- 最终结果：' + (run.summary?.pass ? '通过' : '失败'));
  lines.push('');
  lines.push('## 测试账号');
  lines.push('');
  lines.push('| 用户名 | 分组 | 角色 | 初始额度 | 结束额度 | 令牌已删除 | 账号已停用 |');
  lines.push('|---|---:|---:|---:|---:|---:|---:|');
  for (const account of run.accounts || []) {
    lines.push(
      '| ' + account.username + ' | ' + account.group + ' | ' + account.role + ' | ' +
      (account.startQuota ?? '-') + ' | ' + (account.endQuota ?? '-') + ' | ' +
      (account.tokenDeleted ? '是' : '否') + ' | ' + (account.disabled ? '是' : '否') + ' |',
    );
  }
  lines.push('');
  lines.push('## 用例结果');
  lines.push('');
  lines.push('| 用例 | 分组 | 模型 | 分辨率 | 输入模态 | 状态 | 请求 ID | 任务 ID | 提交耗时 ms | 轮询耗时 ms | 通过 |');
  lines.push('|---|---|---|---|---|---|---|---|---:|---:|---:|');
  for (const result of run.results || []) {
    lines.push(
      '| ' + result.id + ' | ' + result.group + ' | ' + result.model + ' | ' + result.resolution +
      ' | ' + zhModalities(result.modalities || []) + ' | ' + zhStatus(result.finalStatus || result.outcome) +
      ' | ' + (result.requestId || '-') + ' | ' + (result.taskId || '-') + ' | ' +
      (result.submitLatencyMs ?? '-') + ' | ' + (result.pollLatencyMs ?? '-') + ' | ' +
      (result.pass ? '是' : '否') + ' |',
    );
  }
  lines.push('');
  lines.push('## 费用明细');
  lines.push('');
  lines.push('| 用例 | 计价档位 | 单价（' + money.label + '） | 数量 | 分组倍率 | 预计（' + money.label + '） | 日志（' + money.label + '） | 任务额度折算（' + money.label + '） | 余额差（' + money.label + '） | 净日志（' + money.label + '） | 误差（' + money.label + '） |');
  lines.push('|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|');
  for (const result of run.results || []) {
    const billing = result.billing || {};
    lines.push(
      '| ' + result.id + ' | ' + zhVariant(billing.expected?.variant || billing.log?.variant) +
      ' | ' + fmtReportMoney(billing.expected?.unitPriceUsd, money) + ' | ' +
      finiteNumber(billing.expected?.quantity) + ' | ' +
      finiteNumber(billing.expected?.groupRatio) + ' | ' +
      fmtReportMoney(billing.expected?.saleUsd, money) + ' | ' + fmtReportMoney(billing.log?.saleUsd, money) + ' | ' +
      fmtReportMoney(billing.taskQuotaUsd, money) + ' | ' + fmtReportMoney(billing.balanceDeltaUsd, money) + ' | ' +
      fmtReportMoney(billing.netLogUsd, money) + ' | ' + fmtReportMoney(billing.discrepancyUsd, money) + ' |',
    );
  }
  lines.push('');
  if ((run.externalCosts || []).length > 0) {
    lines.push('## 上游辅助费用');
    lines.push('');
    lines.push('| 费用编号 | 范围 | 任务 ID | 币种 | 金额 | AWCoin | 说明 |');
    lines.push('|---|---|---|---|---:|---:|---|');
    for (const cost of run.externalCosts) {
      lines.push(
        '| ' + cost.id + ' | ' + cost.scope + ' | ' + cost.taskId + ' | ' +
        cost.currency + ' | ' + finiteNumber(cost.amount) + ' | ' +
        finiteNumber(cost.awcoin) + ' | ' + cost.description + ' |',
      );
    }
    lines.push('');
  }
  lines.push('## 费用汇总');
  lines.push('');
  appendMarkdownSummary(lines, '按分组', run.summary?.billingByGroup, money);
  appendMarkdownSummary(lines, '按模型', run.summary?.billingByModel, money);
  appendMarkdownSummary(lines, '按分辨率', run.summary?.billingByResolution, money);
  appendMarkdownSummary(lines, '按输入模态', run.summary?.billingByModality, money);
  lines.push('## 费用断言');
  lines.push('');
  for (const assertion of run.assertions || []) {
    lines.push('- ' + (assertion.pass ? '通过' : '失败') + ' — ' + assertion.name + '：' +
      convertUsdText(assertion.detail, money));
  }
  lines.push('');
  lines.push('## 请求、产物与质量复核');
  lines.push('');
  lines.push('自动检查覆盖 HTTP 行为、任务状态、媒体下载、元数据和计费。人工复核建议关注人物一致性、运动因果、手部与物体永久性、遮挡恢复、镜面同步、音画同步、闪烁和明显结构错误。');
  lines.push('');
  for (const result of run.results || []) {
    lines.push('### ' + result.id);
    lines.push('');
    lines.push('- 测试提示词（原文）：' + String(result.prompt || '').replace(/\s+/g, ' ').trim());
    lines.push('- 请求 ID：' + (result.requestId || '-'));
    lines.push('- 任务 ID：' + (result.taskId || '-'));
    lines.push('- 已脱敏请求：');
    for (const requestLine of JSON.stringify(result.request || {}, null, 2).split('\n')) {
      lines.push('    ' + requestLine);
    }
    for (const artifact of (result.artifacts || []).filter((item) => item.path)) {
      lines.push('- 产物：[' + path.basename(artifact.path) + '](' + artifact.relativePath.replace(/\\/g, '/') + ')');
      lines.push('- 媒体元数据：' + JSON.stringify(artifact.media || {}));
      if (artifact.contactSheetRelative) {
        lines.push('- 联系表：[' + path.basename(artifact.contactSheetRelative) + '](' + artifact.contactSheetRelative.replace(/\\/g, '/') + ')');
      }
    }
    lines.push('');
  }
  lines.push('## 安全说明');
  lines.push('');
  lines.push('报告不包含密码、API 令牌、Cookie 或带签名 URL 的查询参数。results.json 位于已忽略的 .test 目录，所有 URL 均已脱敏。');
  lines.push('');
  return lines.join('\n');
}

export function renderHtmlReport(run) {
  const money = reportMoney(run);
  const externalCost = externalCostInReportCurrency(run, money);
  const combinedActual = Number(usdToReportCurrency(run.summary?.actualBalanceUsd, money) || 0) + externalCost;
  const rows = (run.results || []).map((result) => {
    const expected = result.billing?.expected?.saleUsd;
    const actual = result.billing?.balanceDeltaUsd;
    const artifact = result.artifacts?.find((item) => item.path);
    const media = artifact
      ? '<video controls preload="metadata" src="' + escapeHtml(artifact.relativePath.replace(/\\/g, '/')) + '"></video>'
      : '';
    const request = '<details><summary>查看请求</summary><pre>' +
      escapeHtml(JSON.stringify(result.request || {}, null, 2)) + '</pre></details>';
    return '<tr><td>' + escapeHtml(result.id) + '</td><td>' + escapeHtml(result.group) +
      '</td><td>' + escapeHtml(result.model) + '</td><td>' + escapeHtml(result.resolution) +
      '</td><td>' + escapeHtml(zhModalities(result.modalities || [])) + '</td><td>' +
      escapeHtml(zhStatus(result.finalStatus || result.outcome)) + '</td><td>' +
      escapeHtml(fmtReportMoney(expected, money)) + '</td><td>' + escapeHtml(fmtReportMoney(actual, money)) +
      '</td><td class="' + (result.pass ? 'pass' : 'fail') + '">' +
      (result.pass ? '通过' : '失败') + '</td><td><small>' +
      escapeHtml(result.requestId || '-') + '<br>' + escapeHtml(result.taskId || '-') +
      '<br>提交=' + escapeHtml(result.submitLatencyMs ?? '-') + 'ms；轮询=' +
      escapeHtml(result.pollLatencyMs ?? '-') + 'ms</small>' + request + '</td><td>' + media + '</td></tr>';
  }).join('');
  const billingRows = (run.results || []).map((result) => {
    const billing = result.billing || {};
    return '<tr><td>' + escapeHtml(result.id) + '</td><td>' +
      escapeHtml(zhVariant(billing.expected?.variant || billing.log?.variant)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.expected?.unitPriceUsd, money)) + '</td><td>' +
      escapeHtml(finiteNumber(billing.expected?.quantity)) + '</td><td>' +
      escapeHtml(finiteNumber(billing.expected?.groupRatio)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.expected?.saleUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.log?.saleUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.taskQuotaUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.balanceDeltaUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.netLogUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(billing.discrepancyUsd, money)) + '</td></tr>';
  }).join('');
  const assertions = (run.assertions || []).map((item) =>
    '<li class="' + (item.pass ? 'pass' : 'fail') + '">' +
    escapeHtml((item.pass ? '通过 — ' : '失败 — ') + item.name + '：' +
      convertUsdText(item.detail, money)) + '</li>',
  ).join('');
  const externalCostRows = (run.externalCosts || []).map((cost) =>
    '<tr><td>' + escapeHtml(cost.id) + '</td><td>' + escapeHtml(cost.scope) +
    '</td><td>' + escapeHtml(cost.taskId) + '</td><td>' + escapeHtml(cost.currency) +
    '</td><td>' + escapeHtml(finiteNumber(cost.amount)) + '</td><td>' +
    escapeHtml(finiteNumber(cost.awcoin)) + '</td><td>' + escapeHtml(cost.description) + '</td></tr>',
  ).join('');
  return [
    '<!doctype html>',
    '<html lang="zh-CN"><head><meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width,initial-scale=1">',
    '<title>Seedance 全量测试 ' + escapeHtml(run.runId) + '</title>',
    '<style>',
    'body{font-family:Inter,Segoe UI,sans-serif;margin:32px;background:#0b1020;color:#e8ecf5}',
    'h1,h2{letter-spacing:-.02em} .summary{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:12px}',
    '.card{background:#151d32;border:1px solid #27324d;border-radius:12px;padding:14px}',
    'table{width:100%;border-collapse:collapse;background:#11192b}th,td{padding:9px;border:1px solid #2a3653;vertical-align:top}',
    'th{position:sticky;top:0;background:#1c2740}.pass{color:#66e3a4}.fail{color:#ff7b86}video{width:220px;max-height:180px}',
    'code,pre{word-break:break-all;white-space:pre-wrap}pre{max-width:520px}a{color:#8ab4ff}</style></head><body>',
    '<h1>Seedance 双分组全量测试与费用审计</h1>',
    '<div class="summary">',
    '<div class="card">运行编号<br><strong>' + escapeHtml(run.runId) + '</strong></div>',
    '<div class="card">预计费用<br><strong>' + escapeHtml(fmtReportMoney(run.costPlan?.totalUsd, money)) + '</strong></div>',
    '<div class="card">实际支出<br><strong>' + escapeHtml(fmtReportMoney(run.summary?.actualBalanceUsd, money)) + '</strong></div>',
    '<div class="card">费用上限<br><strong>' + escapeHtml(fmtReportMoney(run.costPlan?.maxCostUsd, money)) + '</strong></div>',
    money.code === 'CNY' ? '<div class="card">含辅助费用总支出<br><strong>' +
      escapeHtml(money.symbol + combinedActual.toFixed(6)) + '</strong></div>' : '',
    '<div class="card">最终结果<br><strong class="' + (run.summary?.pass ? 'pass' : 'fail') + '">' +
      (run.summary?.pass ? '通过' : '失败') + '</strong></div>',
    '</div>',
    '<h2>测试环境</h2>',
    '<p>接口地址：<code>' + escapeHtml(redactUrl(run.baseUrl)) + '</code><br>价格版本：<code>' +
      escapeHtml(run.environment?.pricingVersion || '-') + '</code><br>目录版本：<code>' +
      escapeHtml(run.environment?.catalogRevision || '未提供') + '</code>' +
      (money.code === 'CNY' ? '<br>人民币换算：<code>1 美元 = ' + escapeHtml(money.rate) +
        ' 元人民币' + (money.rateDate ? '（' + escapeHtml(money.rateDate) + '）' : '') + '</code>' +
        (money.rateSource ? '，来源：' + escapeHtml(money.rateSource) : '') : '') + '</p>',
    '<h2>用例与计费</h2>',
    '<div style="overflow:auto"><table><thead><tr><th>用例</th><th>分组</th><th>模型</th><th>分辨率</th><th>输入模态</th><th>状态</th><th>预计费用</th><th>实际费用</th><th>结论</th><th>ID / 请求</th><th>产物</th></tr></thead><tbody>',
    rows,
    '</tbody></table></div>',
    '<h2>费用明细</h2>',
    '<div style="overflow:auto"><table><thead><tr><th>用例</th><th>计价档位</th><th>单价（' + money.label + '）</th><th>数量</th><th>分组倍率</th><th>预计（' + money.label + '）</th><th>日志（' + money.label + '）</th><th>任务额度折算（' + money.label + '）</th><th>余额差（' + money.label + '）</th><th>净日志（' + money.label + '）</th><th>误差</th></tr></thead><tbody>',
    billingRows,
    '</tbody></table></div>',
    externalCostRows ? '<h2>上游辅助费用</h2><div style="overflow:auto"><table><thead><tr><th>费用编号</th><th>范围</th><th>任务 ID</th><th>币种</th><th>金额</th><th>AWCoin</th><th>说明</th></tr></thead><tbody>' + externalCostRows + '</tbody></table></div>' : '',
    '<h2>费用汇总</h2>',
    renderHtmlSummaries(run.summary || {}, money),
    '<h2>断言结果</h2><ul>' + assertions + '</ul>',
    '<h2>质量复核标准</h2>',
    '<p>人工检查：人物身份一致性、因果运动、手与物体永久性、遮挡恢复、镜面同步、音画同步、闪烁和明显结构错误。</p>',
    '<h2>安全说明</h2><p>报告不包含管理员密码、用户密码、令牌、Cookie 或带签名 URL 的查询参数。</p>',
    '</body></html>',
  ].join('');
}

function finiteNumber(value) {
  if (value === undefined || value === null || value === '') {
    return '-';
  }
  return Number.isFinite(Number(value)) ? String(Number(value)) : '-';
}

function appendMarkdownSummary(lines, title, rows, money) {
  lines.push('### ' + title);
  lines.push('');
  lines.push('| 项目 | 用例数 | 通过数 | 预计（' + money.label + '） | 实际（' + money.label + '） |');
  lines.push('|---|---:|---:|---:|---:|');
  for (const row of rows || []) {
    lines.push(
      '| ' + zhSummaryKey(row.key) + ' | ' + row.cases + ' | ' + row.passed +
      ' | ' + fmtReportMoney(row.expectedUsd, money) + ' | ' + fmtReportMoney(row.actualUsd, money) + ' |',
    );
  }
  lines.push('');
}

function renderHtmlSummaries(summary, money) {
  const sections = [
    ['按分组', summary.billingByGroup],
    ['按模型', summary.billingByModel],
    ['按分辨率', summary.billingByResolution],
    ['按输入模态', summary.billingByModality],
  ];
  return sections.map(([title, rows]) => {
    const body = (rows || []).map((row) =>
      '<tr><td>' + escapeHtml(zhSummaryKey(row.key)) + '</td><td>' + row.cases + '</td><td>' +
      row.passed + '</td><td>' + escapeHtml(fmtReportMoney(row.expectedUsd, money)) + '</td><td>' +
      escapeHtml(fmtReportMoney(row.actualUsd, money)) + '</td></tr>',
    ).join('');
    return '<h3>' + escapeHtml(title) + '</h3><table><thead><tr><th>项目</th><th>用例数</th><th>通过数</th><th>预计（' + money.label + '）</th><th>实际（' + money.label + '）</th></tr></thead><tbody>' +
      body + '</tbody></table>';
  }).join('');
}

export function extractPrompt(body) {
  if (typeof body?.prompt === 'string') {
    return body.prompt;
  }
  if (Array.isArray(body?.content)) {
    return body.content.filter((item) => item?.type === 'text').map((item) => item.text).filter(Boolean).join(' ');
  }
  return '';
}

export async function writeReports(run, reportDir) {
  await fs.mkdir(reportDir, { recursive: true });
  const privateRun = sanitizeDeep(JSON.parse(JSON.stringify(run, (key, value) => {
    if (['admin', 'password', 'token', 'cookie', 'session', 'fetchImpl'].includes(key)) {
      return undefined;
    }
    return value;
  })));
  await Promise.all([
    fs.writeFile(path.join(reportDir, 'results.json'), JSON.stringify(privateRun, null, 2) + '\n', 'utf8'),
    fs.writeFile(path.join(reportDir, 'billing.csv'), renderBillingCsv(privateRun), 'utf8'),
    fs.writeFile(path.join(reportDir, 'external-costs.csv'), renderExternalCostCsv(privateRun), 'utf8'),
    fs.writeFile(path.join(reportDir, 'report.md'), renderMarkdownReport(privateRun), 'utf8'),
    fs.writeFile(path.join(reportDir, 'report.html'), renderHtmlReport(privateRun), 'utf8'),
  ]);
}

export function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
