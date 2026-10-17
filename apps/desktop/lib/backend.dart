import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

typedef Json = Map<String, dynamic>;

class Backend extends ChangeNotifier {
  final HttpClient _client = HttpClient()
    ..connectionTimeout = const Duration(seconds: 3);
  String endpoint = '', token = '';
  Json status = {}, settings = {};
  List<Json> nodes = [],
      subscriptions = [],
      plugins = [],
      logs = [],
      connections = [];
  String? error;
  bool loading = true, working = false, _polling = false, _disposed = false;
  Timer? _timer;
  int _wake = 0;
  static String get dataDir {
    final env = Platform.environment;
    if (env['SHADOWBOX_DATA_DIR'] case final String value) return value;
    if (Platform.isMacOS) {
      return '${env['HOME']}/Library/Application Support/ShadowBox';
    }
    if (Platform.isWindows) return '${env['APPDATA']}\\ShadowBox';
    return '${env['XDG_CONFIG_HOME'] ?? '${env['HOME']}/.config'}/ShadowBox';
  }

  Future<dynamic> request(
    String path, {
    String method = 'GET',
    dynamic body,
    List<int>? bytes,
    String? filename,
  }) async {
    if (endpoint.isEmpty) throw Exception('后台服务尚未连接');
    final req = await _client.openUrl(method, Uri.parse('$endpoint$path'));
    req.headers.set(HttpHeaders.authorizationHeader, 'Bearer $token');
    if (bytes != null) {
      req.headers.contentType = ContentType.binary;
      req.headers.set('X-Filename', filename ?? 'nodes.txt');
      req.add(bytes);
    } else if (method == 'POST' || method == 'PUT') {
      req.headers.contentType = ContentType.json;
      req.write(jsonEncode(body ?? {}));
    }
    final response = await req.close().timeout(const Duration(seconds: 150));
    final text = await response.transform(utf8.decoder).join();
    dynamic result;
    try {
      result = jsonDecode(text);
    } catch (_) {
      throw Exception('后台响应无法解析（${response.statusCode}）');
    }
    if (response.statusCode >= 400) {
      throw Exception(result is Map ? result['error'] ?? '请求失败' : '请求失败');
    }
    return result;
  }

  Future<void> initialize() async {
    loading = true;
    error = null;
    notifyListeners();
    try {
      final file = File('$dataDir/service.json');
      if (!await file.exists()) {
        final exeDir = File(Platform.resolvedExecutable).parent;
        var tray = File(
          '${exeDir.path}/shadowbox-tray${Platform.isWindows ? '.exe' : ''}',
        );
        if (Platform.isMacOS && !await tray.exists()) {
          tray = File.fromUri(
            exeDir.uri.resolve('../../../../MacOS/shadowbox-tray'),
          );
        }
        if (await tray.exists()) {
          await Process.start(tray.path, [
            '--background',
          ], mode: ProcessStartMode.detached);
          for (var i = 0; i < 80 && !await file.exists(); i++) {
            await Future<void>.delayed(const Duration(milliseconds: 100));
          }
        }
      }
      final data = jsonDecode(await file.readAsString()) as Json;
      endpoint = data['endpoint'] as String;
      token = data['token'] as String;
      await refresh();
      _timer?.cancel();
      _timer = Timer.periodic(const Duration(seconds: 1), (_) => poll());
    } catch (e) {
      error = '无法连接后台服务：${clean(e)}';
    }
    loading = false;
    if (!_disposed) notifyListeners();
  }

  Future<void> refresh() async {
    final values = await Future.wait([
      request('/v1/status'),
      request('/v1/settings'),
      request('/v1/nodes'),
      request('/v1/subscriptions'),
      request('/v1/plugins'),
    ]);
    status = Map<String, dynamic>.from(values[0] as Map);
    settings = Map<String, dynamic>.from(values[1] as Map);
    nodes = maps(values[2]);
    subscriptions = maps(values[3]);
    plugins = maps(values[4]);
    if (!_disposed) notifyListeners();
  }

  Future<void> poll() async {
    if (_polling || _disposed || endpoint.isEmpty) return;
    _polling = true;
    try {
      final discovery = jsonDecode(
        await File('$dataDir/service.json').readAsString(),
      ) as Json;
      final restarted =
          endpoint != discovery['endpoint'] || token != discovery['token'];
      endpoint = discovery['endpoint'] as String;
      token = discovery['token'] as String;
      if (restarted) {
        await refresh();
        error = null;
      }
      final next = Map<String, dynamic>.from(
        await request('/v1/status') as Map,
      );
      if (next['wake_generation'] != _wake) {
        _wake = (next['wake_generation'] as num?)?.toInt() ?? 0;
        try {
          await const MethodChannel('shadowbox/window')
              .invokeMethod<void>('show');
        } catch (_) {
          /* Native runner may not expose window control in widget tests. */
        }
      }
      if (error?.startsWith('后台服务连接中断') == true) {
        error = null;
      }
      final changed =
          next['selected'] != status['selected'] ||
          next['revision'] != status['revision'];
      status = next;
      if (changed && !working) await refresh();
      if (!_disposed) notifyListeners();
    } catch (e) {
      error = '后台服务连接中断：${clean(e)}';
      if (!_disposed) notifyListeners();
    } finally {
      _polling = false;
    }
  }

  Future<bool> action(Future<void> Function() operation) async {
    if (working) return false;
    working = true;
    error = null;
    notifyListeners();
    try {
      await operation();
      await refresh();
      return true;
    } catch (e) {
      error = clean(e);
      return false;
    } finally {
      working = false;
      if (!_disposed) notifyListeners();
    }
  }

  Future<void> setSettings(Json value) async {
    await request('/v1/settings', method: 'POST', body: value);
  }

  Future<void> select(String id) async {
    await request('/v1/select', method: 'POST', body: {'id': id});
  }

  Future<void> connect([String? id]) async {
    await request(
      '/v1/connect',
      method: 'POST',
      body: {'id': id ?? status['selected'] ?? ''},
    );
  }

  Future<void> stop() async {
    await request('/v1/stop', method: 'POST');
  }

  Json? get selectedNode {
    for (final n in nodes) {
      if (n['id'] == status['selected']) return n;
    }
    return null;
  }

  bool get running => status['running'] == true;
  bool get busy => working || status['busy'] == true;
  static List<Json> maps(dynamic value) =>
      (value as List).map((e) => Map<String, dynamic>.from(e as Map)).toList();
  static String clean(Object e) =>
      e.toString().replaceFirst(RegExp(r'^Exception:\s*'), '');
  @override
  void dispose() {
    _disposed = true;
    _timer?.cancel();
    _client.close(force: true);
    super.dispose();
  }
}

String nodeType(Json n) => n['chain'] is List && (n['chain'] as List).isNotEmpty
    ? '代理链'
    : (n['external']?['kind'] ?? n['outbound']?['type'] ?? '配置').toString();
String byteLabel(num? input) {
  var value = (input ?? 0).toDouble();
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  var i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return '${value.toStringAsFixed(i == 0 ? 0 : 1)} ${units[i]}';
}
