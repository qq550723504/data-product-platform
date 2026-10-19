"""Protected runtime wiring, without issuing credentials or touching Docker."""
import copy
import tempfile
from pathlib import Path
import unittest
from unittest.mock import patch

import runtime as r
import resume as c


class WorkspaceStorageContracts(unittest.TestCase):
    def test_protected_hosts_receive_only_profile_reference(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)
            r.write(p/'config.json',{'DSN':'private-dsn','Profile':{'StorageEndpoint':'127.0.0.1:1234'}})
            s={'private':str(p),'ports':{'api':1235,'redis':1236},'workspaceObjectProfile':str(p/'bound-profile.json')}
            with patch.dict(r.os.environ,{'OBJECT_STORAGE_ACCESS_KEY':'inherited-root','OBJECT_STORAGE_SECRET_KEY':'inherited-secret'}):
                env=r.core_environment(s)
            self.assertNotIn('OBJECT_STORAGE_ACCESS_KEY',env)
            self.assertNotIn('OBJECT_STORAGE_SECRET_KEY',env)
            self.assertEqual(env['OBJECT_STORAGE_WORKSPACE_PROFILE'],s['workspaceObjectProfile'])

    def test_profile_renewal_retains_identity_old_profile_and_policy(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d);old=p/'old.json';attempt=p/'attempt';attempt.mkdir()
            original={'Scope':{'WorkspaceID':'same-workspace','ActorID':'same-actor','NamespaceID':'same-namespace','ProviderInstanceID':'same-provider','IncarnationID':'same-incarnation','Bucket':'raw','PrincipalRef':'same-principal','ExpiresAt':'old-expiry'},'Endpoint':'same-endpoint','CredentialFile':'old-credential'}
            r.write(old,original);raw=old.read_bytes()
            s={'workspaceId':'same-workspace','actorId':'same-actor','workspaceObjectProfile':str(old),'privateHashes':{str(old):r.digest(raw)}}
            c.refresh_workspace_profile(s,attempt,attempt/'credential.json','new-expiry')
            actual=r.read(s['workspaceObjectProfile']);expected=copy.deepcopy(original)
            expected['Scope']['ExpiresAt']='new-expiry';expected['CredentialFile']=str(attempt/'credential.json')
            self.assertEqual(actual,expected);self.assertEqual(old.read_bytes(),raw)
            with self.assertRaises(FileExistsError):c.refresh_workspace_profile(s,attempt,attempt/'credential.json','new-expiry')

    def test_profile_foreign_actor_or_workspace_refused(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d);old=p/'old.json';r.write(old,{'Scope':{'WorkspaceID':'foreign','ActorID':'same'}})
            with self.assertRaisesRegex(r.Refused,'identity drift'):
                c.refresh_workspace_profile({'workspaceId':'same','actorId':'same','workspaceObjectProfile':str(old)},p,p/'credential','future')

    def test_existing_pre330_artifact_not_silently_activated(self):
        s={'runId':'retained'}
        with patch.object(r,'read') as read:
            c.refresh_workspace_profile(s,Path('unused'),Path('unused'),'future')
        self.assertEqual(s,{'runId':'retained'});read.assert_not_called()


if __name__=='__main__':unittest.main()
