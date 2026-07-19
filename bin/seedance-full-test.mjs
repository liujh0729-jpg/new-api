#!/usr/bin/env node

import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import {
  DEFAULT_BASE_URL,
  DEFAULT_MAX_COST_USD,
  DEFAULT_POLL_INTERVAL_SECONDS,
  DEFAULT_TIMEOUT_SECONDS,
  SEEDANCE_MODELS,
  HttpSession,
  accountUsername,
  apiSuccess,
  buildCostPlan,
  buildNegativeCases,
  buildPositiveCases,
  computeQuote,
  csv,
  deriveAccountPassword,
  dedupeAccounts,
  downloadArtifact,
  extractPrompt,
  findPricingEntry,
  getOutputUrls,
  getTaskId,
  getTaskStatus,
  groupRatioFor,
  inspectMedia,
  isSuccessStatus,
  login,
  makeRunId,
  materializeCase,
  pageItems,
  parseArgs,
  parseLogOther,
  pollVideoTask,
  preflightAsset,
  redactUrl,
  sanitizeDeep,
  sleep,
  stripTrailingSlash,
  sumNetLogQuota,
  toNumber,
  validateCoverage,
  writeReports,
} from './seedance-full-test-lib.mjs';

const args = parseArgs(process.argv.slice(2));
const config = {
  baseUrl: stripTrailingSlash(args['base-url'] || process.env.NEW_API_BASE_URL || DEFAULT_BASE_URL),
  adminUsername: String(args['admin-username'] || process.env.NEW_API_ADMIN_USERNAME || 'root'),
  imageUrl: args['image-url'] || process.env.SEEDANCE_TEST_IMAGE_URL || '',
  audioUrl: args['audio-url'] || process.env.SEEDANCE_TEST_AUDIO_URL || '',
  callbackUrl: args['callback-url'] || process.env.SEEDANCE_TEST_CALLBACK_URL || '',
  serviceTier: args['service-tier'] === undefined
    ? (process.env.SEEDANCE_TEST_SERVICE_TIER || 'default')
    : String(args['service-tier']),
  groups: csv(args.groups || 'default,VIP1'),
  maxCostUsd: toNumber(args['max-cost-usd'], DEFAULT_MAX_COST_USD),
  timeoutSeconds: toNumber(args['timeout-seconds'], DEFAULT_TIMEOUT_SECONDS),
  pollIntervalSeconds: toNumber(args['poll-interval-seconds'], DEFAULT_POLL_INTERVAL_SECONDS),
  concurrency: Math.max(1, Math.floor(toNumber(args.concurrency, 2))),
  reportRoot: path.resolve(args['report-dir'] || '.test/seedance'),
  allowHttp: Boolean(args['allow-http']),
  dryRun: Boolean(args['dry-run']),
  listCases: Boolean(args['list-cases']),
  cleanupRun: typeof args['cleanup-run'] === 'string' ? args['cleanup-run'] : '',
  catalogBaseUrl: stripTrailingSlash(process.env.AIPDD_CATALOG_BASE_URL || ''),
  only: csv(args.only || ''),
  skipNegative: Boolean(args['skip-negative']),
  referenceVideoUrl: args['reference-video-url'] || process.env.SEEDANCE_TEST_REFERENCE_VIDEO_URL || '',
  repairRun: typeof args['repair-report'] === 'string' ? args['repair-report'] : '',
  repairOnline: Boolean(args.online),
  mergeRuns: csv(args['merge-report'] || ''),
};

let interrupted = false;
let signalCount = 0;
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, () => {
    signalCount += 1;
    interrupted = true;
    if (signalCount === 1) {
      process.stderr.write('\nReceived ' + signal + '; stopping after the current request and cleaning up.\n');
    } else {
      process.stderr.write('\nForced exit; use --cleanup-run if cleanup is incomplete.\n');
      process.exit(130);
    }
  });
}

main().catch((error) => {
  console.error(error?.stack || error?.message || String(error));
  process.exitCode = 1;
});

async function main() {
  if (args.help || args.h) {
    printUsage();
    return;
  }
  validateConfig();
  const previewCases = buildPositiveCases({
    imageUrl: config.imageUrl || undefined,
    audioUrl: config.audioUrl || undefined,
    callbackUrl: config.callbackUrl,
    serviceTier: config.serviceTier,
  });
  if (config.listCases) {
    printCases(previewCases, buildNegativeCases({ audioUrl: config.audioUrl || undefined }));
    return;
  }
  if (config.cleanupRun) {
    await cleanupPreviousRun(config.cleanupRun);
    return;
  }
  if (config.repairRun) {
    await repairExistingReport(config.repairRun);
    return;
  }
  if (config.mergeRuns.length > 0) {
    await mergeReports(config.mergeRuns);
    return;
  }
  if (config.dryRun) {
    await runDryRun(previewCases);
    return;
  }
  await runFullTest();
}

function validateConfig() {
  let parsed;
  try {
    parsed = new URL(config.baseUrl);
  } catch {
    throw new Error('Invalid --base-url: ' + config.baseUrl);
  }
  if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') {
    throw new Error('--base-url must use HTTP or HTTPS.');
  }
  const localHost = ['localhost', '127.0.0.1', '::1'].includes(parsed.hostname);
  if (parsed.protocol === 'http:' && !localHost && !config.allowHttp) {
    throw new Error('Refusing credentials over plain HTTP. Pass --allow-http only for a trusted server.');
  }
  if (config.groups.join(',') !== 'default,VIP1') {
    throw new Error('--groups must be exactly default,VIP1 for this audit.');
  }
  if (!(config.maxCostUsd > 0)) {
    throw new Error('--max-cost-usd must be positive.');
  }
  if (config.maxCostUsd > 12) {
    throw new Error('This audit enforces an absolute maximum of 12 USD.');
  }
  if (!(config.timeoutSeconds > 0) || !(config.pollIntervalSeconds > 0)) {
    throw new Error('Timeout and poll interval must be positive.');
  }
}

function printCases(positive, negative) {
  const coverage = validateCoverage(positive);
  console.log(JSON.stringify({
    coverage,
    positive: positive.map((item) => ({
      id: item.id,
      group: item.group,
      model: item.model,
      resolution: item.resolution,
      modalities: item.modalities,
      expected_seconds: item.expectedSeconds,
      request: sanitizeDeep(item.body),
    })),
    negative: negative.map((item) => ({
      id: item.id,
      group: item.group,
      description: item.description,
      request: sanitizeDeep(item.body),
    })),
  }, null, 2));
}

