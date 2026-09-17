import test from "node:test";
import assert from "node:assert/strict";
import { localEndpoint, parseCommand, projectName } from "./demo.mjs";
for (const command of ["up", "down", "advance", "status", "verify", "logs", "doctor"]) {
  test(`accept only exact ${command}`, () => assert.equal(parseCommand([command]), command));
}
for (const args of [[], ["reset"], ["reset", "--yes"], ["up", "--host=remote"], ["down", "--volumes"], ["up;rm -rf /"], ["status", "extra"]]) {
  test(`reject unsafe/ambiguous ${JSON.stringify(args)}`, () => assert.throws(() => parseCommand(args)));
}
test("reset needs exact explicit confirmation", () => assert.equal(parseCommand(["reset", "--confirm=DELETE_DEMO_DATA"]), "reset"));
test("project is stable within a checkout and separate across paths", () => {
  assert.equal(projectName("/a"), projectName("/a"));
  assert.notEqual(projectName("/a"), projectName("/b"));
  assert.match(projectName("/a"), /^dpp-demo-[a-f0-9]{12}$/);
});
for (const endpoint of ["unix:///var/run/docker.sock", "npipe:////./pipe/docker_engine"]) {
  test(`allow local ${endpoint}`, () => assert.equal(localEndpoint(endpoint), true));
}
for (const endpoint of [null, "ssh://prod", "tcp://prod:2375", "tcp://127.0.0.1:2375", "https://prod", ""]) {
  test(`refuse remote or unsupported ${endpoint}`, () => assert.equal(localEndpoint(endpoint), false));
}
