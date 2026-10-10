import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import {
  HttpSession,
  SEEDANCE_MODELS,
  apiSuccess,
  buildCostPlan,
  buildNegativeCases,
  buildPositiveCases,
  computeQuote,
  dedupeAccounts,
  deriveAccountPassword,
  makeRunId,
  parseArgs,
  pollVideoTask,
  preflightAsset,
  redactUrl,
  sanitizeDeep,
  validateCoverage,
  writeReports,
} from './seedance-full-test-lib.mjs';

test('argument parsing, run IDs, and deterministic recovery passwords', () => {
  assert.deepEqual(
    parseArgs(['--base-url=http://example.test', '--dry-run', '--groups', 'default,VIP1']),
    { 'base-url': 'http://example.test', 'dry-run': true, groups: 'default,VIP1' },
  );
  const runId = makeRunId(new Date('2026-07-17T12:34:56.000Z'), Buffer.from([0xab, 0xcd]));
  assert.equal(runId, 'sd260717123456abcd');
  const first = deriveAccountPassword('admin-secret', runId + 'd');
  const second = deriveAccountPassword('admin-secret', runId + 'd');
  assert.equal(first, second);
  assert.equal(first.length, 18);
  assert.notEqual(first, deriveAccountPassword('different-secret', runId + 'd'));
});

test('the positive matrix covers 13 slots and preserves exact comparison payloads', () => {
  const cases = buildPositiveCases({
    imageUrl: 'https://assets.test/character.jpg?Signature=secret',
    audioUrl: 'https://assets.test/audio.mp3?Signature=secret',
  });
  const coverage = validateCoverage(cases);
  assert.equal(coverage.ok, true);
  assert.equal(coverage.actualSlots.length, 13);
  const byId = new Map(cases.map((item) => [item.id, item]));
  assert.deepEqual(byId.get('C07').body, byId.get('C14').body);
  const c10 = structuredClone(byId.get('C10').body);
  const c16 = structuredClone(byId.get('C16').body);
  c10.generate_audio = true;
  assert.deepEqual(c10, c16);
  assert.equal(cases.filter((item) => item.group === 'default').length, 8);
  assert.equal(cases.filter((item) => item.group === 'VIP1').length, 8);
});

test('negative matrix includes local validation, audio-only, and unsupported frame cases', () => {
  const cases = buildNegativeCases({ audioUrl: 'https://assets.test/audio.mp3' });
  assert.equal(cases.length, 9);
  assert.equal(cases.every((item) => item.expectedNoCharge), true);
  assert.equal(cases.find((item) => item.id === 'N06').providerProbe, true);
  assert.equal(cases.find((item) => item.id === 'N07').body.fps, 24);
  assert.equal(cases.find((item) => item.id === 'N08').body.frames_per_second, 24);
  assert.equal(cases.find((item) => item.id === 'N09').body.framespersecond, 24);
});

test('redaction strips signed URL queries and secret-shaped fields recursively', () => {
  assert.equal(
    redactUrl('https://assets.test/a.jpg?Expires=1&Signature=secret#fragment'),
    'https://assets.test/a.jpg',
  );
  const sanitized = sanitizeDeep({
    image: 'https://assets.test/a.jpg?Signature=secret',
    password: 'secret',
    nested: { api_key: 'value', harmless: 'ok' },
    authorization: 'Bearer token',
  });
  assert.equal(sanitized.image, 'https://assets.test/a.jpg');
  assert.equal(sanitized.password, '[REDACTED]');
  assert.equal(sanitized.nested.api_key, '[REDACTED]');
  assert.equal(sanitized.nested.harmless, 'ok');
  assert.deepEqual(
    sanitizeDeep({ tokenDeleted: true, tokenId: 7, token: 'sk-secret' }),
    { tokenDeleted: true, tokenId: 7, token: '[REDACTED]' },
  );
});

test('task pricing applies native 480p policy and global VIP1 ratio', () => {
  const entry = pricingEntry(SEEDANCE_MODELS.standard, {
    '480p': tier(0.06, 0.08, 'none'),
    '720p': tier(0.10, 0.13, 'global'),
  });
  const at480 = computeQuote(entry, 0.78, {
    model: SEEDANCE_MODELS.standard,
    resolution: '480p',
    expectedSeconds: 5,
    referenceVideo: false,
  }, 500000);
  assert.equal(at480.groupRatio, 1);
  assert.equal(at480.saleUsd, 0.3);
  const at720 = computeQuote(entry, 0.78, {
    model: SEEDANCE_MODELS.standard,
    resolution: '720p',
    expectedSeconds: 5,
    referenceVideo: true,
  }, 500000);
  assert.equal(at720.groupRatio, 0.78);
  assert.equal(at720.variant, 'reference_video');
  assert.ok(Math.abs(at720.saleUsd - 0.507) < 1e-12);
});