async function runDryRun(positiveCases) {
  console.log('Dry-run: no users, tokens, quotas, or model tasks will be created.');
  const publicSession = new HttpSession(config.baseUrl);
  const [statusResponse, pricingResponse, image, audio] = await Promise.all([
    apiSuccess(publicSession, 'GET', '/api/status'),
    apiSuccess(publicSession, 'GET', '/api/pricing'),
    config.imageUrl ? preflightAsset(config.imageUrl, 'image') : Promise.resolve(null),
    config.audioUrl ? preflightAsset(config.audioUrl, 'audio') : Promise.resolve(null),
  ]);
  const coverage = validateCoverage(positiveCases);
  const quotaPerUnit = Number(statusResponse.data?.data?.quota_per_unit || 0);
  const pricing = pricingResponse.data || {};
  const output = {
    base_url: redactUrl(config.baseUrl),
    server_version: statusResponse.data?.data?.version || '',
    quota_per_unit: quotaPerUnit,
    pricing_version: pricing.pricing_version || '',
    image_preflight: image,
    audio_preflight: audio,
    coverage,
    task_pricing_models: (pricing.data || [])
      .filter((item) => Object.values(SEEDANCE_MODELS).includes(item.model_name))
      .map((item) => ({
        model: item.model_name,
        resolutions: item.task_pricing_resolutions,
        billing_mode: item.billing_mode,
      })),
  };
  try {
    if (quotaPerUnit > 0) {
      const pricingByGroup = { default: pricing, VIP1: pricing };
      const costPlan = buildCostPlan(positiveCases, pricingByGroup, quotaPerUnit, config.maxCostUsd);
      output.projected_cost_usd = costPlan.totalUsd;
      output.note = 'Public pricing group ratios were used. The real run recalculates after both group accounts log in.';
    }
  } catch (error) {
    output.projected_cost_unavailable = error.message;
  }
  console.log(JSON.stringify(output, null, 2));
  if (!coverage.ok) {
    process.exitCode = 1;
  }
}

async function runFullTest() {
  if (!config.imageUrl || !config.audioUrl) {
    throw new Error('A real run requires --image-url and --audio-url with fresh, long-lived signed URLs.');
  }
  console.log('Preflighting signed assets with ranged GET requests...');
  const [imagePreflight, audioPreflight] = await Promise.all([
    preflightAsset(config.imageUrl, 'image'),
    preflightAsset(config.audioUrl, 'audio'),
  ]);
  const runId = makeRunId();
  const reportDir = path.join(config.reportRoot, runId);
  const artifactDir = path.join(reportDir, 'artifacts');
  await fs.mkdir(artifactDir, { recursive: true });
  const run = {
    schemaVersion: 1,
    runId,
    baseUrl: config.baseUrl,
    startedAt: new Date().toISOString(),
    finishedAt: '',
    environment: {
      imagePreflight,
      audioPreflight,
      callbackVerification: config.callbackUrl ? 'requested' : 'not_configured',
      serviceTierProbe: config.serviceTier || 'not_configured',
    },
    accounts: [],
    costPlan: null,
    results: [],
    assertions: [],
    cleanup: {},
    summary: {},
  };
  console.log('Run ID: ' + runId);
  console.log('Reports: ' + reportDir);

  const adminPassword = await getAdminPassword();
  let adminSession = null;
  try {
    const adminLogin = await login(config.baseUrl, config.adminUsername, adminPassword);
    adminSession = adminLogin.session;
    if (Number(adminLogin.user.role) < 100) {
      throw new Error('The supplied administrator is not a root user.');
    }
    const [statusResponse, adminPricingResponse, catalog] = await Promise.all([
      apiSuccess(adminSession, 'GET', '/api/status'),
      apiSuccess(adminSession, 'GET', '/api/pricing'),
      fetchOptionalCatalog(),
    ]);
    const status = statusResponse.data?.data || {};
    const adminPricing = adminPricingResponse.data || {};
    const quotaPerUnit = Number(status.quota_per_unit);
    if (!(quotaPerUnit > 0)) {
      throw new Error('Server returned an invalid quota_per_unit.');
    }
    run.environment.serverVersion = status.version || '';
    run.environment.quotaPerUnit = quotaPerUnit;
    run.environment.pricingVersion = adminPricing.pricing_version || '';
    run.environment.catalogRevision = catalog?.revision || '';
    run.environment.catalogGeneratedAt = catalog?.generatedAt || '';
    verifyPricingCoverage(adminPricing);

    for (const group of config.groups) {
      const account = await createTestAccount(adminSession, adminPassword, runId, group);
      run.accounts.push(account);
    }
    const pricingByGroup = {};
    for (const account of run.accounts) {
      const pricingResponse = await apiSuccess(account.session, 'GET', '/api/pricing');
      pricingByGroup[account.group] = pricingResponse.data;
      account.pricingVersion = pricingResponse.data?.pricing_version || '';
      account.advertisedGroupRatio = groupRatioFor(pricingResponse.data, account.group);
    }
    const positiveCases = buildPositiveCases({
      imageUrl: config.imageUrl,
      audioUrl: config.audioUrl,
      callbackUrl: config.callbackUrl,
      serviceTier: config.serviceTier,
    });
    const selectedPositiveCases = config.only.length > 0
      ? positiveCases.filter((item) => config.only.includes(item.id))
      : positiveCases;
    const unknownCaseIds = config.only.filter((id) => !positiveCases.some((item) => item.id === id));
    if (unknownCaseIds.length > 0) {
      throw new Error('Unknown --only case ids: ' + unknownCaseIds.join(', '));
    }
    if (selectedPositiveCases.length === 0) {
      throw new Error('No positive cases were selected.');
    }
    const fullMatrix = selectedPositiveCases.length === positiveCases.length;
    const coverage = validateCoverage(positiveCases);
    if (fullMatrix && !coverage.ok) {
      throw new Error('Internal coverage matrix is invalid: missing ' + coverage.missing.join(', '));
    }
    run.selection = {
      positiveIds: selectedPositiveCases.map((item) => item.id),
      negativeIds: config.skipNegative ? [] : buildNegativeCases({ audioUrl: config.audioUrl }).map((item) => item.id),
      fullMatrix,
      referenceVideoProvided: Boolean(config.referenceVideoUrl),
    };
    const costPlan = buildCostPlan(
      selectedPositiveCases,
      pricingByGroup,
      quotaPerUnit,
      config.maxCostUsd,
      config.groups,
    );
    run.costPlan = {
      maxCostUsd: costPlan.maxCostUsd,
      totalUsd: costPlan.totalUsd,
      totalQuota: costPlan.totalQuota,
      byGroupUsd: costPlan.byGroupUsd,
      quotaCaps: costPlan.quotaCaps,
      cases: costPlan.planned.map((item) => ({
        id: item.id,
        group: item.group,
        model: item.model,
        resolution: item.resolution,
        quote: item.quote,
      })),
    };
    console.log('Projected positive cost: $' + costPlan.totalUsd.toFixed(6));
    console.log('Hard cap: $' + config.maxCostUsd.toFixed(2));

    for (const account of run.accounts) {
      await setUserQuota(adminSession, account.id, costPlan.quotaCaps[account.group]);
      const self = await getSelf(account.session);
      account.startQuota = Number(self.quota);
      account.startUsedQuota = Number(self.used_quota);
      console.log(
        'Prepared ' + account.username + ' group=' + account.group +
        ' quota=$' + (account.startQuota / quotaPerUnit).toFixed(6),
      );
    }

    const plannedById = new Map(costPlan.planned.map((item) => [item.id, item]));
    const referenceState = { url: config.referenceVideoUrl };
    const completedEarly = new Set();
    const bootstrap = plannedById.get('C01');
    if (bootstrap) {
      console.log('Running bootstrap case C01...');
      const bootstrapResult = await runPositiveCase(
        run,
        bootstrap,
        findAccount(run, bootstrap.group),
        reportDir,
        artifactDir,
        quotaPerUnit,
        '',
      );
      run.results.push(bootstrapResult);
      completedEarly.add('C01');
    }

    const referenceBootstrap = !referenceState.url ? plannedById.get('C07') : null;
    if (referenceBootstrap) {
      console.log('Running 720p reference bootstrap case C07...');
      const referenceBootstrapResult = await runPositiveCase(
        run,
        referenceBootstrap,
        findAccount(run, referenceBootstrap.group),
        reportDir,
        artifactDir,
        quotaPerUnit,
        '',
      );
      run.results.push(referenceBootstrapResult);
      completedEarly.add('C07');
      if (isSuccessStatus(referenceBootstrapResult.finalStatus)) {
        referenceState.url = referenceBootstrapResult.outputUrls?.[0] || '';
      }
    }

    const remaining = costPlan.planned.filter((item) => !completedEarly.has(item.id));
    const queues = config.groups.map((group) => remaining.filter((item) => item.group === group));
    const runQueue = async (queue) => {
      for (const plannedCase of queue) {
        if (interrupted) {
          run.results.push(blockedResult(plannedCase, 'interrupted'));
          continue;
        }
        if (plannedCase.referenceVideo && !referenceState.url) {
          run.results.push(blockedResult(plannedCase, 'bootstrap reference video unavailable'));
          continue;
        }
        const result = await runPositiveCase(
          run,
          plannedCase,
          findAccount(run, plannedCase.group),
          reportDir,
          artifactDir,
          quotaPerUnit,
          referenceState.url,
        );
        run.results.push(result);
      }
    };
    if (config.concurrency >= 2) {
      await Promise.all(queues.map(runQueue));
    } else {
      for (const queue of queues) {
        await runQueue(queue);
      }
    }

    const negativeCases = config.skipNegative ? [] : buildNegativeCases({ audioUrl: config.audioUrl });
    for (const negativeCase of negativeCases) {
      if (interrupted) {
        run.results.push(blockedResult(negativeCase, 'interrupted'));
        continue;
      }
      const result = await runNegativeCase(
        run,
        negativeCase,
        findAccount(run, negativeCase.group),
        reportDir,
        artifactDir,
        quotaPerUnit,
        pricingByGroup,
      );
      run.results.push(result);
    }
  } catch (error) {
    run.fatalError = sanitizeDeep(error?.message || String(error));
    console.error('Run error: ' + run.fatalError);
    process.exitCode = 1;
  } finally {
    if (adminSession && run.accounts.length > 0) {
      await cleanupAccounts(adminSession, run.accounts, runId).catch((error) => {
        run.cleanup.error = sanitizeDeep(error?.message || String(error));
        process.exitCode = 1;
      });
    }
    run.results.sort((left, right) => left.id.localeCompare(right.id, undefined, { numeric: true }));
    finalizeRun(run);
    run.finishedAt = new Date().toISOString();
    await writeReports(run, reportDir);
    console.log('Report: ' + path.join(reportDir, 'report.html'));
    console.log('Billing: ' + path.join(reportDir, 'billing.csv'));
    console.log('Overall: ' + (run.summary.pass ? 'PASS' : 'FAIL'));
    if (!run.summary.pass) {
      process.exitCode = 1;
    }
  }
}

