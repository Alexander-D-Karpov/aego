#!/usr/bin/env python3
"""Verify release assets and stage both stable-channel files before cutover.

The production API must use HTTPS. A Unix-socket-only test transport is
available only in the private synthetic Gitea fixture. This script never
prints credentials and refuses to remove unexpected releases or tags.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
import tempfile
from urllib.parse import quote, urlsplit

FILES = ('release.json','release.json.sig')

class ChannelError(Exception):
    pass

def digest(data):
    return hashlib.sha256(data).hexdigest()

def verify_signature(data, signature, key, scratch):
    try:
        public = base64.b64decode(key.strip(),validate=True)
        raw = base64.b64decode(signature.strip(),validate=True)
    except ValueError as e:
        raise ChannelError('invalid Ed25519 key or signature encoding') from e
    if len(public) != 32 or len(raw) != 64:
        raise ChannelError('invalid Ed25519 key or signature size')
    (scratch/'key.der').write_bytes(bytes.fromhex('302a300506032b6570032100')+public)
    (scratch/'message').write_bytes(data)
    (scratch/'signature').write_bytes(raw)
    try:
        result = subprocess.run(['openssl','pkeyutl','-verify','-pubin','-keyform','DER',
            '-inkey',str(scratch/'key.der'),'-rawin','-in',str(scratch/'message'),
            '-sigfile',str(scratch/'signature')],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    except OSError as e:
        raise ChannelError('openssl is required for Ed25519 signature verification') from e
    if result.returncode:
        raise ChannelError('release signature does not match the update key')
    return digest(public)

def verify_candidate(dist,version,channel,base,public_key,scratch,require_windows):
    u=urlsplit(base)
    if u.scheme != 'https' or not u.netloc or u.username or u.password or u.query or u.fragment:
        raise ChannelError('release download base must be an HTTPS URL')
    data=(dist/'release.json').read_bytes()
    signature=(dist/'release.json.sig').read_bytes()
    key_hash=verify_signature(data,signature,public_key,scratch)
    try: manifest=json.loads(data)
    except ValueError as e: raise ChannelError('invalid candidate manifest') from e
    if manifest.get('version') != version or manifest.get('channel') != channel:
        raise ChannelError('candidate manifest version or channel mismatch')
    assets=manifest.get('assets')
    if not isinstance(assets,list) or not assets:
        raise ChannelError('candidate manifest has no release assets')
    identities=set()
    for asset in assets:
        url=asset.get('url','')
        name=url.rsplit('/',1)[-1]
        if not re.fullmatch(r'[A-Za-z0-9_.-]+',name) or url != base.rstrip('/')+'/'+name:
            raise ChannelError('manifest asset URL is outside the release')
        path=dist/name
        if path.is_symlink() or not path.is_file():
            raise ChannelError('manifest asset is not a regular local file: '+name)
        before=path.stat()
        with path.open('rb') as f: sha=hashlib.file_digest(f,'sha256').hexdigest()
        after=path.stat()
        if (before.st_ino,before.st_size,before.st_mtime_ns) != (after.st_ino,after.st_size,after.st_mtime_ns):
            raise ChannelError('manifest asset changed while verifying: '+name)
        if sha != asset.get('sha256') or after.st_size != asset.get('size'):
            raise ChannelError('manifest asset hash or size mismatch: '+name)
        identity=(asset.get('os'),asset.get('arch'),asset.get('kind'))
        if identity in identities:
            raise ChannelError('duplicate asset type in manifest')
        identities.add(identity)
    if ('linux','amd64','tarball') not in identities or (require_windows and ('windows','amd64','installer') not in identities):
        raise ChannelError('manifest lacks a required release installer or archive')
    return key_hash

class API:
    def __init__(self,base,token,resolve,socket_path,scratch):
        self.base = base.rstrip('/')
        parsed = urlsplit(self.base)
        match = re.fullmatch(r'/api/v1/repos/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)',parsed.path)
        if not match or parsed.query or parsed.fragment or not parsed.netloc:
            raise ChannelError('unexpected Gitea API URL')
        if socket_path:
            if (os.environ.get('AEGO_PRIVATE_GITEA') != '1' or parsed.scheme != 'http' or
                parsed.netloc != 'fixture.invalid' or match[1] != 'lab' or
                str(socket_path) != '/run/gitea-lab/gitea.sock'):
                raise ChannelError('Unix socket transport is restricted to the private fixture')
        elif parsed.scheme != 'https':
            raise ChannelError('production Gitea API must use HTTPS')
        if not token or any(c in token for c in '\r\n\0'):
            raise ChannelError('missing or invalid API token')
        self.origin = parsed.scheme+'://'+parsed.netloc
        self.repo = '/'+match[1]+'/'+match[2]
        self.socket = socket_path
        self.resolve = resolve
        self.scratch = scratch
        # Passing the token on stdin keeps it out of process arguments.
        self.config = 'header = "Authorization: token '+token.replace('\\','\\\\').replace('"','\\"')+'"\n'
        self.seq = 0

    def request(self,method,url,body=None,upload=None,accept=(200,)):
        if not url.startswith(self.origin+'/'):
            raise ChannelError('request escapes configured Gitea origin')
        self.seq += 1
        output = self.scratch/('response-'+str(self.seq))
        cmd = ['curl','--config','-','-sS','-m','180','--max-filesize','1048576',
               '--max-redirs','4','-L','-o',str(output),'-w','%{http_code}','-X',method]
        if self.socket:
            cmd += ['--unix-socket',str(self.socket),'--noproxy','*','--proto','=http','--proto-redir','=http']
        else:
            cmd += ['--proto','=https','--proto-redir','=https']
            if self.resolve:
                cmd += ['--resolve',self.resolve]
        if body is not None:
            cmd += ['-H','Content-Type: application/json','--data-binary',json.dumps(body,separators=(',',':'))]
        if upload is not None:
            file,name = upload
            cmd += ['-F',f'attachment=@{file};filename={name}']
        cmd.append(url)
        try:
            p = subprocess.run(cmd,input=self.config,text=True,capture_output=True,timeout=200)
        except (OSError,subprocess.TimeoutExpired) as e:
            raise ChannelError(f'{method} {url}: transport failure') from e
        code = int(p.stdout) if re.fullmatch(r'\d{3}',p.stdout) else 0
        if p.returncode or code not in accept:
            raise ChannelError(f'{method} {url}: HTTP {code or "unavailable"}')
        return code, output.read_bytes() if output.exists() else b''

    def json(self,method,path,body=None,missing=False):
        status,raw = self.request(method,self.base+path,body=body,accept=(200,201,204,404) if missing else (200,201,204))
        if status == 404:
            return None
        if not raw:
            return None
        try: return json.loads(raw)
        except ValueError as e: raise ChannelError('invalid JSON in Gitea response') from e

    def get_release(self,tag):
        return self.json('GET','/releases/tags/'+quote(tag,safe=''),missing=True)

    def get_ref(self,tag):
        value = self.json('GET','/git/refs/tags/'+quote(tag,safe=''),missing=True)
        if value is None or value == []:
            return None
        if not isinstance(value,list) or len(value) != 1 or value[0].get('ref') != 'refs/tags/'+tag:
            raise ChannelError('unexpected or ambiguous Git tag response')
        obj = value[0].get('object',{})
        if obj.get('type') != 'commit' or not re.fullmatch(r'[a-f0-9]{40}',obj.get('sha','')):
            raise ChannelError('stable channel requires a lightweight commit tag')
        return obj['sha']

    def assets(self,release_id):
        value = self.json('GET',f'/releases/{release_id}/assets')
        if not isinstance(value,list):
            raise ChannelError('invalid release attachment inventory')
        return value

    def download(self,tag,name):
        url = self.origin+self.repo+'/releases/download/'+quote(tag,safe='')+'/'+quote(name,safe='')
        return self.request('GET',url)[1]

    def upload(self,release_id,name,path):
        url = self.base+f'/releases/{release_id}/assets?name='+quote(name,safe='')
        return json.loads(self.request('POST',url,upload=(path,name),accept=(201,))[1])

def snapshot(api,tag,release,expected=None,exact=True):
    assets=api.assets(release['id'])
    names=[x.get('name') for x in assets]
    if len(names) != len(set(names)) or any(name not in names for name in FILES):
        raise ChannelError(tag+': missing or duplicate channel files')
    if exact and set(names) != set(FILES):
        raise ChannelError(tag+': unexpected assets would be lost; refusing to replace release')
    data={name:api.download(tag,name) for name in FILES}
    for name in FILES:
        item=next(x for x in assets if x['name']==name)
        if item.get('size') != len(data[name]):
            raise ChannelError(tag+': asset size mismatch: '+name)
        if expected is not None and data[name] != expected[name]:
            raise ChannelError(tag+': asset content mismatch: '+name)
    return data

def publish_channel(api,tag,commit,new_files,public_key,scratch,record,save):
    version = tag[1:]
    new_key = verify_signature(new_files['release.json'],new_files['release.json.sig'],public_key,scratch)
    try: manifest=json.loads(new_files['release.json'])
    except ValueError as e: raise ChannelError('invalid candidate manifest') from e
    if manifest.get('version') != version or manifest.get('channel') != 'stable':
        raise ChannelError('candidate manifest version or channel mismatch')
    release=api.get_release(tag)
    if not release or release.get('draft') or release.get('prerelease') or api.get_ref(tag) != commit:
        raise ChannelError('published version release/tag does not match the expected commit')
    snapshot(api,tag,release,new_files,exact=False)
    old=api.get_release('stable')
    old_ref=api.get_ref('stable')
    old_files=None
    if old is None:
        if old_ref is not None:
            raise ChannelError('unrelated stable tag exists without a channel release')
    else:
        if old.get('draft') or not old.get('prerelease') or old_ref is None:
            raise ChannelError('unexpected current stable channel state')
        old_files=snapshot(api,'stable',old)
        old_key=verify_signature(old_files['release.json'],old_files['release.json.sig'],public_key,scratch)
        if old_key != new_key:
            raise ChannelError('old and new channel signing keys differ')
        try: old_manifest=json.loads(old_files['release.json'])
        except ValueError as e: raise ChannelError('invalid prior channel manifest') from e
        if old_manifest.get('channel') != 'stable' or not old_manifest.get('version'):
            raise ChannelError('unexpected prior channel manifest')
        record.update(old_release_id=old['id'],old_commit=old_ref,old_version=old_manifest['version'])
    record.update(version=tag,commit=commit,key_sha256=new_key,
                  old_hashes={n:digest(v) for n,v in (old_files or {}).items()},
                  new_hashes={n:digest(v) for n,v in new_files.items()})
    save('prechecked')

    def stage(label,target,files,title,body):
        name='stable-'+label+'-'+commit[:12]+'-'+secrets.token_hex(6)
        value=api.json('POST','/releases',{'tag_name':name,'target_commitish':target,
            'name':title,'body':body,'prerelease':True,'draft':True})
        if not value or not value.get('id'):
            raise ChannelError('could not create '+label+' draft')
        record[label]={'id':value['id'],'tag':name,'target':target}
        save(label+'-created')
        for filename in FILES:
            f=scratch/(label+'-'+filename)
            f.write_bytes(files[filename])
            api.upload(value['id'],filename,f)
        snapshot(api,name,value,files)
        save(label+'-verified')
        return value

    backup=stage('backup',old_ref,old_files,old.get('name') or 'Stable channel',
                 old.get('body') or 'Previous stable channel.') if old else None
    candidate=stage('candidate',commit,new_files,'Stable channel',
                    'Where installed copies of Aego look for updates; it points at '+tag+'.')
    # A concurrent release must not be overwritten after staging.
    current=api.get_release('stable')
    if old:
        if not current or current.get('id') != old['id'] or api.get_ref('stable') != old_ref:
            raise ChannelError('stable channel changed during staging')
        snapshot(api,'stable',current,old_files)
    elif current is not None or api.get_ref('stable') is not None:
        raise ChannelError('stable channel appeared during staging')
    save('ready-to-switch')

    def live_matches(expected_id,expected_ref,files):
        current=api.get_release('stable')
        if not current or current.get('id') != expected_id or current.get('draft') or not current.get('prerelease'):
            return False
        if api.get_ref('stable') != expected_ref:
            return False
        snapshot(api,'stable',current,files)
        return True

    try:
        if old:
            save('deleting-old-release')
            api.json('DELETE',f'/releases/{old["id"]}')
            save('deleting-old-tag')
            api.json('DELETE','/tags/stable')
        save('publishing-candidate')
        api.json('PATCH',f'/releases/{candidate["id"]}',
                 {'tag_name':'stable','target_commitish':commit,'draft':False,'prerelease':True})
        if not live_matches(candidate['id'],commit,new_files):
            raise ChannelError('published stable channel did not verify')
        save('published')
        return
    except (ChannelError,OSError) as original:
        record['failure']=str(original)
        save('cutover-failed')
        try:
            # A response can be lost after the server committed the operation.
            if live_matches(candidate['id'],commit,new_files):
                save('published-after-response-loss')
                return
            if old and live_matches(old['id'],old_ref,old_files):
                save('original-intact')
                raise ChannelError('switch failed; original stable channel is intact') from original
            current=api.get_release('stable')
            if current is not None:
                raise ChannelError('stable release changed unexpectedly; refusing destructive rollback')
            ref=api.get_ref('stable')
            if ref is not None:
                if ref not in {old_ref,commit}:
                    raise ChannelError('stable tag changed unexpectedly; refusing destructive rollback')
                api.json('DELETE','/tags/stable')
            if not backup:
                raise ChannelError('no previous stable channel was available to restore')
            save('restoring-backup')
            api.json('PATCH',f'/releases/{backup["id"]}',
                     {'tag_name':'stable','target_commitish':old_ref,'draft':False,'prerelease':True})
            if not live_matches(backup['id'],old_ref,old_files):
                raise ChannelError('restored stable channel did not verify')
            save('restored')
            raise ChannelError('switch failed; previous stable channel was restored') from original
        except (ChannelError,OSError) as recovery:
            if record.get('phase') not in ('restored','original-intact'):
                record['recovery_error']=str(recovery)
                save('manual-recovery-required')
            raise

def main():
    p=argparse.ArgumentParser()
    p.add_argument('--verify-only',action='store_true')
    p.add_argument('--tag',required=True)
    p.add_argument('--commit')
    p.add_argument('--dist',type=Path,required=True)
    p.add_argument('--receipt-dir',type=Path)
    p.add_argument('--version')
    p.add_argument('--channel',choices=('stable','nightly'))
    p.add_argument('--base-url')
    p.add_argument('--require-windows',action='store_true')
    a=p.parse_args()
    if not re.fullmatch(r'(?:nightly|v\d+\.\d+\.\d+)',a.tag):
        raise ChannelError('invalid release tag')
    key=os.environ.get('AEGO_UPDATE_KEY','')
    dist=a.dist.resolve(strict=True)
    if a.verify_only:
        if not a.version or not a.channel or not a.base_url:
            raise ChannelError('version, channel and base URL are required for verification')
        if (a.tag == 'nightly' and a.channel != 'nightly') or (a.tag != 'nightly' and
            (a.channel != 'stable' or a.version != a.tag[1:])):
            raise ChannelError('release tag, version and channel do not agree')
        with tempfile.TemporaryDirectory(prefix='aego-release-verify-',dir=dist) as tmp:
            verified=verify_candidate(dist,a.version,a.channel,a.base_url,key,Path(tmp),a.require_windows)
        print('release manifest, signature and asset checksums verified; public key SHA-256',verified)
        return 0
    if (a.tag == 'nightly' or not a.commit or not re.fullmatch(r'[a-f0-9]{40}',a.commit)
        or a.receipt_dir is None):
        raise ChannelError('stable promotion needs a full commit and receipt directory')
    token=os.environ.get('GITEA_TOKEN','')
    api_url=os.environ.get('GITEA_API','')
    socket_path=os.environ.get('AEGO_STAGE_FIXTURE_SOCKET')
    new_files={name:(dist/name).read_bytes() for name in FILES}
    receipt_dir=a.receipt_dir.resolve()
    receipt_dir.mkdir(parents=True,exist_ok=True)
    path=receipt_dir/('stable-'+a.commit[:12]+'-'+secrets.token_hex(6)+'.json')
    record={'phase':'starting','receipt':str(path),'published':False}
    def save(phase):
        record['phase']=phase
        record['published']=phase in ('published','published-after-response-loss')
        temp=path.with_suffix('.tmp')
        with temp.open('w') as f:
            json.dump(record,f,indent=2); f.write('\n'); f.flush(); os.fsync(f.fileno())
        os.replace(temp,path)
    save('starting')
    try:
        with tempfile.TemporaryDirectory(prefix='aego-stable-channel-',dir=dist) as tmp:
            scratch=Path(tmp)
            api=API(api_url,token,os.environ.get('GITEA_RESOLVE',''),socket_path,scratch)
            publish_channel(api,a.tag,a.commit,new_files,key,scratch,record,save)
    except (ChannelError,OSError,ValueError) as e:
        record['error']=str(e)
        save(record['phase'])
        print('stable channel:',str(e),'; receipt:',path,file=sys.stderr)
        return 1
    print('stable channel verified at',a.tag,'; receipt:',path)
    return 0

if __name__ == '__main__':
    try: sys.exit(main())
    except (ChannelError,OSError,ValueError) as e:
        print('stable channel:',str(e),file=sys.stderr); sys.exit(1)
