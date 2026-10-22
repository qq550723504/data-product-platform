// Explicit local synthetic trial; never adopts, restarts or removes other resources.
import {spawn} from 'node:child_process';
import {mkdir,writeFile,readFile,copyFile} from 'node:fs/promises';
import {createWriteStream} from 'node:fs';
import path from 'node:path';
import {randomBytes} from 'node:crypto';
if(process.env.FORK_HOST_LIVE_SYNTHETIC!=='1') throw Error('Explicit synthetic authorization gate required');
const root=process.env.HFRAW_OUTPUT, fork=process.env.HFRAW_FORK_WORKTREE;
if(!root||!fork) throw Error('New output directory and independent Fork worktree required');
await mkdir(root,{recursive:true});
try {await readFile(path.join(root,'target.json'));throw Error('Refusing to adopt existing target');}catch(e){if(e.code!=='ENOENT')throw e;}
const id='hfraw-'+Date.now(), image=id+':fork', pass=randomBytes(24).toString('hex');
const minio={AccessKey:'hfrawadmin',SecretKey:randomBytes(24).toString('hex'),SessionToken:''};
const target={id,image,fork,pg:id+'-pg',redis:id+'-redis',minio:id+'-minio',app:id+'-fork',worker:id+'-worker',network:id,ports:{fork:25510,pg:25511,minio:25512,api:25513,web:25514,gateway:25515},synthetic:true,coreEnabled:false,containers:[]};
await writeFile(path.join(root,'target.json'),JSON.stringify(target,null,2));
await writeFile(path.join(root,'minio-admin.json'),JSON.stringify(minio),{mode:0o600});
await writeFile(path.join(root,'postgres-dsn'),`postgres://postgres:${pass}@127.0.0.1:25511/hfraw_host?sslmode=disable`,{mode:0o600});
export async function command(cmd,args,log){return new Promise((res,rej)=>{const out=createWriteStream(path.join(root,log),{flags:'a'});const p=spawn(cmd,args,{windowsHide:true,stdio:['ignore','pipe','pipe']});p.stdout.pipe(out);p.stderr.pipe(out);p.on('error',rej);p.on('exit',code=>{out.end();code===0?res():rej(Error(`${cmd} failed (${code}); see ${log}`));});});}
const docker=(args,log='prepare.log')=>command('docker',args,log);
await docker(['image','inspect','annotation-engine-issue79-tests:20261021','--format','{{.Id}}'],'dependency-image.txt');
if(!(await readFile(path.join(root,'dependency-image.txt'),'utf8')).includes('sha256:bccb3b63cef1eb5d06aaee7cdc01278c0356f395f5dcba4e19b7465ab11af121')) throw Error('Dependency image identity changed');
const dependencyTag=id+':dependencies';
await docker(['tag','sha256:bccb3b63cef1eb5d06aaee7cdc01278c0356f395f5dcba4e19b7465ab11af121',dependencyTag]);
await docker(['build','-f',path.join(fork,'deploy/fork-host-live/Dockerfile'),'--build-arg',`DEPENDENCY_IMAGE=${dependencyTag}`,'-t',image,fork],'fork-image.log');
await docker(['network','create','--subnet',process.env.HFRAW_SUBNET??'10.254.233.0/28','--label','codex.owner=fork-host-live',id]);
const common=['--label','codex.owner=fork-host-live','--label',`codex.run=${id}`,'--network',id];
await docker(['run','-d','--name',target.pg,...common,'-e',`POSTGRES_PASSWORD=${pass}`,'-e','POSTGRES_DB=hfraw_fork','-p','127.0.0.1:25511:5432','-v',id+'-pg:/var/lib/postgresql/data','postgres:16-alpine']);target.containers.push(target.pg);
await docker(['run','-d','--name',target.redis,...common,'-v',id+'-redis:/data','redis:7-alpine','redis-server','--appendonly','yes']);target.containers.push(target.redis);
await docker(['run','-d','--name',target.minio,...common,'-p','127.0.0.1:25512:9000','-e',`MINIO_ROOT_USER=${minio.AccessKey}`,'-e',`MINIO_ROOT_PASSWORD=${minio.SecretKey}`,'-v',id+'-minio:/data','--entrypoint','/usr/local/bin/minio','dpp-restored-oci-minio:original-8c712a2fe14b096e','server','/data']);target.containers.push(target.minio);
await writeFile(path.join(root,'target.json'),JSON.stringify(target,null,2));
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
for(let n=0;;n++){try{await docker(['exec',target.pg,'pg_isready','-U','postgres','-d','hfraw_fork']);break;}catch(e){if(n>45)throw e;await sleep(500);}}
await docker(['exec',target.pg,'createdb','-U','postgres','hfraw_host']);
const base={DJANGO_DB:'postgresql',POSTGRE_HOST:target.pg,POSTGRE_NAME:'hfraw_fork',POSTGRE_USER:'postgres',POSTGRE_PASSWORD:pass,LABEL_STUDIO_BASE_DATA_DIR:'/trial/data',APPROVED_EXPORT_ROOT:'/trial/exports',APPROVED_EXPORT_REDIS_URL:`redis://${target.redis}:6379/0`,FORK_HOST_LIVE_SYNTHETIC:'1',TEST_ENVIRONMENT:'1',COLLECT_ANALYTICS:'0',SENTRY_RATE:'0',SENTRY_DSN:'',FRONTEND_SENTRY_DSN:'',LATEST_VERSION_CHECK:'false',DEBUG:'false',LABEL_STUDIO_ENABLE_LEGACY_API_TOKEN:'true',HOSTNAME:'http://127.0.0.1:25510',FRONTEND_HOSTNAME:'http://127.0.0.1:25510',FRONTEND_HMR:'false',ANNOTATION_FIXED_PACKAGES_ENABLED:'false'};
const envfile=path.join(root,'fork.env');
await writeFile(envfile,Object.entries(base).map(([k,v])=>`${k}=${v}`).join('\n'),{mode:0o600});
const forkPrivate=path.join(root,'fork-private');await mkdir(forkPrivate);
const mounts=['-v',`${forkPrivate}:/trial/private`,'-v',id+'-fork-data:/trial/data','-v',id+'-exports:/trial/exports'];
await docker(['run','--rm',...common,'--env-file',envfile,...mounts,image,'migrate','--noinput'],'fork-migrate.log');
await docker(['run','--rm',...common,'--env-file',envfile,...mounts,image,'seed_fork_host_live','--output','/trial/private/fork-fixture.json'],'fork-seed.log');
for(const name of ['fork-fixture.json','fork-policy.json'])await copyFile(path.join(forkPrivate,name),path.join(root,name));
base.APPROVED_EXPORT_POLICY=await readFile(path.join(root,'fork-policy.json'),'utf8');
await writeFile(envfile,Object.entries(base).map(([k,v])=>`${k}=${v}`).join('\n'),{mode:0o600});
const runtimeMounts=['-v',`${forkPrivate}:/trial/private:ro`,'-v',id+'-fork-data:/trial/data','-v',id+'-exports:/trial/exports'];
await docker(['run','-d','--name',target.app,...common,'--env-file',envfile,...runtimeMounts,'-p','127.0.0.1:25510:8080',image,'runserver','0.0.0.0:8080','--noreload']);target.containers.push(target.app);
await docker(['run','-d','--name',target.worker,...common,'--env-file',envfile,...runtimeMounts,image,'rqworker','approved_exports']);target.containers.push(target.worker);
await writeFile(path.join(root,'target.json'),JSON.stringify(target,null,2));
console.log('Isolated Fork/PostgreSQL/Redis/MinIO prepared; private target manifest written.');