function findAccount(run, group) {
  const account = run.accounts.find((item) => item.group === group);
  if (!account) {
    throw new Error('No account was prepared for group ' + group + '.');
  }
  return account;
}

async function createTestAccount(adminSession, adminPassword, runId, group) {
  const username = accountUsername(runId, group);
  const password = deriveAccountPassword(adminPassword, username);
  console.log('Creating test account ' + username + ' for ' + group + '...');
  await apiSuccess(adminSession, 'POST', '/api/user/', {
    body: {
      username,
      password,
      display_name: 'Seedance ' + group,
      role: 1,
    },
  });
  const user = await findUser(adminSession, username);
  await apiSuccess(adminSession, 'PUT', '/api/user/', {
    body: {
      id: user.id,
      username: user.username,
      display_name: user.display_name || username,
      role: 1,
      group,
      remark: 'Automated Seedance audit ' + runId,
      password: '',
    },
  });
  const userLogin = await login(config.baseUrl, username, password);
  const tokenName = 'seedance-' + runId + '-' + group.toLowerCase();
  await apiSuccess(userLogin.session, 'POST', '/api/token/', {
    body: {
      name: tokenName,
      expired_time: -1,
      remain_quota: 0,
      unlimited_quota: true,
      model_limits_enabled: false,
      model_limits: '',
      group: '',
      cross_group_retry: false,
    },
  });
  const tokenList = await apiSuccess(userLogin.session, 'GET', '/api/token/?p=1&page_size=100');
  const token = pageItems(tokenList.data).find((item) => item.name === tokenName);
  if (!token?.id) {
    throw new Error('Created token could not be found for ' + username + '.');
  }
  const keyResponse = await apiSuccess(userLogin.session, 'POST', '/api/token/' + token.id + '/key');
  const key = keyResponse.data?.data?.key;
  if (!key) {
    throw new Error('Token key could not be retrieved for ' + username + '.');
  }
  return {
    id: user.id,
    username,
    group,
    role: 1,
    status: 1,
    password,
    session: userLogin.session,
    tokenId: token.id,
    tokenName,
    token: key.startsWith('sk-') ? key : 'sk-' + key,
    tokenDeleted: false,
    disabled: false,
  };
}

async function findUser(adminSession, username) {
  const response = await apiSuccess(
    adminSession,
    'GET',
    '/api/user/search?keyword=' + encodeURIComponent(username) + '&p=1&page_size=100',
  );
  const user = pageItems(response.data).find((item) => item.username === username);
  if (!user) {
    throw new Error('User ' + username + ' was not found after creation.');
  }
  return user;
}

async function setUserQuota(adminSession, userId, quota) {
  const targetQuota = Number(quota ?? 0);
  if (!Number.isFinite(targetQuota) || targetQuota < 0) {
    throw new Error('Invalid quota target for test user ' + userId + '.');
  }
  const userResponse = await apiSuccess(adminSession, 'GET', '/api/user/' + userId);
  const currentQuota = Number(userResponse.data?.data?.quota || 0);
  const delta = targetQuota - currentQuota;
  if (delta === 0) {
    return;
  }
  await apiSuccess(adminSession, 'POST', '/api/user/manage', {
    body: {
      id: userId,
      action: 'add_quota',
      value: Math.abs(delta),
      mode: delta > 0 ? 'add' : 'subtract',
    },
  });
}

async function getSelf(session) {
  const response = await apiSuccess(session, 'GET', '/api/user/self');
  return response.data?.data || {};
}

