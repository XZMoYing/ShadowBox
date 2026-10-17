import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/services.dart';
import 'package:material_ui/material_ui.dart';
import 'package:file_selector/file_selector.dart';
import 'package:image/image.dart' as image;
import 'package:zxing2/qrcode.dart';

import 'backend.dart';
import 'node_form.dart';

Future<T?> managedDialog<T>({
  required BuildContext context,
  required WidgetBuilder builder,
}) async {
  final route = DialogRoute<T>(context: context, builder: builder);
  final value = await Navigator.of(context, rootNavigator: true).push(route);
  await route.completed;
  return value;
}

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ShadowBox());
}

class ShadowBox extends StatefulWidget {
  final Backend? backend;
  const ShadowBox({super.key, this.backend});
  @override
  State<ShadowBox> createState() => _ShadowBoxState();
}

class _ShadowBoxState extends State<ShadowBox> {
  late final Backend backend = widget.backend ?? Backend();
  @override
  void initState() {
    super.initState();
    backend.initialize();
  }

  @override
  void dispose() {
    backend.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: backend,
    builder: (context, _) {
      const colors = {
        'green': Color(0xFF286C53),
        'amber': Color(0xFF91651B),
        'coral': Color(0xFFAA5140),
        'blue': Color(0xFF356791),
        'violet': Color(0xFF76518E),
      };
      final seed = colors[backend.settings['color']] ?? colors['green']!;
      ThemeData theme(Brightness b) {
        final scheme = ColorScheme.fromSeed(seedColor: seed, brightness: b);
        return ThemeData(
          colorScheme: scheme,
          scaffoldBackgroundColor: b == Brightness.light
              ? const Color(0xFFF5F7F2)
              : const Color(0xFF141916),
          visualDensity: backend.settings['compact'] == true
              ? VisualDensity.compact
              : VisualDensity.standard,
          cardTheme: CardThemeData(
            elevation: 0,
            color: scheme.surfaceContainerLow,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(26),
            ),
          ),
          inputDecorationTheme: InputDecorationTheme(
            filled: true,
            fillColor: scheme.surfaceContainerLow,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(16),
              borderSide: BorderSide.none,
            ),
          ),
          filledButtonTheme: FilledButtonThemeData(
            style: FilledButton.styleFrom(
              padding: const EdgeInsets.symmetric(horizontal: 22, vertical: 18),
            ),
          ),
          dividerTheme: DividerThemeData(
            color: scheme.outlineVariant.withValues(alpha: .5),
          ),
        );
      }

      return MaterialApp(
        debugShowCheckedModeBanner: false,
        title: 'ShadowBox',
        theme: theme(Brightness.light),
        darkTheme: theme(Brightness.dark),
        themeMode: switch (backend.settings['theme']) {
          'light' => ThemeMode.light,
          'dark' => ThemeMode.dark,
          _ => ThemeMode.system,
        },
        home: Shell(backend: backend),
      );
    },
  );
}

class Shell extends StatefulWidget {
  final Backend backend;
  const Shell({super.key, required this.backend});
  @override
  State<Shell> createState() => _ShellState();
}

