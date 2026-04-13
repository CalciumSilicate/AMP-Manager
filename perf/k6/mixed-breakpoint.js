import { Trend } from 'k6/metrics';

import { performChatCompletion, performResponsesStream } from './lib.js';

const stepRpm = Number.parseInt(__ENV.STEP_RPM || '1200', 10);
const preAllocatedVUs = Number.parseInt(__ENV.K6_PREALLOCATED_VUS || '256', 10);
const maxVUs = Number.parseInt(__ENV.K6_MAX_VUS || '2000', 10);
const streamShare = Number.parseFloat(__ENV.MIXED_STREAM_SHARE || '0.20');

const mixedNonstreamLatency = new Trend('mixed_nonstream_latency_ms');
const mixedStreamFirstByte = new Trend('mixed_stream_first_byte_ms');

export const options = {
  discardResponseBodies: false,
  scenarios: {
    mixed: {
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
  if (Math.random() < streamShare) {
    performResponsesStream(mixedStreamFirstByte);
    return;
  }
  performChatCompletion(mixedNonstreamLatency);
}