async function runPositiveCase(run, plannedCase, account, reportDir, artifactDir, quotaPerUnit, referenceVideoUrl) {
  const testCase = materializeCase(plannedCase, referenceVideoUrl);
  const result = baseResult(testCase);
  result.billing.expected = plannedCase.quote;
  result.request = sanitizeDeep(testCase.body);
  result.prompt = extractPrompt(testCase.body);
  console.log('[' + testCase.id + '] ' + testCase.group + ' ' + testCase.model + ' ' + testCase.resolution);
  const before = await getSelf(account.session);
  result.billing.beforeQuota = Number(before.quota);
  const actualSpent = totalAccountSpend(run, quotaPerUnit);
  if (actualSpent + plannedCase.quote.saleUsd > config.maxCostUsd + 1 / quotaPerUnit) {
    return blockedResult(testCase, 'hard cost cap would be exceeded', result);
  }
  const submittedAt = Math.floor(Date.now() / 1000);
  let body = structuredClone(testCase.body);
  let create = await account.session.request('POST', '/v1/videos', {
    token: account.token,
    body,
  });
  result.attempts.push(summarizeAttempt(create, body));
  let taskId = getTaskId(create.data);
  if ((!create.httpOk || !taskId) && testCase.serviceTierProbe && body.service_tier) {
    const safe = await confirmUnbilledFailure(account, create, submittedAt, Number(before.quota));
    if (safe) {
      result.capabilities.serviceTier = 'unsupported_or_rejected';
      delete body.service_tier;
      create = await account.session.request('POST', '/v1/videos', {
        token: account.token,
        body,
      });
      result.attempts.push(summarizeAttempt(create, body));
      taskId = getTaskId(create.data);
    }
  }
  result.requestId = create.requestId;
  result.httpStatus = create.status;
  result.submitLatencyMs = create.elapsedMs;
  result.taskId = taskId;
  if (!create.httpOk || !taskId) {
    const after = await getSelf(account.session);
    await fillBilling(result, account, before, after, quotaPerUnit, submittedAt);
    result.outcome = 'create_failed';
    result.error = sanitizeDeep(create.data?.error || create.data?.message || create.raw);
    result.pass = false;
    return result;
  }
  const pollStarted = Date.now();
  let finalResponse;
  try {
    finalResponse = await pollVideoTask({
      session: account.session,
      token: account.token,
      taskId,
      timeoutSeconds: config.timeoutSeconds,
      pollIntervalSeconds: config.pollIntervalSeconds,
      shouldStop: () => interrupted,
      onPoll: ({ status }) => process.stdout.write('  ' + testCase.id + ' status=' + (status || 'unknown') + '\n'),
    });
  } catch (error) {
    result.error = sanitizeDeep(error.message);
    result.outcome = 'poll_failed';
    result.pass = false;
    const after = await getSelf(account.session);
    await fillBilling(result, account, before, after, quotaPerUnit, submittedAt);
    return result;
  }
  result.pollLatencyMs = Date.now() - pollStarted;
  result.finalStatus = getTaskStatus(finalResponse.data);
  result.outputUrls = getOutputUrls(finalResponse.data);
  const task = await findTask(account.session, taskId, submittedAt);
  result.task = task ? sanitizeDeep(task) : null;
  if (result.outputUrls.length === 0 && task) {
    result.outputUrls = getOutputUrls(task);
  }
  await collectArtifacts(result, artifactDir, reportDir);
  const after = await getSelf(account.session);
  await fillBilling(result, account, before, after, quotaPerUnit, submittedAt, task);
  const billingPass = billingMatches(result.billing, quotaPerUnit);
  result.pass = isSuccessStatus(result.finalStatus) &&
    result.artifacts.some((artifact) => artifact.path && !artifact.error) &&
    billingPass;
  result.outcome = result.pass ? 'completed' : 'completed_with_failures';
  return result;
}

async function runNegativeCase(
  run,
  testCase,
  account,
  reportDir,
  artifactDir,
  quotaPerUnit,
  pricingByGroup,
) {
  const result = baseResult(testCase);
  result.request = sanitizeDeep(testCase.body);
  result.prompt = extractPrompt(testCase.body);
  console.log('[' + testCase.id + '] negative: ' + testCase.description);
  if (testCase.providerProbe) {
    const entry = findPricingEntry(pricingByGroup[testCase.group], testCase.model);
    result.billing.expectedProbe = computeQuote(
      entry,
      groupRatioFor(pricingByGroup[testCase.group], testCase.group),
      { ...testCase, referenceVideo: false },
      quotaPerUnit,
    );
    if (totalAccountSpend(run, quotaPerUnit) + result.billing.expectedProbe.saleUsd > config.maxCostUsd) {
      return blockedResult(testCase, 'audio-only probe could exceed hard cap', result);
    }
  }
  const before = await getSelf(account.session);
  result.billing.beforeQuota = Number(before.quota);
  const submittedAt = Math.floor(Date.now() / 1000);
  const response = await account.session.request('POST', '/v1/videos', {
    token: account.token,
    body: testCase.body,
  });
  result.attempts.push(summarizeAttempt(response, testCase.body));
  result.requestId = response.requestId;
  result.httpStatus = response.status;
  result.submitLatencyMs = response.elapsedMs;
  result.taskId = getTaskId(response.data);
  if (result.taskId) {
    try {
      const final = await pollVideoTask({
        session: account.session,
        token: account.token,
        taskId: result.taskId,
        timeoutSeconds: config.timeoutSeconds,
        pollIntervalSeconds: config.pollIntervalSeconds,
        shouldStop: () => interrupted,
      });
      result.finalStatus = getTaskStatus(final.data);
      result.outputUrls = getOutputUrls(final.data);
      await collectArtifacts(result, artifactDir, reportDir);
    } catch (error) {
      result.error = sanitizeDeep(error.message);
    }
  }
  await sleep(500);
  const after = await getSelf(account.session);
  const task = result.taskId ? await findTask(account.session, result.taskId, submittedAt) : null;
  result.task = task ? sanitizeDeep(task) : null;
  await fillBilling(result, account, before, after, quotaPerUnit, submittedAt, task);
  const rejected = !result.taskId && (!response.httpOk || response.data?.error || response.data?.success === false);
  const zeroNet = Math.abs(result.billing.balanceDeltaQuota || 0) === 0 &&
    Math.abs(result.billing.netLogQuota || 0) === 0;
  result.outcome = rejected ? 'rejected_as_expected' : result.taskId ? 'unexpected_task_created' : 'unexpected_acceptance';
  result.pass = Boolean(rejected && zeroNet);
  if (!result.pass && !result.error) {
    result.error = sanitizeDeep(response.data?.error || response.data?.message || 'Expected rejection with zero net charge.');
  }
  return result;
}

function baseResult(testCase) {
  return {
    id: testCase.id,
    description: testCase.description,
    group: testCase.group,
    model: testCase.model,
    resolution: testCase.resolution,
    ratio: testCase.ratio || testCase.body?.ratio || '',
    modalities: testCase.modalities || [],
    pass: false,
    attempts: [],
    capabilities: {},
    artifacts: [],
    outputUrls: [],
    billing: {},
  };
}

