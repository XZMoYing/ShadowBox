#!/usr/bin/env python3
"""Record real traffic and process identity around manual/CUA window actions."""
import argparse
import importlib.util
import json
import pathlib
import socket
import subprocess
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location('smoke', ROOT / 'scripts/smoke_desktop.py')
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('stage', choices=['connected', 'closed', 'reopened', 'quit'])
    args = parser.parse_args()
    meta = json.loads((ROOT / 'dist/gui-session.json').read_text())
    report_path = ROOT / 'dist/gui-validation.json'
    report = json.loads(report_path.read_text()) if args.stage != 'connected' and report_path.exists() else {}
    pid = meta['tray_pid']
    if args.stage == 'quit':
        alive = subprocess.run(['kill', '-0', str(pid)], capture_output=True).returncode == 0
        assert not alive, 'Tray still running after explicit quit'
        assert not smoke.children(pid), 'Child processes remain after explicit quit'
        result = {'tray_exited': True}
    else:
        discovery = json.loads((pathlib.Path(meta['data_dir']) / 'service.json').read_text())
        req = urllib.request.Request(discovery['endpoint'] + '/v1/status', headers={'Authorization': 'Bearer ' + discovery['token']})
        with urllib.request.urlopen(req) as response:
            status = json.load(response)
        assert status['running'], status
        ui = smoke.ui_pid(pid)
        if args.stage == 'closed':
            assert ui is None, 'Flutter remains after closing the window'
        else:
            assert ui is not None, 'No Flutter process'
        with socket.create_connection(('127.0.0.1', meta['proxy_port']), timeout=5) as sock:
            sock.sendall(f"CONNECT 127.0.0.1:{meta['origin_port']} HTTP/1.1\r\nHost: localhost\r\n\r\n".encode())
            header = b''
            while not header.endswith(b'\r\n\r\n'):
                part = sock.recv(1)
                assert part, 'Proxy closed before CONNECT response'
                header += part
            assert b'200' in header, header
            sock.sendall(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
            payload = b''
            while data := sock.recv(65536):
                payload += data
            assert b'shadowbox-real-core-ok' in payload
        result = {'service_pid': discovery['pid'], 'started_at': status['started_at'], 'ui_pid': ui, 'real_traffic_passed': True}
        if args.stage != 'connected':
            assert result['service_pid'] == report['connected']['service_pid']
            assert result['started_at'] == report['connected']['started_at']
        if args.stage == 'reopened':
            assert ui != report['connected']['ui_pid'], 'UI was not restarted'
    report[args.stage] = result
    report_path.write_text(json.dumps(report, indent=2))
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
