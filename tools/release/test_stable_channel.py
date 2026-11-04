#!/usr/bin/env python3
"""Offline failure-injection tests for the stable-channel promotion helper."""
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.dont_write_bytecode=True
module_path=Path(os.environ.get('AEGO_STAGE_SCRIPT',Path(__file__).with_name('stable-channel.py')))
spec=importlib.util.spec_from_file_location('stable_channel',module_path)
stage=importlib.util.module_from_spec(spec)
spec.loader.exec_module(stage)
OLD='1'*40
NEW='2'*40

class FakeAPI:
    def __init__(self,old_files,new_files,mode='',first=False):
        self.mode=mode
        self.calls=[]
        self.next_id=3
        self.releases={2:{'id':2,'tag_name':'v0.12.0','draft':False,'prerelease':False,'files':dict(new_files)}}
        self.refs={'v0.12.0':NEW}
        if not first:
            self.releases[1]={'id':1,'tag_name':'stable','draft':False,'prerelease':True,
                              'name':'Stable channel','body':'prior release','files':dict(old_files)}
            self.refs['stable']=OLD
        self.fired=set()

    def fail_once(self,name):
        if self.mode==name and name not in self.fired:
            self.fired.add(name)
            raise stage.ChannelError('injected '+name)

    def by_tag(self,tag):
        return next((r for r in self.releases.values() if r['tag_name']==tag),None)

    def get_release(self,tag):
        self.calls.append(('get',tag))
        return self.by_tag(tag)

    def get_ref(self,tag):
        return self.refs.get(tag)

    def assets(self,rid):
        return [{'name':n,'size':len(v)} for n,v in self.releases[rid]['files'].items()]

    def download(self,tag,name):
        return self.by_tag(tag)['files'][name]

    def upload(self,rid,name,path):
        r=self.releases[rid]
        if name=='release.json.sig':
            self.fail_once('upload-backup' if r['tag_name'].startswith('stable-backup-') else 'upload-candidate')
        r['files'][name]=path.read_bytes()

    def json(self,method,path,body=None,missing=False):
        self.calls.append((method,path))
        if method=='POST' and path=='/releases':
            rid=self.next_id;self.next_id+=1
            result=dict(body,id=rid,files={})
            self.releases[rid]=result
            return result
        if method=='DELETE' and path.startswith('/releases/'):
            self.fail_once('delete-release')
            rid=int(path.rsplit('/',1)[1]);del self.releases[rid]
            self.fail_once('delete-response-lost')
            return None
        if method=='DELETE' and path=='/tags/stable':
            self.fail_once('delete-tag')
            del self.refs['stable'];return None
        if method=='PATCH' and path.startswith('/releases/'):
            rid=int(path.rsplit('/',1)[1]);r=self.releases[rid]
            candidate=r['tag_name'].startswith('stable-candidate-')
            if candidate:
                self.fail_once('publish')
                if self.mode=='rollback-fail':
                    raise stage.ChannelError('injected candidate failure')
            elif self.mode=='rollback-fail':
                raise stage.ChannelError('injected backup failure')
            r.update(body)
            if not r['draft']:
                self.refs[r['tag_name']]=r['target_commitish']
            if candidate: self.fail_once('publish-response-lost')
            return r
        raise AssertionError((method,path,body))

class ChannelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.work=tempfile.TemporaryDirectory(prefix='aego-release-unit-')
        cls.root=Path(cls.work.name)
        private=cls.root/'fixture-key.der'
        # Fixed test-only Ed25519 seed; never used for a published release.
        private.write_bytes(bytes.fromhex('302e020100300506032b657004220420')+bytes(range(32)))
        public=subprocess.check_output(['openssl','pkey','-inform','DER','-in',str(private),'-pubout','-outform','DER'])
        cls.public=base64.b64encode(public[-32:]).decode()
        cls.private=private

    @classmethod
    def tearDownClass(cls):
        cls.work.cleanup()

    def sign(self,data):
        msg=self.root/'msg';msg.write_bytes(data)
        signature=subprocess.check_output(['openssl','pkeyutl','-sign','-inkey',str(self.private),
                                           '-keyform','DER','-rawin','-in',str(msg)])
        return base64.b64encode(signature)+b'\n'

    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory(prefix='aego-channel-case-')
        self.scratch=Path(self.tmp.name)
        old=b'{"version":"0.11.0","channel":"stable"}\n'
        new=b'{"version":"0.12.0","channel":"stable"}\n'
        self.old={'release.json':old,'release.json.sig':self.sign(old)}
        self.new={'release.json':new,'release.json.sig':self.sign(new)}
        self.record={}
        self.save=lambda phase:self.record.update(phase=phase)

    def tearDown(self):
        self.tmp.cleanup()

    def run_channel(self,api,key=None):
        return stage.publish_channel(api,'v0.12.0',NEW,self.new,key or self.public,
                                     self.scratch,self.record,self.save)

    def assert_stable(self,api,sha,files):
        self.assertEqual(api.get_ref('stable'),sha)
        release=api.get_release('stable')
        self.assertFalse(release['draft'])
        self.assertEqual(release['files'],files)

    def test_success_and_first_channel(self):
        for first in (False,True):
            with self.subTest(first=first):
                api=FakeAPI(self.old,self.new,first=first)
                self.run_channel(api)
                self.assertEqual(self.record['phase'],'published')
                self.assert_stable(api,NEW,self.new)
                if not first:
                    backup=self.record['backup']
                    self.assertEqual(api.releases[backup['id']]['files'],self.old)
                    self.assertEqual(api.releases[backup['id']]['body'],'prior release')

    def test_staging_failures_leave_original_channel_untouched(self):
        for mode in ('upload-backup','upload-candidate','delete-release'):
            with self.subTest(mode=mode):
                api=FakeAPI(self.old,self.new,mode)
                with self.assertRaises(stage.ChannelError): self.run_channel(api)
                self.assert_stable(api,OLD,self.old)
                self.assertEqual(api.get_release('stable')['id'],1)

    def test_failed_cutover_restores_backup(self):
        for mode in ('delete-response-lost','delete-tag','publish'):
            with self.subTest(mode=mode):
                api=FakeAPI(self.old,self.new,mode)
                with self.assertRaises(stage.ChannelError): self.run_channel(api)
                self.assertEqual(self.record['phase'],'restored')
                self.assert_stable(api,OLD,self.old)
                self.assertEqual(api.get_release('stable')['id'],self.record['backup']['id'])

    def test_response_loss_after_publication_does_not_rollback(self):
        api=FakeAPI(self.old,self.new,'publish-response-lost')
        self.run_channel(api)
        self.assertEqual(self.record['phase'],'published-after-response-loss')
        self.assert_stable(api,NEW,self.new)

    def test_failed_rollback_is_not_reported_as_success(self):
        api=FakeAPI(self.old,self.new,'rollback-fail')
        with self.assertRaises(stage.ChannelError): self.run_channel(api)
        self.assertEqual(self.record['phase'],'manual-recovery-required')
        self.assertIn('recovery_error',self.record)
        self.assertIsNone(api.get_release('stable'))
        self.assertEqual(api.releases[self.record['backup']['id']]['files'],self.old)

    def test_invalid_signature_fails_before_remote_requests(self):
        api=FakeAPI(self.old,self.new)
        with self.assertRaises(stage.ChannelError): self.run_channel(api,base64.b64encode(b'x'*32).decode())
        self.assertEqual(api.calls,[])
        self.assert_stable(api,OLD,self.old)

    def test_prior_channel_with_wrong_signature_is_not_replaced(self):
        old=dict(self.old,**{'release.json.sig':base64.b64encode(b'x'*64)+b'\n'})
        api=FakeAPI(old,self.new)
        with self.assertRaises(stage.ChannelError): self.run_channel(api)
        self.assert_stable(api,OLD,old)
        self.assertFalse(any(method in ('POST','DELETE','PATCH') for method,_ in api.calls))

    def test_unexpected_old_asset_is_not_deleted(self):
        old=dict(self.old,**{'unrelated.txt':b'keep this asset'})
        api=FakeAPI(old,self.new)
        with self.assertRaises(stage.ChannelError): self.run_channel(api)
        self.assert_stable(api,OLD,old)
        self.assertFalse(any(method in ('POST','DELETE','PATCH') for method,_ in api.calls))

    def test_concurrent_stable_change_stops_cutover(self):
        class ChangedAPI(FakeAPI):
            seen=0
            def get_release(self,tag):
                result=super().get_release(tag)
                if tag=='stable':
                    self.seen+=1
                    if self.seen==2: self.refs['stable']='3'*40
                return result
        api=ChangedAPI(self.old,self.new)
        with self.assertRaises(stage.ChannelError): self.run_channel(api)
        self.assertEqual(api.get_release('stable')['id'],1)
        self.assertEqual(api.get_release('stable')['files'],self.old)
        self.assertFalse(any(method=='DELETE' for method,_ in api.calls))

    def test_manifest_signature_and_asset_hashes(self):
        base='https://example.invalid/v0.12.0'
        parts=[('linux','tarball','aego-linux.tar.gz',b'archive'),('windows','installer','aego-setup.exe',b'MZfixture')]
        assets=[]
        for osname,kind,name,data in parts:
            (self.scratch/name).write_bytes(data)
            assets.append({'os':osname,'arch':'amd64','kind':kind,'url':base+'/'+name,
                           'sha256':hashlib.sha256(data).hexdigest(),'size':len(data)})
        data=(json.dumps({'version':'0.12.0','channel':'stable','assets':assets})+'\n').encode()
        (self.scratch/'release.json').write_bytes(data)
        (self.scratch/'release.json.sig').write_bytes(self.sign(data))
        stage.verify_candidate(self.scratch,'0.12.0','stable',base,self.public,self.scratch,True)
        (self.scratch/'aego-setup.exe').write_bytes(b'MZtampered')
        with self.assertRaises(stage.ChannelError):
            stage.verify_candidate(self.scratch,'0.12.0','stable',base,self.public,self.scratch,True)

if __name__ == '__main__':
    unittest.main(verbosity=2)