function blockedResult(testCase, reason, existing = null) {
  const result = existing || baseResult(testCase);
  result.outcome = 'blocked';
  result.error = reason;
  result.pass = false;
  return result;
}

function summarizeAttempt(response, body) {
  return {
    httpStatus: response.status,
    requestId: response.requestId,
    elapsedMs: response.elapsedMs,
    taskId: getTaskId(response.data),
    request: sanitizeDeep(body),
    error: response.httpOk && getTaskId(response.data)
      ? ''
      : sanitizeDeep(response.data?.error || response.data?.message || response.raw),
  };
}

async function confirmUnbilledFailure(account, response, submittedAt, beforeQuota) {
  if (getTaskId(response.data)) {
    return false;
  }
  await sleep(500);
  const [logs, self] = await Promise.all([
    fetchBillingLogs(account.session, response.requestId, '', submittedAt),
    getSelf(account.session),
  ]);
  return sumNetLogQuota(logs) === 0 && Number(self.quota) === beforeQuota;
}

async function findTask(session, taskId, submittedAt) {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    const response = await apiSuccess(
      session,
      'GET',
      '/api/task/self?task_id=' + encodeURIComponent(taskId) +
        '&start_timestamp=' + Math.max(0, submittedAt - 5) + '&p=1&page_size=100',
    );
    const task = pageItems(response.data).find((item) => item.task_id === taskId);
    if (task) {
      return task;
    }
    await sleep(500);
  }
  return null;
}

async function fetchBillingLogs(session, requestId, taskId, startTimestamp) {
  const collected = [];
  if (requestId) {
    for (let attempt = 0; attempt < 6; attempt += 1) {
      const response = await apiSuccess(
        session,
        'GET',
        '/api/log/self?request_id=' + encodeURIComponent(requestId) +
          '&start_timestamp=' + Math.max(0, startTimestamp - 5) + '&p=1&page_size=100',
      );
      const items = pageItems(response.data);
      collected.push(...items);
      if (items.length > 0) {
        break;
      }
      await sleep(500);
    }
  }
  if (taskId) {
    const response = await apiSuccess(
      session,
      'GET',
      '/api/log/self?start_timestamp=' + Math.max(0, startTimestamp - 5) + '&p=1&page_size=100',
    );
    collected.push(...pageItems(response.data).filter((log) => parseLogOther(log).task_id === taskId));
  }
  const seen = new Set();
  return collected.filter((log) => {
    const key = [
      log.created_at,
      log.type,
      log.quota,
      log.request_id,
      log.model_name,
      log.other,
    ].join('|');
    if (seen.has(key)) {
      return false;
    }
    seen.add(key);
    return true;
  });
}

async function fillBilling(result, account, before, after, quotaPerUnit, submittedAt, task = null) {
  const logs = await fetchBillingLogs(account.session, result.requestId, result.taskId, submittedAt);
  const consume = logs.find((log) => Number(log.type) === 2 && parseLogOther(log).billing_mode === 'task_pricing');
  const other = parseLogOther(consume);
  const balanceDeltaQuota = Number(before.quota || 0) - Number(after.quota || 0);
  const taskQuota = Number(task?.quota || 0);
  const netLogQuota = sumNetLogQuota(logs);
  const expectedUsd = result.billing.expected?.saleUsd;
  const authoritativeUsd = taskQuota > 0 ? taskQuota / quotaPerUnit : balanceDeltaQuota / quotaPerUnit;
  result.billing = {
    ...result.billing,
    beforeQuota: Number(before.quota || 0),
    afterQuota: Number(after.quota || 0),
    balanceDeltaQuota,
    balanceDeltaUsd: balanceDeltaQuota / quotaPerUnit,
    taskQuota,
    taskQuotaUsd: taskQuota / quotaPerUnit,
    netLogQuota,
    netLogUsd: netLogQuota / quotaPerUnit,
    log: consume ? {
      type: consume.type,
      quota: consume.quota,
      requestId: consume.request_id,
      billingMode: other.billing_mode,
      unit: other.billing_unit,
      variant: other.pricing_variant,
      unitPriceUsd: Number(other.unit_price_usd),
      quantity: Number(other.quantity),
      saleUsd: Number(other.sale_usd),
      groupRatio: Number(other.group_ratio),
      hasReferenceVideo: Boolean(other.has_reference_video),
      resolution: other.resolution,
    } : null,
    logs: logs.map((log) => ({
      createdAt: log.created_at,
      type: log.type,
      quota: log.quota,
      requestId: log.request_id,
      content: log.content,
      other: parseLogOther(log),
    })),
    discrepancyUsd: Number.isFinite(expectedUsd) ? authoritativeUsd - expectedUsd : null,
  };
}

function billingMatches(billing, quotaPerUnit) {
  if (!billing.expected || !billing.log) {
    return false;
  }
  const tolerance = Math.max(2 / quotaPerUnit, 0.000002);
  return Math.abs(billing.log.saleUsd - billing.expected.saleUsd) <= tolerance &&
    Math.abs(billing.taskQuotaUsd - billing.expected.saleUsd) <= tolerance &&
    billing.log.variant === billing.expected.variant &&
    Math.abs(billing.log.groupRatio - billing.expected.groupRatio) <= 1e-9;
}

async function collectArtifacts(result, artifactDir, reportDir) {
  for (let index = 0; index < result.outputUrls.length; index += 1) {
    const url = result.outputUrls[index];
    const extension = extensionForUrl(url, index);
    const outputPath = path.join(artifactDir, result.id.toLowerCase() + '-' + (index + 1) + extension);
    try {
      const downloaded = await downloadArtifact(url, outputPath);
      const relativePath = path.relative(reportDir, outputPath);
      const contactSheetPath = extension === '.mp4'
        ? path.join(artifactDir, result.id.toLowerCase() + '-' + (index + 1) + '-contact.jpg')
        : '';
      const media = extension === '.mp4'
        ? await inspectMedia(outputPath, contactSheetPath)
        : { metadata: { available: false, reason: 'not_video' }, contactSheet: null };
      result.artifacts.push({
        ...downloaded,
        relativePath,
        media: media.metadata,
        contactSheet: media.contactSheet,
        contactSheetRelative: media.contactSheet ? path.relative(reportDir, media.contactSheet) : '',
      });
    } catch (error) {
      result.artifacts.push({
        url: redactUrl(url),
        error: sanitizeDeep(error.message),
        relativePath: '',
      });
    }
  }
}

function extensionForUrl(url, index) {
  try {
    const extension = path.extname(new URL(url).pathname).toLowerCase();
    if (['.mp4', '.webm', '.mov', '.jpg', '.jpeg', '.png', '.webp'].includes(extension)) {
      return extension;
    }
  } catch {
    // Use the endpoint's primary output convention.
  }
  return index === 0 ? '.mp4' : '.jpg';
}

