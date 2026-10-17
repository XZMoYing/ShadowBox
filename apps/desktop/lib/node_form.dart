import 'dart:convert';

import 'package:material_ui/material_ui.dart';

import 'backend.dart';

class NodeForm extends StatefulWidget {
  final Json? node;
  final Future<void> Function(Json) onSave;
  const NodeForm({super.key, this.node, required this.onSave});
  static const protocols = {
    'shadowsocks': 'Shadowsocks',
    'vmess': 'VMess',
    'vless': 'VLESS',
    'trojan': 'Trojan',
    'anytls': 'AnyTLS',
    'hysteria2': 'Hysteria 2',
    'hysteria': 'Hysteria 1',
    'tuic': 'TUIC v5',
    'socks': 'SOCKS',
    'http': 'HTTP',
    'ssh': 'SSH',
  };
  @override
  State<NodeForm> createState() => _NodeFormState();
}

class _NodeFormState extends State<NodeForm> {
  final fields = <String, TextEditingController>{};
  late String protocol, security, transportType, method, socksVersion;
  bool insecure = false, pending = false;
  String? error;
  Json get source =>
      Map<String, dynamic>.from(widget.node?['outbound'] as Map? ?? {});
  bool get v2 => ['vmess', 'vless', 'trojan'].contains(protocol);
  bool get tlsRequired =>
      ['trojan', 'anytls', 'hysteria', 'hysteria2', 'tuic'].contains(protocol);
  @override
  void initState() {
    super.initState();
    final o = source;
    final tls = Map<String, dynamic>.from(o['tls'] as Map? ?? {});
    final tr = Map<String, dynamic>.from(o['transport'] as Map? ?? {});
    protocol = o['type'] as String? ?? 'shadowsocks';
    security = tls['enabled'] == true
        ? (tls['reality'] != null ? 'reality' : 'tls')
        : 'none';
    insecure = tls['insecure'] == true;
    transportType = tr['type'] as String? ?? 'tcp';
    method = o['method'] as String? ?? 'aes-128-gcm';
    socksVersion = o['version'] as String? ?? '5';
    void put(String k, dynamic value) {
      fields[k] = TextEditingController(text: value?.toString() ?? '');
    }

    put('name', widget.node?['name']);
    put('group', widget.node?['group'] ?? '本地节点');
    for (final k in [
      'server',
      'server_port',
      'uuid',
      'password',
      'flow',
      'security',
      'alter_id',
      'up_mbps',
      'down_mbps',
    ]) {
      put(k, o[k]);
    }
    fields['server_port']!.text = o['server_port']?.toString() ?? '443';
    put('user', o['username'] ?? o['user']);
    put('auth_str', o['auth_str']);
    put('sni', tls['server_name']);
    put('alpn', (tls['alpn'] as List?)?.join(','));
    put('fp', (tls['utls'] as Map?)?['fingerprint']);
    put('public_key', (tls['reality'] as Map?)?['public_key']);
    put('short_id', (tls['reality'] as Map?)?['short_id']);
    put(
      'host',
      (tr['headers'] as Map?)?['Host'] ??
          (tr['host'] is List ? (tr['host'] as List).join(',') : tr['host']),
    );
    put('path', tr['path'] ?? tr['service_name']);
  }

  @override
  void dispose() {
    for (final c in fields.values) {
      c.dispose();
    }
    super.dispose();
  }

