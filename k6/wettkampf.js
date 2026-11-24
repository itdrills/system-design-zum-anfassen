import http from "k6/http";
import { check } from "k6";
import { SharedArray } from "k6/data";

const codes = new SharedArray("codes", () => open("./codes.txt").split("\n").filter(Boolean));
const ZIEL = __ENV.ZIEL || "http://localhost";
const DAUER = __ENV.DAUER || "30s";

// Der Abend: viele Weiterleitungen ohne Schlüssel, dazu ein Kunde, der Links anlegt
export const options = {
  scenarios: {
    klicks: {
      executor: "constant-arrival-rate", exec: "klick",
      rate: 1200, timeUnit: "1s", duration: DAUER, preAllocatedVUs: 50, maxVUs: 500,
    },
    anlegen: {
      executor: "constant-arrival-rate", exec: "anlegen",
      rate: 20, timeUnit: "1s", duration: DAUER, preAllocatedVUs: 10, maxVUs: 200,
    },
  },
};

export function klick() {
  const code = codes[Math.floor(Math.pow(Math.random(), 3) * codes.length)];
  const r = http.get(`${ZIEL}/${code}`, { redirects: 0, tags: { art: "klick" } });
  check(r, { "klick 302": (x) => x.status === 302 });
}

export function anlegen() {
  const url = `https://example.org/tag16/${__VU}-${__ITER}-${Date.now()}`;
  const r = http.post(`${ZIEL}/api/links`, JSON.stringify({ url }), {
    headers: { "Content-Type": "application/json", "X-Api-Key": "abend" },
    tags: { art: "anlegen" }, timeout: "10s",
  });
  check(r, { "anlegen 201": (x) => x.status === 201 });
}