function totalAccountSpend(run, quotaPerUnit) {
  return run.accounts.reduce((sum, account) => {
    if (!Number.isFinite(account.startQuota)) {
      return sum;
    }
    const latest = run.results
      .filter((item) => item.group === account.group && Number.isFinite(item.billing?.afterQuota))
      .sort((left, right) => right.id.localeCompare(left.id, undefined, { numeric: true }))[0];
    const current = latest ? latest.billing.afterQuota : account.startQuota;
    return sum + Math.max(0, account.startQuota - current) / quotaPerUnit;
  }, 0);
}

async function cleanupAccounts(adminSession, accounts, runId) {
  for (const account of accounts) {
    try {
      const self = await getSelf(account.session);
      account.endQuota = Number(self.quota);
      account.endUsedQuota = Number(self.used_quota);
    } catch (error) {
      account.endQuotaError = sanitizeDeep(error.message);
    }
    try {
      if (account.tokenId) {
        await apiSuccess(account.session, 'DELETE', '/api/token/' + account.tokenId);
      } else {
        await deleteRunTokens(account.session, runId);
      }
      account.tokenDeleted = true;
      account.token = '';
    } catch (error) {
      account.tokenDeleteError = sanitizeDeep(error.message);
    }
    try {
      await apiSuccess(adminSession, 'POST', '/api/user/manage', {
        body: { id: account.id, action: 'disable', value: 0, mode: '' },
      });
      account.disabled = true;
    } catch (error) {
      account.disableError = sanitizeDeep(error.message);
    }
  }
}

async function deleteRunTokens(session, runId) {
  const response = await apiSuccess(session, 'GET', '/api/token/?p=1&page_size=100');
  const tokens = pageItems(response.data).filter((token) => token.name?.startsWith('seedance-' + runId));
  for (const token of tokens) {
    await apiSuccess(session, 'DELETE', '/api/token/' + token.id);
  }
}

async function cleanupPreviousRun(runId) {
  if (!/^sd\d{12}[0-9a-f]{4}$/i.test(runId)) {
    throw new Error('Invalid cleanup run id. Expected the sdYYMMDDHHMMSSxxxx format.');
  }
  const adminPassword = await getAdminPassword();
  const adminLogin = await login(config.baseUrl, config.adminUsername, adminPassword);
  const summary = [];
  for (const group of config.groups) {
    const username = accountUsername(runId, group);
    let user;
    try {
      user = await findUser(adminLogin.session, username);
    } catch (error) {
      summary.push({ username, group, found: false, error: sanitizeDeep(error.message) });
      continue;
    }
    const item = { username, group, found: true, tokenDeleted: false, disabled: false };
    try {
      if (Number(user.status) !== 1) {
        await apiSuccess(adminLogin.session, 'POST', '/api/user/manage', {
          body: { id: user.id, action: 'enable', value: 0, mode: '' },
        });
      }
      const password = deriveAccountPassword(adminPassword, username);
      const userLogin = await login(config.baseUrl, username, password);
      await deleteRunTokens(userLogin.session, runId);
      item.tokenDeleted = true;
    } catch (error) {
      item.tokenDeleteError = sanitizeDeep(error.message);
    }
    try {
      await apiSuccess(adminLogin.session, 'POST', '/api/user/manage', {
        body: { id: user.id, action: 'disable', value: 0, mode: '' },
      });
      item.disabled = true;
    } catch (error) {
      item.disableError = sanitizeDeep(error.message);
    }
    summary.push(item);
  }
  console.log(JSON.stringify(summary, null, 2));
  if (summary.some((item) => item.found && (!item.tokenDeleted || !item.disabled))) {
    process.exitCode = 1;
  }
}

async function repairExistingReport(runId) {
  if (!/^sd\d{12}[0-9a-f]{4}$/i.test(runId)) {
    throw new Error('Invalid repair run id.');
  }
  const reportDir = path.join(config.reportRoot, runId);
  const artifactDir = path.join(reportDir, 'artifacts');
  const run = JSON.parse(await fs.readFile(path.join(reportDir, 'results.json'), 'utf8'));
  let adminSession = null;
  if (config.repairOnline) {
    const adminPassword = await getAdminPassword();
    adminSession = (await login(config.baseUrl, config.adminUsername, adminPassword)).session;
  }
  run.selection ||= {
    positiveIds: run.results.filter((item) => item.id.startsWith('C')).map((item) => item.id),
    negativeIds: run.results.filter((item) => item.id.startsWith('N')).map((item) => item.id),
    fullMatrix: true,
    referenceVideoProvided: false,
  };
  if (adminSession) {
    for (const account of run.accounts || []) {
      const liveUser = await findUser(adminSession, account.username);
      account.endQuota = Number(liveUser.quota);
      account.status = Number(liveUser.status);
      account.disabled = Number(liveUser.status) !== 1;
    }
  }
  for (const result of run.results) {
    if (!isSuccessStatus(result.finalStatus)) {
      if (!result.error && result.task?.fail_reason) {
        result.error = result.task.fail_reason;
      }
      continue;
    }
    if (adminSession && result.taskId) {
      const response = await apiSuccess(
        adminSession,
        'GET',
        '/api/task/?task_id=' + encodeURIComponent(result.taskId) + '&p=1&page_size=100',
      );
      const liveTask = pageItems(response.data).find((item) => item.task_id === result.taskId);
      if (liveTask) {
        result.outputUrls = getOutputUrls(liveTask);
        result.task = sanitizeDeep(liveTask);
      }
    } else if (!Array.isArray(result.outputUrls) || result.outputUrls.length === 0) {
      result.outputUrls = getOutputUrls(result.task);
    }
    if (!Array.isArray(result.artifacts) || !result.artifacts.some((item) => item.path && !item.error)) {
      result.artifacts = [];
      await collectArtifacts(result, artifactDir, reportDir);
    }
    result.pass = result.artifacts.some((item) => item.path && !item.error) &&
      billingMatches(result.billing, Number(run.environment.quotaPerUnit));
    result.outcome = result.pass ? 'completed' : 'completed_with_failures';
  }
  run.assertions = [];
  finalizeRun(run);
  run.repairedAt = new Date().toISOString();
  await writeReports(run, reportDir);
  console.log('Repaired report: ' + path.join(reportDir, 'report.html'));
}

