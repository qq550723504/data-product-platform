def cold_topology(a):
    need([g['group'] for g in a['groups']] == list(p.GROUPS), 'finite preparation group membership differs')
    need(all((closed(inspect(cid)) for cid in a['owners'])), 'new owner is active')
    need(sha(PUBLIC / 'owner-descriptors.json') == a['owner_descriptors_sha256'], 'approved allocation descriptor drift')
    descriptors = load(PUBLIC / 'owner-descriptors.json')
    need(set(descriptors) == set(a['owners']) and len(a['owners']) == len(set(a['owners'])), 'allocation owner membership differs')
    for cid in a['owners']:
        x = inspect(cid)
        need(project_owner(x) == descriptors[cid] and (not x['HostConfig']['Privileged']) and (not x['HostConfig'].get('PortBindings')) and (x['HostConfig']['RestartPolicy']['Name'] == 'no') and (not any(x['NetworkSettings']['Ports'].values())), 'actual owner configuration differs from approved allocation')
        need(x['Config']['Labels'].get('lifecycle307.remaining') == p.EXECUTION and x['Config']['Labels'].get('lifecycle307.allocation') == a['allocation_nonce'], 'allocated owner label differs')
    expected_volumes = {v for g in a['groups'] for v in g['volumes'].values()}
    need(expected_volumes == {x['volume'] for x in a['resources'] if 'volume' in x}, 'complete new volume membership differs')
    for volume_name in expected_volumes:
        volume = json.loads(docker('volume', 'inspect', volume_name))[0]
        expected = {cid for cid, item in descriptors.items() for m in item['mounts'] if m['Type'] == 'volume' and m['Name'] == volume_name}
        mounted = docker('ps', '-aq', '--no-trunc', '--filter', 'volume=' + volume_name).decode('utf-8', 'strict').split()
        need(volume['Name'] == volume_name and volume['Driver'] == 'local' and (volume['Scope'] == 'local') and (not volume.get('Options')) and (volume['Labels'].get('lifecycle307.remaining') == p.EXECUTION) and (volume['Labels'].get('lifecycle307.allocation') == a['allocation_nonce']) and (len(mounted) == len(set(mounted))) and (set(mounted) == expected), 'new restore/queue/budget volume ownership or complete mount graph differs')
    return True