class _ShellState extends State<Shell> {
  int page = 0;
  String search = '', group = '全部';
  bool favorites = false;
  final testing = <String>{};
  Backend get b => widget.backend;
  ColorScheme get c => Theme.of(context).colorScheme;
  @override
  Widget build(BuildContext context) {
    if (b.loading) {
      return const Scaffold(body: Center(child: CircularProgressIndicator()));
    }
    if (b.endpoint.isEmpty) {
      return Scaffold(
        body: Center(
          child: SizedBox(
            width: 440,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.cloud_off_rounded, size: 42),
                const SizedBox(height: 20),
                Text(b.error ?? '后台服务未启动', textAlign: TextAlign.center),
                const SizedBox(height: 20),
                FilledButton.icon(
                  onPressed: b.initialize,
                  icon: const Icon(Icons.refresh),
                  label: const Text('重新连接后台'),
                ),
              ],
            ),
          ),
        ),
      );
    }
    final destinations = [
      const NavigationRailDestination(
        icon: Icon(Icons.power_settings_new_rounded),
        label: Text('连接'),
      ),
      const NavigationRailDestination(
        icon: Icon(Icons.dns_outlined),
        label: Text('节点'),
      ),
      const NavigationRailDestination(
        icon: Icon(Icons.sync_rounded),
        label: Text('订阅'),
      ),
      const NavigationRailDestination(
        icon: Icon(Icons.alt_route_rounded),
        label: Text('路由'),
      ),
      const NavigationRailDestination(
        icon: Icon(Icons.monitor_heart_outlined),
        label: Text('活动'),
      ),
      const NavigationRailDestination(
        icon: Icon(Icons.tune_rounded),
        label: Text('设置'),
      ),
    ];
    return Scaffold(
      body: SafeArea(
        child: Row(
          children: [
            NavigationRail(
              selectedIndex: page,
              onDestinationSelected: (i) {
                setState(() => page = i);
                if (i == 4) loadActivity();
              },
              labelType: NavigationRailLabelType.all,
              backgroundColor: Colors.transparent,
              leading: Padding(
                padding: const EdgeInsets.symmetric(vertical: 18),
                child: Icon(
                  Icons.radio_button_checked_rounded,
                  color: c.primary,
                  size: 27,
                ),
              ),
              destinations: destinations,
              trailing: Padding(
                padding: const EdgeInsets.only(top: 20),
                child: Tooltip(
                  message: b.running ? '代理运行中' : '代理已停止',
                  child: Icon(
                    Icons.circle,
                    size: 9,
                    color: b.running ? c.primary : c.outline,
                  ),
                ),
              ),
            ),
            Expanded(
              child: Column(
                children: [
                  if (b.error != null)
                    MaterialBanner(
                      content: Text(b.error!, maxLines: 4),
                      actions: [
                        TextButton(
                          onPressed: () => setState(() => b.error = null),
                          child: const Text('关闭'),
                        ),
                      ],
                    ),
                  if (b.busy) const LinearProgressIndicator(minHeight: 2),
                  Expanded(
                    child: Padding(
                      padding: const EdgeInsets.fromLTRB(24, 24, 28, 24),
                      child: switch (page) {
                        0 => dashboard(),
                        1 => nodesPage(),
                        2 => subscriptionsPage(),
                        3 => routingPage(),
                        4 => activityPage(),
                        _ => settingsPage(),
                      },
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget dashboard() {
    final selected = b.selectedNode;
    final stats = (b.status['stats'] as Map?) ?? {};
    return ListView(
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Chip(
              avatar: Icon(
                b.running ? Icons.check_circle_rounded : Icons.circle_outlined,
                size: 17,
              ),
              label: Text(b.running ? '已连接' : '未连接'),
            ),
            Text(
              'Neko ${b.status['core_version'] ?? ''}',
              style: TextStyle(color: c.onSurfaceVariant, fontSize: 12),
            ),
          ],
        ),
        const SizedBox(height: 24),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              children: [
                SizedBox(
                  width: 100,
                  height: 100,
                  child: Semantics(
                    label: b.running ? '暂停代理' : '开始连接',
                    child: FilledButton.tonal(
                      onPressed: b.busy
                          ? null
                          : () => b.action(b.running ? b.stop : b.connect),
                      style: FilledButton.styleFrom(
                        shape: const CircleBorder(),
                        backgroundColor: b.running
                            ? c.primary
                            : c.secondaryContainer,
                        foregroundColor: b.running
                            ? c.onPrimary
                            : c.onSecondaryContainer,
                      ),
                      child: b.busy
                          ? const SizedBox(
                              width: 26,
                              height: 26,
                              child: CircularProgressIndicator(strokeWidth: 2),
                            )
                          : const Icon(
                              Icons.power_settings_new_rounded,
                              size: 40,
                            ),
                    ),
                  ),
                ),
                const SizedBox(height: 22),
                Text(
                  b.running
                      ? b.status['active_name'].toString()
                      : selected?['name']?.toString() ?? '请选择节点',
                  style: Theme.of(context).textTheme.titleLarge,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 10),
                Text(
                  b.running
                      ? '点击暂停代理'
                      : selected == null
                      ? '从节点或订阅中添加配置'
                      : '点击开始连接',
                  style: TextStyle(color: c.onSurfaceVariant),
                ),
                const SizedBox(height: 22),
                OutlinedButton.icon(
                  onPressed: () => setState(() => page = 1),
                  icon: const Icon(Icons.swap_horiz_rounded),
                  label: Text(
                    selected == null ? '选择节点' : '${nodeType(selected)} · 切换节点',
                  ),
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 18),
        Row(
          children: [
            Expanded(
              child: statCard(
                Icons.north_east_rounded,
                '上传',
                byteLabel(stats['upload'] as num?),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: statCard(
                Icons.south_west_rounded,
                '下载',
                byteLabel(stats['download'] as num?),
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: statCard(
                Icons.hub_outlined,
                '连接',
                '${stats['connections'] ?? 0}',
              ),
            ),
          ],
        ),
        const SizedBox(height: 18),
        Card(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Wrap(
              spacing: 24,
              runSpacing: 20,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                settingDropdown(
                  '接管方式',
                  b.settings['capture'] ?? 'system',
                  const {
                    'system': '系统代理',
                    'tun': 'TUN 虚拟网卡',
                    'manual': '仅本地代理',
                  },
                  (v) => changeSetting('capture', v),
                ),
                settingDropdown('分流策略', b.settings['routing'] ?? 'rule', const {
                  'rule': '按规则',
                  'global': '全局代理',
                  'direct': '全局直连',
                }, (v) => changeSetting('routing', v)),
                SelectableText(
                  '127.0.0.1:${b.settings['port'] ?? 10808}',
                  style: TextStyle(color: c.onSurfaceVariant),
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 16),
        Text(
          '关闭窗口后，界面进程退出；代理与托盘继续运行。完整退出请使用托盘菜单。',
          style: TextStyle(color: c.onSurfaceVariant, fontSize: 12),
        ),
      ],
    );
  }

  Widget statCard(IconData icon, String label, String value) => Card(
    child: Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(icon, size: 20, color: c.primary),
          const SizedBox(height: 18),
          Text(value, style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 4),
          Text(label, style: TextStyle(color: c.onSurfaceVariant)),
        ],
      ),
    ),
  );
  Widget settingDropdown(
    String label,
    String value,
    Map<String, String> values,
    ValueChanged<String> onChange,
  ) => SizedBox(
    width: 205,
    child: DropdownButtonFormField<String>(
      initialValue: values.containsKey(value) ? value : values.keys.first,
      decoration: InputDecoration(labelText: label),
      items: values.entries
          .map((e) => DropdownMenuItem(value: e.key, child: Text(e.value)))
          .toList(),
      onChanged: b.busy
          ? null
          : (v) {
              if (v != null) onChange(v);
            },
    ),
  );
  void changeSetting(String key, dynamic value) {
    final s = Json.from(b.settings)..[key] = value;
    b.action(() => b.setSettings(s));
  }

  List<Json> get visibleNodes => b.nodes
      .where(
        (n) =>
            (group == '全部' || n['group'] == group) &&
            (!favorites || n['favorite'] == true) &&
            '${n['name']} ${nodeType(n)} ${n['outbound']?['server'] ?? ''}'
                .toLowerCase()
                .contains(search.toLowerCase()),
      )
      .toList();

  Widget nodesPage() {
    final nodes = visibleNodes;
    final groups = {
      '全部',
      ...b.nodes.map((n) => (n['group'] ?? '本地节点').toString()),
    };
    return Column(
      children: [
        Row(
          children: [
            Expanded(
              child: TextField(
                decoration: const InputDecoration(
                  hintText: '搜索名称、协议或地址',
                  prefixIcon: Icon(Icons.search_rounded),
                ),
                onChanged: (v) => setState(() => search = v),
              ),
            ),
            const SizedBox(width: 12),
            IconButton.filledTonal(
              onPressed: b.busy
                  ? null
                  : () => setState(() => favorites = !favorites),
              tooltip: '只看收藏',
              icon: Icon(
                favorites ? Icons.star_rounded : Icons.star_outline_rounded,
              ),
            ),
            const SizedBox(width: 10),
            FilledButton.icon(
              onPressed: b.busy ? null : importDialog,
              icon: const Icon(Icons.add_rounded),
              label: const Text('添加'),
            ),
            PopupMenuButton<String>(
              tooltip: '更多操作',
              onSelected: (v) {
                if (v == 'manual') nodeEditor();
                if (v == 'chain') chainEditor();
                if (v == 'file') importFile();
                if (v == 'qr') importFile(qr: true);
                if (v == 'test') testVisible();
              },
              itemBuilder: (_) => const [
                PopupMenuItem(value: 'manual', child: Text('手动填写')),
                PopupMenuItem(value: 'chain', child: Text('新建代理链')),
                PopupMenuItem(value: 'file', child: Text('从文件导入')),
                PopupMenuItem(value: 'qr', child: Text('从二维码图片导入')),
                PopupMenuItem(value: 'test', child: Text('测试当前列表')),
              ],
            ),
          ],
        ),
        const SizedBox(height: 14),
        SizedBox(
          height: 40,
          child: ListView(
            scrollDirection: Axis.horizontal,
            children: groups
                .map(
                  (g) => Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: ChoiceChip(
                      label: Text(g),
                      selected: group == g,
                      onSelected: (_) => setState(() => group = g),
                    ),
                  ),
                )
                .toList(),
          ),
        ),
        const SizedBox(height: 12),
        Expanded(
          child: nodes.isEmpty
              ? empty(Icons.add_link_rounded, '添加节点或订阅', importDialog)
              : ListView.separated(
                  itemCount: nodes.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 6),
                  itemBuilder: (context, index) {
                    final n = nodes[index];
                    final selected = n['id'] == b.status['selected'];
                    final active = n['id'] == b.status['active_id'];
                    return Material(
                      color: selected
                          ? c.secondaryContainer
                          : c.surfaceContainerLow,
                      borderRadius: BorderRadius.circular(20),
                      child: ListTile(
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(20),
                        ),
                        onTap: b.busy
                            ? null
                            : () => b.action(() => b.select(n['id'])),
                        leading: Icon(
                          active
                              ? Icons.play_circle_filled_rounded
                              : selected
                              ? Icons.radio_button_checked_rounded
                              : Icons.radio_button_off_rounded,
                          color: selected ? c.primary : c.outline,
                        ),
                        title: Text(
                          n['name']?.toString() ?? '',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                        subtitle: Text(
                          '${nodeType(n)} · ${n['group'] ?? ''}${active ? ' · 使用中' : ''}',
                          style: const TextStyle(fontSize: 12),
                        ),
                        trailing: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            if (testing.contains(n['id']))
                              const SizedBox(
                                width: 18,
                                height: 18,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                ),
                              )
                            else
                              TextButton(
                                onPressed: () => testNode(n),
                                child: Text(
                                  (n['latency_ms'] as num? ?? -1) >= 0
                                      ? '${n['latency_ms']} ms'
                                      : '测试',
                                ),
                              ),
                            IconButton(
                              onPressed: () => b.action(() async {
                                final value = Json.from(n)
                                  ..['favorite'] = n['favorite'] != true;
                                await b.request(
                                  '/v1/nodes',
                                  method: 'POST',
                                  body: {
                                    'nodes': [value],
                                  },
                                );
                              }),
                              tooltip: '收藏',
                              icon: Icon(
                                n['favorite'] == true
                                    ? Icons.star_rounded
                                    : Icons.star_outline_rounded,
                                size: 20,
                              ),
                            ),
                            PopupMenuButton<String>(
                              tooltip: '节点操作',
                              onSelected: (v) {
                                if (v == 'edit') {
                                  if (nodeType(n) == '代理链') {
                                    chainEditor(n);
                                  } else {
                                    nodeEditor(n);
                                  }
                                }
                                if (v == 'copy') {
                                  Clipboard.setData(
                                    ClipboardData(
                                      text: const JsonEncoder.withIndent('  ')
                                          .convert({
                                            ...?n['outbound'] as Map?,
                                            'tag': n['name'],
                                          }),
                                    ),
                                  );
                                }
                                if (v == 'connect') {
                                  b.action(() => b.connect(n['id']));
                                }
                                if (v == 'delete') deleteNode(n);
                              },
                              itemBuilder: (_) => const [
                                PopupMenuItem(
                                  value: 'connect',
                                  child: Text('连接此节点'),
                                ),
                                PopupMenuItem(value: 'edit', child: Text('编辑')),
                                PopupMenuItem(
                                  value: 'copy',
                                  child: Text('复制出站 JSON'),
                                ),
                                PopupMenuItem(
                                  value: 'delete',
                                  child: Text('删除'),
                                ),
                              ],
                            ),
                          ],
                        ),
                      ),
                    );
                  },
                ),
        ),
      ],
    );
  }

  Widget empty(IconData icon, String text, VoidCallback action) => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 42, color: c.primary),
        const SizedBox(height: 16),
        OutlinedButton(onPressed: action, child: Text(text)),
      ],
    ),
  );
  Future<void> testNode(Json n) async {
    if (testing.contains(n['id'])) return;
    setState(() => testing.add(n['id'] as String));
    try {
      await b.request('/v1/test', method: 'POST', body: {'id': n['id']});
      await b.refresh();
    } catch (e) {
      toast('${n['name']}：${Backend.clean(e)}');
    } finally {
      if (mounted) setState(() => testing.remove(n['id']));
    }
  }

  Future<void> testVisible() async {
    final nodes = visibleNodes.take(100).toList();
    for (var i = 0; i < nodes.length && mounted; i += 2) {
      await Future.wait(nodes.skip(i).take(2).map(testNode));
    }
  }

  void toast(String text) {
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(text)));
    }
  }

  Future<bool> confirm(String text) async =>
      await managedDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          content: Text(text),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('确定'),
            ),
          ],
        ),
      ) ??
      false;
  Future<void> deleteNode(Json n) async {
    if (await confirm('删除节点“${n['name']}”？')) {
      await b.action(() async {
        await b.request('/v1/nodes/${n['id']}', method: 'DELETE');
      });
    }
  }

  Future<void> importDialog({String initial = ''}) async {
    final text = TextEditingController(text: initial);
    final name = TextEditingController();
    var subscription =
        initial.trim().startsWith('https://') ||
        initial.trim().startsWith('http://');
    String? error;
    bool pending = false;
    await managedDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, update) => AlertDialog(
          title: const Text('添加配置'),
          content: SizedBox(
            width: 560,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                SegmentedButton<bool>(
                  segments: const [
                    ButtonSegment(value: false, label: Text('节点链接 / JSON')),
                    ButtonSegment(value: true, label: Text('订阅链接')),
                  ],
                  selected: {subscription},
                  onSelectionChanged: (v) => update(() {
                    subscription = v.first;
                    error = null;
                  }),
                ),
                const SizedBox(height: 18),
                if (subscription) ...[
                  TextField(
                    controller: name,
                    decoration: const InputDecoration(labelText: '订阅名称'),
                  ),
                  const SizedBox(height: 12),
                ],
                TextField(
                  controller: text,
                  minLines: subscription ? 1 : 6,
                  maxLines: subscription ? 3 : 12,
                  decoration: InputDecoration(
                    hintText: subscription
                        ? 'https://…'
                        : '粘贴一个或多个节点链接、Base64 或 sing-box 出站配置',
                  ),
                ),
                if (error != null)
                  Padding(
                    padding: const EdgeInsets.only(top: 12),
                    child: Text(error!, style: TextStyle(color: c.error)),
                  ),
                if (pending)
                  const Padding(
                    padding: EdgeInsets.only(top: 12),
                    child: LinearProgressIndicator(),
                  ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: pending ? null : () => Navigator.pop(ctx),
              child: const Text('取消'),
            ),
            TextButton(
              onPressed: pending
                  ? null
                  : () async {
                      final clip = await Clipboard.getData(
                        Clipboard.kTextPlain,
                      );
                      text.text = clip?.text ?? '';
                    },
              child: const Text('粘贴'),
            ),
            FilledButton(
              onPressed: pending
                  ? null
                  : () async {
                      update(() {
                        pending = true;
                        error = null;
                      });
                      try {
                        if (subscription) {
                          await b.request(
                            '/v1/subscriptions',
                            method: 'POST',
                            body: {
                              'name': name.text.trim(),
                              'url': text.text.trim(),
                              'interval_hours': 24,
                            },
                          );
                          if (ctx.mounted) Navigator.pop(ctx);
                          await b.refresh();
                        } else {
                          final result = Map<String, dynamic>.from(
                            await b.request(
                              '/v1/import/preview',
                              method: 'POST',
                              body: {'text': text.text},
                            ) as Map,
                          );
                          if (ctx.mounted) Navigator.pop(ctx);
                          await previewImport(result);
                        }
                      } catch (e) {
                        update(() => error = Backend.clean(e));
                      } finally {
                        if (ctx.mounted) update(() => pending = false);
                      }
                    },
              child: Text(subscription ? '添加订阅' : '预览节点'),
            ),
          ],
        ),
      ),
    );
    text.dispose();
    name.dispose();
  }

  Future<void> previewImport(Json result) async {
    final nodes = Backend.maps(result['nodes']);
    final errors = (result['errors'] as List?) ?? [];
    final groupName = TextEditingController(text: '本地节点');
    final ok =
        await managedDialog<bool>(
          context: context,
          builder: (ctx) => AlertDialog(
            title: Text('识别到 ${nodes.length} 个节点'),
            content: SizedBox(
              width: 560,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  TextField(
                    controller: groupName,
                    decoration: const InputDecoration(labelText: '保存到分组'),
                  ),
                  const SizedBox(height: 14),
                  Flexible(
                    child: SingleChildScrollView(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          for (final n in nodes.take(20))
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 5),
                              child: Text('${n['name']} · ${nodeType(n)}'),
                            ),
                          if (nodes.length > 20)
                            Text('还有 ${nodes.length - 20} 个节点'),
                          for (final e in errors.take(10))
                            Padding(
                              padding: const EdgeInsets.only(top: 8),
                              child: Text(
                                e.toString(),
                                style: TextStyle(color: c.error),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(ctx, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: nodes.isEmpty
                    ? null
                    : () => Navigator.pop(ctx, true),
                child: const Text('导入可用节点'),
              ),
            ],
          ),
        ) ??
        false;
    if (ok) {
      for (final n in nodes) {
        n['group'] = groupName.text.trim();
      }
      await b.action(() async {
        await b.request('/v1/nodes', method: 'POST', body: {'nodes': nodes});
      });
    }
    groupName.dispose();
  }

  Future<void> importFile({bool qr = false}) async {
    try {
      final file = await openFile(
        acceptedTypeGroups: [
          qr
              ? const XTypeGroup(
                  label: '二维码图片',
                  extensions: ['png', 'jpg', 'jpeg', 'webp', 'bmp'],
                )
              : const XTypeGroup(
                  label: '配置文件',
                  extensions: ['txt', 'json', 'yaml', 'yml', 'conf', 'zip'],
                ),
        ],
      );
      if (file == null) return;
      if (await file.length() > 16 * 1024 * 1024) {
        throw Exception('文件不能超过 16 MiB');
      }
      final bytes = await file.readAsBytes();
      if (qr) {
        final decoded = image.decodeImage(bytes);
        if (decoded == null || decoded.width * decoded.height > 12000000) {
          throw Exception('图片无法读取或分辨率过大');
        }
        final pixels = Int32List(decoded.width * decoded.height);
        for (final pixel in decoded) {
          pixels[pixel.y * decoded.width + pixel.x] =
              (pixel.r.toInt() << 16) |
              (pixel.g.toInt() << 8) |
              pixel.b.toInt();
        }
        final value = QRCodeReader()
            .decode(
              BinaryBitmap(
                HybridBinarizer(
                  RGBLuminanceSource(decoded.width, decoded.height, pixels),
                ),
              ),
            )
            .text;
        await importDialog(initial: value);
      } else {
        final result = Map<String, dynamic>.from(
          await b.request(
            '/v1/import/file',
            method: 'POST',
            bytes: bytes,
            filename: file.name,
          ) as Map,
        );
        await previewImport(result);
      }
    } catch (e) {
      toast('导入失败：${Backend.clean(e)}');
    }
  }

  Future<void> nodeEditor([Json? original]) async {
    final type = (original?['outbound'] as Map?)?['type'];
    if (original != null && !NodeForm.protocols.containsKey(type)) {
      await jsonNodeEditor(original);
      return;
    }
    final result = await managedDialog<String>(
      context: context,
      builder: (ctx) => NodeForm(
        node: original,
        onSave: (node) async {
          await b.request(
            '/v1/nodes',
            method: 'POST',
            body: {
              'nodes': [node],
            },
          );
          if (original != null && b.status['active_id'] == original['id']) {
            await b.connect(original['id'] as String);
          }
          await b.refresh();
        },
      ),
    );
    if (result == 'advanced' && mounted) {
      await jsonNodeEditor(original);
    }
  }

  Future<void> jsonNodeEditor([Json? original]) async {
    final name = TextEditingController(
      text: original?['name']?.toString() ?? '',
    );
    final groupName = TextEditingController(
      text: original?['group']?.toString() ?? '本地节点',
    );
    final raw = TextEditingController(
      text: const JsonEncoder.withIndent('  ').convert(
        original?['external'] != null
            ? {'external': original!['external']}
            : original?['outbound'] ??
                  {
                    'type': 'trojan',
                    'server': '',
                    'server_port': 443,
                    'password': '',
                    'tls': {'enabled': true, 'server_name': ''},
                  },
      ),
    );
    String? error;
    await managedDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, update) => AlertDialog(
          title: const Text('节点配置'),
          content: SizedBox(
            width: 630,
            child: SingleChildScrollView(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: name,
                          decoration: const InputDecoration(labelText: '名称'),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: TextField(
                          controller: groupName,
                          decoration: const InputDecoration(labelText: '分组'),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 14),
                  TextField(
                    controller: raw,
                    minLines: 12,
                    maxLines: 20,
                    style: const TextStyle(
                      fontFamily: 'monospace',
                      fontSize: 12,
                    ),
                    decoration: const InputDecoration(
                      labelText: 'sing-box 出站 JSON / external 插件配置',
                    ),
                  ),
                  if (error != null)
                    Padding(
                      padding: const EdgeInsets.only(top: 10),
                      child: Text(error!, style: TextStyle(color: c.error)),
                    ),
                ],
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () async {
                try {
                  final value = jsonDecode(raw.text) as Json;
                  if (name.text.trim().isEmpty) throw Exception('请填写名称');
                  final n = <String, dynamic>{
                    ...?original,
                    'name': name.text.trim(),
                    'group': groupName.text.trim(),
                    'latency_ms': -1,
                  };
                  if (value['external'] != null) {
                    n.remove('outbound');
                    n['external'] = value['external'];
                  } else {
                    n.remove('external');
                    n['outbound'] = value;
                  }
                  await b.request(
                    '/v1/nodes',
                    method: 'POST',
                    body: {
                      'nodes': [n],
                    },
                  );
                  if (n['id'] == b.status['active_id']) {
                    await b.connect(n['id']);
                  }
                  if (ctx.mounted) Navigator.pop(ctx);
                  await b.refresh();
                } catch (e) {
                  update(() => error = Backend.clean(e));
                }
              },
              child: const Text('保存'),
            ),
          ],
        ),
      ),
    );
    name.dispose();
    groupName.dispose();
    raw.dispose();
  }

  Future<void> chainEditor([Json? original]) async {
    final name = TextEditingController(
      text: original?['name']?.toString() ?? '',
    );
    final ids = List<String>.from(original?['chain'] as List? ?? []);
    String? error;
    String? candidate;
    await managedDialog<void>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, update) => AlertDialog(
          title: const Text('代理链'),
          content: SizedBox(
            width: 560,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                TextField(
                  controller: name,
                  decoration: const InputDecoration(labelText: '名称'),
                ),
                const SizedBox(height: 16),
                const Align(
                  alignment: Alignment.centerLeft,
                  child: Text('流量从上到下，最后一个节点为出口。'),
                ),
                const SizedBox(height: 10),
                for (var i = 0; i < ids.length; i++)
                  ListTile(
                    dense: true,
                    leading: Text('${i + 1}'),
                    title: Text(
                      b.nodes
                              .where((n) => n['id'] == ids[i])
                              .firstOrNull?['name']
                              ?.toString() ??
                          '节点已不存在',
                    ),
                    trailing: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        IconButton(
                          tooltip: '上移一跳',
                          onPressed: i == 0
                              ? null
                              : () => update(() {
                                  final v = ids.removeAt(i);
                                  ids.insert(i - 1, v);
                                }),
                          icon: const Icon(
                            Icons.arrow_upward_rounded,
                            size: 18,
                          ),
                        ),
                        IconButton(
                          tooltip: '移除此跳',
                          onPressed: () => update(() => ids.removeAt(i)),
                          icon: const Icon(Icons.close_rounded, size: 18),
                        ),
                      ],
                    ),
                  ),
                Row(
                  children: [
                    Expanded(
                      child: DropdownButtonFormField<String>(
                        initialValue: candidate,
                        decoration: const InputDecoration(labelText: '选择下一跳'),
                        items: b.nodes
                            .where((n) => n['id'] != original?['id'])
                            .map(
                              (n) => DropdownMenuItem(
                                value: n['id'] as String,
                                child: Text(
                                  n['name'].toString(),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                            )
                            .toList(),
                        onChanged: (v) => candidate = v,
                      ),
                    ),
                    const SizedBox(width: 8),
                    IconButton.filledTonal(
                      tooltip: '添加下一跳',
                      onPressed: () {
                        if (candidate != null && ids.length < 8) {
                          update(() => ids.add(candidate!));
                        }
                      },
                      icon: const Icon(Icons.add_rounded),
                    ),
                  ],
                ),
                if (error != null)
                  Text(error!, style: TextStyle(color: c.error)),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () async {
                try {
                  if (ids.length < 2 || name.text.trim().isEmpty) {
                    throw Exception('填写名称并选择至少两个节点');
                  }
                  final n = <String, dynamic>{
                    ...?original,
                    'name': name.text.trim(),
                    'group': original?['group'] ?? '代理链',
                    'chain': ids,
                    'latency_ms': -1,
                  };
                  await b.request(
                    '/v1/nodes',
                    method: 'POST',
                    body: {
                      'nodes': [n],
                    },
                  );
                  if (ctx.mounted) Navigator.pop(ctx);
                  await b.refresh();
                } catch (e) {
                  update(() => error = Backend.clean(e));
                }
              },
              child: const Text('保存'),
            ),
          ],
        ),
      ),
    );
    name.dispose();
  }

  Widget subscriptionsPage() => Column(
    children: [
      Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          OutlinedButton.icon(
            onPressed: b.busy
                ? null
                : () => b.action(() async {
                    for (final sub in b.subscriptions) {
                      await b.request(
                        '/v1/subscriptions/${sub['id']}/refresh',
                        method: 'POST',
                      );
                    }
                  }),
            icon: const Icon(Icons.sync),
            label: const Text('全部更新'),
          ),
          const SizedBox(width: 12),
          FilledButton.icon(
            onPressed: () => importDialog(initial: 'https://'),
            icon: const Icon(Icons.add_rounded),
            label: const Text('添加订阅'),
          ),
        ],
      ),
      const SizedBox(height: 18),
      Expanded(
        child: b.subscriptions.isEmpty
            ? empty(
                Icons.sync_rounded,
                '添加订阅链接',
                () => importDialog(initial: 'https://'),
              )
            : ListView(
                children: [
                  for (final s in b.subscriptions)
                    Card(
                      child: Padding(
                        padding: const EdgeInsets.all(14),
                        child: ListTile(
                          leading: const Icon(Icons.link_rounded),
                          title: Text(s['name'].toString()),
                          subtitle: Text(
                            '${b.nodes.where((n) => n['source'] == s['id']).length} 个节点 · 每 ${s['interval_hours']} 小时更新${(s['error'] ?? '').toString().isNotEmpty ? '\n${s['error']}' : ''}',
                          ),
                          trailing: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              IconButton(
                                onPressed: b.busy
                                    ? null
                                    : () => b.action(() async {
                                        await b.request(
                                          '/v1/subscriptions/${s['id']}/refresh',
                                          method: 'POST',
                                        );
                                      }),
                                tooltip: '更新',
                                icon: const Icon(Icons.refresh_rounded),
                              ),
                              IconButton(
                                onPressed: () async {
                                  if (await confirm(
                                    '移除订阅“${s['name']}”？现有节点会保留。',
                                  )) {
                                    await b.action(() async {
                                      await b.request(
                                        '/v1/subscriptions/${s['id']}',
                                        method: 'DELETE',
                                      );
                                    });
                                  }
                                },
                                tooltip: '移除订阅',
                                icon: const Icon(Icons.delete_outline_rounded),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
      ),
    ],
  );
  Widget routingPage() => ListView(
    children: [
      Card(
        child: Padding(
          padding: const EdgeInsets.all(22),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              settingDropdown('分流策略', b.settings['routing'] ?? 'rule', const {
                'rule': '按规则',
                'global': '全局代理',
                'direct': '全局直连',
              }, (v) => changeSetting('routing', v)),
              const SizedBox(height: 18),
              const Text('按规则模式默认让局域网直连，其余流量使用当前节点。自定义规则按域名后缀匹配。'),
              const SizedBox(height: 20),
              ruleEditor('直连域名', 'direct_domains'),
              const SizedBox(height: 18),
              ruleEditor('代理域名', 'proxy_domains'),
            ],
          ),
        ),
      ),
      const SizedBox(height: 16),
      Card(
        child: Padding(
          padding: const EdgeInsets.all(22),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.dns_outlined),
                title: Text('DNS · ${b.settings['dns'] ?? '1.1.1.1'}'),
                subtitle: const Text('通过当前出站使用 TCP 查询；节点地址通过系统 DNS 引导解析。'),
                trailing: TextButton(
                  onPressed: () => editTextSetting('DNS 服务器 IP', 'dns'),
                  child: const Text('修改'),
                ),
              ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('接管 IPv6'),
                subtitle: const Text('TUN 模式同时接管 IPv6 地址'),
                value: b.settings['ipv6'] == true,
                onChanged: b.busy ? null : (v) => changeSetting('ipv6', v),
              ),
            ],
          ),
        ),
      ),
    ],
  );
  Widget ruleEditor(String label, String key) {
    final items = List<String>.from(b.settings[key] as List? ?? []);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(label),
            TextButton(
              onPressed: () => editRules(label, key, items),
              child: const Text('编辑'),
            ),
          ],
        ),
        const SizedBox(height: 6),
        Text(
          items.isEmpty ? '未添加' : items.join(' · '),
          style: TextStyle(color: c.onSurfaceVariant),
        ),
      ],
    );
  }

  Future<void> editRules(String title, String key, List<String> items) async {
    final ctrl = TextEditingController(text: items.join('\n'));
    final ok =
        await managedDialog<bool>(
          context: context,
          builder: (ctx) => AlertDialog(
            title: Text(title),
            content: SizedBox(
              width: 480,
              child: TextField(
                controller: ctrl,
                minLines: 8,
                maxLines: 16,
                decoration: const InputDecoration(
                  hintText: '每行一个域名，例如 example.com',
                ),
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(ctx, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(ctx, true),
                child: const Text('保存'),
              ),
            ],
          ),
        ) ??
        false;
    if (ok) {
      changeSetting(
        key,
        ctrl.text
            .split('\n')
            .map((s) => s.trim())
            .where((s) => s.isNotEmpty)
            .toList(),
      );
    }
    ctrl.dispose();
  }

  Future<void> editTextSetting(
    String title,
    String key, {
    bool number = false,
  }) async {
    final ctrl = TextEditingController(text: b.settings[key]?.toString() ?? '');
    final ok =
        await managedDialog<bool>(
          context: context,
          builder: (ctx) => AlertDialog(
            title: Text(title),
            content: SizedBox(
              width: 430,
              child: TextField(
                controller: ctrl,
                keyboardType: number
                    ? TextInputType.number
                    : TextInputType.text,
              ),
            ),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(ctx, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(ctx, true),
                child: const Text('保存'),
              ),
            ],
          ),
        ) ??
        false;
    if (ok) {
      final value = number ? int.tryParse(ctrl.text) : ctrl.text.trim();
      if (value != null) changeSetting(key, value);
    }
    ctrl.dispose();
  }

  Future<void> loadActivity() async {
    try {
      final values = await Future.wait([
        b.request('/v1/logs'),
        b.request('/v1/connections'),
      ]);
      b.logs = Backend.maps(values[0]);
      b.connections = Backend.maps((values[1] as Map)['items'] ?? []);
      if (mounted) setState(() {});
    } catch (e) {
      toast(Backend.clean(e));
    }
  }

  Widget activityPage() => Column(
    children: [
      Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          Text(
            '${b.connections.length} 条连接 · ${b.logs.length} 条日志',
            style: TextStyle(color: c.onSurfaceVariant),
          ),
          const SizedBox(width: 16),
          OutlinedButton.icon(
            onPressed: loadActivity,
            icon: const Icon(Icons.refresh),
            label: const Text('刷新'),
          ),
        ],
      ),
      const SizedBox(height: 16),
      Expanded(
        child: ListView(
          children: [
            for (final n in b.connections)
              Card(
                child: ListTile(
                  dense: true,
                  title: Text(
                    '${n['metadata']?['host'] ?? n['metadata']?['destinationIP'] ?? n['destination'] ?? ''}',
                  ),
                  subtitle: Text(
                    '${n['metadata']?['network'] ?? ''} · ${n['chains'] ?? ''}',
                  ),
                ),
              ),
            for (final e in b.logs.reversed)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 7, horizontal: 8),
                child: SelectableText(
                  '${e['time']}  ${e['level']}  ${e['message']}',
                  style: TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 12,
                    color: e['level'] == 'error' ? c.error : c.onSurfaceVariant,
                  ),
                ),
              ),
            if (b.logs.isEmpty && b.connections.isEmpty)
              const Padding(
                padding: EdgeInsets.all(40),
                child: Center(child: Text('暂无连接或日志')),
              ),
          ],
        ),
      ),
    ],
  );

  Widget settingsPage() => ListView(
    children: [
      Card(
        child: Padding(
          padding: const EdgeInsets.all(22),
          child: Wrap(
            spacing: 24,
            runSpacing: 18,
            children: [
              settingDropdown('外观', b.settings['theme'] ?? 'system', const {
                'system': '跟随系统',
                'light': '浅色',
                'dark': '深色',
              }, (v) => changeSetting('theme', v)),
              settingDropdown('配色', b.settings['color'] ?? 'green', const {
                'green': '青绿（默认）',
                'amber': '暖金',
                'coral': '珊瑚',
                'blue': '蓝色',
                'violet': '紫色',
              }, (v) => changeSetting('color', v)),
            ],
          ),
        ),
      ),
      Card(
        child: Column(
          children: [
            SwitchListTile(
              title: const Text('紧凑列表'),
              value: b.settings['compact'] == true,
              onChanged: (v) => changeSetting('compact', v),
            ),
            SwitchListTile(
              title: const Text('后台启动后自动连接'),
              subtitle: const Text('使用上次选择的节点与接管方式'),
              value: b.settings['auto_connect'] == true,
              onChanged: (v) => changeSetting('auto_connect', v),
            ),
            ListTile(
              title: const Text('本地代理端口'),
              subtitle: Text('${b.settings['port'] ?? 10808} · HTTP / SOCKS5'),
              trailing: TextButton(
                onPressed: () => editTextSetting('本地端口', 'port', number: true),
                child: const Text('修改'),
              ),
            ),
            ListTile(
              title: const Text('连接测试地址'),
              subtitle: Text(b.settings['test_url']?.toString() ?? ''),
              trailing: TextButton(
                onPressed: () => editTextSetting('连接测试地址', 'test_url'),
                child: const Text('修改'),
              ),
            ),
          ],
        ),
      ),
      const SizedBox(height: 12),
      Card(
        child: Padding(
          padding: const EdgeInsets.all(18),
          child: Column(
            children: [
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  const Text('外部协议插件'),
                  TextButton.icon(
                    onPressed: installPlugin,
                    icon: const Icon(Icons.add),
                    label: const Text('添加插件'),
                  ),
                ],
              ),
              for (final p in b.plugins)
                ListTile(
                  dense: true,
                  title: Text('${p['name']} · ${p['kind']}'),
                  subtitle: Text(p['path'].toString(), maxLines: 2),
                  trailing: IconButton(
                    onPressed: () => b.action(() async {
                      await b.request(
                        '/v1/plugins/${p['id']}',
                        method: 'DELETE',
                      );
                    }),
                    icon: const Icon(Icons.delete_outline),
                  ),
                ),
              if (b.plugins.isEmpty)
                const Padding(
                  padding: EdgeInsets.all(12),
                  child: Text('为 NaïveProxy、Trojan-Go 或 Mieru 选择对应桌面程序。'),
                ),
            ],
          ),
        ),
      ),
      const SizedBox(height: 12),
      Card(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Wrap(
            spacing: 12,
            runSpacing: 12,
            children: [
              OutlinedButton.icon(
                onPressed: exportBackup,
                icon: const Icon(Icons.save_alt),
                label: const Text('导出备份'),
              ),
              OutlinedButton.icon(
                onPressed: restoreBackup,
                icon: const Icon(Icons.restore),
                label: const Text('导入备份'),
              ),
              TextButton(
                onPressed: () =>
                    Clipboard.setData(ClipboardData(text: Backend.dataDir)),
                child: const Text('复制数据目录'),
              ),
            ],
          ),
        ),
      ),
      const Padding(
        padding: EdgeInsets.all(20),
        child: Text(
          'ShadowBox 0.1.0 · GPL-3.0-or-later\nNekoBox 同款 sing-box 内核。故障时优先恢复原网络设置。',
          style: TextStyle(fontSize: 12),
        ),
      ),
    ],
  );
  Future<void> installPlugin() async {
    String kind = 'naive';
    final ok =
        await managedDialog<bool>(
          context: context,
          builder: (ctx) => StatefulBuilder(
            builder: (ctx, update) => AlertDialog(
              title: const Text('添加桌面协议插件'),
              content: SizedBox(
                width: 430,
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    DropdownButtonFormField<String>(
                      initialValue: kind,
                      items: const [
                        DropdownMenuItem(
                          value: 'naive',
                          child: Text('NaïveProxy'),
                        ),
                        DropdownMenuItem(
                          value: 'trojan-go',
                          child: Text('Trojan-Go'),
                        ),
                        DropdownMenuItem(value: 'mieru', child: Text('Mieru')),
                      ],
                      onChanged: (v) => update(() => kind = v!),
                    ),
                    const SizedBox(height: 14),
                    const Text('插件会在本机运行。请选择与你的系统和 CPU 架构匹配、来源可信的可执行程序。'),
                  ],
                ),
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.pop(ctx, false),
                  child: const Text('取消'),
                ),
                FilledButton(
                  onPressed: () => Navigator.pop(ctx, true),
                  child: const Text('选择程序'),
                ),
              ],
            ),
          ),
        ) ??
        false;
    if (!ok) return;
    final file = await openFile();
    if (file == null) return;
    await b.action(() async {
      await b.request(
        '/v1/plugins',
        method: 'POST',
        body: {'name': kind, 'kind': kind, 'path': file.path},
      );
    });
  }

  Future<void> exportBackup() async {
    if (!await confirm('备份包含订阅地址与节点密码，请妥善保管。')) return;
    try {
      final loc = await getSaveLocation(suggestedName: 'shadowbox-backup.json');
      if (loc == null) return;
      final data = await b.request('/v1/backup');
      await XFile.fromData(
        Uint8List.fromList(
          utf8.encode(const JsonEncoder.withIndent('  ').convert(data)),
        ),
        mimeType: 'application/json',
      ).saveTo(loc.path);
      toast('备份已保存');
    } catch (e) {
      toast(Backend.clean(e));
    }
  }

  Future<void> restoreBackup() async {
    final file = await openFile(
      acceptedTypeGroups: [
        const XTypeGroup(label: 'ShadowBox 备份', extensions: ['json']),
      ],
    );
    if (file == null) return;
    if (await file.length() > 16 * 1024 * 1024) {
      toast('备份文件过大');
      return;
    }
    try {
      final data = jsonDecode(await file.readAsString());
      if (await confirm('将备份中的节点和订阅合并到当前配置？')) {
        await b.action(() async {
          await b.request('/v1/backup', method: 'POST', body: data);
        });
      }
    } catch (e) {
      toast(Backend.clean(e));
    }
  }
}