async function mergeReports(runIds) {
  if (runIds.length < 2 || runIds.some((id) => !/^sd\d{12}[0-9a-f]{4}$/i.test(id))) {
    throw new Error('--merge-report requires at least two comma-separated run ids.');
  }
  const baseDir = path.join(config.reportRoot, runIds[0]);
  const base = JSON.parse(await fs.readFile(path.join(baseDir, 'results.json'), 'utf8'));
  const supplements = [];
  for (const runId of runIds.slice(1)) {
    supplements.push(JSON.parse(
      await fs.readFile(path.join(config.reportRoot, runId, 'results.json'), 'utf8'),
    ));
  }
  const replacements = new Map();
  for (let index = 0; index < supplements.length; index += 1) {
    const supplement = supplements[index];
    const sourceDir = path.join(config.reportRoot, runIds[index + 1]);
    for (const result of supplement.results || []) {
      replacements.set(result.id, { result, sourceDir });
    }
  }
  base.results = await Promise.all(base.results.map(async (result) => {
    const replacement = replacements.get(result.id);
    if (!replacement) {
      return result;
    }
    return await copyMergedArtifacts(replacement.result, replacement.sourceDir, baseDir);
  }));
  base.accounts = dedupeAccounts([
    ...(base.accounts || []),
    ...supplements.flatMap((supplement) => supplement.accounts || []),
  ]);
  base.supplementalRunIds = runIds.slice(1);
  base.selection = {
    positiveIds: base.results.filter((item) => item.id.startsWith('C')).map((item) => item.id),
    negativeIds: base.results.filter((item) => item.id.startsWith('N')).map((item) => item.id),
    fullMatrix: true,
    referenceVideoProvided: true,
  };
  base.assertions = [];
  finalizeRun(base);
  base.mergedAt = new Date().toISOString();
  await writeReports(base, baseDir);
  console.log('Merged report: ' + path.join(baseDir, 'report.html'));
}

async function copyMergedArtifacts(result, sourceDir, targetDir) {
  const copied = { ...result, artifacts: [] };
  const targetArtifactDir = path.join(targetDir, 'artifacts');
  await fs.mkdir(targetArtifactDir, { recursive: true });
  for (const artifact of result.artifacts || []) {
    const next = { ...artifact };
    if (artifact.path) {
      const sourcePath = path.resolve(artifact.path);
      assertPathInside(sourceDir, sourcePath, 'artifact');
      const targetPath = path.join(targetArtifactDir, path.basename(sourcePath));
      await fs.copyFile(sourcePath, targetPath);
      next.path = targetPath;
      next.relativePath = path.relative(targetDir, targetPath);
    }
    if (artifact.contactSheet) {
      const sourceContactSheet = path.resolve(artifact.contactSheet);
      assertPathInside(sourceDir, sourceContactSheet, 'contact sheet');
      const targetContactSheet = path.join(targetArtifactDir, path.basename(sourceContactSheet));
      await fs.copyFile(sourceContactSheet, targetContactSheet);
      next.contactSheet = targetContactSheet;
      next.contactSheetRelative = path.relative(targetDir, targetContactSheet);
    }
    copied.artifacts.push(next);
  }
  return copied;
}

function assertPathInside(parentDir, candidatePath, label) {
  const relative = path.relative(path.resolve(parentDir), candidatePath);
  if (!relative || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) {
    if (!relative) {
      throw new Error('Refusing to copy a ' + label + ' directory as a file.');
    }
    throw new Error('Refusing to copy a ' + label + ' outside its report directory.');
  }
}

function finalizeRun(run) {
  const quotaPerUnit = Number(run.environment.quotaPerUnit || 1);
  run.summary.actualBalanceQuota = run.accounts.reduce((sum, account) => {
    if (!Number.isFinite(account.startQuota) || !Number.isFinite(account.endQuota)) {
      return sum;
    }
    return sum + account.startQuota - account.endQuota;
  }, 0);
  run.summary.actualBalanceUsd = run.summary.actualBalanceQuota / quotaPerUnit;
  run.summary.billingByGroup = aggregateBilling(run.results, (item) => item.group);
  run.summary.billingByModel = aggregateBilling(run.results, (item) => item.model);
  run.summary.billingByResolution = aggregateBilling(run.results, (item) => item.resolution);
  run.summary.billingByModality = aggregateBilling(
    run.results,
    (item) => (item.modalities || []).join('+') || 'validation-only',
  );
  addAssertions(run, quotaPerUnit);
  const cleanupPass = run.accounts.every((account) => account.tokenDeleted && account.disabled);
  run.summary.positivePassed = run.results.filter((item) => item.id.startsWith('C') && item.pass).length;
  run.summary.positiveTotal = run.results.filter((item) => item.id.startsWith('C')).length;
  run.summary.negativePassed = run.results.filter((item) => item.id.startsWith('N') && item.pass).length;
  run.summary.negativeTotal = run.results.filter((item) => item.id.startsWith('N')).length;
  const expectedResults = run.selection
    ? Number(run.selection.positiveIds?.length || 0) + Number(run.selection.negativeIds?.length || 0)
    : 22;
  run.summary.pass = !run.fatalError &&
    run.results.length === expectedResults &&
    run.results.every((item) => item.pass) &&
    run.assertions.every((item) => item.pass) &&
    cleanupPass;
}

function aggregateBilling(results, keyFor) {
  const values = new Map();
  for (const result of results || []) {
    const key = keyFor(result) || '-';
    const current = values.get(key) || {
      key,
      cases: 0,
      passed: 0,
      expectedUsd: 0,
      actualUsd: 0,
    };
    current.cases += 1;
    current.passed += result.pass ? 1 : 0;
    current.expectedUsd += Number(result.billing?.expected?.saleUsd || 0);
    const settledUsd = result.task?.status === 'SUCCESS'
      ? Number(result.billing?.taskQuotaUsd || 0)
      : Number(result.billing?.netLogUsd || 0);
    current.actualUsd += settledUsd;
    values.set(key, current);
  }
  return [...values.values()].sort((left, right) => left.key.localeCompare(right.key));
}

function addAssertions(run, quotaPerUnit) {
  const byId = new Map(run.results.map((item) => [item.id, item]));
  const c07 = byId.get('C07');
  const c14 = byId.get('C14');
  const c02 = byId.get('C02');
  const c06 = byId.get('C06');
  const c15 = byId.get('C15');
  if (run.selection?.fullMatrix !== false) {
    const pairRatio = c07?.billing?.log?.saleUsd > 0
      ? c14?.billing?.log?.saleUsd / c07.billing.log.saleUsd
      : NaN;
    const expectedVipRatio = c14?.billing?.expected?.groupRatio;
    run.assertions.push({
      name: 'default/VIP1 相同请求倍率',
      pass: Number.isFinite(pairRatio) && Math.abs(pairRatio - expectedVipRatio) <= 0.000002,
      detail: '实测=' + finite(pairRatio) + '，预期=' + finite(expectedVipRatio),
    });
    const ratios480 = [c02, c06].map((item) => item?.billing?.log?.groupRatio);
    run.assertions.push({
      name: '480p 原价策略',
      pass: ratios480.length === 2 && ratios480.every((value) => value === 1),
      detail: 'C02=' + finite(ratios480[0]) + ', C06=' + finite(ratios480[1]),
    });
    run.assertions.push({
      name: '参考视频计价档位',
      pass: c07?.billing?.log?.variant === 'no_reference_video' &&
        c15?.billing?.log?.variant === 'reference_video' &&
        c15?.billing?.log?.hasReferenceVideo === true,
      detail: 'C07=' + (c07?.billing?.log?.variant || '-') + '，C15=' + (c15?.billing?.log?.variant || '-'),
    });
  } else {
    run.assertions.push({
      name: '选定用例执行完成',
      pass: run.results.every((item) => item.pass),
      detail: (run.selection?.positiveIds || []).join(', '),
    });
  }
  run.assertions.push({
    name: '费用硬上限',
    pass: Number(run.summary.actualBalanceUsd) <= Number(config.maxCostUsd) + 1 / quotaPerUnit,
    detail: '实际=$' + Number(run.summary.actualBalanceUsd || 0).toFixed(6) +
      '，上限=$' + Number(config.maxCostUsd).toFixed(2),
  });
  if (run.selection?.fullMatrix !== false) {
    run.assertions.push({
      name: '模型与分辨率槽位覆盖',
      pass: validateCoverage(run.results.filter((item) => item.id.startsWith('C'))).missing.length === 0,
      detail: 'C01-C13 已覆盖目录公布的 13 个槽位',
    });
  }
  run.assertions.push({
    name: '测试账号与令牌清理',
    pass: run.accounts.length >= 2 && run.accounts.every((account) => account.tokenDeleted && account.disabled),
    detail: run.accounts.map((account) =>
      account.username + '：令牌已删除=' + Boolean(account.tokenDeleted) +
      '，账号已停用=' + Boolean(account.disabled),
    ).join('; '),
  });
}

