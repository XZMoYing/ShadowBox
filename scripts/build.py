#!/usr/bin/env python3
"""Native, reproducible desktop builds. Run on the target operating system."""
import argparse
import os
import pathlib
import platform
import plistlib
import shutil
import subprocess
from collect_licenses import collect


ROOT = pathlib.Path(__file__).resolve().parent.parent
TAGS = ','.join(__import__('json').loads((ROOT/'core.lock.json').read_text())['build_tags'])
ENV = dict(os.environ)
ENV['PATH'] = str(pathlib.Path.home() / '.cargo/bin') + os.pathsep + ENV.get('PATH', '')
ENV['FLUTTER_SUPPRESS_ANALYTICS'] = 'true'

def run(*args, cwd=ROOT):
    print('+', ' '.join(map(str, args)), flush=True)
    subprocess.run(list(map(str, args)), cwd=cwd, env=ENV, check=True)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--debug', action='store_true')
    parser.add_argument('--no-archive', action='store_true')
    args = parser.parse_args()
    system = {'Darwin': 'macos', 'Windows': 'windows', 'Linux': 'linux'}[platform.system()]
    arch = 'arm64' if platform.machine().lower() in ('arm64', 'aarch64') else 'x64'
    mode = 'debug' if args.debug else 'release'
    suffix = '.exe' if system == 'windows' else ''
    out = ROOT / 'dist' / f'{system}-{arch}'
    out.mkdir(parents=True, exist_ok=True)
    bindir = ROOT / 'dist/bin'
    bindir.mkdir(parents=True, exist_ok=True)
    run('go', 'build', '-trimpath', '-tags', TAGS, '-ldflags=-s -w', '-o', bindir / f'shadowbox-service{suffix}', './cmd/shadowbox-service', cwd=ROOT/'service')
    command = ['cargo', 'build', '--locked', '--manifest-path', ROOT/'native/tray/Cargo.toml']
    if not args.debug:
        command.append('--release')
    run(*command)
    shutil.copy2(ROOT/f'native/tray/target/{mode}/shadowbox-tray{suffix}', bindir/f'shadowbox-tray{suffix}')
    run('flutter', 'pub', 'get', cwd=ROOT/'apps/desktop')
    run('flutter', 'build', system, '--'+mode, '--no-pub', cwd=ROOT/'apps/desktop')
    licenses=ROOT/'dist/THIRD_PARTY_LICENSES.txt'
    collect(licenses)
    if system == 'macos':
        bundle = out/'ShadowBox.app'
        if bundle.exists():
            shutil.rmtree(bundle)
        helper=bundle/'Contents/Helpers/ShadowBox UI.app'
        helper.parent.mkdir(parents=True)
        shutil.copytree(ROOT/f'apps/desktop/build/macos/Build/Products/{mode.capitalize()}/ShadowBox.app', helper, symlinks=True)
        ui_info_path=helper/'Contents/Info.plist'
        with ui_info_path.open('rb') as f: ui_info=plistlib.load(f)
        (helper/'Contents/MacOS/ShadowBox').rename(helper/'Contents/MacOS/shadowbox-ui')
        ui_info.update(CFBundleExecutable='shadowbox-ui',CFBundleIdentifier='dev.shadowbox.ui',LSUIElement=False)
        with ui_info_path.open('wb') as f: plistlib.dump(ui_info,f)
        macos=bundle/'Contents/MacOS'
        macos.mkdir()
        for name in ['shadowbox-service','shadowbox-tray']: shutil.copy2(bindir/name,macos/name)
        resources=bundle/'Contents/Resources'
        resources.mkdir()
        for icon in (helper/'Contents/Resources').glob('*.icns'): shutil.copy2(icon,resources/icon.name)
        info=dict(ui_info)
        info.update(CFBundleExecutable='shadowbox-tray',CFBundleIdentifier='dev.shadowbox.desktop',LSUIElement=True,CFBundleDisplayName='ShadowBox')
        info.pop('NSMainNibFile',None)
        with (bundle/'Contents/Info.plist').open('wb') as f: plistlib.dump(info,f)
        shutil.copy2(licenses,resources/'THIRD_PARTY_LICENSES.txt')
        shutil.copy2(ROOT/'LICENSE',resources/'LICENSE')
        shutil.copy2(ROOT/'third_party/NOTICE.md',resources/'THIRD_PARTY_NOTICES.md')
        sign = ['codesign', '--force', '--deep', '--sign', '-']
        if args.debug:
            sign += ['--entitlements', ROOT/'apps/desktop/macos/Runner/DebugProfile.entitlements']
        run(*sign, bundle)
        if not args.no_archive:
            image_root=out/'dmg-root'
            if image_root.exists(): shutil.rmtree(image_root)
            image_root.mkdir()
            shutil.copytree(bundle,image_root/'ShadowBox.app',symlinks=True)
            (image_root/'Applications').symlink_to('/Applications',target_is_directory=True)
            shutil.copy2(ROOT/'README.md',image_root/'README.md')
            shutil.copy2(ROOT/'DEVELOPMENT.md',image_root/'DEVELOPMENT.md')
            shutil.copytree(ROOT/'docs',image_root/'docs')
            run('hdiutil', 'create', '-volname', 'ShadowBox', '-srcfolder', image_root, '-ov', '-format', 'UDZO', out/'ShadowBox-0.1.0.dmg')
            shutil.rmtree(image_root)
    else:
        source = (ROOT/f'apps/desktop/build/windows/{arch}/runner/{mode.capitalize()}' if system == 'windows' else ROOT/f'apps/desktop/build/linux/{arch}/{mode}/bundle')
        bundle = out/'ShadowBox'
        shutil.copytree(source, bundle, dirs_exist_ok=True)
        original = bundle/f'shadowbox{suffix}'
        original.replace(bundle/f'shadowbox-ui{suffix}')
        shutil.copy2(bindir/f'shadowbox-service{suffix}', bundle/f'shadowbox-service{suffix}')
        shutil.copy2(bindir/f'shadowbox-tray{suffix}', bundle/f'shadowbox-tray{suffix}')
        shutil.copy2(bindir/f'shadowbox-tray{suffix}', bundle/('ShadowBox.exe' if system=='windows' else 'shadowbox'))
        if system == 'linux':
            shutil.copy2(ROOT/'scripts/shadowbox.desktop', bundle/'shadowbox.desktop')
            shutil.copy2(ROOT/'assets/shadowbox.png', bundle/'shadowbox.png')
        shutil.copytree(ROOT/'docs',bundle/'docs',dirs_exist_ok=True)
        for name in ['LICENSE', 'README.md', 'DEVELOPMENT.md', 'core.lock.json']:
            if (ROOT/name).exists(): shutil.copy2(ROOT/name, bundle/name)
        shutil.copy2(licenses,bundle/'THIRD_PARTY_LICENSES.txt')
        shutil.copy2(ROOT/'third_party/NOTICE.md', bundle/'THIRD_PARTY_NOTICES.md')
        if not args.no_archive:
            shutil.make_archive(str(out/'ShadowBox-0.1.0'), 'zip' if system=='windows' else 'gztar', root_dir=out, base_dir='ShadowBox')
    if system == 'linux' and not args.no_archive and shutil.which('dpkg-deb'):
        deb = out/'deb-root'
        if deb.exists(): shutil.rmtree(deb)
        shutil.copytree(bundle, deb/'opt/shadowbox', symlinks=True)
        applications = deb/'usr/share/applications'
        applications.mkdir(parents=True)
        desktop = (ROOT/'scripts/shadowbox.desktop').read_text().replace('Exec=shadowbox', 'Exec=/opt/shadowbox/shadowbox')
        (applications/'shadowbox.desktop').write_text(desktop)
        icons=deb/'usr/share/icons/hicolor/256x256/apps'
        icons.mkdir(parents=True)
        shutil.copy2(ROOT/'assets/shadowbox.png', icons/'shadowbox.png')
        (deb/'DEBIAN').mkdir()
        (deb/'DEBIAN/control').write_text('Package: shadowbox\nVersion: 0.1.0\nSection: net\nPriority: optional\nArchitecture: '+('arm64' if arch=='arm64' else 'amd64')+'\nMaintainer: ShadowBox contributors\nDepends: libgtk-3-0 | libgtk-3-0t64, libayatana-appindicator3-1, libstdc++6, libc6\nDescription: Desktop proxy client with an on-demand interface\n')
        run('dpkg-deb','--build','--root-owner-group',deb,out/'ShadowBox-0.1.0.deb')
    shutil.copytree(ROOT/'docs',out/'docs',dirs_exist_ok=True)
    for name in ['LICENSE', 'README.md', 'DEVELOPMENT.md', 'core.lock.json']:
        p=ROOT/name
        if p.exists():
            shutil.copy2(p, out/name)
    shutil.copy2(licenses,out/'THIRD_PARTY_LICENSES.txt')
    shutil.copy2(ROOT/'third_party/NOTICE.md', out/'THIRD_PARTY_NOTICES.md')
    print(f'Build ready: {out}', flush=True)

if __name__ == '__main__':
    main()
