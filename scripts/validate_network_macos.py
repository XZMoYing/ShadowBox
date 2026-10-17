#!/usr/bin/env python3
"""Explicitly authorized macOS network acceptance; local fixture, finally cleanup.

Prepare: python3 scripts/validate_network_macos.py --prepare
Run after consent: ... --mode system|tun --apply
TUN raises a normal macOS administrator prompt; the user enters credentials.
"""
import argparse
import http.server
import importlib.util
import json
import os
import pathlib
import select
import socket
import socketserver
import struct
import subprocess
import tempfile
import threading
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location('smoke', ROOT / 'scripts/smoke_desktop.py')
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)
TARGET = '198.51.100.42'


def read_exact(stream, length):
    data = b''
    while len(data) < length:
        part = stream.read(length - len(data))
        if not part:
            raise EOFError()
        data += part
    return data


class DNS(socketserver.StreamRequestHandler):
    hits = 0

    def handle(self):
        while True:
            try:
                packet = read_exact(self.rfile, struct.unpack('!H', read_exact(self.rfile, 2))[0])
            except EOFError:
                return
            end = 12
            while packet[end]:
                end += packet[end] + 1
            end += 5
            qtype = struct.unpack('!H', packet[end - 4:end - 2])[0]
            answer = b'\xc0\x0c\x00\x01\x00\x01\x00\x00\x00\x1e\x00\x04' + socket.inet_aton(TARGET) if qtype == 1 else b''
            response = packet[:2] + struct.pack('!HHHHH', 0x8180, 1, bool(answer), 0, 0) + packet[12:end] + answer
            self.wfile.write(struct.pack('!H', len(response)) + response)
            self.wfile.flush()
            DNS.hits += 1


class FixtureProxy(socketserver.StreamRequestHandler):
    hits = 0
    origin_port = 0
    dns_port = 0

    def handle(self):
        request = self.rfile.readline().decode().split()
        if len(request) < 2 or request[0] != 'CONNECT':
            return
        while self.rfile.readline() not in (b'\r\n', b'\n', b''):
            pass
        host, port = request[1].rsplit(':', 1)
        if host == TARGET:
            target = self.origin_port
        elif host == '1.1.1.1' and port == '53':
            target = self.dns_port
        else:
            self.wfile.write(b'HTTP/1.1 403 Test fixture only\r\n\r\n')
            return
        with socket.create_connection(('127.0.0.1', target), timeout=5) as upstream:
            FixtureProxy.hits += 1
            self.wfile.write(b'HTTP/1.1 200 Connection Established\r\n\r\n')
            self.wfile.flush()
            while True:
                ready, _, _ = select.select([self.connection, upstream], [], [], 10)
                if not ready:
                    return
                for source in ready:
                    data = source.recv(65536)
                    if not data:
                        return
                    (upstream if source is self.connection else self.connection).sendall(data)


def route_interface():
    result = subprocess.check_output(['/sbin/route', '-n', 'get', TARGET], text=True)
    return next(line.split(':', 1)[1].strip() for line in result.splitlines() if line.strip().startswith('interface:'))


