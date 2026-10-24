import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve, dirname, join } from "node:path";

// Disposable process-only trust. This never modifies an OS/browser CA store.
const directory = mkdtempSync(join(tmpdir(), "dpp-iam-test-"));
try {
  const cert = join(directory, "certificate.pem"),
    key = join(directory, "private-key.pem");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-days",
      "1",
      "-keyout",
      key,
      "-out",
      cert,
      "-subj",
      "/CN=localhost",
      "-addext",
      "subjectAltName=DNS:localhost,IP:127.0.0.1",
    ],
    { stdio: "ignore" },
  );
  const result = spawnSync(
    process.execPath,
    ["--test", "tests/iam.test.mjs","tests/acceptance-identity.test.mjs","tests/acceptance-basis.test.mjs"],
    {
      stdio: "inherit",
      env: {
        ...process.env,
        NODE_EXTRA_CA_CERTS: cert,
        IAM_FIXTURE_CERT: cert,
        IAM_FIXTURE_KEY: key,
      },
    },
  );
  process.exitCode = result.status ?? 1;
} finally {
  if (
    dirname(resolve(directory)) !== resolve(tmpdir()) ||
    !directory.split(/[\\/]/).at(-1).startsWith("dpp-iam-test-")
  )
    throw new Error("unsafe cleanup target");
  rmSync(directory, { recursive: true, force: true });
}
