#!/usr/bin/env python3
"""Keep a disposable, local-only GUI validation session alive until /v1/quit."""
import argparse, importlib.util, json, os, pathlib, signal, socket, subprocess, tempfile, threading, time, urllib.request
ROOT=pathlib.Path(__file__).resolve().parent.parent
spec=importlib.util.spec_from_file_location('smoke',ROOT/'scripts/smoke_desktop.py');smoke=importlib.util.module_from_spec(spec);spec.loader.exec_module(smoke)

def main():
 parser=argparse.ArgumentParser();parser.add_argument('--direct-ui',action='store_true');args=parser.parse_args()
 directory=pathlib.Path(tempfile.mkdtemp(prefix='shadowbox-gui-'))
 marker=ROOT/'dist/gui-session.json'
 with smoke.Server(('127.0.0.1',0),smoke.Proxy) as proxy, smoke.http.server.ThreadingHTTPServer(('127.0.0.1',0),smoke.Origin) as origin:
  for server in [proxy,origin]:threading.Thread(target=server.serve_forever,daemon=True).start()
  binary=ROOT/'dist/macos-arm64/ShadowBox.app/Contents/MacOS/shadowbox-tray'
  log=(directory/'fixture-launcher.log').open('w')
  tray=subprocess.Popen([str(binary),'--background'],env={**os.environ,'SHADOWBOX_DATA_DIR':str(directory)},stdout=log,stderr=log)
  def api(path,body=None):
   d=json.loads((directory/'service.json').read_text());r=urllib.request.Request(d['endpoint']+'/v1/'+path,headers={'Authorization':'Bearer '+d['token'],'Content-Type':'application/json'},data=None if body is None else json.dumps(body).encode())
   with urllib.request.urlopen(r,timeout=10) as response:return json.load(response)
  try:
   smoke.wait_for(lambda:api('status'))
   with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
   settings=api('settings');settings.update(capture='manual',routing='global',port=port,test_url=f'http://127.0.0.1:{origin.server_port}/')
   api('settings',settings)
   api('nodes',{'nodes':[{'id':'gui-local','name':'本地验收 · HTTP','group':'桌面验收','outbound':{'type':'http','server':'127.0.0.1','server_port':proxy.server_address[1]},'latency_ms':-1}]})
   ui=None
   if args.direct_ui:
    helper=binary.parent.parent/'Helpers/ShadowBox UI.app/Contents/MacOS/shadowbox-ui'
    ui=subprocess.Popen([str(helper)],env={**os.environ,'SHADOWBOX_DATA_DIR':str(directory)},stdout=log,stderr=log)
   else:api('ui/show',{})
   meta={'data_dir':str(directory),'tray_pid':tray.pid,'origin_port':origin.server_port,'upstream_port':proxy.server_address[1],'proxy_port':port}
   marker.write_text(json.dumps(meta))
   print(json.dumps(meta),flush=True)
   tray.wait()
   if ui:
    try:ui.wait(timeout=15)
    except subprocess.TimeoutExpired:ui.terminate();raise
    assert ui.returncode==0,ui.returncode
    (ROOT/'dist/gui-helper-validation.json').write_text(json.dumps({'direct_helper_command_q_exited_ui_and_tray':True,'ui_returncode':ui.returncode,'tray_returncode':tray.returncode},indent=2))
  finally:
   if tray.poll() is None:
    try:api('quit',{});tray.wait(timeout=10)
    except Exception:tray.terminate()
   try:api('shutdown',{})
   except Exception:pass
   proxy.shutdown();origin.shutdown();log.close()
   print(json.dumps({'upstream_connect_count':smoke.Proxy.hits,'tray_returncode':tray.returncode}),flush=True)
if __name__=='__main__':main()
