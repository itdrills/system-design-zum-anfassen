import http from "k6/http";
import { check } from "k6";

const ZIEL = __ENV.ZIEL || "http://localhost";
const KUNDE = __ENV.KUNDE || "";             // landet im Header X-Api-Key, leer = ohne

export const options = {
  scenarios: {
    anlegen: {
      executor: "constant-arrival-rate",
      rate: Number(__ENV.RATE || 100), timeUnit: "1s",
      duration: __ENV.DAUER || "20s",
      preAllocatedVUs: 50, maxVUs: 2000,
    },
  },
};

export default function () {
  const headers = { "Content-Type": "application/json" };
  if (KUNDE) headers["X-Api-Key"] = KUNDE;
  const url = `https://example.org/tag12/${KUNDE || "anonym"}/${__VU}-${__ITER}-${Date.now()}`;
  const r = http.post(`${ZIEL}/api/links`, JSON.stringify({ url }), { headers, timeout: "10s" });
  check(r, { "201": (x) => x.status === 201, "429": (x) => x.status === 429, "503": (x) => x.status === 503 });
}
