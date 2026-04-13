import { Trend } from 'k6/metrics';

import { performResponsesStream } from './lib.js';

const vus = Number.parseInt(__ENV.STEP_CONCURRENCY || '50', 10);

const firstByte = new Trend('stream_first_byte_ms');

export const options = {
  discardResponseBodies: false,
  scenarios: {
    stream: {
      executor: 'constant-vus',
      vus,
      duration: __ENV.STEP_DURATION || '10m',
    },
  },
};

export default function () {
  performResponsesStream(firstByte);
}
