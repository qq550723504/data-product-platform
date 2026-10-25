"""Isolated tracked-source build context; no retained mounts or credentials."""
import hashlib,json,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
context=Path.home()/'.codex/fixture-private/ci-portability-fix01/browser-image01';context.mkdir(exist_ok=False)
names=subprocess.check_output(['git','-c','core.longpaths=true','ls-files','--','apps/web','tests/browser'],cwd=root).decode().splitlines();files={}
for name in names:
 source=root/name;raw=source.read_bytes();target=context/name;target.parent.mkdir(parents=True,exist_ok=True)
 with target.open('xb') as stream:stream.write(raw)
 files[name]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
dockerfile='''FROM node:22-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d AS node22
FROM mcr.microsoft.com/playwright:v1.63.0-noble@sha256:eff16c30e6f3f4af0a03fa4b706120d5e9b0891c344a27d64559aff5900a4a27
COPY --from=node22 /usr/local/bin/node /usr/local/bin/node
ENV CI=true NEXT_TELEMETRY_DISABLED=1 POC_WORKSPACE_ID="" POC_ENABLE_REVIEW_ACTIONS=false POC_ENABLE_RELEASE_ACTIONS=false HOME=/tmp PLAYWRIGHT_HTML_OUTPUT_DIR=/out/report
WORKDIR /work
COPY . /work
RUN npm --prefix apps/web ci --no-audit --no-fund && npm --prefix tests/browser install --no-audit --no-fund && npm --prefix apps/web run build && mkdir /work/.data && chown -R 10001:10001 /work
USER 10001:10001
CMD ["sh","-ec","node --version; npm --version; npm --prefix tests/browser run test:fixture; node --test apps/web/tests/release-readiness.test.mjs; cd tests/browser; node node_modules/playwright/cli.js test --retries=0 --output=/out/test-results"]
'''
with (context/'Dockerfile').open('x',encoding='utf-8',newline='\n') as stream:stream.write(dockerfile)
with (out/'linux-browser-build-input01.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(dict(context=str(context),files=files,dockerfile=dockerfile,retained_source_start=0),stream,indent=2)
print(str(context))