test('cost planner covers the live-shaped matrix and rejects a low hard cap', () => {
  const cases = buildPositiveCases();
  const defaultPricing = pricingEnvelope(1);
  const vipPricing = pricingEnvelope(0.78);
  const plan = buildCostPlan(
    cases,
    { default: defaultPricing, VIP1: vipPricing },
    500000,
    12,
  );
  assert.equal(plan.planned.length, 16);
  assert.ok(plan.totalUsd > 10 && plan.totalUsd < 11);
  assert.equal(Object.values(plan.quotaCaps).reduce((sum, value) => sum + value, 0), 6000000);
  assert.throws(
    () => buildCostPlan(cases, { default: defaultPricing, VIP1: vipPricing }, 500000, 5),
    /exceeds hard cap/,
  );
});

test('selected cases assign zero quota to an idle audit group', () => {
  const selected = buildPositiveCases().filter((item) => item.id === 'C16');
  const plan = buildCostPlan(
    selected,
    { default: pricingEnvelope(1), VIP1: pricingEnvelope(0.78) },
    500000,
    12,
    ['default', 'VIP1'],
  );
  assert.equal(plan.quotaCaps.default, 0);
  assert.equal(plan.quotaCaps.VIP1, 6000000);
  assert.equal(plan.byGroupUsd.default, 0);
});

test('account merge is idempotent and keeps verified cleanup state', () => {
  const accounts = dedupeAccounts([
    { id: 1, username: 'run-default', tokenDeleted: true, disabled: true },
    { id: 1, username: 'run-default', tokenDeleted: '[REDACTED]', disabled: true },
    { id: 2, username: 'run-vip', tokenDeleted: true, disabled: true },
  ]);
  assert.equal(accounts.length, 2);
  assert.equal(accounts.find((item) => item.username === 'run-default').tokenDeleted, true);
});

test('asset preflight uses ranged GET and validates MIME', async (context) => {
  let method = '';
  let range = '';
  const server = http.createServer((request, response) => {
    method = request.method;
    range = request.headers.range;
    response.writeHead(206, {
      'Content-Type': 'image/jpeg',
      'Content-Range': 'bytes 0-3/4',
      'Content-Length': '4',
    });
    response.end(Buffer.from([1, 2, 3, 4]));
  });
  const baseUrl = await listen(server);
  context.after(() => server.close());
  const result = await preflightAsset(baseUrl + '/asset.jpg?Signature=secret', 'image');
  assert.equal(method, 'GET');
  assert.equal(range, 'bytes=0-1023');
  assert.equal(result.contentType, 'image/jpeg');
  assert.equal(result.url, baseUrl + '/asset.jpg');
});

test('HTTP session persists cookies and applies New-Api-User', async (context) => {
  const server = http.createServer(async (request, response) => {
    if (request.url === '/api/user/login') {
      response.writeHead(200, {
        'Content-Type': 'application/json',
        'Set-Cookie': 'session=test-cookie; Path=/; HttpOnly',
      });
      response.end(JSON.stringify({ success: true, data: { id: 42, username: 'root', role: 100 } }));
      return;
    }
    assert.equal(request.headers.cookie, 'session=test-cookie');
    assert.equal(request.headers['new-api-user'], '42');
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ success: true, data: { id: 42, quota: 100 } }));
  });
  const baseUrl = await listen(server);
  context.after(() => server.close());
  const session = new HttpSession(baseUrl);
  const loginResponse = await apiSuccess(session, 'POST', '/api/user/login', {
    body: { username: 'root', password: 'secret' },
  });
  session.userId = loginResponse.data.data.id;
  const self = await apiSuccess(session, 'GET', '/api/user/self');
  assert.equal(self.data.data.quota, 100);
});

test('page response helpers accept both API envelopes and response wrappers', async () => {
  const { pageItems } = await import('./seedance-full-test-lib.mjs');
  const page = { page: 1, page_size: 10, total: 1, items: [{ id: 7 }] };
  assert.deepEqual(pageItems({ success: true, data: page }), [{ id: 7 }]);
  assert.deepEqual(pageItems({ data: { success: true, data: page } }), [{ id: 7 }]);
});

test('task polling retries GET reads and never submits another POST', async (context) => {
  let gets = 0;
  let posts = 0;
  const server = http.createServer((request, response) => {
    if (request.method === 'POST') {
      posts += 1;
    }
    if (request.method === 'GET') {
      gets += 1;
    }
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({
      id: 'task_test',
      status: gets < 2 ? 'queued' : 'completed',
      output: gets < 2 ? [] : ['https://assets.test/output.mp4'],
    }));
  });
  const baseUrl = await listen(server);
  context.after(() => server.close());
  const session = new HttpSession(baseUrl);
  const result = await pollVideoTask({
    session,
    token: 'sk-test',
    taskId: 'task_test',
    timeoutSeconds: 2,
    pollIntervalSeconds: 0.001,
  });
  assert.equal(result.data.status, 'completed');
  assert.equal(gets, 2);
  assert.equal(posts, 0);
});

