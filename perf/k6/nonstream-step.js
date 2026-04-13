import { Trend } from 'k6/metrics';

import { performChatCompletion } from './lib.js';

const stepRpm = Number.parseInt(__ENV.STEP_RPM || '600', 10);
const preAllocatedVUs = Number.parseInt(__ENV.K6_PREALLOCATED_VUS || '256', 10);
const maxVUs = Number.parseInt(__ENV.K6_MAX_VUS || '2000', 10);

const nonstreamLatency = new Trend('amp_req_latency_ms');

export const options = {
  discardResponseBodies: false,
  scenarios: {
    nonstream: {
      executor: 'constant-arrival-rate',
      rate: stepRpm,
      timeUnit: '1m',
      duration: __ENV.STEP_DURATION || '5m',
      preAllocatedVUs,
      maxVUs,
    },
  },
};

export default function () {
  performChatCompletion(nonstreamLatency);
}
