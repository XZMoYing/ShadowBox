import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';
import 'package:shadowbox/backend.dart';
import 'package:shadowbox/main.dart';
import 'package:shadowbox/node_form.dart';

class FakeBackend extends Backend {
  int starts = 0;
  FakeBackend() {
    endpoint = 'http://127.0.0.1:1';
    loading = false;
    settings = {
      'capture': 'manual',
      'routing': 'rule',
      'port': 10808,
      'theme': 'light',
      'color': 'green',
    };
    nodes = [
      {
        'id': 'n',
        'name': 'Local test node',
        'group': '本地节点',
        'outbound': {'type': 'socks'},
        'latency_ms': -1,
      },
    ];
    status = {
      'selected': 'n',
      'running': false,
      'core_version': '1.12.19-neko-1',
      'stats': {},
    };
  }
  @override
  Future<void> initialize() async {}
  @override
  Future<void> refresh() async {
    notifyListeners();
  }

  @override
  Future<void> connect([String? id]) async {
    starts++;
    status = {
      ...status,
      'running': true,
      'active_id': 'n',
      'active_name': 'Local test node',
    };
  }

  @override
  Future<dynamic> request(
    String path, {
    String method = 'GET',
    dynamic body,
    List<int>? bytes,
    String? filename,
  }) async {
    if (path == '/v1/import/preview') {
      return {
        'nodes': [
          {
            'id': 'new',
            'name': 'Imported node',
            'group': '本地节点',
            'outbound': {'type': 'trojan'},
          },
        ],
        'errors': ['第 2 行格式无效'],
      };
    }
    if (path == '/v1/nodes' && method == 'POST') {
      nodes.addAll(Backend.maps(body['nodes']));
      return {'count': 1};
    }
    return {};
  }
}

void main() {
  testWidgets('all desktop pages fit the minimum window size', (tester) async {
    tester.view.physicalSize = const Size(850, 620);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend();
    await tester.pumpWidget(ShadowBox(backend: backend));
    await tester.pumpAndSettle();
    for (final label in ['节点', '订阅', '路由', '活动', '设置', '连接']) {
      await tester.tap(
        find.descendant(
          of: find.byType(NavigationRail),
          matching: find.text(label),
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull, reason: label);
    }
  });
  testWidgets(
    'manual form validates address and produces a real Shadowsocks outbound',
    (tester) async {
      Json? saved;
      await tester.pumpWidget(
        MaterialApp(
          home: NodeForm(
            onSave: (n) async {
              saved = n;
            },
          ),
        ),
      );
      await tester.tap(find.widgetWithText(FilledButton, '保存'));
      await tester.pumpAndSettle();
      expect(find.text('请填写有效的服务器地址和端口'), findsOneWidget);
      final address = find.widgetWithText(TextField, '服务器地址');
      await tester.enterText(address, '127.0.0.1');
      await tester.enterText(
        find.widgetWithText(TextField, '密码'),
        'test-password',
      );
      await tester.tap(find.widgetWithText(FilledButton, '保存'));
      await tester.pumpAndSettle();
      expect(saved?['outbound'], {
        'type': 'shadowsocks',
        'server': '127.0.0.1',
        'server_port': 443,
        'password': 'test-password',
        'method': 'aes-128-gcm',
      });
    },
  );
  testWidgets(
    'connection uses the selected profile and the default theme is green',
    (tester) async {
      tester.view.resetPhysicalSize();
      tester.view.physicalSize = const Size(1160, 780);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final backend = FakeBackend();
      await tester.pumpWidget(ShadowBox(backend: backend));
      await tester.pumpAndSettle();
      expect(find.text('Local test node'), findsOneWidget);
      final theme = Theme.of(tester.element(find.byType(Scaffold)));
      expect(
        theme.colorScheme.primary.g,
        greaterThan(theme.colorScheme.primary.b),
      );
      final connect = find.ancestor(
        of: find.byIcon(Icons.power_settings_new_rounded).last,
        matching: find.byType(FilledButton),
      );
      await tester.tap(connect);
      await tester.pumpAndSettle();
      expect(backend.starts, 1);
      expect(find.text('已连接'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );
  testWidgets('import previews errors before committing usable profiles', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1160, 780);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final backend = FakeBackend();
    await tester.pumpWidget(ShadowBox(backend: backend));
    await tester.pumpAndSettle();
    await tester.tap(find.text('节点').first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('添加'));
    await tester.pumpAndSettle();
    await tester.enterText(
      find.byType(TextField).last,
      'trojan://secret@example.com#Imported',
    );
    await tester.tap(find.text('预览节点'));
    await tester.pumpAndSettle();
    expect(find.text('第 2 行格式无效'), findsOneWidget);
    expect(backend.nodes.length, 1);
    await tester.tap(find.text('导入可用节点'));
    await tester.pumpAndSettle();
    expect(backend.nodes.length, 2);
    expect(find.text('Imported node'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