def proxy_snapshot():
    services = subprocess.check_output(['/usr/sbin/networksetup', '-listallnetworkservices'], text=True).splitlines()[1:]
    result = {}
    for service in services:
        if not service or service.startswith('*'):
            continue
        for kind in ['webproxy', 'securewebproxy', 'socksfirewallproxy', 'autoproxyurl']:
            raw = subprocess.check_output(['/usr/sbin/networksetup', '-get' + kind, service], text=True)
            result[service + '/' + kind] = dict(line.split(': ', 1) for line in raw.splitlines() if ': ' in line)
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--prepare', action='store_true')
    parser.add_argument('--mode', choices=['system', 'tun'])
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    client = ROOT / 'dist/system-http-fixture'
    if args.prepare:
        subprocess.run(['/usr/bin/swiftc', str(ROOT / 'scripts/fixtures/SystemHTTP.swift'), '-o', str(client)], check=True)
        print(json.dumps({'prepared': client.exists(), 'route_interface': route_interface(), 'network_changed': False}))
        return
    if not args.apply or not args.mode:
        parser.error('Requires explicit --mode and --apply after user consent')
    before_route = route_interface()
    if args.mode == 'tun' and before_route.startswith(('utun', 'tun')):
        raise RuntimeError('Existing VPN/TUN route detected; leave the other application unchanged')
    before_proxy = proxy_snapshot()
    report = {'mode': args.mode, 'started_at': time.strftime('%Y-%m-%dT%H:%M:%S%z')}
    # Retain private recovery files on failure; never delete a pending restore.
    tmp = tempfile.mkdtemp(prefix='shadowbox-network-')
    with smoke.Server(('127.0.0.1', 0), FixtureProxy) as proxy, smoke.Server(('127.0.0.1', 0), DNS) as dns, http.server.ThreadingHTTPServer(('127.0.0.1', 0), smoke.Origin) as origin:
        for server in [proxy, dns, origin]:
            threading.Thread(target=server.serve_forever, daemon=True).start()
        FixtureProxy.origin_port = origin.server_port
        FixtureProxy.dns_port = dns.server_address[1]
        directory = pathlib.Path(tmp)
        (directory / 'network-before.json').write_text(json.dumps(before_proxy))
        (directory / 'network-before.json').chmod(0o600)
        report['recovery_directory'] = str(directory)
        tray = subprocess.Popen([str(ROOT / 'dist/macos-arm64/ShadowBox.app/Contents/MacOS/shadowbox-tray'), '--background'], env={**os.environ, 'SHADOWBOX_DATA_DIR': tmp}, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

        def api(path, body=None):
            d = json.loads((directory / 'service.json').read_text())
            request = urllib.request.Request(d['endpoint'] + '/v1/' + path, headers={'Authorization': 'Bearer ' + d['token'], 'Content-Type': 'application/json'}, data=None if body is None else json.dumps(body).encode())
            with urllib.request.urlopen(request, timeout=150) as response:
                return json.load(response)

        try:
            smoke.wait_for(lambda: api('status'))
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0))
                port = sock.getsockname()[1]
            settings = api('settings')
            settings.update(capture=args.mode, routing='global', port=port, dns='1.1.1.1', ipv6=True)
            api('settings', settings)
            api('nodes', {'nodes': [{'id': 'network-local', 'name': 'Network acceptance fixture', 'outbound': {'type': 'http', 'server': '127.0.0.1', 'server_port': proxy.server_address[1]}}]})
            print('Starting authorized network acceptance; enter macOS administrator authorization if prompted.', flush=True)
            api('connect', {'id': 'network-local'})
            if args.mode == 'system':
                active = proxy_snapshot()
                for key, value in active.items():
                    if key.endswith('/autoproxyurl'):
                        continue
                    assert value['Enabled'] == 'Yes' and value['Server'] == '127.0.0.1' and int(value['Port']) == port
                report['os_proxy_applied'] = True
            else:
                assert route_interface().startswith('utun'), 'TUN route not installed'
                report['tun_route_installed'] = True
                question = b'\x09shadowbox\x0avalidation\x07invalid\x00' + struct.pack('!HH', 1, 1)
                packet = struct.pack('!HHHHHH', 0x5A42, 0x0100, 1, 0, 0, 0) + question
                with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
                    sock.settimeout(6)
                    sock.sendto(packet, ('198.51.100.53', 53))
                    reply, _ = sock.recvfrom(4096)
                    assert reply[:2] == packet[:2] and socket.inet_aton(TARGET) in reply
                assert DNS.hits > 0
                report['dns_hijack_via_selected_outbound'] = True
            subprocess.run([str(client), 'http://' + TARGET + '/'], check=True, timeout=10)
            assert FixtureProxy.hits > 0
            report['cfnetwork_real_traffic_passed'] = True
            api('stop', {})
        finally:
            try:
                api('stop', {})
                api('quit', {})
                tray.wait(timeout=15)
            except Exception:
                tray.terminate()
                try:
                    tray.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    tray.kill()
            # Allow the privileged orphan heartbeat guard to complete too.
            smoke.wait_for(lambda: route_interface() == before_route, timeout=10)
            after_proxy = proxy_snapshot()
            assert after_proxy == before_proxy, 'System proxy restoration differs; inspect recovery snapshot before continuing'
            report['os_proxy_restored'] = True
            report['route_restored'] = True
            for server in [proxy, dns, origin]:
                server.shutdown()
            (ROOT / ('dist/network-' + args.mode + '-macos.json')).write_text(json.dumps(report, indent=2))
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
