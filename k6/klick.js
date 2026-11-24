import http from "k6/http";
import { check } from "k6";
import { SharedArray } from "k6/data";

const codes = new SharedArray("codes", () => open("./codes.txt").split("\n").filter(Boolean));
const ZIEL = __ENV.ZIEL || "http://localhost";

export const options = {
  scenarios: {
    klicks: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 1500), timeUnit: "1s",
      duration: __ENV.DAUER || "30s",
      preAllocatedVUs: 50, maxVUs: 500,
    },
  },
};

// nur Weiterleitungen, beliebte Links viel häufiger als seltene
export default function () {
  const code = codes[Math.floor(Math.pow(Math.random(), 3) * codes.length)];
  const r = http.get(`${ZIEL}/${code}`, { redirects: 0 });
  check(r, { "302": (x) => x.status === 302 });
}
