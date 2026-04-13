import exec from 'k6/execution';
import http from 'k6/http';
import { Rate } from 'k6/metrics';

const manifestPath = __ENV.MANIFEST_PATH || '/workspace/perf/runtime/seed-manifest.json';
const manifest = JSON.parse(open(manifestPath));

export const scenarioErrorRate = new Rate('scenario_error_rate');
export const scenarioTransportErrorRate = new Rate('scenario_transport_error_rate');

const baseUrl = (__ENV.BASE_URL || 'http://ampmanager:16823').replace(/\/$/, '');
const promptChars = Number.parseInt(__ENV.PROMPT_CHARS || '1024', 10);
const activeKeyPool = resolveActiveKeyPool();
const sharedPrompt = buildPrompt(promptChars);

export function getBaseUrl() {
  return baseUrl;
}

export function performChatCompletion(latencyTrend) {
  const response = submitJson('/v1/chat/completions', {
    model: manifest.public_models.chat,
    stream: false,
    messages: [
      { role: 'user', content: sharedPrompt },
    ],
  });

  if (response && response.status >= 200 && response.status < 400) {
    latencyTrend.add(response.timings.duration);
  }
  return response;
}

export function performResponsesStream(firstByteTrend) {
  const response = submitJson('/v1/responses', {
    model: manifest.public_models.responses,
    stream: true,
    input: sharedPrompt,
  });

  if (response && response.status >= 200 && response.status < 400) {
    firstByteTrend.add(response.timings.waiting);
  }
  return response;
}

export function submitJson(path, payload) {
  let response;

  try {
    response = http.post(`${baseUrl}${path}`, JSON.stringify(payload), {
      headers: {
        Authorization: `Bearer ${pickApiKey()}`,
        'Content-Type': 'application/json',
      },
      responseType: 'text',
      timeout: __ENV.K6_HTTP_TIMEOUT || '180s',
    });
  } catch (error) {
    scenarioErrorRate.add(true);
    scenarioTransportErrorRate.add(true);
    return null;
  }

  const transportFailure = !response || response.status === 0;
  const applicationFailure = transportFailure || response.status >= 400 || !responseLooksComplete(path, response);

  scenarioTransportErrorRate.add(transportFailure);
  scenarioErrorRate.add(applicationFailure);

  return response;
}

export function iterationIndex() {
  return exec.scenario.iterationInTest;
}

function pickApiKey() {
  const poolSize = Math.min(activeKeyPool, manifest.api_keys.length);
  const index = Math.floor(Math.random() * poolSize);
  return manifest.api_keys[index].api_key;
}

function responseLooksComplete(path, response) {
  if (!response || response.status < 200 || response.status >= 400) {
    return false;
  }

  const body = `${response.body || ''}`;
  if (path === '/v1/responses') {
    return body.includes('event: response.completed') && body.includes('data: [DONE]');
  }

  return body.includes('"usage"');
}

function resolveActiveKeyPool() {
  const configured = Number.parseInt(__ENV.ACTIVE_KEY_POOL || `${manifest.api_keys.length}`, 10);
  if (Number.isNaN(configured) || configured < 1) {
    return manifest.api_keys.length;
  }
  return Math.min(configured, manifest.api_keys.length);
}

function buildPrompt(size) {
  const template = 'performance hot path prompt ';
  let value = '';
  while (value.length < size) {
    value += template;
  }
  return value.slice(0, size);
}