  String value(String key) => fields[key]!.text.trim();
  Widget field(String key, String label, {bool secret = false, String? hint}) =>
      Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: TextField(
          controller: fields[key],
          obscureText: secret,
          enableSuggestions: !secret,
          autocorrect: false,
          decoration: InputDecoration(labelText: label, hintText: hint),
        ),
      );
  Widget choice(
    String label,
    String current,
    Map<String, String> options,
    ValueChanged<String> changed,
  ) => Padding(
    padding: const EdgeInsets.only(bottom: 12),
    child: DropdownButtonFormField<String>(
      initialValue: current,
      decoration: InputDecoration(labelText: label),
      items: options.entries
          .map((e) => DropdownMenuItem(value: e.key, child: Text(e.value)))
          .toList(),
      onChanged: pending
          ? null
          : (v) {
              if (v != null) changed(v);
            },
    ),
  );
  Future<void> save() async {
    setState(() {
      pending = true;
      error = null;
    });
    try {
      final port = int.tryParse(value('server_port'));
      if (value('server').isEmpty || port == null || port < 1 || port > 65535) {
        throw Exception('请填写有效的服务器地址和端口');
      }
      final o = widget.node == null
          ? <String, dynamic>{}
          : jsonDecode(jsonEncode(source)) as Json;
      o['type'] = protocol;
      o['server'] = value('server');
      o['server_port'] = port;
      if (['vmess', 'vless', 'tuic'].contains(protocol)) {
        if (!RegExp(
          r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
        ).hasMatch(value('uuid'))) {
          throw Exception('UUID 格式不正确');
        }
        o['uuid'] = value('uuid');
      }
      if ([
        'shadowsocks',
        'trojan',
        'anytls',
        'hysteria2',
        'tuic',
        'socks',
        'http',
        'ssh',
      ].contains(protocol)) {
        o['password'] = fields['password']!.text;
      }
      if (['socks', 'http', 'ssh'].contains(protocol)) {
        o[protocol == 'ssh' ? 'user' : 'username'] = value('user');
      }
      if (protocol == 'shadowsocks') {
        o['method'] = method;
      }
      if (protocol == 'socks') {
        o['version'] = socksVersion;
      }
      if (protocol == 'vless') {
        if (value('flow').isNotEmpty) {
          o['flow'] = value('flow');
        } else {
          o.remove('flow');
        }
      }
      if (protocol == 'vmess') {
        o['security'] = value('security').isEmpty ? 'auto' : value('security');
        o['alter_id'] = int.tryParse(value('alter_id')) ?? 0;
      }
      if (protocol == 'hysteria') {
        o['auth_str'] = fields['auth_str']!.text;
        o['up_mbps'] = int.tryParse(value('up_mbps')) ?? 10;
        o['down_mbps'] = int.tryParse(value('down_mbps')) ?? 50;
      }
      if (security != 'none' || tlsRequired) {
        final tls = Map<String, dynamic>.from(o['tls'] as Map? ?? {});
        tls['enabled'] = true;
        tls['server_name'] = value('sni');
        tls['insecure'] = insecure;
        if (value('alpn').isNotEmpty) {
          tls['alpn'] = value('alpn').split(',').map((s) => s.trim()).toList();
        } else {
          tls.remove('alpn');
        }
        if (value('fp').isNotEmpty || security == 'reality') {
          tls['utls'] = {
            'enabled': true,
            'fingerprint': value('fp').isEmpty ? 'chrome' : value('fp'),
          };
        } else {
          tls.remove('utls');
        }
        if (security == 'reality') {
          if (value('public_key').isEmpty) {
            throw Exception('Reality 公钥不能为空');
          }
          tls['reality'] = {
            'enabled': true,
            'public_key': value('public_key'),
            'short_id': value('short_id'),
          };
        } else {
          tls.remove('reality');
        }
        o['tls'] = tls;
      } else {
        o.remove('tls');
      }
      if (v2 && transportType != 'tcp') {
        final tr = Map<String, dynamic>.from(o['transport'] as Map? ?? {});
        tr['type'] = transportType;
        if (transportType == 'grpc') {
          tr['service_name'] = value('path');
          tr.remove('path');
          tr.remove('host');
          tr.remove('headers');
        } else {
          tr['path'] = value('path');
          tr.remove('service_name');
          if (transportType == 'ws') {
            tr['headers'] = {
              ...Map<String, dynamic>.from(tr['headers'] as Map? ?? {}),
              'Host': value('host'),
            };
            tr.remove('host');
          } else {
            tr['host'] = transportType == 'http'
                ? value('host').split(',')
                : value('host');
            tr.remove('headers');
          }
        }
        o['transport'] = tr;
      } else {
        o.remove('transport');
      }
      final node = <String, dynamic>{
        ...?widget.node,
        'name': value('name').isEmpty ? value('server') : value('name'),
        'group': value('group'),
        'outbound': o,
        'latency_ms': -1,
      };
      await widget.onSave(node);
      if (mounted) Navigator.pop(context, 'saved');
    } catch (e) {
      if (mounted) setState(() => error = Backend.clean(e));
    } finally {
      if (mounted) setState(() => pending = false);
    }
  }

  @override
  Widget build(BuildContext context) => AlertDialog(
    title: Text(widget.node == null ? '手动添加节点' : '编辑节点'),
    content: SizedBox(
      width: 580,
      height: 540,
      child: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Expanded(child: field('name', '名称')),
                const SizedBox(width: 12),
                Expanded(child: field('group', '分组')),
              ],
            ),
            if (widget.node == null)
              choice(
                '协议',
                protocol,
                NodeForm.protocols,
                (v) => setState(() {
                  protocol = v;
                  security = tlsRequired ? 'tls' : 'none';
                  transportType = 'tcp';
                  fields['server_port']!.text = v == 'socks'
                      ? '1080'
                      : v == 'ssh'
                      ? '22'
                      : v == 'http'
                      ? '8080'
                      : '443';
                }),
              )
            else
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Chip(
                    label: Text(NodeForm.protocols[protocol] ?? protocol),
                  ),
                ),
              ),
            Row(
              children: [
                Expanded(flex: 3, child: field('server', '服务器地址')),
                const SizedBox(width: 12),
                Expanded(child: field('server_port', '端口')),
              ],
            ),
            if (['vmess', 'vless', 'tuic'].contains(protocol))
              field('uuid', 'UUID'),
            if (['socks', 'http', 'ssh'].contains(protocol))
              field('user', '用户名'),
            if ([
              'shadowsocks',
              'trojan',
              'anytls',
              'hysteria2',
              'tuic',
              'socks',
              'http',
              'ssh',
            ].contains(protocol))
              field('password', '密码', secret: true),
            if (protocol == 'shadowsocks')
              choice('加密方式', method, {
                for (final v in {
                  'aes-128-gcm',
                  'aes-256-gcm',
                  'chacha20-ietf-poly1305',
                  '2022-blake3-aes-128-gcm',
                  '2022-blake3-aes-256-gcm',
                  '2022-blake3-chacha20-poly1305',
                  'none',
                  method,
                })
                  v: v,
              }, (v) => setState(() => method = v)),
            if (protocol == 'socks')
              choice('版本', socksVersion, {
                '5': 'SOCKS 5',
                '4': 'SOCKS 4',
                '4a': 'SOCKS 4a',
              }, (v) => setState(() => socksVersion = v)),
            if (protocol == 'vmess')
              Row(
                children: [
                  Expanded(child: field('security', '加密方式', hint: 'auto')),
                  const SizedBox(width: 12),
                  Expanded(child: field('alter_id', 'Alter ID', hint: '0')),
                ],
              ),
            if (protocol == 'vless')
              field('flow', 'Flow', hint: '留空或 xtls-rprx-vision'),
            if (protocol == 'hysteria') ...[
              field('auth_str', '认证字符串', secret: true),
              Row(
                children: [
                  Expanded(child: field('up_mbps', '上传 Mbps', hint: '10')),
                  const SizedBox(width: 12),
                  Expanded(child: field('down_mbps', '下载 Mbps', hint: '50')),
                ],
              ),
            ],
            if (v2)
              choice('传输方式', transportType, {
                'tcp': 'TCP',
                'ws': 'WebSocket',
                'grpc': 'gRPC',
                'http': 'HTTP/2',
                'httpupgrade': 'HTTP Upgrade',
                if (transportType == 'quic') 'quic': 'QUIC',
              }, (v) => setState(() => transportType = v)),
            if (v2 && transportType != 'tcp' && transportType != 'quic') ...[
              if (transportType != 'grpc') field('host', 'Host'),
              field('path', transportType == 'grpc' ? 'Service name' : '路径'),
            ],
            if (['vmess', 'vless', 'http'].contains(protocol))
              choice('安全层', security, {
                'none': '无',
                'tls': 'TLS',
                if (protocol == 'vless') 'reality': 'Reality',
              }, (v) => setState(() => security = v)),
            if (tlsRequired || security != 'none') ...[
              field('sni', 'TLS 服务器名称（SNI）', hint: '留空时使用服务器地址'),
              field('alpn', 'ALPN', hint: '多个值用逗号分隔'),
              field('fp', 'TLS 指纹', hint: '留空或 chrome / firefox / safari'),
              if (security == 'reality') ...[
                field('public_key', 'Reality 公钥'),
                field('short_id', 'Reality Short ID'),
              ],
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('跳过证书验证'),
                value: insecure,
                onChanged: (v) => setState(() => insecure = v),
              ),
            ],
            if (error != null)
              Align(
                alignment: Alignment.centerLeft,
                child: Text(
                  error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ),
          ],
        ),
      ),
    ),
    actions: [
      TextButton(
        onPressed: pending ? null : () => Navigator.pop(context, 'advanced'),
        child: const Text('高级 JSON'),
      ),
      TextButton(
        onPressed: pending ? null : () => Navigator.pop(context),
        child: const Text('取消'),
      ),
      FilledButton(
        onPressed: pending ? null : save,
        child: Text(pending ? '保存中…' : '保存'),
      ),
    ],
  );
}