test('report writer removes in-memory credentials and emits all deliverables', async () => {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'seedance-report-'));
  const run = {
    runId: 'sd260717123456abcd',
    baseUrl: 'http://example.test',
    startedAt: '2026-07-17T00:00:00Z',
    finishedAt: '2026-07-17T01:00:00Z',
    environment: { quotaPerUnit: 500000, pricingVersion: 'p1' },
    reporting: {
      currency: 'CNY',
      usdCnyRate: 6.7934,
      usdCnyRateDate: '2026-07-17',
      usdCnyRateSource: '国家外汇管理局人民币汇率中间价',
    },
    costPlan: { maxCostUsd: 12, totalUsd: 1 },
    accounts: [{
      username: 'test',
      group: 'default',
      role: 1,
      password: 'user-secret',
      token: 'sk-secret',
      session: { cookie: 'session-secret', fetchImpl: () => {} },
      startQuota: 10,
      endQuota: 5,
      tokenDeleted: true,
      disabled: true,
    }],
    results: [{
      id: 'C01',
      outputUrls: ['https://assets.test/output.mp4?X-Tos-Signature=secret'],
      billing: {
        expected: { unitPriceUsd: 0.1, saleUsd: 0.5 },
        log: { saleUsd: 0.5 },
        taskQuotaUsd: 0.5,
        balanceDeltaUsd: 0.5,
        netLogUsd: 0.5,
        discrepancyUsd: 0,
      },
    }],
    externalCosts: [{
      id: 'AUX01',
      scope: 'upstream-reference-asset',
      taskId: 'cgt-test',
      currency: 'CNY',
      amount: 1.25,
      awcoin: 12500,
      description: 'Reference asset generation.',
    }],
    assertions: [],
    summary: { pass: true, actualBalanceUsd: 0.1 },
  };
  await writeReports(run, directory);
  for (const name of ['report.html', 'report.md', 'results.json', 'billing.csv', 'external-costs.csv']) {
    await fs.access(path.join(directory, name));
  }
  const json = await fs.readFile(path.join(directory, 'results.json'), 'utf8');
  assert.equal(json.includes('user-secret'), false);
  assert.equal(json.includes('sk-secret'), false);
  assert.equal(json.includes('session-secret'), false);
  assert.equal(json.includes('X-Tos-Signature'), false);
  assert.equal(json.includes('https://assets.test/output.mp4'), true);
  const externalCsv = await fs.readFile(path.join(directory, 'external-costs.csv'), 'utf8');
  assert.equal(externalCsv.includes('12500'), true);
  const html = await fs.readFile(path.join(directory, 'report.html'), 'utf8');
  assert.equal(html.includes('¥6.793400'), true);
  assert.equal(html.includes('1 美元 = 6.7934 元人民币（2026-07-17）'), true);
  const billingCsv = await fs.readFile(path.join(directory, 'billing.csv'), 'utf8');
  assert.equal(billingCsv.includes('expected_sale_cny'), true);
  assert.equal(billingCsv.includes('3.3967'), true);
});

function tier(noReference, reference, policy = '') {
  return {
    no_reference_video_unit_price: noReference,
    reference_video_policy: 'custom',
    reference_video_unit_price: reference,
    ...(policy ? { group_ratio_policy: policy } : {}),
  };
}

function pricingEntry(model, byResolution) {
  return {
    model_name: model,
    billing_mode: 'task_pricing',
    task_pricing_resolutions: Object.keys(byResolution),
    task_pricing: { unit: 'second', by_resolution: byResolution },
  };
}

function pricingEnvelope(groupRatio) {
  return {
    success: true,
    group_ratio: { default: 1, VIP1: groupRatio },
    data: [
      pricingEntry(SEEDANCE_MODELS.vip, {
        '480p': tier(0.07994089205, 0.10624307095, 'none'),
        '720p': tier(0.094569632, 0.1256002925),
        '1080p': tier(0.2024381185, 0.268932391),
        '4k': tier(0.52205392165, 0.6876985427),
      }),
      pricingEntry(SEEDANCE_MODELS.standard, {
        '480p': tier(0.0653121521, 0.08614702415, 'none'),
        '720p': tier(0.0682674531, 0.0901366805),
        '1080p': tier(0.14613963445, 0.18869596885),
        '4k': tier(0.1767269998, 0.2192833342),
      }),
      pricingEntry(SEEDANCE_MODELS.light, {
        '480p': tier(0.0425563344, 0.0617657909, 'none'),
        '720p': tier(0.05038788205, 0.0729959347),
        '1080p': tier(0.10683413115, 0.1459918694),
      }),
      pricingEntry(SEEDANCE_MODELS.value, {
        '1080p': tier(0.1149612089, 0.1489471704),
        '4k': tier(0.1628370851, 0.1968230466),
      }),
    ],
  };
}

async function listen(server) {
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  return 'http://127.0.0.1:' + address.port;
}
