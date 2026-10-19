class Allocation:

    def __init__(self):
        self.owners = []
        self.resources = []
        self.pending = []
        self.nonce = str(uuid.uuid4())

    def create(self, role, group, args):
        name = str(args[args.index('--name') + 1])
        self.pending.append(name)
        save(PUBLIC / ('allocation-intent-%03d.json' % len(self.pending)), {'role': role, 'group': group, 'name': name, 'execution': EXECUTION, 'argv': list(map(str, args))})
        cid = docker('create', '--pull', 'never', '--label', 'lifecycle307.remaining=' + EXECUTION, '--label', 'lifecycle307.allocation=' + self.nonce, *args).decode('utf-8', 'strict').strip()
        need(len(cid) == 64 and all((c in '0123456789abcdef' for c in cid)), 'Docker create did not return one full CID')
        self.owners.append(cid)
        descriptor = {'role': role, 'group': group, 'cid': cid, 'argv': list(map(str, args))}
        save(PUBLIC / ('allocation-%03d.json' % len(self.owners)), descriptor)
        return cid

    def volume(self, name):
        existing = docker('volume', 'ls', '--filter', 'name=^' + name + '$', '--format', '{{.Name}}').decode('utf-8', 'strict').split()
        need(not existing, 'same-name retained volume exists; never adopt or start it')
        number = len(self.resources) + 1
        save(PUBLIC / ('resource-intent-%03d.json' % number), {'volume': name, 'execution': EXECUTION, 'allocation': self.nonce, 'preexisting': False})
        value = docker('volume', 'create', '--driver', 'local', '--label', 'lifecycle307.remaining=' + EXECUTION, '--label', 'lifecycle307.allocation=' + self.nonce, name).decode('utf-8', 'strict').strip()
        self.resources.append({'volume': name, 'allocation': self.nonce})
        volume = json.loads(docker('volume', 'inspect', name))[0]
        need(value == name == volume['Name'] and volume['Driver'] == 'local' and (volume['Scope'] == 'local') and (not volume.get('Options')) and (volume['Labels'].get('lifecycle307.remaining') == EXECUTION) and (volume['Labels'].get('lifecycle307.allocation') == self.nonce) and (not docker('ps', '-aq', '--no-trunc', '--filter', 'volume=' + name).strip()), 'new volume owner/driver/options or empty mount graph differs')
        save(PUBLIC / ('resource-%03d.json' % number), {'volume': name, 'driver': volume['Driver'], 'scope': volume['Scope'], 'labels': volume['Labels'], 'mounted_cids': []})
        return name

    def network(self, name, subnet):
        need(not docker('network', 'ls', '--filter', 'name=^' + name + '$', '--format', '{{.Name}}').strip(), 'same-name retained network exists')
        number = len(self.resources) + 1
        save(PUBLIC / ('resource-intent-%03d.json' % number), {'network_name': name, 'subnet': subnet, 'execution': EXECUTION, 'allocation': self.nonce, 'preexisting': False})
        ident = docker('network', 'create', '--driver', 'bridge', '--internal', '--subnet', subnet, '--label', 'lifecycle307.remaining=' + EXECUTION, '--label', 'lifecycle307.allocation=' + self.nonce, name).decode('utf-8', 'strict').strip()
        self.resources.append({'network': ident, 'name': name, 'subnet': subnet, 'allocation': self.nonce})
        n = json.loads(docker('network', 'inspect', ident))[0]
        need(len(ident) == 64 and n['Id'] == ident and (n['Name'] == name) and (n['Driver'] == 'bridge') and (n['Scope'] == 'local') and n['Internal'] and (not n.get('Containers')) and (n['Labels'].get('lifecycle307.remaining') == EXECUTION) and (n['Labels'].get('lifecycle307.allocation') == self.nonce), 'new isolated network ownership differs')
        save(PUBLIC / ('resource-%03d.json' % number), self.resources[-1])
        return ident

    def helper(self, group, name, mode, folder, binary, netns, output, *, budget=None, operator=False):
        folder.mkdir(parents=True, exist_ok=True)
        output.mkdir(parents=True, exist_ok=True)
        entry = '/run/operator' if operator else '/run/reader'
        args = ['--name', name, '--user', '10001:10001', '--network', 'container:' + netns, '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--tmpfs', '/tmp', '--tmpfs', '/var/lib/postgresql/data:size=1m', '--mount', f'type=bind,source={folder},target=/run/private,readonly', '--mount', f'type=bind,source={binary},target={entry},readonly', '--mount', f'type=bind,source={output},target=/run/output']
        if budget:
            args += ['--mount', 'type=volume,source=' + budget + ',target=/run/budget']
        args += ['--entrypoint', entry, p1.PG, '-stage' if operator else '-mode', mode]
        return self.create(mode, group, args)
