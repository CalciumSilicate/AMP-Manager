import { sleep } from 'k6';
import { Trend } from 'k6/metrics';

import { iterationIndex, performChatCompletion, performResponsesStream } from './lib.js';

const smokeLatency = new Trend('amp_req_latency_ms');
const smokeStreamFirstByte = new Trend('stream_first_byte_ms');

export const options = {
  discardResponseBodies: false,
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 2,
      duration: __ENV.SMOKE_DURATION || '1m',
    },
  },
};

export default function () {
  if (iterationIndex() % 2 === 0) {
    performChatCompletion(smokeLatency);
  } else {
    performResponsesStream(smokeStreamFirstByte);
  }
  sleep(1);
}
