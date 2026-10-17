#!/usr/bin/env python3
"""Packaged macOS smoke test with disposable profiles and local-only traffic.
No OS proxy settings or elevated TUN permissions are changed.
"""
import argparse, http.server, json, os, pathlib, select, signal, socket, socketserver, subprocess, tempfile, threading, time, urllib.request

class Proxy(socketserver.StreamRequestHandler):
    hits = 0
    def handle(self):
        request = self.rfile.readline().decode().split()
        if len(request)<2 or request[0]!='CONNECT': return
        while self.rfile.readline() not in (b'\r\n',b'\n',b''): pass
        host,port = request[1].rsplit(':',1)
        with socket.create_connection((host,int(port))) as upstream:
            Proxy.hits += 1
            self.wfile.write(b'HTTP/1.1 200 Connection Established\r\n\r\n'); self.wfile.flush()
            while True:
                ready,_,_=select.select([self.connection,upstream],[],[],10)
                if not ready: return
                for source in ready:
                    data=source.recv(65536)
                    if not data:return
                    (upstream if source is self.connection else self.connection).sendall(data)
class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address=True
    daemon_threads=True
class Origin(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b'shadowbox-real-core-ok')
    def log_message(self,*args): pass

def wait_for(call,timeout=15):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        try:
            result=call()
            if result: return result
        except (OSError,ValueError,KeyError,urllib.error.URLError): pass
        time.sleep(.15)
    raise RuntimeError('Timed out waiting for '+str(call))
def children(pid):
    out=subprocess.check_output(['ps','-axo','pid=,ppid=,rss=,comm='],text=True)
    return [line.split(None,3) for line in out.splitlines() if len(line.split(None,3))==4 and int(line.split(None,3)[1])==pid]
def ui_pid(pid):
    return next((int(p[0]) for p in children(pid) if p[3].endswith('shadowbox-ui')),None)

def main():
    parser=argparse.ArgumentParser();parser.add_argument('bundle',type=pathlib.Path);args=parser.parse_args()
    binary=args.bundle.resolve()/'Contents/MacOS/shadowbox-tray'
    report={};ui=None
    with tempfile.TemporaryDirectory(prefix='shadowbox-smoke-') as tmp:
        directory=pathlib.Path(tmp)
        with Server(('127.0.0.1',0),Proxy) as proxy, http.server.ThreadingHTTPServer(('127.0.0.1',0),Origin) as origin:
            for s in [proxy,origin]: threading.Thread(target=s.serve_forever,daemon=True).start()
            launcher=subprocess.Popen([str(binary),'--background'],env={**os.environ,'SHADOWBOX_DATA_DIR':tmp},stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            def discovery():return json.loads((directory/'service.json').read_text())
            def api(path,body=None):
                d=discovery();req=urllib.request.Request(d['endpoint']+'/v1/'+path,headers={'Authorization':'Bearer '+d['token'],'Content-Type':'application/json'},data=None if body is None else json.dumps(body).encode())
                with urllib.request.urlopen(req,timeout=10) as r:return json.load(r)
            def traffic():
                with socket.create_connection(('127.0.0.1',port),timeout=5) as sock:
                    sock.sendall(f'CONNECT 127.0.0.1:{origin.server_port} HTTP/1.1\r\nHost: 127.0.0.1:{origin.server_port}\r\n\r\n'.encode())
                    header=b''
                    while not header.endswith(b'\r\n\r\n'):header+=sock.recv(1)
                    assert b'200' in header,header
                    sock.sendall(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n')
                    result=b''
                    while data:=sock.recv(65536):result+=data
                    assert b'shadowbox-real-core-ok' in result,result
            try:
                wait_for(lambda:api('status'))
                settings=api('settings');settings.update(capture='manual',routing='global')
                with socket.socket() as free:free.bind(('127.0.0.1',0));port=free.getsockname()[1]
                settings['port']=port;api('settings',settings)
                api('nodes',{'nodes':[{'id':'local-test','name':'Local HTTP fixture','group':'Test','outbound':{'type':'http','server':'127.0.0.1','server_port':proxy.server_address[1]}}]})
                api('connect',{'id':'local-test'});traffic();start=api('status')['started_at'];service_pid=discovery()['pid']
                api('ui/show',{});ui=wait_for(lambda:ui_pid(launcher.pid))
                report['foreground_rss_mib']=round(sum(int(p[2]) for p in children(launcher.pid))/1024 + int(subprocess.check_output(['ps','-p',str(launcher.pid),'-o','rss=']))/1024,1)
                os.kill(ui,signal.SIGTERM)
                wait_for(lambda:not ui_pid(launcher.pid))
                traffic();assert discovery()['pid']==service_pid;assert api('status')['started_at']==start
                samples=[]
                for _ in range(10):
                    time.sleep(1)
                    rss=sum(int(p[2]) for p in children(launcher.pid))+int(subprocess.check_output(['ps','-p',str(launcher.pid),'-o','rss=']))
                    samples.append(rss/1024)
                report['background_rss_mib_max']=round(max(samples),1)
                report['ui_process_absent_after_termination']=ui_pid(launcher.pid) is None
                api('ui/show',{});new_ui=wait_for(lambda:ui_pid(launcher.pid));assert new_ui!=ui
                assert api('status')['started_at']==start;traffic()
                os.kill(new_ui,signal.SIGTERM);wait_for(lambda:not ui_pid(launcher.pid))
                os.kill(service_pid,signal.SIGKILL)
                wait_for(lambda:discovery()['pid']!=service_pid and api('status'))
                assert not api('status')['running'];api('connect',{'id':'local-test'});traffic()
                report.update(core_version=api('status')['core_version'],upstream_connect_count=Proxy.hits,proxy_survived_ui_exit=True,ui_reopened_without_core_restart=True,service_crash_recovered=True)
                api('quit',{})
                wait_for(lambda:launcher.poll() is not None)
                assert launcher.returncode==0
                report['explicit_quit_stops_tray']=True
            finally:
                launcher.terminate();launcher.wait(timeout=5)
                current=ui_pid(launcher.pid)
                if current:
                    try:os.kill(current,signal.SIGTERM)
                    except ProcessLookupError:pass
                try:api('shutdown',{})
                except Exception:pass
                time.sleep(4)
                proxy.shutdown();origin.shutdown()
    print(json.dumps(report,indent=2))
    report_path=pathlib.Path(__file__).resolve().parent.parent/'dist/smoke-macos.json';report_path.write_text(json.dumps(report,indent=2))
if __name__=='__main__':main()
