// Fake EasyPanel panel for E2E: mirrors the real tRPC/REST surface used by
// easypanel-migrate and stores an admin user (with apiToken) in an LMDB store
// at the same path the real panel uses, so the docker-exec LMDB token
// extraction path is exercised verbatim.
const http = require("http");
const { open } = require("/app/node_modules/lmdb");

const DB_PATH = "/etc/easypanel/data/data.mdb";
const TOKEN = process.env.PANEL_TOKEN || "faketoken";
const SEED = (process.env.PANEL_SEED || "").trim(); // "project:service,project2"

// Match the real panel's on-disk shape: JSON text with a short binary
// length/version prefix, stored under a binary-encoded store. This exercises
// the extractor's prefix-skip + JSON-parse path exactly as in production.
const db = open({ path: DB_PATH, encoding: "binary" });

// In-memory project/service state, seeded from env, persisted lightly to LMDB.
const projects = new Map(); // name -> Set(serviceName)
function ensureProject(name) {
  if (!projects.has(name)) projects.set(name, new Set());
  return projects.get(name);
}
for (const spec of SEED.split(",").map((s) => s.trim()).filter(Boolean)) {
  const [p, s] = spec.split(":");
  const set = ensureProject(p);
  if (s) set.add(s);
}

// Seed the admin user + apiToken as prefixed JSON bytes, mirroring the real
// panel record (outer {json,meta}; inner user object holds admin + apiToken).
const record = {
  json: {
    admin: true,
    createdAt: new Date().toISOString(),
    email: "admin@example.com",
    id: "admin000000000000000000000",
    password: "$2b$09$doesnotmatterfakehashvalueforintegrationtesting..",
    apiToken: TOKEN,
  },
  meta: { values: { createdAt: ["Date"] }, v: 1 },
};
// Binary length/version prefix before the JSON payload, like the real store.
// A leading spurious '{' (0x7b) reproduces a real panel build where a naive
// first-'{' skip mis-parses the record and yields an empty token; the extractor
// must scan past it to the real {"json":...} payload.
const payload = Buffer.concat([Buffer.from([0x7b, 0x08, 0x01]), Buffer.from(JSON.stringify(record), "utf8")]);
db.putSync("users:admin000000000000000000000", payload);

function readBody(req) {
  return new Promise((resolve) => {
    let b = "";
    req.on("data", (c) => (b += c));
    req.on("end", () => {
      try { resolve(JSON.parse(b || "{}")); } catch { resolve({}); }
    });
  });
}
function send(res, status, obj) {
  res.writeHead(status, { "content-type": "application/json" });
  res.end(JSON.stringify(obj));
}
function servicesOf(name) {
  return [...ensureProject(name)].map((s) => ({ name: s, type: "app", projectName: name }));
}

const server = http.createServer(async (req, res) => {
  const auth = req.headers["authorization"] || "";
  const url = req.url.split("?")[0];
  const body = await readBody(req);
  // tRPC queries send input in the ?input= query param (GET); mutations send it
  // in the body (POST). The real client negotiates GET/POST per panel build, so
  // accept both here.
  let queryInput = {};
  try {
    const raw = new URL(req.url, "http://x").searchParams.get("input");
    if (raw) queryInput = JSON.parse(raw).json || {};
  } catch (e) {}
  const input = (body && body.json) || queryInput || {};

  if (url === "/api/trpc/projects.listProjects") {
    return send(res, 200, { json: [...projects.keys()].map((name) => ({ name })) });
  }
  if (url === "/api/trpc/projects.inspectProject") {
    const name = input.projectName;
    if (!projects.has(name)) {
      return send(res, 404, { json: { code: "NOT_FOUND", status: 404, message: "Project not found." } });
    }
    return send(res, 200, { json: { project: { name }, services: servicesOf(name) } });
  }
  if (url === "/api/trpc/projects.createProject") {
    ensureProject(input.name);
    return send(res, 200, { json: {} });
  }
  if (url === "/api/trpc/projects.destroyProject") {
    projects.delete(input.name);
    return send(res, 200, { json: {} });
  }
  if (url === "/api/trpc/services.app.destroyService") {
    ensureProject(input.projectName).delete(input.serviceName);
    return send(res, 200, { json: {} });
  }
  if (url === "/api/migrate-service") {
    // Push the service into the remote panel named by remoteEasypanelUrl.
    // We simulate by creating the service on THIS panel's remote target set,
    // but for the test the source panel forwards to the remote by HTTP.
    const remoteURL = body.remoteEasypanelUrl;
    const remoteToken = body.remoteApiToken;
    try {
      // create service on remote via its tRPC-ish create (use inspect+seed).
      await forwardCreate(remoteURL, remoteToken, body.remoteProjectName, body.remoteServiceName);
      return send(res, 200, { success: true });
    } catch (e) {
      return send(res, 500, { message: String(e && e.message || e), success: false });
    }
  }
  send(res, 404, { error: "Not found" });
});

// forwardCreate registers the migrated service on the remote panel so the E2E
// test can assert it appeared there.
function forwardCreate(remoteURL, token, project, service) {
  return new Promise((resolve, reject) => {
    const u = new URL(remoteURL + "/api/trpc/__seedService");
    const payload = JSON.stringify({ json: { projectName: project, serviceName: service } });
    const opts = {
      method: "POST",
      hostname: u.hostname,
      port: u.port || 3000,
      path: u.pathname,
      headers: { "content-type": "application/json", authorization: "Bearer " + token, "content-length": Buffer.byteLength(payload) },
    };
    const r = http.request(opts, (resp) => {
      resp.resume();
      resp.on("end", () => (resp.statusCode < 300 ? resolve() : reject(new Error("remote seed http " + resp.statusCode))));
    });
    r.on("error", reject);
    r.write(payload);
    r.end();
  });
}

// __seedService lets a peer panel register a migrated service (test-only hook).
const origHandler = server.listeners("request")[0];
server.removeAllListeners("request");
server.on("request", async (req, res) => {
  if (req.url.split("?")[0] === "/api/trpc/__seedService") {
    const body = await readBody(req);
    const input = (body && body.json) || {};
    ensureProject(input.projectName).add(input.serviceName);
    return send(res, 200, { json: {} });
  }
  return origHandler(req, res);
});

server.listen(3000, "0.0.0.0", () => console.log("fake easypanel on :3000 token=" + TOKEN));