function finite(value) {
  return Number.isFinite(Number(value)) ? Number(value).toFixed(6) : '-';
}

function verifyPricingCoverage(pricing) {
  const expected = {
    [SEEDANCE_MODELS.vip]: ['480p', '720p', '1080p', '4k'],
    [SEEDANCE_MODELS.standard]: ['480p', '720p', '1080p', '4k'],
    [SEEDANCE_MODELS.light]: ['480p', '720p', '1080p'],
    [SEEDANCE_MODELS.value]: ['1080p', '4k'],
  };
  const failures = [];
  for (const [model, resolutions] of Object.entries(expected)) {
    const entry = findPricingEntry(pricing, model);
    if (!entry || entry.billing_mode !== 'task_pricing') {
      failures.push(model + ': missing task_pricing');
      continue;
    }
    const available = new Set(entry.task_pricing_resolutions || []);
    for (const resolution of resolutions) {
      if (!available.has(resolution)) {
        failures.push(model + ': missing ' + resolution);
      }
    }
  }
  if (failures.length > 0) {
    throw new Error('Live pricing coverage does not match the 13-slot matrix: ' + failures.join('; '));
  }
}

async function fetchOptionalCatalog() {
  if (!config.catalogBaseUrl) {
    return null;
  }
  const key = String(process.env.AIPDD_API_KEY || '').trim();
  const headers = {};
  if (key) {
    headers.Authorization = 'Bearer ' + key;
    headers['X-API-Key'] = key;
  }
  const response = await fetch(config.catalogBaseUrl + '/v1/new-api/catalog', { headers });
  if (!response.ok) {
    throw new Error('Optional AIPDD catalog fetch failed with HTTP ' + response.status + '.');
  }
  const body = await response.json();
  if (body.code !== 0 || !body.data) {
    throw new Error('Optional AIPDD catalog returned an invalid envelope.');
  }
  return body.data;
}

async function getAdminPassword() {
  const fromEnvironment = String(process.env.NEW_API_ADMIN_PASSWORD || '');
  if (fromEnvironment) {
    return fromEnvironment;
  }
  return await hiddenPrompt('New API administrator password: ');
}

async function hiddenPrompt(label) {
  if (!process.stdin.isTTY || !process.stdout.isTTY) {
    throw new Error('Interactive password input is unavailable. Set NEW_API_ADMIN_PASSWORD in the process environment.');
  }
  return await new Promise((resolve, reject) => {
    const stdin = process.stdin;
    const stdout = process.stdout;
    const wasRaw = stdin.isRaw;
    let value = '';
    const restore = () => {
      stdin.off('data', onData);
      stdin.setRawMode(Boolean(wasRaw));
      stdin.pause();
    };
    const onData = (chunk) => {
      const text = chunk.toString('utf8');
      for (const character of text) {
        if (character === '\u0003') {
          restore();
          stdout.write('\n');
          reject(new Error('Password prompt interrupted.'));
          return;
        }
        if (character === '\r' || character === '\n') {
          restore();
          stdout.write('\n');
          resolve(value);
          return;
        }
        if (character === '\u007f' || character === '\b') {
          value = value.slice(0, -1);
          continue;
        }
        if (character >= ' ') {
          value += character;
        }
      }
    };
    stdout.write(label);
    stdin.setRawMode(true);
    stdin.resume();
    stdin.on('data', onData);
  });
}

function printUsage() {
  console.log([
    'Seedance 双分组全量测试与费用审计',
    '',
    '真实运行：',
    '  node bin/seedance-full-test.mjs --base-url http://14.103.100.4:6070 --allow-http',
    '    --image-url "https://..." --audio-url "https://..." --max-cost-usd 12',
    '',
    '安全检查：',
    '  node bin/seedance-full-test.mjs --list-cases',
    '  node bin/seedance-full-test.mjs --dry-run --base-url http://14.103.100.4:6070 --allow-http',
    '',
    '异常恢复：',
    '  node bin/seedance-full-test.mjs --cleanup-run sdYYMMDDHHMMSSxxxx',
    '    --base-url http://14.103.100.4:6070 --allow-http',
    '  node bin/seedance-full-test.mjs --repair-report sdYYMMDDHHMMSSxxxx',
    '    --online --base-url http://14.103.100.4:6070 --allow-http',
    '  node bin/seedance-full-test.mjs --merge-report runId1,runId2',
    '',
    '选项：',
    '  --admin-username root          管理员用户名（密码不接受命令行参数）',
    '  --groups default,VIP1          固定审计分组',
    '  --callback-url URL             可选的公网回调接收地址',
    '  --service-tier default         能力探测值；传入空值可省略',
    '  --max-cost-usd 12              费用硬上限；超过 12 美元将被拒绝',
    '  --timeout-seconds 1800         单任务轮询超时',
    '  --poll-interval-seconds 10     轮询间隔；仅重试 GET 请求',
    '  --concurrency 2                每个账号一个顺序执行器',
    '  --only C04,C08                 只运行选定的正向用例',
    '  --skip-negative                不重复运行异常校验用例',
    '  --reference-video-url URL      为指定参考用例提供已有的公开视频',
    '  --report-dir .test/seedance    已忽略的私有报告根目录',
    '',
    '环境变量：',
    '  NEW_API_ADMIN_PASSWORD         可选的非交互式密码输入',
    '  SEEDANCE_TEST_IMAGE_URL        --image-url 的替代输入',
    '  SEEDANCE_TEST_AUDIO_URL        --audio-url 的替代输入',
    '  SEEDANCE_TEST_CALLBACK_URL     --callback-url 的替代输入',
    '  SEEDANCE_TEST_REFERENCE_VIDEO_URL  --reference-video-url 的替代输入',
    '  AIPDD_CATALOG_BASE_URL         可选的目录服务地址',
    '  AIPDD_API_KEY                  可选目录凭据；永不写入报告',
  ].join('\n'));
}
