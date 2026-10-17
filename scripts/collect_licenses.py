#!/usr/bin/env python3
"""Collect native dependency notices; Flutter additionally bundles NOTICES.Z."""
import json, pathlib, subprocess, urllib.request, urllib.error, concurrent.futures
ROOT=pathlib.Path(__file__).resolve().parent.parent

def json_stream(text):
    decoder=json.JSONDecoder();cursor=0
    while cursor<len(text):
        while cursor<len(text) and text[cursor].isspace():cursor+=1
        if cursor==len(text):break
        value,end=decoder.raw_decode(text,cursor);cursor=end;yield value

def collect(destination):
    entries=[];missing=[]
    package_sources={}
    fallbacks={}
    tags=','.join(json.loads((ROOT/'core.lock.json').read_text())['build_tags'])
    used=set(subprocess.check_output(['go','list','-deps','-tags',tags,'-f','{{if .Module}}{{.Module.Path}}{{end}}','./cmd/shadowbox-service'],cwd=ROOT/'service',text=True).splitlines())
    modules=subprocess.check_output(['go','list','-m','-json','all'],cwd=ROOT/'service',text=True)
    for module in json_stream(modules):
        if module['Path'] not in used:continue
        m=module.get('Replace',module);directory=m.get('Dir')
        if directory:entries.append(('Go: '+m['Path']+' '+m.get('Version','local'),pathlib.Path(directory)))
    cargo=str(pathlib.Path.home()/'.cargo/bin/cargo')
    if not pathlib.Path(cargo).exists():cargo='cargo'
    rustc=str(pathlib.Path(cargo).parent/'rustc') if pathlib.Path(cargo).is_absolute() else 'rustc'
    host=next(x.split(': ',1)[1] for x in subprocess.check_output([rustc,'-vV'],text=True).splitlines() if x.startswith('host: '))
    metadata=json.loads(subprocess.check_output([cargo,'metadata','--locked','--format-version','1','--filter-platform',host,'--manifest-path',str(ROOT/'native/tray/Cargo.toml')],cwd=ROOT,text=True))
    graph={n['id']:[d['pkg'] for d in n['deps']] for n in metadata['resolve']['nodes']}
    reachable=set();stack=list(metadata['workspace_members'])
    while stack:
        key=stack.pop()
        if key in reachable:continue
        reachable.add(key);stack.extend(graph.get(key,[]))
    repositories={}
    for package in metadata['packages']:
        repo=package.get('repository')
        directory=pathlib.Path(package['manifest_path']).parent
        files=[p for p in directory.iterdir() if p.is_file() and p.name.lower().startswith(('license','copying','copyright','notice'))]
        if files and repo:repositories[repo]=files
    for package in metadata['packages']:
        if package['id'] not in reachable:continue
        if package.get('source'):
            directory=pathlib.Path(package['manifest_path']).parent
            entries.append(('Rust: '+package['name']+' '+package['version']+' ('+str(package.get('license',''))+')',directory))
            fallbacks[directory]=repositories.get(package.get('repository'),[])
            vcs=directory/'.cargo_vcs_info.json'
            repo=package.get('repository','') or ''
            if vcs.exists() and repo.startswith('https://github.com/'):
                package_sources[directory]=(repo.removesuffix('.git').removesuffix('/').replace('https://github.com/',''),json.loads(vcs.read_text())['git']['sha1'])
    cache=ROOT/'third_party/license-cache';cache.mkdir(exist_ok=True)
    pairs=set(package_sources.values())
    def fetch(pair):
        repo,sha=pair
        prefix=repo.replace('/','_')+'_'+sha[:12]
        existing=list(cache.glob(prefix+'_*'))
        if existing:return
        for name in ['LICENSE','LICENSE-MIT','LICENSE-APACHE','LICENSE.md','LICENSE.txt','COPYING']:
            try:
                text=urllib.request.urlopen(f'https://raw.githubusercontent.com/{repo}/{sha}/{name}',timeout=15).read()
                (cache/(prefix+'_'+name)).write_bytes(text)
            except (urllib.error.HTTPError,urllib.error.URLError,TimeoutError):pass
    # Fetch only missing root notices, from the exact crate source commits.
    needed=set()
    for _,directory in entries:
        if directory in package_sources and not any(p.is_file() and p.name.lower().startswith(('license','copying','copyright','notice')) for p in directory.iterdir()):needed.add(package_sources[directory])
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(fetch,needed))
    output=['Native dependency license texts. See Flutter assets/NOTICES.Z for Dart package notices.\n']
    for label,directory in entries:
        files=[p for p in directory.iterdir() if p.is_file() and p.name.lower().startswith(('license','copying','copyright','notice'))]
        if not files and directory in package_sources:
            repo,sha=package_sources[directory];files=list(cache.glob(repo.replace('/','_')+'_'+sha[:12]+'_*'))
        if not files: files=fallbacks.get(directory,[])
        if not files and label.startswith('Go: shadowbox/'):
            files=[ROOT/'LICENSE']
        if not files:missing.append(label)
        for path in sorted(files):
            output.append('\n'+'='*72+'\n'+label+' / '+path.name+'\n'+'='*72+'\n'+path.read_text(errors='replace', encoding='utf-8'))
    if missing:output.append('\nModules without root license text (inspect source before public distribution):\n'+'\n'.join(missing))
    destination.parent.mkdir(parents=True,exist_ok=True)
    destination.write_text('\n'.join(output), encoding='utf-8')
    print(f'Collected notices for {len(entries)} native modules; {len(missing)} without root license files: {destination}')
if __name__=='__main__':collect(ROOT/'dist/THIRD_PARTY_LICENSES.txt')
